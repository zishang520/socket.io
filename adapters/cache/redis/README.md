# adapters/cache/redis

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/adapters/cache/redis/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/adapters/cache/redis/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/adapters/cache/redis/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/adapters/cache/redis/v3)

## Description

Redis client wrapper (`RedisClient`) implementing the `cache.CacheClient` interface from
[`adapters/cache`](../README.md).  This module contains **only** the client wrapper; all adapter
and emitter logic lives in `adapters/cache`.

For Valkey support see [`adapters/cache/valkey`](../valkey).

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/cache/redis/v3
go get github.com/zishang520/socket.io/adapters/cache/v3
```

## Features

- Wraps `go-redis` `UniversalClient` (standalone, sentinel, cluster)
- Implements `cache.CacheClient` and `cache.CacheSubscription`
- Error propagation via the `EventEmitter` interface (`On("error", …)`)
- Transparent nil-error mapping (`rds.Nil` → `cache.ErrNil`)

## How to use

```go
package main

import (
    "context"
    "fmt"

    rds          "github.com/redis/go-redis/v9"
    redisc       "github.com/zishang520/socket.io/adapters/cache/redis/v3"
    cacheadapter "github.com/zishang520/socket.io/adapters/cache/v3/adapter"
    "github.com/zishang520/socket.io/servers/socket/v3"
)

func main() {
    ctx := context.Background()

    rdb := rds.NewClient(&rds.Options{Addr: "127.0.0.1:6379"})
    client := redisc.NewRedisClient(ctx, rdb)

    client.On("error", func(a ...any) { fmt.Println("redis error:", a) })

    io := socket.NewServer(nil, socket.DefaultServerOptions())
    io.Adapter(&cacheadapter.CacheAdapterBuilder{Cache: client})
    // ...
}
```

See [`adapters/cache`](../README.md) for the full list of adapter modes and options.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
