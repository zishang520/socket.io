// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webtransport

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/zishang520/webtransport-go"
)

const (
	maxFrameHeaderSize = 1 + 8 // Fixed header + length

	defaultReadBufferSize  = 4096
	defaultWriteBufferSize = 4096

	continuationFrame = 0
	noFrame           = -1
)

// Close codes defined in RFC 6455, section 11.7.
const (
	CloseNormalClosure           = 1000
	CloseGoingAway               = 1001
	CloseProtocolError           = 1002
	CloseUnsupportedData         = 1003
	CloseNoStatusReceived        = 1005
	CloseAbnormalClosure         = 1006
	CloseInvalidFramePayloadData = 1007
	ClosePolicyViolation         = 1008
	CloseMessageTooBig           = 1009
	CloseMandatoryExtension      = 1010
	CloseInternalServerErr       = 1011
	CloseServiceRestart          = 1012
	CloseTryAgainLater           = 1013
	CloseTLSHandshake            = 1015
)

// The message types are defined in RFC 6455, section 11.8.
const (
	// TextMessage denotes a text data message. The text message payload is
	// interpreted as UTF-8 encoded text data.
	TextMessage = 1

	// BinaryMessage denotes a binary data message.
	BinaryMessage = 2
)

// ErrCloseSent is returned when the application writes a message to the
// connection after sending a close message.
var ErrCloseSent = errors.New("webtransport: close sent")

// ErrReadLimit is returned when reading a message that is larger than the
// read limit set for the connection.
var ErrReadLimit = errors.New("webtransport: read limit exceeded")

// netError satisfies the net Error interface.
type netError struct {
	msg       string
	temporary bool
	timeout   bool
}

func (e *netError) Error() string   { return e.msg }
func (e *netError) Temporary() bool { return e.temporary }
func (e *netError) Timeout() bool   { return e.timeout }

// CloseError represents a close message.
type CloseError struct {
	// Code is defined in RFC 6455, section 11.7.
	Code int

	// Text is the optional text payload.
	Text string
}

func (e *CloseError) Error() string {
	s := []byte("webtransport: close ")
	s = strconv.AppendInt(s, int64(e.Code), 10)
	switch e.Code {
	case CloseNormalClosure:
		s = append(s, " (normal)"...)
	case CloseGoingAway:
		s = append(s, " (going away)"...)
	case CloseProtocolError:
		s = append(s, " (protocol error)"...)
	case CloseUnsupportedData:
		s = append(s, " (unsupported data)"...)
	case CloseNoStatusReceived:
		s = append(s, " (no status)"...)
	case CloseAbnormalClosure:
		s = append(s, " (abnormal closure)"...)
	case CloseInvalidFramePayloadData:
		s = append(s, " (invalid payload data)"...)
	case ClosePolicyViolation:
		s = append(s, " (policy violation)"...)
	case CloseMessageTooBig:
		s = append(s, " (message too big)"...)
	case CloseMandatoryExtension:
		s = append(s, " (mandatory extension missing)"...)
	case CloseInternalServerErr:
		s = append(s, " (internal server error)"...)
	case CloseTLSHandshake:
		s = append(s, " (TLS handshake error)"...)
	}
	if e.Text != "" {
		s = append(s, ": "...)
		s = append(s, e.Text...)
	}
	return string(s)
}

// IsCloseError returns boolean indicating whether the error is a *CloseError
// with one of the specified codes.
func IsCloseError(err error, codes ...int) bool {
	if e, ok := err.(*CloseError); ok {
		for _, code := range codes {
			if e.Code == code {
				return true
			}
		}
	}
	return false
}

// IsUnexpectedCloseError returns boolean indicating whether the error is a
// *CloseError with a code not in the list of expected codes.
func IsUnexpectedCloseError(err error, expectedCodes ...int) bool {
	if e, ok := err.(*CloseError); ok {
		for _, code := range expectedCodes {
			if e.Code == code {
				return false
			}
		}
		return true
	}
	return false
}

// IsWebTransportUpgrade returns true if the client requested upgrade to the
// WebTransport protocol.
func IsWebTransportUpgrade(r *http.Request) bool {
	return r.Method == http.MethodConnect && r.Proto == "webtransport" && tokenListContainsValue(r.Header, "Sec-Webtransport-Http3-Draft02", "1")
}

