# Socket.IO Client for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/clients/socket/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/clients/socket/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/clients/socket/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/clients/socket/v3)

A robust Go client implementation for [Socket.IO](https://socket.io/), featuring real-time bidirectional event-based communication.

## Features

- **Transport Support**
  - WebSocket
  - HTTP long-polling
  - WebTransport (experimental)
  - Automatic transport upgrade
  - Fallback mechanism

- **Connection Management**
  - Automatic reconnection with exponential backoff
  - Connection state handling
  - Multiple namespaces support
  - Room support

- **Event Handling**
  - Event emitting and listening
  - Acknowledgements support
  - Volatile events
  - Binary data support
  - Custom event listeners

- **Advanced Features**
  - Multiplexing
  - Custom middleware
  - Connection timeout handling
  - Debug logging

## Installation

```bash
go get github.com/zishang520/socket.io/clients/socket/v3
```

## Quick Start

### Basic Connection

```go
package main

import (
    "log"

    "github.com/zishang520/socket.io/clients/socket/v3"
)

func main() {
    client, err := socket.Connect("http://localhost:3000", nil)
    if err != nil {
        log.Fatal(err)
    }

    client.On("connect", func(...any) {
        log.Println("Connected!")
        client.Emit("message", "Hello Server!")
    })

    client.On("event", func(data ...any) {
        log.Printf("Received: %v", data)
    })

    select {}
}
```

### Advanced Usage

```go
package main

import (
    "time"

    "github.com/zishang520/socket.io/clients/socket/v3"
    "github.com/zishang520/socket.io/clients/engine/v3/transports"
    "github.com/zishang520/socket.io/v3/pkg/types"
)

func main() {
    opts := socket.DefaultOptions()
    opts.SetTransports(types.NewSet(
        transports.Polling,
        transports.WebSocket,
    ))
    opts.SetTimeout(5 * time.Second)
    opts.SetReconnection(true)
    opts.SetReconnectionAttempts(5)

    manager := socket.NewManager("http://localhost:3000", opts)

    // Custom namespace
    socket := manager.Socket("/custom", nil)

    // Event handling
    socket.On("connect", func(...any) {
        socket.Emit("auth", map[string]string{
            "token": "your-auth-token",
        })
    })

    // Acknowledgement
    socket.Emit("event", "data", func(args ...any) {
        log.Printf("Server acknowledged: %v", args)
    })

    // Listen to all events
    socket.OnAny(func(args ...any) {
        log.Printf("Caught event: %v", args)
    })
}
```

## API Reference

### Manager Options

```go
opts := socket.DefaultManagerOptions()
opts.SetReconnection(true)                   // Enable/disable reconnection
opts.SetReconnectionAttempts(math.Inf(1))    // Number of reconnection attempts
opts.SetReconnectionDelay(1000)              // Initial delay in milliseconds
opts.SetReconnectionDelayMax(5000)           // Maximum delay between reconnections
opts.SetRandomizationFactor(0.5)             // Randomization factor for delays
opts.SetTimeout(20000)                       // Connection timeout
```

### Socket Methods

- **Emit**: `socket.Emit(eventName string, args ...any)`
- **On**: `socket.On(eventName string, fn func(...any))`
- **Once**: `socket.Once(eventName string, fn func(...any))`
- **Off**: `socket.Off(eventName string, fn func(...any))`
- **OnAny**: `socket.OnAny(fn func(...any))`
- **Connect**: `socket.Connect()`
- **Disconnect**: `socket.Disconnect()`

### Events

- `connect`: Fired upon connection
- `disconnect`: Fired upon disconnection
- `connect_error`: Fired upon connection error
- `reconnect`: Fired upon successful reconnection
- `reconnect_attempt`: Fired upon reconnection attempt
- `reconnect_error`: Fired upon reconnection error
- `reconnect_failed`: Fired when reconnection fails

## Debugging

Enable debug logs:


```go
import "github.com/zishang520/socket.io/v3/pkg/log"

log.DEBUG = true
```

```bash
make test
```

## Testing

```bash
make test
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Related Projects

- [Socket.IO](https://socket.io/)
- [Engine.IO Client for Go](https://github.com/zishang520/socket.io/tree/v3/clients/engine)
- [Socket.IO Server for Go](https://github.com/zishang520/socket.io/tree/v3/servers/socket)