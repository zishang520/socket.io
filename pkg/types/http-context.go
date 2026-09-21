package types

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Exported sentinel errors returned by HttpContext.
var (
	ErrResponseAlreadyWritten = errors.New("response has already been written")
	ErrInvalidStatusCode      = errors.New("invalid status code")
	ErrNilRequest             = errors.New("http.Request must not be nil")
	ErrNilResponseWriter      = errors.New("http.ResponseWriter must not be nil")
)

// HTTP status code boundaries as per RFC 9110.
const (
	minStatusCode = 100
	maxStatusCode = 599
)

const (
	httpResponseCommitted uint32 = 1 << iota
	httpContextClosed
)

// HttpContext wraps an http.Request / http.ResponseWriter pair with extra
// features: event emission, lazy-computed request metadata, one-shot writing
// semantics, and a done-channel tied to the request context.
//
// Instances must be created with NewHttpContext. A single HttpContext is not
// meant to be written to more than once; subsequent writes return
// ErrResponseAlreadyWritten.
type HttpContext struct {
	noCopy noCopy

	EventEmitter

	IdleTimeout time.Duration
	// Optional protocol upgrades. Set by the caller when applicable.
	Websocket    *WebSocketConn
	WebTransport *WebTransportConn

	// Cleanup is invoked exactly once when the context is closed.
	Cleanup Callable

	transportMu             sync.Mutex
	transportReadPermission <-chan bool
	transportReady          Callable

	ctx         context.Context
	request     *http.Request
	response    http.ResponseWriter
	stopContext func() bool

	statusCode atomic.Int32

	// Track response commitment separately from cancellation and finalization.
	state atomic.Uint32
	done  chan struct{}

	// responseHeadersUsed tracks whether ResponseHeaders() was ever called.
	// When false, flushResponseHeaders can skip the redundant copy.
	responseHeadersUsed atomic.Bool

	// Keep each accessor's lazy state in the context instead of a heap closure.
	headers, query, responseHeaders httpContextValue[*ParameterBag]
	method, host, path, userAgent   httpContextValue[string]
}

// httpContextValue preserves OnceValue's initialization and panic semantics
// without retaining an initializer function for every context accessor.
type httpContextValue[T any] struct {
	once       sync.Once
	valid      bool
	value      T
	panicValue any
}

func (v *httpContextValue[T]) get(load func() T) T {
	v.once.Do(func() {
		defer func() {
			if !v.valid {
				v.panicValue = recover()
				panic(v.panicValue)
			}
		}()
		v.value = load()
		v.valid = true
	})
	if !v.valid {
		panic(v.panicValue)
	}
	return v.value
}

// NewHttpContext creates a fully-initialized HttpContext. It panics if either
// w or r is nil since that indicates a programming error at the caller site.
//
// Write, Flush, or request cancellation releases the cancellation registration.
func NewHttpContext(w http.ResponseWriter, r *http.Request) *HttpContext {
	if r == nil {
		panic(ErrNilRequest)
	}
	if w == nil {
		panic(ErrNilResponseWriter)
	}

	c := &HttpContext{
		EventEmitter: NewEventEmitter(),
		request:      r,
		response:     w,
		ctx:          r.Context(),
		done:         make(chan struct{}),
	}

	c.statusCode.Store(http.StatusOK)

	if c.ctx.Done() != nil {
		// Cancellation may run before stopContext is assigned, so it bypasses Flush.
		c.stopContext = context.AfterFunc(c.ctx, func() {
			c.closeWithError(c.ctx.Err())
		})
	}

	return c
}

// IsDone reports whether the response has been claimed for writing or finalized.
func (c *HttpContext) IsDone() bool {
	return c.state.Load() != 0
}

// Done closes on finalization, before Cleanup and close listeners finish.
func (c *HttpContext) Done() <-chan struct{} {
	return c.done
}

// ResponseCommitted reports whether Write claimed the response or Flush
// finalized it normally. It does not imply that the body write has finished
// or succeeded. Cancellation before either operation leaves it false.
func (c *HttpContext) ResponseCommitted() bool {
	return c.state.Load()&httpResponseCommitted != 0
}

// Flush finalizes the context without writing a response body.
// Repeated calls do not wait for an ongoing Cleanup callback.
func (c *HttpContext) Flush() {
	if c.stopContext != nil {
		c.stopContext()
	}
	c.closeWithError(nil)
}

// SetStatusCode sets the HTTP status code to be used by the next Write.
// Returns ErrInvalidStatusCode for values outside [100, 599], or
// ErrResponseAlreadyWritten if a response has already been written.
//
// Set the status before calling Write; concurrent status changes need not
// affect a response whose write has already started.
func (c *HttpContext) SetStatusCode(code int) error {
	if code < minStatusCode || code > maxStatusCode {
		return ErrInvalidStatusCode
	}
	if c.IsDone() {
		return ErrResponseAlreadyWritten
	}
	c.statusCode.Store(int32(code))
	return nil
}

