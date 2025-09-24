# socket.io-go-redis

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/adapters/redis/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/adapters/redis/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/adapters/redis/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/adapters/redis/v3)

## Description

A Redis adapter for Socket.IO server in Go, allowing to scale Socket.IO applications across multiple processes or servers.

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/redis/v3
```

## Features

- Multiple servers support
- Real-time communication between processes
- Automatic reconnection
- Custom Redis configuration
- Event emission across servers

## How to use

Basic usage example:

```golang
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    rds "github.com/redis/go-redis/v9"
    "github.com/zishang520/socket.io/adapters/redis/v3"
    "github.com/zishang520/socket.io/adapters/redis/v3/adapter"
    "github.com/zishang520/socket.io/servers/socket/v3"
)

func main() {
    // Initialize Redis client
    redisClient := redis.NewRedisClient(context.TODO(), rds.NewClient(&rds.Options{
        Addr:     "127.0.0.1:6379",
        Username: "",
        Password: "",
        DB:       0,
    }))

    // Redis error handling
    redisClient.On("error", func(a ...any) {
        fmt.Println(a)
    })

    // Socket.IO server configuration
    config := socket.DefaultServerOptions()
    config.SetAdapter(&adapter.RedisAdapterBuilder{
        Redis: redisClient,
        Opts:  &adapter.RedisAdapterOptions{},
    })

    // Create and configure server
    httpServer := s.CreateServer(nil)
    io := socket.NewServer(httpServer, config)

    // Handle socket connections
    io.On("connection", func(clients ...any) {
        client := clients[0].(*socket.Socket)
        client.On("event", func(datas ...any) {
            // Handle your events here
        })
        client.On("disconnect", func(...any) {
            // Handle disconnect
        })
    })

    // Start server
    httpServer.Listen("127.0.0.1:9000", nil)

    // Graceful shutdown handling
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

## Configuration Options

The Redis adapter accepts the following options:

```golang
type RedisAdapterOptions struct {
    Prefix  string // Optional prefix for Redis keys
    // Add other available options here
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
