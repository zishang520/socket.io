package engine

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
	webtrans "github.com/zishang520/socket.io/v3/pkg/webtransport"
)

// Hold the real transport constructor before MaybeUpgrade can install its
// listeners. The observer records premature delivery without consuming packets.
type upgradeReadinessBuilder struct {
	transports.TransportCtor
	constructed chan struct{}
	resume      chan struct{}
	earlyPacket chan struct{}
	released    atomic.Bool
}

func (b *upgradeReadinessBuilder) New(ctx *types.HttpContext) transports.Transport {
	transport := b.TransportCtor.New(ctx)
	_ = transport.On("packet", func(...any) {
		if !b.released.Load() {
			select {
			case b.earlyPacket <- struct{}{}:
			default:
			}
		}
	})
	close(b.constructed)
	<-b.resume
	return transport
}

type readinessConnection interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

func TestUpgradeWaitsForProbeListeners(t *testing.T) {
	for _, ctor := range []transports.TransportCtor{WebSocket, WebTransport} {
		t.Run(ctor.Name(), func(t *testing.T) {
			builder := &upgradeReadinessBuilder{
				TransportCtor: ctor,
				constructed:   make(chan struct{}),
				resume:        make(chan struct{}),
				earlyPacket:   make(chan struct{}, 1),
			}
			release := sync.OnceFunc(func() {
				builder.released.Store(true)
				close(builder.resume)
			})
			defer release()

			opts := config.DefaultServerOptions()
			opts.SetTransports(types.NewSet[transports.TransportCtor](Polling, builder))
			s := NewServer(opts)
			t.Cleanup(func() { s.Close() })
			connected := make(chan struct{})
			upgraded := make(chan struct{})
			_ = s.On("connection", func(args ...any) {
				socket := args[0].(Socket)
				readinessEcho(t, socket)
				_ = socket.Once("upgrade", func(...any) { close(upgraded) })
				close(connected)
			})
			httpServer := httptest.NewServer(s)
			t.Cleanup(httpServer.Close)
			sid := readinessPollingHandshake(t, httpServer)
			readinessWait(t, connected, "polling connection initialization")

			var conn readinessConnection
			if ctor.Name() == transports.WEBSOCKET {
				url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/engine.io/?EIO=4&transport=websocket&sid=" + sid
				ws, _, err := websocket.DefaultDialer.Dial(url, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = ws.Close() })
				conn = ws
			} else {
				conn = readinessWebTransport(t, s)
				readinessWrite(t, conn, `0{"sid":"`+sid+`"}`)
			}
			readinessWait(t, builder.constructed, "candidate transport construction")
			readinessWrite(t, conn, "2probe")

			// Without the readiness boundary this is a positive observation of
			// the probe being lost before MaybeUpgrade has registered a listener.
			select {
			case <-builder.earlyPacket:
				t.Fatal("candidate transport dispatched the probe before upgrade listeners were installed")
			case <-time.After(50 * time.Millisecond):
			}
			release()
			readinessRead(t, conn, "3probe")
			readinessWrite(t, conn, "5")
			readinessWrite(t, conn, "4first")
			readinessWrite(t, conn, "4second")
			readinessRead(t, conn, "4first")
			readinessRead(t, conn, "4second")
			readinessWait(t, upgraded, "transport upgrade")
		})
	}
}

