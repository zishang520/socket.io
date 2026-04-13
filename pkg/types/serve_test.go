package types

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewServeMux(t *testing.T) {
	// Test with nil handler (should use DefaultServeMux)
	mux := NewServeMux(nil)
	if mux == nil {
		t.Fatal("Expected non-nil ServeMux")
	}
	if mux.DefaultHandler != http.DefaultServeMux {
		t.Error("Expected DefaultHandler to be http.DefaultServeMux")
	}
}

func TestNewServeMuxWithHandler(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	mux := NewServeMux(handler)

	// Just verify it doesn't panic and mux is not nil
	if mux == nil {
		t.Fatal("Expected non-nil ServeMux")
	}
	// Can't compare functions, but we verified mux was created successfully
}

func TestServeMuxHandle(t *testing.T) {
	mux := NewServeMux(nil)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.Handle("/test", testHandler)
}

func TestServeMuxHandleFunc(t *testing.T) {
	mux := NewServeMux(nil)

	called := false
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	// Make a request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	h, _ := mux.Handler(req)
	h.ServeHTTP(w, req)

	if !called {
		t.Error("Expected handler to be called")
	}
}

func TestServeMuxHandleEmptyPattern(t *testing.T) {
	mux := NewServeMux(nil)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for empty pattern")
		}
	}()

	mux.Handle("", http.NotFoundHandler())
}

func TestServeMuxHandleNilHandler(t *testing.T) {
	mux := NewServeMux(nil)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for nil handler")
		}
	}()

	mux.Handle("/test", nil)
}

func TestServeMuxHandleDuplicate(t *testing.T) {
	mux := NewServeMux(nil)

	mux.Handle("/test", http.NotFoundHandler())

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for duplicate pattern")
		}
	}()

	mux.Handle("/test", http.NotFoundHandler())
}

func TestServeMuxMatch(t *testing.T) {
	mux := NewServeMux(nil)

	mux.Handle("/exact", http.NotFoundHandler())
	mux.Handle("/prefix/", http.NotFoundHandler())

	// Test exact match
	h, pattern := mux.match("/exact")
	if h == nil {
		t.Error("Expected handler for exact match")
	}
	if pattern != "/exact" {
		t.Errorf("Expected pattern '/exact', got %q", pattern)
	}

	// Test prefix match
	h, pattern = mux.match("/prefix/something")
	if h == nil {
		t.Error("Expected handler for prefix match")
	}
	if pattern != "/prefix/" {
		t.Errorf("Expected pattern '/prefix/', got %q", pattern)
	}

	// Test no match
	h, pattern = mux.match("/nonexistent")
	if h != nil {
		t.Error("Expected nil handler for no match")
	}
	if pattern != "" {
		t.Errorf("Expected empty pattern for no match, got %q", pattern)
	}
}

func TestServeMuxHandler(t *testing.T) {
	mux := NewServeMux(nil)

	mux.Handle("/test", http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	h, pattern := mux.Handler(req)

	if h == nil {
		t.Error("Expected handler to be returned")
	}
	if pattern != "/test" {
		t.Errorf("Expected pattern '/test', got %q", pattern)
	}
}

func TestServeMuxHandlerDefault(t *testing.T) {
	called := false
	defaultHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	mux := NewServeMux(defaultHandler)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	h, pattern := mux.Handler(req)

	if h == nil {
		t.Error("Expected default handler to be returned")
	}
	if pattern != "" {
		t.Errorf("Expected empty pattern for default handler, got %q", pattern)
	}

	// Call the handler
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !called {
		t.Error("Expected default handler to be called")
	}
}

func TestServeMuxServeHTTP(t *testing.T) {
	mux := NewServeMux(nil)

	called := false
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if !called {
		t.Error("Expected handler to be called via ServeHTTP")
	}
}

func TestServeMuxServeHTTPAsterisk(t *testing.T) {
	mux := NewServeMux(nil)

	req := httptest.NewRequest("GET", "*", nil)
	req.ProtoMajor = 1
	req.ProtoMinor = 1
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for asterisk path, got %d", w.Code)
	}

	connection := w.Header().Get("Connection")
	if connection != "close" {
		t.Errorf("Expected Connection header 'close', got %q", connection)
	}
}

func TestServeMuxHostSpecific(t *testing.T) {
	mux := NewServeMux(nil)

	mux.Handle("example.com/test", http.NotFoundHandler())
	mux.Handle("/test", http.NotFoundHandler())

	// Host-specific should take precedence
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	h, pattern := mux.Handler(req)

	if h == nil {
		t.Error("Expected handler to be found")
	}
	if pattern != "example.com/test" {
		t.Errorf("Expected pattern 'example.com/test', got %q", pattern)
	}
}

func TestServeMuxPrefixSorting(t *testing.T) {
	mux := NewServeMux(nil)

	// Register prefixes in non-sorted order
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/api/v2/", http.NotFoundHandler())
	mux.Handle("/api/v1/", http.NotFoundHandler())

	// /api/v2/something should match /api/v2/ (longest prefix)
	req := httptest.NewRequest("GET", "/api/v2/users", nil)
	h, pattern := mux.Handler(req)

	if h == nil {
		t.Error("Expected handler to be found")
	}
	if pattern != "/api/v2/" {
		t.Errorf("Expected pattern '/api/v2/', got %q", pattern)
	}
}

func TestServeMuxStripHostPort(t *testing.T) {
	mux := NewServeMux(nil)

	mux.Handle("/test", http.NotFoundHandler())

	// Request with port should still match
	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "example.com:8080"

	h, _ := mux.Handler(req)
	if h == nil {
		t.Error("Expected handler to match even with port in host")
	}
}

func TestServeMuxConnectMethod(t *testing.T) {
	mux := NewServeMux(nil)

	mux.Handle("/test", http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodConnect, "/test", nil)
	h, pattern := mux.Handler(req)

	if h == nil {
		t.Error("Expected handler for CONNECT method")
	}
	if pattern != "/test" {
		t.Errorf("Expected pattern '/test', got %q", pattern)
	}
}
