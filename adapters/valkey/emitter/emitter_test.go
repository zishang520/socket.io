package emitter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	vk "github.com/valkey-io/valkey-go"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type encodeOnly struct {
	called bool
}

type recordingClient struct {
	vk.Client
	commands [][]string
}

func (c *recordingClient) Do(_ context.Context, command vk.Completed) vk.ValkeyResult {
	c.commands = append(c.commands, append([]string(nil), command.Commands()...))
	return vk.ValkeyResult{}
}

func (e *encodeOnly) Encode(value any) ([]byte, error) {
	e.called = true
	return utils.MsgPack().Encode(value)
}

func newEmitterTestClient(t *testing.T) (*valkey.ValkeyClient, string) {
	t.Helper()
	server := miniredis.RunT(t)
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{server.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	valkeyClient, err := valkey.NewValkeyClient(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	return valkeyClient, server.Addr()
}

func subscribe(t *testing.T, address, channel string) <-chan vk.PubSubMessage {
	t.Helper()
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{address},
		DisableCache: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ready := make(chan struct{})
	messages := make(chan vk.PubSubMessage, 1)
	var once sync.Once
	ctx = vk.WithOnSubscriptionHook(ctx, func(subscription vk.PubSubSubscription) {
		if subscription.Channel == channel {
			once.Do(func() { close(ready) })
		}
	})
	go func() {
		_ = client.Receive(ctx, client.B().Subscribe().Channel(channel).Build(), func(message vk.PubSubMessage) {
			messages <- message
		})
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("subscription was not confirmed")
	}
	return messages
}

var _ EmitterOptionsInterface = (*EmitterOptions)(nil)

func TestEmitterOptionsUseOutboundEncoder(t *testing.T) {
	opts := DefaultEmitterOptions()
	if opts.GetRawEncoder() != nil || opts.Encoder() != nil {
		t.Fatal("default encoder must be unset")
	}
	encoder := new(encodeOnly)
	opts.SetEncoder(encoder)
	if opts.Encoder() != encoder {
		t.Fatal("encoder was not preserved")
	}
}

func TestClassicEmitterNodeWire(t *testing.T) {
	valkeyClient, address := newEmitterTestClient(t)
	room := socket.Room("room")
	channel := "socket.io#/chat#room#"
	messages := subscribe(t, address, channel)

	if err := NewEmitter(valkeyClient, nil, "/chat").To(room).Emit("event", "value"); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		var packet valkey.ValkeyPacket
		if err := utils.MsgPack().Decode([]byte(message.Message), &packet); err != nil {
			t.Fatal(err)
		}
		if packet.Uid != adapter.EMITTER_UID || packet.Packet.Nsp != "/chat" ||
			packet.Packet.Type != parser.EVENT || !reflect.DeepEqual(packet.Packet.Data, []any{"event", "value"}) {
			t.Fatalf("packet = %#v", packet)
		}
		if !reflect.DeepEqual(packet.Opts.Rooms, []socket.Room{room}) || packet.Opts.Except == nil || packet.Opts.Flags == nil {
			t.Fatalf("options = %#v", packet.Opts)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for emitted packet")
	}
}

func TestClassicEmitterAcceptsEncodeOnlyEncoder(t *testing.T) {
	valkeyClient, _ := newEmitterTestClient(t)
	encoder := new(encodeOnly)
	opts := DefaultEmitterOptions()
	opts.SetEncoder(encoder)
	if err := NewEmitter(valkeyClient, opts).Emit("event"); err != nil {
		t.Fatal(err)
	}
	if !encoder.called {
		t.Fatal("encode-only encoder was not called")
	}
}

func TestClassicRequestWire(t *testing.T) {
	valkeyClient, address := newEmitterTestClient(t)
	const channel = "socket.io-request#/#"
	messages := subscribe(t, address, channel)
	emitter := NewEmitter(valkeyClient, nil)

	if err := emitter.DisconnectSockets(false); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		var request map[string]any
		if err := json.Unmarshal([]byte(message.Message), &request); err != nil {
			t.Fatal(err)
		}
		closeValue, exists := request["close"]
		if !exists || closeValue != false {
			t.Fatalf("close = %#v, exists = %t", closeValue, exists)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for disconnect request")
	}

	if err := emitter.ServerSideEmit("event", "value"); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		var request valkey.ValkeyRequest
		if err := json.Unmarshal([]byte(message.Message), &request); err != nil {
			t.Fatal(err)
		}
		if request.Uid != adapter.EMITTER_UID || request.Type != valkey.SERVER_SIDE_EMIT ||
			!reflect.DeepEqual(request.Data, []any{"event", "value"}) {
			t.Fatalf("request = %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server-side emit request")
	}
}

func TestEmitterRoutingAndOperatorType(t *testing.T) {
	emitter := NewEmitter(nil, nil, "")
	if emitter.broadcastOptions.Nsp != "" || emitter.broadcastOptions.BroadcastChannel != "socket.io##" {
		t.Fatalf("namespace/channel = %q/%q", emitter.broadcastOptions.Nsp, emitter.broadcastOptions.BroadcastChannel)
	}
	if got := emitter.Of("").broadcastOptions.Nsp; got != "/" {
		t.Fatalf("Of empty namespace = %q, want /", got)
	}
	if got := emitter.Of("chat").broadcastOptions.Nsp; got != "/chat" {
		t.Fatalf("Of namespace = %q, want /chat", got)
	}

	opts := DefaultEmitterOptions()
	opts.SetSharded(true)
	sharded := NewEmitter(nil, opts)
	if _, ok := sharded.To("room").(*ShardedBroadcastOperator); !ok {
		t.Fatal("Emitter.To changed the sharded operator type")
	}
}

func TestShardedEmitterUsesCommonCodecAndDynamicRouting(t *testing.T) {
	client, _ := newEmitterTestClient(t)
	recorder := &recordingClient{Client: client.Client()}
	valkeyClient, err := valkey.NewValkeyClient(context.Background(), recorder)
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultEmitterOptions()
	opts.SetSharded(true)
	emitter := NewEmitter(valkeyClient, opts)

	if err = emitter.Emit("event", "value"); err != nil {
		t.Fatal(err)
	}
	if len(recorder.commands) != 1 || len(recorder.commands[0]) != 3 ||
		recorder.commands[0][0] != "SPUBLISH" || recorder.commands[0][1] != "socket.io#/#" {
		t.Fatalf("commands = %#v", recorder.commands)
	}
	message, err := adapter.DecodeClusterMessage([]byte(recorder.commands[0][2]))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || message.Uid != adapter.EMITTER_UID || message.Nsp != "/" ||
		message.Type != adapter.BROADCAST || data.Packet == nil ||
		!reflect.DeepEqual(data.Packet.Data, []any{"event", "value"}) {
		t.Fatalf("message = %#v, data = %#v", message, message.Data)
	}

	if err := emitter.To("public-room").Emit("event"); err != nil {
		t.Fatal(err)
	}
	if got := recorder.commands[1][1]; got != "socket.io#/#public-room#" {
		t.Fatalf("public-room channel = %q", got)
	}
	if err := emitter.To("abcdefghijklmnopqrst").Emit("event"); err != nil {
		t.Fatal(err)
	}
	if got := recorder.commands[2][1]; got != "socket.io#/#" {
		t.Fatalf("socket room channel = %q", got)
	}
}

func TestServerSideEmitRejectsAcknowledgements(t *testing.T) {
	for _, operator := range []BroadcastOperatorInterface{
		MakeBroadcastOperator(),
		MakeShardedBroadcastOperator(),
		MakeValkeyStreamsBroadcastOperator(),
	} {
		if err := operator.ServerSideEmit("event", func([]any, error) {}); !errors.Is(err, errAcknowledgementsNotSupported) {
			t.Fatalf("error = %v, want %v", err, errAcknowledgementsNotSupported)
		}
	}
}
