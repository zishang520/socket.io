package webtransport

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestIsUnexpectedCloseError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		codes    []int
		expected bool
	}{
		{
			name:     "unexpected code",
			err:      &CloseError{Code: CloseAbnormalClosure},
			codes:    []int{CloseNormalClosure, CloseGoingAway},
			expected: true,
		},
		{
			name:     "expected code",
			err:      &CloseError{Code: CloseNormalClosure},
			codes:    []int{CloseNormalClosure, CloseGoingAway},
			expected: false,
		},
		{
			name:     "non-CloseError",
			err:      errors.New("some error"),
			codes:    []int{CloseNormalClosure},
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			codes:    []int{CloseNormalClosure},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUnexpectedCloseError(tt.err, tt.codes...)
			if got != tt.expected {
				t.Errorf("IsUnexpectedCloseError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCloseErrorAllCodes(t *testing.T) {
	// Test all known close codes have proper descriptions
	codesWithDesc := []struct {
		code    int
		contain string
	}{
		{CloseNormalClosure, "(normal)"},
		{CloseGoingAway, "(going away)"},
		{CloseProtocolError, "(protocol error)"},
		{CloseUnsupportedData, "(unsupported data)"},
		{CloseNoStatusReceived, "(no status)"},
		{CloseAbnormalClosure, "(abnormal closure)"},
		{CloseInvalidFramePayloadData, "(invalid payload data)"},
		{ClosePolicyViolation, "(policy violation)"},
		{CloseMessageTooBig, "(message too big)"},
		{CloseMandatoryExtension, "(mandatory extension missing)"},
		{CloseInternalServerErr, "(internal server error)"},
		{CloseTLSHandshake, "(TLS handshake error)"},
	}
	for _, tc := range codesWithDesc {
		e := &CloseError{Code: tc.code}
		msg := e.Error()
		if !bytes.Contains([]byte(msg), []byte(tc.contain)) {
			t.Errorf("CloseError{%d}.Error() = %q, missing %q", tc.code, msg, tc.contain)
		}
	}
}

func TestIsWebTransportUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		proto    string
		headers  http.Header
		expected bool
	}{
		{
			name:   "valid upgrade",
			method: http.MethodConnect,
			proto:  "webtransport",
			headers: http.Header{
				"Sec-Webtransport-Http3-Draft02": {"1"},
			},
			expected: true,
		},
		{
			name:     "wrong method",
			method:   http.MethodGet,
			proto:    "webtransport",
			headers:  http.Header{"Sec-Webtransport-Http3-Draft02": {"1"}},
			expected: false,
		},
		{
			name:     "wrong proto",
			method:   http.MethodConnect,
			proto:    "HTTP/1.1",
			headers:  http.Header{"Sec-Webtransport-Http3-Draft02": {"1"}},
			expected: false,
		},
		{
			name:     "missing header",
			method:   http.MethodConnect,
			proto:    "webtransport",
			headers:  http.Header{},
			expected: false,
		},
		{
			name:     "wrong header value",
			method:   http.MethodConnect,
			proto:    "webtransport",
			headers:  http.Header{"Sec-Webtransport-Http3-Draft02": {"0"}},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Method: tt.method,
				Proto:  tt.proto,
				Header: tt.headers,
			}
			got := IsWebTransportUpgrade(r)
			if got != tt.expected {
				t.Errorf("IsWebTransportUpgrade() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSkipSpace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  hello", "hello"},
		{"\t\thello", "hello"},
		{" \t hello", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		got := skipSpace(tt.input)
		if got != tt.expected {
			t.Errorf("skipSpace(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNextToken(t *testing.T) {
	tests := []struct {
		input     string
		wantToken string
		wantRest  string
	}{
		{"hello world", "hello", " world"},
		{"hello,world", "hello", ",world"},
		{"hello", "hello", ""},
		{"", "", ""},
		{" hello", "", " hello"},
	}
	for _, tt := range tests {
		token, rest := nextToken(tt.input)
		if token != tt.wantToken || rest != tt.wantRest {
			t.Errorf("nextToken(%q) = (%q, %q), want (%q, %q)", tt.input, token, rest, tt.wantToken, tt.wantRest)
		}
	}
}

func TestEqualASCIIFold(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		{"hello", "hello", true},
		{"Hello", "hello", true},
		{"HELLO", "hello", true},
		{"hello", "world", false},
		{"hello", "hell", false},
		{"", "", true},
		{"a", "b", false},
	}
	for _, tt := range tests {
		got := equalASCIIFold(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("equalASCIIFold(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestTokenListContainsValue(t *testing.T) {
	tests := []struct {
		name     string
		header   http.Header
		key      string
		value    string
		expected bool
	}{
		{
			name:     "single value match",
			header:   http.Header{"Upgrade": {"websocket"}},
			key:      "Upgrade",
			value:    "websocket",
			expected: true,
		},
		{
			name:     "case insensitive",
			header:   http.Header{"Upgrade": {"WebSocket"}},
			key:      "Upgrade",
			value:    "websocket",
			expected: true,
		},
		{
			name:     "comma-separated",
			header:   http.Header{"Connection": {"keep-alive, Upgrade"}},
			key:      "Connection",
			value:    "Upgrade",
			expected: true,
		},
		{
			name:     "no match",
			header:   http.Header{"Upgrade": {"http2"}},
			key:      "Upgrade",
			value:    "websocket",
			expected: false,
		},
		{
			name:     "missing header",
			header:   http.Header{},
			key:      "Upgrade",
			value:    "websocket",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenListContainsValue(tt.header, tt.key, tt.value)
			if got != tt.expected {
				t.Errorf("tokenListContainsValue() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHideTempErr(t *testing.T) {
	// Regular error should pass through
	regular := errors.New("regular error")
	if got := hideTempErr(regular); got != regular {
		t.Error("hideTempErr should pass through regular errors")
	}

	// net.Error should have Temporary cleared
	netErr := &netError{msg: "net error", temporary: true, timeout: true}
	got := hideTempErr(netErr)
	if ne, ok := got.(*netError); ok {
		if ne.Temporary() {
			t.Error("hideTempErr should clear Temporary flag")
		}
		if !ne.Timeout() {
			t.Error("hideTempErr should preserve Timeout flag")
		}
	} else {
		t.Error("hideTempErr result should be *netError")
	}
}

func TestIsData(t *testing.T) {
	if !isData(TextMessage) {
		t.Error("isData(TextMessage) should be true")
	}
	if !isData(BinaryMessage) {
		t.Error("isData(BinaryMessage) should be true")
	}
	if isData(0) {
		t.Error("isData(0) should be false")
	}
	if isData(3) {
		t.Error("isData(3) should be false")
	}
}

func TestNewPreparedMessage(t *testing.T) {
	t.Run("text message", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte("hello"))
		if err != nil {
			t.Fatalf("NewPreparedMessage error: %v", err)
		}
		if pm == nil {
			t.Fatal("NewPreparedMessage returned nil")
		}
	})

	t.Run("binary message", func(t *testing.T) {
		pm, err := NewPreparedMessage(BinaryMessage, []byte{0x01, 0x02, 0x03})
		if err != nil {
			t.Fatalf("NewPreparedMessage error: %v", err)
		}
		if pm == nil {
			t.Fatal("NewPreparedMessage returned nil")
		}
	})

	t.Run("empty data", func(t *testing.T) {
		pm, err := NewPreparedMessage(TextMessage, []byte{})
		if err != nil {
			t.Fatalf("NewPreparedMessage error: %v", err)
		}
		if pm == nil {
			t.Fatal("NewPreparedMessage returned nil")
		}
	})

	t.Run("invalid message type", func(t *testing.T) {
		_, err := NewPreparedMessage(0, []byte("hello"))
		if err == nil {
			t.Error("Expected error for invalid message type")
		}
	})
}

func TestConn_WriteMessage(t *testing.T) {
	t.Run("server text message", func(t *testing.T) {
		conn, nc := newTestConn(true)
		err := conn.WriteMessage(TextMessage, []byte("hello"))
		if err != nil {
			t.Fatalf("WriteMessage error: %v", err)
		}
		if nc.buf.Len() == 0 {
			t.Error("Expected data to be written")
		}
	})

	t.Run("server binary message", func(t *testing.T) {
		conn, nc := newTestConn(true)
		err := conn.WriteMessage(BinaryMessage, []byte{1, 2, 3})
		if err != nil {
			t.Fatalf("WriteMessage error: %v", err)
		}
		if nc.buf.Len() == 0 {
			t.Error("Expected data to be written")
		}
	})

	t.Run("client text message", func(t *testing.T) {
		conn, nc := newTestConn(false)
		err := conn.WriteMessage(TextMessage, []byte("hello"))
		if err != nil {
			t.Fatalf("WriteMessage error: %v", err)
		}
		if nc.buf.Len() == 0 {
			t.Error("Expected data to be written")
		}
	})

	t.Run("invalid message type", func(t *testing.T) {
		conn, _ := newTestConn(true)
		err := conn.WriteMessage(0, []byte("hello"))
		if err == nil {
			t.Error("Expected error for invalid message type")
		}
	})
}

func TestConn_NextWriter(t *testing.T) {
	conn, _ := newTestConn(true)

	w, err := conn.NextWriter(TextMessage)
	if err != nil {
		t.Fatalf("NextWriter error: %v", err)
	}

	_, err = w.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

func TestConn_SetWriteDeadline(t *testing.T) {
	conn, _ := newTestConn(true)
	err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		t.Errorf("SetWriteDeadline error: %v", err)
	}
}

func TestConn_SetReadLimit(t *testing.T) {
	conn, _ := newTestConn(true)
	conn.SetReadLimit(1024)
	if conn.readLimit != 1024 {
		t.Errorf("readLimit = %d, want 1024", conn.readLimit)
	}
}

func TestConn_ReadWriteRoundTrip(t *testing.T) {
	// Create a pipe to simulate bidirectional stream
	serverR, clientW := io.Pipe()
	clientR, serverW := io.Pipe()

	serverStream := &pipeStream{r: serverR, w: serverW}
	clientStream := &pipeStream{r: clientR, w: clientW}

	serverConn := NewConn(nil, serverStream, true, 0, 0, nil, nil, nil)
	clientConn := NewConn(nil, clientStream, false, 0, 0, nil, nil, nil)

	msg := []byte("Hello, World!")

	// Write from server
	done := make(chan error, 1)
	go func() {
		done <- serverConn.WriteMessage(TextMessage, msg)
	}()

	// Read on client
	msgType, reader, err := clientConn.NextReader()
	if err != nil {
		t.Fatalf("NextReader error: %v", err)
	}
	if msgType != TextMessage {
		t.Errorf("messageType = %d, want %d", msgType, TextMessage)
	}
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !bytes.Equal(result, msg) {
		t.Errorf("Read = %q, want %q", result, msg)
	}

	if err := <-done; err != nil {
		t.Fatalf("WriteMessage error: %v", err)
	}
}

func TestConn_WritePreparedMessage(t *testing.T) {
	conn, nc := newTestConn(true)

	pm, err := NewPreparedMessage(TextMessage, []byte("prepared"))
	if err != nil {
		t.Fatalf("NewPreparedMessage error: %v", err)
	}

	if err := conn.WritePreparedMessage(pm); err != nil {
		t.Fatalf("WritePreparedMessage error: %v", err)
	}

	if nc.buf.Len() == 0 {
		t.Error("Expected data written for prepared message")
	}
}

func TestConn_LargeMessage(t *testing.T) {
	// Test with message larger than 125 bytes (triggers 2-byte length)
	conn, _ := newTestConn(true)
	msg := bytes.Repeat([]byte("x"), 200)
	if err := conn.WriteMessage(BinaryMessage, msg); err != nil {
		t.Fatalf("WriteMessage large error: %v", err)
	}

	// Test with message larger than 65535 bytes (triggers 8-byte length)
	conn2, _ := newTestConn(true)
	msg2 := bytes.Repeat([]byte("y"), 70000)
	if err := conn2.WriteMessage(BinaryMessage, msg2); err != nil {
		t.Fatalf("WriteMessage very large error: %v", err)
	}
}

// Helper function to create a test Conn with a mock stream
func newTestConn(isServer bool) (*Conn, *prepareConn) {
	nc := &prepareConn{}
	mu := make(chan struct{}, 1)
	mu <- struct{}{}
	c := &Conn{
		stream:       nc,
		mu:           mu,
		isServer:     isServer,
		br:           bufio.NewReaderSize(nc, defaultReadBufferSize),
		writeBuf:     make([]byte, defaultWriteBufferSize+maxFrameHeaderSize),
		writeBufSize: defaultWriteBufferSize + maxFrameHeaderSize,
	}
	c.writeDeadline.Store(time.Time{})
	return c, nc
}

// pipeStream implements streamWithDeadline using io.Pipe
type pipeStream struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (ps *pipeStream) Read(p []byte) (int, error)       { return ps.r.Read(p) }
func (ps *pipeStream) Write(p []byte) (int, error)      { return ps.w.Write(p) }
func (ps *pipeStream) Close() error                     { _ = ps.r.Close(); return ps.w.Close() }
func (ps *pipeStream) SetWriteDeadline(time.Time) error { return nil }
func (ps *pipeStream) SetReadDeadline(time.Time) error  { return nil }

// Ensure pipeStream implements streamWithDeadline
var _ streamWithDeadline = &pipeStream{}

// Verify netError implements net.Error
func TestNetErrorImplementsInterface(t *testing.T) {
	var _ net.Error = &netError{}
}
