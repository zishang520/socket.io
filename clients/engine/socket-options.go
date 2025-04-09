package engine

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

// SocketOptionsInterface defines the configuration interface for Socket connections.
// It provides methods to get and set various connection parameters including host,
// port, security settings, and transport options.
type SocketOptionsInterface interface {
	Host() string
	GetRawHost() *string
	SetHost(string)

	Hostname() string
	GetRawHostname() *string
	SetHostname(string)

	Secure() bool
	GetRawSecure() *bool
	SetSecure(bool)

	Port() string
	GetRawPort() *string
	SetPort(string)

	Query() url.Values
	GetRawQuery() url.Values
	SetQuery(url.Values)

	Agent() string
	GetRawAgent() *string
	SetAgent(string)

	Upgrade() bool
	GetRawUpgrade() *bool
	SetUpgrade(bool)

	ForceBase64() bool
	GetRawForceBase64() *bool
	SetForceBase64(bool)

	TimestampParam() string
	GetRawTimestampParam() *string
	SetTimestampParam(string)

	TimestampRequests() bool
	GetRawTimestampRequests() *bool
	SetTimestampRequests(bool)

	Transports() *types.Set[TransportCtor]
	GetRawTransports() *types.Set[TransportCtor]
	SetTransports(*types.Set[TransportCtor])

	TryAllTransports() bool
	GetRawTryAllTransports() *bool
	SetTryAllTransports(bool)

	RememberUpgrade() bool
	GetRawRememberUpgrade() *bool
	SetRememberUpgrade(bool)

	RequestTimeout() time.Duration
	GetRawRequestTimeout() *time.Duration
	SetRequestTimeout(time.Duration)

	TransportOptions() map[string]SocketOptionsInterface
	GetRawTransportOptions() map[string]SocketOptionsInterface
	SetTransportOptions(map[string]SocketOptionsInterface)

	TLSClientConfig() *tls.Config
	GetRawTLSClientConfig() *tls.Config
	SetTLSClientConfig(*tls.Config)

	QUICConfig() *quic.Config
	GetRawQUICConfig() *quic.Config
	SetQUICConfig(*quic.Config)

	ExtraHeaders() http.Header
	GetRawExtraHeaders() http.Header
	SetExtraHeaders(http.Header)

	WithCredentials() bool
	GetRawWithCredentials() *bool
	SetWithCredentials(bool)

	UseNativeTimers() bool
	GetRawUseNativeTimers() *bool
	SetUseNativeTimers(bool)

	AutoUnref() bool
	GetRawAutoUnref() *bool
	SetAutoUnref(bool)

	CloseOnBeforeunload() bool
	GetRawCloseOnBeforeunload() *bool
	SetCloseOnBeforeunload(bool)

	PerMessageDeflate() *types.PerMessageDeflate
	GetRawPerMessageDeflate() *types.PerMessageDeflate
	SetPerMessageDeflate(*types.PerMessageDeflate)

	Path() string
	GetRawPath() *string
	SetPath(string)

	AddTrailingSlash() bool
	GetRawAddTrailingSlash() *bool
	SetAddTrailingSlash(addTrailingSlash bool)

	Protocols() []string
	GetRawProtocols() []string
	SetProtocols([]string)
}

