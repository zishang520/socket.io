package request

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name    string
		options []ClientOption
		wantErr bool
	}{
		{
			name:    "Default options",
			options: nil,
			wantErr: false,
		},
		{
			name: "With timeout",
			options: []ClientOption{
				WithTimeout(5 * time.Second),
			},
			wantErr: false,
		},
		{
			name: "With TLS config",
			options: []ClientOption{
				WithTLSClientConfig(&tls.Config{
					InsecureSkipVerify: true,
				}),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewHTTPClient(tt.options...)
			if !tt.wantErr && client == nil {
				t.Error("NewHTTPClient() returned nil client")
			}
		})
	}
}

func TestHTTPClient_Request(t *testing.T) {
	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("get response"))
		case "/post":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		case "/headers":
			customHeader := r.Header.Get("X-Custom-Header")
			w.Header().Set("X-Response-Header", customHeader)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	client := NewHTTPClient()

	t.Run("GET request", func(t *testing.T) {
		resp, err := client.Get(ts.URL+"/get", &Options{})
		if err != nil {
			t.Errorf("GET request failed: %v", err)
			return
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}
		if string(resp.Bytes()) != "get response" {
			t.Errorf("Expected body 'get response', got '%s'", string(resp.Bytes()))
		}
	})

	t.Run("POST request with JSON", func(t *testing.T) {
		data := map[string]string{"key": "value"}
		resp, err := client.Post(ts.URL+"/post", &Options{
			JSON: data,
		})
		if err != nil {
			t.Errorf("POST request failed: %v", err)
			return
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}

		var response map[string]string
		if err := json.Unmarshal(resp.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response JSON: %v", err)
			return
		}
		if response["key"] != "value" {
			t.Errorf("Expected response key 'value', got '%s'", response["key"])
		}
	})

	t.Run("Request with headers", func(t *testing.T) {
		resp, err := client.Get(ts.URL+"/headers", &Options{
			Headers: map[string][]string{
				"X-Custom-Header": {"test-value"},
			},
		})
		if err != nil {
			t.Errorf("Request with headers failed: %v", err)
			return
		}
		if resp.Header().Get("X-Response-Header") != "test-value" {
			t.Errorf("Expected header value 'test-value', got '%s'", resp.Header().Get("X-Response-Header"))
		}
	})

	t.Run("Request with context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		resp, err := client.Request(ctx, "GET", ts.URL+"/get", &Options{})
		if err != nil {
			t.Errorf("Request with context failed: %v", err)
			return
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}
	})

	t.Run("Request with form data", func(t *testing.T) {
		formData := map[string]string{
			"field1": "value1",
			"field2": "value2",
		}
		resp, err := client.Post(ts.URL+"/post", &Options{
			Form: formData,
		})
		if err != nil {
			t.Errorf("POST request with form data failed: %v", err)
			return
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}
	})

	t.Run("Request with string body", func(t *testing.T) {
		bodyStr := "test body content"
		resp, err := client.Post(ts.URL+"/post", &Options{
			Body: bodyStr,
		})
		if err != nil {
			t.Errorf("POST request with string body failed: %v", err)
			return
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}
		if string(resp.Bytes()) != bodyStr {
			t.Errorf("Expected body '%s', got '%s'", bodyStr, string(resp.Bytes()))
		}
	})

	t.Run("Request with invalid URL", func(t *testing.T) {
		_, err := client.Get("invalid-url", &Options{})
		if err == nil {
			t.Error("Expected error for invalid URL, got nil")
		}
	})
}

func TestHTTPClient_RequestMethods(t *testing.T) {
	client := NewHTTPClient()
	methods := map[string]func(string, *Options) (*Response, error){
		http.MethodGet:     func(url string, opts *Options) (*Response, error) { return client.Get(url, opts) },
		http.MethodPost:    func(url string, opts *Options) (*Response, error) { return client.Post(url, opts) },
		http.MethodPut:     func(url string, opts *Options) (*Response, error) { return client.Put(url, opts) },
		http.MethodDelete:  func(url string, opts *Options) (*Response, error) { return client.Delete(url, opts) },
		http.MethodPatch:   func(url string, opts *Options) (*Response, error) { return client.Patch(url, opts) },
		http.MethodHead:    func(url string, opts *Options) (*Response, error) { return client.Head(url, opts) },
		http.MethodOptions: func(url string, opts *Options) (*Response, error) { return client.Options(url, opts) },
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Method", r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	for method, fn := range methods {
		t.Run(method, func(t *testing.T) {
			resp, err := fn(ts.URL, &Options{})
			if err != nil {
				t.Errorf("%s request failed: %v", method, err)
				return
			}
			if resp.StatusCode() != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode())
			}
			if resp.Header().Get("X-Request-Method") != method {
				t.Errorf("Expected method %s, got %s", method, resp.Header().Get("X-Request-Method"))
			}
		})
	}
}

func TestHTTPClient_Authentication(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		w.Header().Set("X-Received-Auth", auth)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewHTTPClient()

	t.Run("Basic Auth", func(t *testing.T) {
		resp, err := client.Get(ts.URL, &Options{
			BasicAuth: &BasicAuth{
				Username: "user",
				Password: "pass",
			},
		})
		if err != nil {
			t.Errorf("Basic auth request failed: %v", err)
			return
		}
		auth := resp.Header().Get("X-Received-Auth")
		if !strings.HasPrefix(auth, "Basic ") {
			t.Errorf("Expected Basic auth header, got %s", auth)
		}
	})

	t.Run("Bearer Token", func(t *testing.T) {
		token := "test-token"
		resp, err := client.Get(ts.URL, &Options{
			BearerToken: token,
		})
		if err != nil {
			t.Errorf("Bearer token request failed: %v", err)
			return
		}
		auth := resp.Header().Get("X-Received-Auth")
		expected := "Bearer " + token
		if auth != expected {
			t.Errorf("Expected auth header %s, got %s", expected, auth)
		}
	})
}
