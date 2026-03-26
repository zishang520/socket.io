# @socket.io/valkey-adapter (Go)

A Valkey adapter for [Socket.IO](https://socket.io/) in Go. This module provides
Socket.IO clustering via Valkey Pub/Sub, Valkey Sharded Pub/Sub (Valkey 7+), and
Valkey Streams — mirroring the functionality of the `adapters/redis` module but
using the [`valkey-go`](https://github.com/valkey-io/valkey-go) client.

## Adapter Types

| Type | Description |
|---|---|
| `ValkeyAdapterBuilder` | Classic Pub/Sub — suitable for standalone and replicated Valkey |
| `ShardedValkeyAdapterBuilder` | Sharded Pub/Sub (SSUBSCRIBE/SPUBLISH) — for Valkey Cluster |
| `ValkeyStreamsAdapterBuilder` | Valkey Streams — persistent messages + session recovery |

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/valkey/v3
```

## Usage

### Classic Pub/Sub Adapter

```go
import (
    "context"

    vk "github.com/valkey-io/valkey-go"
    io "github.com/zishang520/socket.io/servers/socket/v3"
    vkadapter "github.com/zishang520/socket.io/adapters/valkey/v3/adapter"
    valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
)

client, err := vk.NewClient(vk.ClientOption{
    InitAddress: []string{"localhost:6379"},
})
if err != nil {
    log.Fatal(err)
}

valkeyClient := valkey.NewValkeyClient(context.Background(), client)

server := io.NewServer(nil, nil)
server.SetAdapter(&vkadapter.ValkeyAdapterBuilder{Valkey: valkeyClient})
```

### Sharded Pub/Sub Adapter (Valkey Cluster)

```go
server.SetAdapter(&vkadapter.ShardedValkeyAdapterBuilder{Valkey: valkeyClient})
```

### Streams Adapter

```go
server.SetAdapter(&vkadapter.ValkeyStreamsAdapterBuilder{Valkey: valkeyClient})
```

### Reusing an Existing Client

Pass a pre-created `vk.Client` to `NewValkeyClient`. This allows you to share
an existing connection pool instead of creating a second one:

```go
// existing client created elsewhere in your application
existingClient := myAppValkeyClient

valkeyClient := valkey.NewValkeyClient(ctx, existingClient)
server.SetAdapter(&vkadapter.ValkeyAdapterBuilder{Valkey: valkeyClient})
```

### Emitter

Use the emitter to broadcast events from a process that does not run a
Socket.IO server:

```go
import (
    "context"

    vk "github.com/valkey-io/valkey-go"
    valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
    "github.com/zishang520/socket.io/adapters/valkey/v3/emitter"
)

client, _ := vk.NewClient(vk.ClientOption{InitAddress: []string{"localhost:6379"}})
valkeyClient := valkey.NewValkeyClient(context.Background(), client)

e := emitter.NewEmitter(valkeyClient, nil)
e.To("room1").Emit("hello", "world")
```

## Configuration

### ValkeyAdapterOptions

| Option | Type | Default | Description |
|---|---|---|---|
| `Key` | `string` | `"socket.io"` | Channel prefix |
| `RequestsTimeout` | `time.Duration` | `5000ms` | Inter-node request timeout |
| `PublishOnSpecificResponseChannel` | `bool` | `false` | Route responses to per-node channels |
| `Parser` | `valkey.Parser` | MsgPack | Encoder/decoder for messages |

### ShardedValkeyAdapterOptions

| Option | Type | Default | Description |
|---|---|---|---|
| `ChannelPrefix` | `string` | `"socket.io"` | Channel prefix |
| `SubscriptionMode` | `SubscriptionMode` | `DynamicSubscriptionMode` | Channel strategy |

### ValkeyStreamsAdapterOptions

| Option | Type | Default | Description |
|---|---|---|---|
| `StreamName` | `string` | `"socket.io"` | Stream name |
| `MaxLen` | `int64` | `10000` | Approximate stream max length |
| `ReadCount` | `int64` | `100` | Messages per XREAD call |
| `SessionKeyPrefix` | `string` | `"sio:session:"` | Session key prefix |

## Subscription Modes

| Mode | Description |
|---|---|
| `StaticSubscriptionMode` | 2 fixed channels per namespace |
| `DynamicSubscriptionMode` | 2 + 1 channel per public room (default) |
| `DynamicPrivateSubscriptionMode` | Separate channel per room (including private) |

## License

MIT
