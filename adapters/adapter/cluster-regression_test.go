package adapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestClusterLocalFetchSkipsServerCount(t *testing.T) {
	cluster, _ := newClusterPublishTestAdapter(t, nil)
	defer cluster.Close()
	transport := &prototypeClusterAdapter{ClusterAdapter: cluster, countErr: errors.New("backend unavailable")}
	cluster.Prototype(transport)
	var calls int
	cluster.FetchSockets(&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Local: true}})(func(_ []socket.SocketDetails, err error) {
		calls++
		if err != nil {
			t.Errorf("local FetchSockets returned backend error: %v", err)
		}
	})
	if calls != 1 || transport.serverCountCalls.Load() != 0 {
		t.Fatalf("callback/count calls = %d/%d, want 1/0", calls, transport.serverCountCalls.Load())
	}
}

func TestClusterResponsesDeduplicateNodes(t *testing.T) {
	for _, test := range []struct {
		name       string
		heartbeat  bool
		operation  string
		concurrent bool
	}{
		{"base/emit/sequential", false, "emit", false},
		{"base/emit/concurrent", false, "emit", true},
		{"base/fetch/sequential", false, "fetch", false},
		{"base/fetch/concurrent", false, "fetch", true},
		{"heartbeat/emit/sequential", true, "emit", false},
		{"heartbeat/emit/concurrent", true, "emit", true},
		{"heartbeat/fetch/sequential", true, "fetch", false},
		{"heartbeat/fetch/concurrent", true, "fetch", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cluster ClusterAdapter
			var transport *testClusterAdapter
			if test.heartbeat {
				h := newHeartbeatPublishTestAdapter(t, nil)
				h.nodesMap.Delete("remote")
				h.nodesMap.Store("A", time.Now().UnixMilli())
				h.nodesMap.Store("B", time.Now().UnixMilli())
				cluster = h
				transport = &testClusterAdapter{ClusterAdapter: h}
				h.Prototype(transport)
			} else {
				c, tr := newClusterPublishTestAdapter(t, nil)
				c.Adapter.(*fixedServerCountAdapter).serverCount = 3
				cluster, transport = c, tr
			}
			defer cluster.Close()
			transport.onPublish = func(message *ClusterMessage) {
				var requestId string
				switch data := message.Data.(type) {
				case *ServerSideEmitMessage:
					requestId = *data.RequestId
				case *FetchSocketsMessage:
					requestId = data.RequestId
				default:
					return // ADAPTER_CLOSE
				}
				respond := func(uid ServerId) {
					response := &ClusterResponse{Uid: uid}
					if test.operation == "emit" {
						response.Type = SERVER_SIDE_EMIT_RESPONSE
						response.Data = &ServerSideEmitResponse{RequestId: requestId, Packet: string(uid)}
					} else {
						response.Type = FETCH_SOCKETS_RESPONSE
						response.Data = &FetchSocketsResponse{RequestId: requestId, Sockets: []SocketResponse{{Id: socket.SocketId(uid)}}}
					}
					cluster.OnResponse(response)
				}
				if test.heartbeat {
					respond("unknown")
				}
				if test.concurrent {
					var wg sync.WaitGroup
					for range 16 {
						wg.Go(func() { respond("A") })
						wg.Go(func() { respond("B") })
					}
					wg.Wait()
				} else {
					respond("A")
					respond("A")
					respond("B")
				}
			}
			var calls atomic.Int64
			var got []any
			complete := func(values []any, err error) {
				calls.Add(1)
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				got = values
				cluster.Close() // Completion must release internal locks before user callbacks.
			}
			if test.operation == "emit" {
				if err := cluster.ServerSideEmit([]any{"event", complete}); err != nil {
					t.Fatal(err)
				}
			} else {
				cluster.FetchSockets(nil)(func(details []socket.SocketDetails, err error) {
					ids := make([]any, len(details))
					for i, detail := range details {
						ids[i] = string(detail.Id())
					}
					complete(ids, err)
				})
			}
			if calls.Load() != 1 || len(got) != 2 || got[0] == got[1] ||
				(got[0] != "A" && got[0] != "B") || (got[1] != "A" && got[1] != "B") {
				t.Fatalf("calls/responses = %d/%v, want one result containing A and B once", calls.Load(), got)
			}
		})
	}
}

type snapshotTaggedValue struct {
	Value string `json:"json_key" msgpack:"msgpack_key"`
}

type snapshotCodecValue struct {
	jsonErr      error
	msgpackErr   error
	value        string
	jsonCalls    int
	msgpackCalls int
}

func (v *snapshotCodecValue) MarshalJSON() ([]byte, error) {
	v.jsonCalls++
	if v.jsonErr != nil {
		return nil, v.jsonErr
	}
	return json.Marshal(v.value)
}

func (v *snapshotCodecValue) MarshalMsgpack() ([]byte, error) {
	v.msgpackCalls++
	if v.msgpackErr != nil {
		return nil, v.msgpackErr
	}
	return msgpack.Marshal(v.value)
}

