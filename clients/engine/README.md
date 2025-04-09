# Engine.IO Client for Go

[![Build Status](https://github.com/zishang520/socket.io/clients/engine/v3/actions/workflows/go.yml/badge.svg)](https://github.com/zishang520/socket.io/clients/engine/v3/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/clients/engine/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/clients/engine/v3)

A robust Go client implementation for [Engine.IO](http://github.com/zishang520/engine.io), the reliable real-time bidirectional communication layer that powers [Socket.IO](http://github.com/zishang520/socket.io).

## Features

- Haven't written it yet.

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
    "time"
    eio "github.com/zishang520/socket.io/clients/engine/v3"
    "github.com/zishang520/socket.io/servers/engine/v3/utils"
    "github.com/zishang520/socket.io/servers/engine/v3/types"
)

func main() {
    socket := eio.NewSocket("ws://localhost", nil)
    
    socket.On("open", func(args ...any) {
        log.Println("Connection established")
        
        // Send a message after 1 second
        utils.SetTimeout(func() {
            socket.Send(types.NewStringBufferString("Hello, Server!"), nil, nil)
        }, 1*time.Second)
    })

    socket.On("message", func(args ...any) {
        log.Printf("Received message: %v", args[0])
    })

    socket.On("close", func(args ...any) {
        log.Println("Connection closed")
    })
}
```

### Advanced Configuration

```go
package main

import (
    "github.com/zishang520/socket.io/clients/engine/v3"
    "github.com/zishang520/socket.io/clients/engine/v3/transports"
    "github.com/zishang520/socket.io/servers/engine/v3/types"
)

func main() {
    // Create custom socket options
    opts := engine.DefaultSocketOptions()
    
    // Configure connection settings
    opts.SetPath("/engine.io")
    opts.SetQuery(map[string][]string{
        "token": {"abc123"},
    })
    
    // Specify preferred transports
    opts.SetTransports(types.NewSet(
        transports.WebSocket,
        transports.Polling,
    ))
    
    // Configure timeouts
    opts.SetRequestTimeout(time.Second * 10)
    
    // Create socket with custom options
    socket := engine.NewSocket("ws://localhost", opts)
    
    // Handle events
    socket.On("open", func(args ...any) {
        // Connection established
    })
}
```

## Development

### Running Tests

```bash
git clone https://github.com/zishang520/socket.io/clients/engine/v3.git
cd engine.io-client-go
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

- [Engine.IO Protocol](https://github.com/socketio/engine.io-protocol)
- [Engine.IO Server](https://github.com/zishang520/engine.io)
- [Socket.IO](https://github.com/zishang520/socket.io)