// Package engine implements the Engine.IO socket, which manages client connections, transport upgrades, and protocol state.
package engine

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/errors"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var socket_log = log.NewLog("engine:socket")

type socket struct {
	types.EventEmitter

	// The revision of the protocol:
	//
	// - 3rd is used in Engine.IO v3 / Socket.IO v2
	// - 4th is used in Engine.IO v4 and above / Socket.IO v3 and above
	//
	// It is found in the `EIO` query parameters of the HTTP requests.
	//
	// See: https://github.com/socketio/engine.io-protocol
	protocol int
	// A reference to the first HTTP request of the session
	//
	// TODO for the next major release: remove it
	request *types.HttpContext
	// The IP address of the client.
	remoteAddress string

	// The current state of the socket.
	readyState atomic.Value
	// The current low-level transport.
	transport atomic.Pointer[transports.Transport]

	// This is the session identifier that the client will use in the subsequent HTTP requests. It must not be shared with
	// others parties, as it might lead to session hijacking.
	id                string
	server            BaseServer
	upgrading         atomic.Bool
	upgraded          atomic.Bool
	writeBuffer       *types.Slice[*packet.Packet]
	packetsFn         *types.Slice[SendCallback]
	sentCallbackFn    *types.Slice[[]SendCallback]
	cleanupFn         *types.Slice[types.Callable]
	pingTimeoutTimer  atomic.Pointer[utils.Timer]
	pingIntervalTimer atomic.Pointer[utils.Timer]

	flushMu sync.Mutex
}

func (s *socket) Protocol() int {
	return s.protocol
}

func (s *socket) Upgraded() bool {
	return s.upgraded.Load()
}

func (s *socket) Upgrading() bool {
	return s.upgrading.Load()
}

func (s *socket) Id() string {
	return s.id
}

func (s *socket) RemoteAddress() string {
	return s.remoteAddress
}

func (s *socket) Request() *types.HttpContext {
	return s.request
}

func (s *socket) Transport() transports.Transport {
	if v := s.transport.Load(); v != nil {
		return *v
	}
	return nil
}

func (s *socket) Server() BaseServer {
	return s.server
}

func (s *socket) ReadyState() string {
	if v, ok := s.readyState.Load().(string); ok {
		return v
	}
	return ""
}

func (s *socket) SetReadyState(state string) {
	socket_log.Debug("readyState updated from %s to %s", s.ReadyState(), state)

	s.readyState.Store(state)
}

// Client class.
func MakeSocket() Socket {
	s := &socket{
		EventEmitter: types.NewEventEmitter(),

		writeBuffer:    types.NewSlice[*packet.Packet](),
		packetsFn:      types.NewSlice[SendCallback](),
		sentCallbackFn: types.NewSlice[[]SendCallback](),
		cleanupFn:      types.NewSlice[types.Callable](),
	}
	s.readyState.Store("opening")

	return s
}

// Client class.
func NewSocket(id string, server BaseServer, transport transports.Transport, ctx *types.HttpContext, protocol int) Socket {
	s := MakeSocket()

	s.Construct(id, server, transport, ctx, protocol)

	return s
}

func (s *socket) Construct(id string, server BaseServer, transport transports.Transport, ctx *types.HttpContext, protocol int) {
	s.id = id
	s.server = server
	s.request = ctx
	s.protocol = protocol

	// Cache IP since it might not be in the req later
	if ctx.WebTransport != nil {
		s.remoteAddress = ctx.WebTransport.RemoteAddr().String()
	} else if ctx.Websocket != nil && ctx.Websocket.Conn != nil {
		s.remoteAddress = ctx.Websocket.RemoteAddr().String()
	} else {
		s.remoteAddress = ctx.Request().RemoteAddr
	}

	s.setTransport(transport)
	s.onOpen()
}

