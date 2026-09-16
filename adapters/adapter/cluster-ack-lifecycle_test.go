package adapter

import (
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type reentrantClusterAckValue struct{ again func() }

func TestClusterBroadcastAckExpiresDuringEncoding(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cluster, _ := newClusterPublishTestAdapter(t, nil)
		defer cluster.Close()
		started, release := make(chan struct{}), make(chan struct{})
		value := reentrantClusterAckValue{again: sync.OnceFunc(func() {
			close(started)
			<-release
		})}
		returned := make(chan struct{})
		go func() {
			cluster.BroadcastWithAck(
				&parser.Packet{Type: parser.EVENT, Data: []any{"event", value}},
				&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: new(float64(10))}},
				func(uint64) {}, func([]any, error) {},
			)
			close(returned)
		}()
		<-started
		time.Sleep(20 * time.Millisecond)
		synctest.Wait()
		pending := cluster.ackRequests.Len()
		close(release)
		<-returned
		if pending != 0 {
			t.Fatalf("ACK requests after timeout while encoding was blocked = %d, want 0", pending)
		}
		if cluster.ackRequests.Len() != 0 {
			t.Fatal("finishing encoding restored an expired ACK request")
		}
	})
}

func TestClusterBroadcastCountsEachNodeOnce(t *testing.T) {
	cluster, _ := newClusterPublishTestAdapter(t, nil)
	defer cluster.Close()
	cluster.Adapter.(*fixedServerCountAdapter).serverCount = 3
	var countCalls, clientCount, ackCalls atomic.Uint64
	cluster.BroadcastWithAck(&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
		&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: new(float64(1000))}},
		func(count uint64) { countCalls.Add(1); clientCount.Add(count) },
		func([]any, error) { ackCalls.Add(1) })
	requestId := cluster.ackRequests.Keys()[0]
	response := &ClusterResponse{Uid: "A", Type: BROADCAST_CLIENT_COUNT,
		Data: &BroadcastClientCount{RequestId: requestId, ClientCount: 2}}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { cluster.OnResponse(response) })
	}
	wg.Wait()
	cluster.OnResponse(&ClusterResponse{Uid: "B", Type: BROADCAST_CLIENT_COUNT,
		Data: &BroadcastClientCount{RequestId: requestId, ClientCount: 1}})
	if countCalls.Load() != 3 || clientCount.Load() != 3 {
		t.Fatalf("server count callbacks/client count = %d/%d, want 3/3", countCalls.Load(), clientCount.Load())
	}
	// Two clients on the same node may acknowledge with the very same value.
	for range 2 {
		cluster.OnResponse(&ClusterResponse{Uid: "A", Type: BROADCAST_ACK,
			Data: &BroadcastAck{RequestId: requestId, Packet: "same"}})
	}
	if ackCalls.Load() != 2 {
		t.Fatalf("client ACK callbacks = %d, want 2", ackCalls.Load())
	}
}

func (v reentrantClusterAckValue) MarshalJSON() ([]byte, error) {
	v.again()
	return []byte(`"first"`), nil
}

