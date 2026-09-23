package request

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/utils"

	"resty.dev/v3"
)

type HTTPClient struct {
	client *resty.Client
	isDone atomic.Bool
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

	return &HTTPClient{client: client}
}

func (c *HTTPClient) Request(ctx context.Context, method, url string, options *Options) (*Response, error) {
	// Create resty request
	req := c.client.R().
		SetContext(ctx)

	// Set request body
	if err := c.setRequestBody(req, options); err != nil {
		return nil, err
	}

	if len(options.Query) > 0 {
		req.SetQueryParamsFromValues(options.Query)
	}

	// Set request headers
	if err := c.setRequestHeaders(req, options); err != nil {
		return nil, err
	}

	// Set authentication information
	if options.BasicAuth != nil && options.BasicAuth.Username != "" {
		req.SetBasicAuth(options.BasicAuth.Username, options.BasicAuth.Password)
	}
	if options.BearerToken != "" {
		req.SetAuthToken(options.BearerToken)
	}
	if len(options.Cookies) > 0 {
		req.SetCookies(options.Cookies)
	}

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

func (c *HTTPClient) Close() (err error) {
	if !c.isDone.CompareAndSwap(false, true) {
		return nil
	}
	// Close idle HTTP connections to prevent goroutine leaks.
	if httpClient := c.client.Client(); httpClient != nil {
		if transport, ok := httpClient.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	if transport, ok := c.client.Transport().(io.Closer); ok {
		defer func() {
			closeErr := transport.Close()
			if err == nil {
				err = closeErr
			} else if closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()
	}
	return c.client.Close()
}

func (c *HTTPClient) setRequestBody(req *resty.Request, options *Options) error {
	switch {
	case options.JSON != nil:
		req.SetBody(options.JSON)
	case options.Form != nil:
		req.SetFormData(options.Form)
	case options.Multipart != nil:
		for k, v := range options.Multipart {
			if v == nil {
				return fmt.Errorf("multipart field %q has nil value", k)
			}
			if v.Reader == nil {
				return fmt.Errorf("multipart field %q has nil Reader", k)
			}
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

func (c *HTTPClient) setRequestHeaders(req *resty.Request, options *Options) error {
	// Set default headers first
	req.SetHeader("User-Agent", "engine.io-go/1.0")
	req.SetHeader("Accept", "*/*")

	// Then set custom headers, allowing override of defaults
	for name, values := range options.Headers {
		if slices.ContainsFunc(values, utils.CheckInvalidHeaderChar) {
			return fmt.Errorf("invalid character in header %q value", name)
		}
	}
	req.SetHeaderMultiValues(options.Headers)
	return nil
}