// tokenListContainsValue returns true if the 1#token header with the given
// name contains a token equal to value with ASCII case folding.
func tokenListContainsValue(header http.Header, name string, value string) bool {
headers:
	for _, s := range header[name] {
		for {
			var t string
			t, s = nextToken(skipSpace(s))
			if t == "" {
				continue headers
			}
			s = skipSpace(s)
			if s != "" && s[0] != ',' {
				continue headers
			}
			if equalASCIIFold(t, value) {
				return true
			}
			if s == "" {
				continue headers
			}
			s = s[1:]
		}
	}
	return false
}

// skipSpace returns a slice of the string s with all leading RFC 2616 linear
// whitespace removed.
func skipSpace(s string) (rest string) {
	i := 0
	for ; i < len(s); i++ {
		if b := s[i]; b != ' ' && b != '\t' {
			break
		}
	}
	return s[i:]
}

// Token octets per RFC 2616.
var isTokenOctet = [256]bool{
	'!':  true,
	'#':  true,
	'$':  true,
	'%':  true,
	'&':  true,
	'\'': true,
	'*':  true,
	'+':  true,
	'-':  true,
	'.':  true,
	'0':  true,
	'1':  true,
	'2':  true,
	'3':  true,
	'4':  true,
	'5':  true,
	'6':  true,
	'7':  true,
	'8':  true,
	'9':  true,
	'A':  true,
	'B':  true,
	'C':  true,
	'D':  true,
	'E':  true,
	'F':  true,
	'G':  true,
	'H':  true,
	'I':  true,
	'J':  true,
	'K':  true,
	'L':  true,
	'M':  true,
	'N':  true,
	'O':  true,
	'P':  true,
	'Q':  true,
	'R':  true,
	'S':  true,
	'T':  true,
	'U':  true,
	'W':  true,
	'V':  true,
	'X':  true,
	'Y':  true,
	'Z':  true,
	'^':  true,
	'_':  true,
	'`':  true,
	'a':  true,
	'b':  true,
	'c':  true,
	'd':  true,
	'e':  true,
	'f':  true,
	'g':  true,
	'h':  true,
	'i':  true,
	'j':  true,
	'k':  true,
	'l':  true,
	'm':  true,
	'n':  true,
	'o':  true,
	'p':  true,
	'q':  true,
	'r':  true,
	's':  true,
	't':  true,
	'u':  true,
	'v':  true,
	'w':  true,
	'x':  true,
	'y':  true,
	'z':  true,
	'|':  true,
	'~':  true,
}

// nextToken returns the leading RFC 2616 token of s and the string following
// the token.
func nextToken(s string) (token, rest string) {
	i := 0
	for ; i < len(s); i++ {
		if !isTokenOctet[s[i]] {
			break
		}
	}
	return s[:i], s[i:]
}

// equalASCIIFold returns true if s is equal to t with ASCII case folding as
// defined in RFC 4790.
func equalASCIIFold(s, t string) bool {
	for s != "" && t != "" {
		sr, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		tr, size := utf8.DecodeRuneInString(t)
		t = t[size:]
		if sr == tr {
			continue
		}
		if 'A' <= sr && sr <= 'Z' {
			sr = sr + 'a' - 'A'
		}
		if 'A' <= tr && tr <= 'Z' {
			tr = tr + 'a' - 'A'
		}
		if sr != tr {
			return false
		}
	}
	return s == t
}

var (
	errUnexpectedEOF  = &CloseError{Code: CloseAbnormalClosure, Text: io.ErrUnexpectedEOF.Error()}
	errBadWriteOpCode = errors.New("webtransport: bad write message type")
	errWriteClosed    = errors.New("webtransport: write closed")
)

func hideTempErr(err error) error {
	if e, ok := err.(net.Error); ok {
		err = &netError{msg: e.Error(), timeout: e.Timeout()}
	}
	return err
}

func isData(frameType int) bool {
	return frameType == TextMessage || frameType == BinaryMessage
}

// BufferPool represents a pool of buffers. The *sync.Pool type satisfies this
// interface.  The type of the value stored in a pool is not specified.
type BufferPool interface {
	// Get gets a value from the pool or returns nil if the pool is empty.
	Get() interface{}
	// Put adds a value to the pool.
	Put(interface{})
}