func TestClusterServerSideAckCanReenterDuringEncoding(t *testing.T) {
	cluster, transport := newClusterPublishTestAdapter(t, nil)
	defer cluster.Close()
	var responses atomic.Int64
	transport.onResponse = func(*ClusterResponse) { responses.Add(1) }
	if err := cluster.Nsp().On("event", func(args ...any) {
		ack := args[len(args)-1].(socket.Ack)
		ack([]any{reentrantClusterAckValue{again: func() { ack([]any{"duplicate"}, nil) }}}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		cluster.OnMessage(&ClusterMessage{Uid: "peer", Nsp: cluster.Nsp().Name(), Type: SERVER_SIDE_EMIT,
			Data: &ServerSideEmitMessage{RequestId: new("request"), Packet: []any{"event"}}}, "")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ACK encoding deadlocked on repeated acknowledgement")
	}
	cluster.responses.Close()
	response := transport.response.Load()
	if responses.Load() != 1 || response == nil || response.Data.(*ServerSideEmitResponse).Packet != "first" {
		t.Fatalf("responses = %d, last = %#v; want one response with first value", responses.Load(), response)
	}
}

func TestClusterRemoteBroadcastAckExpires(t *testing.T) {
	for _, blockWrite := range []bool{false, true} {
		name := "normal delivery"
		if blockWrite {
			name = "registration after blocked write"
		}
		t.Run(name, func(t *testing.T) {
			server := socket.NewServer(nil, nil)
			defer server.Close(nil)
			connected := make(chan *socket.Socket, 2)
			if err := server.On("connection", func(args ...any) { connected <- args[0].(*socket.Socket) }); err != nil {
				t.Fatal(err)
			}
			httpServer := httptest.NewServer(server.ServeHandler(nil))
			defer httpServer.Close()
			var clients []*socket.Socket
			connections := make([]*websocket.Conn, 0, 2)
			for range 2 {
				conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/socket.io/?EIO=4&transport=websocket", nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = conn.Close() }()
				if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
					t.Fatal(err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatal(err)
				}
				if err := conn.WriteMessage(websocket.TextMessage, []byte("40")); err != nil {
					t.Fatal(err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatal(err)
				}
				select {
				case client := <-connected:
					clients = append(clients, client)
				case <-time.After(time.Second):
					t.Fatal("namespace did not connect")
				}
				connections = append(connections, conn)
			}
			cluster := NewClusterAdapter(server.Sockets()).(*clusterAdapter)
			defer cluster.Close()
			cluster.Adapter = server.Sockets().Adapter()
			started, release := make(chan struct{}), make(chan struct{})
			releaseWrite := sync.OnceFunc(func() { close(release) })
			defer releaseWrite()
			if blockWrite {
				var once sync.Once
				for _, client := range clients {
					client.OnAnyOutgoing(func(...any) { once.Do(func() { close(started); <-release }) })
				}
			}
			var ackCalls atomic.Int64
			transport := &testClusterAdapter{ClusterAdapter: cluster, onResponse: func(response *ClusterResponse) {
				if response.Type == BROADCAST_ACK {
					ackCalls.Add(1)
				}
			}}
			cluster.Prototype(transport)
			delivered := make(chan struct{})
			go func() {
				cluster.OnMessage(&ClusterMessage{Uid: "peer", Nsp: "/", Type: BROADCAST, Data: &BroadcastMessage{
					RequestId: new("remote-request"),
					Opts:      EncodeOptions(&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: new(float64(20))}}),
					Packet:    &parser.Packet{Type: parser.EVENT, Data: []any{"ignored"}},
				}}, "")
				close(delivered)
			}()
			waitForExpiry := func() {
				t.Helper()
				deadline := time.Now().Add(time.Second)
				for {
					count := 0
					for _, client := range clients {
						count += client.Acks().Len()
					}
					if count == 0 {
						return
					}
					if time.Now().After(deadline) {
						t.Fatalf("%d Socket ACK registrations survived the broadcast timeout", count)
					}
					time.Sleep(time.Millisecond)
				}
			}
			if blockWrite {
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("outgoing write did not block")
				}
				waitForExpiry()
				releaseWrite()
			}
			select {
			case <-delivered:
			case <-time.After(time.Second):
				t.Fatal("broadcast did not complete local delivery")
			}
			for _, conn := range connections {
				_, wire, err := conn.ReadMessage()
				if err != nil || !strings.HasPrefix(string(wire), "42") {
					t.Fatalf("broadcast was not delivered: %s, %v", wire, err)
				}
			}
			waitForExpiry()
			cluster.responses.Close()
			if ackCalls.Load() != 0 {
				t.Fatal("expiry synthesized a client acknowledgement")
			}
		})
	}
}
