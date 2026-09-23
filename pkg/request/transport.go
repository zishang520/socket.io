package request

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Maximum number of retry attempts for alternative services
const (
	maxRetryAttempts int32 = 2
)

// Transport implements an HTTP transport that supports both standard HTTP/HTTPS
// and HTTP/3 (QUIC) protocols. It handles alternative services (Alt-Svc) for
// protocol negotiation and connection upgrades.
type Transport struct {
	standardTransport *http.Transport              // Standard HTTP/1.1 and HTTP/2 transport
	h3Transport       *http3.Transport             // HTTP/3 (QUIC) transport
	altSvcCache       types.Map[string, []*altSvc] // Cache for alternative services by origin
}

// altSvc represents an alternative service entry with protocol, endpoint,
// expiration time, and failure counter information
type altSvc struct {
	protocol string       // Protocol identifier (e.g., "h3", "h2")
	endpoint string       // Server endpoint (host:port)
	expires  time.Time    // Expiration time of this alt-svc entry
	failures atomic.Int32 // Counter for failed connection attempts
	persist  bool         // Whether this entry should persist across sessions
}

// NewTransport creates a new Transport instance with the specified TLS and QUIC configurations
func NewTransport(tlsClientConfig *tls.Config, quicConfig *quic.Config) *Transport {
	transport := &Transport{
		standardTransport: &http.Transport{
			ForceAttemptHTTP2: true,
		},
		h3Transport: &http3.Transport{
			QUICConfig: quicConfig,
		},
	}
	if tlsClientConfig != nil {
		// HTTP/2 initializes ALPN on its config; HTTP/3 and the caller retain
		// independent configurations.
		transport.standardTransport.TLSClientConfig = tlsClientConfig.Clone()
		transport.h3Transport.TLSClientConfig = tlsClientConfig.Clone()
	}
	return transport
}

// RoundTrip implements the http.RoundTripper interface. It attempts to send the request
// first using available alternative services, falling back to standard transport if needed
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var response *http.Response
	// An alternative may have processed the request before its response fails.
	// Only idempotent requests with replayable bodies can safely fall back.
	if canRetryRequest(req) {
		services, _ := t.altSvcCache.Load(getOrigin(req.URL))
		for _, svc := range services {
			if isServiceValid(svc) {
				if resp, err := t.tryService(req, svc); err == nil {
					response = resp
					break
				}
			}
		}
	}

	if response == nil {
		var err error
		response, err = t.standardTransport.RoundTrip(req)
		if err != nil {
			return nil, err
		}
	} else if req.Body != nil {
		// Alternative attempts own independent bodies. The original was never
		// handed to a transport, so close it when fallback is no longer needed.
		_ = req.Body.Close()
	}

	// Both the origin and its alternatives can update this origin's cache.
	t.processAltSvc(response.Header, req.URL)
	return response, nil
}