// writePoolData is the type added to the write buffer pool. This wrapper is
// used to prevent applications from peeking at and depending on the values
// added to the pool.
type writePoolData struct{ buf []byte }

// The Conn type represents a WebTransport connection.
type Conn struct {
	session *webtransport.Session
	stream  webtransport.Stream

	isServer bool

	// Write fields
	mu            chan struct{} // used as mutex to protect write to conn
	writeBuf      []byte        // frame is constructed in this buffer.
	writePool     BufferPool
	writeBufSize  int
	writeDeadline time.Time
	writer        io.WriteCloser // the current writer returned to the application
	isWriting     bool           // for best-effort concurrent write detection

	writeErrMu sync.Mutex
	writeErr   error

	// Read fields
	reader  io.ReadCloser // the current reader returned to the application
	readErr error
	br      *bufio.Reader
	// bytes remaining in current frame.
	// set setReadRemaining to safely update this value and prevent overflow
	readRemaining int64
	readLength    int64 // Message size.
	readLimit     int64 // Maximum message size.
	readErrCount  int
	messageReader *messageReader // the current low-level reader
}

func NewConn(session *webtransport.Session, stream webtransport.Stream, isServer bool, readBufferSize, writeBufferSize int, writeBufferPool BufferPool, br *bufio.Reader, writeBuf []byte) *Conn {

	if br == nil {
		if readBufferSize == 0 {
			readBufferSize = defaultReadBufferSize
		}
		br = bufio.NewReaderSize(stream, readBufferSize)
	}

	if writeBufferSize <= 0 {
		writeBufferSize = defaultWriteBufferSize
	}
	writeBufferSize += maxFrameHeaderSize

	if writeBuf == nil && writeBufferPool == nil {
		writeBuf = make([]byte, writeBufferSize)
	}

	mu := make(chan struct{}, 1)
	mu <- struct{}{}
	c := &Conn{
		isServer:     isServer,
		br:           br,
		session:      session,
		stream:       stream,
		mu:           mu,
		writeBuf:     writeBuf,
		writePool:    writeBufferPool,
		writeBufSize: writeBufferSize,
	}
	return c
}

// setReadRemaining tracks the number of bytes remaining on the connection. If n
// overflows, an ErrReadLimit is returned.
func (c *Conn) setReadRemaining(n int64) error {
	if n < 0 {
		return ErrReadLimit
	}

	c.readRemaining = n
	return nil
}

// Close closes the underlying network connection without sending or waiting
// for a close message.
func (c *Conn) CloseWithError(code webtransport.SessionErrorCode, msg string) error {
	return c.session.CloseWithError(code, msg)
}

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	return c.session.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.session.RemoteAddr()
}

// Write methods
func (c *Conn) writeFatal(err error) error {
	err = hideTempErr(err)
	c.writeErrMu.Lock()
	if c.writeErr == nil {
		c.writeErr = err
	}
	c.writeErrMu.Unlock()
	return err
}

func (c *Conn) read(n int) ([]byte, error) {
	p, err := c.br.Peek(n)
	if err == io.EOF {
		err = errUnexpectedEOF
	}
	if _, err := c.br.Discard(len(p)); err != nil {
		return p, err
	}
	return p, err
}

func (c *Conn) write(frameType int, deadline time.Time, buf0, buf1 []byte) error {
	<-c.mu
	defer func() { c.mu <- struct{}{} }()

	c.writeErrMu.Lock()
	err := c.writeErr
	c.writeErrMu.Unlock()
	if err != nil {
		return err
	}

	if err := c.stream.SetWriteDeadline(deadline); err != nil {
		return c.writeFatal(err)
	}
	if len(buf1) == 0 {
		_, err = c.stream.Write(buf0)
	} else {
		err = c.writeBufs(buf0, buf1)
	}
	if err != nil {
		return c.writeFatal(err)
	}

	return nil
}

func (c *Conn) writeBufs(bufs ...[]byte) error {
	b := net.Buffers(bufs)
	_, err := b.WriteTo(c.stream)
	return err
}

