# Socket.IO Client for Go

[![Build Status](https://github.com/zishang520/socket.io/clients/socket/v3/actions/workflows/go.yml/badge.svg)](https://github.com/zishang520/socket.io/clients/socket/v3/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/clients/socket/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/clients/socket/v3)

A robust Go client implementation for [Socket.IO](http://github.com/zishang520/engine.io), the reliable real-time bidirectional communication layer that powers [Socket.IO](http://github.com/zishang520/socket.io).

## Features

- Haven't written it yet.

## Installation

```bash
go get github.com/zishang520/socket.io/clients/socket/v3
```

## Quick Start

### Basic Usage

```go
package main

import (
    "time"

    "github.com/zishang520/socket.io/clients/engine/v3/transports"
    "github.com/zishang520/socket.io/servers/engine/v3/types"
    "github.com/zishang520/socket.io/servers/engine/v3/utils"
    "github.com/zishang520/socket.io/clients/socket/v3"
)

func main() {
    opts := socket.DefaultOptions()
    opts.SetTransports(types.NewSet(transports.Polling, transports.WebSocket /*transports.WebTransport*/))

    manager := socket.NewManager("http://127.0.0.1:3000", opts)
    // Listening to manager events
    manager.On("error", func(errs ...any) {
        utils.Log().Warning("Manager Error: %v", errs)
    })

    manager.On("ping", func(...any) {
        utils.Log().Warning("Manager Ping")
    })

    manager.On("reconnect", func(...any) {
        utils.Log().Warning("Manager Reconnected")
    })

    manager.On("reconnect_attempt", func(...any) {
        utils.Log().Warning("Manager Reconnect Attempt")
    })

    manager.On("reconnect_error", func(errs ...any) {
        utils.Log().Warning("Manager Reconnect Error: %v", errs)
    })

    manager.On("reconnect_failed", func(errs ...any) {
        utils.Log().Warning("Manager Reconnect Failed: %v", errs)
    })
    io := manager.Socket("/custom", opts)
    utils.Log().Error("socket %v", io)
    io.On("connect", func(args ...any) {
        utils.Log().Warning("io iD %v", io.Id())
        utils.SetTimeout(func() {
            io.Emit("message", types.NewStringBufferString("test"))
        }, 1*time.Second)
        utils.Log().Warning("connect %v", args)
    })

    io.On("connect_error", func(args ...any) {
        utils.Log().Warning("connect_error %v", args)
    })

    io.On("disconnect", func(args ...any) {
        utils.Log().Warning("disconnect: %+v", args)
    })

    io.OnAny(func(args ...any) {
        utils.Log().Warning("OnAny: %+v", args)
    })

    io.On("message-back", func(args ...any) {
        // io.Emit("message", types.NewStringBufferString("88888"))
        utils.Log().Question("message-back: %+v", args)
    })
}
```

## Development

### Running Tests

```bash
git clone https://github.com/zishang520/socket.io/clients/socket/v3.git
cd socket.io-client-go
go test ./...
```

### Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License

Copyright (c) 2025 luoyy

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## Related Projects

- [Socket.IO Protocol](https://github.com/socketio/socket.io-protocol)
- [Socket.IO Server](https://github.com/zishang520/socket.io)