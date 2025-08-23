// Package config provides configuration types and options for the Engine.IO server, including server and attach options.
package config

import (
	"io"
	"net/http"
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	AllowRequest func(*types.HttpContext) error

	ServerOptionsInterface interface {
		SetPingTimeout(time.Duration)
		GetRawPingTimeout() types.Optional[time.Duration]
		PingTimeout() time.Duration

		SetPingInterval(time.Duration)
		GetRawPingInterval() types.Optional[time.Duration]
		PingInterval() time.Duration

		SetUpgradeTimeout(time.Duration)
		GetRawUpgradeTimeout() types.Optional[time.Duration]
		UpgradeTimeout() time.Duration

		SetMaxHttpBufferSize(int64)
		GetRawMaxHttpBufferSize() types.Optional[int64]
		MaxHttpBufferSize() int64

		SetAllowRequest(AllowRequest)
		GetRawAllowRequest() types.Optional[AllowRequest]
		AllowRequest() AllowRequest

		SetTransports(*types.Set[transports.TransportCtor])
		GetRawTransports() types.Optional[*types.Set[transports.TransportCtor]]
		Transports() *types.Set[transports.TransportCtor]

		SetAllowUpgrades(bool)
		GetRawAllowUpgrades() types.Optional[bool]
		AllowUpgrades() bool

		SetPerMessageDeflate(*types.PerMessageDeflate)
		GetRawPerMessageDeflate() types.Optional[*types.PerMessageDeflate]
		PerMessageDeflate() *types.PerMessageDeflate

		SetHttpCompression(*types.HttpCompression)
		GetRawHttpCompression() types.Optional[*types.HttpCompression]
		HttpCompression() *types.HttpCompression

		SetInitialPacket(io.Reader)
		GetRawInitialPacket() types.Optional[io.Reader]
		InitialPacket() io.Reader

		SetCookie(*http.Cookie)
		GetRawCookie() types.Optional[*http.Cookie]
		Cookie() *http.Cookie

		SetCors(*types.Cors)
		GetRawCors() types.Optional[*types.Cors]
		Cors() *types.Cors

		SetAllowEIO3(bool)
		GetRawAllowEIO3() types.Optional[bool]
		AllowEIO3() bool
	}

	ServerOptions struct {
		// how many ms without a pong packet to consider the connection closed
		pingTimeout types.Optional[time.Duration]

		// how many ms before sending a new ping packet
		pingInterval types.Optional[time.Duration]

		// how many ms before an uncompleted transport upgrade is cancelled
		upgradeTimeout types.Optional[time.Duration]

		// how many bytes or characters a message can be, before closing the session (to avoid DoS).
		maxHttpBufferSize types.Optional[int64]

		// A function that receives a given handshake or upgrade request as its first parameter,
		// and can decide whether to continue. Returning an error indicates that the request was rejected.
		allowRequest types.Optional[AllowRequest]

		// the low-level transports that are enabled
		transports types.Optional[*types.Set[transports.TransportCtor]]

		// whether to allow transport upgrades
		allowUpgrades types.Optional[bool]

		// parameters of the WebSocket permessage-deflate extension (see ws module api docs). Set to false to disable.
		perMessageDeflate types.Optional[*types.PerMessageDeflate]

		// parameters of the http compression for the polling transports (see zlib api docs). Set to false to disable.
		httpCompression types.Optional[*types.HttpCompression]

		// TODO: Implement pluggable WebSocket engine support.
		// The default engine will be gorilla/websocket. Future engines to support include
		// coder/websocket, gobwas/ws, coder/websocket, etc.
		// Field type: types.Optional[WsEngine]
		// wsEngine types.Optional[WsEngine]

		// an optional packet which will be concatenated to the handshake packet emitted by Engine.IO.
		initialPacket types.Optional[io.Reader]

		// configuration of the cookie that contains the client sid to send as part of handshake response headers. This cookie
		// might be used for sticky-session. Defaults to not sending any cookie.
		cookie types.Optional[*http.Cookie]

		// the options that will be forwarded to the cors module
		cors types.Optional[*types.Cors]

		// whether to enable compatibility with Socket.IO v2 clients
		allowEIO3 types.Optional[bool]
	}
)

func DefaultServerOptions() *ServerOptions {
	return &ServerOptions{}
}