// SocketOptions implements the SocketOptionsInterface and provides the default
// configuration for Socket connections. It contains all the necessary parameters
// to establish and maintain a connection to an Engine.IO server.
type SocketOptions struct {
	// host specifies the server host to connect to. This is extracted from the URI
	// when establishing the connection.
	host *string

	// hostname specifies the server hostname for the connection. This is extracted
	// from the URI when establishing the connection.
	hostname *string

	// secure indicates whether the connection should use HTTPS/WSS. This is determined
	// from the URI scheme (https/wss vs http/ws).
	secure *bool

	// port specifies the server port to connect to. This is extracted from the URI
	// when establishing the connection.
	port *string

	// query contains any query parameters to be included in the connection URI.
	// These parameters are extracted from the URI when establishing the connection.
	query url.Values

	// agent specifies the HTTP agent to use for requests. This is primarily used
	// in Node.js environments and is ignored in browser environments.
	// Note: The type should be "undefined | http.Agent | https.Agent | false",
	// but this would break browser-only clients.
	agent *string

	// upgrade determines whether the client should attempt to upgrade the transport
	// from long-polling to a more efficient transport like WebSocket or WebTransport.
	// Default: true
	upgrade *bool

	// forceBase64 forces base64 encoding for polling transport even when XHR2
	// responseType is available, and for WebSocket even when binary support is available.
	// This can be useful when dealing with legacy systems or specific network requirements.
	forceBase64 *bool

	// timestampParam specifies the parameter name to use for timestamp in requests.
	// This helps prevent caching issues with certain proxies and browsers.
	// Default: 't'
	timestampParam *string

	// timestampRequests determines whether to add a timestamp to each transport request.
	// This is ignored for IE or Android browsers where requests are always stamped.
	// Default: false
	timestampRequests *bool

	// transports specifies the list of transport types to try, in order of preference.
	// The client will attempt to connect using the first transport that passes
	// feature detection.
	// Default: types.NewSet(transports.Polling, transports.WebSocket, transports.WebTransport)
	transports *types.Set[TransportCtor]

	// tryAllTransports determines whether to attempt all available transports
	// if the first one fails. If true, the client will try HTTP long-polling,
	// then WebSocket, and finally WebTransport if previous attempts fail.
	// If false, the client will abort after the first transport fails.
	// Default: false
	tryAllTransports *bool

	// rememberUpgrade enables optimization for SSL/TLS connections by remembering
	// successful WebSocket connections and attempting to use WebSocket directly
	// on subsequent connections.
	// Default: false
	rememberUpgrade *bool

	// requestTimeout specifies the timeout duration for XHR-polling requests.
	// This only affects the polling transport.
	// Default: 0 (no timeout)
	requestTimeout *time.Duration

	// transportOptions contains specific options for each transport type.
	// This allows for fine-grained control over individual transport configurations.
	transportOptions map[string]SocketOptionsInterface

	// tlsClientConfig specifies the TLS configuration for secure connections.
	// If nil, the default configuration is used.
	tlsClientConfig *tls.Config

	// quicConfig specifies the QUIC configuration for WebTransport connections.
	// If nil, the default configuration is used.
	quicConfig *quic.Config

	// extraHeaders specifies additional HTTP headers to be included in all requests.
	// These headers can be used for authentication, custom protocols, or other
	// special requirements.
	extraHeaders http.Header

	// withCredentials determines whether to include credentials (cookies, auth headers,
	// TLS certificates) with cross-origin requests.
	// Note: This is not used in Go implementations as it's browser-specific.
	// Default: false
	withCredentials *bool

	// closeOnBeforeunload determines whether to automatically close the connection
	// when the beforeunload event is received.
	// Default: true
	closeOnBeforeunload *bool

	// useNativeTimers determines whether to use native timeout functions instead of
	// custom implementations. This is useful when working with mock clocks or
	// custom time implementations.
	// Default: false
	useNativeTimers *bool

	// autoUnref determines whether the heartbeat timer should be unref'ed to prevent
	// keeping the Node.js event loop active.
	// Default: false
	autoUnref *bool

	// perMessageDeflate specifies the WebSocket permessage-deflate extension parameters.
	// Set to nil to disable compression.
	// Default: nil
	perMessageDeflate *types.PerMessageDeflate

	// path specifies the path to the Engine.IO endpoint on the server.
	// Default: '/engine.io'
	path *string

	// addTrailingSlash determines whether to append a trailing slash to the request path.
	// Default: true
	addTrailingSlash *bool

	// protocols specifies the WebSocket sub-protocols to use.
	// This allows for protocol negotiation between client and server.
	// Default: []string{}
	protocols []string
}

func DefaultSocketOptions() *SocketOptions {
	return &SocketOptions{}
}

