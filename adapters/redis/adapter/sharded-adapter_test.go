package adapter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

// serveFakeRedis answers just enough RESP for a Pub/Sub connection:
// SSUBSCRIBE / SUNSUBSCRIBE / PING. Every other command gets an error.
func serveFakeRedis(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	for {
		args, err := readRESPCommand(r)
		if err != nil {
			return
		}
		switch kind := strings.ToLower(args[0]); kind {
		case "ssubscribe", "sunsubscribe":
			for i, ch := range args[1:] {
				_, _ = fmt.Fprintf(conn, "*3\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n:%d\r\n", len(kind), kind, len(ch), ch, i+1)
			}
		case "ping":
			_, _ = conn.Write([]byte("+PONG\r\n"))
		default:
			_, _ = conn.Write([]byte("-ERR unknown command\r\n"))
		}
	}
}

func readRESPCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
	if err != nil || n < 1 {
		return nil, fmt.Errorf("bad command header %q", line)
	}
	args := make([]string, 0, n)
	for range n {
		line, err = r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "$")))
		if err != nil {
			return nil, fmt.Errorf("bad bulk header %q", line)
		}
		buf := make([]byte, size+2)
		if _, err = io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:size]))
	}
	return args, nil
}

// A delete-room for a channel must not drop the per-channel lock while a
// create-room is still waiting on it, or a later create-room gets a fresh
// lock and subscribes the same channel a second time.
func TestShardedAdapter_NonClusterChannelLockKeepsExclusion(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakeRedis(conn)
		}
	}()

	// Every new connection waits for a token, so a subscribeNode call stays
	// inside its critical section until the test lets it go.
	var inDial, maxInDial atomic.Int32
	dialStarted := make(chan struct{}, 16)
	release := make(chan struct{}, 16)
	client := rds.NewClient(&rds.Options{
		Addr:            ln.Addr().String(),
		DisableIdentity: true,
		MaxRetries:      -1,
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			n := inDial.Add(1)
			for {
				m := maxInDial.Load()
				if n <= m || maxInDial.CompareAndSwap(m, n) {
					break
				}
			}
			dialStarted <- struct{}{}
			<-release
			inDial.Add(-1)
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	})
	defer func() { _ = client.Close() }()

	s := MakeShardedRedisAdapter().(*shardedRedisAdapter)
	s.SetRedis(redis.NewRedisClient(context.Background(), client))
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	const channel = "socket.io#/#room#"
	waitDial := func(who string) {
		t.Helper()
		select {
		case <-dialStarted:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not start subscribing", who)
		}
	}

	var wg sync.WaitGroup
	run := func(f func(string)) {
		wg.Add(1)
		go func() { defer wg.Done(); f(channel) }()
	}

	// create-room #1 holds the channel lock while it connects.
	run(s.subscribeNode)
	waitDial("first subscribe")

	// delete-room and create-room #2 queue up on the same lock.
	run(s.unsubscribeNode)
	time.Sleep(50 * time.Millisecond)
	run(s.subscribeNode)
	time.Sleep(50 * time.Millisecond)

	// #1 finishes; the delete-room runs and create-room #2 starts subscribing again.
	release <- struct{}{}
	waitDial("second subscribe")

	// create-room #3 must wait for #2, not run beside it.
	run(s.subscribeNode)
	time.Sleep(100 * time.Millisecond)

	close(release)
	wg.Wait()

	if got := maxInDial.Load(); got != 1 {
		t.Fatalf("expected at most 1 subscribe in flight for %q, got %d", channel, got)
	}
}
