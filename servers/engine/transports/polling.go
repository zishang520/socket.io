// Package transports implements the HTTP long-polling transport for Engine.IO.
package transports

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var pollingLog = log.NewLog("engine:polling")

var (
	gzipWriterPool = sync.Pool{
		New: func() any {
			w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
			return w
		},
	}
	flateWriterPool = sync.Pool{
		New: func() any {
			w, _ := flate.NewWriter(io.Discard, flate.DefaultCompression)
			return w
		},
	}
	brotliWriterPool = sync.Pool{
		New: func() any {
			return brotli.NewWriterLevel(io.Discard, brotli.DefaultCompression)
		},
	}
	zstdWriterPool = sync.Pool{
		New: func() any {
			w, _ := zstd.NewWriter(io.Discard, zstd.WithEncoderLevel(zstd.SpeedDefault))
			return w
		},
	}
)

type pollingCompressionWriter interface {
	io.WriteCloser
	Reset(io.Writer)
}

const (
	// DefaultPollingCloseTimeout is the default time to wait for pending writes before closing a polling transport.
	DefaultPollingCloseTimeout = 30_000 * time.Millisecond
)

type polling struct {
	Transport

	closeTimeout time.Duration

	// Keep request ownership and writable/discard transitions together. This
	// lock is never held while emitting events or writing an HTTP response.
	reqMu   sync.Mutex
	req     *types.HttpContext
	dataCtx atomic.Pointer[types.HttpContext]

	shouldClose atomic.Pointer[types.Callable]
	writeQueue  *queue.Queue
}

// HTTP polling New.
func MakePolling() Polling {
	p := &polling{Transport: MakeTransport()}

	p.Prototype(p)

	return p
}

func NewPolling(ctx *types.HttpContext) Polling {
	p := MakePolling()

	p.Construct(ctx)

	return p
}

func (p *polling) Construct(ctx *types.HttpContext) {
	p.Transport.Construct(ctx)

	p.closeTimeout = DefaultPollingCloseTimeout
	p.writeQueue = queue.New()
}

func (p *polling) Name() string {
	return POLLING
}

// Overrides onRequest.
func (p *polling) OnRequest(ctx *types.HttpContext) {
	if ctx.IsDone() || ctx.Context().Err() != nil {
		return
	}
	method := ctx.Method()

	switch method {
	case http.MethodGet:
		p.onPollRequest(ctx)
	case http.MethodPost:
		p.onDataRequest(ctx)
	default:
		_ = ctx.SetStatusCode(http.StatusInternalServerError)
		_, _ = ctx.Write(nil)
	}
}

// The client sends a request awaiting for us to send data.
func (p *polling) onPollRequest(ctx *types.HttpContext) {
	onClose := types.EventListener(func(args ...any) {
		// The writer or a successor request retires a successful response.
		if len(args) > 0 && args[0] == nil {
			return
		}
		p.abortPollRequest(ctx)
	})

	_ = ctx.Once("close", onClose)

	// Subscribe before publishing the request so cancellation cannot be missed.
	p.reqMu.Lock()
	if ctx.IsDone() || ctx.Context().Err() != nil {
		p.reqMu.Unlock()
		ctx.RemoveListener("close", onClose)
		return
	}
	if p.req != nil && p.req.ResponseCommitted() {
		// The client can receive a response before DoWrite calls back.
		// Cancellation alone does not commit a response.
		p.SetWritable(false)
		p.req = nil
	}
	discarded := p.Discarded()
	if discarded || p.req != nil {
		p.reqMu.Unlock()
		ctx.RemoveListener("close", onClose)
		if !discarded {
			pollingLog.Debug("request overlap")
			p.OnError("overlap from client", nil)
		}
		_ = ctx.SetStatusCode(http.StatusBadRequest)
		_, _ = ctx.Write(nil)
		return
	}
	p.req = ctx
	p.SetWritable(true)
	p.reqMu.Unlock()

	pollingLog.Debug("setting request")
	// Discard can race registration during an upgrade. Publish writable first
	// so a later discard also sees the pending response when closing.
	if p.Discarded() {
		p.releasePollRequest(ctx)
		ctx.RemoveListener("close", onClose)
		_ = ctx.SetStatusCode(http.StatusBadRequest)
		_, _ = ctx.Write(nil)
		return
	}
	p.Emit("ready")

	// if we're still writable but had a pending close, trigger an empty send
	if p.Writable() && p.shouldClose.Load() != nil {
		pollingLog.Debug("triggering empty send to append close packet")
		p.Send([]*packet.Packet{
			{
				Type: packet.NOOP,
			},
		})
	}
}

func (p *polling) releasePollRequest(ctx *types.HttpContext) {
	p.reqMu.Lock()
	if p.req == ctx {
		p.SetWritable(false)
		p.req = nil
	}
	p.reqMu.Unlock()
}

