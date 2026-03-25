package types

import (
	"net/http"
	// "sort"
	"strings"
	"sync"

	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	ServeMux struct {
		DefaultHandler http.Handler // Default Handler

		mu    sync.RWMutex
		m     map[string]muxEntry
		es    []muxEntry // slice of entries sorted from longest to shortest.
		hosts bool       // whether any patterns contain hostnames
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

// Find a handler on a handler map given a path string.
// Most-specific (longest) pattern wins.
func (mux *ServeMux) match(path string) (h http.Handler, pattern string) {
	// Check for exact match first.
	v, ok := mux.m[path]
	if ok {
		return v.h, v.pattern
	}

	// Check for longest valid match.  mux.es contains all patterns
	// that end in / sorted from longest to shortest.
	for _, e := range mux.es {
		if strings.HasPrefix(path, e.pattern) {
			return e.h, e.pattern
		}
	}
	return nil, ""
}

// Handler returns the handler to use for the given request,
// consulting r.Method, r.Host, and r.URL.Path. It always returns
// a non-nil handler. If the path is not in its canonical form, the
// handler will be an internally-generated handler that redirects
// to the canonical path. If the host contains a port, it is ignored
// when matching handlers.
//
// The path and host are used unchanged for CONNECT requests.
//
// Handler also returns the registered pattern that matches the
// request or, in the case of internally-generated redirects,
// the pattern that will match after following the redirect.
//
// If there is no registered handler that applies to the request,
// Handler returns a “page not found” handler and an empty pattern.
func (mux *ServeMux) Handler(r *http.Request) (h http.Handler, pattern string) {
	path := utils.CleanPath(r.URL.Path)
	// CONNECT requests are not canonicalized.
	if r.Method == http.MethodConnect {
		return mux.handler(r.Host, path)
	}

	// All other requests have any port stripped and path cleaned
	// before passing to mux.handler.
	host := utils.StripHostPort(r.Host)

	return mux.handler(host, path)
}

// handler is the main implementation of Handler.
// The path is known to be in canonical form, except for CONNECT methods.
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

// ServeHTTP dispatches the request to the handler whose
// pattern most closely matches the request URL.
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
// If a handler already exists for pattern, Handle panics.
func (mux *ServeMux) Handle(pattern string, handler http.Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if pattern == "" {
		panic("http: invalid pattern")
	}
	if handler == nil {
		panic("http: nil handler")
	}
	if _, exist := mux.m[pattern]; exist {
		panic("http: multiple registrations for " + pattern)
	}

	if mux.m == nil {
		mux.m = make(map[string]muxEntry)
	}
	e := muxEntry{h: handler, pattern: pattern}
	if pattern[len(pattern)-1] == '/' {
		mux.es = appendSorted(mux.es, e)
	} else {
		mux.m[pattern] = e
	}

	if pattern[0] != '/' {
		mux.hosts = true
	}
}

func appendSorted(es []muxEntry, e muxEntry) []muxEntry {
	// n := len(es)
	// i := sort.Search(n, func(i int) bool {
	// 	return len(es[i].pattern) < len(e.pattern)
	// })
	// if i == n {
	// 	return append(es, e)
	// }
	i := 0
	// we now know that i points at where we want to insert
	es = append(es, muxEntry{}) // try to grow the slice in place, any entry works.
	copy(es[i+1:], es[i:])      // Move shorter entries down
	es[i] = e
	return es
}

// HandleFunc registers the handler function for the given pattern.
func (mux *ServeMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if handler == nil {
		panic("http: nil handler")
	}
	mux.Handle(pattern, http.HandlerFunc(handler))
}
