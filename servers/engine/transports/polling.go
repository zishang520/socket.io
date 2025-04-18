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
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var polling_log = log.NewLog("engine:polling")

type polling struct {
	Transport

	closeTimeout time.Duration

	req     atomic.Pointer[types.HttpContext]
	dataCtx atomic.Pointer[types.HttpContext]

	shouldClose atomic.Pointer[types.Callable]
	mu          sync.Mutex
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

	p.closeTimeout = 30 * 1000 * time.Millisecond
}

func (p *polling) Name() string {
	return POLLING
}

// Overrides onRequest.
func (p *polling) OnRequest(ctx *types.HttpContext) {
	method := ctx.Method()

	if method == http.MethodGet {
		p.onPollRequest(ctx)
	} else if method == http.MethodPost {
		p.onDataRequest(ctx)
	} else {
		ctx.SetStatusCode(http.StatusInternalServerError)
		ctx.Write(nil)
	}
}

// The client sends a request awaiting for us to send data.
func (p *polling) onPollRequest(ctx *types.HttpContext) {
	if p.req.Load() != nil {
		polling_log.Debug("request overlap")
		// assert: p.res, '.req should be (un)set together'
		p.OnError("overlap from client", nil)
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.Write(nil)
		return
	}

	p.req.Store(ctx)

	polling_log.Debug("setting request")

	onClose := types.EventListener(func(...any) {
		p.SetWritable(false)
		p.OnError("poll connection closed prematurely", nil)
	})

	ctx.Cleanup = func() {
		ctx.RemoveListener("close", onClose)
		p.req.Store(nil)
	}

	ctx.Once("close", onClose)

	p.SetWritable(true)
	p.Emit("ready")

	// if we're still writable but had a pending close, trigger an empty send
	if p.Writable() && p.shouldClose.Load() != nil {
		polling_log.Debug("triggering empty send to append close packet")
		p.Send([]*packet.Packet{
			{
				Type: packet.NOOP,
			},
		})
	}
}

// The client sends a request with data.
func (p *polling) onDataRequest(ctx *types.HttpContext) {
	if p.dataCtx.Load() != nil {
		// assert: p.dataRes, '.dataCtx should be (un)set together'
		p.OnError("data request overlap from client", nil)
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.Write(nil)
		return
	}

	isBinary := ctx.Headers().Peek("Content-Type") == "application/octet-stream"

	if isBinary && p.Protocol() == 4 {
		p.OnError("invalid content", nil)
		return
	}

	p.dataCtx.Store(ctx)

	var cleanup types.Callable

	onClose := func(...any) {
		cleanup()
		p.OnError("data request connection closed prematurely", nil)
	}

	cleanup = func() {
		ctx.RemoveListener("close", onClose)
		p.dataCtx.Store(nil)
	}

	ctx.Once("close", onClose)

	if ctx.Request().ContentLength > p.MaxHttpBufferSize() {
		cleanup()

		ctx.SetStatusCode(http.StatusRequestEntityTooLarge)
		ctx.Write(nil)
		return
	}

	var packet types.BufferInterface
	if isBinary {
		packet = types.NewBytesBuffer(nil)
	} else {
		packet = types.NewStringBuffer(nil)
	}
	if body := ctx.Request().Body; body != nil {
		packet.ReadFrom(body)
		body.Close()
	}
	p.Proto().OnData(packet)

	cleanup()

	headers := utils.NewParameterBag(map[string][]string{
		// text/html is required instead of text/plain to avoid an
		// unwanted download dialog on certain user-agents (GH-43)
		"Content-Type":   {"text/html"},
		"Content-Length": {"2"},
	})

	// The following process in nodejs is asynchronous.
	ctx.ResponseHeaders.With(p.headers(ctx, headers).All())
	ctx.SetStatusCode(http.StatusOK)
	io.WriteString(ctx, "ok")
}