func (p *polling) abortPollRequest(ctx *types.HttpContext) {
	p.reqMu.Lock()
	if p.req != ctx || ctx.ResponseCommitted() {
		p.reqMu.Unlock()
		return
	}
	// Reject a successor before releasing the canceled request's slot.
	p.Discard()
	p.SetWritable(false)
	p.req = nil
	p.reqMu.Unlock()
	p.OnError("poll connection closed prematurely", nil)
}

// The client sends a request with data.
func (p *polling) onDataRequest(ctx *types.HttpContext) {
	if p.dataCtx.Load() != nil {
		// assert: p.dataRes, '.dataCtx should be (un)set together'
		p.OnError("data request overlap from client", nil)
		_ = ctx.SetStatusCode(http.StatusBadRequest)
		_, _ = ctx.Write(nil)
		return
	}

	isBinary := ctx.Headers().Peek("Content-Type") == "application/octet-stream"

	if isBinary && p.Protocol() == 4 {
		p.OnError("invalid content", nil)
		_ = ctx.SetStatusCode(http.StatusBadRequest)
		_, _ = ctx.Write(nil)
		return
	}

	p.dataCtx.Store(ctx)

	var cleanup types.Callable

	onClose := func(...any) {
		if cleanup != nil {
			cleanup()
		}
		p.OnError("data request connection closed prematurely", nil)
	}

	cleanup = func() {
		ctx.RemoveListener("close", onClose)
		p.dataCtx.Store(nil)
	}

	_ = ctx.Once("close", onClose)

	if ctx.Request().ContentLength > p.MaxHttpBufferSize() {
		cleanup()

		_ = ctx.SetStatusCode(http.StatusRequestEntityTooLarge)
		_, _ = ctx.Write(nil)
		return
	}

	var packet types.BufferInterface
	if isBinary {
		packet = types.NewBytesBuffer(nil)
	} else {
		packet = types.NewStringBuffer(nil)
	}
	if body := ctx.Request().Body; body != nil {
		_, _ = packet.ReadFrom(io.LimitReader(body, p.MaxHttpBufferSize()))
		_ = body.Close()
	}
	p.Proto().OnData(packet)

	cleanup()

	headers := types.NewParameterBag(map[string][]string{
		// text/html is required instead of text/plain to avoid an
		// unwanted download dialog on certain user-agents (GH-43)
		"Content-Type":   {"text/html"},
		"Content-Length": {"2"},
	})

	// The following process in nodejs is asynchronous.
	ctx.ResponseHeaders().With(p.headers(ctx, headers).All())
	_ = ctx.SetStatusCode(http.StatusOK)
	_, _ = io.WriteString(ctx, "ok")
}

// Processes the incoming data payload.
func (p *polling) OnData(data types.BufferInterface) {
	pollingLog.Debug(`received "%s"`, data)

	packets, _ := p.Parser().DecodePayload(data)
	for _, packetData := range packets {
		if packet.CLOSE == packetData.Type {
			pollingLog.Debug("got xhr close packet")
			p.OnClose()
			return
		}

		p.OnPacket(packetData)
	}
}

// Overrides onClose.
func (p *polling) OnClose() {
	if p.Writable() {
		// close pending poll request
		p.Send([]*packet.Packet{
			{
				Type: packet.NOOP,
			},
		})
	}
	// Always tear down the write queue, even on ungraceful disconnects
	// where DoClose never runs. See the matching override on websocket
	// for the full rationale.
	p.writeQueue.TryClose()
	p.Transport.OnClose()
}

// Writes a packet payload.
func (p *polling) Send(packets []*packet.Packet) {
	p.SetWritable(false)
	p.writeQueue.Enqueue(func() { p.send(packets) })
}
func (p *polling) send(packets []*packet.Packet) {
	if shouldClose := p.shouldClose.Load(); shouldClose != nil {
		pollingLog.Debug("appending close packet to payload")
		packets = append(packets, &packet.Packet{
			Type: packet.CLOSE,
		})
		(*shouldClose)()
		p.shouldClose.Store(nil)
	}

	compress := false
	for _, packetData := range packets {
		if packetData.Options != nil && packetData.Options.Compress != nil && *packetData.Options.Compress {
			compress = true
			break
		}
	}
	option := &packet.Options{Compress: new(compress)}

	if p.Protocol() == 3 {
		data, _ := p.Parser().EncodePayload(packets, p.SupportsBinary())
		p.write(data, option)
	} else {
		data, _ := p.Parser().EncodePayload(packets)
		p.write(data, option)
	}
}

