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

	// Optional protocol upgrades. Set by the caller when applicable.
	Websocket    *WebSocketConn
	WebTransport *WebTransportConn

	// Cleanup is invoked exactly once when the context is closed.
	Cleanup Callable

	ctx      context.Context
	request  *http.Request
	response http.ResponseWriter

	statusCode atomic.Int32

	// written indicates that the response body has been (or is being) written.
	written atomic.Bool
	done    chan struct{}

	closeOnce sync.Once
	writeOnce sync.Once

	// responseHeadersUsed tracks whether ResponseHeaders() was ever called.
	// When false, flushResponseHeaders can skip the redundant copy.
	responseHeadersUsed atomic.Bool

	// Lazily-computed, cached request/response metadata.
	onceHeaders         func() *ParameterBag
	onceQuery           func() *ParameterBag
	onceResponseHeaders func() *ParameterBag
	onceMethod          func() string
	onceHost            func() string
	oncePath            func() string
	onceUserAgent       func() string
}

// NewHttpContext creates a fully-initialized HttpContext. It panics if either
// w or r is nil since that indicates a programming error at the caller site.
//
// The caller MUST ensure the response is eventually written (Write) or
// finalized (Flush), or that r.Context() will be canceled. Otherwise the
// internal context-watcher goroutine will leak.
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

	c.initLazyAccessors()
	c.statusCode.Store(http.StatusOK)

	// Watch the request context exactly once; terminates when either the
	// request context is canceled or the response is finalized.
	go c.contextWatcher()

	return c
}

// initLazyAccessors sets up OnceValue-backed accessors for commonly used
// request/response metadata. Centralizing this keeps the constructor short
// and easy to extend.
func (c *HttpContext) initLazyAccessors() {
	r, w := c.request, c.response

	c.onceHeaders = sync.OnceValue(func() *ParameterBag {
		return NewParameterBag(r.Header)
	})
	c.onceQuery = sync.OnceValue(func() *ParameterBag {
		return NewParameterBag(r.URL.Query())
	})
	c.onceResponseHeaders = sync.OnceValue(func() *ParameterBag {
		return NewParameterBag(w.Header())
	})
	c.onceMethod = sync.OnceValue(func() string {
		return strings.ToUpper(r.Method)
	})
	c.onceHost = sync.OnceValue(func() string {
		host := strings.TrimSpace(r.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			return h
		}
		return host
	})
	c.oncePath = sync.OnceValue(func() string {
		p := strings.Trim(r.URL.Path, "/")
		if p == "" {
			return "/"
		}
		return p
	})
	c.onceUserAgent = sync.OnceValue(func() string {
		return r.Header.Get("User-Agent")
	})
}

// IsDone reports whether the response body has been written or the context
// has been closed.
func (c *HttpContext) IsDone() bool {
	return c.written.Load()
}

// Done returns a channel that is closed when the context is finalized.
func (c *HttpContext) Done() <-chan struct{} {
	return c.done
}

// Flush finalizes the context without writing a response body.
func (c *HttpContext) Flush() {
	c.closeWithError(nil)
}

// ---------------------------------------------------------------------------
// Status code
// ---------------------------------------------------------------------------

// SetStatusCode sets the HTTP status code to be used by the next Write.
// Returns ErrInvalidStatusCode for values outside [100, 599], or
// ErrResponseAlreadyWritten if a response has already been written.
//
// Note: there is an inherent race between the written-check and the store.
// If a concurrent goroutine calls Write between the two, the new status code
// will be stored but not used for the already-flushed response. This is
// acceptable because callers are expected to call SetStatusCode and Write
// sequentially from the same goroutine.
func (c *HttpContext) SetStatusCode(code int) error {
	if code < minStatusCode || code > maxStatusCode {
		return ErrInvalidStatusCode
	}
	if c.written.Load() {
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
func (c *HttpContext) Write(data []byte) (n int, err error) {
	if c.written.Load() {
		return 0, ErrResponseAlreadyWritten
	}

	executed := false
	c.writeOnce.Do(func() {
		executed = true
		c.written.Store(true)

		n, err = c.performWrite(data)
		c.closeWithError(nil)
	})

	if !executed {
		return 0, ErrResponseAlreadyWritten
	}
	return n, err
}

// performWrite flushes headers and body to the underlying ResponseWriter.
func (c *HttpContext) performWrite(data []byte) (int, error) {
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
	for key, values := range c.onceResponseHeaders().All() {
		switch len(values) {
		case 0:
			// Nothing to write; leave existing values untouched.
			continue
		case 1:
			dst.Set(key, values[0])
		default:
			dst.Del(key)
			for _, v := range values {
				dst.Add(key, v)
			}
		}
	}
}

func (c *HttpContext) Request() *http.Request        { return c.request }
func (c *HttpContext) Response() http.ResponseWriter { return c.response }
func (c *HttpContext) Context() context.Context      { return c.ctx }
func (c *HttpContext) Headers() *ParameterBag        { return c.onceHeaders() }
func (c *HttpContext) Query() *ParameterBag          { return c.onceQuery() }
func (c *HttpContext) ResponseHeaders() *ParameterBag {
	c.responseHeadersUsed.Store(true)
	return c.onceResponseHeaders()
}
func (c *HttpContext) Method() string    { return c.onceMethod() }
func (c *HttpContext) Host() string      { return c.onceHost() }
func (c *HttpContext) Path() string      { return c.oncePath() }
func (c *HttpContext) UserAgent() string { return c.onceUserAgent() }
func (c *HttpContext) PathInfo() string  { return c.request.URL.Path }
func (c *HttpContext) Secure() bool      { return c.request.TLS != nil }

// contextWatcher closes the context when the underlying request context is
// canceled. It exits once either signal is observed.
func (c *HttpContext) contextWatcher() {
	select {
	case <-c.ctx.Done():
		c.closeWithError(c.ctx.Err())
	case <-c.done:
	}
}

// closeWithError finalizes the context exactly once, running Cleanup and
// emitting a "close" event asynchronously so it never blocks the caller.
func (c *HttpContext) closeWithError(err error) {
	c.closeOnce.Do(func() {
		// Mark written to make IsDone reflect finalization as well,
		// even for paths that never wrote a response body.
		c.written.Store(true)

		close(c.done)
		if c.Cleanup != nil {
			c.Cleanup()
		}
		// Fire the event asynchronously so listeners can't deadlock us.
		// Clear the emitter afterwards to release listener references for GC.
		go func() {
			c.Emit("close", err)
			c.Clear()
		}()
	})
}
