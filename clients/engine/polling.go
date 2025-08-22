package engine

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/request"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// polling implements the HTTP long-polling transport for Engine.IO.
// It provides real-time communication by repeatedly making HTTP requests.
// Polling serves as a fallback when WebSocket is not available and ensures maximum compatibility.
//
// Features:
//   - HTTP/HTTPS support
//   - Automatic reconnection
//   - Binary data support (via base64 encoding)
//   - Cross-domain compatibility
//   - Timeout handling
type polling struct {
	Transport

	// client is the HTTP client used for polling requests (GET/POST).
	client *request.HTTPClient

	// _polling indicates if a polling request is currently in progress.
	_polling atomic.Bool
}

// Name returns the identifier for the polling transport ("polling").
func (p *polling) Name() string {
	return transports.POLLING
}

// MakePolling creates a new Polling transport instance with default settings.
func MakePolling() Polling {
	s := &polling{
		Transport: MakeTransport(),
	}
	s._polling.Store(false)
	s.Prototype(s)
	return s
}

// NewPolling creates a new Polling transport instance with the specified socket and options.
func NewPolling(socket Socket, opts SocketOptionsInterface) Polling {
	s := MakePolling()
	s.Construct(socket, opts)
	return s
}

// Construct initializes the Polling transport with the given socket and options.
func (p *polling) Construct(socket Socket, opts SocketOptionsInterface) {
	p.Transport.Construct(socket, opts)
	p.client = request.NewHTTPClient(
		request.WithLogger(NewLog("HTTPClient")),
		request.WithTimeout(p.Opts().RequestTimeout()),
		request.WithCookieJar(p.Socket().CookieJar()),
		request.WithTransport(request.NewTransport(p.Opts().TLSClientConfig(), p.Opts().QUICConfig())),
	)
}

// DoOpen starts the polling cycle to establish the connection.
func (p *polling) DoOpen() {
	p._poll()
}

// Pause suspends the polling transport, used during upgrades to prevent packet loss.
// onPause is called when the transport is paused.
func (p *polling) Pause(onPause func()) {
	p.SetReadyState(TransportStatePausing)
	pause := func() {
		client_polling_log.Debug("paused")
		p.SetReadyState(TransportStatePaused)
		onPause()
	}
	if p._polling.Load() || !p.Writable() {
		var total atomic.Uint32
		if p._polling.Load() {
			client_polling_log.Debug("we are currently polling - waiting to pause")
			total.Add(1)
			p.Once("pollComplete", func(...any) {
				client_polling_log.Debug("pre-pause polling complete")
				if total.Add(^uint32(0)) == 0 {
					pause()
				}
			})
		}
		if !p.Writable() {
			total.Add(1)
			p.Once("drain", func(...any) {
				client_polling_log.Debug("pre-pause writing complete")
				if total.Add(^uint32(0)) == 0 {
					pause()
				}
			})
		}
	} else {
		pause()
	}
}

// _poll starts a new polling cycle and initiates a new polling request.
func (p *polling) _poll() {
	client_polling_log.Debug("polling")
	p._polling.Store(true)
	p.Emit("poll")
	go p.doPoll()
}

// _onPacket handles incoming packets and updates the transport state accordingly.
func (p *polling) _onPacket(data *packet.Packet) {
	if TransportStateOpening == p.ReadyState() && data.Type == packet.OPEN {
		p.OnOpen()
	}
	if packet.CLOSE == data.Type {
		p.OnClose(errors.New("transport closed by the server"))
		return
	}
	p.OnPacket(data)
}

// OnData decodes the payload and handles each packet in the payload.
func (p *polling) OnData(data types.BufferInterface) {
	client_polling_log.Debug("polling got data %#v", data)
	packets, _ := parser.Parserv4().DecodePayload(data)
	for _, data := range packets {
		p._onPacket(data)
	}
	if readyState := p.ReadyState(); TransportStateClosed != readyState {
		p._polling.Store(false)
		p.Emit("pollComplete")
		if TransportStateOpen == readyState {
			p._poll()
		} else {
			client_polling_log.Debug(`ignoring poll - transport state "%s"`, readyState)
		}
	}
}

// DoClose gracefully closes the polling transport, sending a close packet if needed.
func (p *polling) DoClose() {
	defer p.client.Close()
	cleanup := func(...any) {
		client_polling_log.Debug("writing close packet")
		p.Write([]*packet.Packet{{Type: packet.CLOSE}})
	}
	if TransportStateOpen == p.ReadyState() {
		client_polling_log.Debug("transport open - closing")
		cleanup()
	} else {
		client_polling_log.Debug("transport not open - deferring close")
		p.Once("open", cleanup)
	}
}

// Write encodes and sends packets to the server asynchronously.
func (p *polling) Write(packets []*packet.Packet) {
	p.SetWritable(false)
	go p.write(packets)
}

// write performs the actual packet writing operation asynchronously.
func (p *polling) write(packets []*packet.Packet) {
	data, _ := parser.Parserv4().EncodePayload(packets)
	p.doWrite(data, func() {
		p.SetWritable(true)
		p.Emit("drain")
	})
}

// uri constructs the HTTP URL for the polling transport with query parameters.
func (p *polling) uri() *url.URL {
	schema := "http"
	if p.Opts().Secure() {
		schema = "https"
	}
	query := url.Values{}
	for k, vs := range p.Query() {
		for _, v := range vs {
			query.Add(k, v)
		}
	}
	if p.Opts().TimestampRequests() {
		query.Set(p.Opts().TimestampParam(), request.RandomString())
	}
	if !p.SupportsBinary() && !query.Has("sid") {
		query.Set("b64", "1")
	}
	return p.CreateUri(schema, query)
}

// doPoll performs the HTTP GET request to poll for data from the server.
func (p *polling) doPoll() {
	res, err := p._fetch(nil)
	if err != nil {
		p.OnError("fetch read error", err, nil)
		return
	}
	defer res.Body.Close()
	if !res.Ok() {
		p.OnError("fetch read error", res.Err, res.Request.Context())
		return
	}
	data, err := types.NewStringBufferReader(res.Body)
	if err != nil {
		p.OnError("fetch read error", err, nil)
		return
	}
	p.OnData(data)
}

// doWrite performs the HTTP POST request to write data to the server.
// fn is called after a successful write.
func (p *polling) doWrite(data types.BufferInterface, fn func()) {
	res, err := p._fetch(data)
	if err != nil {
		p.OnError("fetch write error", err, nil)
		return
	}
	defer res.Body.Close()
	if !res.Ok() {
		p.OnError("fetch write error", res.Err, res.Request.Context())
		return
	}
	fn()
}

// _fetch performs the HTTP request with the given data (GET if data is nil, POST otherwise).
func (p *polling) _fetch(data io.Reader) (res *request.Response, err error) {
	headers := http.Header{}
	for k, vs := range p.Opts().ExtraHeaders() {
		for _, v := range vs {
			headers.Add(k, v)
		}
	}
	if data != nil {
		headers.Set("Content-Type", "text/plain;charset=UTF-8")
		res, err = p.client.Post(p.uri().String(), &request.Options{
			Body:    data,
			Headers: headers,
		})
	} else {
		res, err = p.client.Get(p.uri().String(), &request.Options{
			Headers: headers,
		})
	}
	return
}
