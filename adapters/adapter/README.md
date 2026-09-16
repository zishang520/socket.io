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
    Broadcast(*parser.Packet, *socket.BroadcastOptions)
    BroadcastWithAck(*parser.Packet, *socket.BroadcastOptions, func(uint64), socket.Ack)
    // ... other methods
}
```

2. Cluster Adapter

```golang
type ClusterAdapter interface {
    Adapter
    ServerCount() (int64, error)
    // Additional cluster-specific methods
}
```

3. Session-Aware Adapter

```golang
type SessionAwareAdapter interface {
    Adapter
    PersistSession(*socket.SessionToPersist)
    RestoreSession(socket.PrivateSessionId, string) (*socket.Session, error)
    // Session management methods
}
```

## Configuration Options

### ClusterAdapterOptions

```golang
opts := adapter.DefaultClusterAdapterOptions()
opts.SetHeartbeatInterval(5 * time.Second)
opts.SetHeartbeatTimeout(10_000) // milliseconds
// Pass opts to your concrete cluster adapter builder.
```

Reader preparation can fail. `PrepareClusterData` returns `(any, bool, bool, error)`
(value, changed, contains binary, error), and `EncodeClusterMessageData` returns
`(any, bool, error)`. Callers must handle the error before encoding or publishing;
a failed read may already have consumed and closed the reader.

## Custom cluster transports

Implement `PreparePublish(*ClusterMessage) (PublishFunc, error)` and
`PreparePublishResponse(ServerId, *ClusterResponse) (PublishFunc, error)`.
These replace the former `DoPublish` and `DoPublishResponse` extension methods.
Application methods such as `Broadcast`, `FetchSockets` and `ServerSideEmit`
keep their existing signatures.

Preparation runs synchronously: select the destination and encode with the
transport's actual codec, then return a `PublishFunc` (`func() (Offset, error)`).
The returned function performs network or database I/O and owns the encoded
payload and routing values. It must not capture the mutable input message,
packet, options or application data. Preparation must not send the message.

The base adapter executes the function on its publisher queue. Responses use a
separate queue and ignore the returned offset. `PublishAndReturnOffset` waits on
the publisher queue; all accepted work precedes the final heartbeat close
message. Create operation timeouts inside the returned function, so waiting in
the queue does not consume the network timeout.

The heartbeat adapter embeds `ClusterAdapter` and owns the decision to send an
`ADAPTER_CLOSE` notification. It calls `PublishAndClose(message)` to prepare that
message, queue it after accepted messages and responses, and stop further
publishing. This method returns without waiting for delivery. The base adapter's
plain `Close()` stops publishing without choosing a protocol notification.

`PublishAndClose` is a publishing-layer operation for lifecycle implementations,
not a substitute for closing the outer adapter. Applications should call the
outer adapter's `Close()` so heartbeat timers, subscriptions and builder cleanup
are handled by their owners. For example, calling inherited `PublishAndClose`
directly on a heartbeat adapter does not stop its timers.

Adapters embedding the base implementation inherit `PublishAndClose`. Independent
implementations of `ClusterAdapter` must provide it with the same queue ordering
and stop publishing even if preparation fails. `MakeClusterAdapter()` remains a
single, parameterless constructor; no type assertion is needed by heartbeat shutdown.

There is no fallback to the old transport hooks or a generic deep-copy codec.
See the Unix adapter for a JSON/MessagePack example, Redis/Valkey Streams for
destination-specific encoding, and PostgreSQL for NOTIFY/attachment selection.

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