// Processes the incoming data payload.
func (p *polling) OnData(data types.BufferInterface) {
	polling_log.Debug(`received "%s"`, data)

	packets, _ := p.Parser().DecodePayload(data)
	for _, packetData := range packets {
		if packet.CLOSE == packetData.Type {
			polling_log.Debug("got xhr close packet")
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
	p.Transport.OnClose()
}

// Writes a packet payload.
func (p *polling) Send(packets []*packet.Packet) {
	p.SetWritable(false)
	go p.send(packets)
}
func (p *polling) send(packets []*packet.Packet) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if shouldClose := p.shouldClose.Load(); shouldClose != nil {
		polling_log.Debug("appending close packet to payload")
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
	option := &packet.Options{Compress: utils.Ptr(compress)}

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
	polling_log.Debug(`writing %#v`, data)
	ctx := p.req.Load()
	if ctx == nil {
		p.OnError("polling write error", nil)
		return
	}
	p.Proto().(Polling).DoWrite(ctx, data, options, func(err error) {
		if err != nil {
			p.OnError("polling write error", err)
			return
		}
		p.Emit("drain")
	})
}

// Performs the write.
func (p *polling) DoWrite(ctx *types.HttpContext, data types.BufferInterface, options *packet.Options, callback func(error)) {
	contentType := "application/octet-stream"
	// explicit UTF-8 is required for pages not served under utf
	switch data.(type) {
	case *types.StringBuffer:
		contentType = "text/plain; charset=UTF-8"
	}

	headers := utils.NewParameterBag(map[string][]string{
		"Content-Type": {contentType},
	})

	respond := func(data types.BufferInterface, length string) {
		ctx.Cleanup()
		defer callback(nil)

		headers.Set("Content-Length", length)
		ctx.ResponseHeaders.With(p.headers(ctx, headers).All())
		ctx.SetStatusCode(http.StatusOK)
		io.Copy(ctx, data)
	}

	if p.HttpCompression() == nil || (options != nil && options.Compress != nil && !*options.Compress) {
		respond(data, strconv.Itoa(data.Len()))
		return
	}

	if data.Len() < p.HttpCompression().Threshold {
		respond(data, strconv.Itoa(data.Len()))
		return
	}

	encoding := utils.Contains(ctx.Headers().Peek("Accept-Encoding"), []string{"gzip", "deflate", "br", "zstd"})
	if encoding == "" {
		respond(data, strconv.Itoa(data.Len()))
		return
	}

	buf, err := p.compress(data, encoding)
	if err != nil {
		ctx.Cleanup()
		defer callback(err)

		ctx.SetStatusCode(http.StatusInternalServerError)
		ctx.Write(nil)
		return
	}

	headers.Set("Content-Encoding", encoding)
	respond(buf, strconv.Itoa(buf.Len()))
}

// Compresses data.
func (p *polling) compress(data types.BufferInterface, encoding string) (types.BufferInterface, error) {
	polling_log.Debug("compressing")
	buf := types.NewBytesBuffer(nil)
	switch encoding {
	case "gzip":
		gz, err := gzip.NewWriterLevel(buf, gzip.DefaultCompression)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		if _, err := io.Copy(gz, data); err != nil {
			return nil, err
		}
	case "deflate":
		fl, err := flate.NewWriter(buf, flate.DefaultCompression)
		if err != nil {
			return nil, err
		}
		defer fl.Close()
		if _, err := io.Copy(fl, data); err != nil {
			return nil, err
		}
	case "br":
		br := brotli.NewWriterLevel(buf, brotli.DefaultCompression)
		defer br.Close()
		if _, err := io.Copy(br, data); err != nil {
			return nil, err
		}
	case "zstd":
		zd, err := zstd.NewWriter(buf, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			return nil, err
		}
		defer zd.Close()
		if _, err := io.Copy(zd, data); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// Closes the transport.
func (p *polling) DoClose(fn types.Callable) {
	polling_log.Debug("closing")

	if dataCtx := p.dataCtx.Load(); dataCtx != nil && !dataCtx.IsDone() {
		polling_log.Debug("aborting ongoing data request")
		dataCtx.ResponseHeaders.Set("Connection", "close")
		dataCtx.SetStatusCode(http.StatusTooManyRequests)
		dataCtx.Write(nil)
	}

	onClose := func() {
		if fn != nil {
			fn()
		}
		p.OnClose()
	}

	if p.Writable() {
		polling_log.Debug("transport writable - closing right away")
		p.Send([]*packet.Packet{
			{
				Type: packet.CLOSE,
			},
		})
		onClose()
	} else if p.Discarded() {
		polling_log.Debug("transport discarded - closing right away")
		onClose()
	} else {
		polling_log.Debug("transport not writable - buffering orderly close")
		closeTimeoutTimer := utils.SetTimeout(onClose, p.closeTimeout)
		shouldClose := func() {
			utils.ClearTimeout(closeTimeoutTimer)
			onClose()
		}
		p.shouldClose.Store(&shouldClose)
	}
}

// Returns headers for a response.
func (p *polling) headers(ctx *types.HttpContext, headers *utils.ParameterBag) *utils.ParameterBag {
	// prevent XSS warnings on IE
	// https://github.com/socketio/socket.io/pull/1333
	if ua := ctx.UserAgent(); (len(ua) > 0) && (strings.Contains(ua, ";MSIE") || strings.Contains(ua, "Trident/")) {
		headers.Set("X-XSS-Protection", "0")
	}
	headers.Set("Cache-Control", "no-store")
	p.Emit("headers", headers, ctx)
	return headers
}
