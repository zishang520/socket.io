# Engine.IO: The Realtime Engine for Golang

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/servers/engine/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/servers/engine/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/servers/engine/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/servers/engine/v3)

## Overview

Engine.IO is a transport-based cross-browser/cross-device bi-directional communication layer implementation for [Socket.IO in Go](https://github.com/zishang520/socket.io/tree/v3/servers/socket). It abstracts away the differences between various transports (WebSocket, Polling, WebTransport) and provides a unified API.

## Features

- Multiple transport support (WebSocket, Polling, WebTransport)
- Automatic transport upgrade
- Stateful connections with heartbeat mechanism
- Binary data support
- Multiplexing support
- Auto-reconnection support
- Cross-browser compatibility
- Engine.IO protocol v3 and v4 support

## Installation

```bash
go get github.com/zishang520/socket.io/servers/engine/v3
```

## Quick Start

```go
package main

import (
    "github.com/zishang520/socket.io/servers/engine/v3"
    "github.com/zishang520/socket.io/servers/engine/v3/config"
    "github.com/zishang520/socket.io/v3/pkg/types"
)

func main() {
    // Configure server options
    serverOptions := &config.ServerOptions{}
    serverOptions.SetAllowEIO3(true)
    serverOptions.SetCors(&types.Cors{
        Origin:      "*",
        Credentials: true,
    })

    // Create and start server
    server := engine.Listen(":4444", serverOptions, nil)

    // Handle connections
    server.On("connection", func(sockets ...any) {
        socket := sockets[0].(engine.Socket)
        socket.On("message", func(args ...any) {
            // Handle messages
        })
    })

    // Keep the server running
    select {}
}
```

## Usage

### Server Initialization Methods

1. **Direct Listening**
2. **HTTP Server Integration**
3. **Custom Request Handling**
4. **WebSocket Integration**
5. **WebTransport Support**

## Configuration

### Server Options

```go
opts := &config.ServerOptions{}
opts.SetPingTimeout(20_000 * time.Millisecond)
opts.SetPingInterval(25_000 * time.Millisecond)
opts.SetUpgradeTimeout(10_000 * time.Millisecond)
opts.SetMaxHttpBufferSize(1e6)
// ...
```

## Transport Implementations

- **Polling**: XHR/JSONP transport
- **WebSocket**: Standard WebSocket transport
- **WebTransport**: Experimental WebTransport support

## Events

### Server Events

- `connection`: New client connection
- `connection_error`: Connection error
- `flush`: Buffer flush
- `drain`: Buffer drain

### Socket Events

- `message`: Incoming message
- `close`: Connection closed
- `error`: Error occurred
- `flush`: Write buffer flush
- `drain`: Write buffer drained
- `packet`: Raw packet received
- `packetCreate`: Before packet send
- `heartbeat`: Ping/Pong received

## Development

### Prerequisites

- Go 1.24.1+
- Make

### Testing

```bash
make test
```

### Debugging

Set the DEBUG environment variable:

```bash
DEBUG=engine*
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- [Documentation](https://pkg.go.dev/github.com/zishang520/socket.io/servers/engine/v3)
- [Issue Tracker](https://github.com/zishang520/socket.io/issues)
