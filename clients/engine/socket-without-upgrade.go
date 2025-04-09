package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/events"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/servers/engine/v3/types"
	"github.com/zishang520/socket.io/servers/engine/v3/utils"
)

// SocketWithoutUpgrade provides a WebSocket-like interface to connect to an Engine.IO server.
// This implementation maintains a single transport throughout the connection lifecycle without
// attempting to upgrade to a different transport type.
//
// Key features:
//   - Maintains a persistent connection using the initial transport
//   - Supports various transport types (HTTP long-polling, WebSocket, WebTransport)
//   - Handles packet buffering and flushing
//   - Manages connection state and heartbeat
//   - Provides event-based communication
//
// Example usage:
//
//	import (
//		"github.com/zishang520/socket.io/clients/engine/v3"
//		"github.com/zishang520/socket.io/clients/engine/v3/transports"
//		"github.com/zishang520/socket.io/servers/engine/v3/types"
//	)
//
//	func main() {
//		opts := engine.DefaultSocketOptions()
//		opts.SetTransports(types.NewSet(transports.Polling, transports.WebSocket, transports.WebTransport))
//		socket := engine.NewSocket("http://localhost:8080", opts)
//		socket.On("open", func(...any) {
//			socket.Send("hello")
//		})
//	}
//
// See: [SocketWithUpgrade]
// See: [Socket]
type socketWithoutUpgrade struct {
	types.EventEmitter

	// _proto_ is used for interface method rewriting and prototype pattern implementation
	_proto_ SocketWithoutUpgrade

	// Public fields
	id          atomic.Value                 // Unique session identifier
	transport   atomic.Pointer[Transport]    // Current transport instance
	readyState  atomic.Value                 // Current connection state
	writeBuffer *types.Slice[*packet.Packet] // Buffer for outgoing packets

	// Protected fields (read-only after initialization)
	opts       *SocketOptions     // Connection options
	transports *types.Set[string] // Available transport types
	upgrading  atomic.Bool        // Upgrade status flag

	// Private fields
	_prevBufferLen             atomic.Uint32               // Previous buffer length for write operations
	_pingInterval              int64                       // Interval between ping messages
	_pingTimeout               int64                       // Timeout for ping responses
	_maxPayload                int64                       // Maximum payload size allowed
	_pingTimeoutTimer          atomic.Pointer[utils.Timer] // Timer for ping timeout
	_pingTimeoutTime           atomic.Value                // Timestamp for ping timeout expiration
	_beforeunloadEventListener types.Listener              // Event listener for page unload
	_offlineEventListener      types.Listener              // Event listener for offline status

	// Connection configuration (read-only)
	secure            bool                     // Whether to use secure connection
	hostname          string                   // Server hostname
	port              string                   // Server port
	_transportsByName map[string]TransportCtor // Transport constructors by name
	_cookieJar        http.CookieJar           // Cookie storage for HTTP requests

	// Static fields
	priorWebsocketSuccess atomic.Bool // Previous WebSocket connection success flag
	protocol              int         // Engine.IO protocol version

	flushMu sync.Mutex
}

// Prototype sets the prototype for method rewriting.
func (s *socketWithoutUpgrade) Prototype(_proto_ SocketWithoutUpgrade) {
	s._proto_ = _proto_
}

// Proto returns the current prototype instance.
func (s *socketWithoutUpgrade) Proto() SocketWithoutUpgrade {
	return s._proto_
}

// Id returns the unique session identifier.
func (s *socketWithoutUpgrade) Id() string {
	if id := s.id.Load(); id != nil {
		return id.(string)
	}
	return ""
}

// Transport returns the current transport instance.
func (s *socketWithoutUpgrade) Transport() Transport {
	if transport := s.transport.Load(); transport != nil {
		return *transport
	}
	return nil
}

// ReadyState returns the current connection state.
func (s *socketWithoutUpgrade) ReadyState() SocketState {
	if readyState := s.readyState.Load(); readyState != nil {
		return readyState.(SocketState)
	}
	return ""
}

