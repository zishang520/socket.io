package request

import (
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

type mockLogger struct {
	debugCalled bool
	infoCalled  bool
	warnCalled  bool
	errorCalled bool
}

func (m *mockLogger) Debugf(s string, v ...interface{}) {
	m.debugCalled = true
}

func (m *mockLogger) Infof(s string, v ...interface{}) {
	m.infoCalled = true
}

func (m *mockLogger) Warnf(s string, v ...interface{}) {
	m.warnCalled = true
}

func (m *mockLogger) Errorf(s string, v ...interface{}) {
	m.errorCalled = true
}

func TestWithFollowRedirects(t *testing.T) {
	tests := []struct {
		name          string
		follow        bool
		maxRedirects  int
		expectFollow  bool
		expectMaxReds int
	}{
		{
			name:          "Enable redirects with max 5",
			follow:        true,
			maxRedirects:  5,
			expectFollow:  true,
			expectMaxReds: 5,
		},
		{
			name:          "Disable redirects",
			follow:        false,
			maxRedirects:  0,
			expectFollow:  false,
			expectMaxReds: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := WithFollowRedirects(tt.follow, tt.maxRedirects)
			opts := &clientOptions{}
			opt(opts)

			if opts.FollowRedirects != tt.expectFollow {
				t.Errorf("FollowRedirects = %v, want %v", opts.FollowRedirects, tt.expectFollow)
			}
			if opts.MaxRedirects != tt.expectMaxReds {
				t.Errorf("MaxRedirects = %v, want %v", opts.MaxRedirects, tt.expectMaxReds)
			}
		})
	}
}

func TestWithLogger(t *testing.T) {
	logger := &mockLogger{}
	opt := WithLogger(logger)
	opts := &clientOptions{}
	opt(opts)

	if opts.Logger != logger {
		t.Error("Logger was not properly set")
	}
}

func TestWithBaseURL(t *testing.T) {
	baseURL := "https://api.example.com"
	opt := WithBaseURL(baseURL)
	opts := &clientOptions{}
	opt(opts)

	if opts.BaseURL != baseURL {
		t.Errorf("BaseURL = %v, want %v", opts.BaseURL, baseURL)
	}
}

func TestWithTimeout(t *testing.T) {
	timeout := 5 * time.Second
	opt := WithTimeout(timeout)
	opts := &clientOptions{}
	opt(opts)

	if opts.Timeout != timeout {
		t.Errorf("Timeout = %v, want %v", opts.Timeout, timeout)
	}
}

func TestWithCookieJar(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}

	opt := WithCookieJar(jar)
	opts := &clientOptions{}
	opt(opts)

	if opts.Jar != jar {
		t.Error("CookieJar was not properly set")
	}
}

func TestWithTLSClientConfig(t *testing.T) {
	config := &tls.Config{
		InsecureSkipVerify: true,
	}
	opt := WithTLSClientConfig(config)
	opts := &clientOptions{}
	opt(opts)

	if opts.TLSClientConfig != config {
		t.Error("TLSClientConfig was not properly set")
	}
}

func TestWithProxy(t *testing.T) {
	proxy := "http://proxy.example.com"
	opt := WithProxy(proxy)
	opts := &clientOptions{}
	opt(opts)

	if opts.Proxy != proxy {
		t.Errorf("Proxy = %v, want %v", opts.Proxy, proxy)
	}
}

func TestApplyOptions(t *testing.T) {
	t.Run("Default options", func(t *testing.T) {
		opts := applyOptions()
		if opts.Timeout != 0*time.Second {
			t.Errorf("Default timeout = %v, want %v", opts.Timeout, 0*time.Second)
		}
	})

	t.Run("Multiple options", func(t *testing.T) {
		logger := &mockLogger{}
		timeout := 5 * time.Second
		baseURL := "https://api.example.com"

		opts := applyOptions(
			WithLogger(logger),
			WithTimeout(timeout),
			WithBaseURL(baseURL),
			WithFollowRedirects(true, 3),
		)

		if opts.Logger != logger {
			t.Error("Logger was not properly set")
		}
		if opts.Timeout != timeout {
			t.Errorf("Timeout = %v, want %v", opts.Timeout, timeout)
		}
		if opts.BaseURL != baseURL {
			t.Errorf("BaseURL = %v, want %v", opts.BaseURL, baseURL)
		}
		if !opts.FollowRedirects {
			t.Error("FollowRedirects was not enabled")
		}
		if opts.MaxRedirects != 3 {
			t.Errorf("MaxRedirects = %v, want 3", opts.MaxRedirects)
		}
	})
}

func TestOptions(t *testing.T) {
	t.Run("Basic Auth", func(t *testing.T) {
		auth := &BasicAuth{
			Username: "user",
			Password: "pass",
		}
		opts := &Options{
			BasicAuth: auth,
		}
		if opts.BasicAuth.Username != "user" || opts.BasicAuth.Password != "pass" {
			t.Error("BasicAuth not properly set")
		}
	})

	t.Run("Headers", func(t *testing.T) {
		headers := http.Header{
			"Content-Type": []string{"application/json"},
			"X-Custom":     []string{"value"},
		}
		opts := &Options{
			Headers: headers,
		}
		if opts.Headers.Get("Content-Type") != "application/json" {
			t.Error("Headers not properly set")
		}
	})

	t.Run("Query Parameters", func(t *testing.T) {
		query := url.Values{}
		query.Add("key", "value")
		opts := &Options{
			Query: query,
		}
		if opts.Query.Get("key") != "value" {
			t.Error("Query parameters not properly set")
		}
	})

	t.Run("Multipart", func(t *testing.T) {
		content := strings.NewReader("test content")
		multipart := map[string]*Multipart{
			"file": {
				FileName:    "test.txt",
				ContentType: "text/plain",
				Reader:      content,
			},
		}
		opts := &Options{
			Multipart: multipart,
		}
		if opts.Multipart["file"].FileName != "test.txt" {
			t.Error("Multipart not properly set")
		}
	})

	t.Run("Form Data", func(t *testing.T) {
		form := map[string]string{
			"field1": "value1",
			"field2": "value2",
		}
		opts := &Options{
			Form: form,
		}
		if opts.Form["field1"] != "value1" || opts.Form["field2"] != "value2" {
			t.Error("Form data not properly set")
		}
	})

	t.Run("JSON Body", func(t *testing.T) {
		jsonData := map[string]interface{}{
			"key": "value",
		}
		opts := &Options{
			JSON: jsonData,
		}
		if opts.JSON.(map[string]interface{})["key"] != "value" {
			t.Error("JSON body not properly set")
		}
	})

	t.Run("Raw Body", func(t *testing.T) {
		body := "raw content"
		opts := &Options{
			Body: body,
		}
		if opts.Body.(string) != "raw content" {
			t.Error("Raw body not properly set")
		}
	})

	t.Run("Cookies", func(t *testing.T) {
		cookie := &http.Cookie{
			Name:  "session",
			Value: "123",
		}
		opts := &Options{
			Cookies: []*http.Cookie{cookie},
		}
		if len(opts.Cookies) != 1 || opts.Cookies[0].Name != "session" {
			t.Error("Cookies not properly set")
		}
	})
}
