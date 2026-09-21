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
- **WebTransport**: Experimental WebTransport support (Engine.IO v4)

WebTransport uses Engine.IO v4 for direct connections and upgrades, and is only
advertised for EIO4 sessions. An attempted WebTransport upgrade from an EIO3 session
is rejected while its polling connection remains available.

For WebTransport, the Engine.IO `Cors.Origin` rule and the supplied
`webtransport.Server.CheckOrigin` policy must both allow the request. Engine.IO
does not overwrite the shared WebTransport server's callback. Without a custom
callback, WebTransport retains its default same-origin policy, even when Engine.IO
CORS allows a cross-origin request. Configure `CheckOrigin` before starting the
WebTransport server when cross-origin access is required.

For example, create and configure the WebTransport server before opening its
listener (replace the origin and certificate paths for your application):

```go
package main

import (
    "log"
    "net/http"

    "github.com/quic-go/quic-go/http3"
    "github.com/quic-go/webtransport-go"
    "github.com/zishang520/socket.io/servers/engine/v3"
    "github.com/zishang520/socket.io/servers/engine/v3/config"
    "github.com/zishang520/socket.io/v3/pkg/types"
)

func main() {
    const origin = "https://app.example"
    opts := config.DefaultServerOptions()
    opts.SetTransports(types.NewSet(engine.WebTransport))
    opts.SetCors(&types.Cors{Origin: origin})
    eio := engine.NewServer(opts)
    defer eio.Close()
    // Register connection and message listeners before serving.

    wt := &webtransport.Server{
        H3: &http3.Server{Addr: ":4433"},
        CheckOrigin: func(r *http.Request) bool {
            return r.Header.Get("Origin") == origin
        },
    }
    defer wt.Close()
    mux := http.NewServeMux()
    mux.HandleFunc("/engine.io/", func(w http.ResponseWriter, r *http.Request) {
        eio.OnWebTransportSession(types.NewHttpContext(w, r), wt)
    })
    wt.H3.Handler = mux
    if err := wt.ListenAndServeTLS("cert.pem", "key.pem"); err != nil {
        log.Print(err)
    }
}
```

Both origin policies above are set before serving. The convenience method
`HttpServer.ListenWebTransportTLS` starts serving before it returns, so configure
an explicit `webtransport.Server` as above when a custom origin policy is needed.

### Connection initialization

For WebSocket and WebTransport sessions, the server starts reading packets after
the synchronous `connection` listeners return. Register your `data`, `message`,
`error`, and `close` listeners in that callback; do not block it waiting for an
incoming packet. During transport upgrades, reading starts after the probe
listeners have been installed.

For a new stream session, server heartbeat timers start after read permission is
granted, so a delayed initialization callback does not consume the PONG deadline
while inbound heartbeat packets cannot yet be processed. Polling and standalone
socket construction retain their existing heartbeat startup order.

`UpgradeTimeout` also bounds this stream initialization phase (10 seconds by
default). If construction or a synchronous connection callback exceeds that
budget, the server closes the WebSocket connection or WebTransport session and
cancels reader startup.
A constructor returning after cancellation cannot publish a live connection or
read buffered packets. An expired upgrade candidate does not close its existing
polling session. A non-positive `UpgradeTimeout` expires immediately; it does not
disable this protection. Completion also checks the elapsed budget in case the
timer callback has not yet run.

This deadline cannot interrupt arbitrary Go application code. Custom constructors
and callbacks must apply their own cancellation or deadlines to blocking work.
It does not extend client-side connection or heartbeat deadlines; initialization
callbacks should remain short.

The server manages this ordering automatically. Applications and builders that
delegate to the built-in transport constructors do not need a separate startup
call or readiness option. Standalone constructors retain automatic reading.

### Custom transports

Custom transport builders and `CreateTransport` overrides retain the
`Make → Prototype → Construct` pattern. Implement the transport's backend,
`Send`, and `DoClose` as usual; replacing the complete reader does not require a
new interface method or server option. At the end of a custom stream constructor,
use `transports.StartReader` instead of starting the reader goroutine directly:

```go
func newCustomTransport(ctx *types.HttpContext) transports.Transport {
    t := &customTransport{Transport: transports.MakeTransport()}
    t.Prototype(t)
    t.Construct(ctx)
    return t
}

func (t *customTransport) Construct(ctx *types.HttpContext) {
    t.Transport.Construct(ctx)
    // Initialize the backend, writer, and close/error handlers here.
    transports.StartReader(t, ctx.TransportReadPermission(), t.read)
}
```

Call `StartReader` once per transport, without an extra `go` statement. It owns
the permission channel's single read; other code must not consume that result.
A nil permission starts the reader automatically, preserving standalone use.
It accepts an `opening` transport: `read` may wait for backend readiness, while
the transport controls `Writable` and emits `ready` when it can send. The helper
only delays reader startup and does not implement backend readiness or cleanup.

Engine.IO supplies this permission for its WebSocket and WebTransport connection
paths, including custom constructors using those paths. A different underlying
connection or a custom server entry point must manage its own initialization and
cancellation; it may pass its own permission channel to `StartReader`. Constructors
must not emit inbound packets synchronously before connection listeners exist.
An override that delegates to a built-in constructor already uses the helper and
must not start a second reader.

Polling transports may fully override `DoWrite`. Prepare the response headers and
body, commit with `ctx.Write` (or `io.Copy` into `ctx`), then invoke the supplied
completion callback. The next GET can be accepted once the response is committed,
even if the writer or callback has not returned. Request ownership is managed by
the base transport; overrides do not need to replace or invoke `ctx.Cleanup`.

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

- Go 1.26.0+
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