func TestClusterPreparationPreservesTransportSemantics(t *testing.T) {
	for _, encode := range []func(*ClusterMessage) ([]byte, error){EncodeClusterMessage, EncodeClusterMessageMsgpack} {
		for _, value := range []any{int64(9007199254740993), float32(1.25), &snapshotTaggedValue{Value: "value"}} {
			cluster, transport := newClusterPublishTestAdapter(t, nil)
			defer cluster.Close()
			message := &ClusterMessage{Uid: cluster.Uid(), Nsp: cluster.Nsp().Name(), Type: SERVER_SIDE_EMIT_RESPONSE,
				Data: &ServerSideEmitResponse{RequestId: "r", Packet: value}}
			expected, err := encode(message)
			if err != nil {
				t.Fatal(err)
			}
			var prepared []byte
			transport.encode = func(message *ClusterMessage) ([]byte, error) {
				wire, encodeErr := encode(message)
				prepared = bytes.Clone(wire)
				return wire, encodeErr
			}
			_, err = cluster.PublishAndReturnOffset(message)
			if err != nil || !bytes.Equal(expected, prepared) {
				t.Fatalf("%T: wire changed: expected=%x prepared=%x err=%v", value, expected, prepared, err)
			}
		}
	}
}

func TestClusterPreparationUsesOnlyTransportCodec(t *testing.T) {
	unsupported := errors.New("unused codec must not run")
	for _, jsonOnly := range []bool{false, true} {
		cluster, transport := newClusterPublishTestAdapter(t, nil)
		defer cluster.Close()
		value := &snapshotCodecValue{value: "before", jsonErr: unsupported}
		transport.encode = EncodeClusterMessageMsgpack
		if jsonOnly {
			value.jsonErr, value.msgpackErr = nil, unsupported
			transport.encode = EncodeClusterMessage
		}
		var got any
		// Keep the worker occupied while a second message is encoded and queued.
		blocked, release := make(chan struct{}), make(chan struct{})
		transport.onPublish = func(message *ClusterMessage) {
			if message.Type == HEARTBEAT {
				close(blocked)
				<-release
				return
			}
			if message.Type != SERVER_SIDE_EMIT_RESPONSE {
				return
			}
			got = message.Data.(*ServerSideEmitResponse).Packet
		}
		cluster.Publish(&ClusterMessage{Type: HEARTBEAT})
		<-blocked
		cluster.Publish(&ClusterMessage{Type: SERVER_SIDE_EMIT_RESPONSE, Data: &ServerSideEmitResponse{RequestId: "r", Packet: value}})
		value.value = "after"
		close(release)
		// A synchronous final publish waits for the preceding message to complete.
		if _, err := cluster.PublishAndReturnOffset(&ClusterMessage{Type: ADAPTER_CLOSE}); err != nil {
			t.Fatal(err)
		}
		wantJSON, wantMsgpack := 0, 1
		if jsonOnly {
			wantJSON, wantMsgpack = 1, 0
		}
		if value.jsonCalls != wantJSON || value.msgpackCalls != wantMsgpack {
			t.Fatalf("JSON/MessagePack calls=%d/%d, want %d/%d", value.jsonCalls, value.msgpackCalls, wantJSON, wantMsgpack)
		}
		if got != "before" {
			t.Fatalf("published value=%#v, want before", got)
		}
	}
}

type snapshotClosingValue struct{ close func() }

func (v snapshotClosingValue) MarshalJSON() ([]byte, error) {
	v.close()
	return []byte(`"value"`), nil
}

func TestHeartbeatPreparationCanCloseAdapter(t *testing.T) {
	cluster := newHeartbeatPublishTestAdapter(t, nil)
	done := make(chan struct{})
	go func() {
		cluster.Publish(&ClusterMessage{Type: SERVER_SIDE_EMIT, Data: &ServerSideEmitMessage{
			Packet: []any{"event", snapshotClosingValue{close: cluster.Close}},
		}})
		close(done)
	}()
	select {
	case <-done:
		cluster.Close()
	case <-time.After(time.Second):
		utils.ClearInterval(cluster.cleanupTimer.Swap(nil))
		utils.ClearTimeout(cluster.heartbeatTimer.Swap(nil))
		t.Fatal("preparation deadlocked while MarshalJSON reentered Close")
	}
}

func TestHeartbeatClosePreparation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		cluster := newHeartbeatPublishTestAdapter(t, nil)
		transport := &testClusterAdapter{ClusterAdapter: cluster}
		transport.encode = func(message *ClusterMessage) ([]byte, error) {
			if message.Type == ADAPTER_CLOSE {
				cluster.Close()
				if fail {
					return nil, errors.New("cannot encode close")
				}
			}
			return EncodeClusterMessage(message)
		}
		cluster.Prototype(transport)
		done := make(chan struct{})
		go func() { cluster.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("close preparation deadlocked on reentrant Close")
		}
		if _, err := cluster.PublishAndReturnOffset(&ClusterMessage{Type: HEARTBEAT}); !errors.Is(err, ErrAdapterClosed) {
			t.Fatalf("publish after close error = %v, want ErrAdapterClosed", err)
		}
	}
}
