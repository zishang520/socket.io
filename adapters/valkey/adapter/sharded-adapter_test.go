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

	vk "github.com/valkey-io/valkey-go"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
)

// fakeValkey answers just enough RESP2 for the client handshake and sharded
// Pub/Sub. Each SSUBSCRIBE reply waits for a token on release, so the caller
// stays inside its critical section until the test lets it go.
type fakeValkey struct {
	inFlight, maxInFlight atomic.Int32
	started               chan struct{}
	release               chan struct{}
}

func (f *fakeValkey) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	for {
		args, err := readRESPCommand(r)
		if err != nil {
			return
		}
		switch kind := strings.ToLower(args[0]); kind {
		case "hello":
			_, _ = conn.Write([]byte("-ERR unknown command 'HELLO'\r\n"))
		case "ssubscribe":
			n := f.inFlight.Add(1)
			for {
				m := f.maxInFlight.Load()
				if n <= m || f.maxInFlight.CompareAndSwap(m, n) {
					break
				}
			}
			f.started <- struct{}{}
			<-f.release
			f.inFlight.Add(-1)
			fallthrough
		case "sunsubscribe":
			for i, ch := range args[1:] {
				_, _ = fmt.Fprintf(conn, "*3\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n:%d\r\n", len(kind), kind, len(ch), ch, i+1)
			}
		case "ping":
			_, _ = conn.Write([]byte("+PONG\r\n"))
		default:
			_, _ = conn.Write([]byte("+OK\r\n"))
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

// An unsubscribe for a channel must not drop the per-channel lock while a
// subscribe is still waiting on it, or a later subscribe gets a fresh lock
// and subscribes the same channel a second time.
func TestShardedAdapter_ChannelLockKeepsExclusion(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	f := &fakeValkey{started: make(chan struct{}, 16), release: make(chan struct{}, 16)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()

	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:       []string{ln.Addr().String()},
		DisableCache:      true,
		ForceSingleClient: true,
		ClientSetInfo:     vk.DisableClientSetInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	s := MakeShardedValkeyAdapter().(*shardedValkeyAdapter)
	s.SetValkey(valkey.NewValkeyClient(context.Background(), client))
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	const channel = "socket.io#/#room#"
	waitSubscribe := func(who string) {
		t.Helper()
		select {
		case <-f.started:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not start subscribing", who)
		}
	}

	var wg sync.WaitGroup
	run := func(fn func(string)) {
		wg.Add(1)
		go func() { defer wg.Done(); fn(channel) }()
	}

	// Subscribe #1 holds the channel lock while it waits for the server.
	run(s.subscribeChannel)
	waitSubscribe("first subscribe")

	// An unsubscribe and subscribe #2 queue up on the same lock.
	run(s.unsubscribeChannel)
	time.Sleep(50 * time.Millisecond)
	run(s.subscribeChannel)
	time.Sleep(50 * time.Millisecond)

	// #1 finishes; the unsubscribe runs and subscribe #2 starts again.
	f.release <- struct{}{}
	waitSubscribe("second subscribe")

	// Subscribe #3 must wait for #2, not run beside it.
	run(s.subscribeChannel)
	time.Sleep(100 * time.Millisecond)

	close(f.release)
	wg.Wait()

	if got := f.maxInFlight.Load(); got != 1 {
		t.Fatalf("expected at most 1 subscribe in flight for %q, got %d", channel, got)
	}
}