func (s *ServerOptions) Assign(data ServerOptionsInterface) ServerOptionsInterface {
	if data == nil {
		return s
	}

	if data.GetRawPingTimeout() != nil {
		s.SetPingTimeout(data.PingTimeout())
	}
	if data.GetRawPingInterval() != nil {
		s.SetPingInterval(data.PingInterval())
	}
	if data.GetRawUpgradeTimeout() != nil {
		s.SetUpgradeTimeout(data.UpgradeTimeout())
	}
	if data.GetRawMaxHttpBufferSize() != nil {
		s.SetMaxHttpBufferSize(data.MaxHttpBufferSize())
	}
	if data.GetRawAllowRequest() != nil {
		s.SetAllowRequest(data.AllowRequest())
	}
	if data.GetRawTransports() != nil {
		s.SetTransports(data.Transports())
	}
	if data.GetRawAllowUpgrades() != nil {
		s.SetAllowUpgrades(data.AllowUpgrades())
	}
	if data.GetRawPerMessageDeflate() != nil {
		s.SetPerMessageDeflate(data.PerMessageDeflate())
	}
	if data.GetRawHttpCompression() != nil {
		s.SetHttpCompression(data.HttpCompression())
	}
	if data.GetRawInitialPacket() != nil {
		s.SetInitialPacket(data.InitialPacket())
	}
	if data.GetRawCookie() != nil {
		s.SetCookie(data.Cookie())
	}
	if data.GetRawCors() != nil {
		s.SetCors(data.Cors())
	}
	if data.GetRawAllowEIO3() != nil {
		s.SetAllowEIO3(data.AllowEIO3())
	}

	return s
}

// how many ms without a pong packet to consider the connection closed
func (s *ServerOptions) SetPingTimeout(pingTimeout time.Duration) {
	s.pingTimeout = types.NewSome(pingTimeout)
}
func (s *ServerOptions) GetRawPingTimeout() types.Optional[time.Duration] {
	return s.pingTimeout
}
func (s *ServerOptions) PingTimeout() time.Duration {
	if s.pingTimeout == nil {
		return 0
	}

	return s.pingTimeout.Get()
}

// how many ms before sending a new ping packet
func (s *ServerOptions) SetPingInterval(pingInterval time.Duration) {
	s.pingInterval = types.NewSome(pingInterval)
}
func (s *ServerOptions) GetRawPingInterval() types.Optional[time.Duration] {
	return s.pingInterval
}
func (s *ServerOptions) PingInterval() time.Duration {
	if s.pingInterval == nil {
		return 0
	}

	return s.pingInterval.Get()
}

// how many ms before an uncompleted transport upgrade is cancelled
func (s *ServerOptions) SetUpgradeTimeout(upgradeTimeout time.Duration) {
	s.upgradeTimeout = types.NewSome(upgradeTimeout)
}
func (s *ServerOptions) GetRawUpgradeTimeout() types.Optional[time.Duration] {
	return s.upgradeTimeout
}
func (s *ServerOptions) UpgradeTimeout() time.Duration {
	if s.upgradeTimeout == nil {
		return 0
	}

	return s.upgradeTimeout.Get()
}

// how many bytes or characters a message can be, before closing the session (to avoid DoS).
func (s *ServerOptions) SetMaxHttpBufferSize(maxHttpBufferSize int64) {
	s.maxHttpBufferSize = types.NewSome(maxHttpBufferSize)
}
func (s *ServerOptions) GetRawMaxHttpBufferSize() types.Optional[int64] {
	return s.maxHttpBufferSize
}
func (s *ServerOptions) MaxHttpBufferSize() int64 {
	if s.maxHttpBufferSize == nil {
		return 0
	}

	return s.maxHttpBufferSize.Get()
}

// A function that receives a given handshake or upgrade request as its first parameter,
// and can decide whether to continue or not. The second argument is a function that needs
// to be called with the decided information: fn(err, success), where success is a boolean
// value where false means that the request is rejected, and err is an error code.
func (s *ServerOptions) SetAllowRequest(allowRequest AllowRequest) {
	s.allowRequest = types.NewSome(allowRequest)
}
func (s *ServerOptions) GetRawAllowRequest() types.Optional[AllowRequest] {
	return s.allowRequest
}
func (s *ServerOptions) AllowRequest() AllowRequest {
	if s.allowRequest == nil {
		return nil
	}

	return s.allowRequest.Get()
}