// Called upon transport considered open.
func (s *socket) onOpen() {
	s.SetReadyState("open")

	// sends an `open` packet
	s.Transport().SetSid(s.id)

	data, err := json.Marshal(map[string]any{
		"sid":          s.id,
		"upgrades":     s.getAvailableUpgrades(),
		"pingInterval": int64(s.server.Opts().PingInterval() / time.Millisecond),
		"pingTimeout":  int64(s.server.Opts().PingTimeout() / time.Millisecond),
		"maxPayload":   s.server.Opts().MaxHttpBufferSize(),
	})

	if err != nil {
		socket_log.Debug("json.Marshal err")
	}
	s.sendPacket(
		packet.OPEN,
		types.NewStringBuffer(data),
		nil, nil,
	)

	if i := s.server.Opts().InitialPacket(); i != nil {
		s.sendPacket(packet.MESSAGE, i, nil, nil)
	}

	s.Emit("open")

	if s.protocol == 3 {
		// in protocol v3, the client sends a ping, and the server answers with a pong
		s.resetPingTimeout()
	} else {
		// in protocol v4, the server sends a ping, and the client answers with a pong
		s.schedulePing()
	}
}

// Called upon transport packet.
func (s *socket) onPacket(data *packet.Packet) {
	if s.ReadyState() != "open" {
		socket_log.Debug("packet received with closed socket")
		return
	}

	// export packet event
	socket_log.Debug(`received packet %s`, data.Type)
	s.Emit("packet", data)

	switch data.Type {
	case packet.PING:
		if s.Transport().Protocol() != 3 {
			s.onError(errors.ErrInvalidHeartbeat)
			return
		}
		socket_log.Debug("got ping")
		s.pingTimeoutTimer.Load().Refresh()
		s.sendPacket(packet.PONG, nil, nil, nil)
		s.Emit("heartbeat")
	case packet.PONG:
		if s.Transport().Protocol() == 3 {
			s.onError(errors.ErrInvalidHeartbeat)
			return
		}
		socket_log.Debug("got pong")
		utils.ClearTimeout(s.pingTimeoutTimer.Load())
		s.pingIntervalTimer.Load().Refresh()
		s.Emit("heartbeat")
	case packet.ERROR:
		s.OnClose("parse error")
	case packet.MESSAGE:
		s.Emit("data", data.Data)
		s.Emit("message", data.Data)
	}
}

// Called upon transport error.
func (s *socket) onError(err error) {
	socket_log.Debug("transport error %v", err)
	s.OnClose("transport error", err)
}

// Pings client every `this.pingInterval` and expects response
// within `this.pingTimeout` or closes connection.
func (s *socket) schedulePing() {
	s.pingIntervalTimer.Store(utils.SetTimeout(func() {
		socket_log.Debug("writing ping packet - expecting pong within %dms", int64(s.server.Opts().PingTimeout()/time.Millisecond))
		s.sendPacket(packet.PING, nil, nil, nil)
		s.resetPingTimeout()
	}, s.server.Opts().PingInterval()))
}

// Resets ping timeout.
func (s *socket) resetPingTimeout() {
	utils.ClearTimeout(s.pingTimeoutTimer.Load())
	s.pingTimeoutTimer.Store(utils.SetTimeout(func() {
		if s.ReadyState() == "closed" {
			return
		}
		s.OnClose("ping timeout")
	}, s.resetPingTimeoutDuration()))
}
func (s *socket) resetPingTimeoutDuration() time.Duration {
	if s.protocol == 3 {
		return s.server.Opts().PingInterval() + s.server.Opts().PingTimeout()
	}
	return s.server.Opts().PingTimeout()
}

// Attaches handlers for the given transport.
func (s *socket) setTransport(transport transports.Transport) {
	onError := func(err ...any) {
		s.onError(err[0].(error))
	}
	onReady := func(...any) { s.flush() }
	onPacket := func(packets ...any) {
		if len(packets) > 0 {
			s.onPacket(packets[0].(*packet.Packet))
		}
	}
	onDrain := func(...any) { s.onDrain() }
	onClose := func(...any) { s.OnClose("transport close") }

	s.transport.Store(&transport)

	transport.Once("error", onError)
	transport.On("ready", onReady)
	transport.On("packet", onPacket)
	transport.On("drain", onDrain)
	transport.Once("close", onClose)

	s.cleanupFn.Push(func() {
		transport.RemoveListener("error", onError)
		transport.RemoveListener("ready", onReady)
		transport.RemoveListener("packet", onPacket)
		transport.RemoveListener("drain", onDrain)
		transport.RemoveListener("close", onClose)
	})
}

