package socket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/version"
)

type attachReadinessEvents struct {
	types.EventEmitter
	beforePublish func()
}

func (e *attachReadinessEvents) Once(name types.EventName, listeners ...types.EventListener) error {
	if name == "listening" {
		// Engine Attach subscribes to listening immediately before it publishes
		// its HTTP handler. Observe that boundary through the existing interface.
		e.beforePublish()
	}
	return e.EventEmitter.Once(name, listeners...)
}

func TestAttachPreparesSocketIOBeforePublishingHandler(t *testing.T) {
	server := MakeServer()
	httpServer := types.NewWebServer(http.NotFoundHandler())
	observed := false
	httpServer.EventEmitter = &attachReadinessEvents{
		EventEmitter: httpServer.EventEmitter,
		beforePublish: func() {
			observed = true
			if server.Engine() == nil || server.Engine().ListenerCount("connection") == 0 {
				t.Error("HTTP handler is being published before Socket.IO binds to Engine.IO")
			}
			if server.httpServer != httpServer {
				t.Error("HTTP handler is being published before its server is available for cleanup")
			}
			if server._corsMiddleware == nil {
				t.Error("HTTP handler is being published before client-file CORS is initialized")
			}
		},
	}
	opts := DefaultServerOptions()
	opts.SetServeClient(true)
	opts.SetCors(&types.Cors{Origin: "https://client.example"})

	// Attach to an already-running HTTP server, which may dispatch requests as
	// soon as a handler is registered, before Construct returns.
	network := httptest.NewServer(httpServer)
	t.Cleanup(func() {
		server.Close(nil)
		network.Close()
	})
	server.Construct(httpServer, opts)
	if !observed {
		t.Fatal("Engine.IO did not attach to the HTTP server")
	}

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(network.URL, "http")+"/socket.io/?EIO=4&transport=websocket", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	_ = client.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if writeErr := client.WriteMessage(websocket.TextMessage, []byte(`40{"token":"first"}`)); writeErr != nil {
		t.Fatal(writeErr)
	}
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	for _, prefix := range []string{"0{", "40{"} {
		kind, payload, readErr := client.ReadMessage()
		if readErr != nil || kind != websocket.TextMessage || !strings.HasPrefix(string(payload), prefix) {
			t.Fatalf("frame = (%d, %q, %v), want text with prefix %q", kind, payload, readErr, prefix)
		}
	}

	request, err := http.NewRequest(http.MethodGet, network.URL+"/socket.io/socket.io.js", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://client.example")
	request.Header.Set("If-None-Match", `"`+version.VERSION+`"`)
	response, err := network.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotModified {
		t.Fatalf("client-file status = %d, want 304", response.StatusCode)
	}
	if origin := response.Header.Get("Access-Control-Allow-Origin"); origin != "https://client.example" {
		t.Fatalf("client-file CORS origin = %q", origin)
	}
}

func TestAttachAddressStartsListeningAfterInitialization(t *testing.T) {
	for _, address := range []struct {
		name  string
		value any
	}{
		{name: "port", value: 0},
		{name: "address", value: "127.0.0.1:0"},
	} {
		t.Run(address.name, func(t *testing.T) {
			server := NewServer(nil, nil)
			t.Cleanup(func() { server.Close(nil) })
			server.Attach(address.value, nil)
			// Engine Attach installs a one-time listening callback. Starting the
			// HTTP server before attaching leaves it waiting for an event it missed.
			if pending := server.httpServer.ListenerCount("listening"); pending != 0 {
				t.Fatalf("%d listening callbacks missed the server startup", pending)
			}
		})
	}
}

func TestEngineInitializationOptionsAndHandlerReuse(t *testing.T) {
	for _, attach := range []bool{false, true} {
		name := "handler"
		if attach {
			name = "attach"
		}
		t.Run(name, func(t *testing.T) {
			options := DefaultServerOptions()
			options.SetPingInterval(time.Minute)
			server := NewServer(nil, options)
			server.SetPath("/custom")
			t.Cleanup(func() { server.Close(nil) })
			engineOptions := DefaultServerOptions()
			engineOptions.SetPingInterval(time.Second)
			engineOptions.SetMaxHttpBufferSize(4096)
			if attach {
				server.Attach(types.NewWebServer(http.NotFoundHandler()), engineOptions)
			} else {
				server.ServeHandler(engineOptions)
			}
			if engineOptions.Path() != "/custom" || server.Engine().Opts().PingInterval() != time.Minute || server.Engine().Opts().MaxHttpBufferSize() != 4096 {
				t.Fatal("initialization changed path fallback or option precedence")
			}
			if server.Engine().ListenerCount("connection") != 1 {
				t.Fatal("Engine.IO must be bound to Socket.IO exactly once")
			}
			handler := server.ServeHandler(nil)
			replacement := DefaultServerOptions()
			replacement.SetPingInterval(2 * time.Second)
			if server.ServeHandler(replacement) != handler || server.Engine().Opts().PingInterval() != time.Minute {
				t.Fatal("ServeHandler replaced or reconfigured an existing engine")
			}
		})
	}
}
