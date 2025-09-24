# Socket.IO for Golang

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/servers/socket/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/servers/socket/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/servers/socket/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/servers/socket/v3)

## Overview

Socket.IO is a real-time bidirectional event-based communication library for Golang. This repository contains the Socket.IO server implementation.

## Features

- **Protocol Support**
  - Socket.IO v4+ protocol
  - Binary data transmission
  - Multiplexing (namespaces)
  - Room support

- **Transport Layer**
  - WebSocket
  - HTTP long-polling
  - WebTransport (experimental)

- **Advanced Features**
  - Automatic reconnection
  - Packet buffering
  - Acknowledgments
  - Broadcasting
  - Multiple server instances support

## Installation

```bash
go get github.com/zishang520/socket.io/servers/socket/v3
```

## Quick Start

### Basic Usage

```go
package main

import (
    "github.com/zishang520/socket.io/servers/socket/v3"
    "github.com/zishang520/socket.io/v3/pkg/types"
)

func main() {
    server := socket.NewServer(nil, nil)

    server.On("connection", func(clients ...any) {
        client := clients[0].(*socket.Socket)

        // Handle events
        client.On("message", func(data ...any) {
            // Echo the received message
            client.Emit("message", data...)
        })
    })

    server.Listen(":3000", nil)
}
```

## Server Integration

### Standard HTTP Server

```go
http.Handle("/socket.io/", server.ServeHandler(nil))
http.ListenAndServe(":3000", nil)
```

### Fasthttp

```go
fasthttp.ListenAndServe(":3000", fasthttpadaptor.NewFastHTTPHandler(
    server.ServeHandler(nil),
))
```

### Fiber

```go
app := fiber.New()
app.Use("/socket.io/", adaptor.HTTPHandler(server.ServeHandler(nil)))
```

## Advanced Usage

### Namespaces

```go
// Create a custom namespace
nsp := server.Of("/custom", nil)

nsp.On("connection", func(clients ...any) {
    client := clients[0].(*socket.Socket)
    // Handle namespace specific events
})
```

### Rooms

```go
server.On("connection", func(clients ...any) {
    client := clients[0].(*socket.Socket)

    // Join a room
    client.Join("room1")

    // Broadcast to room
    server.To("room1").Emit("event", "message")
})
```

### Middleware

```go
server.Use(func(client *socket.Socket, next func()) {
    // Middleware logic
    next()
})
```

## Configuration

```go
opts := socket.DefaultServerOptions()
opts.SetPingTimeout(20 * time.Second)
opts.SetPingInterval(25 * time.Second)
opts.SetMaxHttpBufferSize(1e6)
opts.SetCors(&types.Cors{
    Origin: "*",
    Credentials: true,
})
```

## Debugging

Enable debug logging:

```bash
DEBUG=socket.io*
```

## Testing

Run the test suite:

```bash
make test
```

## API Documentation

For detailed API documentation, please visit:

- [GoDoc Documentation](https://pkg.go.dev/github.com/zishang520/socket.io/servers/socket/v3)
- [Socket.IO Protocol](https://github.com/socketio/socket.io-protocol)

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