func TestWebTransportHandshakeWaitsForConnectionListeners(t *testing.T) {
	for _, mode := range []string{"after-open", "pipelined"} {
		t.Run(mode, func(t *testing.T) {
			opts := config.DefaultServerOptions()
			opts.SetTransports(types.NewSet[transports.TransportCtor](WebTransport))
			s := NewServer(opts)
			t.Cleanup(func() { s.Close() })
			initializing := make(chan struct{})
			resume := make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			defer release()
			earlyPacket := make(chan struct{}, 1)
			var initialized atomic.Bool
			_ = s.On("connection", func(args ...any) {
				socket := args[0].(Socket)
				_ = socket.On("packet", func(...any) {
					if !initialized.Load() {
						select {
						case earlyPacket <- struct{}{}:
						default:
						}
					}
				})
				close(initializing)
				<-resume
				readinessEcho(t, socket)
				initialized.Store(true)
			})
			conn := readinessWebTransport(t, s)
			if mode == "pipelined" {
				// Two complete text frames in one stream write allow the handshake
				// reader to buffer MESSAGE while reading OPEN. Readiness must guard
				// NextReader even when no further network read is needed.
				_ = conn.stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if _, err := conn.stream.Write([]byte("\x010\x064first")); err != nil {
					t.Fatal(err)
				}
			} else {
				readinessWrite(t, conn, "0")
			}
			readinessWait(t, initializing, "WebTransport connection initialization")
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, data, err := conn.ReadMessage()
			if err != nil || !strings.HasPrefix(string(data), `0{"`) {
				t.Fatalf("Engine.IO OPEN = %q, error = %v", data, err)
			}
			if mode == "after-open" {
				readinessWrite(t, conn, "4first")
			}
			select {
			case <-earlyPacket:
				t.Fatal("WebTransport dispatched a message before connection listeners were installed")
			case <-time.After(50 * time.Millisecond):
			}
			release()
			readinessRead(t, conn, "4first")
			readinessWrite(t, conn, "4second")
			readinessRead(t, conn, "4second")
		})
	}
}

// The two factory positions cover cancellation before a transport exists and
// cancellation while an already-constructed candidate awaits probe listeners.
type timeoutReadinessBuilder struct {
	transports.WebTransportBuilder
	beforeConstruct bool
	entered         chan struct{}
	resume          chan struct{}
	packet          chan struct{}
	closed          chan struct{}
}

func (b *timeoutReadinessBuilder) New(ctx *types.HttpContext) transports.Transport {
	if b.beforeConstruct {
		close(b.entered)
		<-b.resume
	}
	transport := transports.MakeWebTransport()
	_ = transport.On("packet", func(...any) {
		select {
		case b.packet <- struct{}{}:
		default:
		}
	})
	_ = transport.Once("close", func(...any) { close(b.closed) })
	transport.Construct(ctx)
	if !b.beforeConstruct {
		close(b.entered)
		<-b.resume
	}
	return transport
}

func TestWebTransportConnectionInitializationTimeout(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetUpgradeTimeout(50 * time.Millisecond)
	opts.SetTransports(types.NewSet[transports.TransportCtor](WebTransport))
	s := NewServer(opts)
	t.Cleanup(func() { s.Close() })
	initializing := make(chan struct{})
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	closed := make(chan struct{})
	_ = s.On("connection", func(args ...any) {
		socket := args[0].(Socket)
		_ = socket.Once("close", func(...any) { close(closed) })
		close(initializing)
		<-resume
	})
	conn := readinessWebTransport(t, s)
	readinessWrite(t, conn, "0")
	readinessWait(t, initializing, "WebTransport connection initialization")
	readinessWait(t, conn.session.Context().Done(), "initialization timeout closing the session")
	readinessWait(t, closed, "initialization timeout closing the Engine.IO socket")
	if count := s.ClientsCount(); count != 0 {
		t.Fatalf("timed-out connection remains registered: clients=%d", count)
	}
	release()
	readinessWait(t, conn.handlerDone, "timed-out handshake return")
}

