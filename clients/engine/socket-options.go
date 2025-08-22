package engine

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// SocketOptionsInterface defines the configuration interface for Socket connections.
// It provides methods to get and set various connection parameters including host,
// port, security settings, and transport options.
type SocketOptionsInterface interface {
	SetHost(string)
	GetRawHost() types.Optional[string]
	Host() string

	SetHostname(string)
	GetRawHostname() types.Optional[string]
	Hostname() string

	SetSecure(bool)
	GetRawSecure() types.Optional[bool]
	Secure() bool

	SetPort(string)
	GetRawPort() types.Optional[string]
	Port() string

	SetQuery(url.Values)
	GetRawQuery() types.Optional[url.Values]
	Query() url.Values

	SetAgent(string)
	GetRawAgent() types.Optional[string]
	Agent() string

	SetUpgrade(bool)
	GetRawUpgrade() types.Optional[bool]
	Upgrade() bool

	SetForceBase64(bool)
	GetRawForceBase64() types.Optional[bool]
	ForceBase64() bool

	SetTimestampParam(string)
	GetRawTimestampParam() types.Optional[string]
	TimestampParam() string

	SetTimestampRequests(bool)
	GetRawTimestampRequests() types.Optional[bool]
	TimestampRequests() bool

	SetTransports(*types.Set[TransportCtor])
	GetRawTransports() types.Optional[*types.Set[TransportCtor]]
	Transports() *types.Set[TransportCtor]

	SetTryAllTransports(bool)
	GetRawTryAllTransports() types.Optional[bool]
	TryAllTransports() bool

	SetRememberUpgrade(bool)
	GetRawRememberUpgrade() types.Optional[bool]
	RememberUpgrade() bool

	SetRequestTimeout(time.Duration)
	GetRawRequestTimeout() types.Optional[time.Duration]
	RequestTimeout() time.Duration

	SetTransportOptions(map[string]SocketOptionsInterface)
	GetRawTransportOptions() types.Optional[map[string]SocketOptionsInterface]
	TransportOptions() map[string]SocketOptionsInterface

	SetTLSClientConfig(*tls.Config)
	GetRawTLSClientConfig() types.Optional[*tls.Config]
	TLSClientConfig() *tls.Config

	SetQUICConfig(*quic.Config)
	GetRawQUICConfig() types.Optional[*quic.Config]
	QUICConfig() *quic.Config

	SetExtraHeaders(http.Header)
	GetRawExtraHeaders() types.Optional[http.Header]
	ExtraHeaders() http.Header

	SetWithCredentials(bool)
	GetRawWithCredentials() types.Optional[bool]
	WithCredentials() bool

	SetUseNativeTimers(bool)
	GetRawUseNativeTimers() types.Optional[bool]
	UseNativeTimers() bool

	SetAutoUnref(bool)
	GetRawAutoUnref() types.Optional[bool]
	AutoUnref() bool

	SetCloseOnBeforeunload(bool)
	GetRawCloseOnBeforeunload() types.Optional[bool]
	CloseOnBeforeunload() bool

	SetPerMessageDeflate(*types.PerMessageDeflate)
	GetRawPerMessageDeflate() types.Optional[*types.PerMessageDeflate]
	PerMessageDeflate() *types.PerMessageDeflate

	SetPath(string)
	GetRawPath() types.Optional[string]
	Path() string

	SetAddTrailingSlash(addTrailingSlash bool)
	GetRawAddTrailingSlash() types.Optional[bool]
	AddTrailingSlash() bool

	SetProtocols([]string)
	GetRawProtocols() types.Optional[[]string]
	Protocols() []string
}

