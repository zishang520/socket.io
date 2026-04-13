package types

import (
	"net/http/httptest"
	"regexp"
	"testing"
)

func createTestContext(method, origin string) *HttpContext {
	req := httptest.NewRequest(method, "/", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	return NewHttpContext(w, req)
}

func TestCorsWildcardOrigin(t *testing.T) {
	ctx := createTestContext("GET", "http://example.com")

	options := &Cors{
		Origin:               "*",
		Methods:              "GET,POST",
		PreflightContinue:    false,
		OptionsSuccessStatus: 204,
	}

	called := false
	CorsMiddleware(options, ctx, func(err error) {
		called = true
	})

	if !called {
		t.Error("Expected next handler to be called")
	}
}

func TestCorsFixedOrigin(t *testing.T) {
	ctx := createTestContext("GET", "http://example.com")

	options := &Cors{
		Origin:            "http://allowed.com",
		Methods:           "GET",
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://allowed.com" {
		t.Errorf("Expected origin 'http://allowed.com', got %q", origin)
	}
}

func TestCorsRegexOrigin(t *testing.T) {
	ctx := createTestContext("GET", "http://sub.example.com")

	options := &Cors{
		Origin:            regexp.MustCompile(`^http://.*\.example\.com$`),
		Methods:           "GET",
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://sub.example.com" {
		t.Errorf("Expected allowed origin, got %q", origin)
	}
}

func TestCorsNotAllowedOrigin(t *testing.T) {
	ctx := createTestContext("GET", "http://malicious.com")

	options := &Cors{
		Origin:            "http://allowed.com",
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	// When Origin is a string (not "*"), it sets that fixed origin regardless of request origin
	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://allowed.com" {
		t.Errorf("Expected origin 'http://allowed.com' (fixed), got %q", origin)
	}
}

func TestCorsAllowedOriginSlice(t *testing.T) {
	ctx := createTestContext("GET", "http://allowed2.com")

	options := &Cors{
		Origin:            []any{"http://allowed1.com", "http://allowed2.com"},
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://allowed2.com" {
		t.Errorf("Expected origin 'http://allowed2.com', got %q", origin)
	}
}

func TestCorsBoolOrigin(t *testing.T) {
	ctx := createTestContext("GET", "http://example.com")

	options := &Cors{
		Origin:            true,
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://example.com" {
		t.Errorf("Expected origin to be reflected, got %q", origin)
	}
}

func TestCorsMethods(t *testing.T) {
	ctx := createTestContext("OPTIONS", "")

	options := &Cors{
		Origin:            "*",
		Methods:           "GET,POST,PUT,DELETE",
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	methods := ctx.ResponseHeaders().Peek("Access-Control-Allow-Methods")
	if methods != "GET,POST,PUT,DELETE" {
		t.Errorf("Expected methods 'GET,POST,PUT,DELETE', got %q", methods)
	}
}

func TestCorsMethodsSlice(t *testing.T) {
	ctx := createTestContext("OPTIONS", "")

	options := &Cors{
		Origin:            "*",
		Methods:           []string{"GET", "POST"},
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	methods := ctx.ResponseHeaders().Peek("Access-Control-Allow-Methods")
	if methods != "GET,POST" {
		t.Errorf("Expected methods 'GET,POST', got %q", methods)
	}
}

func TestCorsCredentials(t *testing.T) {
	ctx := createTestContext("GET", "")

	options := &Cors{
		Origin:            "*",
		Credentials:       true,
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	credentials := ctx.ResponseHeaders().Peek("Access-Control-Allow-Credentials")
	if credentials != "true" {
		t.Errorf("Expected credentials 'true', got %q", credentials)
	}
}

func TestCorsNoCredentials(t *testing.T) {
	ctx := createTestContext("GET", "")

	options := &Cors{
		Origin:            "*",
		Credentials:       false,
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	credentials := ctx.ResponseHeaders().Peek("Access-Control-Allow-Credentials")
	if credentials != "" {
		t.Errorf("Expected no credentials header, got %q", credentials)
	}
}

func TestCorsMaxAge(t *testing.T) {
	ctx := createTestContext("OPTIONS", "")

	options := &Cors{
		Origin:            "*",
		MaxAge:            "3600",
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	maxAge := ctx.ResponseHeaders().Peek("Access-Control-Max-Age")
	if maxAge != "3600" {
		t.Errorf("Expected max age '3600', got %q", maxAge)
	}
}

func TestCorsExposedHeaders(t *testing.T) {
	ctx := createTestContext("GET", "")

	options := &Cors{
		Origin:            "*",
		ExposedHeaders:    "X-Custom-Header, X-Another",
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	exposed := ctx.ResponseHeaders().Peek("Access-Control-Expose-Headers")
	if exposed != "X-Custom-Header, X-Another" {
		t.Errorf("Expected exposed headers 'X-Custom-Header, X-Another', got %q", exposed)
	}
}

func TestCorsExposedHeadersSlice(t *testing.T) {
	ctx := createTestContext("GET", "")

	options := &Cors{
		Origin:            "*",
		ExposedHeaders:    []string{"X-One", "X-Two"},
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	exposed := ctx.ResponseHeaders().Peek("Access-Control-Expose-Headers")
	if exposed != "X-One,X-Two" {
		t.Errorf("Expected exposed headers 'X-One,X-Two', got %q", exposed)
	}
}

func TestCorsAllowedHeaders(t *testing.T) {
	ctx := createTestContext("OPTIONS", "")

	options := &Cors{
		Origin:            "*",
		AllowedHeaders:    "Content-Type, Authorization",
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	allowed := ctx.ResponseHeaders().Peek("Access-Control-Allow-Headers")
	if allowed != "Content-Type, Authorization" {
		t.Errorf("Expected allowed headers 'Content-Type, Authorization', got %q", allowed)
	}
}

func TestCorsReflectRequestHeaders(t *testing.T) {
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Access-Control-Request-Headers", "X-Custom, X-Another")
	w := httptest.NewRecorder()
	ctx := NewHttpContext(w, req)

	options := &Cors{
		Origin:            "*",
		PreflightContinue: false,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	allowed := ctx.ResponseHeaders().Peek("Access-Control-Allow-Headers")
	if allowed != "X-Custom, X-Another" {
		t.Errorf("Expected to reflect request headers, got %q", allowed)
	}
}

func TestCorsPreflightContinue(t *testing.T) {
	ctx := createTestContext("OPTIONS", "http://example.com")

	called := false
	options := &Cors{
		Origin:               "*",
		PreflightContinue:    true,
		OptionsSuccessStatus: 200,
	}

	CorsMiddleware(options, ctx, func(err error) {
		called = true
	})

	if !called {
		t.Error("Expected next handler to be called with PreflightContinue=true")
	}
}

func TestCorsPreflightNoContinue(t *testing.T) {
	ctx := createTestContext("OPTIONS", "http://example.com")

	called := false
	options := &Cors{
		Origin:               "*",
		PreflightContinue:    false,
		OptionsSuccessStatus: 204,
	}

	CorsMiddleware(options, ctx, func(err error) {
		called = true
	})

	if called {
		t.Error("Expected next handler NOT to be called with PreflightContinue=false")
	}

	contentLength := ctx.ResponseHeaders().Peek("Content-Length")
	if contentLength != "0" {
		t.Errorf("Expected Content-Length '0', got %q", contentLength)
	}
}

func TestCorsMiddlewareWrapper(t *testing.T) {
	ctx := createTestContext("GET", "http://example.com")

	middleware := MiddlewareWrapper(&Cors{
		Origin: "http://custom.com",
	})

	called := false
	middleware(ctx, func(err error) {
		called = true
	})

	if !called {
		t.Error("Expected middleware to call next handler")
	}
}

func TestCorsMiddlewareWrapperNilOptions(t *testing.T) {
	ctx := createTestContext("GET", "http://example.com")

	middleware := MiddlewareWrapper(nil)

	called := false
	middleware(ctx, func(err error) {
		called = true
	})

	if !called {
		t.Error("Expected middleware with nil options to call next handler")
	}
}

func TestCorsMiddlewareWrapperNilOrigin(t *testing.T) {
	ctx := createTestContext("GET", "")

	middleware := MiddlewareWrapper(&Cors{
		Origin: nil,
	})

	called := false
	middleware(ctx, func(err error) {
		called = true
	})

	if !called {
		t.Error("Expected middleware with nil origin to call next handler")
	}
}

func TestParseVary(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Accept, Content-Type", []string{"Accept", "Content-Type"}},
		{"Origin", []string{"Origin"}},
		{"Accept, Origin, X-Custom", []string{"Accept", "Origin", "X-Custom"}},
	}

	for _, tt := range tests {
		result := parseVary(tt.input)
		if result.Len() != len(tt.expected) {
			t.Errorf("parseVary(%q) expected %d items, got %d", tt.input, len(tt.expected), result.Len())
		}
	}
}