// WriteBuffer returns the buffer for outgoing packets.
func (s *socketWithoutUpgrade) WriteBuffer() *types.Slice[*packet.Packet] {
	return s.writeBuffer
}

// Opts returns the socket options interface.
func (s *socketWithoutUpgrade) Opts() SocketOptionsInterface {
	return s.opts
}

// Transports returns the set of available transport types.
func (s *socketWithoutUpgrade) Transports() *types.Set[string] {
	return s.transports
}

// SetUpgrading sets the upgrading status flag.
func (s *socketWithoutUpgrade) SetUpgrading(upgrading bool) {
	s.upgrading.Store(upgrading)
}

// Upgrading returns the current upgrading status.
func (s *socketWithoutUpgrade) Upgrading() bool {
	return s.upgrading.Load()
}

// CookieJar returns the HTTP cookie jar for the socket.
func (s *socketWithoutUpgrade) CookieJar() http.CookieJar {
	return s._cookieJar
}

// SetPriorWebsocketSuccess sets the flag indicating previous WebSocket connection success.
func (s *socketWithoutUpgrade) SetPriorWebsocketSuccess(priorWebsocketSuccess bool) {
	s.priorWebsocketSuccess.Store(priorWebsocketSuccess)
}

// PriorWebsocketSuccess returns whether previous WebSocket connection was successful.
func (s *socketWithoutUpgrade) PriorWebsocketSuccess() bool {
	return s.priorWebsocketSuccess.Load()
}

// Protocol returns the Engine.IO protocol version.
func (s *socketWithoutUpgrade) Protocol() int {
	return parser.Protocol
}

// MakeSocketWithoutUpgrade creates a new socketWithoutUpgrade instance with default settings.
// It initializes the basic socket structure without establishing a connection.
func MakeSocketWithoutUpgrade() SocketWithoutUpgrade {
	s := &socketWithoutUpgrade{
		EventEmitter: types.NewEventEmitter(),
		writeBuffer:  &types.Slice[*packet.Packet]{},

		_pingInterval: -1,
		_pingTimeout:  -1,
		_maxPayload:   -1,
	}

	s._pingTimeoutTime.Store(math.Inf(1))
	s.protocol = parser.Protocol
	s.Prototype(s)

	return s
}

// NewSocketWithoutUpgrade creates and initializes a new socket connection.
// It takes a URI string and optional socket options to configure the connection.
func NewSocketWithoutUpgrade(uri string, opts SocketOptionsInterface) SocketWithoutUpgrade {
	s := MakeSocketWithoutUpgrade()
	s.Construct(uri, opts)
	return s
}