// beginMessage prepares a connection and message writer for a new message.
func (c *Conn) beginMessage(mw *messageWriter, messageType int) error {
	// Close previous writer if not already closed by the application. It's
	// probably better to return an error in this situation, but we cannot
	// change this without breaking existing applications.
	if c.writer != nil {
		if err := c.writer.Close(); err != nil {
			log.Printf("webtransport: discarding writer close error: %v", err)
		}
		c.writer = nil
	}

	if !isData(messageType) {
		return errBadWriteOpCode
	}

	c.writeErrMu.Lock()
	err := c.writeErr
	c.writeErrMu.Unlock()
	if err != nil {
		return err
	}

	mw.c = c
	mw.frameType = messageType
	mw.pos = maxFrameHeaderSize

	if c.writeBuf == nil {
		wpd, ok := c.writePool.Get().(writePoolData)
		if ok {
			c.writeBuf = wpd.buf
		} else {
			c.writeBuf = make([]byte, c.writeBufSize)
		}
	}
	return nil
}

// NextWriter returns a writer for the next message to send. The writer's Close
// method flushes the complete message to the network.
//
// There can be at most one open writer on a connection. NextWriter closes the
// previous writer if the application has not already done so.
//
// All message types (TextMessage, BinaryMessage, CloseMessage, PingMessage and
// PongMessage) are supported.
func (c *Conn) NextWriter(messageType int) (io.WriteCloser, error) {
	var mw messageWriter
	if err := c.beginMessage(&mw, messageType); err != nil {
		return nil, err
	}
	c.writer = &mw
	return c.writer, nil
}

type messageWriter struct {
	c         *Conn
	pos       int // end of data in writeBuf.
	frameType int // type of the current frame.
	err       error
}

func (w *messageWriter) endMessage(err error) error {
	if w.err != nil {
		return err
	}
	c := w.c
	w.err = err
	c.writer = nil
	if c.writePool != nil {
		c.writePool.Put(writePoolData{buf: c.writeBuf})
		c.writeBuf = nil
	}
	return err
}

// flushFrame writes buffered data and extra as a frame to the network. The
// final argument indicates that this is the last frame in the message.
func (w *messageWriter) flushFrame(final bool, extra []byte) error {
	c := w.c
	length := w.pos - maxFrameHeaderSize + len(extra)

	b0 := (byte(w.frameType) - 1) << 7

	b1 := byte(0)

	// Assume that the frame starts at beginning of c.writeBuf.
	framePos := 0

	switch {
	case length >= 65536:
		c.writeBuf[framePos] = b1 | 127 | b0
		binary.BigEndian.PutUint64(c.writeBuf[framePos+1:], uint64(length))
	case length > 125:
		framePos += 6
		c.writeBuf[framePos] = b1 | 126 | b0
		binary.BigEndian.PutUint16(c.writeBuf[framePos+1:], uint16(length))
	default:
		framePos += 8
		c.writeBuf[framePos] = b1 | byte(length) | b0
	}

	// Write the buffers to the connection with best-effort detection of
	// concurrent writes. See the concurrency section in the package
	// documentation for more info.

	if c.isWriting {
		panic("concurrent write to webtransport connection")
	}
	c.isWriting = true

	err := c.write(w.frameType, c.writeDeadline, c.writeBuf[framePos:w.pos], extra)

	if !c.isWriting {
		panic("concurrent write to webtransport connection")
	}
	c.isWriting = false

	if err != nil {
		return w.endMessage(err)
	}

	if final {
		_ = w.endMessage(errWriteClosed)
		return nil
	}

	// Setup for next frame.
	w.pos = maxFrameHeaderSize
	w.frameType = continuationFrame
	return nil
}

func (w *messageWriter) ncopy(max int) (int, error) {
	n := len(w.c.writeBuf) - w.pos
	if n <= 0 {
		if err := w.flushFrame(false, nil); err != nil {
			return 0, err
		}
		n = len(w.c.writeBuf) - w.pos
	}
	if n > max {
		n = max
	}
	return n, nil
}

func (w *messageWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	if len(p) > 2*len(w.c.writeBuf) && w.c.isServer {
		// Don't buffer large messages.
		err := w.flushFrame(false, p)
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}

	nn := len(p)
	for len(p) > 0 {
		n, err := w.ncopy(len(p))
		if err != nil {
			return 0, err
		}
		copy(w.c.writeBuf[w.pos:], p[:n])
		w.pos += n
		p = p[n:]
	}
	return nn, nil
}