// Upon transport "drain" event
func (s *socket) onDrain() {
	if seqFn, err := s.sentCallbackFn.Shift(); err == nil {
		socket_log.Debug("executing batch send callback")
		for _, fn := range seqFn {
			fn(s.Transport())
		}
	}
}

// Upgrades socket to the given transport
func (s *socket) MaybeUpgrade(transport transports.Transport) {
	socket_log.Debug(`might upgrade socket transport from "%s" to "%s"`, s.Transport().Name(), transport.Name())

	s.upgrading.Store(true)

	var check, cleanup func()
	var onPacket, onError, onTransportClose, onClose types.EventListener
	var upgradeTimeoutTimer, checkIntervalTimer atomic.Pointer[utils.Timer]

	onPacket = func(datas ...any) {
		data := datas[0].(*packet.Packet)
		sb := new(strings.Builder)
		io.Copy(sb, data.Data)
		if data.Type == packet.PING && sb.String() == "probe" {
			socket_log.Debug("got probe ping packet, sending pong")
			transport.Send([]*packet.Packet{{Type: packet.PONG, Data: strings.NewReader("probe")}})
			s.Emit("upgrading", transport)

			utils.ClearInterval(checkIntervalTimer.Load())
			checkIntervalTimer.Store(utils.SetInterval(check, 100*time.Millisecond))

		} else if packet.UPGRADE == data.Type && s.ReadyState() != "closed" {
			socket_log.Debug("got upgrade packet - upgrading")
			cleanup()
			s.Transport().Discard()

			s.upgraded.Store(true)

			s.clearTransport()
			s.setTransport(transport)
			s.Emit("upgrade", transport)
			s.flush()
			if s.ReadyState() == "closing" {
				transport.Close(func() {
					s.OnClose("forced close")
				})
			}
		} else {
			cleanup()
			transport.Close()
		}
	}

	// we force a polling cycle to ensure a fast upgrade
	check = func() {
		if transports.POLLING == s.Transport().Name() && s.Transport().Writable() {
			socket_log.Debug("writing a noop packet to polling for fast upgrade")
			s.Transport().Send([]*packet.Packet{{Type: packet.NOOP}})
		}
	}

	cleanup = func() {
		s.upgrading.Store(false)

		utils.ClearInterval(checkIntervalTimer.Load())
		utils.ClearTimeout(upgradeTimeoutTimer.Load())

		if transport != nil {
			transport.RemoveListener("packet", onPacket)
			transport.RemoveListener("close", onTransportClose)
			transport.RemoveListener("error", onError)
		}
		s.RemoveListener("close", onClose)
	}

	onError = func(err ...any) {
		socket_log.Debug("client did not complete upgrade - %v", err[0])
		cleanup()
		if transport != nil {
			transport.Close()
			transport = nil
		}
	}

	onTransportClose = func(...any) {
		onError("transport closed")
	}

	onClose = func(...any) {
		onError("socket closed")
	}

	// set transport upgrade timer
	upgradeTimeoutTimer.Store(utils.SetTimeout(func() {
		socket_log.Debug("client did not complete upgrade - closing transport")
		cleanup()
		if transport != nil {
			if transport.ReadyState() == "open" {
				transport.Close()
			}
		}
	}, s.server.Opts().UpgradeTimeout()))

	transport.On("packet", onPacket)
	transport.Once("close", onTransportClose)
	transport.Once("error", onError)

	s.Once("close", onClose)
}

// Clears listeners and timers associated with current transport.
func (s *socket) clearTransport() {
	s.cleanupFn.DoWrite(func(cleanups []types.Callable) []types.Callable {
		for _, cleanup := range cleanups {
			cleanup()
		}
		return cleanups[:0]
	})

	// silence further transport errors and prevent uncaught exceptions
	s.Transport().On("error", func(...any) {
		socket_log.Debug("error triggered by discarded transport")
	})

	// ensure transport won't stay open
	s.Transport().Close()

	utils.ClearTimeout(s.pingTimeoutTimer.Load())
}

