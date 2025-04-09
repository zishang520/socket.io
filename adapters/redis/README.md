
# socket.io-go-redis

[![Go](https://github.com/zishang520/socket.io/adapters/redis/v3/actions/workflows/go.yml/badge.svg)](https://github.com/zishang520/socket.io/adapters/redis/v3/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/zishang520/socket.io/adapters/redis/v3?utm_source=godoc)](https://pkg.go.dev/github.com/zishang520/socket.io/adapters/redis/v3)

## How to use

```golang
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/redis/go-redis/v9"
    s "github.com/zishang520/socket.io/servers/engine/v3/types"
    "github.com/zishang520/socket.io/adapters/redis/v3/adapter"
    "github.com/zishang520/socket.io/adapters/redis/v3/types"
    "github.com/zishang520/socket.io/servers/socket/v3"
    // "github.com/zishang520/socket.io/adapters/redis/v3/emitter"
)

func main() {

    redisClient := types.NewRedisClient(context.TODO(), redis.NewClient(&redis.Options{
        Addr:     "127.0.0.1:6379",
        Username: "",
        Password: "",
        DB:       0,
    }))

    redisClient.On("error", func(a ...any) {
        fmt.Println(a)
    })

    config := socket.DefaultServerOptions()
    config.SetAdapter(&adapter.RedisAdapterBuilder{
        Redis: redisClient,
        Opts:  &adapter.RedisAdapterOptions{},
    })
    httpServer := s.CreateServer(nil)
    io := socket.NewServer(httpServer, config)
    io.On("connection", func(clients ...any) {
        client := clients[0].(*socket.Socket)
        client.On("event", func(datas ...any) {
        })
        client.On("disconnect", func(...any) {
        })
    })
    httpServer.Listen("127.0.0.1:9000", nil)

    // emitter.NewEmitter(redisClient, nil, "/web") // more ....

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

## Tests

Standalone tests can be run with `make test` which will run the golang tests.

You can run the tests locally using the following command:

```
make test
```

## Support

[issues](https://github.com/zishang520/socket.io/adapters/redis/v3/issues)

## Development

To contribute patches, run tests or benchmarks, make sure to clone the
repository:

```bash
git clone git://github.com/zishang520/socket.io/adapters/redis/v3.git
```

Then:

```bash
cd socket.io-go-redis
make test
```

See the `Tests` section above for how to run tests before submitting any patches.

## License

MIT