func (w *messageWriter) WriteString(p string) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	nn := len(p)
	for len(p) > 0 {
		n, err := w.ncopy(len(p))
		if err != nil {
			return 0, err
		}
		copy(w.c.writeBuf[w.pos:], p[:n])
		w.pos += n
		p = p[n:]
	}
	return nn, nil
}

func (w *messageWriter) ReadFrom(r io.Reader) (nn int64, err error) {
	if w.err != nil {
		return 0, w.err
	}
	for {
		if w.pos == len(w.c.writeBuf) {
			err = w.flushFrame(false, nil)
			if err != nil {
				break
			}
		}
		var n int
		n, err = r.Read(w.c.writeBuf[w.pos:])
		w.pos += n
		nn += int64(n)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
	}
	return nn, err
}

func (w *messageWriter) Close() error {
	if w.err != nil {
		return w.err
	}
	return w.flushFrame(true, nil)
}

// WritePreparedMessage writes prepared message into connection.
func (c *Conn) WritePreparedMessage(pm *PreparedMessage) error {
	frameType, frameData, err := pm.frame(prepareKey{
		isServer: c.isServer,
	})
	if err != nil {
		return err
	}
	if c.isWriting {
		panic("concurrent write to webtransport connection")
	}
	c.isWriting = true
	err = c.write(frameType, c.writeDeadline, frameData, nil)
	if !c.isWriting {
		panic("concurrent write to webtransport connection")
	}
	c.isWriting = false
	return err
}

// WriteMessage is a helper method for getting a writer using NextWriter,
// writing the message and closing the writer.
func (c *Conn) WriteMessage(messageType int, data []byte) error {

	if c.isServer {
		// Fast path with no allocations and single frame.

		var mw messageWriter
		if err := c.beginMessage(&mw, messageType); err != nil {
			return err
		}
		n := copy(c.writeBuf[mw.pos:], data)
		mw.pos += n
		data = data[n:]
		return mw.flushFrame(true, data)
	}

	w, err := c.NextWriter(messageType)
	if err != nil {
		return err
	}
	if _, err = w.Write(data); err != nil {
		return err
	}
	return w.Close()
}

// SetWriteDeadline sets the write deadline on the underlying network
// connection. After a write has timed out, the webtransport state is corrupt and
// all future writes will return an error. A zero value for t means writes will
// not time out.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

// Read methods
func (c *Conn) advanceFrame() (int, error) {
	// 1. Skip remainder of previous frame.

	if c.readRemaining > 0 {
		if _, err := io.CopyN(io.Discard, c.br, c.readRemaining); err != nil {
			return noFrame, err
		}
	}

	// 2. Read and parse first one bytes of frame header.
	// To aid debugging, collect and report all errors in the first one bytes
	// of the header.

	p, err := c.read(1)
	if err != nil {
		return noFrame, err
	}

	frameType := (int(p[0]&0x80) >> 7) + 1
	if err := c.setReadRemaining(int64(p[0] & 0x7f)); err != nil {
		return noFrame, err
	}

	// 3. Read and parse frame length as per
	// https://tools.ietf.org/html/rfc6455#section-5.2
	//
	// The length of the "Payload data", in bytes: if 0-125, that is the payload
	// length.
	// - If 126, the following 2 bytes interpreted as a 16-bit unsigned
	// integer are the payload length.
	// - If 127, the following 8 bytes interpreted as
	// a 64-bit unsigned integer (the most significant bit MUST be 0) are the
	// payload length. Multibyte length quantities are expressed in network byte
	// order.

	switch c.readRemaining {
	case 126:
		p, err := c.read(2)
		if err != nil {
			return noFrame, err
		}

		if err := c.setReadRemaining(int64(binary.BigEndian.Uint16(p))); err != nil {
			return noFrame, err
		}
	case 127:
		p, err := c.read(8)
		if err != nil {
			return noFrame, err
		}

		if err := c.setReadRemaining(int64(binary.BigEndian.Uint64(p))); err != nil {
			return noFrame, err
		}
	}

	// 4. For text and binary messages, enforce read limit and return.

	if frameType == TextMessage || frameType == BinaryMessage {

		c.readLength += c.readRemaining
		// Don't allow readLength to overflow in the presence of a large readRemaining
		// counter.
		if c.readLength < 0 {
			return noFrame, ErrReadLimit
		}

		if c.readLimit > 0 && c.readLength > c.readLimit {
			if err := c.CloseWithError(CloseMessageTooBig, ""); err != nil {
				return noFrame, err
			}
			return noFrame, ErrReadLimit
		}

		return frameType, nil
	}

	return frameType, nil
}

