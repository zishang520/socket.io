package request

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	"resty.dev/v3"
)

type HTTPClient struct {
	client  *resty.Client
	options *clientOptions
	isDone  atomic.Bool
}

func NewHTTPClient(options ...ClientOption) *HTTPClient {
	opts := applyOptions(options...)

	// Create resty client
	client := resty.New()

	// Add decompresser into Resty
	client.AddContentDecompresser("br", decompressBrotli)
	client.AddContentDecompresser("zstd", decompressZstd)

	// Set basic configurations
	client.SetTimeout(opts.Timeout)
	client.SetRedirectPolicy(resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
		if !opts.FollowRedirects {
			return http.ErrUseLastResponse
		}
		if len(via) >= opts.MaxRedirects {
			return fmt.Errorf("maximum number of redirects (%d) followed", opts.MaxRedirects)
		}
		return nil
	}))

	if opts.Logger != nil {
		client.SetLogger(opts.Logger)
	}

	if opts.BaseURL != "" {
		client.SetBaseURL(opts.BaseURL)
	}

	if opts.Transport != nil {
		client.SetTransport(opts.Transport)
	}

	// Set SSL/TLS configuration
	if opts.TLSClientConfig != nil {
		client.SetTLSClientConfig(opts.TLSClientConfig)
	}

	// Set proxy
	if opts.Proxy != "" {
		client.SetProxy(opts.Proxy)
	}

	// Set cookie jar
	if opts.Jar != nil {
		client.SetCookieJar(opts.Jar)
	}

	httpClient := &HTTPClient{
		client:  client,
		options: opts,
	}

	return httpClient
}

func (c *HTTPClient) Request(ctx context.Context, method, url string, options *Options) (*Response, error) {
	// Create resty request
	req := c.client.R().
		SetContext(ctx)

	// Set request body
	if err := c.setRequestBody(req, options); err != nil {
		return nil, err
	}

	c.setQuery(req, options)

	// Set request headers
	c.setRequestHeaders(req, options)

	// Set authentication information
	c.setAuthentication(req, options)

	c.setCookies(req, options)

	// Send request
	resp, err := req.Execute(method, url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	return &Response{resp}, nil
}

// Get sends a GET request
func (c *HTTPClient) Get(url string, options *Options) (*Response, error) {
	return c.Request(context.Background(), http.MethodGet, url, options)
}

// Post sends a POST request
func (c *HTTPClient) Post(url string, options *Options) (*Response, error) {
	return c.Request(context.Background(), http.MethodPost, url, options)
}

// Put sends a PUT request
func (c *HTTPClient) Put(url string, options *Options) (*Response, error) {
	return c.Request(context.Background(), http.MethodPut, url, options)
}

// Delete sends a DELETE request
func (c *HTTPClient) Delete(url string, options *Options) (*Response, error) {
	return c.Request(context.Background(), http.MethodDelete, url, options)
}

// Patch sends a PATCH request
func (c *HTTPClient) Patch(url string, options *Options) (*Response, error) {
	return c.Request(context.Background(), http.MethodPatch, url, options)
}

// Head sends a HEAD request
func (c *HTTPClient) Head(url string, options *Options) (*Response, error) {
	return c.Request(context.Background(), http.MethodHead, url, options)
}

// Options sends an OPTIONS request
func (c *HTTPClient) Options(url string, options *Options) (*Response, error) {
	return c.Request(context.Background(), http.MethodOptions, url, options)
}

func (c *HTTPClient) Close() error {
	if c.isDone.CompareAndSwap(false, true) {
		if transport, ok := c.client.Transport().(io.Closer); ok {
			defer transport.Close()
		}
		return c.client.Close()
	}
	return nil
}

func (c *HTTPClient) setRequestBody(req *resty.Request, options *Options) error {
	switch {
	case options.JSON != nil:
		req.SetBody(options.JSON)
	case options.Form != nil:
		req.SetFormData(options.Form)
	case options.Multipart != nil:
		for k, v := range options.Multipart {
			req.SetMultipartField(k, v.FileName, v.ContentType, v.Reader)
		}
	case options.Body != nil:
		switch v := options.Body.(type) {
		case string, []byte, io.Reader:
			req.SetBody(v)
		default:
			return fmt.Errorf("unsupported body type: %T", options.Body)
		}
	}
	return nil
}

func (c *HTTPClient) setRequestHeaders(req *resty.Request, options *Options) {
	// Set default User-Agent
	// Set default headers first
	req.SetHeaders(map[string]string{
		"User-Agent": "engine.io-go/1.0",
		"Accept":     "*/*",
	})

	// Then set custom headers, allowing override of defaults
	if len(options.Headers) > 0 {
		req.SetHeaderMultiValues(options.Headers)
	}
}

func (c *HTTPClient) setQuery(req *resty.Request, options *Options) {
	if len(options.Query) > 0 {
		req.SetQueryParamsFromValues(options.Query)
	}
}

func (c *HTTPClient) setCookies(req *resty.Request, options *Options) {
	if len(options.Cookies) > 0 {
		req.SetCookies(options.Cookies)
	}
}

func (c *HTTPClient) setAuthentication(req *resty.Request, options *Options) {
	if options.BasicAuth != nil && options.BasicAuth.Username != "" {
		req.SetBasicAuth(options.BasicAuth.Username, options.BasicAuth.Password)
	}

	if options.BearerToken != "" {
		req.SetAuthToken(options.BearerToken)
	}
}