// Construct initializes the socket with the given URI and options.
// It parses the URI, sets up transport options, and establishes the initial connection.
func (s *socketWithoutUpgrade) Construct(uri string, opts SocketOptionsInterface) {
	if opts == nil {
		opts = DefaultSocketOptions()
	}
	if uri != "" {
		if parsedURI, err := url.Parse(uri); err == nil {
			opts.SetHostname(parsedURI.Hostname())
			opts.SetSecure(parsedURI.Scheme == "https" || parsedURI.Scheme == "wss")
			opts.SetPort(parsedURI.Port())
			if parsedURI.RawQuery != "" {
				opts.SetQuery(parsedURI.Query())
			}
		} else {
			client_socket_log.Error("Invalid URL address: %v", err)
		}
	} else if opts.GetRawHost() != nil {
		if parsedURI, err := url.Parse(opts.Host()); err == nil {
			opts.SetHostname(parsedURI.Hostname())
		}
	}

	s.secure = opts.Secure()

	if opts.GetRawHostname() != nil && opts.GetRawPort() == nil {
		// if no port is specified manually, use the protocol default
		if s.secure {
			opts.SetPort("443")
		} else {
			opts.SetPort("80")
		}
	}

	if opts.GetRawHostname() != nil {
		s.hostname = opts.Hostname()
	} else {
		s.hostname = "localhost"
	}

	if opts.GetRawPort() != nil {
		s.port = opts.Port()
	} else {
		if s.secure {
			s.port = "443"
		} else {
			s.port = "80"
		}
	}

	s.transports = types.NewSet[string]()
	s._transportsByName = map[string]TransportCtor{}
	if transports := opts.Transports(); transports != nil {
		for _, transport := range transports.Keys() {
			transportName := transport.Name()
			s.transports.Add(transportName)
			s._transportsByName[transportName] = transport
		}
	}

	s.opts = DefaultSocketOptions()
	s.opts.SetPath("/engine.io")
	s.opts.SetAgent("")
	s.opts.SetWithCredentials(false)
	s.opts.SetUpgrade(true)
	s.opts.SetTimestampParam("t")
	s.opts.SetRememberUpgrade(false)
	s.opts.SetAddTrailingSlash(true)
	s.opts.SetPerMessageDeflate(&types.PerMessageDeflate{
		Threshold: 1024,
	})
	s.opts.SetTransportOptions(map[string]SocketOptionsInterface{})
	s.opts.SetCloseOnBeforeunload(false)

	s.opts.Assign(opts)

	path := strings.TrimRight(s.opts.Path(), "/")
	if s.opts.AddTrailingSlash() {
		path += "/"
	}

	s.opts.SetPath(path)

	if s.opts.CloseOnBeforeunload() {
		s._beforeunloadEventListener = func(...any) {
			if transport := s.Transport(); transport != nil {
				transport.Clear()
				transport.Close()
			}
		}

		events.Once(EventBeforeUnload, s._beforeunloadEventListener)

		if s.hostname != "localhost" {
			client_socket_log.Debug("adding listener for the 'offline' event")
			s._offlineEventListener = func(...any) {
				s._onClose("transport close", errors.New("network connection lost"))
			}
			events.Once(EventOffline, s._offlineEventListener)
		}
	}

	if s.opts.WithCredentials() {
		if jar, err := cookiejar.New(nil); err == nil {
			s._cookieJar = jar
		}
	}

	s._open()
}

// CreateTransport initializes a new transport instance with the specified name.
// It sets up the necessary query parameters and configuration for the transport.
func (s *socketWithoutUpgrade) CreateTransport(name string) Transport {
	client_socket_log.Debug(`creating transport "%s"`, name)

	query := url.Values{}

	for k, vs := range s.opts.Query() {
		for _, v := range vs {
			query.Add(k, v)
		}
	}

	// append engine.io protocol identifier
	query.Set("EIO", strconv.FormatInt(int64(s.protocol), 10))

	// transport name
	query.Set("transport", name)

	// session id if we already have one
	if id := s.Id(); id != "" {
		query.Set("sid", id)
	}

	opts := DefaultSocketOptions()
	opts.Assign(s.opts)
	opts.SetQuery(query)
	opts.SetHostname(s.hostname)
	opts.SetSecure(s.secure)
	opts.SetPort(s.port)
	if transportOptions := s.opts.TransportOptions(); transportOptions != nil {
		if topts, ok := transportOptions[name]; ok {
			opts.Assign(topts)
		}
	}

	client_socket_log.Debug(`options "%v"`, opts)

	return s._transportsByName[name].New(s._proto_, opts)
}

// _open initializes the connection by selecting and creating the appropriate transport.
// It handles transport selection based on configuration and previous connection history.
func (s *socketWithoutUpgrade) _open() {
	if s.transports.Len() == 0 {
		// Emit error on next tick so it can be listened to
		go s.Emit("error", errors.New("No transports available"))
		return
	}
	transportName := s.transports.Keys()[0]
	if s.opts.RememberUpgrade() && s.PriorWebsocketSuccess() && s.transports.Has(transports.WEBSOCKET) {
		transportName = transports.WEBSOCKET
	}
	s.readyState.Store(SocketStateOpening)

	transport := s._proto_.CreateTransport(transportName)
	s._proto_.SetTransport(transport)

	transport.Open()
}

