package engine

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestSocketOptions(t *testing.T) {
	opts := DefaultSocketOptions()

	// Test Host
	opts.SetHost("example.com")
	if opts.Host() != "example.com" {
		t.Errorf("Host() = %v, want %v", opts.Host(), "example.com")
	}

	// Test Hostname
	opts.SetHostname("test.com")
	if opts.Hostname() != "test.com" {
		t.Errorf("Hostname() = %v, want %v", opts.Hostname(), "test.com")
	}

	// Test Secure
	opts.SetSecure(true)
	if !opts.Secure() {
		t.Error("Secure() should be true")
	}

	// Test Port
	opts.SetPort("8080")
	if opts.Port() != "8080" {
		t.Errorf("Port() = %v, want %v", opts.Port(), "8080")
	}

	// Test Query
	query := url.Values{}
	query.Set("key", "value")
	opts.SetQuery(query)
	if opts.Query().Get("key") != "value" {
		t.Errorf("Query() = %v, want %v", opts.Query().Get("key"), "value")
	}

	// Test Agent
	opts.SetAgent("test-agent")
	if opts.Agent() != "test-agent" {
		t.Errorf("Agent() = %v, want %v", opts.Agent(), "test-agent")
	}

	// Test Upgrade
	opts.SetUpgrade(true)
	if !opts.Upgrade() {
		t.Error("Upgrade() should be true")
	}

	// Test ForceBase64
	opts.SetForceBase64(true)
	if !opts.ForceBase64() {
		t.Error("ForceBase64() should be true")
	}

	// Test TimestampParam
	opts.SetTimestampParam("timestamp")
	if opts.TimestampParam() != "timestamp" {
		t.Errorf("TimestampParam() = %v, want %v", opts.TimestampParam(), "timestamp")
	}

	// Test TimestampRequests
	opts.SetTimestampRequests(true)
	if !opts.TimestampRequests() {
		t.Error("TimestampRequests() should be true")
	}

	// Test Transports
	transports := types.NewSet[TransportCtor]()
	opts.SetTransports(transports)
	if opts.Transports() != transports {
		t.Error("Transports() should match the set value")
	}

	// Test TryAllTransports
	opts.SetTryAllTransports(true)
	if !opts.TryAllTransports() {
		t.Error("TryAllTransports() should be true")
	}

	// Test RememberUpgrade
	opts.SetRememberUpgrade(true)
	if !opts.RememberUpgrade() {
		t.Error("RememberUpgrade() should be true")
	}

	// Test RequestTimeout
	timeout := 5 * time.Second
	opts.SetRequestTimeout(timeout)
	if opts.RequestTimeout() != timeout {
		t.Errorf("RequestTimeout() = %v, want %v", opts.RequestTimeout(), timeout)
	}

	// Test TransportOptions
	transportOpts := make(map[string]SocketOptionsInterface)
	opts.SetTransportOptions(transportOpts)
	if opts.TransportOptions() == nil {
		t.Error("TransportOptions() should not be nil")
	}

	// Test TLSClientConfig
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	opts.SetTLSClientConfig(tlsConfig)
	if opts.TLSClientConfig() != tlsConfig {
		t.Error("TLSClientConfig() should match the set value")
	}

	// Test QUICConfig
	quicConfig := &quic.Config{}
	opts.SetQUICConfig(quicConfig)
	if opts.QUICConfig() != quicConfig {
		t.Error("QUICConfig() should match the set value")
	}

	// Test ExtraHeaders
	headers := http.Header{}
	headers.Set("X-Test", "test")
	opts.SetExtraHeaders(headers)
	if opts.ExtraHeaders().Get("X-Test") != "test" {
		t.Errorf("ExtraHeaders() = %v, want %v", opts.ExtraHeaders().Get("X-Test"), "test")
	}

	// Test WithCredentials
	opts.SetWithCredentials(true)
	if !opts.WithCredentials() {
		t.Error("WithCredentials() should be true")
	}

	// Test UseNativeTimers
	opts.SetUseNativeTimers(true)
	if !opts.UseNativeTimers() {
		t.Error("UseNativeTimers() should be true")
	}

	// Test AutoUnref
	opts.SetAutoUnref(true)
	if !opts.AutoUnref() {
		t.Error("AutoUnref() should be true")
	}

	// Test CloseOnBeforeunload
	opts.SetCloseOnBeforeunload(true)
	if !opts.CloseOnBeforeunload() {
		t.Error("CloseOnBeforeunload() should be true")
	}

	// Test PerMessageDeflate
	pmd := &types.PerMessageDeflate{}
	opts.SetPerMessageDeflate(pmd)
	if opts.PerMessageDeflate() != pmd {
		t.Error("PerMessageDeflate() should match the set value")
	}

	// Test Path
	opts.SetPath("/test")
	if opts.Path() != "/test" {
		t.Errorf("Path() = %v, want %v", opts.Path(), "/test")
	}

	// Test AddTrailingSlash
	opts.SetAddTrailingSlash(true)
	if !opts.AddTrailingSlash() {
		t.Error("AddTrailingSlash() should be true")
	}

	// Test Protocols
	protocols := []string{"protocol1", "protocol2"}
	opts.SetProtocols(protocols)
	if len(opts.Protocols()) != len(protocols) {
		t.Errorf("Protocols() length = %v, want %v", len(opts.Protocols()), len(protocols))
	}
	for i, p := range protocols {
		if opts.Protocols()[i] != p {
			t.Errorf("Protocols()[%d] = %v, want %v", i, opts.Protocols()[i], p)
		}
	}
}

