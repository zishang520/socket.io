# Engine.IO Client for Go

[![Build Status](https://github.com/zishang520/socket.io/clients/engine/v3/actions/workflows/go.yml/badge.svg)](https://github.com/zishang520/socket.io/clients/engine/v3/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/clients/engine/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/clients/engine/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/clients/engine/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/clients/engine/v3)

A robust Go client implementation for [Engine.IO](https://github.com/zishang520/socket.io/servers/engine), the reliable real-time bidirectional communication layer that powers [Socket.IO](https://github.com/zishang520/socket.io/servers/socket).

## Features

- **Multiple Transport Support**
  - WebSocket transport
  - HTTP long-polling transport
  - WebTransport (experimental)
  - Automatic transport upgrade
  - Fallback mechanism

- **Connection Management**
  - Automatic reconnection
  - Heartbeat mechanism
  - Connection state handling
  - Cookie support
  - Custom headers

- **Data Handling**
  - Binary data support
  - Base64 encoding fallback
  - Packet buffering
  - Message compression

- **Advanced Features**
  - Event-driven architecture
  - Configurable timeouts
  - Debug logging
  - Cross-platform compatibility
  - Protocol v3 and v4 support

## Installation

```bash
go get github.com/zishang520/socket.io/clients/engine/v3
```

## Quick Start

### Basic Usage

```go
package main

import (
    "log"

    eio "github.com/zishang520/socket.io/clients/engine/v3"
    "github.com/zishang520/socket.io/v3/pkg/types"
)

func main() {
    socket := eio.NewSocket("ws://localhost", nil)

    socket.On("open", func(args ...any) {
        log.Println("Connection established")
        socket.Send(types.NewStringBufferString("Hello!"), nil, nil)
    })

    socket.On("message", func(args ...any) {
        log.Printf("Received: %v", args[0])
    })

    socket.On("close", func(args ...any) {
        log.Println("Connection closed")
    })

    select {}
}
```

### Advanced Configuration

```go
package main

import (
    "time"

    "github.com/zishang520/socket.io/clients/engine/v3"
    "github.com/zishang520/socket.io/clients/engine/v3/transports"
)

func main() {
    opts := engine.DefaultSocketOptions()

    // Transport configuration
    opts.SetTransports(types.NewSet(
        transports.WebSocket,
        transports.Polling,
        transports.WebTransport,
    ))

    // Connection settings
    opts.SetPath("/engine.io")
    opts.SetRequestTimeout(10 * time.Second)
    opts.SetWithCredentials(true)

    // Upgrade configuration
    opts.SetUpgrade(true)
    opts.SetRememberUpgrade(true)

    socket := engine.NewSocket("ws://localhost", opts)
    // ... event handlers
}
```

## API Reference

### Socket States

- `SocketStateOpening`: Connection is being established
- `SocketStateOpen`: Connection is open and ready
- `SocketStateClosing`: Connection is closing
- `SocketStateClosed`: Connection is closed

### Events

- `open`: Connection established
- `message`: Message received
- `close`: Connection closed
- `error`: Error occurred
- `upgrade`: Transport upgraded
- `upgradeError`: Transport upgrade failed
- `packet`: Raw packet received
- `drain`: Write buffer drained

### Transport Types

```go
import "github.com/zishang520/socket.io/clients/engine/v3/transports"

// Available transports
transports.Polling      // HTTP long-polling
transports.WebSocket    // WebSocket
transports.WebTransport // WebTransport (experimental)
```

## Development

### Running Tests

```bash
make test
```

### Debugging

Enable debug logs:

```go
import "github.com/zishang520/socket.io/v3/pkg/log"

log.DEBUG = true
```

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

- [Engine.IO Protocol](https://github.com/socketio/engine.io-protocol)
- [Engine.IO Server](https://github.com/zishang520/socket.io/servers/engine)