// The low-level transports that are enabled. WebTransport is disabled by default and must be manually enabled:
//
//	opts := &ServerOptions{}
//	opts.SetTransports(types.NewSet(engine.Polling, engine.Websocket, engine.WebTransport))
//	NewServer(opts)
func (s *ServerOptions) SetTransports(transports *types.Set[transports.TransportCtor]) {
	s.transports = types.NewSome(transports)
}
func (s *ServerOptions) GetRawTransports() types.Optional[*types.Set[transports.TransportCtor]] {
	return s.transports
}
func (s *ServerOptions) Transports() *types.Set[transports.TransportCtor] {
	if s.transports == nil {
		return nil
	}

	return s.transports.Get()
}

// whether to allow transport upgrades
//
// Default: true
func (s *ServerOptions) SetAllowUpgrades(allowUpgrades bool) {
	s.allowUpgrades = types.NewSome(allowUpgrades)
}
func (s *ServerOptions) GetRawAllowUpgrades() types.Optional[bool] {
	return s.allowUpgrades
}
func (s *ServerOptions) AllowUpgrades() bool {
	if s.allowUpgrades == nil {
		return false
	}

	return s.allowUpgrades.Get()
}

// parameters of the WebSocket permessage-deflate extension (see ws module api docs). Set to false to disable.
func (s *ServerOptions) SetPerMessageDeflate(perMessageDeflate *types.PerMessageDeflate) {
	s.perMessageDeflate = types.NewSome(perMessageDeflate)
}
func (s *ServerOptions) GetRawPerMessageDeflate() types.Optional[*types.PerMessageDeflate] {
	return s.perMessageDeflate
}
func (s *ServerOptions) PerMessageDeflate() *types.PerMessageDeflate {
	if s.perMessageDeflate == nil {
		return nil
	}

	return s.perMessageDeflate.Get()
}

// parameters of the http compression for the polling transports (see zlib api docs). Set to false to disable.
func (s *ServerOptions) SetHttpCompression(httpCompression *types.HttpCompression) {
	s.httpCompression = types.NewSome(httpCompression)
}
func (s *ServerOptions) GetRawHttpCompression() types.Optional[*types.HttpCompression] {
	return s.httpCompression
}
func (s *ServerOptions) HttpCompression() *types.HttpCompression {
	if s.httpCompression == nil {
		return nil
	}

	return s.httpCompression.Get()
}

// an optional packet which will be concatenated to the handshake packet emitted by Engine.IO.
func (s *ServerOptions) SetInitialPacket(initialPacket io.Reader) {
	s.initialPacket = types.NewSome(initialPacket)
}
func (s *ServerOptions) GetRawInitialPacket() types.Optional[io.Reader] {
	return s.initialPacket
}
func (s *ServerOptions) InitialPacket() io.Reader {
	if s.initialPacket == nil {
		return nil
	}

	return s.initialPacket.Get()
}

// configuration of the cookie that contains the client sid to send as part of handshake response headers. This cookie
// might be used for sticky-session. Defaults to not sending any cookie.
func (s *ServerOptions) SetCookie(cookie *http.Cookie) {
	s.cookie = types.NewSome(cookie)
}
func (s *ServerOptions) GetRawCookie() types.Optional[*http.Cookie] {
	return s.cookie
}
func (s *ServerOptions) Cookie() *http.Cookie {
	if s.cookie == nil {
		return nil
	}

	return s.cookie.Get()
}

// the options that will be forwarded to the cors module
func (s *ServerOptions) SetCors(cors *types.Cors) {
	s.cors = types.NewSome(cors)
}
func (s *ServerOptions) GetRawCors() types.Optional[*types.Cors] {
	return s.cors
}
func (s *ServerOptions) Cors() *types.Cors {
	if s.cors == nil {
		return nil
	}

	return s.cors.Get()
}

// whether to enable compatibility with Socket.IO v2 clients
func (s *ServerOptions) SetAllowEIO3(allowEIO3 bool) {
	s.allowEIO3 = types.NewSome(allowEIO3)
}
func (s *ServerOptions) GetRawAllowEIO3() types.Optional[bool] {
	return s.allowEIO3
}
func (s *ServerOptions) AllowEIO3() bool {
	if s.allowEIO3 == nil {
		return false
	}

	return s.allowEIO3.Get()
}
