package engine

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// socketWithUpgrade implements an Engine.IO socket that supports transport upgrades.
// It starts with a basic transport (typically HTTP long-polling) and automatically
// attempts to upgrade to more efficient transports (like WebSocket) after establishing
// the initial connection.
//
// Features:
//   - Automatic transport upgrade
//   - Fallback to lower-level transports
//   - Seamless packet handling during upgrades
//   - Connection state management
//   - Binary data support
//
// Example:
//
//	opts := engine.DefaultSocketOptions()
//	opts.SetTransports(types.NewSet(
//	    transports.Polling,    // Initial transport
//	    transports.WebSocket,  // Upgrade target
//	))
//	socket := engine.NewSocketWithUpgrade("http://localhost:8080", opts)
//	socket.On("open", func(...any) {
//	    socket.Send("hello")
//	})
//
// See: [SocketWithoutUpgrade]
//
// See: [Socket]
type socketWithUpgrade struct {
	SocketWithoutUpgrade

	// _upgrades holds the set of available transport upgrades.
	_upgrades *types.Set[string]
}

// MakeSocketWithUpgrade creates a new SocketWithUpgrade instance with upgrade support.
// It initializes the base socket and prepares the upgrade mechanism.
func MakeSocketWithUpgrade() SocketWithUpgrade {
	s := &socketWithUpgrade{
		SocketWithoutUpgrade: MakeSocketWithoutUpgrade(),
		_upgrades:            types.NewSet[string](),
	}

	s.Prototype(s)
	return s
}

// NewSocketWithUpgrade creates a new SocketWithUpgrade with the specified URI and options.
//
// Parameters:
//   - uri: The server URI to connect to (e.g., "http://localhost:8080")
//   - opts: Socket configuration options
//
// Returns:
//   - SocketWithUpgrade: A configured socket instance
func NewSocketWithUpgrade(uri string, opts SocketOptionsInterface) SocketWithUpgrade {
	s := MakeSocketWithUpgrade()
	s.Construct(uri, opts)
	return s
}

// OnOpen handles the socket open event. If upgrade is enabled, it initiates
// transport upgrade probes for available upgrade options.
func (s *socketWithUpgrade) OnOpen() {
	s.SocketWithoutUpgrade.OnOpen()

	if SocketStateOpen == s.ReadyState() && s.Opts().Upgrade() {
		client_socket_log.Debug("starting upgrade probes")
		for _, upgrade := range s._upgrades.Keys() {
			s._probe(upgrade)
		}
	}
}

// _probe attempts to upgrade to a specified transport type by creating a new transport
// and testing its viability through a probe packet exchange.
//
// The probe process:
//  1. Creates a new transport instance
//  2. Sends a probe packet
//  3. Waits for probe response
//  4. If successful, upgrades to the new transport
//
// Parameters:
//   - name: The name of the transport to probe (e.g., "websocket")
func (s *socketWithUpgrade) _probe(name string) {
	client_socket_log.Debug(`probing transport "%s"`, name)
	transport := s.Proto().CreateTransport(name)
	var (
		failed  atomic.Bool
		cleanup func()
	)

	s.SetPriorWebsocketSuccess(false)

	onTransportOpen := func(...any) {
		if failed.Load() {
			return
		}

		client_socket_log.Debug(`probe transport "%s" opened`, name)
		transport.Send([]*packet.Packet{
			{
				Type: packet.PING,
				Data: types.NewStringBufferString("probe"),
			},
		})
		transport.Once("packet", func(msgs ...any) {
			if failed.Load() {
				return
			}
			msg, ok := msgs[0].(*packet.Packet)
			if !ok {
				return
			}
			sb := new(strings.Builder)
			io.Copy(sb, msg.Data)

			if msg.Type == packet.PONG && sb.String() == "probe" {
				client_socket_log.Debug(`probe transport "%s" pong`, name)
				s.SetUpgrading(true)
				s.Emit("upgrading", transport)
				if transport == nil {
					return
				}
				s.SetPriorWebsocketSuccess(transports.WEBSOCKET == transport.Name())
				client_socket_log.Debug(`pausing current transport "%s"`, s.Transport().Name())
				s.Transport().Pause(func() {
					if failed.Load() {
						return
					}
					if SocketStateClosed == s.ReadyState() {
						return
					}
					client_socket_log.Debug("changing transport and sending upgrade packet")

					cleanup()

					s.Proto().SetTransport(transport)
					transport.Send([]*packet.Packet{
						{
							Type: packet.UPGRADE,
						},
					})
					s.Emit("upgrade", transport)
					transport = nil
					s.SetUpgrading(false)
					s.Proto().Flush()
				})
			} else {
				client_socket_log.Debug(`probe transport "%s" failed`, name)
				s.Emit("upgradeError", errors.New("["+transport.Name()+"] probe error"))
			}
		})
	}

	freezeTransport := func() {
		if failed.Load() {
			return
		}
		failed.Store(true)
		cleanup()
		transport.Close()
		transport = nil
	}

	onerror := func(errs ...any) {
		e := fmt.Errorf("[%s] probe error: %w", transport.Name(), slices.TryGetAny[error](errs, 0))
		freezeTransport()
		client_socket_log.Debug(`probe transport "%s" failed because of error: %v`, name, e)
		s.Emit("upgradeError", e)
	}

	onTransportClose := func(...any) {
		onerror(errors.New("transport closed"))
	}

	onclose := func(...any) {
		onerror(errors.New("socket closed"))
	}

	onupgrade := func(to ...any) {
		if to, ok := to[0].(Transport); ok && to != nil && transport != nil && to.Name() != transport.Name() {
			client_socket_log.Debug(`"%s" works - aborting "%s"`, to.Name(), transport.Name())
			freezeTransport()
		}
	}

	cleanup = func() {
		transport.RemoveListener("open", onTransportOpen)
		transport.RemoveListener("error", onerror)
		transport.RemoveListener("close", onTransportClose)
		s.RemoveListener("close", onclose)
		s.RemoveListener("upgrading", onupgrade)
	}

	transport.Once("open", onTransportOpen)
	transport.Once("error", onerror)
	transport.Once("close", onTransportClose)

	s.Once("close", onclose)
	s.Once("upgrading", onupgrade)

	if s._upgrades.Has(transports.WEBTRANSPORT) && name != transports.WEBTRANSPORT {
		// favor WebTransport
		utils.SetTimeout(func() {
			if !failed.Load() {
				transport.Open()
			}
		}, 200*time.Millisecond)
	} else {
		transport.Open()
	}
}

// OnHandshake processes the initial handshake data from the server and filters
// the available transport upgrades based on server and client configuration.
//
// Parameters:
//   - data: Handshake data received from the server
func (s *socketWithUpgrade) OnHandshake(data *HandshakeData) {
	s._upgrades = s._filterUpgrades(data.Upgrades)
	s.SocketWithoutUpgrade.OnHandshake(data)
}

// _filterUpgrades filters the server's supported upgrades against the client's
// configured transports and returns the mutually supported set.
//
// Parameters:
//   - upgrades: List of transport names supported by the server
//
// Returns:
//   - *types.Set[string]: Set of mutually supported transport names
func (s *socketWithUpgrade) _filterUpgrades(upgrades []string) *types.Set[string] {
	filteredUpgrades := types.NewSet[string]()
	for _, upgrade := range upgrades {
		if s.Transports().FindIndex(func(s string) bool {
			return s == upgrade
		}) != -1 {
			filteredUpgrades.Add(upgrade)
		}
	}
	return filteredUpgrades
}