// Writes data as response to poll request.
func (p *polling) write(data types.BufferInterface, options *packet.Options) {
	pollingLog.Debug(`writing %#v`, data)
	p.reqMu.Lock()
	ctx := p.req
	p.reqMu.Unlock()
	if ctx == nil {
		p.OnError("polling write error", nil)
		return
	}
	if ctx.Context().Err() != nil || !ctx.BeginResponse() {
		p.abortPollRequest(ctx)
		return
	}
	var completed atomic.Bool
	// The callback can be asynchronous. A panic before DoWrite returns must
	// still release the response, without releasing it twice after a callback.
	returned := false
	defer func() {
		if !returned && completed.CompareAndSwap(false, true) {
			ctx.EndResponse()
		}
	}()
	p.Proto().(Polling).DoWrite(ctx, data, options, func(err error) {
		if !completed.CompareAndSwap(false, true) {
			return
		}
		ctx.EndResponse()
		if !ctx.ResponseCommitted() && ctx.Context().Err() != nil {
			p.abortPollRequest(ctx)
			return
		}
		p.releasePollRequest(ctx)
		if err != nil {
			p.OnError("polling write error", err)
			return
		}
		p.Emit("drain")
	})
	returned = true
}

// Performs the write.
func (p *polling) DoWrite(ctx *types.HttpContext, data types.BufferInterface, options *packet.Options, callback func(error)) {
	contentType := "application/octet-stream"
	// explicit UTF-8 is required for pages not served under utf
	switch data.(type) {
	case *types.StringBuffer:
		contentType = "text/plain; charset=UTF-8"
	}

	headers := types.NewParameterBag(map[string][]string{
		"Content-Type": {contentType},
	})

	compression := p.HttpCompression()
	if compression != nil &&
		(options == nil || options.Compress == nil || *options.Compress) &&
		data.Len() >= compression.Threshold {
		encoding := utils.Contains(ctx.Headers().Peek("Accept-Encoding"), []string{"gzip", "deflate", "br", "zstd"})
		if encoding != "" {
			var err error
			data, err = p.compress(data, encoding)
			if err != nil {
				defer callback(err)
				_ = ctx.SetStatusCode(http.StatusInternalServerError)
				_, _ = ctx.Write(nil)
				return
			}
			headers.Set("Content-Encoding", encoding)
		}
	}

	defer callback(nil)
	headers.Set("Content-Length", strconv.Itoa(data.Len()))
	ctx.ResponseHeaders().With(p.headers(ctx, headers).All())
	_ = ctx.SetStatusCode(http.StatusOK)
	_, _ = io.Copy(ctx, data)
}

// Compresses data.
func (p *polling) compress(data types.BufferInterface, encoding string) (types.BufferInterface, error) {
	pollingLog.Debug("compressing")
	buf := types.NewBytesBuffer(nil)
	var pool *sync.Pool
	switch encoding {
	case "gzip":
		pool = &gzipWriterPool
	case "deflate":
		pool = &flateWriterPool
	case "br":
		pool = &brotliWriterPool
	case "zstd":
		pool = &zstdWriterPool
	default:
		return buf, nil
	}
	writer := pool.Get().(pollingCompressionWriter)
	writer.Reset(buf)
	_, err := io.Copy(writer, data)
	closeErr := writer.Close()
	pool.Put(writer)
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return buf, nil
}

// Closes the transport.
func (p *polling) DoClose(fn types.Callable) {
	pollingLog.Debug("closing")

	if dataCtx := p.dataCtx.Load(); dataCtx != nil && !dataCtx.IsDone() {
		pollingLog.Debug("aborting ongoing data request")
		dataCtx.ResponseHeaders().Set("Connection", "close")
		_ = dataCtx.SetStatusCode(http.StatusTooManyRequests)
		_, _ = dataCtx.Write(nil)
	}

	onClose := func() {
		if fn != nil {
			fn()
		}
		p.OnClose()
	}

	if p.Writable() {
		pollingLog.Debug("transport writable - closing right away")
		p.Send([]*packet.Packet{
			{
				Type: packet.CLOSE,
			},
		})
		onClose()
	} else if p.Discarded() {
		pollingLog.Debug("transport discarded - closing right away")
		onClose()
	} else {
		pollingLog.Debug("transport not writable - buffering orderly close")
		closeTimeoutTimer := utils.SetTimeout(onClose, p.closeTimeout)
		shouldClose := func() {
			utils.ClearTimeout(closeTimeoutTimer)
			onClose()
		}
		p.shouldClose.Store(&shouldClose)
	}
}

// Returns headers for a response.
func (p *polling) headers(ctx *types.HttpContext, headers *types.ParameterBag) *types.ParameterBag {
	// prevent XSS warnings on IE
	// https://github.com/socketio/socket.io/pull/1333
	if ua := ctx.UserAgent(); (len(ua) > 0) && (strings.Contains(ua, ";MSIE") || strings.Contains(ua, "Trident/")) {
		headers.Set("X-XSS-Protection", "0")
	}
	headers.Set("Cache-Control", "no-store")
	p.Emit("headers", headers, ctx)
	return headers
}