// GetStatusCode returns the currently configured status code (default 200).
func (c *HttpContext) GetStatusCode() int {
	return int(c.statusCode.Load())
}

// Write commits the response. It may be called at most once; subsequent
// invocations return ErrResponseAlreadyWritten.
func (c *HttpContext) Write(data []byte) (int, error) {
	if !c.state.CompareAndSwap(0, httpResponseCommitted) {
		return 0, ErrResponseAlreadyWritten
	}
	defer c.Flush()
	c.flushResponseHeaders()
	c.response.WriteHeader(c.GetStatusCode())
	return c.response.Write(data)
}

// flushResponseHeaders copies any headers staged in the ParameterBag into the
// underlying http.Header. It is a no-op when the bag was never materialized.
func (c *HttpContext) flushResponseHeaders() {
	if !c.responseHeadersUsed.Load() {
		return
	}
	dst := c.response.Header()
	for key, values := range c.ResponseHeaders().All() {
		if len(values) > 0 {
			// All returns detached slices, which the response can use directly.
			dst[http.CanonicalHeaderKey(key)] = values
		}
	}
}

func (c *HttpContext) Request() *http.Request        { return c.request }
func (c *HttpContext) Response() http.ResponseWriter { return c.response }
func (c *HttpContext) Context() context.Context      { return c.ctx }
func (c *HttpContext) Headers() *ParameterBag {
	return c.headers.get(func() *ParameterBag { return NewParameterBag(c.request.Header) })
}
func (c *HttpContext) Query() *ParameterBag {
	return c.query.get(func() *ParameterBag { return NewParameterBag(c.request.URL.Query()) })
}
func (c *HttpContext) ResponseHeaders() *ParameterBag {
	c.responseHeadersUsed.Store(true)
	return c.responseHeaders.get(func() *ParameterBag { return NewParameterBag(c.response.Header()) })
}
func (c *HttpContext) Method() string {
	return c.method.get(func() string { return strings.ToUpper(c.request.Method) })
}
func (c *HttpContext) Host() string {
	return c.host.get(func() string {
		host := strings.TrimSpace(c.request.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			return h
		}
		return host
	})
}
func (c *HttpContext) Path() string {
	return c.path.get(func() string {
		path := strings.Trim(c.request.URL.Path, "/")
		if path == "" {
			return "/"
		}
		return path
	})
}
func (c *HttpContext) UserAgent() string {
	return c.userAgent.get(func() string { return c.request.Header.Get("User-Agent") })
}
func (c *HttpContext) PathInfo() string { return c.request.URL.Path }
func (c *HttpContext) Secure() bool     { return c.request.TLS != nil }

// SetTransportReadPermission publishes the single-reader initialization result.
// Set it before constructing the transport; do not replace it while in use.
func (c *HttpContext) SetTransportReadPermission(permission <-chan bool) {
	c.transportMu.Lock()
	c.transportReadPermission = permission
	c.transportMu.Unlock()
}

// TransportReadPermission returns the initialization result channel.
// Only true permits reading; nil preserves standalone automatic startup.
func (c *HttpContext) TransportReadPermission() <-chan bool {
	c.transportMu.Lock()
	permission := c.transportReadPermission
	c.transportMu.Unlock()
	return permission
}

// SetTransportReady registers the callback before initialization completes.
// Passing nil discards a pending callback without invoking it.
func (c *HttpContext) SetTransportReady(ready Callable) {
	c.transportMu.Lock()
	c.transportReady = ready
	c.transportMu.Unlock()
}

// TakeTransportReady removes and returns the callback for the caller to invoke
// after successful initialization. The callback runs outside the context lock.
func (c *HttpContext) TakeTransportReady() Callable {
	c.transportMu.Lock()
	ready := c.transportReady
	c.transportReady = nil
	c.transportMu.Unlock()
	return ready
}

// closeWithError claims finalization before invoking Cleanup, allowing reentry.
// Done closes before Cleanup; close listeners run asynchronously afterwards.
func (c *HttpContext) closeWithError(err error) {
	previous := c.state.Or(httpContextClosed)
	if previous&httpContextClosed != 0 {
		// Wait for finalization to be published, not for Cleanup to finish.
		<-c.done
		return
	}
	if err == nil && previous&httpResponseCommitted == 0 {
		c.state.Or(httpResponseCommitted)
	}
	close(c.done)
	if c.Cleanup != nil {
		c.Cleanup()
	}
	go func() {
		c.Emit("close", err)
		c.Clear()
	}()
}
