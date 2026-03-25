# adapters/cache

Backend-agnostic Socket.IO cluster adapter and emitter.

`adapters/cache` is the central module for Socket.IO clustering. It defines:

- `CacheClient` — a unified interface for pub/sub and stream backends (Redis, Valkey, …)
- All three adapter modes: **classic pub/sub**, **sharded pub/sub** (Redis 7+ / Valkey), and **Redis Streams**
- The emitter for sending events from non-Socket.IO services

Client implementations live as sub-modules:

| Module | Backend |
|---|---|
| [`adapters/cache/redis`](redis) | Redis (standalone, sentinel, cluster) via `go-redis/v9` |
| [`adapters/cache/valkey`](valkey) | Valkey (standalone, cluster) via `valkey-go` |

---

## Usage

### Classic pub/sub adapter

```go
import (
    cacheadapter "github.com/zishang520/socket.io/adapters/cache/v3/adapter"
    redisc       "github.com/zishang520/socket.io/adapters/cache/redis/v3"
    goredis      "github.com/redis/go-redis/v9"
)

rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
client := redisc.NewRedisClient(ctx, rdb)

io.Adapter(&cacheadapter.CacheAdapterBuilder{Cache: client})
```

### With Valkey

```go
import (
    cacheadapter "github.com/zishang520/socket.io/adapters/cache/v3/adapter"
    valkeyc      "github.com/zishang520/socket.io/adapters/cache/valkey/v3"
    "github.com/valkey-io/valkey-go"
)

vc, _ := valkey.NewClient(valkey.ClientOption{InitAddress: []string{"127.0.0.1:6379"}})
client := valkeyc.NewValkeyClient(ctx, vc)

io.Adapter(&cacheadapter.CacheAdapterBuilder{Cache: client})
```

### Sharded pub/sub adapter (Redis 7+ / Valkey cluster)

```go
io.Adapter(&cacheadapter.ShardedCacheAdapterBuilder{
    Cache: client,
    Opts: &cacheadapter.ShardedCacheAdapterOptions{}, // optional
})
```

### Redis Streams adapter (persistent / session-recovery)

```go
io.Adapter(&cacheadapter.CacheStreamsAdapterBuilder{
    Cache: client,
    Opts: &cacheadapter.CacheStreamsAdapterOptions{}, // optional
})
```

### Emitter (from outside a Socket.IO server)

```go
import "github.com/zishang520/socket.io/adapters/cache/v3/emitter"

e := emitter.NewEmitter(client, nil)
e.To("room1").Emit("hello", "world")
```

---

## Adapter options

### `CacheAdapterOptions`

| Option | Default | Description |
|---|---|---|
| `Key` | `"socket.io"` | Channel prefix |
| `RequestsTimeout` | `5000ms` | Inter-node request timeout |
| `PublishOnSpecificResponseChannel` | `false` | Use per-node response channels |
| `Parser` | MessagePack | Codec for inter-node messages |

### `ShardedCacheAdapterOptions`

| Option | Default | Description |
|---|---|---|
| `ChannelPrefix` | `"socket.io"` | Channel prefix |
| `SubscriptionMode` | `DynamicSubscriptionMode` | Channel allocation strategy |

### `CacheStreamsAdapterOptions`

| Option | Default | Description |
|---|---|---|
| `StreamName` | `"socket.io"` | Redis/Valkey stream key |
| `MaxLen` | `10000` | Approximate maximum stream length |
| `ReadCount` | `100` | Entries fetched per XRead call |
| `SessionKeyPrefix` | `"sio:session:"` | Prefix for session persistence keys |

### Subscription modes

| Mode | Description |
|---|---|
| `StaticSubscriptionMode` | Two fixed channels per namespace |
| `DynamicSubscriptionMode` (default) | +1 channel per public room |
| `DynamicPrivateSubscriptionMode` | +1 channel per room including private rooms |

---

## Implementing a custom backend

Implement `cache.CacheClient` and `cache.CacheSubscription`. The compile-time assertions in `adapters/cache/redis` and `adapters/cache/valkey` show the expected patterns.

```go
var _ cache.CacheClient       = (*MyClient)(nil)
var _ cache.CacheSubscription = (*MySubscription)(nil)
```
