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

## Supported Redis deployments

The adapter supports standalone Redis, Redis Sentinel, and Redis Cluster through
go-redis `*redis.Client` and `*redis.ClusterClient` instances.

go-redis `*redis.Ring` is not supported. Ring is client-side sharding across
independent Redis servers and does not provide the Pub/Sub routing semantics
required by the adapter.

Create clients with `NewRedisClient` or `NewRedisClientWithSub`. Both
constructors return an error for invalid configurations, and the resulting
client configuration is immutable. Use `Client()`, `Sub()`, and `Context()` to
access it.

A go-redis `*redis.ClusterClient` passed as the primary client must not enable
`ReadOnly`, `RouteByLatency`, or `RouteRandomly`; these settings may route
recovery reads to replicas, so construction returns `redis.ErrReadOnlyRedisClient`.
A read-only cluster client may instead be passed as `subClient` to
`NewRedisClientWithSub`.

For standalone and Sentinel deployments, every Socket.IO process must route
`Sub()` to the same Pub/Sub node or logical endpoint. Redis subscriber counts
are node-local, so splitting subscriptions across independent replicas makes
`ServerCount()` undercount.

## How to use

Basic usage example:

```golang
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"

    rds "github.com/redis/go-redis/v9"
    "github.com/zishang520/socket.io/adapters/redis/v3"
    "github.com/zishang520/socket.io/adapters/redis/v3/adapter"
    "github.com/zishang520/socket.io/servers/socket/v3"
    "github.com/zishang520/socket.io/v3/pkg/types"
)

func main() {
    rdsClient := rds.NewClient(&rds.Options{
        Addr:     "127.0.0.1:6379",
        Username: "",
        Password: "",
        DB:       0,
    })
    defer func() { _ = rdsClient.Close() }()

    redisClient, err := redis.NewRedisClient(context.Background(), rdsClient)
    if err != nil {
        panic(err)
    }

    redisClient.On("error", func(args ...any) {
        fmt.Println("Redis error:", args)
    })

    config := socket.DefaultServerOptions()
    config.SetAdapter(&adapter.RedisAdapterBuilder{
        Redis: redisClient,
        Opts:  adapter.DefaultRedisAdapterOptions(),
    })

    httpServer := types.NewWebServer(nil)
    io := socket.NewServer(httpServer, config)

    io.On("connection", func(clients ...any) {
        if len(clients) == 0 {
            return
        }
        client, ok := clients[0].(*socket.Socket)
        if !ok {
            return
        }
        client.On("event", func(data ...any) {
            // Handle your events here
        })
        client.On("disconnect", func(...any) {
            // Handle disconnect
        })
    })

    httpServer.Listen("127.0.0.1:9000", nil)

    shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()
    <-shutdown.Done()

    // Close Socket.IO, its adapters, and the attached HTTP server.
    io.Close(nil)
}
```

## Configuration Options

Option values are set through the public setter methods:

```golang
options := adapter.DefaultRedisAdapterOptions()
options.SetKey("socket.io")
options.SetRequestsTimeout(10 * time.Second)
options.SetPublishOnSpecificResponseChannel(true)
```

Pass `options` as `RedisAdapterBuilder.Opts`. A custom full encoder/decoder can
also be configured with `options.SetParser(parser)`.

Classic (non-sharded) Redis emitter options only encode outbound messages, so
configure them with `SetEncoder`:

```golang
emitterOptions := emitter.DefaultEmitterOptions()
emitterOptions.SetEncoder(encoder)
```

## Redis Streams deployment constraints

When using the Redis Streams adapter or emitter:

- Mixed Go and Node.js deployments must keep `streamCount` set to `1`. The Node.js standalone Redis Streams emitter always writes to the base stream. Its adapter also creates readers only for non-negative stream indexes even though its signed namespace hash can select a negative index; the Go adapter polls the stream selected by that signed hash. A single stream is therefore the supported interoperable topology.
- Every process that writes to the same stream must use the same `maxLen`. A writer with a lower limit can trim history that other nodes still expect to retain for connection-state recovery, so the common value must also cover the deployment's recovery window and write rate.
- The Go adapter currently accepts `blockTimeInMs` values from `1` through `5000`. For values outside that range, the Redis client emits an `error` event and the poller falls back to `5000ms`. This hard cap ensures that a blocking `XREAD` on the caller's shared Redis client returns at least every five seconds, allowing cancellation to be observed without closing that shared client.
- Each distinct active stream poller holds one connection from the `Sub()` pool during blocking `XREAD`. Size that pool for the active pollers plus Pub/Sub and regular command headroom, or provide a separate `subClient`; namespaces on the same server that use the same `RedisClient`, route to the same stream, and have identical read settings share a poller.

## Cross-language payloads

To preserve binary values when communicating with Node.js, compound payloads must use `[]any` and
`map[string]any`. A `[]byte` value can be sent directly or nested inside either supported container.
Typed containers such as structs, `[]T`, and `map[string]T` are not recursively inspected for nested
binary values and must not be used for cross-language binary payloads.

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