func TestSocketOptionsAssign(t *testing.T) {
	source := DefaultSocketOptions()
	source.SetHost("test.com")
	source.SetPort("8080")
	source.SetSecure(true)

	target := DefaultSocketOptions()
	target.Assign(source)

	if target.Host() != "test.com" {
		t.Errorf("Assigned Host() = %v, want %v", target.Host(), "test.com")
	}
	if target.Port() != "8080" {
		t.Errorf("Assigned Port() = %v, want %v", target.Port(), "8080")
	}
	if !target.Secure() {
		t.Error("Assigned Secure() should be true")
	}
}

func TestSocketOptionsDefaultValues(t *testing.T) {
	opts := DefaultSocketOptions()

	// Test default values
	if opts.Host() != "" {
		t.Errorf("Default Host() = %v, want empty string", opts.Host())
	}
	if opts.Hostname() != "" {
		t.Errorf("Default Hostname() = %v, want empty string", opts.Hostname())
	}
	if opts.Secure() {
		t.Error("Default Secure() should be false")
	}
	if opts.Port() != "" {
		t.Errorf("Default Port() = %v, want empty string", opts.Port())
	}
	if opts.Query() != nil {
		t.Error("Default Query() should be nil")
	}
	if opts.Agent() != "" {
		t.Errorf("Default Agent() = %v, want empty string", opts.Agent())
	}
	if opts.Upgrade() {
		t.Error("Default Upgrade() should be false")
	}
	if opts.ForceBase64() {
		t.Error("Default ForceBase64() should be false")
	}
	if opts.TimestampParam() != "" {
		t.Errorf("Default TimestampParam() = %v, want empty string", opts.TimestampParam())
	}
	if opts.TimestampRequests() {
		t.Error("Default TimestampRequests() should be false")
	}
	if opts.Transports() != nil {
		t.Error("Default Transports() should be nil")
	}
	if opts.TryAllTransports() {
		t.Error("Default TryAllTransports() should be false")
	}
	if opts.RememberUpgrade() {
		t.Error("Default RememberUpgrade() should be false")
	}
	if opts.RequestTimeout() != 0 {
		t.Errorf("Default RequestTimeout() = %v, want 0", opts.RequestTimeout())
	}
	if opts.TransportOptions() != nil {
		t.Error("Default TransportOptions() should be nil")
	}
	if opts.TLSClientConfig() != nil {
		t.Error("Default TLSClientConfig() should be nil")
	}
	if opts.QUICConfig() != nil {
		t.Error("Default QUICConfig() should be nil")
	}
	if opts.ExtraHeaders() != nil {
		t.Error("Default ExtraHeaders() should be nil")
	}
	if opts.WithCredentials() {
		t.Error("Default WithCredentials() should be false")
	}
	if opts.UseNativeTimers() {
		t.Error("Default UseNativeTimers() should be false")
	}
	if opts.AutoUnref() {
		t.Error("Default AutoUnref() should be false")
	}
	if opts.CloseOnBeforeunload() {
		t.Error("Default CloseOnBeforeunload() should be false")
	}
	if opts.PerMessageDeflate() != nil {
		t.Error("Default PerMessageDeflate() should be nil")
	}
	if opts.Path() != "" {
		t.Errorf("Default Path() = %v, want empty string", opts.Path())
	}
	if opts.AddTrailingSlash() {
		t.Error("Default AddTrailingSlash() should be false")
	}
	if len(opts.Protocols()) != 0 {
		t.Errorf("Default Protocols() length = %v, want 0", len(opts.Protocols()))
	}
}
