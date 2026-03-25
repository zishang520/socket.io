# adapters/valkey

Valkey client adapter for Socket.IO clustering.

This module provides `ValkeyClient`, a `cache.CacheClient` implementation backed by
[valkey-go](https://github.com/valkey-io/valkey-go).  It enables the Socket.IO cluster
adapters to use [Valkey](https://valkey.io/) as the pub/sub and stream backend.

[Valkey](https://valkey.io/) is an open-source, Redis-compatible data store — the
`adapters/valkey` module gives you a drop-in alternative to `adapters/redis` for
environments that run Valkey instead of (or alongside) Redis.

---

## Installation

```sh
go get github.com/zishang520/socket.io/adapters/valkey/v3
```

---

## Usage

```go
import (
    "context"

    vk          "github.com/valkey-io/valkey-go"
    cacheadapter "github.com/zishang520/socket.io/adapters/cache/v3/adapter"
    valkeyc      "github.com/zishang520/socket.io/adapters/valkey/v3"
)

func main() {
    ctx := context.Background()

    vc, err := vk.NewClient(vk.ClientOption{
        InitAddress: []string{"127.0.0.1:6379"},
    })
    if err != nil {
        panic(err)
    }

    client := valkeyc.NewValkeyClient(ctx, vc)

    // Classic pub/sub adapter
    io.Adapter(&cacheadapter.CacheAdapterBuilder{Cache: client})

    // Sharded pub/sub adapter (Valkey cluster)
    io.Adapter(&cacheadapter.ShardedCacheAdapterBuilder{Cache: client})

    // Streams adapter (persistent, session recovery)
    io.Adapter(&cacheadapter.CacheStreamsAdapterBuilder{Cache: client})
}
```

---

## Subscription model

`valkey-go` uses a callback-based `Receive` API.  `ValkeySubscription` wraps each
`Receive` call in a goroutine and forwards messages to a buffered Go channel.
Closing or unsubscribing from a `ValkeySubscription` cancels the underlying context,
which terminates the `Receive` call and closes the message channel.

---

## Supported adapter modes

| Adapter | Method | Notes |
|---|---|---|
| Classic pub/sub | `Subscribe` / `PSubscribe` / `Publish` | All Valkey versions |
| Sharded pub/sub | `SSubscribe` / `SPublish` | Requires Valkey cluster |
| Streams | `XAdd` / `XRead` / `XRange` / `Set` / `GetDel` | All Valkey versions |