// SetTransport configures the current transport and sets up event listeners.
// It ensures proper cleanup of any existing transport before setting up the new one.
func (s *socketWithoutUpgrade) SetTransport(transport Transport) {
	client_socket_log.Debug("setting transport %s", transport.Name())

	if transport := s.Transport(); transport != nil {
		client_socket_log.Debug("clearing existing transport %s", transport.Name())
		transport.Clear()
	}

	// set up transport
	s.transport.Store(&transport)

	// set up transport listeners
	transport.On("drain", func(...any) { s._onDrain() })
	transport.On("packet", func(packets ...any) {
		if len(packets) > 0 {
			s._onPacket(packets[0].(*packet.Packet))
		}
	})
	transport.On("error", func(err ...any) { s._onError(err[0].(error)) })
	transport.On("close", func(reason ...any) { s._onClose("transport close", reason[0].(error)) })
}

// OnOpen is called when the connection is successfully established.
// It updates the connection state and triggers necessary initialization.
func (s *socketWithoutUpgrade) OnOpen() {
	client_socket_log.Debug("socket open")
	s.readyState.Store(SocketStateOpen)
	s.SetPriorWebsocketSuccess(transports.WEBSOCKET == s.Transport().Name())
	s.Emit("open")
	s._proto_.Flush()
}

// _onPacket handles incoming packets from the transport.
// It processes different packet types and triggers appropriate events.
func (s *socketWithoutUpgrade) _onPacket(data *packet.Packet) {
	if readyState := s.ReadyState(); SocketStateOpening == readyState || SocketStateOpen == readyState || SocketStateClosing == readyState {
		client_socket_log.Debug(`socket receive: type "%s", data "%v"`, data.Type, data.Data)

		s.Emit("packet", data)

		// Socket is live - any packet counts
		s.Emit("heartbeat")

		switch data.Type {
		case packet.OPEN:
			if data.Data == nil {
				s._onError(errors.New("data must not be nil"))
				return
			}
			var handshake *HandshakeData
			if err := json.NewDecoder(data.Data).Decode(&handshake); err != nil {
				s._onError(err)
				return
			}
			if handshake == nil {
				s._onError(errors.New("decode error"))
				return
			}
			s._proto_.OnHandshake(handshake)
		case packet.PING:
			s._sendPacket(packet.PONG, nil, nil, nil)
			s.Emit("ping")
			s.Emit("pong")
			s._resetPingTimeout()
		case packet.ERROR:
			s._onError(fmt.Errorf("server error: %v", data.Data))
		case packet.MESSAGE:
			s.Emit("data", data.Data)
			s.Emit("message", data.Data)
		}
	} else {
		client_socket_log.Debug(`packet received with socket readyState "%s"`, readyState)
	}
}

// OnHandshake processes the handshake data received from the server.
// It initializes connection parameters and starts the heartbeat mechanism.
func (s *socketWithoutUpgrade) OnHandshake(data *HandshakeData) {
	s.Emit("handshake", data)
	s.id.Store(data.Sid)
	s.Transport().Query().Set("sid", data.Sid)
	s._pingInterval = data.PingInterval
	s._pingTimeout = data.PingTimeout
	s._maxPayload = data.MaxPayload
	s._proto_.OnOpen()
	// In case open handler closes socket
	if SocketStateClosed == s.ReadyState() {
		return
	}
	s._resetPingTimeout()
}

