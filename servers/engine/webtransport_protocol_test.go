package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	wt "github.com/quic-go/webtransport-go"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestWebTransportUpgradePreservesProtocolAndHeartbeat(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetTransports(types.NewSet[transports.TransportCtor](Polling, WebTransport))
	opts.SetPingInterval(250 * time.Millisecond)
	opts.SetPingTimeout(time.Second)
	s := NewServer(opts)
	t.Cleanup(func() { s.Close() })
	connected := make(chan Socket, 1)
	upgraded := make(chan struct{})
	heartbeat := make(chan struct{}, 1)
	closed := make(chan string, 1)
	_ = s.On("connection", func(args ...any) {
		socket := args[0].(Socket)
		_ = socket.Once("upgrade", func(...any) { close(upgraded) })
		_ = socket.Once("heartbeat", func(...any) { heartbeat <- struct{}{} })
		_ = socket.Once("close", func(args ...any) { closed <- args[0].(string) })
		connected <- socket
	})
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)
	client := httpServer.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Get(httpServer.URL + "/engine.io/?EIO=4&transport=polling")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || len(data) < 2 || data[0] != '0' {
		t.Fatalf("EIO4 polling OPEN: status=%d data=%q error=%v", response.StatusCode, data, err)
	}
	var handshake struct {
		Sid      string   `json:"sid"`
		Upgrades []string `json:"upgrades"`
	}
	if decodeErr := json.Unmarshal(data[1:], &handshake); decodeErr != nil || handshake.Sid == "" {
		t.Fatalf("invalid EIO4 polling OPEN: %q, error=%v", data, decodeErr)
	}
	if !slices.Equal(handshake.Upgrades, []string{transports.WEBTRANSPORT}) {
		t.Fatalf("EIO4 upgrades = %v, want [webtransport]", handshake.Upgrades)
	}
	var socket Socket
	select {
	case socket = <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("polling connection was not published")
	}

	// WebTransport carries the SID in OPEN, without an EIO query parameter.
	peer := readinessWebTransport(t, s)
	readinessWrite(t, peer, `0{"sid":"`+handshake.Sid+`"}`)
	readinessWrite(t, peer, "2probe")
	readinessRead(t, peer, "3probe")
	readinessWrite(t, peer, "5")
	readinessWait(t, upgraded, "WebTransport upgrade")
	if protocol := socket.Transport().Protocol(); protocol != socket.Protocol() {
		t.Errorf("upgraded transport protocol = %d, want socket protocol %d", protocol, socket.Protocol())
	}

	// An EIO4 PONG must remain valid after replacing the polling transport.
	readinessRead(t, peer, "2")
	readinessWrite(t, peer, "3")
	select {
	case <-heartbeat:
	case reason := <-closed:
		t.Fatalf("responsive peer closed after WebTransport upgrade: %s", reason)
	case <-time.After(5 * time.Second):
		t.Fatal("WebTransport PONG did not complete the heartbeat")
	}
}

func TestWebTransportRejectsEIO3UpgradePreservingPolling(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetAllowEIO3(true)
	opts.SetTransports(types.NewSet[transports.TransportCtor](Polling, WebSocket, WebTransport))
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
	client := httpServer.Client()
	client.Timeout = 5 * time.Second
	url := httpServer.URL + "/engine.io/?EIO=3&transport=polling"
	response, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("EIO3 polling OPEN: status=%d data=%q error=%v", response.StatusCode, data, err)
	}
	packets, err := parser.Parserv3().DecodePayload(types.NewStringBuffer(data))
	if err != nil || len(packets) != 1 || packets[0].Type != packet.OPEN {
		t.Fatalf("invalid EIO3 polling OPEN: %q, error=%v", data, err)
	}
	var handshake struct {
		Upgrades []string `json:"upgrades"`
	}
	if decodeErr := json.NewDecoder(packets[0].Data).Decode(&handshake); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if !slices.Equal(handshake.Upgrades, []string{transports.WEBSOCKET}) {
		t.Fatalf("EIO3 upgrades = %v, want [websocket]", handshake.Upgrades)
	}
	var socket Socket
	select {
	case socket = <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("EIO3 polling connection was not published")
	}

	peer := readinessWebTransport(t, s)
	readinessWrite(t, peer, `0{"sid":"`+socket.Id()+`"}`)
	readinessWait(t, peer.session.Context().Done(), "rejection of EIO3 WebTransport upgrade")
	readinessWait(t, peer.handlerDone, "rejected WebTransport handler return")
	if socket.ReadyState() != "open" || socket.Transport().Name() != transports.POLLING || socket.Upgrading() || socket.Upgraded() {
		t.Fatal("rejected WebTransport upgrade changed the EIO3 polling session")
	}

	url += "&sid=" + socket.Id()
	response, err = client.Post(url, "text/plain", strings.NewReader("11:4still-open"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("EIO3 polling POST status = %d", response.StatusCode)
	}
	response, err = client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err = io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || err != nil || string(data) != "11:4still-open" {
		t.Fatalf("EIO3 polling echo after rejected upgrade: status=%d data=%q error=%v", response.StatusCode, data, err)
	}
}

