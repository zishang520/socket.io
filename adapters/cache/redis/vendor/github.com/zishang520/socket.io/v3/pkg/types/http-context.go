package types

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	ErrResponseAlreadyWritten = errors.New("response has already been written")
	ErrInvalidStatusCode      = errors.New("invalid status code")
)

type HttpContext struct {
	EventEmitter

	Websocket    *WebSocketConn
	WebTransport *WebTransportConn

	Cleanup Callable

	ctx context.Context

	request  *http.Request
	response http.ResponseWriter

	statusCode atomic.Int32

	state atomic.Bool
	done  chan struct{}

	closeOnce sync.Once
	writeOnce sync.Once

	headers         func() *ParameterBag
	query           func() *ParameterBag
	responseHeaders func() *ParameterBag
	method          func() string
	host            func() string
	path            func() string
	userAgent       func() string
}

func NewHttpContext(w http.ResponseWriter, r *http.Request) *HttpContext {
	c := &HttpContext{
		EventEmitter: NewEventEmitter(),

		request:  r,
		response: w,
		ctx:      r.Context(),
		done:     make(chan struct{}),

		headers: sync.OnceValue(func() *ParameterBag {
			return NewParameterBag(r.Header)
		}),
		query: sync.OnceValue(func() *ParameterBag {
			return NewParameterBag(r.URL.Query())
		}),
		responseHeaders: sync.OnceValue(func() *ParameterBag {
			return NewParameterBag(w.Header())
		}),
		method: sync.OnceValue(func() string {
			return strings.ToUpper(r.Method)
		}),
		host: sync.OnceValue(func() string {
			host := strings.TrimSpace(r.Host)
			if h, _, err := net.SplitHostPort(host); err == nil {
				return h
			}
			return host
		}),
		path: sync.OnceValue(func() string {
			path := strings.Trim(r.URL.Path, "/")
			if path == "" {
				return "/"
			}
			return path
		}),
		userAgent: sync.OnceValue(func() string {
			return r.Header.Get("User-Agent")
		}),
	}

	c.statusCode.Store(http.StatusOK)

	c.state.Store(false)

	// This goroutine is invoked only once.
	go c.contextWatcher()

	return c
}

func (c *HttpContext) IsDone() bool {
	return c.state.Load()
}

func (c *HttpContext) Done() <-chan struct{} {
	return c.done
}

func (c *HttpContext) SetStatusCode(code int) error {
	if code < 100 || code > 599 {
		return ErrInvalidStatusCode
	}
	if c.IsDone() {
		return ErrResponseAlreadyWritten
	}
	c.statusCode.Store(int32(code))
	return nil
}

func (c *HttpContext) GetStatusCode() int {
	return int(c.statusCode.Load())
}

func (c *HttpContext) Write(data []byte) (int, error) {
	if c.IsDone() {
		return 0, ErrResponseAlreadyWritten
	}

	var writeResult struct {
		n   int
		err error
	}

	c.writeOnce.Do(func() {
		if !c.state.CompareAndSwap(false, true) {
			writeResult.err = ErrResponseAlreadyWritten
			return
		}

		writeResult.n, writeResult.err = c.performWrite(data)

		c.closeWithError(nil)
	})

	return writeResult.n, writeResult.err
}

func (c *HttpContext) Flush() {
	c.closeWithError(nil)
}

func (c *HttpContext) Request() *http.Request {
	return c.request
}

func (c *HttpContext) Response() http.ResponseWriter {
	return c.response
}

func (c *HttpContext) Headers() *ParameterBag {
	return c.headers()
}

func (c *HttpContext) Query() *ParameterBag {
	return c.query()
}

func (c *HttpContext) ResponseHeaders() *ParameterBag {
	return c.responseHeaders()
}

func (c *HttpContext) Context() context.Context {
	return c.ctx
}

func (c *HttpContext) PathInfo() string {
	return c.request.URL.Path
}

func (c *HttpContext) Path() string {
	return c.path()
}

func (c *HttpContext) Method() string {
	return c.method()
}

func (c *HttpContext) Host() string {
	return c.host()
}

func (c *HttpContext) UserAgent() string {
	return c.userAgent()
}

func (c *HttpContext) Secure() bool {
	return c.request.TLS != nil
}

func (c *HttpContext) contextWatcher() {
	select {
	case <-c.ctx.Done():
		c.closeWithError(c.ctx.Err())
	case <-c.done:
	}
}

func (c *HttpContext) performWrite(data []byte) (int, error) {
	c.writeResponseHeaders()
	c.response.WriteHeader(c.GetStatusCode())
	return c.response.Write(data)
}

func (c *HttpContext) writeResponseHeaders() {
	headers := c.response.Header()
	for key, values := range c.ResponseHeaders().All() {
		switch len(values) {
		case 0:
			continue
		case 1:
			headers.Set(key, values[0])
		default:
			headers.Del(key)
			for _, v := range values {
				headers.Add(key, v)
			}
		}
	}
}

func (c *HttpContext) closeWithError(err error) {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.Cleanup != nil {
			c.Cleanup()
		}
		// This goroutine is invoked only once.
		go c.Emit("close", err)
	})
}
