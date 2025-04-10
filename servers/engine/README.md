# Engine.IO: the realtime engine for golang

[![Build Status](https://github.com/zishang520/engine.io/actions/workflows/go.yml/badge.svg)](https://github.com/zishang520/engine.io/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/zishang520/socket.io/servers/engine/v3?utm_source=godoc)](https://pkg.go.dev/github.com/zishang520/socket.io/servers/engine/v3)

`Engine.IO` is the implementation of transport-based
cross-browser/cross-device bi-directional communication layer for
[Socket.IO for golang](http://github.com/zishang520/socket.io).

## How to use

### Server

#### (A) Listening on a port

```golang
package main

import (
    "os"
    "os/signal"
    "strings"
    "syscall"

    "github.com/zishang520/socket.io/servers/engine/v3/config"
    "github.com/zishang520/socket.io/servers/engine/v3"
    "github.com/zishang520/socket.io/v3/pkg/types"
    "github.com/zishang520/socket.io/v3/pkg/utils"
)

func main() {
    serverOptions := &config.ServerOptions{}
    serverOptions.SetAllowEIO3(true)
    serverOptions.SetCors(&types.Cors{
        Origin:      "*",
        Credentials: true,
    })

    engineServer := engine.Listen("127.0.0.1:4444", serverOptions, nil)
    engineServer.On("connection", func(sockets ...any) {
        socket := sockets[0].(engine.Socket)
        socket.Send(strings.NewReader("utf 8 string"), nil, nil)
        socket.Send(types.NewBytesBuffer([]byte{0, 1, 2, 3, 4, 5}), nil, nil)
        socket.Send(types.NewBytesBufferString("BufferString by string"), nil, nil)
        socket.Send(types.NewStringBuffer([]byte("StringBuffer by byte")), nil, nil)
        socket.Send(types.NewStringBufferString("StringBuffer by string"), nil, nil)
        socket.On("message", func(...any) {
            // socket.Send(strings.NewReader("utf 8 string"), nil, nil)
        })
    })
    utils.Log().Println("%v", engineServer)

    exit := make(chan struct{})
    SignalC := make(chan os.Signal)

    signal.Notify(SignalC, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go func() {
        for s := range SignalC {
            switch s {
            case os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
                close(exit)
                return
            }
        }
    }()

    <-exit
    os.Exit(0)
}

```

#### (B) Intercepting requests for a `*types.HttpServer`

```golang
package main

import (
    "os"
    "os/signal"
    "syscall"

    "github.com/zishang520/socket.io/servers/engine/v3/config"
    "github.com/zishang520/socket.io/servers/engine/v3"
    "github.com/zishang520/socket.io/v3/pkg/types"
    "github.com/zishang520/socket.io/v3/pkg/utils"
)

func main() {
    serverOptions := &config.ServerOptions{}
    serverOptions.SetAllowEIO3(true)
    serverOptions.SetCors(&types.Cors{
        Origin:      "*",
        Credentials: true,
    })

    httpServer := types.NewWebServer(nil)
    httpServer.Listen("127.0.0.1:4444", nil)

    engineServer := engine.Attach(httpServer, serverOptions)

    engineServer.On("connection", func(sockets ...any) {
        socket := sockets[0].(engine.Socket)
        socket.On("message", func(...any) {
        })
        socket.Once("close", func(...any) {
            utils.Log().Println("client close.")
        })
    })
    utils.Log().Println("%v", engineServer)

    exit := make(chan struct{})
    SignalC := make(chan os.Signal)

    signal.Notify(SignalC, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go func() {
        for s := range SignalC {
            switch s {
            case os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
                close(exit)
                return
            }
        }
    }()

    <-exit
    os.Exit(0)
}
```

#### (C) Passing in requests

```golang
package main

import (
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/gorilla/websocket"
    "github.com/zishang520/socket.io/servers/engine/v3/config"
    "github.com/zishang520/socket.io/servers/engine/v3"
    "github.com/zishang520/socket.io/v3/pkg/types"
    "github.com/zishang520/socket.io/v3/pkg/utils"
)

func main() {
    serverOptions := &config.ServerOptions{}
    serverOptions.SetAllowEIO3(true)
    serverOptions.SetCors(&types.Cors{
        Origin:      "*",
        Credentials: true,
    })

    httpServer := types.NewWebServer(nil)
    httpServer.Listen("127.0.0.1:4444", nil)

    engineServer := engine.New(serverOptions)

    httpServer.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if !websocket.IsWebSocketUpgrade(r) {
            engineServer.HandleRequest(types.NewHttpContext(w, r))
        } else if engineServer.Opts().Transports().Has("websocket") {
            engineServer.HandleUpgrade(types.NewHttpContext(w, r))
        } else {
            httpServer.DefaultHandler.ServeHTTP(w, r)
        }
    })

    engineServer.On("connection", func(sockets ...any) {
        socket := sockets[0].(engine.Socket)
        socket.On("message", func(...any) {
        })
        socket.Once("close", func(...any) {
            utils.Log().Println("client close.")
        })
    })
    utils.Log().Println("%v", engineServer)

    exit := make(chan struct{})
    SignalC := make(chan os.Signal)

    signal.Notify(SignalC, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go func() {
        for s := range SignalC {
            switch s {
            case os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
                close(exit)
                return
            }
        }
    }()

    <-exit
    httpServer.Close(nil)
    os.Exit(0)
}

```

#### (D) Passing in requests (http.Handler interface)

```golang
package main

import (
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/zishang520/socket.io/servers/engine/v3/config"
    "github.com/zishang520/socket.io/servers/engine/v3"
    "github.com/zishang520/socket.io/v3/pkg/types"
    "github.com/zishang520/socket.io/v3/pkg/utils"
)

func main() {
    serverOptions := &config.ServerOptions{}
    serverOptions.SetAllowEIO3(true)
    serverOptions.SetCors(&types.Cors{
        Origin:      "*",
        Credentials: true,
    })

    engineServer := engine.New(serverOptions)

    engineServer.On("connection", func(sockets ...any) {
        socket := sockets[0].(engine.Socket)
        socket.On("message", func(...any) {
        })
        socket.Once("close", func(...any) {
            utils.Log().Println("client close.")
        })
    })

    http.Handle("/engine.io/", engineServer)
    go http.ListenAndServe(":8090", nil)

    utils.Log().Println("%v", engineServer)

    exit := make(chan struct{})
    SignalC := make(chan os.Signal)

    signal.Notify(SignalC, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go func() {
        for s := range SignalC {
            switch s {
            case os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
                close(exit)
                return
            }
        }
    }()

    <-exit

    // Need to handle server shutdown disconnecting client connections.
    engineServer.Close()
    os.Exit(0)
}
```

#### (E) Passing in requests (WebTransport)

```golang
package main

import (
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/gorilla/websocket"
    "github.com/zishang520/socket.io/servers/engine/v3/config"
    "github.com/zishang520/socket.io/servers/engine/v3"
    "github.com/zishang520/socket.io/v3/pkg/types"
    "github.com/zishang520/socket.io/v3/pkg/utils"
    "github.com/zishang520/socket.io/v3/pkg/webtransport"
)

func main() {
    serverOptions := &config.ServerOptions{}
    serverOptions.SetAllowEIO3(true)
    serverOptions.SetCors(&types.Cors{
        Origin:      "*",
        Credentials: true,
    })
    // serverOptions.SetTransports(types.NewSet("polling", "webtransport"))
    serverOptions.SetTransports(types.NewSet("polling", "websocket", "webtransport"))

    httpServer := types.NewWebServer(nil)
    httpServer.ListenTLS(":443", "server.crt", "server.key", nil)
    wts := httpServer.ListenWebTransportTLS(":443", "server.crt", "server.key", nil, nil)

    engineServer := engine.New(serverOptions)

    httpServer.HandleFunc("/engine.io/", func(w http.ResponseWriter, r *http.Request) {
        if webtransport.IsWebTransportUpgrade(r) {
            engineServer.OnWebTransportSession(types.NewHttpContext(w, r), wts)
        } else if !websocket.IsWebSocketUpgrade(r) {
            engineServer.HandleRequest(types.NewHttpContext(w, r))
        } else if engineServer.Opts().Transports().Has("websocket") {
            engineServer.HandleUpgrade(types.NewHttpContext(w, r))
        } else {
            httpServer.DefaultHandler.ServeHTTP(w, r)
        }
    })

    engineServer.On("connection", func(sockets ...any) {
        socket := sockets[0].(engine.Socket)
        socket.On("message", func(...any) {
        })
        socket.Once("close", func(...any) {
            utils.Log().Println("client close.")
        })
    })
    utils.Log().Println("%v", engineServer)

    exit := make(chan struct{})
    SignalC := make(chan os.Signal)

    signal.Notify(SignalC, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go func() {
        for s := range SignalC {
            switch s {
            case os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
                close(exit)
                return
            }
        }
    }()

    <-exit
    engineServer.Close()
    httpServer.Close(nil)
    os.Exit(0)
}
```

### Client

```golang
package main

import (
 "context"
 "os"
 "os/signal"
 "syscall"

 "github.com/zishang520/socket.io/clients/engine/v3"
 "github.com/zishang520/socket.io/clients/engine/v3/transports"
 "github.com/zishang520/socket.io/v3/pkg/log"
 "github.com/zishang520/socket.io/v3/pkg/types"
 "github.com/zishang520/socket.io/v3/pkg/utils"
)

func main() {
 log.DEBUG = true
 opts := engine.DefaultSocketOptions()
 opts.SetTransports(types.NewSet(transports.Polling /*transports.WebSocket, transports.WebTransport*/))

 e := engine.NewSocket("http://127.0.0.1:4444", opts)
 e.On("open", func(args ...any) {
  e.Send(types.NewStringBufferString("88888"), nil, nil)
  utils.Log().Debug("close %v", args)
 })

 e.On("close", func(args ...any) {
  utils.Log().Debug("close %v", args)
 })

 e.On("packet", func(args ...any) {
  utils.Log().Warning("packet: %+v", args)
 })

 e.On("ping", func(...any) {
  utils.Log().Warning("ping")
 })

 e.On("pong", func(...any) {
  utils.Log().Warning("pong")
 })

 e.On("message", func(args ...any) {
  e.Send(types.NewStringBufferString("6666666"), nil, nil)
  utils.Log().Warning("message %v", args)
 })

 e.On("heartbeat", func(...any) {
  utils.Log().Debug("heartbeat")
 })

 ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

 defer stop()
 <-ctx.Done()

 e.Close()
}
```

```html
<script src="engine.io.js"></script>
<script>
  const socket = new eio.Socket('ws://localhost:4444');
  socket.on('open', () => {
    socket.on('message', data => {});
    socket.on('close', () => {});
  });
</script>
```

For more information on the client refer to the
[engine-client-golang](https://github.com/zishang520/socket.io/clients/engine/v3) repository.

## What features does it have?

- **Support engine-client 3+**

## API

### Server

<hr><br>

#### Top-level

These are exposed by `import "github.com/zishang520/socket.io/servers/engine/v3"`:

##### Events

- `flush`
    - Called when a socket buffer is being flushed.
    - **Arguments**
      - `engine.Socket`: socket being flushed
      - `[]*packet.Packet`: write buffer
- `drain`
    - Called when a socket buffer is drained
    - **Arguments**
      - `engine.Socket`: socket being flushed

##### Constants and types

- `Protocol` _(int)_: protocol revision number
- `Server`: Server struct
- `Socket`: Socket struct

##### Methods

- `New`
    - Returns a new `Server` instance. If the first argument is an `*types.HttpServer`() then the
      new `Server` instance will be attached to it. Otherwise, the arguments are passed
      directly to the `Server` constructor.
    - **Parameters**
      - `*types.HttpServer`: optional, server to attach to.
      - `any`: can be nil, interface `config.ServerOptionsInterface` or `config.AttachOptionsInterface`

  The following are identical ways to instantiate a server and then attach it.

```golang
import "github.com/zishang520/socket.io/servers/engine/v3/config"
import "github.com/zishang520/socket.io/servers/engine/v3"
import "github.com/zishang520/socket.io/v3/pkg/types"

var httpServer *types.HttpServer // previously created with `types.NewWebServer(nil);`.
var eioServer engine.Server

// create a server first, and then attach
eioServer = engine.NewServer(nil)
eioServer.Attach(httpServer)

// or call the module as a function to get `Server`
eioServer = engine.New(nil)
eioServer.Attach(httpServer)

// immediately attach
eioServer = engine.New(httpServer)

// with custom options
c := &config.ServerOptions{}
c.SetMaxHttpBufferSize(1e3)
eioServer = engine.New(httpServer, c)

```

- `Listen`
    - Creates an `*types.HttpServer` which listens on the given port and attaches WS
      to it. It returns `501 Not Implemented` for regular http requests.
    - **Parameters**
      - `string`: address to listen on.
      - `any`: can be nil, interface `config.ServerOptionsInterface` or `config.AttachOptionsInterface`.
      - `func()`: callback for `listen`.
    - **Options**
      - All options from `engine.Server.Attach` method, documented below.
      - **Additionally** See Server `New` below for options you can pass for creating the new Server
    - **Returns** `engine.Server`

```golang
import "github.com/zishang520/socket.io/servers/engine/v3"
import "github.com/zishang520/socket.io/servers/engine/v3/config"

c := &config.ServerOptions{}
c.SetPingTimeout(2000)
c.SetPingInterval(10_000)

const server = engine.Listen("127.0.0.1:3000", c);

server.On('connection', func(...any) {});
```

- `Attach`
    - Captures `upgrade` requests for a `*types.HttpServer`. In other words, makes
      a regular `*types.HttpServer` WebSocket-compatible.
    - **Parameters**
      - `*types.HttpServer`: server to attach to.
      - `any`: `config.ServerOptionsInterface`: can be nil, interface config.ServerOptionsInterface or config.AttachOptionsInterface
    - **Options**
      - All options from `engine.Server.attach` method, documented below.
      - **Additionally** See Server `New` below for options you can pass for creating the new Server
    - **Returns** `engine.Server` a new Server instance.

#### engine.Server

The main server/manager. _Inherits from types.EventEmitter_.

##### Events

- `connection`
    - Fired when a new connection is established.
    - **Arguments**
      - `engine.Socket`: a Socket object

- `initial_headers`
    - Fired on the first request of the connection, before writing the response headers
    - **Arguments**
      - `headers` (`*utils.ParameterBag`): a hash of headers
      - `ctx` (`*types.HttpContext`): the request

- `headers`
    - Fired on the all requests of the connection, before writing the response headers
    - **Arguments**
      - `headers` (`*utils.ParameterBag`): a hash of headers
      - `ctx` (`*types.HttpContext`): the request

- `connection_error`
    - Fired when an error occurs when establishing the connection.
    - **Arguments**
      - `types.ErrorMessage`: an object with following properties:
        - `req` (`*types.HttpContext`): the request that was dropped
        - `code` (`int`): one of `Server.errors`
        - `message` (`string`): one of `Server.errorMessages`
        - `context` (`map[string]any`): extra info about the error

| Code | Message |
| ---- | ------- |
| 0 | "Transport unknown"
| 1 | "Session ID unknown"
| 2 | "Bad handshake method"
| 3 | "Bad request"
| 4 | "Forbidden"
| 5 | "Unsupported protocol version"

##### Read-only methods

**Important**: if you plan to use Engine.IO in a scalable way, please
keep in mind the properties below will only reflect the clients connected
to a single process.

- `Clients()` _(*types.Map\[string, engine.Socket\])_: hash of connected clients by id.
- `ClientsCount()` _(uint64)_: number of connected clients.

##### Methods

- **New**
    - Initializes the server
    - **Parameters**
      - `config.ServerOptionsInterface`: can be nil, interface config.ServerOptionsInterface
    - **Options**
      - `SetPingTimeout(time.Duration)`: how many ms without a pong packet to
        consider the connection closed (`20_000 * time.Millisecond`)
      - `SetPingInterval(time.Duration)`: how many ms before sending a new ping
        packet (`25_000 * time.Millisecond`)
      - `SetUpgradeTimeout(time.Duration)`: how many ms before an uncompleted transport upgrade is cancelled (`10_000 * time.Millisecond`)
      - `SetMaxHttpBufferSize(int64)`: how many bytes or characters a message
        can be, before closing the session (to avoid DoS). Default
        value is `1E6`.
      - `SetAllowRequest(config.AllowRequest)`: A function that receives a given handshake or upgrade request as its first argument and can decide whether to continue. error is not empty to indicate that the request was rejected.
      - `SetTransports(*types.Set[string])`: transports to allow connections
        to (`['polling', 'websocket']`)
      - `SetAllowUpgrades(bool)`: whether to allow transport upgrades
        (`true`)
      - `SetPerMessageDeflate(*types.PerMessageDeflate)`: parameters of the WebSocket permessage-deflate extension
        - `Threshold` (`int`): data is compressed only if the byte size is above this value (`1024`)
      - `SetHttpCompression(*types.HttpCompression)`: parameters of the http compression for the polling transports
        - `Threshold` (`int`): data is compressed only if the byte size is above this value (`1024`)
      - `SetCookie(*http.Cookie)`: configuration of the cookie that
        contains the client sid to send as part of handshake response
        headers. This cookie might be used for sticky-session. Defaults to not sending any cookie (`nil`).
      - `SetCors(*types.Cors)`: the options that will be forwarded to the cors module. See [there](https://pkg.go.dev/github.com/zishang520/socket.io/v3/pkg/types#Cors) for all available options. Defaults to no CORS allowed.
      - `SetInitialPacket(io.Reader)`: an optional packet which will be concatenated to the handshake packet emitted by Engine.IO.
      - `SetAllowEIO3(bool)`: whether to support v3 Engine.IO clients (defaults to `false`)
- `Close`
    - Closes all clients
    - **Returns** `engine.Server` for chaining
- `HandleRequest`
    - Called internally when a `Engine` request is intercepted.
    - **Parameters**
      - `*types.HttpContext`: a node request context
- `HandleUpgrade`
    - Called internally when a `Engine` ws upgrade is intercepted.
    - **Parameters**
      - `*types.HttpContext`: a node request context
- `Attach`
    - Attach this Server instance to an `*types.HttpServer`
    - Captures `upgrade` requests for a `*types.HttpServer`. In other words, makes
      a regular *types.HttpServer WebSocket-compatible.
    - **Parameters**
      - `*types.HttpServer`: server to attach to.
      - `any`: can be nil, interface config.AttachOptionsInterface
    - **Options**
      - `SetPath(string)`: name of the path to capture (`/engine.io`).
      - ~~`SetDestroyUpgrade(bool)`~~: destroy unhandled upgrade requests (`true`)
      - ~~`SetDestroyUpgradeTimeout(time.Duration)`~~: milliseconds after which unhandled requests are ended (`1000 * time.Millisecond`)
      - `SetAddTrailingSlash(bool)`: Whether we should add a trailing slash to the request path (`true`)
- `GenerateId`
    - Generate a socket id.
    - Overwrite this method to generate your custom socket id.
    - **Parameters**
      - `*types.HttpContext`: a node request context
  - **Returns** A socket id for connected client.

<hr><br>

#### engine.Socket

A representation of a client. _Inherits from types.EventEmitter_.

##### Events

- `close`
    - Fired when the client is disconnected.
    - **Arguments**
      - `string`: reason for closing
      - `any`: description (optional)
- `message`
    - Fired when the client sends a message.
    - **Arguments**
      - `io.Reader`: `*types.StringBuffer` or `*types.BytesBuffer` with binary contents
- `error`
    - Fired when an error occurs.
    - **Arguments**
      - `error`: error type
- `flush`
    - Called when the write buffer is being flushed.
    - **Arguments**
      - `[]*packet.Packet`: write buffer
- `drain`
    - Called when the write buffer is drained
- `packet`
    - Called when a socket received a packet (`message`, `ping`)
    - **Arguments**
      - `*packet.Packet`: packet
- `packetCreate`
    - Called before a socket sends a packet (`message`, `ping`)
    - **Arguments**
      - `*packet.Packet`: packet
- `heartbeat`
    - Called when `ping` or `pong` packed is received (depends of client version)

##### Read-only methods

- `Id()` _(string)_: unique identifier
- `Server()` _(engine.Server)_: engine parent reference
- `Request()` _(*types.HttpContext)_: request that originated the Socket
- `Upgraded()` _(bool)_: whether the transport has been upgraded
- `ReadyState()` _(string)_: opening|open|closing|closed
- `Transport()` _(transports.Transport)_: transport reference

##### Methods

- `Send`:
    - Sends a message.
    - **Parameters**
      - `io.Reader`: `*types.StringBuffer` and `*strings.Reader` are treated as strings, others that implement the `io.Reader` interface are treated as binary.
      - `*packet.Options`: can be nil, Options struct.
      - `func(transports.Transport)`: can be nil, a callback executed when the message gets flushed out by the transport
    - **\*packet.Options**
      - `Compress` (`bool`): whether to compress sending data. This option might be ignored and forced to be `true` when using polling. (`true`)
    - **Returns** `engine.Socket` for chaining
- `Close`
    - Disconnects the client
    - **Parameters**
      - `bool`: Flags the transport as discarded. (`false`)

### Client

<hr><br>

Exposed in the `eio` global namespace (in the browser), or by
`require('engine.io-client')` (in Node.JS).

For the client API refer to the
[engine-client](https://github.com/socketio/engine.io-client) repository.

## Debug / logging

In order to see all the debug output, run your app with the environment variable
`DEBUG` including the desired scope.

To see the output from all of Engine.IO's debugging scopes you can use:

```
DEBUG=engine*
```

## Transports

- `polling`: XHR / JSONP polling transport.
- `websocket`: WebSocket transport.
- `webtransport`: WebTransport transport.

## Tests

Tests run with `make test`.

## License


MIT License

Copyright (c) 2023 luoyy

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