func (s *SocketOptions) Assign(data SocketOptionsInterface) SocketOptionsInterface {
	if data == nil {
		return s
	}

	if data.GetRawHost() != nil {
		s.SetHost(data.Host())
	}
	if data.GetRawHostname() != nil {
		s.SetHostname(data.Hostname())
	}
	if data.GetRawSecure() != nil {
		s.SetSecure(data.Secure())
	}
	if data.GetRawPort() != nil {
		s.SetPort(data.Port())
	}
	if data.GetRawQuery() != nil {
		s.SetQuery(data.Query())
	}
	if data.GetRawAgent() != nil {
		s.SetAgent(data.Agent())
	}
	if data.GetRawUpgrade() != nil {
		s.SetUpgrade(data.Upgrade())
	}
	if data.GetRawForceBase64() != nil {
		s.SetForceBase64(data.ForceBase64())
	}
	if data.GetRawTimestampParam() != nil {
		s.SetTimestampParam(data.TimestampParam())
	}
	if data.GetRawTimestampRequests() != nil {
		s.SetTimestampRequests(data.TimestampRequests())
	}
	if data.GetRawTransports() != nil {
		s.SetTransports(data.Transports())
	}
	if data.GetRawTryAllTransports() != nil {
		s.SetTryAllTransports(data.TryAllTransports())
	}
	if data.GetRawRememberUpgrade() != nil {
		s.SetRememberUpgrade(data.RememberUpgrade())
	}
	if data.GetRawRequestTimeout() != nil {
		s.SetRequestTimeout(data.RequestTimeout())
	}
	if data.GetRawTransportOptions() != nil {
		s.SetTransportOptions(data.TransportOptions())
	}
	if data.GetRawTLSClientConfig() != nil {
		s.SetTLSClientConfig(data.TLSClientConfig())
	}
	if data.GetRawQUICConfig() != nil {
		s.SetQUICConfig(data.QUICConfig())
	}
	if data.GetRawExtraHeaders() != nil {
		s.SetExtraHeaders(data.ExtraHeaders())
	}
	if data.GetRawWithCredentials() != nil {
		s.SetWithCredentials(data.WithCredentials())
	}
	if data.GetRawCloseOnBeforeunload() != nil {
		s.SetCloseOnBeforeunload(data.CloseOnBeforeunload())
	}
	if data.GetRawUseNativeTimers() != nil {
		s.SetUseNativeTimers(data.UseNativeTimers())
	}
	if data.GetRawAutoUnref() != nil {
		s.SetAutoUnref(data.AutoUnref())
	}
	if data.GetRawPerMessageDeflate() != nil {
		s.SetPerMessageDeflate(data.PerMessageDeflate())
	}
	if data.GetRawPath() != nil {
		s.SetPath(data.Path())
	}
	if data.GetRawAddTrailingSlash() != nil {
		s.SetAddTrailingSlash(data.AddTrailingSlash())
	}
	if data.GetRawProtocols() != nil {
		s.SetProtocols(data.Protocols())
	}

	return s
}

func (s *SocketOptions) Host() string {
	if s.host == nil {
		return ""
	}

	return *s.host
}
func (s *SocketOptions) GetRawHost() *string {
	return s.host
}
func (s *SocketOptions) SetHost(host string) {
	s.host = &host
}

func (s *SocketOptions) Hostname() string {
	if s.hostname == nil {
		return ""
	}

	return *s.hostname
}
func (s *SocketOptions) GetRawHostname() *string {
	return s.hostname
}
func (s *SocketOptions) SetHostname(hostname string) {
	s.hostname = &hostname
}

func (s *SocketOptions) Secure() bool {
	if s.secure == nil {
		return false
	}

	return *s.secure
}
func (s *SocketOptions) GetRawSecure() *bool {
	return s.secure
}
func (s *SocketOptions) SetSecure(secure bool) {
	s.secure = &secure
}

func (s *SocketOptions) Port() string {
	if s.port == nil {
		return ""
	}

	return *s.port
}
func (s *SocketOptions) GetRawPort() *string {
	return s.port
}
func (s *SocketOptions) SetPort(port string) {
	s.port = &port
}

