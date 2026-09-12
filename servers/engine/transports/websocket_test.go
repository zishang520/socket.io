package transports

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// recordingConn records Close calls on the underlying connection so a test can
// assert that the transport actually closed the hijacked HTTP connection.
type recordingConn struct {
	net.Conn
	closed atomic.Bool
}

func (c *recordingConn) Close() error {
	c.closed.Store(true)

	return c.Conn.Close()
}

// hijackResponseWriter is an http.ResponseWriter that hands the underlying
// connection to gorilla's Upgrader via Hijack, mirroring what net/http does for
// a real websocket upgrade.
type hijackResponseWriter struct {
	conn net.Conn
}

func (w *hijackResponseWriter) Header() http.Header         { return make(http.Header) }
func (w *hijackResponseWriter) WriteHeader(int)             {}
func (w *hijackResponseWriter) Write(b []byte) (int, error) { return len(b), nil }

func (w *hijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

// TestWebsocketTransportClosesConnectionOnPeerClose reproduces the
// connection-cap slot leak (DELIVERY-725): on a client-initiated websocket
// close the transport marks itself "closed" without closing the hijacked HTTP
// connection. The hijacked connection must be closed once the transport
// reaches the closed state, regardless of the close path.
func TestWebsocketTransportClosesConnectionOnPeerClose(t *testing.T) {
	clientRaw, serverRaw := net.Pipe()
	server := &recordingConn{Conn: serverRaw}
	defer clientRaw.Close()
	defer server.Close()

	// Client handshake runs over the pipe: gorilla's Dialer writes the upgrade
	// request to clientRaw; the server side reads it and upgrades. A custom
	// dialer returns the pipe end so no real TCP connection is opened.
	type clientResult struct {
		conn *ws.Conn
		err  error
	}
	resultCh := make(chan clientResult, 1)
	go func() {
		dialer := &ws.Dialer{
			NetDial: func(string, string) (net.Conn, error) {
				return clientRaw, nil
			},
		}
		conn, resp, err := dialer.Dial("ws://localhost/socket.io/?EIO=4&transport=websocket", nil)
		if resp != nil {
			resp.Body.Close()
		}
		resultCh <- clientResult{conn: conn, err: err}
	}()

	// Server side: read the client's upgrade request from the pipe and upgrade.
	// The client sends nothing beyond the request until the handshake completes,
	// so no bytes are stranded in the ReadRequest buffer when gorilla creates
	// its own reader after the upgrade.
	reqCtx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()
	req, err := http.ReadRequest(bufio.NewReader(server))
	if err != nil {
		t.Fatalf("read upgrade request: %v", err)
	}
	req = req.WithContext(reqCtx)
	upgrader := ws.Upgrader{}
	serverWS, err := upgrader.Upgrade(&hijackResponseWriter{conn: server}, req, nil)
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	res := <-resultCh
	if res.err != nil {
		t.Fatalf("client dial failed: %v", res.err)
	}
	clientWS := res.conn
	defer clientWS.Close()

	// Build the transport around the server-side websocket, as the engine
	// server does after a successful upgrade.
	wsc := &types.WebSocketConn{EventEmitter: types.NewEventEmitter(), Conn: serverWS}
	ctx := types.NewHttpContext(&hijackResponseWriter{conn: server}, req)
	ctx.Websocket = wsc
	NewWebSocket(ctx)

	// The peer closes the websocket connection.
	if err := clientWS.WriteMessage(ws.CloseMessage, ws.FormatCloseMessage(ws.CloseNormalClosure, "")); err != nil {
		t.Fatalf("write close frame: %v", err)
	}

	// The hijacked connection must be closed; otherwise the slot it holds
	// leaks (a poll is used so a missing close fails the test cleanly).
	deadline := time.Now().Add(2 * time.Second)
	for !server.closed.Load() {
		if time.Now().After(deadline) {
			t.Fatal("websocket transport did not close the hijacked connection on peer close")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