// _resetPingTimeout manages the ping timeout timer to ensure connection health.
// It handles timer cleanup and sets up new timeout periods based on server configuration.
func (s *socketWithoutUpgrade) _resetPingTimeout() {
	utils.ClearTimeout(s._pingTimeoutTimer.Load())
	delay := s._pingInterval + s._pingTimeout
	s._pingTimeoutTime.Store(float64(time.Now().UnixMilli() + delay))
	s._pingTimeoutTimer.Store(utils.SetTimeout(func() {
		s._onClose("ping timeout", nil)
	}, time.Duration(delay)*time.Millisecond))
	if s.opts.AutoUnref() {
		s._pingTimeoutTimer.Load().Unref()
	}
}

// _onDrain handles the drain event from the transport.
// It manages the write buffer and triggers appropriate events when the buffer is cleared.
func (s *socketWithoutUpgrade) _onDrain() {
	if s.writeBuffer.Len() == 0 {
		s.Emit("drain")
	} else {
		s._proto_.Flush()
	}
}

// Flush sends buffered packets to the transport.
// It ensures packets are sent within payload size limits and handles transport state.
func (s *socketWithoutUpgrade) Flush() {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	if SocketStateClosed != s.ReadyState() && s.Transport().Writable() && !s.Upgrading() {
		if packets := s._getWritablePackets(); len(packets) > 0 {
			client_socket_log.Debug("flushing %d packets in socket", len(packets))
			s.Transport().Send(packets)
			s.Emit("flush")
		}
	}
}

// _getWritablePackets prepares packets for sending while respecting payload size limits.
// It handles packet encoding and size calculation for different transport types.
func (s *socketWithoutUpgrade) _getWritablePackets() (res []*packet.Packet) {
	if !(s._maxPayload != 0 && s.Transport().Name() == transports.POLLING && s.writeBuffer.Len() > 1) {
		return s.writeBuffer.AllAndClear()
	}

	payloadSize := int64(1) // first packet type
	if datas, _ := s.writeBuffer.RangeAndSplice(func(packet *packet.Packet, i int) (bool, int, int, []*packet.Packet) {
		if packet.Data != nil {
			switch v := packet.Data.(type) {
			case *types.StringBuffer:
				payloadSize += int64(v.Len())
			case *strings.Reader:
				payloadSize += int64(v.Len())
			case interface{ Len() int }:
				payloadSize += int64(math.Ceil(float64(v.Len()) * BASE64_OVERHEAD))
			default:
				snapshot, _ := types.NewBytesBufferReader(v)
				payloadSize += int64(math.Ceil(float64(snapshot.Len()) * BASE64_OVERHEAD))
				packet.Data = snapshot
			}
			if i > 0 && payloadSize > s._maxPayload {
				client_socket_log.Debug("only send %d out of %d packets", i, payloadSize)
				return true, 0, i, nil
			}
			payloadSize += 2 // separator + packet type
		}
		return false, 0, i, nil
	}, false); len(datas) > 0 {
		return datas
	}

	client_socket_log.Debug("payload size is %d (max: %d)", payloadSize, s._maxPayload)
	return s.writeBuffer.AllAndClear()
}

// HasPingExpired checks if the connection has timed out due to missed heartbeats.
// It handles timer throttling and connection cleanup for timeout scenarios.
func (s *socketWithoutUpgrade) HasPingExpired() bool {
	if s._pingTimeoutTime.Load().(float64) == 0 {
		return true
	}
	hasExpired := float64(time.Now().UnixMilli()) > s._pingTimeoutTime.Load().(float64)
	if hasExpired {
		client_socket_log.Debug("throttled timer detected, scheduling connection close")
		s._pingTimeoutTime.Store(0)

		go s._onClose("ping timeout", nil)
	}

	return hasExpired
}

// Write sends a message through the socket.
// It buffers the message and triggers a flush operation.
func (s *socketWithoutUpgrade) Write(msg io.Reader, options *packet.Options, fn func()) SocketWithoutUpgrade {
	s._sendPacket(packet.MESSAGE, msg, options, fn)
	return s
}

