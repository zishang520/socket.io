package socket

import (
	"errors"
	"math"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/clients/engine/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	tools "github.com/zishang520/socket.io/v3/pkg/utils"
	"github.com/zishang520/socket.io/clients/socket/v3/utils"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

type Engine = engine.Socket

// Manager implements the Socket.IO client manager that handles connections to a Socket.IO server.
// It manages connection lifecycle, automatic reconnection, and socket namespaces.
//
// Example usage:
//
//	opts := DefaultManagerOptions()
//	opts.SetTimeout(5 * time.Second)
//
//	manager, err := NewManager("http://localhost:8080", opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get a socket for the default namespace
//	socket := manager.Socket("/", nil)
type Manager struct {
	types.EventEmitter

	// The Engine.IO client instance
	//
	// Public
	engine Engine
	// Private
	_autoConnect bool
	// ReadyStateOpening | ReadyStateOpen | ReadyStateClosed
	//
	// Private
	_readyState atomic.Value
	// Private
	_reconnecting atomic.Bool

	// Private
	//
	// Readonly
	uri string
	// Public
	opts ManagerOptionsInterface

	// Private
	nsps *types.Map[string, *Socket]
	// Private
	subs *types.Slice[types.Callable]
	// Private
	backoff *utils.Backoff
	// Private
	_reconnection atomic.Bool
	// Private
	_reconnectionAttempts atomic.Value
	// Private
	_reconnectionDelay atomic.Value
	// Private
	_randomizationFactor atomic.Value
	// Private
	_reconnectionDelayMax atomic.Value
	// Private
	_timeout atomic.Pointer[time.Duration]

	// Private
	encoder parser.Encoder
	// Private
	decoder parser.Decoder
	// Private
	skipReconnect atomic.Bool
}

func MakeManager() *Manager {
	r := &Manager{
		EventEmitter: types.NewEventEmitter(),

		nsps: &types.Map[string, *Socket]{},
		subs: types.NewSlice[types.Callable](),
	}
	return r
}

// NewManager creates a new Manager instance with the specified URI and options.
// It establishes and manages the connection to a Socket.IO server.
//
// Parameters:
//   - uri: The URI of the Socket.IO server (e.g., "http://localhost:8080")
//   - opts: Configuration options for the manager
//
// Returns:
//   - *Manager: A new manager instance
func NewManager(uri string, opts ManagerOptionsInterface) *Manager {
	r := MakeManager()

	r.Construct(uri, opts)

	return r
}

func (m *Manager) Engine() Engine {
	return m.engine
}

func (m *Manager) Opts() ManagerOptionsInterface {
	return m.opts
}

func (m *Manager) Construct(uri string, opts ManagerOptionsInterface) {
	if opts == nil {
		opts = DefaultManagerOptions()
	}

	if opts.GetRawPath() == nil {
		opts.SetPath("/socket.io")
	}
	m.opts = opts
	if opts.GetRawReconnection() != nil {
		m.SetReconnection(opts.Reconnection())
	} else {
		m.SetReconnection(true)
	}
	if opts.GetRawReconnectionAttempts() != nil {
		m.SetReconnectionAttempts(opts.ReconnectionAttempts())
	} else {
		m.SetReconnectionAttempts(math.Inf(1))
	}
	if opts.GetRawReconnectionDelay() != nil {
		m.SetReconnectionDelay(opts.ReconnectionDelay())
	} else {
		m.SetReconnectionDelay(1_000)
	}
	if opts.GetRawReconnectionDelayMax() != nil {
		m.SetReconnectionDelayMax(opts.ReconnectionDelayMax())
	} else {
		m.SetReconnectionDelayMax(5_000)
	}
	if opts.GetRawRandomizationFactor() != nil {
		m.SetRandomizationFactor(opts.RandomizationFactor())
	} else {
		m.SetRandomizationFactor(0.5)
	}
	m.backoff = utils.NewBackoff(utils.WithMin(m.ReconnectionDelay()), utils.WithMax(m.ReconnectionDelayMax()), utils.WithJitter(m.RandomizationFactor()))
	if opts.GetRawTimeout() != nil {
		m.SetTimeout(opts.Timeout())
	} else {
		m.SetTimeout(20_000 * time.Millisecond)
	}

	m._readyState.Store(ReadyStateClosed)
	m.uri = uri
	if opts.GetRawParser() != nil {
		_parser := opts.Parser()
		m.encoder = _parser.NewEncoder()
		m.decoder = _parser.NewDecoder()
	} else {
		_parser := parser.NewParser()
		m.encoder = _parser.NewEncoder()
		m.decoder = _parser.NewDecoder()
	}
	if opts.GetRawAutoConnect() != nil {
		m._autoConnect = opts.AutoConnect()
	} else {
		m._autoConnect = true
	}
	if m._autoConnect {
		m.Open(nil)
	}
}

// Sets the `reconnection` config.
func (m *Manager) SetReconnection(reconnection bool) {
	m._reconnection.Store(reconnection)
	if !reconnection {
		m.skipReconnect.Store(true)
	}
}
func (m *Manager) Reconnection() bool {
	return m._reconnection.Load()
}

// Sets the reconnection attempts config.
func (m *Manager) SetReconnectionAttempts(reconnectionAttempts float64) {
	m._reconnectionAttempts.Store(reconnectionAttempts)
}
func (m *Manager) ReconnectionAttempts() float64 {
	if reconnectionAttempts := m._reconnectionAttempts.Load(); reconnectionAttempts != nil {
		return reconnectionAttempts.(float64)
	}
	return 0
}

// Sets the delay between reconnections.
func (m *Manager) SetReconnectionDelay(reconnectionDelay float64) {
	m._reconnectionDelay.Store(reconnectionDelay)
	if backoff := m.backoff; backoff != nil {
		backoff.SetMin(reconnectionDelay)
	}
}
func (m *Manager) ReconnectionDelay() float64 {
	if reconnectionDelay := m._reconnectionDelay.Load(); reconnectionDelay != nil {
		return reconnectionDelay.(float64)
	}
	return 0
}

// Sets the maximum delay between reconnections.
func (m *Manager) SetRandomizationFactor(randomizationFactor float64) {
	m._randomizationFactor.Store(randomizationFactor)
	if backoff := m.backoff; backoff != nil {
		backoff.SetJitter(randomizationFactor)
	}
}
func (m *Manager) RandomizationFactor() float64 {
	if randomizationFactor := m._randomizationFactor.Load(); randomizationFactor != nil {
		return randomizationFactor.(float64)
	}
	return 0
}

// Sets the randomization factor
func (m *Manager) SetReconnectionDelayMax(reconnectionDelayMax float64) {
	m._reconnectionDelayMax.Store(reconnectionDelayMax)
	if backoff := m.backoff; backoff != nil {
		backoff.SetMax(reconnectionDelayMax)
	}
}
func (m *Manager) ReconnectionDelayMax() float64 {
	if reconnectionDelayMax := m._reconnectionDelayMax.Load(); reconnectionDelayMax != nil {
		return reconnectionDelayMax.(float64)
	}
	return 0
}

// Sets the connection timeout. `false` to disable
func (m *Manager) SetTimeout(timeout time.Duration) {
	m._timeout.Store(&timeout)
}
func (m *Manager) Timeout() *time.Duration {
	return m._timeout.Load()
}

// Starts trying to reconnect if reconnection is enabled and we have not
// started reconnecting yet
func (m *Manager) maybeReconnectOnOpen() {
	// Only try to reconnect if it's the first time we're connecting
	if !m._reconnecting.Load() && m._reconnection.Load() && m.backoff.Attempts() == 0 {
		// keeps reconnection from firing twice for the same reconnection loop
		m.reconnect()
	}
}

// Open initiates the connection to the server.
// This is called automatically when autoConnect is true (default).
//
// Parameters:
//   - fn: Optional callback that will be called when connection is established or fails
//
// Returns:
//   - *Manager: The manager instance for chaining
func (m *Manager) Open(fn func(error)) *Manager {
	manager_log.Debug("readyState %s", m._readyState.Load())
	if m._readyState.Load() == ReadyStateOpen || m._readyState.Load() == ReadyStateOpening {
		return m
	}

	manager_log.Debug("opening %s", m.uri)
	m.engine = engine.NewSocket(m.uri, m.opts)
	m._readyState.Store(ReadyStateOpening)
	m.skipReconnect.Store(false)

	// emit `open`
	openSubDestroy := on(m.engine, "open", func(...any) {
		m.onopen()
		if fn != nil {
			fn(nil)
		}
	})

	onError := func(errs ...any) {
		err := errs[0].(error)
		manager_log.Debug("error")
		m.cleanup()
		m._readyState.Store(ReadyStateClosed)
		m.EventEmitter.Emit("error", err)
		if fn != nil {
			fn(err)
		} else {
			// Only do this if there is no fn to handle the error
			m.maybeReconnectOnOpen()
		}
	}

	// emit `error`
	errorSub := on(m.engine, "error", onError)

	if timeout := m._timeout.Load(); timeout != nil {
		manager_log.Debug("connect attempt will timeout after %v", timeout)

		// set timer
		timer := tools.SetTimeout(func() {
			manager_log.Debug("connect attempt timed out after %v", timeout)
			openSubDestroy()
			onError(errors.New("timeout"))
			m.engine.Close()
		}, *timeout)

		if m.opts.AutoUnref() {
			timer.Unref()
		}

		m.subs.Push(func() {
			tools.ClearTimeout(timer)
		})
	}

	m.subs.Push(openSubDestroy, errorSub)

	return m
}

// Alias for [Manager.Open]
func (m *Manager) Connect(fn func(error)) *Manager {
	return m.Open(fn)
}

// Called upon transport open.
func (m *Manager) onopen() {
	manager_log.Debug("open")

	// clear old subs
	m.cleanup()

	// mark as open
	m._readyState.Store(ReadyStateOpen)
	m.EventEmitter.Emit("open")

	// add new subs
	m.subs.Push(
		on(m.engine, "ping", m.onping),
		on(m.engine, "data", m.ondata),
		on(m.engine, "error", m.onerror),
		on(m.engine, "close", func(args ...any) {
			m.onclose(args[0].(string), args[1].(error))
		}),
		on(m.decoder, "decoded", m.ondecoded),
	)
}

// Called upon a ping.
func (m *Manager) onping(...any) {
	m.EventEmitter.Emit("ping")
}

// Called with data.
func (m *Manager) ondata(datas ...any) {
	if err := m.decoder.Add(datas[0]); err != nil {
		m.onclose("parse error", err)
	}
}

// Called when parser fully decodes a packet.
func (m *Manager) ondecoded(packets ...any) {
	go m.EventEmitter.Emit("packet", packets...)
}

// Called upon socket error.
func (m *Manager) onerror(errs ...any) {
	manager_log.Debug("error: %v", errs)
	m.EventEmitter.Emit("error", errs...)
}

// Socket creates or returns an existing Socket instance for the specified namespace.
//
// Parameters:
//   - nsp: The namespace to connect to (defaults to "/")
//   - opts: Socket-specific options
//
// Returns:
//   - *Socket: A Socket instance for the namespace
func (m *Manager) Socket(nsp string, opts SocketOptionsInterface) *Socket {
	socket, ok := m.nsps.Load(nsp)
	if !ok {
		socket = NewSocket(m, nsp, opts)
		m.nsps.Store(nsp, socket)
	} else if m._autoConnect && !socket.Active() {
		socket.Connect()
	}

	return socket
}

// _destroy cleans up resources when a socket is closed.
// If this is the last active socket, the manager connection will also be closed.
//
// Parameters:
//   - socket: The socket being destroyed
func (m *Manager) _destroy(_ *Socket) {
	close := true
	m.nsps.Range(func(nsp string, socket *Socket) bool {
		if socket.Active() {
			manager_log.Debug("socket %s is still active, skipping close", nsp)
			close = false
		}
		return close
	})

	if close {
		m._close()
	}
}

// Writes a packet.
func (m *Manager) _packet(packet *Packet) {
	manager_log.Debug("writing packet %#v", packet)

	for _, encodedPacket := range m.encoder.Encode(packet.Packet) {
		m.engine.Write(encodedPacket.Clone(), packet.Options, nil)
	}
}

// Clean up transport subscriptions and packet buffer.
func (m *Manager) cleanup() {
	manager_log.Debug("cleanup")

	m.subs.Range(func(subDestroy func(), i int) bool {
		subDestroy()
		return true
	})

	m.decoder.Destroy()
}

// Close the current socket.
func (m *Manager) _close() {
	manager_log.Debug("disconnect")
	m.skipReconnect.Store(true)
	m._reconnecting.Store(false)
	m.onclose("forced close", nil)
}

// Alias for [Manager._close]
func (m *Manager) disconnect() {
	m._close()
}

// _onclose handles cleanup when the connection is closed.
//
// Parameters:
//   - reason: The reason for disconnection
//   - description: Additional error details if applicable
func (m *Manager) onclose(reason string, description error) {
	manager_log.Debug("closed due to %s", reason)

	m.cleanup()
	if m.engine != nil {
		m.engine.Close()
	}
	m.backoff.Reset()
	m._readyState.Store(ReadyStateClosed)
	m.EventEmitter.Emit("close", reason, description)

	if m._reconnection.Load() && !m.skipReconnect.Load() {
		m.reconnect()
	}
}

// _reconnect attempts to reconnect to the server after a disconnection.
// It implements exponential backoff with jitter for retry timing.
func (m *Manager) reconnect() {
	if m._reconnecting.Load() || m.skipReconnect.Load() {
		return
	}

	if float64(m.backoff.Attempts()) >= m.ReconnectionAttempts() {
		manager_log.Debug("reconnect failed")
		m.backoff.Reset()
		m.EventEmitter.Emit("reconnect_failed")
		m._reconnecting.Store(false)
	} else {
		delay := m.backoff.Duration()
		manager_log.Debug("will wait %dms before reconnect attempt", delay)

		m._reconnecting.Store(true)
		timer := tools.SetTimeout(func() {
			if m.skipReconnect.Load() {
				return
			}

			manager_log.Debug("attempting reconnect")
			m.EventEmitter.Emit("reconnect_attempt", m.backoff.Attempts())

			// check again for the case socket closed in above events
			if m.skipReconnect.Load() {
				return
			}

			m.Open(func(err error) {
				if err != nil {
					manager_log.Debug("reconnect attempt error")
					m._reconnecting.Store(false)
					m.reconnect()
					m.EventEmitter.Emit("reconnect_error", err)
				} else {
					manager_log.Debug("reconnect success")
					m.onreconnect()
				}
			})
		}, time.Duration(delay)*time.Millisecond)

		if m.opts.AutoUnref() {
			timer.Unref()
		}

		m.subs.Push(func() {
			tools.ClearTimeout(timer)
		})
	}
}

// Called upon successful reconnect.
func (m *Manager) onreconnect() {
	attempt := m.backoff.Attempts()
	m._reconnecting.Store(false)
	m.backoff.Reset()
	m.EventEmitter.Emit("reconnect", attempt)
}