// SocketOptions implements the SocketOptionsInterface and provides the default
// configuration for Socket connections. It contains all the necessary parameters
// to establish and maintain a connection to an Engine.IO server.
type SocketOptions struct {
	// host specifies the server host to connect to. This is extracted from the URI
	// when establishing the connection.
	host types.Optional[string]

	// hostname specifies the server hostname for the connection. This is extracted
	// from the URI when establishing the connection.
	hostname types.Optional[string]

	// secure indicates whether the connection should use HTTPS/WSS. This is determined
	// from the URI scheme (https/wss vs http/ws).
	secure types.Optional[bool]

	// port specifies the server port to connect to. This is extracted from the URI
	// when establishing the connection.
	port types.Optional[string]

	// query contains any query parameters to be included in the connection URI.
	// These parameters are extracted from the URI when establishing the connection.
	query types.Optional[url.Values]

	// agent specifies the HTTP agent to use for requests. This is primarily used
	// in Node.js environments and is ignored in browser environments.
	agent types.Optional[string]

	// upgrade determines whether the client should attempt to upgrade the transport
	// from long-polling to a more efficient transport like WebSocket or WebTransport.
	upgrade types.Optional[bool]

	// forceBase64 forces base64 encoding for polling transport even when XHR2
	// responseType is available, and for WebSocket even when binary support is available.
	// This can be useful when dealing with legacy systems or specific network requirements.
	forceBase64 types.Optional[bool]

	// timestampParam specifies the parameter name to use for timestamp in requests.
	// This helps prevent caching issues with certain proxies and browsers.
	timestampParam types.Optional[string]

	// timestampRequests determines whether to add a timestamp to each transport request.
	// This is ignored for IE or Android browsers where requests are always stamped.
	timestampRequests types.Optional[bool]

	// transports specifies the list of transport types to try, in order of preference.
	// The client will attempt to connect using the first transport that passes
	// feature detection.
	transports types.Optional[*types.Set[TransportCtor]]

	// tryAllTransports determines whether to attempt all available transports
	// if the first one fails. If true, the client will try HTTP long-polling,
	// then WebSocket, and finally WebTransport if previous attempts fail.
	// If false, the client will abort after the first transport fails.
	tryAllTransports types.Optional[bool]

	// rememberUpgrade enables optimization for SSL/TLS connections by remembering
	// successful WebSocket connections and attempting to use WebSocket directly
	// on subsequent connections.
	rememberUpgrade types.Optional[bool]

	// requestTimeout specifies the timeout duration for XHR-polling requests.
	// This only affects the polling transport.
	requestTimeout types.Optional[time.Duration]

	// transportOptions contains specific options for each transport type.
	// This allows for fine-grained control over individual transport configurations.
	transportOptions types.Optional[map[string]SocketOptionsInterface]

	// tlsClientConfig specifies the TLS configuration for secure connections.
	// If nil, the default configuration is used.
	tlsClientConfig types.Optional[*tls.Config]

	// quicConfig specifies the QUIC configuration for WebTransport connections.
	// If nil, the default configuration is used.
	quicConfig types.Optional[*quic.Config]

	// extraHeaders specifies additional HTTP headers to be included in all requests.
	// These headers can be used for authentication, custom protocols, or other
	// special requirements.
	extraHeaders types.Optional[http.Header]

	// withCredentials determines whether to include credentials (cookies, auth headers,
	// TLS certificates) with cross-origin requests.
	// Note: This is not used in Go implementations as it's browser-specific.
	withCredentials types.Optional[bool]

	// closeOnBeforeunload determines whether to automatically close the connection
	// when the beforeunload event is received.
	closeOnBeforeunload types.Optional[bool]

	// useNativeTimers determines whether to use native timeout functions instead of
	// custom implementations. This is useful when working with mock clocks or
	// custom time implementations.
	useNativeTimers types.Optional[bool]

	// autoUnref determines whether the heartbeat timer should be unref'ed to prevent
	// keeping the Node.js event loop active.
	autoUnref types.Optional[bool]

	// perMessageDeflate specifies the WebSocket permessage-deflate extension parameters.
	// Set to nil to disable compression.
	perMessageDeflate types.Optional[*types.PerMessageDeflate]

	// path specifies the path to the Engine.IO endpoint on the server.
	path types.Optional[string]

	// addTrailingSlash determines whether to append a trailing slash to the request path.
	addTrailingSlash types.Optional[bool]

	// protocols specifies the WebSocket sub-protocols to use.
	// This allows for protocol negotiation between client and server.
	protocols types.Optional[[]string]
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

func (s *SocketOptions) SetHost(host string) {
	s.host = types.NewSome(host)
}
func (s *SocketOptions) GetRawHost() types.Optional[string] {
	return s.host
}
func (s *SocketOptions) Host() string {
	if s.host == nil {
		return ""
	}

	return s.host.Get()
}

func (s *SocketOptions) SetHostname(hostname string) {
	s.hostname = types.NewSome(hostname)
}
func (s *SocketOptions) GetRawHostname() types.Optional[string] {
	return s.hostname
}
func (s *SocketOptions) Hostname() string {
	if s.hostname == nil {
		return ""
	}

	return s.hostname.Get()
}

func (s *SocketOptions) SetSecure(secure bool) {
	s.secure = types.NewSome(secure)
}
func (s *SocketOptions) GetRawSecure() types.Optional[bool] {
	return s.secure
}
func (s *SocketOptions) Secure() bool {
	if s.secure == nil {
		return false
	}

	return s.secure.Get()
}

func (s *SocketOptions) SetPort(port string) {
	s.port = types.NewSome(port)
}
func (s *SocketOptions) GetRawPort() types.Optional[string] {
	return s.port
}
func (s *SocketOptions) Port() string {
	if s.port == nil {
		return ""
	}

	return s.port.Get()
}

func (s *SocketOptions) SetQuery(query url.Values) {
	s.query = types.NewSome(query)
}
func (s *SocketOptions) GetRawQuery() types.Optional[url.Values] {
	return s.query
}
func (s *SocketOptions) Query() url.Values {
	if s.query == nil {
		return nil
	}

	return s.query.Get()
}

func (s *SocketOptions) SetAgent(agent string) {
	s.agent = types.NewSome(agent)
}
func (s *SocketOptions) GetRawAgent() types.Optional[string] {
	return s.agent
}
func (s *SocketOptions) Agent() string {
	if s.agent == nil {
		return ""
	}

	return s.agent.Get()
}

func (s *SocketOptions) SetUpgrade(upgrade bool) {
	s.upgrade = types.NewSome(upgrade)
}
func (s *SocketOptions) GetRawUpgrade() types.Optional[bool] {
	return s.upgrade
}
func (s *SocketOptions) Upgrade() bool {
	if s.upgrade == nil {
		return false
	}

	return s.upgrade.Get()
}

func (s *SocketOptions) SetForceBase64(forceBase64 bool) {
	s.forceBase64 = types.NewSome(forceBase64)
}
func (s *SocketOptions) GetRawForceBase64() types.Optional[bool] {
	return s.forceBase64
}
func (s *SocketOptions) ForceBase64() bool {
	if s.forceBase64 == nil {
		return false
	}

	return s.forceBase64.Get()
}

func (s *SocketOptions) SetTimestampParam(timestampParam string) {
	s.timestampParam = types.NewSome(timestampParam)
}
func (s *SocketOptions) GetRawTimestampParam() types.Optional[string] {
	return s.timestampParam
}
func (s *SocketOptions) TimestampParam() string {
	if s.timestampParam == nil {
		return ""
	}

	return s.timestampParam.Get()
}

func (s *SocketOptions) SetTimestampRequests(timestampRequests bool) {
	s.timestampRequests = types.NewSome(timestampRequests)
}
func (s *SocketOptions) GetRawTimestampRequests() types.Optional[bool] {
	return s.timestampRequests
}
func (s *SocketOptions) TimestampRequests() bool {
	if s.timestampRequests == nil {
		return false
	}

	return s.timestampRequests.Get()
}

func (s *SocketOptions) SetTransports(transports *types.Set[TransportCtor]) {
	s.transports = types.NewSome(transports)
}
func (s *SocketOptions) GetRawTransports() types.Optional[*types.Set[TransportCtor]] {
	return s.transports
}
func (s *SocketOptions) Transports() *types.Set[TransportCtor] {
	if s.transports == nil {
		return nil
	}

	return s.transports.Get()
}

func (s *SocketOptions) SetTryAllTransports(tryAllTransports bool) {
	s.tryAllTransports = types.NewSome(tryAllTransports)
}
func (s *SocketOptions) GetRawTryAllTransports() types.Optional[bool] {
	return s.tryAllTransports
}
func (s *SocketOptions) TryAllTransports() bool {
	if s.tryAllTransports == nil {
		return false
	}

	return s.tryAllTransports.Get()
}

func (s *SocketOptions) SetRememberUpgrade(rememberUpgrade bool) {
	s.rememberUpgrade = types.NewSome(rememberUpgrade)
}
func (s *SocketOptions) GetRawRememberUpgrade() types.Optional[bool] {
	return s.rememberUpgrade
}
func (s *SocketOptions) RememberUpgrade() bool {
	if s.rememberUpgrade == nil {
		return false
	}

	return s.rememberUpgrade.Get()
}

func (s *SocketOptions) SetRequestTimeout(requestTimeout time.Duration) {
	s.requestTimeout = types.NewSome(requestTimeout)
}
func (s *SocketOptions) GetRawRequestTimeout() types.Optional[time.Duration] {
	return s.requestTimeout
}
func (s *SocketOptions) RequestTimeout() time.Duration {
	if s.requestTimeout == nil {
		return 0
	}

	return s.requestTimeout.Get()
}

func (s *SocketOptions) SetTransportOptions(transportOptions map[string]SocketOptionsInterface) {
	s.transportOptions = types.NewSome(transportOptions)
}
func (s *SocketOptions) GetRawTransportOptions() types.Optional[map[string]SocketOptionsInterface] {
	return s.transportOptions
}
func (s *SocketOptions) TransportOptions() map[string]SocketOptionsInterface {
	if s.transportOptions == nil {
		return nil
	}

	return s.transportOptions.Get()
}

func (s *SocketOptions) SetTLSClientConfig(tlsClientConfig *tls.Config) {
	s.tlsClientConfig = types.NewSome(tlsClientConfig)
}
func (s *SocketOptions) GetRawTLSClientConfig() types.Optional[*tls.Config] {
	return s.tlsClientConfig
}
func (s *SocketOptions) TLSClientConfig() *tls.Config {
	if s.tlsClientConfig == nil {
		return nil
	}

	return s.tlsClientConfig.Get()
}

func (s *SocketOptions) SetQUICConfig(quicConfig *quic.Config) {
	s.quicConfig = types.NewSome(quicConfig)
}
func (s *SocketOptions) GetRawQUICConfig() types.Optional[*quic.Config] {
	return s.quicConfig
}
func (s *SocketOptions) QUICConfig() *quic.Config {
	if s.quicConfig == nil {
		return nil
	}

	return s.quicConfig.Get()
}

func (s *SocketOptions) SetExtraHeaders(extraHeaders http.Header) {
	s.extraHeaders = types.NewSome(extraHeaders)
}
func (s *SocketOptions) GetRawExtraHeaders() types.Optional[http.Header] {
	return s.extraHeaders
}
func (s *SocketOptions) ExtraHeaders() http.Header {
	if s.extraHeaders == nil {
		return nil
	}

	return s.extraHeaders.Get()
}

func (s *SocketOptions) SetWithCredentials(withCredentials bool) {
	s.withCredentials = types.NewSome(withCredentials)
}
func (s *SocketOptions) GetRawWithCredentials() types.Optional[bool] {
	return s.withCredentials
}
func (s *SocketOptions) WithCredentials() bool {
	if s.withCredentials == nil {
		return false
	}

	return s.withCredentials.Get()
}

func (s *SocketOptions) SetUseNativeTimers(useNativeTimers bool) {
	s.useNativeTimers = types.NewSome(useNativeTimers)
}
func (s *SocketOptions) GetRawUseNativeTimers() types.Optional[bool] {
	return s.useNativeTimers
}
func (s *SocketOptions) UseNativeTimers() bool {
	if s.useNativeTimers == nil {
		return false
	}

	return s.useNativeTimers.Get()
}

func (s *SocketOptions) SetAutoUnref(autoUnref bool) {
	s.autoUnref = types.NewSome(autoUnref)
}
func (s *SocketOptions) GetRawAutoUnref() types.Optional[bool] {
	return s.autoUnref
}
func (s *SocketOptions) AutoUnref() bool {
	if s.autoUnref == nil {
		return false
	}

	return s.autoUnref.Get()
}

func (s *SocketOptions) SetCloseOnBeforeunload(closeOnBeforeunload bool) {
	s.closeOnBeforeunload = types.NewSome(closeOnBeforeunload)
}
func (s *SocketOptions) GetRawCloseOnBeforeunload() types.Optional[bool] {
	return s.closeOnBeforeunload
}
func (s *SocketOptions) CloseOnBeforeunload() bool {
	if s.closeOnBeforeunload == nil {
		return false
	}

	return s.closeOnBeforeunload.Get()
}

func (s *SocketOptions) SetPerMessageDeflate(perMessageDeflate *types.PerMessageDeflate) {
	s.perMessageDeflate = types.NewSome(perMessageDeflate)
}
func (s *SocketOptions) GetRawPerMessageDeflate() types.Optional[*types.PerMessageDeflate] {
	return s.perMessageDeflate
}
func (s *SocketOptions) PerMessageDeflate() *types.PerMessageDeflate {
	if s.perMessageDeflate == nil {
		return nil
	}

	return s.perMessageDeflate.Get()
}

func (s *SocketOptions) SetPath(path string) {
	s.path = types.NewSome(path)
}
func (s *SocketOptions) GetRawPath() types.Optional[string] {
	return s.path
}
func (s *SocketOptions) Path() string {
	if s.path == nil {
		return ""
	}

	return s.path.Get()
}

func (s *SocketOptions) SetAddTrailingSlash(addTrailingSlash bool) {
	s.addTrailingSlash = types.NewSome(addTrailingSlash)
}
func (s *SocketOptions) GetRawAddTrailingSlash() types.Optional[bool] {
	return s.addTrailingSlash
}
func (s *SocketOptions) AddTrailingSlash() bool {
	if s.addTrailingSlash == nil {
		return false
	}

	return s.addTrailingSlash.Get()
}

func (s *SocketOptions) SetProtocols(protocols []string) {
	s.protocols = types.NewSome(protocols)
}
func (s *SocketOptions) GetRawProtocols() types.Optional[[]string] {
	return s.protocols
}
func (s *SocketOptions) Protocols() []string {
	if s.protocols == nil {
		return nil
	}

	return s.protocols.Get()
}
