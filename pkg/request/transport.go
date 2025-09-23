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
	return &Transport{
		standardTransport: &http.Transport{
			TLSClientConfig: tlsClientConfig,
		},
		h3Transport: &http3.Transport{
			TLSClientConfig: tlsClientConfig,
			QUICConfig:      quicConfig,
		},
		altSvcCache: types.Map[string, []*altSvc]{},
	}
}

// RoundTrip implements the http.RoundTripper interface. It attempts to send the request
// first using available alternative services, falling back to standard transport if needed
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Try alternative services first
	if resp, err := t.tryAltServices(req); err == nil {
		return resp, nil
	}

	// Fallback to standard transport
	resp, err := t.standardTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Process Alt-Svc header
	t.processAltSvc(resp.Header, req.URL)
	return resp, nil
}

// tryAltServices attempts to send the request using available alternative services
func (t *Transport) tryAltServices(req *http.Request) (*http.Response, error) {
	services, _ := t.altSvcCache.Load(getOrigin(req.URL))

	for _, svc := range services {
		if !isServiceValid(svc) {
			continue
		}

		if resp, err := t.tryService(req, svc); err == nil {
			return resp, nil
		}
	}

	return nil, errors.New("all alt-svc attempts failed")
}

// isServiceValid checks if the service is still valid and hasn't exceeded retry attempts
func isServiceValid(svc *altSvc) bool {
	return !svc.expires.Before(time.Now()) && svc.failures.Load() < maxRetryAttempts
}

// tryService attempts to send the request using a specific alternative service
func (t *Transport) tryService(req *http.Request, svc *altSvc) (*http.Response, error) {
	altReq := req.Clone(req.Context())

	// If endpoint only contains port (e.g., ":443"), use original host
	if endpoint := svc.endpoint; endpoint != "" {
		if after, ok := strings.CutPrefix(endpoint, ":"); ok {
			endpoint = net.JoinHostPort(req.URL.Hostname(), after)
		}
		altReq.URL.Host = endpoint
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
		// If persist flag is set, store in persistent storage
		// Note: This is a simplified implementation. In a real-world scenario,
		// you would want to implement proper persistent storage.
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
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}

		protocol := strings.TrimSpace(parts[0])
		params := strings.TrimSpace(parts[1])

		// Parse parameters
		maxAge := int64(24 * 3600) // Default 24 hours
		persist := false

		// Split endpoint and parameters
		paramParts := strings.Split(params, ";")
		endpoint := strings.Trim(strings.TrimSpace(paramParts[0]), `"`)

		// Parse additional parameters
		for _, param := range paramParts[1:] {
			param = strings.TrimSpace(param)
			if param == "" {
				continue
			}

			kv := strings.SplitN(param, "=", 2)
			if len(kv) != 2 {
				continue
			}

			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])

			switch key {
			case "ma":
				if age, err := strconv.ParseInt(value, 10, 64); err == nil {
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

// getOrigin extracts the origin (host:port) from a URL
// If port is not specified, it uses the default port for the scheme
func getOrigin(u *url.URL) string {
	if u.Port() == "" {
		return net.JoinHostPort(u.Hostname(), defaultPort(u.Scheme))
	}
	return u.Host
}

// defaultPort returns the default port number for a given scheme
func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}