func TestWebTransportUpgradeInitializationTimeoutPreservesPolling(t *testing.T) {
	builder := &timeoutReadinessBuilder{
		entered: make(chan struct{}),
		resume:  make(chan struct{}),
		packet:  make(chan struct{}, 1),
		closed:  make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(builder.resume) })
	defer release()
	opts := config.DefaultServerOptions()
	opts.SetUpgradeTimeout(50 * time.Millisecond)
	opts.SetTransports(types.NewSet[transports.TransportCtor](Polling, builder))
	s := NewServer(opts)
	t.Cleanup(func() { s.Close() })
	connected := make(chan Socket, 1)
	_ = s.On("connection", func(args ...any) {
		socket := args[0].(Socket)
		readinessEcho(t, socket)
		connected <- socket
	})
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)
	sid := readinessPollingHandshake(t, httpServer)
	var socket Socket
	select {
	case socket = <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for polling connection initialization")
	}
	conn := readinessWebTransport(t, s)
	readinessWrite(t, conn, `0{"sid":"`+sid+`"}`)
	readinessWait(t, builder.entered, "upgrade transport construction")
	readinessWrite(t, conn, "2probe")
	readinessWait(t, conn.session.Context().Done(), "initialization timeout closing the candidate session")
	release()
	readinessWait(t, conn.handlerDone, "timed-out upgrade return")
	readinessWait(t, builder.closed, "timed-out candidate transport cleanup")
	if socket.Upgrading() || socket.Upgraded() || socket.ReadyState() != "open" || socket.Transport().Name() != transports.POLLING {
		t.Fatalf("failed candidate changed the polling socket: state=%s transport=%s upgrading=%t upgraded=%t",
			socket.ReadyState(), socket.Transport().Name(), socket.Upgrading(), socket.Upgraded())
	}
	select {
	case <-builder.packet:
		t.Fatal("timed-out candidate dispatched its queued probe")
	default:
	}
	readinessPollingEcho(t, httpServer, sid)
}

func TestWebTransportLateFactoryDoesNotReadBufferedPacket(t *testing.T) {
	builder := &timeoutReadinessBuilder{
		beforeConstruct: true,
		entered:         make(chan struct{}),
		resume:          make(chan struct{}),
		packet:          make(chan struct{}, 1),
		closed:          make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(builder.resume) })
	defer release()
	opts := config.DefaultServerOptions()
	opts.SetUpgradeTimeout(50 * time.Millisecond)
	opts.SetTransports(types.NewSet[transports.TransportCtor](builder))
	s := NewServer(opts)
	t.Cleanup(func() { s.Close() })
	var connections atomic.Int32
	_ = s.On("connection", func(...any) { connections.Add(1) })
	conn := readinessWebTransport(t, s)
	// The first read can buffer MESSAGE along with OPEN before the factory
	// pauses. Closing the underlying stream alone must not release that packet.
	_ = conn.stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.stream.Write([]byte("\x010\x064first")); err != nil {
		t.Fatal(err)
	}
	readinessWait(t, builder.entered, "WebTransport factory entry")
	readinessWait(t, conn.session.Context().Done(), "factory timeout closing the session")
	release()
	readinessWait(t, conn.handlerDone, "late factory handshake return")
	readinessWait(t, builder.closed, "late transport cleanup")
	if count := connections.Load(); count != 0 {
		t.Fatalf("late factory published %d connections after timeout", count)
	}
	if count := s.ClientsCount(); count != 0 {
		t.Fatalf("late factory left %d registered clients", count)
	}
	select {
	case <-builder.packet:
		t.Fatal("late transport reader consumed the buffered message after timeout")
	case <-time.After(50 * time.Millisecond):
	}
}

func readinessEcho(t *testing.T, socket Socket) {
	t.Helper()
	_ = socket.On("message", func(args ...any) {
		data, err := io.ReadAll(args[0].(io.Reader))
		if err != nil {
			t.Error(err)
			return
		}
		socket.Send(types.NewStringBuffer(data), nil, nil)
	})
}

func readinessPollingHandshake(t *testing.T, s *httptest.Server) string {
	t.Helper()
	client := s.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Get(s.URL + "/engine.io/?EIO=4&transport=polling")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || len(data) < 2 || data[0] != '0' {
		t.Fatalf("polling OPEN: status=%d data=%q error=%v", response.StatusCode, data, err)
	}
	var handshake struct {
		Sid string `json:"sid"`
	}
	if err := json.Unmarshal(data[1:], &handshake); err != nil || handshake.Sid == "" {
		t.Fatalf("invalid polling OPEN: %q, error=%v", data, err)
	}
	return handshake.Sid
}

