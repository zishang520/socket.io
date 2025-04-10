package request

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"time"

	"resty.dev/v3"
)

// Option defines the configuration function type for request options
type ClientOption func(*clientOptions)

// Options contains all available request options
type clientOptions struct {
	// Basic options
	Logger          resty.Logger
	Timeout         time.Duration
	FollowRedirects bool
	MaxRedirects    int
	Proxy           string
	TLSClientConfig *tls.Config
	Transport       http.RoundTripper

	BaseURL string
	// Cookie Jar
	Jar http.CookieJar
}

func WithTransport(transport http.RoundTripper) ClientOption {
	return func(o *clientOptions) {
		o.Transport = transport
	}
}

func WithFollowRedirects(followRedirects bool, maxRedirects int) ClientOption {
	return func(o *clientOptions) {
		o.FollowRedirects = followRedirects
		o.MaxRedirects = maxRedirects
	}
}

func WithLogger(logger resty.Logger) ClientOption {
	return func(o *clientOptions) {
		o.Logger = logger
	}
}

func WithBaseURL(baseURL string) ClientOption {
	return func(o *clientOptions) {
		o.BaseURL = baseURL
	}
}

// WithTimeout sets the timeout duration
func WithTimeout(timeout time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.Timeout = timeout
	}
}

// WithCookieJar sets cookies
func WithCookieJar(jar http.CookieJar) ClientOption {
	return func(o *clientOptions) {
		o.Jar = jar
	}
}

// WithTLSClientConfig sets SSL/TLS options
func WithTLSClientConfig(config *tls.Config) ClientOption {
	return func(o *clientOptions) {
		o.TLSClientConfig = config
	}
}

// WithProxy sets proxy
func WithProxy(proxy string) ClientOption {
	return func(o *clientOptions) {
		o.Proxy = proxy
	}
}

// applyOptions applies request options
func applyOptions(opts ...ClientOption) *clientOptions {
	options := &clientOptions{}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

type BasicAuth struct {
	Username string
	Password string
}

type Multipart struct {
	FileName    string // Optional: not required if not a file
	ContentType string // Optional: not required if not a file
	io.Reader
}

type Options struct {
	// Header options
	Headers http.Header

	// Authentication options
	BasicAuth   *BasicAuth
	BearerToken string

	Cookies []*http.Cookie

	// Query parameters
	Query url.Values

	// Request body options
	Body      any
	JSON      any
	Form      map[string]string
	Multipart map[string]*Multipart
}