// Send is an alias for Write, providing the same functionality.
func (s *socketWithoutUpgrade) Send(msg io.Reader, options *packet.Options, fn func()) SocketWithoutUpgrade {
	s._sendPacket(packet.MESSAGE, msg, options, fn)
	return s
}

// _sendPacket handles the internal packet sending logic.
// It manages packet buffering and callback handling.
func (s *socketWithoutUpgrade) _sendPacket(_type packet.Type, data io.Reader, options *packet.Options, fn func()) {
	if readyState := s.ReadyState(); SocketStateClosing == readyState || SocketStateClosed == readyState {
		return
	}
	packet := &packet.Packet{
		Type:    _type,
		Data:    data,
		Options: options,
	}
	s.Emit("packetCreate", packet)

	s.writeBuffer.Push(packet)

	if fn != nil {
		s.Once("flush", func(...any) {
			fn()
		})
	}
	s._proto_.Flush()
}

// Close gracefully closes the socket connection.
// It handles cleanup of resources and ensures proper transport closure.
func (s *socketWithoutUpgrade) Close() SocketWithoutUpgrade {
	close := func() {
		s._onClose("forced close", nil)
		client_socket_log.Debug("socket closing - telling transport to close")
		s.Transport().Close()
	}

	var cleanupAndClose types.Listener
	cleanupAndClose = func(...any) {
		s.RemoveListener("upgrade", cleanupAndClose)
		s.RemoveListener("upgradeError", cleanupAndClose)
		close()
	}

	waitForUpgrade := func() {
		// wait for upgrade to finish since we can't send packets while pausing a transport
		s.Once("upgrade", cleanupAndClose)
		s.Once("upgradeError", cleanupAndClose)
	}

	if readyState := s.ReadyState(); SocketStateOpening == readyState || SocketStateOpen == readyState {
		s.readyState.Store(SocketStateClosing)
		if s.writeBuffer.Len() > 0 {
			s.Once("drain", func(...any) {
				if s.Upgrading() {
					waitForUpgrade()
				} else {
					close()
				}
			})
		} else if s.Upgrading() {
			waitForUpgrade()
		} else {
			close()
		}
	}

	return s
}

// _onError handles transport errors and connection failures.
// It manages transport fallback and error reporting.
func (s *socketWithoutUpgrade) _onError(err error) {
	client_socket_log.Debug("socket error %v", err)
	s.SetPriorWebsocketSuccess(false)

	if s.opts.TryAllTransports() && s.transports.Len() > 1 && s.ReadyState() == SocketStateOpening {
		client_socket_log.Debug("trying next transport")
		s.transports.Delete(s.transports.Keys()[0])
		s._open()
		return
	}

	s.Emit("error", err)
	s._onClose("transport error", err)
}

// _onClose handles the connection closure process.
// It performs cleanup operations and notifies listeners of the closure.
func (s *socketWithoutUpgrade) _onClose(reason string, description error) {
	if readyState := s.ReadyState(); SocketStateOpening == readyState || SocketStateOpen == readyState || SocketStateClosing == readyState {
		client_socket_log.Debug(`socket close with reason: "%s"`, reason)

		// clear timers
		utils.ClearTimeout(s._pingTimeoutTimer.Load())

		if transport := s.Transport(); transport != nil {
			// stop event from firing again for transport
			transport.RemoveAllListeners("close")

			// ensure transport won't stay open
			transport.Close()

			// ignore further transport communication
			transport.Clear()
		}

		if s._beforeunloadEventListener != nil {
			events.RemoveListener(EventBeforeUnload, s._beforeunloadEventListener)
		}

		if s._offlineEventListener != nil {
			events.RemoveListener(EventOffline, s._offlineEventListener)
		}

		// set ready state
		s.readyState.Store(SocketStateClosed)

		// clear session id
		s.id.Store("")

		// emit close event
		s.Emit("close", reason, description)

		// clean buffers after, so users can still
		// grab the buffers on `close` event
		s.writeBuffer.Clear()
	}
}
