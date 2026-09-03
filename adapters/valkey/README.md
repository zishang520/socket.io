# @socket.io/valkey-adapter (Go)

A Valkey adapter for [Socket.IO](https://socket.io/) in Go. It provides classic
Pub/Sub, sharded Pub/Sub (Valkey 7+), and Valkey Streams transports through the
[`valkey-go`](https://github.com/valkey-io/valkey-go) client.

## Transports

| Transport | Server adapter | External emitter |
|---|---|---|
| Classic Pub/Sub | `ValkeyAdapterBuilder` | `NewEmitter` (default) |
| Sharded Pub/Sub | `ShardedValkeyAdapterBuilder` | `NewEmitter` with `Sharded` enabled |
| Valkey Streams | `ValkeyStreamsAdapterBuilder` | `NewValkeyStreamsEmitter` |

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/valkey/v3
```

## Usage

### Adapters

```go
import (
    "context"
    "log"

    vk "github.com/valkey-io/valkey-go"
    valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
    vkadapter "github.com/zishang520/socket.io/adapters/valkey/v3/adapter"
    io "github.com/zishang520/socket.io/servers/socket/v3"
)

client, err := vk.NewClient(vk.ClientOption{
    InitAddress: []string{"localhost:6379"},
})
if err != nil {
    log.Fatal(err)
}

valkeyClient, err := valkey.NewValkeyClient(context.Background(), client)
if err != nil {
    log.Fatal(err)
}

server := io.NewServer(nil, nil)

// Choose exactly one transport for this server.
server.SetAdapter(&vkadapter.ValkeyAdapterBuilder{Valkey: valkeyClient})
// server.SetAdapter(&vkadapter.ShardedValkeyAdapterBuilder{Valkey: valkeyClient})
// server.SetAdapter(&vkadapter.ValkeyStreamsAdapterBuilder{Valkey: valkeyClient})
```

### Client roles

`NewValkeyClientWithSub` accepts two pre-created `vk.Client` instances with
different roles:

- `Client()` is the primary-routed client. It performs writes and the
  consistency-sensitive reads used by connection-state recovery.
- `Sub()` performs subscriptions, blocking `XREAD`, and Pub/Sub subscriber
  counts. Without a separate client, `Sub()` returns `Client()`.

```go
primaryClient, _ := vk.NewClient(vk.ClientOption{
    InitAddress: []string{"primary:6379"},
})
subClient, _ := vk.NewClient(vk.ClientOption{
    InitAddress: []string{"subscriber:6379"},
})

valkeyClient, err := valkey.NewValkeyClientWithSub(
    context.Background(), primaryClient, subClient,
)
if err != nil {
    log.Fatal(err)
}
```

The first client must accept writes and provide primary-consistent recovery
reads. `valkey-go` does not expose the read-only or routing options of an
already-created `vk.Client`, so `NewValkeyClientWithSub` cannot validate this
requirement; it is part of the caller's configuration contract. The second
client must route subscriptions and subscriber-count commands to the intended
Pub/Sub topology.

Use one `ValkeyClient` wrapper and one underlying `Sub()` client per Socket.IO
server, and share that wrapper across the server's namespace adapters. A write
client may be shared by passing it to multiple `NewValkeyClientWithSub` calls,
but each server needs a distinct subscription client. This matches the Node.js
adapters, which duplicate the subscription client for each server, and keeps
Pub/Sub subscriber counts equal to the number of Socket.IO servers.

Subscriptions are fixed when `Subscribe` or `SSubscribe` creates a
`ValkeyPubSub`. Call `Close` to stop that subscription. `SSubscribe` accepts
one channel, so channels in different cluster slots use separate subscriptions.
Logical subscriptions with the same kind and topic within one `ValkeyClient`
share one physical Valkey subscription while retaining independent message
queues. The wrapper and its `Sub()` client therefore define one server-side
subscriber identity. Delivery order is preserved within a topic; no ordering
is defined across different topics.

The ACL used by `Sub()` must allow each subscription command together with its
matching cleanup command: `SUBSCRIBE`/`UNSUBSCRIBE`,
`PSUBSCRIBE`/`PUNSUBSCRIBE`, and `SSUBSCRIBE`/`SUNSUBSCRIBE`. Allowing only the
subscribe side lets startup succeed but prevents `Close` from removing the
server-side subscription. `Close` waits for cleanup and returns any unsubscribe
error; the same error is also emitted by the `ValkeyClient`.

### Emitters

Emitters broadcast from a process that does not run a Socket.IO server:

```go
import "github.com/zishang520/socket.io/adapters/valkey/v3/emitter"

classicEmitter := emitter.NewEmitter(valkeyClient, nil)
classicEmitter.To("room1").Emit("hello", "world")

shardedOptions := emitter.DefaultEmitterOptions()
shardedOptions.SetSharded(true)
shardedEmitter := emitter.NewEmitter(valkeyClient, shardedOptions)
shardedEmitter.Emit("hello", "world")

streamsEmitter := emitter.NewValkeyStreamsEmitter(valkeyClient, nil)
streamsEmitter.Emit("hello", "world")
```

The classic emitter accepts an outbound-only `Encoder`. The sharded emitter
always uses the shared cluster-message codec, so `Encoder` does not alter its
wire format.

## Configuration

### Classic adapter

| Option | Type | Default | Description |
|---|---|---|---|
| `Key` | `string` | `"socket.io"` | Channel prefix |
| `RequestsTimeout` | `time.Duration` | `5000ms` | Inter-node request timeout |
| `PublishOnSpecificResponseChannel` | `bool` | `false` | Route responses to per-node channels |
| `Parser` | `valkey.Parser` | MsgPack | Full encoder/decoder for adapter messages |

### Sharded adapter

| Option | Type | Default | Description |
|---|---|---|---|
| `ChannelPrefix` | `string` | `"socket.io"` | Channel prefix |
| `SubscriptionMode` | `valkey.SubscriptionMode` | `DynamicSubscriptionMode` | Channel strategy |

### Streams adapter

| Option | Type | Default | Description |
|---|---|---|---|
| `StreamName` | `string` | `"socket.io"` | Base stream name |
| `StreamCount` | `int` | `1` | Number of namespace-sharded streams |
| `ChannelPrefix` | `string` | `"socket.io"` | Pub/Sub channel prefix |
| `UseShardedPubSub` | `bool` | `false` | Use sharded Pub/Sub for notifications |
| `MaxLen` | `int64` | `10000` | Approximate stream maximum length |
| `ReadCount` | `int64` | `100` | Messages requested per `XREAD` |
| `BlockTimeInMs` | `int64` | `5000` | Blocking `XREAD` duration; `0` blocks indefinitely |
| `SessionKeyPrefix` | `string` | `"sio:session:"` | Recovery-session key prefix |
| `OnlyPlaintext` | `bool` | `false` | Skip binary detection for known plaintext payloads |

### Classic and sharded emitters

| Option | Type | Default | Description |
|---|---|---|---|
| `Key` | `string` | `"socket.io"` | Classic Pub/Sub channel prefix |
| `Encoder` | `valkey.Encoder` | MsgPack | Classic outbound encoder; ignored in sharded mode |
| `Sharded` | `bool` | `false` | Use sharded Pub/Sub |
| `SubscriptionMode` | `valkey.SubscriptionMode` | `DynamicSubscriptionMode` | Sharded channel strategy |

### Streams emitter

| Option | Type | Default | Description |
|---|---|---|---|
| `StreamName` | `string` | `"socket.io"` | Base stream name |
| `StreamCount` | `int` | `1` | Number of namespace-sharded streams |
| `MaxLen` | `int64` | `10000` | Approximate stream maximum length |

Emitter routing settings are part of the deployment contract. A classic
emitter's `Key` must match the classic adapter's `Key`. In sharded mode, the
emitter's `Key` and `SubscriptionMode` must match the adapter's `ChannelPrefix`
and `SubscriptionMode`. A Streams emitter and adapter must use identical
`StreamName` and `StreamCount` routing values. They should also use the same
`MaxLen`, which controls stream retention rather than routing. Mismatched
routing values can publish messages to a channel or stream that no adapter
consumes.

## Valkey Streams deployment constraints

When using the Valkey Streams adapter or emitter:

- Mixed Go and Node.js deployments must keep `StreamCount` set to `1`. The
  Node.js standalone Redis Streams emitter always writes to the base stream, so
  multiple streams are not an interoperable topology. The Node.js server
  adapter also derives signed stream suffixes but only starts pollers for
  non-negative suffixes, so namespaces with a negative hash would be missed.
- Every process that writes to the same stream must use the same `MaxLen`. A
  lower value can trim history that other nodes still need for connection-state
  recovery; size the common value for the recovery window and write rate.
- Each distinct active poller holds one connection from the `Sub()` client's
  blocking pool during `XREAD`. Classic Pub/Sub holds one additional dedicated
  connection for all namespaces that share a `ValkeyClient` on the same
  Socket.IO server, which preserves request and broadcast ordering. Size the
  pool for these shared connections and any other blocking operations, or
  provide a separate `subClient`. Sharded and Streams Pub/Sub notifications
  otherwise use valkey-go's multiplexed receive path. Namespaces that route to
  the same stream and have identical read settings share a poller.

## Subscription modes

| Mode | Description |
|---|---|
| `StaticSubscriptionMode` | Two fixed channels per namespace |
| `DynamicSubscriptionMode` | Two channels plus one per public room (default) |
| `DynamicPrivateSubscriptionMode` | A separate channel for every room |

## Cross-language compatibility

Classic and sharded messages follow the Node.js Redis adapter wire protocol.
Streams entries follow the Node.js Redis Streams adapter and emitter wire
protocols.
Room classification and namespace-to-stream routing use JavaScript UTF-16 code
unit semantics so Go and Node.js select the same channel or stream.

To preserve binary values across Go and Node.js, compound payloads must use
`[]any` or `map[string]any`. A `[]byte` value may be sent directly or nested in
either supported container. Structs, `[]T`, and `map[string]T` are not
recursively inspected for nested binary values and must not be used for
cross-language binary payloads.

## License

MIT
