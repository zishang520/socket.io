package engine

import (
	"context"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// Logger instances for different components of the Engine.IO client.
// These loggers provide structured logging with component-specific prefixes.
var (
	// client_socket_log handles logging for the socket component
	client_socket_log = log.NewLog("engine.io-client:socket")

	// client_polling_log handles logging for the polling transport
	client_polling_log = log.NewLog("engine.io-client:polling")

	// client_transport_log handles logging for the transport layer
	client_transport_log = log.NewLog("engine.io-client:transport")

	// client_websocket_log handles logging for the WebSocket transport
	client_websocket_log = log.NewLog("engine.io-client:websocket")

	// client_webtransport_log handles logging for the WebTransport transport
	client_webtransport_log = log.NewLog("engine.io-client:webtransport")
)

// Event names for system-level events that can be emitted by the Engine.IO client.
const (
	// EventBeforeUnload is emitted when the application is about to unload/exit.
	// This allows for graceful cleanup of connections.
	EventBeforeUnload types.EventName = "beforeunload"

	// EventOffline is emitted when the network connection is lost.
	// This can be used to handle disconnection scenarios.
	EventOffline types.EventName = "offline"

	// EventOnline is emitted when the network connection is restored.
	// This can be used to handle reconnection scenarios.
	EventOnline types.EventName = "online"
)

const Protocol = parser.Protocol

// BASE64_OVERHEAD represents the size overhead when encoding binary data as base64.
// Base64 encoding increases the size of binary data by approximately 33%.
// See: https://en.wikipedia.org/wiki/Base64
const BASE64_OVERHEAD float64 = 1.33

// init performs initialization tasks for the Engine.IO client.
// This includes setting up signal handling and network monitoring.
func init() {
	setupSignalHandling()
	setupNetworkHandling()
}

// setupSignalHandling configures handlers for system signals to ensure
// graceful shutdown of the Engine.IO client. It listens for:
// - os.Interrupt (Ctrl+C)
// - syscall.SIGINT (interrupt signal)
// - syscall.SIGTERM (termination signal)
// - syscall.SIGQUIT (quit signal)
func setupSignalHandling() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		defer stop()

		<-ctx.Done()
		events.Emit(EventBeforeUnload)
	}()
}

// isNetworkOnline checks if there is an active network connection available.
// It examines all network interfaces and returns true if at least one non-loopback
// interface is up and has an IP address assigned.
//
// Returns:
//   - true if there is an active network connection
//   - false if no active network connection is found
func isNetworkOnline() bool {
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			if addrs, err := iface.Addrs(); err == nil && len(addrs) > 0 {
				return true
			}
		}
	}
	return false
}

// setupNetworkHandling configures network status monitoring for the Engine.IO client.
// It periodically checks the network status and emits appropriate events when
// the network state changes (online/offline).
//
// The check interval is set to 3 seconds, which provides a good balance between
// responsiveness and system resource usage.
func setupNetworkHandling() {
	var previousState atomic.Bool
	utils.SetInterval(func() {
		if currentState := isNetworkOnline(); currentState != previousState.Load() {
			previousState.Store(currentState)
			if currentState {
				events.Emit(EventOnline)
			} else {
				events.Emit(EventOffline)
			}
		}
	}, 3000*time.Millisecond)
}
