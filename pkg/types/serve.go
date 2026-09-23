package types

import (
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	ServeMux struct {
		DefaultHandler http.Handler // Default Handler

		mu       sync.RWMutex
		exact    map[string]muxEntry
		prefixes []muxEntry
		hosts    bool // whether any patterns contain hostnames
	}

	muxEntry struct {
		h       http.Handler
		pattern string
	}
)

// NewServeMux allocates and returns a new ServeMux.
func NewServeMux(defaultHandler http.Handler) *ServeMux {
	if defaultHandler == nil {
		defaultHandler = http.DefaultServeMux
	}
	return &ServeMux{DefaultHandler: defaultHandler}
}

// Exact matches precede prefixes. Among prefixes, the latest registration wins.
func (mux *ServeMux) match(path string) (h http.Handler, pattern string) {
	// Check for exact match first.
	v, ok := mux.exact[path]
	if ok {
		return v.h, v.pattern
	}

	for _, e := range slices.Backward(mux.prefixes) {
		if strings.HasPrefix(path, e.pattern) {
			return e.h, e.pattern
		}
	}
	return nil, ""
}

// Handler returns the handler to use for the given request,
// consulting r.Method, r.Host, and r.URL.Path. It cleans the path for matching
// without redirecting the request. Host ports are ignored except for CONNECT.
// It returns the matching pattern, or DefaultHandler and an empty pattern.
func (mux *ServeMux) Handler(r *http.Request) (h http.Handler, pattern string) {
	path := utils.CleanPath(r.URL.Path)
	// CONNECT preserves the host, including its port.
	if r.Method == http.MethodConnect {
		return mux.handler(r.Host, path)
	}

	// All other requests have any port stripped and path cleaned
	// before passing to mux.handler.
	host := utils.StripHostPort(r.Host)

	return mux.handler(host, path)
}

// handler is the main implementation of Handler.
// The path is known to be in canonical form.
func (mux *ServeMux) handler(host, path string) (h http.Handler, pattern string) {
	mux.mu.RLock()
	defer mux.mu.RUnlock()

	// Host-specific pattern takes precedence over generic ones
	if mux.hosts {
		h, pattern = mux.match(host + path)
	}
	if h == nil {
		h, pattern = mux.match(path)
	}
	if h == nil {
		h, pattern = mux.DefaultHandler, ""
	}
	return
}

// ServeHTTP dispatches the request to the matching handler.
func (mux *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "*" {
		if r.ProtoAtLeast(1, 1) {
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h, _ := mux.Handler(r)
	h.ServeHTTP(w, r)
}

// Handle registers the handler for the given pattern.
// Duplicate exact patterns panic. Prefix patterns ending in / may be
// registered again; the latest matching prefix takes precedence.
func (mux *ServeMux) Handle(pattern string, handler http.Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if pattern == "" {
		panic("http: invalid pattern")
	}
	if handler == nil {
		panic("http: nil handler")
	}
	if _, exist := mux.exact[pattern]; exist {
		panic("http: multiple registrations for " + pattern)
	}

	e := muxEntry{h: handler, pattern: pattern}
	if pattern[len(pattern)-1] == '/' {
		mux.prefixes = append(mux.prefixes, e)
	} else {
		if mux.exact == nil {
			mux.exact = make(map[string]muxEntry)
		}
		mux.exact[pattern] = e
	}

	if pattern[0] != '/' {
		mux.hosts = true
	}
}

// HandleFunc registers the handler function for the given pattern.
func (mux *ServeMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if handler == nil {
		panic("http: nil handler")
	}
	mux.Handle(pattern, http.HandlerFunc(handler))
}
