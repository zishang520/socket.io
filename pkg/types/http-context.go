package types

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	portRegexp = regexp.MustCompile(`:\d+$`)
	hostRegexp = regexp.MustCompile(`(?:^\[)?[a-zA-Z0-9-:\]_]+\.?`)
)

type HttpContext struct {
	EventEmitter

	Websocket    *WebSocketConn
	WebTransport *WebTransportConn

	Cleanup Callable

	request  *http.Request
	response http.ResponseWriter

	headers *utils.ParameterBag
	query   *utils.ParameterBag

	method      string
	pathInfo    string
	isHostValid bool

	ctx context.Context

	isDone atomic.Bool
	done   chan Void

	statusCode      atomic.Value
	ResponseHeaders *utils.ParameterBag

	mu sync.Mutex
}

func NewHttpContext(w http.ResponseWriter, r *http.Request) *HttpContext {
	c := &HttpContext{
		EventEmitter:    NewEventEmitter(),
		ctx:             r.Context(),
		done:            make(chan Void),
		request:         r,
		response:        w,
		headers:         utils.NewParameterBag(r.Header),
		query:           utils.NewParameterBag(r.URL.Query()),
		isHostValid:     true,
		ResponseHeaders: utils.NewParameterBag(nil),
	}
	c.ResponseHeaders.With(w.Header())

	go func() {
		select {
		case <-c.ctx.Done():
			c.Flush()
			c.Emit("close")
		case <-c.done:
			c.Emit("close")
		}
	}()

	return c
}

func (c *HttpContext) Flush() {
	if c.isDone.CompareAndSwap(false, true) {
		close(c.done)
	}
}

func (c *HttpContext) Done() <-chan Void {
	return c.done
}

func (c *HttpContext) IsDone() bool {
	return c.isDone.Load()
}

func (c *HttpContext) SetStatusCode(statusCode int) {
	c.statusCode.Store(statusCode)
}

func (c *HttpContext) GetStatusCode() int {
	if v, ok := c.statusCode.Load().(int); ok {
		return v
	}
	return http.StatusOK
}

// Data synchronous writing
func (c *HttpContext) Write(wb []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.IsDone() {
		return 0, errors.New("you cannot write data repeatedly")
	}
	defer c.Flush()

	for k, v := range c.ResponseHeaders.All() {
		c.response.Header().Set(k, v[0])
	}
	c.response.WriteHeader(c.GetStatusCode())

	return c.response.Write(wb)
}

func (c *HttpContext) Request() *http.Request {
	return c.request
}

func (c *HttpContext) Response() http.ResponseWriter {
	return c.response
}

func (c *HttpContext) Headers() *utils.ParameterBag {
	return c.headers
}

func (c *HttpContext) Query() *utils.ParameterBag {
	return c.query
}

func (c *HttpContext) Context() context.Context {
	return c.ctx
}

func (c *HttpContext) GetPathInfo() string {
	if c.pathInfo == "" {
		c.pathInfo = c.request.URL.Path
	}
	return c.pathInfo
}

func (c *HttpContext) Get(key string, _default ...string) string {
	v, _ := c.query.Get(key, _default...)
	return v
}

func (c *HttpContext) Gets(key string, _default ...[]string) []string {
	v, _ := c.query.Gets(key, _default...)
	return v
}

func (c *HttpContext) Method() string {
	return c.GetMethod()
}

func (c *HttpContext) GetMethod() string {
	if c.method == "" {
		c.method = strings.ToUpper(c.request.Method)
	}
	return c.method
}

func (c *HttpContext) Path() string {
	if pattern := strings.Trim(c.GetPathInfo(), "/"); pattern != "" {
		return pattern
	}
	return "/"
}

func (c *HttpContext) GetHost() (string, error) {
	host := strings.TrimSpace(c.request.Host)
	host = portRegexp.ReplaceAllString(host, "")

	if host != "" {
		if host = hostRegexp.ReplaceAllString(host, ""); host != "" {
			if !c.isHostValid {
				return "", nil
			}
			c.isHostValid = false
			return "", errors.New(fmt.Sprintf(`Invalid host "%s".`, host)).Err()
		}
	}
	return host, nil
}

func (c *HttpContext) UserAgent() string {
	return c.headers.Peek("User-Agent")
}

func (c *HttpContext) Secure() bool {
	return c.request.TLS != nil
}