func canRetryRequest(req *http.Request) bool {
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return false
	}
	switch req.Method {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// isServiceValid checks if the service is still valid and hasn't exceeded retry attempts
func isServiceValid(svc *altSvc) bool {
	return !svc.expires.Before(time.Now()) && svc.failures.Load() < maxRetryAttempts
}

// tryService attempts to send the request using a specific alternative service
func (t *Transport) tryService(req *http.Request, svc *altSvc) (*http.Response, error) {
	// Apply alt-svc endpoint with same-origin validation (RFC 7838):
	// only port changes are allowed; the hostname must remain the same.
	endpoint := svc.endpoint
	if endpoint != "" {
		if after, ok := strings.CutPrefix(endpoint, ":"); ok {
			// Port-only override (e.g., ":443") — safe, keeps original hostname
			endpoint = net.JoinHostPort(req.URL.Hostname(), after)
		} else {
			// Full host:port — validate the hostname matches the original request
			// to prevent SSRF via malicious Alt-Svc headers.
			host, _, err := net.SplitHostPort(endpoint)
			if err != nil {
				// endpoint might be a bare hostname without port — reject
				return nil, errors.New("invalid alt-svc endpoint format")
			}
			if !strings.EqualFold(host, req.URL.Hostname()) {
				return nil, errors.New("alt-svc endpoint hostname does not match origin")
			}
			// Reject endpoints containing path/query/fragment characters
			if strings.ContainsAny(endpoint, "/?#") {
				return nil, errors.New("alt-svc endpoint contains invalid characters")
			}
		}
	}
	altReq := req.Clone(req.Context())
	if endpoint != "" {
		altReq.URL.Host = endpoint
	}
	if req.Body != nil && req.Body != http.NoBody {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		altReq.Body = body
	}

	var transport http.RoundTripper
	if strings.HasPrefix(svc.protocol, "h3") {
		transport = t.h3Transport
	} else {
		transport = t.standardTransport
	}

	resp, err := transport.RoundTrip(altReq)
	if err != nil {
		svc.failures.Add(1)
	}
	return resp, err
}

// processAltSvc processes the Alt-Svc header from responses and updates the alt-svc cache
// according to RFC 7838 specification
func (t *Transport) processAltSvc(header http.Header, reqURL *url.URL) {
	altSvc := header.Get("Alt-Svc")
	if altSvc == "" {
		return
	}
	origin := getOrigin(reqURL)

	// Handle "clear" directive
	if altSvc == "clear" {
		t.altSvcCache.Delete(origin)
		return
	}

	// Parse and store new alternative services
	entries := parseAltSvc(altSvc)
	if len(entries) > 0 {
		// The cache is scoped to this transport, including entries marked persist.
		t.altSvcCache.Store(origin, entries)
	}
}

// parseAltSvc parses the Alt-Svc header value into a slice of altSvc entries
// Format: protocol=host:port; ma=seconds; persist=1
func parseAltSvc(value string) []*altSvc {
	var result []*altSvc
	now := time.Now()

	// Split multiple entries
	entries := strings.SplitSeq(value, ",")
	for entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Split protocol and parameters
		protocol, params, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}

		protocol = strings.TrimSpace(protocol)
		params = strings.TrimSpace(params)

		// Parse parameters
		maxAge := int64(24 * 3600) // Default 24 hours
		persist := false

		// Split endpoint and parameters
		endpoint, params, _ := strings.Cut(params, ";")
		endpoint = strings.Trim(strings.TrimSpace(endpoint), `"`)

		// Parse additional parameters
		for param := range strings.SplitSeq(params, ";") {
			param = strings.TrimSpace(param)
			if param == "" {
				continue
			}

			key, value, ok := strings.Cut(param, "=")
			if !ok {
				continue
			}

			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)

			switch key {
			case "ma":
				if age, err := strconv.ParseInt(value, 10, 64); err == nil {
					// Clamp to prevent time.Duration overflow (max 1 year)
					const maxMaxAge int64 = 365 * 24 * 3600
					if age < 0 {
						age = 0
					} else if age > maxMaxAge {
						age = maxMaxAge
					}
					maxAge = age
				}
			case "persist":
				persist = value == "1"
			}
		}

		result = append(result, &altSvc{
			protocol: protocol,
			endpoint: endpoint,
			expires:  now.Add(time.Duration(maxAge) * time.Second),
			persist:  persist,
		})
	}

	return result
}

// Close closes both the standard and HTTP/3 transports
func (t *Transport) Close() error {
	t.standardTransport.CloseIdleConnections()
	return t.h3Transport.Close()
}

// Helper functions

// getOrigin extracts the scheme and authority from a URL.
// If port is not specified, it uses the default port for the scheme
func getOrigin(u *url.URL) string {
	authority := u.Host
	if u.Port() == "" {
		authority = net.JoinHostPort(u.Hostname(), defaultPort(u.Scheme))
	}
	return u.Scheme + "://" + authority
}

// defaultPort returns the default port number for a given scheme
func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}