func TestWebTransportUpgradeRequiresCompleteJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		suffix string
		valid  bool
	}{
		{name: "trailing garbage", suffix: "garbage"},
		{name: "second JSON value", suffix: " {}"},
		{name: "trailing whitespace", suffix: " \t\r\n", valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := config.DefaultServerOptions()
			opts.SetTransports(types.NewSet[transports.TransportCtor](Polling, WebTransport))
			s := NewServer(opts)
			t.Cleanup(func() { s.Close() })
			connected := make(chan Socket, 1)
			upgraded := make(chan struct{})
			_ = s.On("connection", func(args ...any) {
				socket := args[0].(Socket)
				readinessEcho(t, socket)
				_ = socket.Once("upgrade", func(...any) { close(upgraded) })
				connected <- socket
			})
			httpServer := httptest.NewServer(s)
			t.Cleanup(httpServer.Close)
			sid := readinessPollingHandshake(t, httpServer)
			var socket Socket
			select {
			case socket = <-connected:
			case <-time.After(5 * time.Second):
				t.Fatal("polling connection was not published")
			}
			peer := readinessWebTransport(t, s)
			readinessWrite(t, peer, `0{"sid":"`+sid+`"}`+tc.suffix)
			if tc.valid {
				readinessWrite(t, peer, "2probe")
				readinessRead(t, peer, "3probe")
				readinessWrite(t, peer, "5")
				readinessWait(t, upgraded, "WebTransport upgrade with trailing whitespace")
				readinessWrite(t, peer, "4still-open")
				readinessRead(t, peer, "4still-open")
				return
			}
			readinessWait(t, peer.session.Context().Done(), "rejection of incomplete JSON")
			readinessWait(t, peer.handlerDone, "rejected WebTransport handler return")
			if socket.ReadyState() != "open" || socket.Transport().Name() != transports.POLLING || socket.Upgrading() || socket.Upgraded() {
				t.Fatal("rejected WebTransport OPEN changed the polling session")
			}
			readinessPollingEcho(t, httpServer, sid)
		})
	}
}

type webTransportPanicObserver struct {
	Server
	returned chan any
}

func (s *webTransportPanicObserver) OnWebTransportSession(ctx *types.HttpContext, server *wt.Server) {
	defer func() { s.returned <- recover() }()
	s.Server.OnWebTransportSession(ctx, server)
}

func TestWebTransportRejectsNullOpen(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetTransports(types.NewSet[transports.TransportCtor](WebTransport))
	s := &webTransportPanicObserver{Server: NewServer(opts), returned: make(chan any, 1)}
	t.Cleanup(func() { s.Close() })
	peer := readinessWebTransport(t, s)
	readinessWrite(t, peer, "0null")
	select {
	case recovered := <-s.returned:
		if recovered != nil {
			t.Fatalf("null WebTransport OPEN caused a panic: %v", recovered)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("null WebTransport OPEN did not finish")
	}
	readinessWait(t, peer.session.Context().Done(), "rejection of null WebTransport OPEN")
	if count := s.ClientsCount(); count != 0 {
		t.Fatalf("null WebTransport OPEN registered %d clients", count)
	}
}

type webTransportCanceledRequest struct{ Server }

func (s *webTransportCanceledRequest) OnWebTransportSession(ctx *types.HttpContext, server *wt.Server) {
	requestContext, cancel := context.WithCancel(ctx.Context())
	cancel()
	canceled := types.NewHttpContext(ctx.Response(), ctx.Request().WithContext(requestContext))
	<-canceled.Done()
	s.Server.OnWebTransportSession(canceled, server)
}

func TestWebTransportCanceledStreamAcceptClosesSession(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetTransports(types.NewSet[transports.TransportCtor](WebTransport))
	opts.SetUpgradeTimeout(time.Minute)
	s := &webTransportCanceledRequest{Server: NewServer(opts)}
	t.Cleanup(func() { s.Close() })
	session, handlerDone := readinessWebTransportSession(t, s)
	readinessWait(t, handlerDone, "canceled WebTransport stream accept")
	readinessWait(t, session.Context().Done(), "WebTransport session cleanup after stream accept cancellation")
	if count := s.ClientsCount(); count != 0 {
		t.Fatalf("canceled WebTransport stream accept registered %d clients", count)
	}
}
