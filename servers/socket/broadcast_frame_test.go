package socket

import (
	"errors"
	"testing"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type preparedFrameForTest interface {
	PreparedWebSocketFrame(func(types.BufferInterface) (any, error)) (any, error)
	PreparedWebTransportFrame(func(types.BufferInterface) (any, error)) (any, error)
}

func TestBroadcastFrameCachesPreparedMessages(t *testing.T) {
	frame, ok := newBroadcastFrame(types.NewStringBufferString("42/test,[\"event\"]")).(preparedFrameForTest)
	if !ok {
		t.Fatal("expected broadcast frame to expose prepared frame cache")
	}

	wsSentinel := &struct{ name string }{name: "websocket"}
	wsBuilds := 0
	for range 10 {
		got, err := frame.PreparedWebSocketFrame(func(types.BufferInterface) (any, error) {
			wsBuilds++
			return wsSentinel, nil
		})
		if err != nil {
			t.Fatalf("unexpected websocket cache error: %v", err)
		}
		if got != wsSentinel {
			t.Fatalf("unexpected websocket cached value: %p", got)
		}
	}
	if wsBuilds != 1 {
		t.Fatalf("expected websocket prepared frame to be built once, got %d", wsBuilds)
	}

	wtSentinel := &struct{ name string }{name: "webtransport"}
	wtBuilds := 0
	for range 10 {
		got, err := frame.PreparedWebTransportFrame(func(types.BufferInterface) (any, error) {
			wtBuilds++
			return wtSentinel, nil
		})
		if err != nil {
			t.Fatalf("unexpected webtransport cache error: %v", err)
		}
		if got != wtSentinel {
			t.Fatalf("unexpected webtransport cached value: %p", got)
		}
	}
	if wtBuilds != 1 {
		t.Fatalf("expected webtransport prepared frame to be built once, got %d", wtBuilds)
	}
}

func TestBroadcastFrameDoesNotCacheErrors(t *testing.T) {
	frame := newBroadcastFrame(types.NewStringBufferString("42/test,[\"event\"]")).(preparedFrameForTest)
	expectedErr := errors.New("build failed")
	builds := 0

	if _, err := frame.PreparedWebSocketFrame(func(types.BufferInterface) (any, error) {
		builds++
		return nil, expectedErr
	}); !errors.Is(err, expectedErr) {
		t.Fatalf("expected first build error, got %v", err)
	}

	sentinel := &struct{}{}
	got, err := frame.PreparedWebSocketFrame(func(types.BufferInterface) (any, error) {
		builds++
		return sentinel, nil
	})
	if err != nil {
		t.Fatalf("expected second build to retry after error: %v", err)
	}
	if got != sentinel {
		t.Fatalf("unexpected cached value after retry: %p", got)
	}

	got, err = frame.PreparedWebSocketFrame(func(types.BufferInterface) (any, error) {
		builds++
		return &struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("unexpected cached success error: %v", err)
	}
	if got != sentinel {
		t.Fatal("expected successful prepared frame to be cached")
	}
	if builds != 2 {
		t.Fatalf("expected one failed build and one successful build, got %d", builds)
	}
}

func TestAdapterEncodeUsesBroadcastScopedFrame(t *testing.T) {
	adapter := newTestAdapter().(*adapter)
	packetOpts := &WriteOptions{}
	encoded := adapter._encode(&parser.Packet{
		Type: parser.EVENT,
		Data: []any{"event", "payload"},
	}, packetOpts)

	if len(encoded) != 1 {
		t.Fatalf("expected single encoded packet, got %d", len(encoded))
	}
	if packetOpts.WsPreEncodedFrame == nil {
		t.Fatal("expected pre-encoded frame for single string packet")
	}
	if _, ok := packetOpts.WsPreEncodedFrame.(preparedFrameForTest); !ok {
		t.Fatalf("expected pre-encoded frame to carry broadcast-scoped cache, got %T", packetOpts.WsPreEncodedFrame)
	}
	if got := string(packetOpts.WsPreEncodedFrame.Bytes()); got == "" || got[0] != '4' {
		t.Fatalf("expected Engine.IO message prefix in pre-encoded frame, got %q", got)
	}
}