func readinessPollingEcho(t *testing.T, s *httptest.Server, sid string) {
	t.Helper()
	url := s.URL + "/engine.io/?EIO=4&transport=polling&sid=" + sid
	response, err := s.Client().Post(url, "text/plain", strings.NewReader("4still-open"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("polling POST after failed upgrade: status=%d", response.StatusCode)
	}
	response, err = s.Client().Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || err != nil || string(data) != "4still-open" {
		t.Fatalf("polling GET after failed upgrade: status=%d data=%q error=%v", response.StatusCode, data, err)
	}
}

// Exercise the real HTTP/3 server and QUIC stream with a loopback UDP listener
// and httptest's local TLS certificate. No browser or external service is used.
func readinessWebTransport(t *testing.T, s Server) *readinessWebTransportConnection {
	t.Helper()
	session, handlerDone := readinessWebTransportSession(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return &readinessWebTransportConnection{
		Conn:        webtrans.NewConn(session, stream, false, 0, 0, nil, nil, nil),
		stream:      stream,
		session:     session,
		handlerDone: handlerDone,
	}
}

func readinessWebTransportSession(t *testing.T, s Server) (*wt.Session, <-chan struct{}) {
	t.Helper()
	webServer := &wt.Server{H3: &http3.Server{}}
	handlerDone := make(chan struct{})
	webServer.H3.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		s.OnWebTransportSession(types.NewHttpContext(w, r), webServer)
	})
	baseURL, dialer := readinessWebTransportServer(t, webServer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	_, session, err := dialer.Dial(ctx, baseURL+"/engine.io/", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.CloseWithError(0, "") })
	return session, handlerDone
}

func readinessWebTransportServer(t *testing.T, webServer *wt.Server) (string, *wt.Transport) {
	t.Helper()
	tlsServer := httptest.NewTLSServer(nil)
	certificate := tlsServer.TLS.Certificates[0]
	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(tlsServer.Certificate())
	tlsServer.Close()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	webServer.H3.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		NextProtos:   []string{http3.NextProtoH3},
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- webServer.Serve(udp) }()
	t.Cleanup(func() {
		_ = webServer.Close()
		_ = udp.Close()
		select {
		case <-serverDone:
		case <-time.After(5 * time.Second):
			t.Error("WebTransport server did not stop")
		}
	})
	// The default QUIC dialer binds a wildcard UDP socket even for loopback peers.
	clientUDP, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientTransport := &quic.Transport{Conn: clientUDP}
	dialer := &wt.Transport{
		TLSClientConfig: &tls.Config{RootCAs: rootCAs},
		DialAddr: func(ctx context.Context, address string, tlsConfig *tls.Config, config *quic.Config) (*quic.Conn, error) {
			peer, err := net.ResolveUDPAddr("udp", address)
			if err != nil {
				return nil, err
			}
			return clientTransport.DialEarly(ctx, peer, tlsConfig, config)
		},
	}
	t.Cleanup(func() {
		_ = dialer.Close()
		_ = clientTransport.Close()
		_ = clientUDP.Close()
	})
	return "https://" + udp.LocalAddr().String(), dialer
}

type readinessWebTransportConnection struct {
	*webtrans.Conn
	stream      *wt.Stream
	session     *wt.Session
	handlerDone <-chan struct{}
}

func readinessWait(t *testing.T, done <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for " + description)
	}
}

func readinessWrite(t *testing.T, conn readinessConnection, message string) {
	t.Helper()
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		t.Fatalf("write %q: %v", message, err)
	}
}

func readinessRead(t *testing.T, conn readinessConnection, want string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil || string(data) != want {
		t.Fatalf("received %q, error=%v; want %q", data, err, want)
	}
}