func (s *SocketOptions) Query() url.Values {
	return s.query
}
func (s *SocketOptions) GetRawQuery() url.Values {
	return s.query
}
func (s *SocketOptions) SetQuery(query url.Values) {
	s.query = query
}

func (s *SocketOptions) Agent() string {
	if s.agent == nil {
		return ""
	}
	return *s.agent
}
func (s *SocketOptions) GetRawAgent() *string {
	return s.agent
}
func (s *SocketOptions) SetAgent(agent string) {
	s.agent = &agent
}

func (s *SocketOptions) Upgrade() bool {
	if s.upgrade == nil {
		return false
	}

	return *s.upgrade
}
func (s *SocketOptions) GetRawUpgrade() *bool {
	return s.upgrade
}
func (s *SocketOptions) SetUpgrade(upgrade bool) {
	s.upgrade = &upgrade
}

func (s *SocketOptions) ForceBase64() bool {
	if s.forceBase64 == nil {
		return false
	}

	return *s.forceBase64
}
func (s *SocketOptions) GetRawForceBase64() *bool {
	return s.forceBase64
}
func (s *SocketOptions) SetForceBase64(forceBase64 bool) {
	s.forceBase64 = &forceBase64
}

func (s *SocketOptions) TimestampParam() string {
	if s.timestampParam == nil {
		return ""
	}

	return *s.timestampParam
}
func (s *SocketOptions) GetRawTimestampParam() *string {
	return s.timestampParam
}
func (s *SocketOptions) SetTimestampParam(timestampParam string) {
	s.timestampParam = &timestampParam
}

func (s *SocketOptions) TimestampRequests() bool {
	if s.timestampRequests == nil {
		return false
	}

	return *s.timestampRequests
}
func (s *SocketOptions) GetRawTimestampRequests() *bool {
	return s.timestampRequests
}
func (s *SocketOptions) SetTimestampRequests(timestampRequests bool) {
	s.timestampRequests = &timestampRequests
}

func (s *SocketOptions) Transports() *types.Set[TransportCtor] {
	return s.transports
}
func (s *SocketOptions) GetRawTransports() *types.Set[TransportCtor] {
	return s.transports
}
func (s *SocketOptions) SetTransports(transports *types.Set[TransportCtor]) {
	s.transports = transports
}

func (s *SocketOptions) TryAllTransports() bool {
	if s.tryAllTransports == nil {
		return false
	}

	return *s.tryAllTransports
}
func (s *SocketOptions) GetRawTryAllTransports() *bool {
	return s.tryAllTransports
}
func (s *SocketOptions) SetTryAllTransports(tryAllTransports bool) {
	s.tryAllTransports = &tryAllTransports
}

func (s *SocketOptions) RememberUpgrade() bool {
	if s.rememberUpgrade == nil {
		return false
	}

	return *s.rememberUpgrade
}
func (s *SocketOptions) GetRawRememberUpgrade() *bool {
	return s.rememberUpgrade
}
func (s *SocketOptions) SetRememberUpgrade(rememberUpgrade bool) {
	s.rememberUpgrade = &rememberUpgrade
}

func (s *SocketOptions) RequestTimeout() time.Duration {
	if s.requestTimeout == nil {
		return 0
	}

	return *s.requestTimeout
}
func (s *SocketOptions) GetRawRequestTimeout() *time.Duration {
	return s.requestTimeout
}
func (s *SocketOptions) SetRequestTimeout(requestTimeout time.Duration) {
	s.requestTimeout = &requestTimeout
}

func (s *SocketOptions) TransportOptions() map[string]SocketOptionsInterface {
	return s.transportOptions
}
func (s *SocketOptions) GetRawTransportOptions() map[string]SocketOptionsInterface {
	return s.transportOptions
}
func (s *SocketOptions) SetTransportOptions(transportOptions map[string]SocketOptionsInterface) {
	s.transportOptions = transportOptions
}