// NextReader returns the next data message received from the peer. The
// returned messageType is either TextMessage or BinaryMessage.
//
// There can be at most one open reader on a connection. NextReader discards
// the previous message if the application has not already consumed it.
//
// Applications must break out of the application's read loop when this method
// returns a non-nil error value. Errors returned from this method are
// permanent. Once this method returns a non-nil error, all subsequent calls to
// this method return the same error.
func (c *Conn) NextReader() (messageType int, r io.Reader, err error) {
	// Close previous reader, only relevant for decompression.
	if c.reader != nil {
		if err := c.reader.Close(); err != nil {
			log.Printf("webtransport: discarding reader close error: %v", err)
		}
		c.reader = nil
	}

	c.messageReader = nil
	c.readLength = 0

	for c.readErr == nil {
		frameType, err := c.advanceFrame()
		if err != nil {
			c.readErr = hideTempErr(err)
			break
		}

		if frameType == TextMessage || frameType == BinaryMessage {
			c.messageReader = &messageReader{c}
			c.reader = c.messageReader
			return frameType, c.reader, nil
		}
	}

	// Applications that do handle the error returned from this method spin in
	// tight loop on connection failure. To help application developers detect
	// this error, panic on repeated reads to the failed connection.
	c.readErrCount++
	if c.readErrCount >= 1000 {
		panic("repeated read on failed webtransport connection")
	}

	return noFrame, nil, c.readErr
}

type messageReader struct{ c *Conn }

func (r *messageReader) Read(b []byte) (int, error) {
	c := r.c
	if c.messageReader != r {
		return 0, io.EOF
	}

	for c.readErr == nil {
		if c.readRemaining > 0 {
			if int64(len(b)) > c.readRemaining {
				b = b[:c.readRemaining]
			}
			n, err := c.br.Read(b)
			c.readErr = hideTempErr(err)
			rem := c.readRemaining
			rem -= int64(n)
			if err := c.setReadRemaining(rem); err != nil {
				return 0, err
			}
			if c.readRemaining > 0 && c.readErr == io.EOF {
				c.readErr = errUnexpectedEOF
			}
			return n, c.readErr
		}

		// The frame data of websocket is not fully implemented and ends after receiving it.
		c.messageReader = nil
		return 0, io.EOF
	}

	err := c.readErr
	if err == io.EOF && c.messageReader == r {
		err = errUnexpectedEOF
	}
	return 0, err
}

func (r *messageReader) Close() error {
	return nil
}

// ReadMessage is a helper method for getting a reader using NextReader and
// reading from that reader to a buffer.
func (c *Conn) ReadMessage() (messageType int, p []byte, err error) {
	var r io.Reader
	messageType, r, err = c.NextReader()
	if err != nil {
		return messageType, nil, err
	}
	p, err = io.ReadAll(r)
	return messageType, p, err
}

// SetReadDeadline sets the read deadline on the underlying network connection.
// After a read has timed out, the webtransport connection state is corrupt and
// all future reads will return an error. A zero value for t means reads will
// not time out.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

// SetReadLimit sets the maximum size in bytes for a message read from the peer. If a
// message exceeds the limit, the connection sends a close message to the peer
// and returns ErrReadLimit to the application.
func (c *Conn) SetReadLimit(limit int64) {
	c.readLimit = limit
}

// Stream returns the underlying connection that is wrapped by c.
// Note that writing to or reading from this connection directly will corrupt the
// WebTransport stream.
func (c *Conn) Stream() webtransport.Stream {
	return c.stream
}

// Session returns the underlying connection that is wrapped by c.
// Note that writing to or reading from this connection directly will corrupt the
// WebTransport session.
func (c *Conn) Session() *webtransport.Session {
	return c.session
}
