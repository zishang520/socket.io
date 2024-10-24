# socket.io for golang

[![Build Status](https://github.com/zishang520/socket.io/workflows/Go/badge.svg?branch=main)](https://github.com/zishang520/socket.io/actions)
[![GoDoc](https://pkg.go.dev/badge/github.com/zishang520/socket.io/v2?utm_source=godoc)](https://pkg.go.dev/github.com/zishang520/socket.io/v2)

## Features

Socket.IO enables real-time bidirectional event-based communication. It consists of:

- **Support Socket.IO v4+ 🚀🚀🚀**
- a Golang server (this repository)
- a [Javascript client library](https://github.com/socketio/socket.io-client) for the browser (or a Node.js client)

Some implementations in other languages are also available:

- [Java](https://github.com/socketio/socket.io-client-java)
- [C++](https://github.com/socketio/socket.io-client-cpp)
- [Swift](https://github.com/socketio/socket.io-client-swift)
- [Dart](https://github.com/rikulo/socket.io-client-dart)
- [Python](https://github.com/miguelgrinberg/python-socketio)
- [.NET](https://github.com/doghappy/socket.io-client-csharp)

Its main features are:

#### Reliability

Connections are established even in the presence of:
  - proxies and load balancers.
  - personal firewall and antivirus software.

For this purpose, it relies on [Engine.IO for golang](https://github.com/zishang520/engine.io), which first establishes a long-polling connection, then tries to upgrade to better transports that are "tested" on the side, like WebSocket. Please see the [Goals](https://github.com/zishang520/engine.io#goals) section for more information.

#### Auto-reconnection support

Unless instructed otherwise a disconnected client will try to reconnect forever, until the server is available again. Please see the available reconnection options [here](https://socket.io/docs/v3/client-api/#new-Manager-url-options).

#### Disconnection detection

A heartbeat mechanism is implemented at the Engine.IO level, allowing both the server and the client to know when the other one is not responding anymore.

That functionality is achieved with timers set on both the server and the client, with timeout values (the `pingInterval` and `pingTimeout` parameters) shared during the connection handshake. Those timers require any subsequent client calls to be directed to the same server, hence the `sticky-session` requirement when using multiples nodes.

#### Binary support

Any serializable data structures can be emitted, including:

- `[]byte` and `io.Reader`


#### Simple and convenient API

Sample code:

```golang
import (
    "github.com/zishang520/socket.io/v2/socket"
)
io.On("connection", func(clients ...any) {
    client := clients[0].(*socket.Socket)
    client.Emit("request" /* … */)                       // emit an event to the socket
    io.Emit("broadcast" /* … */)                         // emit an event to all connected sockets
    client.On("reply", func(...any) { /* … */ }) // listen to the event
})
```

#### Multiplexing support

In order to create separation of concerns within your application (for example per module, or based on permissions), Socket.IO allows you to create several `Namespaces`, which will act as separate communication channels but will share the same underlying connection.

#### Room support

Within each `Namespace`, you can define arbitrary channels, called `Rooms`, that sockets can join and leave. You can then broadcast to any given room, reaching every socket that has joined it.

This is a useful feature to send notifications to a group of users, or to a given user connected on several devices for example.


**Note:** Socket.IO is not a WebSocket implementation. Although Socket.IO indeed uses WebSocket as a transport when possible, it adds some metadata to each packet: the packet type, the namespace and the ack id when a message acknowledgement is needed. That is why a WebSocket client will not be able to successfully connect to a Socket.IO server, and a Socket.IO client will not be able to connect to a WebSocket server (like `ws://echo.websocket.org`) either. Please see the protocol specification [here](https://github.com/socketio/socket.io-protocol).


## How to use

The following example attaches socket.io to a plain engine.io *types.CreateServer listening on port `3000`.
```golang
package main

import (
    "github.com/zishang520/engine.io/v2/types"
    "github.com/zishang520/engine.io/v2/utils"
    "github.com/zishang520/socket.io/v2/socket"
    "os"
    "os/signal"
    "syscall"
)

func main() {
    httpServer := types.CreateServer(nil)
    io := socket.NewServer(httpServer, nil)
    io.On("connection", func(clients ...any) {
        client := clients[0].(*socket.Socket)
        client.On("event", func(datas ...any) {
        })
        client.On("disconnect", func(...any) {
        })
    })
    httpServer.Listen("127.0.0.1:3000", nil)

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

other: Use [http.Handler](https://pkg.go.dev/net/http#Handler) interface
```golang
package main

import (
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/zishang520/socket.io/v2/socket"
)

func main() {
    io := socket.NewServer(nil, nil)
    http.Handle("/socket.io/", io.ServeHandler(nil))
    go http.ListenAndServe(":3000", nil)

    io.On("connection", func(clients ...any) {
        client := clients[0].(*socket.Socket)
        client.On("event", func(datas ...any) {
        })
        client.On("disconnect", func(...any) {
        })
    })

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
    io.Close(nil)
    os.Exit(0)
}

```

other: Use fasthttp (fasthttp version 1.56.0+ is required)
```golang
package main

import (
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/valyala/fasthttp"
    "github.com/valyala/fasthttp/fasthttpadaptor"
    "github.com/zishang520/engine.io/v2/log"
    "github.com/zishang520/engine.io/v2/types"
    "github.com/zishang520/socket.io/v2/socket"
)

func main() {
    log.DEBUG = true
    c := socket.DefaultServerOptions()
    c.SetServeClient(true)
    // c.SetConnectionStateRecovery(&socket.ConnectionStateRecovery{})
    // c.SetAllowEIO3(true)
    c.SetPingInterval(300 * time.Millisecond)
    c.SetPingTimeout(200 * time.Millisecond)
    c.SetMaxHttpBufferSize(1000000)
    c.SetConnectTimeout(1000 * time.Millisecond)
    c.SetCors(&types.Cors{
        Origin:      "*",
        Credentials: true,
    })
    socketio := socket.NewServer(nil, nil)
    socketio.On("connection", func(clients ...interface{}) {
        client := clients[0].(*socket.Socket)

        client.On("message", func(args ...interface{}) {
            client.Emit("message-back", args...)
        })
        client.Emit("auth", client.Handshake().Auth)

        client.On("message-with-ack", func(args ...interface{}) {
            ack := args[len(args)-1].(socket.Ack)
            ack(args[:len(args)-1], nil)
        })
    })

    socketio.Of("/custom", nil).On("connection", func(clients ...interface{}) {
        client := clients[0].(*socket.Socket)
        client.Emit("auth", client.Handshake().Auth)
    })

    go fasthttp.ListenAndServe(":3000", fasthttpadaptor.NewFastHTTPHandler(socketio.ServeHandler(c)))

    exit := make(chan struct{})
    SignalC := make(chan os.Signal)

    signal.Notify(SignalC, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go func() {
        for s := range SignalC {
            switch s {
            case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
                close(exit)
                return
            }
        }
    }()

    <-exit
    socketio.Close(nil)
    os.Exit(0)
}

```

other: Use fiber (fasthttp version 1.56.0+ is required)
```golang
package main

import (
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/adaptor"
    "github.com/zishang520/engine.io/v2/log"
    "github.com/zishang520/engine.io/v2/types"
    "github.com/zishang520/socket.io/v2/socket"
)

func main() {
    log.DEBUG = true
    c := socket.DefaultServerOptions()
    c.SetServeClient(true)
    // c.SetConnectionStateRecovery(&socket.ConnectionStateRecovery{})
    // c.SetAllowEIO3(true)
    c.SetPingInterval(300 * time.Millisecond)
    c.SetPingTimeout(200 * time.Millisecond)
    c.SetMaxHttpBufferSize(1000000)
    c.SetConnectTimeout(1000 * time.Millisecond)
    c.SetCors(&types.Cors{
        Origin:      "*",
        Credentials: true,
    })
    socketio := socket.NewServer(nil, nil)
    socketio.On("connection", func(clients ...interface{}) {
        client := clients[0].(*socket.Socket)

        client.On("message", func(args ...interface{}) {
            client.Emit("message-back", args...)
        })
        client.Emit("auth", client.Handshake().Auth)

        client.On("message-with-ack", func(args ...interface{}) {
            ack := args[len(args)-1].(socket.Ack)
            ack(args[:len(args)-1], nil)
        })
    })

    socketio.Of("/custom", nil).On("connection", func(clients ...interface{}) {
        client := clients[0].(*socket.Socket)
        client.Emit("auth", client.Handshake().Auth)
    })

    app := fiber.New()

    // app.Put("/socket.io", adaptor.HTTPHandler(socketio.ServeHandler(c))) // test
    app.Get("/socket.io", adaptor.HTTPHandler(socketio.ServeHandler(c)))
    app.Post("/socket.io", adaptor.HTTPHandler(socketio.ServeHandler(c)))

    go app.Listen(":3000")

    exit := make(chan struct{})
    SignalC := make(chan os.Signal)

    signal.Notify(SignalC, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go func() {
        for s := range SignalC {
            switch s {
            case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
                close(exit)
                return
            }
        }
    }()

    <-exit
    socketio.Close(nil)
    os.Exit(0)
}

```

## Documentation

Please see the documentation [here](https://pkg.go.dev/github.com/zishang520/socket.io/v2).

## Debug / logging

In order to see all the debug output, run your app with the environment variable
`DEBUG` including the desired scope.

To see the output from all of Socket.IO's debugging scopes you can use:

```
DEBUG=socket.io*
```

## Testing

```
make test
```




## License

[MIT](LICENSE)
