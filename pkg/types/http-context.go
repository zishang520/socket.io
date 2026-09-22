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
	httpContextFinalizing
)

// HttpContext wraps an http.Request / http.ResponseWriter pair with lazy request
// metadata, one-shot writing, and response completion notifications.
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

	request  *http.Request
	response http.ResponseWriter
	ctx      context.Context

	// Guard response lifecycle and transport initialization; never hold across I/O or callbacks.
	mu         sync.Mutex
	statusCode atomic.Int32
	// Track response commitment separately from cancellation and finalization.
	state         atomic.Uint32
	responseUsers int
	closeErr      error
	done          chan struct{}
	stopContext   func() bool

	// Keep each accessor's lazy state in the context instead of a heap closure.
	headers, query, responseHeaders httpContextValue[*ParameterBag]
	method, host, path, userAgent   httpContextValue[string]
	// Skip copying response headers if ResponseHeaders was never called.
	responseHeadersUsed atomic.Bool

	// Transport initialization is independent of HTTP response completion.
	transportReadPermission <-chan bool
	transportReady          Callable
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
			c.requestClose(c.ctx.Err())
		})
	}

	return c
}

func (c *HttpContext) Request() *http.Request { return c.request }

func (c *HttpContext) Context() context.Context { return c.ctx }

func (c *HttpContext) Headers() *ParameterBag {
	return c.headers.get(func() *ParameterBag { return NewParameterBag(c.request.Header) })
}

func (c *HttpContext) Query() *ParameterBag {
	return c.query.get(func() *ParameterBag { return NewParameterBag(c.request.URL.Query()) })
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

func (c *HttpContext) Secure() bool { return c.request.TLS != nil }

func (c *HttpContext) Response() http.ResponseWriter { return c.response }

func (c *HttpContext) ResponseHeaders() *ParameterBag {
	c.responseHeadersUsed.Store(true)
	return c.responseHeaders.get(func() *ParameterBag { return NewParameterBag(c.response.Header()) })
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

// IsDone reports whether Write has claimed the response or finalization was requested.
func (c *HttpContext) IsDone() bool {
	return c.state.Load() != 0
}

// Done closes after finalization is requested and all response operations finish,
// before Cleanup and close listeners run. Use Context().Done() for cancellation
// while preparing or writing a response.
func (c *HttpContext) Done() <-chan struct{} {
	return c.done
}

// ResponseCommitted reports whether Write claimed the response or Flush
// finalized it normally. It does not imply that the body write has finished
// or succeeded. Cancellation before either operation leaves it false.
func (c *HttpContext) ResponseCommitted() bool {
	return c.state.Load()&httpResponseCommitted != 0
}

// BeginResponse keeps ResponseWriter valid while preparing an asynchronous
// response, without marking it committed. It returns false if the response was
// already committed or finalized. Each successful call needs one EndResponse.
func (c *HttpContext) BeginResponse() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.IsDone() {
		return false
	}
	c.responseUsers++
	return true
}

// EndResponse releases a successful BeginResponse. Call it after the last access
// to ResponseWriter, including any asynchronous response preparation or writing.
func (c *HttpContext) EndResponse() {
	c.mu.Lock()
	c.responseUsers--
	finished := c.completeResponseLocked()
	c.mu.Unlock()
	if finished {
		c.finishClose()
	}
}

// Write commits the response. It may be called at most once; subsequent
// invocations return ErrResponseAlreadyWritten.
func (c *HttpContext) Write(data []byte) (int, error) {
	c.mu.Lock()
	if c.IsDone() {
		c.mu.Unlock()
		return 0, ErrResponseAlreadyWritten
	}
	c.state.Store(httpResponseCommitted)
	c.responseUsers++
	c.mu.Unlock()
	defer c.finishWrite()
	c.flushResponseHeaders()
	c.response.WriteHeader(c.GetStatusCode())
	return c.response.Write(data)
}

// Flush requests finalization without writing a response body. An active Write
// or BeginResponse scope delays Done; Flush does not wait for those operations.
// Repeated calls do not wait for an ongoing Cleanup callback.
func (c *HttpContext) Flush() {
	if c.stopContext != nil {
		c.stopContext()
	}
	c.requestClose(nil)
}

// finishWrite ends the write and requests normal finalization in one critical
// section. An outer preparation scope may still own the response.
func (c *HttpContext) finishWrite() {
	if c.stopContext != nil {
		c.stopContext()
	}
	c.mu.Lock()
	c.state.Store(c.state.Load() | httpContextFinalizing)
	c.responseUsers--
	finished := c.completeResponseLocked()
	c.mu.Unlock()
	if finished {
		c.finishClose()
	}
}

// requestClose records the first finalization request. A response already
// committed by Write keeps ownership when its request context is canceled.
func (c *HttpContext) requestClose(err error) {
	c.mu.Lock()
	state := c.state.Load()
	if state&httpContextFinalizing != 0 || (err != nil && state&httpResponseCommitted != 0) {
		c.mu.Unlock()
		return
	}
	c.closeErr = err
	c.state.Store(state | httpContextFinalizing)
	finished := c.completeResponseLocked()
	c.mu.Unlock()
	if finished {
		c.finishClose()
	}
}

// completeResponseLocked publishes completion after the last response operation.
// Call only while holding mu, after requesting finalization or releasing an operation.
func (c *HttpContext) completeResponseLocked() bool {
	state := c.state.Load()
	if c.responseUsers != 0 || state&httpContextFinalizing == 0 {
		return false
	}
	if c.closeErr == nil {
		c.state.Store(state | httpResponseCommitted)
	}
	close(c.done)
	return true
}

// Done is already closed and closeErr is immutable. No lock is held across callbacks.
func (c *HttpContext) finishClose() {
	if c.Cleanup != nil {
		c.Cleanup()
	}
	go func() {
		c.Emit("close", c.closeErr)
		c.Clear()
	}()
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

// SetTransportReadPermission publishes the single-reader initialization result.
// Set it before constructing the transport; do not replace it while in use.
func (c *HttpContext) SetTransportReadPermission(permission <-chan bool) {
	c.mu.Lock()
	c.transportReadPermission = permission
	c.mu.Unlock()
}

// TransportReadPermission returns the initialization result channel.
// Only true permits reading; nil preserves standalone automatic startup.
func (c *HttpContext) TransportReadPermission() <-chan bool {
	c.mu.Lock()
	permission := c.transportReadPermission
	c.mu.Unlock()
	return permission
}

// SetTransportReady registers the callback before initialization completes.
// Passing nil discards a pending callback without invoking it.
func (c *HttpContext) SetTransportReady(ready Callable) {
	c.mu.Lock()
	c.transportReady = ready
	c.mu.Unlock()
}

// TakeTransportReady removes and returns the callback for the caller to invoke
// after successful initialization. The callback runs outside the context lock.
func (c *HttpContext) TakeTransportReady() Callable {
	c.mu.Lock()
	ready := c.transportReady
	c.transportReady = nil
	c.mu.Unlock()
	return ready
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
