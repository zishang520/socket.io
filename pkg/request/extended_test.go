package request

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestRandomString(t *testing.T) {
	seen := make(map[string]struct{})
	for range 100 {
		r := RandomString()
		if len(r) != 13 || strings.Trim(r, "0123456789abcdefghijklmnopqrstuvwxyz") != "" {
			t.Fatalf("RandomString() = %q, want 13 lowercase base36 characters", r)
		}
		if _, exists := seen[r]; exists {
			t.Errorf("RandomString() produced duplicate: %s", r)
		}
		seen[r] = struct{}{}
	}
}

func BenchmarkRandomString(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = RandomString()
	}
}

func TestHTTPClientClose(t *testing.T) {
	client := NewHTTPClient()
	// First close should succeed
	if err := client.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
	// Second close should be a no-op (idempotent)
	if err := client.Close(); err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}

func TestHTTPClientCloseOwnsTransport(t *testing.T) {
	wantErr := errors.New("transport close")
	var calls []string
	transport := &closingTransport{close: func() error {
		calls = append(calls, "transport")
		return wantErr
	}}
	client := NewHTTPClient(WithTransport(transport))
	client.client.OnClose(func() {
		calls = append(calls, "client")
		if err := client.Close(); err != nil {
			t.Errorf("reentrant Close() = %v", err)
		}
	})
	if err := client.Close(); err != wantErr {
		t.Fatalf("Close() = %v, want original transport error", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("repeated Close() = %v", err)
	}
	if !slices.Equal(calls, []string{"client", "transport"}) {
		t.Fatalf("close callbacks = %v", calls)
	}
}

type closingTransport struct{ close func() error }

func (*closingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func (t *closingTransport) Close() error { return t.close() }

func TestHTTPClientBodyTypes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	client := NewHTTPClient()
	defer func() { _ = client.Close() }()

	t.Run("bytes body", func(t *testing.T) {
		data := []byte("bytes body content")
		resp, err := client.Post(ts.URL, &Options{
			Body: data,
		})
		if err != nil {
			t.Fatalf("POST with []byte body failed: %v", err)
		}
		if !bytes.Equal(resp.Bytes(), data) {
			t.Errorf("Expected %q, got %q", data, resp.Bytes())
		}
	})

	t.Run("io.Reader body", func(t *testing.T) {
		data := "reader body content"
		resp, err := client.Post(ts.URL, &Options{
			Body: strings.NewReader(data),
		})
		if err != nil {
			t.Fatalf("POST with io.Reader body failed: %v", err)
		}
		if string(resp.Bytes()) != data {
			t.Errorf("Expected %q, got %q", data, string(resp.Bytes()))
		}
	})

	t.Run("unsupported body type", func(t *testing.T) {
		_, err := client.Post(ts.URL, &Options{
			Body: 12345, // int is not a supported body type
		})
		if err == nil {
			t.Error("Expected error for unsupported body type")
		}
		if !strings.Contains(err.Error(), "unsupported body type") {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}

func TestHTTPClientQueryParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.RawQuery))
	}))
	defer ts.Close()

	client := NewHTTPClient()
	defer func() { _ = client.Close() }()

	resp, err := client.Get(ts.URL, &Options{
		Query: url.Values{
			"foo": {"bar"},
			"baz": {"qux"},
		},
	})
	if err != nil {
		t.Fatalf("GET with query params failed: %v", err)
	}
	body := string(resp.Bytes())
	if !strings.Contains(body, "foo=bar") || !strings.Contains(body, "baz=qux") {
		t.Errorf("Query params not forwarded correctly: %s", body)
	}
}

func TestHTTPClientCookies(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("test-cookie")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(cookie.Value))
	}))
	defer ts.Close()

	client := NewHTTPClient()
	defer func() { _ = client.Close() }()

	resp, err := client.Get(ts.URL, &Options{
		Cookies: []*http.Cookie{
			{Name: "test-cookie", Value: "cookie-value"},
		},
	})
	if err != nil {
		t.Fatalf("GET with cookies failed: %v", err)
	}
	if string(resp.Bytes()) != "cookie-value" {
		t.Errorf("Expected cookie value 'cookie-value', got %q", string(resp.Bytes()))
	}
}

func TestHTTPClientMultipartSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("upload")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()
		data, _ := io.ReadAll(file)
		_, _ = w.Write(data)
	}))
	defer ts.Close()

	client := NewHTTPClient()
	defer func() { _ = client.Close() }()

	content := "file content here"
	resp, err := client.Post(ts.URL, &Options{
		Multipart: map[string]*Multipart{
			"upload": {
				FileName:    "test.txt",
				ContentType: "text/plain",
				Reader:      strings.NewReader(content),
			},
		},
	})
	if err != nil {
		t.Fatalf("Multipart upload failed: %v", err)
	}
	if string(resp.Bytes()) != content {
		t.Errorf("Expected %q, got %q", content, string(resp.Bytes()))
	}
}

func TestResponseOkActual(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/created":
			w.WriteHeader(http.StatusCreated)
		case "/not-found":
			w.WriteHeader(http.StatusNotFound)
		case "/server-error":
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	client := NewHTTPClient()
	defer func() { _ = client.Close() }()

	tests := []struct {
		path     string
		expected bool
	}{
		{"/ok", true},
		{"/created", true},
		{"/not-found", false},
		{"/server-error", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := client.Get(ts.URL+tt.path, &Options{})
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			if resp.Ok() != tt.expected {
				t.Errorf("Ok() = %v, want %v for status %d", resp.Ok(), tt.expected, resp.StatusCode())
			}
		})
	}
}
