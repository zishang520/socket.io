package request

import (
	"net/url"
	"testing"
	"time"
)

func TestParseAltSvc(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "single entry",
			input:    `h3=":443"; ma=3600`,
			expected: 1,
		},
		{
			name:     "multiple entries",
			input:    `h3=":443"; ma=3600, h3=":4433"; ma=7200`,
			expected: 2,
		},
		{
			name:     "with persist",
			input:    `h3=":443"; ma=3600; persist=1`,
			expected: 1,
		},
		{
			name:     "empty input",
			input:    "",
			expected: 0,
		},
		{
			name:     "invalid format",
			input:    "invalid",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAltSvc(tt.input)
			if len(result) != tt.expected {
				t.Errorf("Expected %d entries, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestParseAltSvcDetails(t *testing.T) {
	input := `h3=":443"; ma=7200; persist=1`
	result := parseAltSvc(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}

	svc := result[0]
	if svc.protocol != "h3" {
		t.Errorf("Expected protocol 'h3', got %q", svc.protocol)
	}
	if svc.endpoint != ":443" {
		t.Errorf("Expected endpoint ':443', got %q", svc.endpoint)
	}
	if !svc.persist {
		t.Error("Expected persist to be true")
	}
	// Expiration should be approximately 7200 seconds from now
	expectedExpiry := time.Now().Add(7200 * time.Second)
	if svc.expires.Sub(expectedExpiry) > time.Second {
		t.Errorf("Expected expiration around %v, got %v", expectedExpiry, svc.expires)
	}
}

func TestParseAltSvcMultipleProtocols(t *testing.T) {
	input := `h3=":443"; ma=3600, h2="example.com:443"; ma=1800`
	result := parseAltSvc(input)

	if len(result) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(result))
	}

	if result[0].protocol != "h3" {
		t.Errorf("Expected first protocol 'h3', got %q", result[0].protocol)
	}
	if result[1].protocol != "h2" {
		t.Errorf("Expected second protocol 'h2', got %q", result[1].protocol)
	}
}

func TestParseAltSvcDefaultMaxAge(t *testing.T) {
	input := `h3=":443"`
	result := parseAltSvc(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}

	// Default max-age is 24 hours
	expectedExpiry := time.Now().Add(24 * 3600 * time.Second)
	if result[0].expires.Sub(expectedExpiry) > time.Second {
		t.Errorf("Expected default expiration around %v, got %v", expectedExpiry, result[0].expires)
	}
}

func TestParseAltSvcInvalidMaxAge(t *testing.T) {
	input := `h3=":443"; ma=invalid`
	result := parseAltSvc(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}

	// Should use default max-age when invalid
	expectedExpiry := time.Now().Add(24 * 3600 * time.Second)
	if result[0].expires.Sub(expectedExpiry) > time.Second {
		t.Errorf("Expected default expiration for invalid max-age, got %v", result[0].expires)
	}
}

func TestParseAltSvcNegativeMaxAge(t *testing.T) {
	input := `h3=":443"; ma=-100`
	result := parseAltSvc(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}

	// Negative max-age should be clamped to 0
	expectedExpiry := time.Now()
	if result[0].expires.Sub(expectedExpiry) > time.Second {
		t.Errorf("Expected expiration at now for negative max-age, got %v", result[0].expires)
	}
}

func TestParseAltSvcLargeMaxAge(t *testing.T) {
	// Max age larger than 1 year should be clamped
	input := `h3=":443"; ma=999999999`
	result := parseAltSvc(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}

	// Should be clamped to 1 year
	maxExpiry := time.Now().Add(365 * 24 * 3600 * time.Second)
	if result[0].expires.After(maxExpiry.Add(time.Second)) {
		t.Errorf("Expected expiration clamped to 1 year, got %v", result[0].expires)
	}
}

func TestGetOrigin(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "https with port",
			url:      "https://example.com:8443/path",
			expected: "example.com:8443",
		},
		{
			name:     "https without port",
			url:      "https://example.com/path",
			expected: "example.com:443",
		},
		{
			name:     "http with port",
			url:      "http://example.com:8080/path",
			expected: "example.com:8080",
		},
		{
			name:     "http without port",
			url:      "http://example.com/path",
			expected: "example.com:80",
		},
		{
			name:     "localhost",
			url:      "http://localhost:3000/api",
			expected: "localhost:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			origin := getOrigin(u)
			if origin != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, origin)
			}
		})
	}
}

func TestDefaultPort(t *testing.T) {
	tests := []struct {
		scheme   string
		expected string
	}{
		{"https", "443"},
		{"http", "80"},
		{"ws", "80"},
		{"custom", "80"},
	}

	for _, tt := range tests {
		t.Run(tt.scheme, func(t *testing.T) {
			port := defaultPort(tt.scheme)
			if port != tt.expected {
				t.Errorf("defaultPort(%q) = %q, want %q", tt.scheme, port, tt.expected)
			}
		})
	}
}

func TestIsServiceValid(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		svc      *altSvc
		expected bool
	}{
		{
			name: "valid service",
			svc: &altSvc{
				expires: now.Add(time.Hour),
			},
			expected: true,
		},
		{
			name: "expired service",
			svc: &altSvc{
				expires: now.Add(-time.Hour),
			},
			expected: false,
		},
		{
			name: "service at max failures",
			svc: &altSvc{
				expires: now.Add(time.Hour),
			},
			expected: false,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set failures for the third test case
			if i == 2 {
				tt.svc.failures.Store(maxRetryAttempts)
			}

			result := isServiceValid(tt.svc)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsServiceValidExceedsRetries(t *testing.T) {
	svc := &altSvc{
		expires: time.Now().Add(time.Hour),
	}
	svc.failures.Store(maxRetryAttempts + 1)

	if isServiceValid(svc) {
		t.Error("Expected service to be invalid after exceeding retry attempts")
	}
}

func TestNewTransport(t *testing.T) {
	transport := NewTransport(nil, nil)

	if transport == nil {
		t.Fatal("Expected transport to be created")
	}
	if transport.standardTransport == nil {
		t.Error("Expected standard transport to be initialized")
	}
	if transport.h3Transport == nil {
		t.Error("Expected h3 transport to be initialized")
	}
}

func TestTransportClose(t *testing.T) {
	transport := NewTransport(nil, nil)

	err := transport.Close()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