// Called upon transport considered closed.
// Possible reasons: `ping timeout`, `client error`, `parse error`,
// `transport error`, `server close`, `transport close`
func (s *socket) OnClose(reason string, description ...error) {
	if s.ReadyState() != "closed" {
		description = append(description, nil)

		s.SetReadyState("closed")

		// clear timers
		utils.ClearTimeout(s.pingIntervalTimer.Load())

		utils.ClearTimeout(s.pingTimeoutTimer.Load())

		// clean writeBuffer in defer, so developers can still
		// grab the writeBuffer on 'close' event
		defer func() {
			go s.writeBuffer.Clear()
		}()

		s.packetsFn.Clear()

		s.sentCallbackFn.Clear()

		s.clearTransport()
		s.Emit("close", reason, description[0])
	}
}

// Sends a message packet.
func (s *socket) Send(
	data io.Reader,
	options *packet.Options,
	callback SendCallback,
) Socket {
	s.sendPacket(packet.MESSAGE, data, options, callback)
	return s
}

// Alias of [Send]
func (s *socket) Write(
	data io.Reader,
	options *packet.Options,
	callback SendCallback,
) Socket {
	s.sendPacket(packet.MESSAGE, data, options, callback)
	return s
}

// Sends a packet.
func (s *socket) sendPacket(
	packetType packet.Type,
	data io.Reader,
	options *packet.Options,
	callback SendCallback,
) {
	if readystate := s.ReadyState(); readystate != "closing" && readystate != "closed" {
		socket_log.Debug(`sending packet "%s" (%p)`, packetType, data)

		if options == nil {
			options = &packet.Options{}
		}

		if options.Compress == nil || *options.Compress {
			options.Compress = utils.Ptr(true)
		} else {
			options.Compress = utils.Ptr(false)
		}

		packet := &packet.Packet{
			Type:    packetType,
			Data:    data,
			Options: options,
		}

		// exports packetCreate event
		s.Emit("packetCreate", packet)

		s.writeBuffer.Push(packet)

		// add send callback to object, if defined
		if callback != nil {
			s.packetsFn.Push(callback)
		}

		s.flush()
	}
}

// Attempts to flush the packets buffer.
func (s *socket) flush() {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	if s.ReadyState() != "closed" && s.Transport().Writable() {
		if wbuf := s.writeBuffer.AllAndClear(); len(wbuf) > 0 {
			socket_log.Debug("flushing buffer to transport")
			s.Emit("flush", wbuf)
			s.server.Emit("flush", s, wbuf)
			if packetsFn := s.packetsFn.AllAndClear(); len(packetsFn) > 0 {
				s.sentCallbackFn.Push(packetsFn)
			} else {
				s.sentCallbackFn.Push(nil)
			}
			s.Transport().Send(wbuf)
			s.Emit("drain")
			s.server.Emit("drain", s)
		}
	}
}

// Get available upgrades for this socket.
func (s *socket) getAvailableUpgrades() []string {
	availableUpgrades := []string{}
	for _, upg := range s.server.Upgrades(s.Transport().Name()).Keys() {
		if s.server.Opts().Transports().Has(upg) {
			availableUpgrades = append(availableUpgrades, upg)
		}
	}
	return availableUpgrades
}

// Closes the socket and underlying transport.
func (s *socket) Close(discard bool) {
	if discard &&
		(s.ReadyState() == "open" || s.ReadyState() == "closing") {
		s.closeTransport(discard)
		return
	}

	if s.ReadyState() != "open" {
		return
	}

	s.SetReadyState("closing")

	if length := s.writeBuffer.Len(); length > 0 {
		socket_log.Debug("there are %d remaining packets in the buffer, waiting for the 'drain' event", length)
		s.Once("drain", func(...any) {
			socket_log.Debug("all packets have been sent, closing the transport")
			s.closeTransport(discard)
		})
		return
	}

	socket_log.Debug("the buffer is empty, closing the transport right away")
	s.closeTransport(discard)
}

// Closes the underlying transport.
func (s *socket) closeTransport(discard bool) {
	socket_log.Debug("closing the transport (discard? %t)", discard)
	if discard {
		s.Transport().Discard()
	}
	s.Transport().Close(func() { s.OnClose("forced close") })
}
