# socket.io-go-adapter

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/adapters/adapter/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/adapters/adapter/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/adapters/adapter/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/adapters/adapter/v3)

## Description

A base adapter implementation for Socket.IO server in Go, providing core functionality for building custom adapters and scaling Socket.IO applications.

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/adapter/v3
```

## Features

- Base adapter interface and implementation
- Cluster adapter support
- Session-aware adapter capabilities
- Heartbeat mechanism for cluster communication
- Remote socket handling
- Extensible adapter architecture

## How to use

Basic usage example:

```golang
package main

import (
    "github.com/zishang520/socket.io/adapters/adapter/v3"
    "github.com/zishang520/socket.io/servers/socket/v3"
)

func main() {
    // Create Socket.IO server configuration
    config := socket.DefaultServerOptions()

    // Use default adapter
    config.SetAdapter(&adapter.AdapterBuilder{})

    // Create server with adapter
    io := socket.NewServer(nil, config)

    // Handle connections
    io.On("connection", func(clients ...any) {
        client := clients[0].(*socket.Socket)
        // Your connection handling logic
    })
}
```

## Adapter Types

The package provides several adapter implementations:

1. Base Adapter

```golang
type Adapter interface {
    Broadcast([]Room, *BroadcastOptions, ...any)
    BroadcastWithAck([]Room, *BroadcastOptions, ...any) <-chan []any
    // ... other methods
}
```

2. Cluster Adapter

```golang
type ClusterAdapter interface {
    Adapter
    ServerCount() int
    // Additional cluster-specific methods
}
```

3. Session-Aware Adapter

```golang
type SessionAwareAdapter interface {
    Adapter
    SaveSession(id string, session any)
    GetSession(id string) any
    // Session management methods
}
```

## Configuration Options

### ClusterAdapterOptions

```golang
type ClusterAdapterOptions struct {
    HeartbeatInterval time.Duration
    HeartbeatTimeout  time.Duration
}
```

## Testing

Run the test suite with:

```bash
make test
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Support

If you encounter any issues or have questions, please file them in the [issues section](https://github.com/zishang520/socket.io/issues).

## License

This project is licensed under the MIT License - see the LICENSE file for details.