func (s *SocketOptions) TLSClientConfig() *tls.Config {
	return s.tlsClientConfig
}
func (s *SocketOptions) GetRawTLSClientConfig() *tls.Config {
	return s.tlsClientConfig
}
func (s *SocketOptions) SetTLSClientConfig(tlsClientConfig *tls.Config) {
	s.tlsClientConfig = tlsClientConfig
}

func (s *SocketOptions) QUICConfig() *quic.Config {
	return s.quicConfig
}
func (s *SocketOptions) GetRawQUICConfig() *quic.Config {
	return s.quicConfig
}
func (s *SocketOptions) SetQUICConfig(quicConfig *quic.Config) {
	s.quicConfig = quicConfig
}

func (s *SocketOptions) ExtraHeaders() http.Header {
	return s.extraHeaders
}
func (s *SocketOptions) GetRawExtraHeaders() http.Header {
	return s.extraHeaders
}
func (s *SocketOptions) SetExtraHeaders(extraHeaders http.Header) {
	s.extraHeaders = extraHeaders
}

func (s *SocketOptions) WithCredentials() bool {
	if s.withCredentials == nil {
		return false
	}

	return *s.withCredentials
}
func (s *SocketOptions) GetRawWithCredentials() *bool {
	return s.withCredentials
}
func (s *SocketOptions) SetWithCredentials(withCredentials bool) {
	s.withCredentials = &withCredentials
}

func (s *SocketOptions) UseNativeTimers() bool {
	if s.useNativeTimers == nil {
		return false
	}

	return *s.useNativeTimers
}
func (s *SocketOptions) GetRawUseNativeTimers() *bool {
	return s.useNativeTimers
}
func (s *SocketOptions) SetUseNativeTimers(useNativeTimers bool) {
	s.useNativeTimers = &useNativeTimers
}

func (s *SocketOptions) AutoUnref() bool {
	if s.autoUnref == nil {
		return false
	}

	return *s.autoUnref
}
func (s *SocketOptions) GetRawAutoUnref() *bool {
	return s.autoUnref
}
func (s *SocketOptions) SetAutoUnref(autoUnref bool) {
	s.autoUnref = &autoUnref
}

func (s *SocketOptions) CloseOnBeforeunload() bool {
	if s.closeOnBeforeunload == nil {
		return false
	}

	return *s.closeOnBeforeunload
}
func (s *SocketOptions) GetRawCloseOnBeforeunload() *bool {
	return s.closeOnBeforeunload
}
func (s *SocketOptions) SetCloseOnBeforeunload(closeOnBeforeunload bool) {
	s.closeOnBeforeunload = &closeOnBeforeunload
}

func (s *SocketOptions) PerMessageDeflate() *types.PerMessageDeflate {
	return s.perMessageDeflate
}
func (s *SocketOptions) GetRawPerMessageDeflate() *types.PerMessageDeflate {
	return s.perMessageDeflate
}
func (s *SocketOptions) SetPerMessageDeflate(perMessageDeflate *types.PerMessageDeflate) {
	s.perMessageDeflate = perMessageDeflate
}

func (s *SocketOptions) Path() string {
	if s.path == nil {
		return ""
	}

	return *s.path
}
func (s *SocketOptions) GetRawPath() *string {
	return s.path
}
func (s *SocketOptions) SetPath(path string) {
	s.path = &path
}

func (s *SocketOptions) AddTrailingSlash() bool {
	if s.addTrailingSlash == nil {
		return false
	}
	return *s.addTrailingSlash
}
func (s *SocketOptions) GetRawAddTrailingSlash() *bool {
	return s.addTrailingSlash
}
func (s *SocketOptions) SetAddTrailingSlash(addTrailingSlash bool) {
	s.addTrailingSlash = &addTrailingSlash
}

func (s *SocketOptions) Protocols() []string {
	return s.protocols
}
func (s *SocketOptions) GetRawProtocols() []string {
	return s.protocols
}
func (s *SocketOptions) SetProtocols(protocols []string) {
	s.protocols = protocols
}
