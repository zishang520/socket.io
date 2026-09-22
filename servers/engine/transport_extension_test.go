package engine_test

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Model an external transport waiting for its backend after New returns.
// It uses only exported APIs, including the shared stream-reader boundary.
type deferredTransport struct {
	transports.Transport
	ctx         *types.HttpContext
	name        string
	initialized chan struct{}
	readerDone  chan struct{}
	backend     chan struct{}
	closed      chan struct{}
	closing     atomic.Bool
}

func (t *deferredTransport) Name() string { return t.name }

func (t *deferredTransport) read() {
	defer close(t.readerDone)
	close(t.initialized)
	select {
	case <-t.backend:
	case <-t.closed:
		return
	}
	t.SetReadyState("open")
	t.SetWritable(true)
	t.Emit("ready")
	if t.ctx.Websocket == nil {
		return
	}
	defer t.Close()
	for {
		_, data, err := t.ctx.Websocket.ReadMessage()
		if err != nil {
			return
		}
		t.OnData(types.NewStringBuffer(data))
	}
}

func (t *deferredTransport) Send(packets []*packet.Packet) {
	if t.ctx.Websocket != nil {
		for _, value := range packets {
			data, err := t.Parser().EncodePacket(value, true)
			if err == nil {
				err = t.ctx.Websocket.WriteMessage(websocket.TextMessage, data.Bytes())
			}
			if err != nil {
				t.OnError("external transport write", err)
				return
			}
		}
	} else {
		t.SetWritable(false)
		data, err := t.Parser().EncodePayload(packets)
		if err == nil {
			_, err = io.Copy(t.ctx, data)
		}
		if err != nil {
			t.OnError("external transport write", err)
			return
		}
	}
	t.Emit("drain")
}

func (t *deferredTransport) DoClose(done types.Callable) {
	if t.closing.CompareAndSwap(false, true) {
		close(t.closed)
		if t.ctx.Websocket != nil {
			_ = t.ctx.Websocket.Conn.Close()
		}
		t.OnClose()
	}
	if done != nil {
		done()
	}
}

type deferredTransportBuilder struct {
	name    string
	created chan *deferredTransport
}

func (b *deferredTransportBuilder) Name() string        { return b.name }
func (*deferredTransportBuilder) HandlesUpgrades() bool { return true }
func (*deferredTransportBuilder) UpgradesTo() []string  { return nil }
func (b *deferredTransportBuilder) New(ctx *types.HttpContext) transports.Transport {
	transport := &deferredTransport{
		Transport:   transports.MakeTransport(),
		ctx:         ctx,
		name:        b.name,
		initialized: make(chan struct{}),
		readerDone:  make(chan struct{}),
		backend:     make(chan struct{}),
		closed:      make(chan struct{}),
	}
	transport.Prototype(transport)
	transport.Construct(ctx)
	transport.SetReadyState("opening")
	transports.StartReader(transport, ctx.TransportReadPermission(), transport.read)
	b.created <- transport
	return transport
}

func awaitExtension[T any](t *testing.T, values <-chan T, name string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for " + name)
	}
	var zero T
	return zero
}

func TestExternalOpeningTransportHandshake(t *testing.T) {
	builder := &deferredTransportBuilder{name: "external", created: make(chan *deferredTransport, 1)}
	opts := config.DefaultServerOptions()
	opts.SetTransports(types.NewSet[transports.TransportCtor](builder))
	opts.SetPingInterval(time.Hour)
	server := engine.NewServer(opts)
	connected := make(chan struct{})
	_ = server.Once("connection", func(...any) { close(connected) })
	network := httptest.NewServer(server)
	var transport *deferredTransport
	t.Cleanup(func() {
		server.Close()
		if transport != nil {
			transport.Close()
			transport.ctx.Flush()
		}
		network.Close()
	})
	client := network.Client()
	client.Timeout = 2 * time.Second
	result := make(chan error, 1)
	go func() {
		response, err := client.Get(network.URL + "/engine.io/?EIO=4&transport=external")
		if err == nil {
			defer func() { _ = response.Body.Close() }()
			body, readErr := io.ReadAll(response.Body)
			err = readErr
			if err == nil && (response.StatusCode != http.StatusOK || !strings.HasPrefix(string(body), "0{")) {
				t.Errorf("deferred OPEN response = (%d, %q)", response.StatusCode, body)
			}
		}
		result <- err
	}()
	transport = awaitExtension(t, builder.created, "external transport construction")
	awaitExtension(t, connected, "external connection publication")
	if transport.ReadyState() != "opening" || transport.Writable() {
		t.Fatal("transport did not retain its asynchronous opening state")
	}
	close(transport.backend)
	if err := awaitExtension(t, result, "OPEN after backend readiness"); err != nil {
		t.Fatal(err)
	}
}

