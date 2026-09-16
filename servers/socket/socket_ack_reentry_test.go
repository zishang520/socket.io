package socket

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSocketAckCanReenterOnEncodingError(t *testing.T) {
	encodingError := make(chan any, 1)
	returned := make(chan struct{})
	conn := connectAckTestSocket(t, func(client *Socket) {
		if err := client.On("request", func(args ...any) {
			ack := args[len(args)-1].(Ack)
			if err := client.On("error", func(args ...any) {
				encodingError <- args[0]
				ack([]any{"fallback"}, nil)
			}); err != nil {
				t.Error(err)
				return
			}
			ack([]any{make(chan int)}, nil)
			close(returned)
			if err := client.Emit("after"); err != nil {
				t.Error(err)
			}
		}); err != nil {
			t.Error(err)
		}
	})
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`420["request"]`)); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-encodingError:
		err, ok := value.(error)
		var unsupported *json.UnsupportedTypeError
		if !ok || !errors.As(err, &unsupported) || unsupported.Type != reflect.TypeFor[chan int]() {
			t.Fatalf("encoding error = %T (%v), want UnsupportedTypeError for chan int", value, value)
		}
	case <-time.After(time.Second):
		t.Fatal("ACK encoding error was not reported")
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("same ACK reentered from its encoding error handler deadlocked")
	}
	// The first invocation owns the ACK even when encoding fails. Its duplicate
	// must return without sending a fallback or blocking the next valid packet.
	_, wire, err := conn.ReadMessage()
	if err != nil || string(wire) != `42["after"]` {
		t.Fatalf("next packet = %q, %v; want the event after the failed ACK", wire, err)
	}
}

func TestSocketAckSendsOnceConcurrently(t *testing.T) {
	conn := connectAckTestSocket(t, func(client *Socket) {
		if err := client.On("request", func(args ...any) {
			ack := args[len(args)-1].(Ack)
			var callers sync.WaitGroup
			for range 20 {
				callers.Go(func() { ack([]any{"ok"}, nil) })
			}
			callers.Wait()
			if err := client.Emit("after"); err != nil {
				t.Error(err)
			}
		}); err != nil {
			t.Error(err)
		}
	})
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`420["request"]`)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`430["ok"]`, `42["after"]`} {
		_, wire, err := conn.ReadMessage()
		if err != nil || string(wire) != want {
			t.Fatalf("packet = %q, %v; want %q", wire, err, want)
		}
	}
}

func connectAckTestSocket(t *testing.T, listen func(*Socket)) *websocket.Conn {
	t.Helper()
	server := NewServer(nil, nil)
	t.Cleanup(func() { server.Close(nil) })
	ready := make(chan struct{})
	if err := server.On("connection", func(args ...any) {
		listen(args[0].(*Socket))
		close(ready)
	}); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.ServeHandler(nil))
	t.Cleanup(httpServer.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/socket.io/?EIO=4&transport=websocket", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, wire, err := conn.ReadMessage(); err != nil || !strings.HasPrefix(string(wire), "0{") {
		t.Fatalf("Engine.IO handshake = %q, %v", wire, err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("40")); err != nil {
		t.Fatal(err)
	}
	if _, wire, err := conn.ReadMessage(); err != nil || !strings.HasPrefix(string(wire), "40{") {
		t.Fatalf("Socket.IO handshake = %q, %v", wire, err)
	}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("connection listener did not finish")
	}
	return conn
}
