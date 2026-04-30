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
	ctx := createTestContext("GET", "http://allowed.com")

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

	// When request origin doesn't match, ACAO header is omitted entirely
	// so the browser blocks the request without leaking the allowed origin.
	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("Expected empty ACAO header for non-matching origin, got %q", origin)
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

// --- New tests for enhanced IsOriginAllowed and configureOrigin ---

func TestIsOriginAllowed_StringSlice(t *testing.T) {
	c := &Cors{}
	allowed := []string{"http://a.com", "http://b.com"}
	if !c.IsOriginAllowed("http://b.com", allowed) {
		t.Error("Expected http://b.com to be allowed")
	}
	if c.IsOriginAllowed("http://c.com", allowed) {
		t.Error("Expected http://c.com to be rejected")
	}
}

func TestIsOriginAllowed_StringSliceCaseInsensitive(t *testing.T) {
	c := &Cors{}
	allowed := []string{"http://Example.COM"}
	if !c.IsOriginAllowed("http://example.com", allowed) {
		t.Error("Expected case-insensitive match to succeed")
	}
}

func TestIsOriginAllowed_FuncCallback(t *testing.T) {
	c := &Cors{}
	fn := func(origin string) bool {
		return origin == "http://dynamic.com"
	}
	if !c.IsOriginAllowed("http://dynamic.com", fn) {
		t.Error("Expected func callback to allow http://dynamic.com")
	}
	if c.IsOriginAllowed("http://other.com", fn) {
		t.Error("Expected func callback to reject http://other.com")
	}
}

func TestIsOriginAllowed_StringCaseInsensitive(t *testing.T) {
	c := &Cors{}
	if !c.IsOriginAllowed("HTTP://EXAMPLE.COM", "http://example.com") {
		t.Error("Expected case-insensitive string match")
	}
}

func TestIsOriginAllowed_UnsupportedType(t *testing.T) {
	c := &Cors{}
	if c.IsOriginAllowed("http://x.com", 42) {
		t.Error("Expected unsupported type to return false")
	}
}

func TestIsOriginAllowed_BoolFalse(t *testing.T) {
	c := &Cors{}
	if c.IsOriginAllowed("http://any.com", false) {
		t.Error("Expected bool false to reject")
	}
}

func TestIsOriginAllowed_NestedAnySlice(t *testing.T) {
	c := &Cors{}
	allowed := []any{
		"http://a.com",
		regexp.MustCompile(`^http://.*\.example\.com$`),
		func(origin string) bool { return origin == "http://dynamic.com" },
	}
	if !c.IsOriginAllowed("http://a.com", allowed) {
		t.Error("Expected string in []any to match")
	}
	if !c.IsOriginAllowed("http://sub.example.com", allowed) {
		t.Error("Expected regexp in []any to match")
	}
	if !c.IsOriginAllowed("http://dynamic.com", allowed) {
		t.Error("Expected func in []any to match")
	}
	if c.IsOriginAllowed("http://evil.com", allowed) {
		t.Error("Expected non-matching origin to be rejected")
	}
}

func TestCorsEmptyOriginHeader(t *testing.T) {
	ctx := createTestContext("GET", "")

	options := &Cors{
		Origin:  "http://allowed.com",
		Methods: "GET",
	}

	CorsMiddleware(options, ctx, func(err error) {})

	// No Origin header → CORS does not apply, ACAO should be absent.
	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("Expected no ACAO header for empty Origin, got %q", origin)
	}
}

func TestCorsFixedOriginCaseInsensitive(t *testing.T) {
	ctx := createTestContext("GET", "HTTP://ALLOWED.COM")

	options := &Cors{
		Origin:  "http://allowed.com",
		Methods: "GET",
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://allowed.com" {
		t.Errorf("Expected case-insensitive fixed origin match, got %q", origin)
	}
}

func TestCorsFixedOriginMismatchOmitsHeader(t *testing.T) {
	ctx := createTestContext("GET", "http://evil.com")

	options := &Cors{
		Origin:  "http://allowed.com",
		Methods: "GET",
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("Expected ACAO to be omitted for mismatched origin, got %q", origin)
	}
}

func TestCorsReflectOriginNotAllowedOmitsHeader(t *testing.T) {
	ctx := createTestContext("GET", "http://evil.com")

	options := &Cors{
		Origin:  regexp.MustCompile(`^http://good\.com$`),
		Methods: "GET",
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("Expected ACAO to be omitted for non-allowed origin, got %q", origin)
	}
}

func TestCorsWildcardWithCredentialsReflectsOrigin(t *testing.T) {
	ctx := createTestContext("GET", "http://specific.com")

	options := &Cors{
		Origin:      "*",
		Credentials: true,
		Methods:     "GET",
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://specific.com" {
		t.Errorf("Expected reflected origin with wildcard+credentials, got %q", origin)
	}
	creds := ctx.ResponseHeaders().Peek("Access-Control-Allow-Credentials")
	if creds != "true" {
		t.Errorf("Expected credentials 'true', got %q", creds)
	}
}

func TestCorsPreflightNoExposeHeaders(t *testing.T) {
	ctx := createTestContext("OPTIONS", "http://example.com")

	options := &Cors{
		Origin:               "*",
		Methods:              "GET",
		ExposedHeaders:       "X-Custom",
		PreflightContinue:    false,
		OptionsSuccessStatus: 204,
	}

	CorsMiddleware(options, ctx, func(err error) {})

	exposed := ctx.ResponseHeaders().Peek("Access-Control-Expose-Headers")
	if exposed != "" {
		t.Errorf("Expected no Expose-Headers in preflight, got %q", exposed)
	}
}

func TestCorsActualResponseHasExposeHeaders(t *testing.T) {
	ctx := createTestContext("GET", "http://example.com")

	options := &Cors{
		Origin:         "*",
		Methods:        "GET",
		ExposedHeaders: "X-Custom",
	}

	CorsMiddleware(options, ctx, func(err error) {})

	exposed := ctx.ResponseHeaders().Peek("Access-Control-Expose-Headers")
	if exposed != "X-Custom" {
		t.Errorf("Expected Expose-Headers in actual response, got %q", exposed)
	}
}

func TestCorsStringSliceOriginMiddleware(t *testing.T) {
	ctx := createTestContext("GET", "http://b.com")

	options := &Cors{
		Origin:  []string{"http://a.com", "http://b.com"},
		Methods: "GET",
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://b.com" {
		t.Errorf("Expected reflected origin from []string, got %q", origin)
	}
}

func TestCorsFuncOriginMiddleware(t *testing.T) {
	ctx := createTestContext("GET", "http://dynamic.com")

	options := &Cors{
		Origin: func(origin string) bool {
			return origin == "http://dynamic.com"
		},
		Methods: "GET",
	}

	CorsMiddleware(options, ctx, func(err error) {})

	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "http://dynamic.com" {
		t.Errorf("Expected reflected origin from func, got %q", origin)
	}
}

func TestMiddlewareWrapperDefaults(t *testing.T) {
	mw := MiddlewareWrapper(&Cors{})

	ctx := createTestContext("GET", "http://example.com")
	called := false
	mw(ctx, func(err error) { called = true })

	if !called {
		t.Error("Expected next to be called")
	}
	// Origin defaults to "*"
	origin := ctx.ResponseHeaders().Peek("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("Expected default wildcard origin, got %q", origin)
	}
}