func TestExternalOpeningTransportUpgrade(t *testing.T) {
	for _, expires := range []bool{false, true} {
		name := "backend_ready"
		if expires {
			name = "backend_timeout"
		}
		t.Run(name, func(t *testing.T) {
			builder := &deferredTransportBuilder{name: transports.WEBSOCKET, created: make(chan *deferredTransport, 1)}
			opts := config.DefaultServerOptions()
			opts.SetTransports(types.NewSet[transports.TransportCtor](engine.Polling, builder))
			opts.SetPingInterval(time.Hour)
			if expires {
				opts.SetUpgradeTimeout(100 * time.Millisecond)
			}
			server := engine.NewServer(opts)
			connected := make(chan engine.Socket, 1)
			upgraded := make(chan struct{})
			_ = server.Once("connection", func(args ...any) {
				socket := args[0].(engine.Socket)
				_ = socket.Once("upgrade", func(...any) { close(upgraded) })
				_ = socket.On("message", func(args ...any) {
					socket.Send(args[0].(io.Reader), nil, nil)
				})
				connected <- socket
			})
			network := httptest.NewServer(server)
			t.Cleanup(func() { server.Close(); network.Close() })
			client := network.Client()
			client.Timeout = 2 * time.Second
			response, err := client.Get(network.URL + "/engine.io/?EIO=4&transport=polling")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			socket := awaitExtension(t, connected, "polling connection publication")
			peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(network.URL, "http")+"/engine.io/?EIO=4&transport=websocket&sid="+socket.Id(), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = peer.Close() })
			transport := awaitExtension(t, builder.created, "upgrade transport construction")
			t.Cleanup(func() { transport.Close() })
			awaitExtension(t, transport.initialized, "upgrade listener initialization")
			if transport.ReadyState() != "opening" || !socket.Upgrading() {
				t.Fatal("upgrade candidate was rejected before backend readiness")
			}
			if expires {
				awaitExtension(t, transport.closed, "opening candidate timeout")
				awaitExtension(t, transport.readerDone, "backend waiter cancellation")
				if socket.Upgrading() || socket.ReadyState() != "open" || socket.Transport().Name() != transports.POLLING {
					t.Fatal("expired candidate changed the original polling session")
				}
				_ = peer.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _, readErr := peer.ReadMessage()
				var timeout net.Error
				if readErr == nil || (errors.As(readErr, &timeout) && timeout.Timeout()) {
					t.Fatalf("candidate network connection was not closed: %v", readErr)
				}
				return
			}
			close(transport.backend)
			_ = peer.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err := peer.WriteMessage(websocket.TextMessage, []byte("2probe")); err != nil {
				t.Fatal(err)
			}
			_ = peer.SetReadDeadline(time.Now().Add(2 * time.Second))
			if _, data, err := peer.ReadMessage(); err != nil || string(data) != "3probe" {
				t.Fatalf("upgrade probe = %q, error = %v", data, err)
			}
			for _, message := range []string{"5", "4after-upgrade"} {
				if err := peer.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
					t.Fatal(err)
				}
			}
			if _, data, err := peer.ReadMessage(); err != nil || string(data) != "4after-upgrade" {
				t.Fatalf("upgraded message = %q, error = %v", data, err)
			}
			awaitExtension(t, upgraded, "external transport upgrade")
		})
	}
}
