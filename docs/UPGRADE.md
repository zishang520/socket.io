# Upgrade Guide

## Table of Contents

- [What's New in v3](#whats-new-in-v3)
- [Upgrading from v1/v2 to v3](#upgrading-from-v1v2-to-v3)
  - [Estimated Upgrade Time](#estimated-upgrade-time-30---60-minutes)
  - [High Impact Changes](#high-impact-changes)
  - [Medium Impact Changes](#medium-impact-changes)
  - [Low Impact Changes](#low-impact-changes)
  - [Updating Dependencies](#updating-dependencies)
  - [Import Path Updates](#import-path-updates)
  - [Breaking Changes](#breaking-changes)
  - [Quick Start Example](#quick-start-example)
  - [Testing Your Upgrade](#testing-your-upgrade)
  - [Common Issues](#common-issues)
  - [Need Help?](#need-help)
- [Release Notes](#release-notes)
  - [v3.0.0](#v300)
  - [v3.0.0-rc.14](#v300-rc14)
  - [v3.0.0-rc.13](#v300-rc13)
  - [v3.0.0-rc.12](#v300-rc12)
  - [v3.0.0-rc.8](#v300-rc8)
  - [v3.0.0-rc.4](#v300-rc4)
  - [v3.0.0-rc.2](#v300-rc2)
  - [v3.0.0-beta.1](#v300-beta1)
  - [v3.0.0-alpha.0 ~ alpha.4](#v300-alpha0--alpha4)

---

## What's New in v3

Socket.IO for Go **v3.0.0** is a major release that brings the following key improvements:

| Feature | Description |
|---------|-------------|
| **Monorepo Consolidation** | All previously separate repositories (`engine.io-go-parser`, `engine.io`, `socket.io-go-parser`, `socket.io-client-go`, `socket.io-go-redis`) have been merged into a single monorepo with 9 versioned submodules |
| **Unified Version Management** | All modules share a single version definition in `pkg/version`, ensuring consistency across the entire ecosystem |
| **Protocol Alignment** | Aligned with the Socket.IO v4+ protocol for improved compatibility with the JavaScript ecosystem |
| **Thread Safety** | Comprehensive concurrency fixes including atomic socket flags, mutex-protected middleware, copy-on-write patterns, and goroutine leak prevention |
| **Type Safety** | Generic `types.Atomic[T]` replacing `atomic.Value`, `types.Optional[T]` for null safety, strongly typed `Handshake` fields |
| **New Utility Packages** | `pkg/slices` for safe slice operations, `pkg/queue` for ordered message delivery, `pkg/request` for HTTP client |
| **Redis Cluster Support** | Sharded broadcast operator, CROSSSLOT error fixes, and dynamic channel subscription management |
| **DoS Prevention** | HTTP body size limits on polling transport, configurable attachment count limits |
| **Go 1.26.0** | Minimum Go version requirement updated to Go 1.26.0 |

### Module Architecture

```
github.com/zishang520/socket.io/
├── v3                          # Root: shared types, interfaces
├── parsers/
│   ├── engine/v3               # Engine.IO protocol parser
│   └── socket/v3               # Socket.IO protocol parser
├── servers/
│   ├── engine/v3               # Engine.IO server
│   └── socket/v3               # Socket.IO server
├── clients/
│   ├── engine/v3               # Engine.IO client
│   └── socket/v3               # Socket.IO client
└── adapters/
    ├── adapter/v3              # Base adapter interface
    └── redis/v3                # Redis adapter (+ emitter)
```

---

## Upgrading from v1/v2 to v3

### Estimated Upgrade Time: 30 - 60 Minutes

We recommend reviewing this entire upgrade guide to understand all changes. The upgrade process consolidates dependencies and updates import paths to align with the Socket.IO v4+ protocol, introducing improved performance and updated APIs.

### High Impact Changes

<details>
<summary>Dependency Structure Consolidation</summary>

All Socket.IO related packages are now consolidated under the main `github.com/zishang520/socket.io/` repository with versioned submodules.

**Likelihood Of Impact: Very High**

This change affects every import in your application. All imports must be updated to use the new v3 paths.
</details>

<details>
<summary>Protocol Compatibility Update</summary>

Socket.IO v3 aligns with the Socket.IO v4+ protocol, which means compatibility changes for all client connections.

**Likelihood Of Impact: Very High**

Your client-side Socket.IO library must be upgraded to version 4.x or higher. Clients using older versions (v2.x or v3.x) will not be able to connect to the v3 server.

```bash
# Update your frontend dependency
npm install socket.io-client@^4.0.0
```

**Action Required:**

- Coordinate with your frontend team to upgrade client libraries
- Test all client connections after upgrade
- Ensure backward compatibility strategy if gradual rollout is needed

Newly generated default Engine.IO session IDs, Socket.IO socket IDs, and
connection-state recovery private IDs now use the same 20-character format as
the Node.js server. Accordingly, `utils.Base64Id().GenerateId()` returns 20
instead of 24 characters. Existing restored IDs are not rewritten; continue to
treat IDs as opaque strings because legacy or custom IDs may use another length.

</details>

<details>
<summary>Import Path Restructuring</summary>

Every Socket.IO import path requires updating to the new v3 structure. This affects 8 major package categories.

**Likelihood Of Impact: Very High**

All package imports across your entire codebase must be systematically updated. This includes:

- Engine.IO Parser (`parsers/engine/v3`)
- Socket.IO Parser (`parsers/socket/v3`)
- Engine.IO Server (`servers/engine/v3`)
- Socket.IO Server (`servers/socket/v3`)
- Redis Adapter (`adapters/redis/v3`)
- Valkey Adapter (`adapters/valkey/v3`) — new in v3
- Engine.IO Client (`clients/engine/v3`)
- Socket.IO Client (`clients/socket/v3`)
- Common Types and Utils (`v3/pkg`)

See the [Import Path Updates](#import-path-updates) section for complete mapping tables.
</details>

<details>
<summary>Redis Adapter Type Changes</summary>

The Redis adapter has replaced `types.String` with `types.Atomic[string]` for better type safety.

**Likelihood Of Impact: High (if using Redis adapter)**

```go
// Before
import "github.com/zishang520/socket.io-go-redis/types"
var s types.String

// After
import "github.com/zishang520/socket.io/v3/pkg/types"
var s types.Atomic[string]
```

</details>

<details>
<summary>Adapter Client Constructors and Accessors</summary>

Redis, Valkey, MongoDB, PostgreSQL, and Unix adapter clients now validate their
required dependencies during construction. Their constructors return an error,
and connection details are immutable after construction and exposed through
read-only accessor methods.

**Likelihood Of Impact: High (if constructing adapter clients directly)**

```go
// Before
redisClient := redis.NewRedisClient(ctx, client)
mongoClient := mongo.NewMongoClient(ctx, collection)
postgresClient := postgres.NewPostgresClient(ctx, pool)
valkeyClient := valkey.NewValkeyClient(ctx, client)
unixClient := unix.NewUnixClient(ctx, socketPath)

// After
redisClient, err := redis.NewRedisClient(ctx, client)
mongoClient, err := mongo.NewMongoClient(ctx, collection)
postgresClient, err := postgres.NewPostgresClient(ctx, pool)
valkeyClient, err := valkey.NewValkeyClient(ctx, client)
unixClient, err := unix.NewUnixClient(ctx, socketPath, nil)
```

Direct field access must be replaced with accessors:

- Redis and Valkey: `Client()`, `Sub()`, `Context()`
- MongoDB: `Collection()`, `Context()`
- PostgreSQL: `Pool()`, `Context()`
- Unix: `SocketPath()`, `Context()`

`PostgresClient.Close` releases its listener connection; it does not cancel the
caller-provided context or close the caller-owned pool.

Finite PostgreSQL operations, including publishing, attachment access,
listener connection attempts, LISTEN/UNLISTEN updates, and attachment cleanup,
now use a fixed 5-second I/O deadline instead of waiting indefinitely. The
steady-state notification wait remains open until a message, connection error,
or lifecycle cancellation occurs.

PostgreSQL notifications now use one receive queue per namespace. Attachment
queries no longer block the shared listener or delivery to other namespaces.
Messages within a namespace remain ordered, including attachment and direct
notifications; slow attachment queries can still delay that namespace's heartbeats
and responses. Node's concurrent attachment reads do not provide this ordering.

Each `PostgresAdapterBuilder` must use its own `PostgresClient`, because the
client owns that builder's listener and subscriptions. The underlying
`*pgxpool.Pool` can still be shared by multiple clients and emitters. Close the
Socket.IO server before closing its `PostgresClient`, then close the pool. The
builder exclusively owns that client's listener, so application code must not
call `Listen` or `Unlisten` on it directly.

Opening a PostgreSQL listener now returns
`postgres.ErrPostgresOnNotificationUnsupported` when the pool has a static
`ConnConfig.OnNotification` callback. For listener clients, leave it nil and do not
install it from `BeforeConnect`, because the adapter depends on pgx's default
notification buffer. `NewPostgresClient` still accepts these pools for emitter-only
use, which does not depend on notification buffering.

`PostgresAdapter.SetChannel` was removed; configure `ChannelPrefix` and let the
adapter derive the namespace channel. The PostgreSQL root package no longer
re-exports `EMITTER_UID` or the shared cluster message constants; use the
equivalent constants from `adapters/adapter` directly.

A go-redis `*redis.ClusterClient` passed as the primary Redis client must not
enable `ReadOnly`, `RouteByLatency`, or `RouteRandomly`; construction now
returns `redis.ErrReadOnlyRedisClient`. Pass a primary-routed client first and,
if needed, a read-only client as `subClient` to `NewRedisClientWithSub`.

The Redis module now uses go-redis v9.22.0. Applications that rely on go-redis
defaults inherit its new read/write timeouts, retry backoff, cluster reload
interval, and TCP keep-alive settings; explicitly configured values are unchanged.
This release also adds methods to the go-redis `UniversalClient` and `Cmdable`
interfaces. Custom clients that implement either interface directly must add
those methods; official clients and wrappers that embed the interface are
unaffected.

</details>

<details>
<summary>Unix Adapter Transport and API Simplification</summary>

The Unix adapter now uses the same length-prefixed `SOCK_STREAM` transport on
every platform. Linux no longer prefers `unixpacket`, which restores transport
compatibility with the `SOCK_STREAM` implementation shipped in v3.0.4 and
removes its platform-specific packet-size limit. This does not make a
mixed-version rolling upgrade safe: processes using this version cannot safely
run alongside Linux processes that still use `unixpacket` in the same socket
directory. Stop all old processes and perform a coordinated restart.

**Likelihood Of Impact: High (if using the Unix adapter directly)**

`UnixClient.ReadMessage` now returns its complete owned payload instead of
copying into a caller-provided buffer:

```go
// Before
buf := make([]byte, 64*1024)
n, addr, err := client.ReadMessage(buf)
payload := buf[:n]

// After
payload, err := client.ReadMessage()
```

The unused peer address return value and `UnixClient.ListenerPath` were
removed. Listener paths are now an internal transport detail managed by
`UnixAdapterBuilder`; cluster node identity continues to come from the message
UID.

The client is now the single source of transport configuration and owns the
listener shared by every namespace. Close it when the server or standalone
emitter shuts down; closing one namespace adapter no longer removes the shared
listener. `NewUnixAdapter` remains the low-level initialized constructor, but it
does not register the adapter with the shared listener. `MakeUnixAdapter` is
deprecated. Install adapters through `UnixAdapterBuilder` as shown below so
incoming messages are routed to the matching namespace.

`Listen` remains idempotent for the active listener path, but now returns
`ErrUnixClientAlreadyListening` when called with a different path. `Send`
rejects empty payloads and payloads larger than 10 MiB with
`ErrUnixMessageSize` before dialing. Peer dials and framed writes are bounded to
5 seconds by default. `NewUnixClient` now accepts `*UnixClientOptions` as its
third argument; pass `nil` for the bounded defaults or provide custom limits.
Non-positive option values retain the bounded defaults. `Send` retries once
only when a non-timeout write fails on an existing pooled connection. Dial
failures, first-use write failures, and write timeouts are returned without
retrying.

The base socket path must not end with a path separator. Listener paths used
with `Broadcast` must append one non-empty suffix that does not contain `.`, as
in `{base-socket-path}.{listener-id}`; `UnixAdapterBuilder` handles this naming
automatically. Rejecting a trailing separator prevents that path from silently
discovering no listeners, while the suffix rule keeps overlapping base names in
the same directory from exchanging cluster traffic.

```go
client, err := unix.NewUnixClient(ctx, socketPath, nil)
if err != nil {
	return err
}
defer client.Close()

server.SetAdapter(&unixadapter.UnixAdapterBuilder{Unix: client})
emitter := unixemitter.NewEmitter(client)
```

`unixemitter.NewEmitter` and `(*Emitter).Construct` no longer accept an
`*EmitterOptions` argument. `EmitterOptions`, `EmitterOptionsInterface`, and
`DefaultEmitterOptions` were removed.

The unused Unix emitter `Key`, `Parser`, and `SocketPath` options were removed,
along with the unused adapter `Key` and `ErrorHandler` options. Remove calls to
`SetKey`, `SetParser`, and `SetSocketPath`; pass the base path and transport
options only to `NewUnixClient`. The obsolete Unix-specific message constants,
`UnixMessage`, `Parser`, `ErrNilUnixPacket`, `SetChannel`, and `OnRawMessage`
were also removed in favor of the shared `adapters/adapter` cluster protocol.
This also removes
the old exported Unix defaults (`unixadapter.DefaultChannelPrefix`,
`unixadapter.DefaultSocketPath`, `unixadapter.DefaultHeartbeatInterval`,
`unixadapter.DefaultHeartbeatTimeout`, `unixemitter.DefaultEmitterKey`, and
`unixemitter.DefaultSocketPath`) and `BroadcastOptions.SocketPath`.

Register an `"error"` listener on `UnixClient` in place of the removed adapter
`ErrorHandler`. Broadcast and emitter delivery is broker-like: invalid input or
a failed directory scan is returned to the caller, while failures for individual
listener paths are reported through this event and do not stop delivery to the
remaining listeners.

Unix cluster messages now use the shared typed JSON/MessagePack codec, which
normalizes required arrays and preserves binary payloads and readers for every
cluster response type. The ACK envelope itself is unchanged. A coordinated
restart is still required when upgrading older Unix adapter versions because
the transport framing, including Linux's move from `unixpacket` to
`SOCK_STREAM`, is not compatible with mixed-version processes.

</details>

### Medium Impact Changes

<details>
<summary>Broadcast Timeout Uses Fractional Milliseconds</summary>

`socket.BroadcastFlags.Timeout` is now `*float64` instead of `*int64`.
JSON, MessagePack, and BSON preserve fractional milliseconds received from
Node.js adapters. Update direct field assignments to use a floating-point value:

```go
flags := &socket.BroadcastFlags{Timeout: new(16.5)}
```

The chainable `Timeout(time.Duration)` methods keep their signatures and now
preserve fractional milliseconds. Timer delays are truncated and bounded when
the timer is created. A nil timeout remains distinct from an explicit zero.
`utils.NormalizeTimerMilliseconds` likewise now accepts `float64`; convert
existing integer variables with `float64(delay)` when calling it directly.

</details>

<details>
<summary>Socket Handshake Type Updates</summary>

The `socket.Handshake` structure now uses more strongly typed fields:

**Likelihood Of Impact: Medium**

```go
// Before
type Handshake struct {
    Headers map[string][]string
    Query   map[string][]string
    Auth    any
}

// After
type Handshake struct {
    Headers types.IncomingHttpHeaders  // provides Header() method
    Query   types.ParsedUrlQuery       // provides Query() method
    Auth    map[string]any
}
```

Access patterns must be updated:

```go
// Before
headers := socket.Handshake().Headers
userAgent := headers["user-agent"][0]

// After
headers := socket.Handshake().Headers.Header()
userAgent := headers.Get("User-Agent")
```

</details>

<details>
<summary>HttpContext API Refactoring</summary>

Several methods and properties of `*types.HttpContext` have been renamed or refactored from properties to methods.

**Likelihood Of Impact: Medium**

```go
// Before
func example(ctx *types.HttpContext) {
    headers := ctx.ResponseHeaders
    host := ctx.GetHost()
    method := ctx.GetMethod()
    values := ctx.Gets("foo")
    value := ctx.Get("bar")
    path := ctx.GetPathInfo()
}

// After
func example(ctx *types.HttpContext) {
    headers := ctx.ResponseHeaders()
    host := ctx.Host()
    method := ctx.Method()
    values, _ := ctx.Query().Gets("foo")
    value, _ := ctx.Query().Get("bar")
    path := ctx.PathInfo()
}
```

**Changes summary:**

- `ResponseHeaders` → `ResponseHeaders()` (property to method)
- `GetHost()` → `Host()`
- `GetMethod()` → `Method()`
- `Gets(key)` → `Query().Gets(key)`
- `Get(key)` → `Query().Get(key)`
- `GetPathInfo()` → `PathInfo()`

**New/Updated methods:**

| Method | Description |
|--------|-------------|
| `Path()` | Returns cleaned path (without leading/trailing slashes) |
| `UserAgent()` | Returns User-Agent header value |
| `Secure()` | Returns `true` if TLS connection |
| `SetStatusCode(code)` | Now returns `error` for validation |
| `IsDone()` | Check if response has been written |
| `Done()` | Returns `<-chan struct{}` instead of `<-chan Void` |

</details>

<details>
<summary>Config GetRaw* Method Changes</summary>

All `GetRaw*` methods now return `types.Optional[T]` instead of pointer types for better null safety:

**Likelihood Of Impact: Medium**

```go
// Before
func configExample(config ConnectionStateRecoveryInterface) {
    if duration := config.GetRawMaxDisconnectionDuration(); duration != nil {
        fmt.Printf("Duration: %d", *duration)
    }
}

// After
func configExample(config ConnectionStateRecoveryInterface) {
    if duration := config.GetRawMaxDisconnectionDuration(); duration != nil {
        fmt.Printf("Duration: %d", duration.Get())
    }
}
```

</details>

<details>
<summary>ParameterBag Package Migration</summary>

`ParameterBag` has been moved from the `utils` package to the `types` package.

**Likelihood Of Impact: Medium**

```go
// Before
import "github.com/zishang520/socket.io/v3/pkg/utils"

func example() {
    var bag *utils.ParameterBag
    bag = utils.NewParameterBag(nil)
}

// After
import "github.com/zishang520/socket.io/v3/pkg/types"

func example() {
    var bag *types.ParameterBag
    bag = types.NewParameterBag(nil)
}
```

</details>

<details>
<summary>Adapter Utility Functions Reorganization</summary>

Utility functions like `SliceMap` and `Tap` have been moved from the `adapter` package to dedicated `pkg` subpackages.

**Likelihood Of Impact: Medium**

```go
// Before
import "github.com/zishang520/socket.io/adapters/adapter/v3"

func example() {
    adapter.SliceMap(/**/)
    adapter.Tap(/**/)
}

// After
import (
    "github.com/zishang520/socket.io/v3/pkg/slices"
    "github.com/zishang520/socket.io/v3/pkg/utils"
)

func example() {
    slices.Map(/**/)
    utils.Tap(/**/)
}
```

**Changes summary:**

- `adapter.SliceMap` → `slices.Map` (moved to `pkg/slices`)
- `adapter.Tap` → `utils.Tap` (moved to `pkg/utils`)

**New functions in `pkg/slices`:**

The new `pkg/slices` package provides additional utility functions:

| Function | Description |
|----------|-------------|
| `Get(s, idx)` | Safely retrieves an element with bounds checking |
| `GetAny[O](vals, idx)` | Retrieves and type-asserts from `[]any` |
| `TryGet(s, idx)` | Returns zero value if out of bounds |
| `TryGetAny[O](vals, idx)` | Type-asserts from `[]any` or returns zero |
| `GetWithDefault(s, idx, def)` | Returns default value if out of bounds |
| `GetPtr(s, idx)` | Returns pointer to element or nil |
| `Slice(s, start)` | Safe sub-slice with bounds checking |
| `First(s)` / `Last(s)` | Get first/last element safely |
| `Filter(s, predicate)` | Filter elements by predicate |
| `Map(vals, transform)` | Transform each element |
| `Reduce(vals, initial, reducer)` | Reduce to single value |
| `IsEmpty(s)` | Check if slice is nil or empty |
| `IsValidIndex(s, idx)` | Check if index is valid |

</details>

<details>
<summary>ExtendedError Type Consolidation</summary>

The `ExtendedError` type has been consolidated from separate implementations in `clients/socket` and `servers/socket` packages into a single shared implementation in `pkg/types`. This eliminates code duplication and provides a consistent error type across the entire codebase.

**Likelihood Of Impact: Medium**

```go
// Before (client-side)
import "github.com/zishang520/socket.io-client-go/socket"

err := socket.NewExtendedError("connection failed", nil)

// Before (server-side)
import "github.com/zishang520/socket.io/v2/socket"

err := socket.NewExtendedError("middleware error", map[string]any{"code": 401})
data := err.Data()  // Note: server-side had Data() method

// After (unified)
import "github.com/zishang520/socket.io/v3/pkg/types"

err := types.NewExtendedError("error message", map[string]any{"code": 401})
data := err.Data  // Now uses direct field access
```

**Key changes:**

- `clients/socket.ExtendedError` → `types.ExtendedError`
- `servers/socket.ExtendedError` → `types.ExtendedError` (type alias maintained for backward compatibility)
- Server-side `Data()` method replaced with `Data` field for consistency
- Both client and server now share the same `ExtendedError` implementation

**Note:** The server-side `socket` package retains a type alias for `ExtendedError` and a wrapper function `NewExtendedError` for backward compatibility, so existing server code may continue to work without changes. However, client-side code must update imports.
</details>

<details>
<summary>Redis SubscriptionMode Type Migration</summary>

The `SubscriptionMode` type has been moved from `adapters/redis/adapter` package to the root `adapters/redis` package for better organization and sharing between adapter and emitter.

**Likelihood Of Impact: Medium (if using Redis sharded adapter)**

```go
// Before
import "github.com/zishang520/socket.io/adapters/redis/v3/adapter"

opts := adapter.NewShardedRedisAdapterOptions()
opts.SetSubscriptionMode(adapter.DynamicSubscriptionMode)

// After
import (
    "github.com/zishang520/socket.io/adapters/redis/v3"
    "github.com/zishang520/socket.io/adapters/redis/v3/adapter"
)

opts := adapter.DefaultShardedRedisAdapterOptions()
opts.SetSubscriptionMode(redis.DynamicSubscriptionMode)
```

**Key changes:**

| Before | After |
|--------|-------|
| `adapter.SubscriptionMode` | `redis.SubscriptionMode` |
| `adapter.StaticSubscriptionMode` | `redis.StaticSubscriptionMode` |
| `adapter.DynamicSubscriptionMode` | `redis.DynamicSubscriptionMode` |
| `adapter.DynamicPrivateSubscriptionMode` | `redis.DynamicPrivateSubscriptionMode` |

**New additions:**

- `redis.DefaultSubscriptionMode` - Default mode constant
- `redis.PrivateRoomIdLength` - Length constant for private room detection
- `redis.ShouldUseDynamicChannel(mode, room)` - Shared helper function

**Emitter options extended:**

The `EmitterOptions` now supports sharded Pub/Sub configuration:

```go
emitterOpts := emitter.DefaultEmitterOptions()
emitterOpts.SetSharded(true)
emitterOpts.SetSubscriptionMode(redis.DynamicSubscriptionMode)
```
</details>

<details>
<summary>Redis Emitter Parser Renamed to Encoder</summary>

The classic (non-sharded) Redis emitter's custom codec now requires only the
outbound `redis.Encoder` interface. Sharded mode continues to use its fixed
cluster-message codec.

**Likelihood Of Impact: Medium (if using a custom Redis emitter parser)**

Rename `SetParser`, `GetRawParser`, and `Parser` calls to `SetEncoder`,
`GetRawEncoder`, and `Encoder`. The `BroadcastOptions.Parser` field is now
`BroadcastOptions.Encoder`. Existing implementations with an `Encode` method
already satisfy the narrower interface.

</details>

<details>
<summary>Redis Cluster Codec Helpers Moved</summary>

The Redis package no longer re-exports the shared cluster-message codec. Import
`github.com/zishang520/socket.io/adapters/adapter/v3` and replace:

| Before | After |
|--------|-------|
| `redis.EncodeClusterMessage` | `adapter.EncodeClusterMessage` |
| `redis.EncodeClusterMessageMsgpack` | `adapter.EncodeClusterMessageMsgpack` |
| `redis.UnmarshalClusterMessage` | `adapter.DecodeClusterMessage` |

</details>

<details>
<summary>Classic Redis RequestType Separation</summary>

Classic Redis request/response codes now use `redis.RequestType` instead of the
cluster adapter's `adapter.MessageType`. The numeric wire values are unchanged;
the separate Go type prevents the two protocols' overlapping values from being
mixed accidentally.

Code that explicitly stored `redis.SOCKETS` through `redis.BROADCAST_ACK` in an
`adapter.MessageType` variable must change that variable to `redis.RequestType`
or convert deliberately at an API boundary.
</details>

<details>
<summary>Classic Redis Request Scope Wire Format</summary>

Classic Redis requests for `REMOTE_JOIN`, `REMOTE_LEAVE`,
`REMOTE_DISCONNECT`, and `REMOTE_FETCH` now require `opts.rooms` and
`opts.except`. The legacy single-socket `sid` and `room` fields are no longer
supported; use empty arrays for an unrestricted selection.

This wire format is not backward-compatible. Upgrade all Go Redis adapter and
emitter nodes together; do not mix this version with older Redis adapter nodes
during a rolling upgrade.
</details>

<details>
<summary>Classic Redis Broadcast Acknowledgement Wire Format</summary>

The classic Redis adapter now matches the Node.js wire protocol:
`BROADCAST_ACK.packet` contains the first acknowledgement argument instead of
the complete `[]any` argument list emitted by v3.0.4. Current responses also
always include `clientCount`, including when its value is zero, while v3.0.4
omitted the zero-valued field.

Because a valid acknowledgement argument may itself be an array, the two wire
formats cannot be distinguished safely. Clusters using cross-node
`BroadcastWithAck` must upgrade all Go nodes together and must not mix v3.0.4
with newer versions during a rolling upgrade. Keep the scalar format when
interoperating with the Node.js Redis adapter.
</details>

### Low Impact Changes

<details>
<summary>Debug Logging Improvements</summary>

Debug logging has been updated to provide more consistent output across all packages.

**Likelihood Of Impact: Low**

No code changes required, but log output format may differ slightly.
</details>

<details>
<summary>Internal Type Reorganization</summary>

Some internal types have been reorganized for better code maintainability. These changes should not affect public API usage but may impact code that relies on internal types.

**Likelihood Of Impact: Low**

If you're importing internal packages, review your imports after upgrading.
</details>

## Updating Dependencies

Update your `go.mod` to require the Socket.IO v3 packages:

```bash
go get github.com/zishang520/socket.io/v3@latest
go get github.com/zishang520/socket.io/parsers/engine/v3@latest
go get github.com/zishang520/socket.io/parsers/socket/v3@latest
go get github.com/zishang520/socket.io/servers/engine/v3@latest
go get github.com/zishang520/socket.io/servers/socket/v3@latest
go get github.com/zishang520/socket.io/adapters/adapter/v3@latest
go get github.com/zishang520/socket.io/adapters/redis/v3@latest
go get github.com/zishang520/socket.io/clients/engine/v3@latest
go get github.com/zishang520/socket.io/clients/socket/v3@latest
```

Clean up your dependencies after updating:

```bash
go mod tidy
```

Example `go.mod` entries:

```go
require (
    github.com/zishang520/socket.io/v3 v3.0.0
    github.com/zishang520/socket.io/parsers/engine/v3 v3.0.0
    github.com/zishang520/socket.io/parsers/socket/v3 v3.0.0
    github.com/zishang520/socket.io/servers/engine/v3 v3.0.0
    github.com/zishang520/socket.io/servers/socket/v3 v3.0.0
    github.com/zishang520/socket.io/adapters/adapter/v3 v3.0.0
    github.com/zishang520/socket.io/adapters/redis/v3 v3.0.0
    github.com/zishang520/socket.io/clients/engine/v3 v3.0.0
    github.com/zishang520/socket.io/clients/socket/v3 v3.0.0
)
```

---

## Import Path Updates

Update all Socket.IO import paths throughout your application using the following reference tables:

### Engine.IO Parser

| v1/v2 Import | v3 Import |
|--------------|-----------|
| `github.com/zishang520/engine.io-go-parser/packet` | `github.com/zishang520/socket.io/parsers/engine/v3/packet` |
| `github.com/zishang520/engine.io-go-parser/parser` | `github.com/zishang520/socket.io/parsers/engine/v3/parser` |
| `github.com/zishang520/engine.io-go-parser/types` | `github.com/zishang520/socket.io/v3/pkg/types` |
| `github.com/zishang520/engine.io-go-parser/utils` | `github.com/zishang520/socket.io/v3/pkg/utils` |

### Socket.IO Parser

| v1/v2 Import | v3 Import |
|--------------|-----------|
| `github.com/zishang520/socket.io-go-parser/parser` | `github.com/zishang520/socket.io/parsers/socket/v3/parser` |
| `github.com/zishang520/socket.io-go-parser/v2/parser` | `github.com/zishang520/socket.io/parsers/socket/v3/parser` |

### Engine.IO Server

| v1/v2 Import | v3 Import |
|--------------|-----------|
| `github.com/zishang520/engine.io/config` | `github.com/zishang520/socket.io/servers/engine/v3/config` |
| `github.com/zishang520/engine.io/v2/config` | `github.com/zishang520/socket.io/servers/engine/v3/config` |
| `github.com/zishang520/engine.io/engine` | `github.com/zishang520/socket.io/servers/engine/v3` |
| `github.com/zishang520/engine.io/v2/engine` | `github.com/zishang520/socket.io/servers/engine/v3` |
| `github.com/zishang520/engine.io/errors` | `github.com/zishang520/socket.io/servers/engine/v3/errors` |
| `github.com/zishang520/engine.io/v2/errors` | `github.com/zishang520/socket.io/servers/engine/v3/errors` |
| `github.com/zishang520/engine.io/events` | `github.com/zishang520/socket.io/v3/pkg/events` |
| `github.com/zishang520/engine.io/v2/events` | `github.com/zishang520/socket.io/v3/pkg/events` |
| `github.com/zishang520/engine.io/log` | `github.com/zishang520/socket.io/v3/pkg/log` |
| `github.com/zishang520/engine.io/v2/log` | `github.com/zishang520/socket.io/v3/pkg/log` |
| `github.com/zishang520/engine.io/transports` | `github.com/zishang520/socket.io/servers/engine/v3/transports` |
| `github.com/zishang520/engine.io/v2/transports` | `github.com/zishang520/socket.io/servers/engine/v3/transports` |
| `github.com/zishang520/engine.io/types` | `github.com/zishang520/socket.io/v3/pkg/types` |
| `github.com/zishang520/engine.io/v2/types` | `github.com/zishang520/socket.io/v3/pkg/types` |
| `github.com/zishang520/engine.io/utils` | `github.com/zishang520/socket.io/v3/pkg/utils` |
| `github.com/zishang520/engine.io/v2/utils` | `github.com/zishang520/socket.io/v3/pkg/utils` |
| `github.com/zishang520/engine.io/v2/webtransport` | `github.com/zishang520/socket.io/v3/pkg/webtransport` |

### Socket.IO Server

| v1/v2 Import | v3 Import |
|--------------|-----------|
| `github.com/zishang520/socket.io/socket` | `github.com/zishang520/socket.io/servers/socket/v3` |
| `github.com/zishang520/socket.io/v2/socket` | `github.com/zishang520/socket.io/servers/socket/v3` |
| `github.com/zishang520/socket.io/v2/adapter` | `github.com/zishang520/socket.io/adapters/adapter/v3` |

### Redis Adapter

| v1 Import | v3 Import |
|-----------|-----------|
| `github.com/zishang520/socket.io-go-redis/adapter` | `github.com/zishang520/socket.io/adapters/redis/v3/adapter` |
| `github.com/zishang520/socket.io-go-redis/emitter` | `github.com/zishang520/socket.io/adapters/redis/v3/emitter` |
| `github.com/zishang520/socket.io-go-redis/types` | `github.com/zishang520/socket.io/adapters/redis/v3` |

### Redis Adapter Internal Migrations (v3)

| Before (adapter subpackage) | After (redis root package) |
|-----------------------------|----------------------------|
| `adapter.SubscriptionMode` | `redis.SubscriptionMode` |
| `adapter.StaticSubscriptionMode` | `redis.StaticSubscriptionMode` |
| `adapter.DynamicSubscriptionMode` | `redis.DynamicSubscriptionMode` |
| `adapter.DynamicPrivateSubscriptionMode` | `redis.DynamicPrivateSubscriptionMode` |

### Valkey Adapter (new in v3)

The Valkey adapter is a new, independent module introduced in v3. It mirrors
the Redis adapter protocols through the
[`valkey-go`](https://github.com/valkey-io/valkey-go) client.

```bash
go get github.com/zishang520/socket.io/adapters/valkey/v3@latest
```

| Package | Import Path | Contents |
|---------|-------------|----------|
| Root | `github.com/zishang520/socket.io/adapters/valkey/v3` | Protocol types, codecs, and `ValkeyClient` |
| Adapter | `github.com/zishang520/socket.io/adapters/valkey/v3/adapter` | Classic, sharded, and Streams adapters |
| Emitter | `github.com/zishang520/socket.io/adapters/valkey/v3/emitter` | Classic, sharded, and Streams emitters |

**Example `go.mod`:**

```go
require (
    github.com/zishang520/socket.io/adapters/valkey/v3 v3.x.y
)
```

**Usage:**

```go
import (
    "context"
    vk "github.com/valkey-io/valkey-go"
    valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
    vkadapter "github.com/zishang520/socket.io/adapters/valkey/v3/adapter"
)

client, _ := vk.NewClient(vk.ClientOption{InitAddress: []string{"localhost:6379"}})
valkeyClient, err := valkey.NewValkeyClient(context.Background(), client)
if err != nil {
    panic(err)
}
server.SetAdapter(&vkadapter.ValkeyAdapterBuilder{Valkey: valkeyClient})
```

#### Client roles

The first `vk.Client` passed to `NewValkeyClient` or
`NewValkeyClientWithSub` is primary-routed. It handles writes and the
consistency-sensitive reads used for connection-state recovery. The optional
second client handles subscriptions, blocking `XREAD`, and Pub/Sub subscriber
counts.

```go
primaryClient, _ := vk.NewClient(vk.ClientOption{InitAddress: []string{"primary:6379"}})
subClient, _ := vk.NewClient(vk.ClientOption{InitAddress: []string{"replica:6380"}})
valkeyClient, err := valkey.NewValkeyClientWithSub(context.Background(), primaryClient, subClient)
if err != nil {
    panic(err)
}
server.SetAdapter(&vkadapter.ValkeyAdapterBuilder{Valkey: valkeyClient})
```

`valkey-go` does not expose the read-only or routing options used to create an
existing `vk.Client`, so the wrapper cannot verify that the first client is
primary-routed. Supplying a write-capable client with primary-consistent
recovery reads is therefore a caller contract. Configure the second client so
its subscriptions and subscriber-count commands see the intended Pub/Sub
topology.

Create one `ValkeyClient` wrapper and one underlying `Sub()` client per
Socket.IO server, then share the wrapper across that server's namespaces. The
primary write client may be shared through `NewValkeyClientWithSub`, but the
subscription client must be distinct for each server. This matches the Node.js
adapters' duplicated subscription-client ownership and preserves server counts.

#### API and protocol finalization

- Classic adapter, sharded adapter, and emitter options are separate. Use
  `ValkeyAdapterOptions` for `Key`, `Parser`, `RequestsTimeout`, and
  `PublishOnSpecificResponseChannel`; use `ShardedValkeyAdapterOptions` for
  `ChannelPrefix` and `SubscriptionMode`; and configure external emission with
  `EmitterOptions`.
- The classic emitter custom codec is outbound-only. Rename `SetParser`,
  `GetRawParser`, and `Parser` to `SetEncoder`, `GetRawEncoder`, and `Encoder`.
  `BroadcastOptions.Parser` is likewise now `Encoder`, and its `Sharded` field
  was removed. Sharded emission always uses the shared cluster-message codec.
  Custom `BroadcastOperatorInterface` implementations must also add
  `ServerSideEmit(args ...any) error`.
- `ValkeyPubSub` subscriptions are fixed at construction. The public
  unsubscribe mutation methods were removed; call `Close` to end a
  subscription. `SSubscribe` now accepts one channel, preventing a multi-slot
  sharded subscription from reaching valkey-go's cluster panic path. Logical
  subscriptions with the same kind and topic within one `ValkeyClient` now
  share one physical subscription while retaining independent queues. One
  wrapper and its underlying `Sub()` client represent one Socket.IO server;
  each server must use a distinct subscription client. The subscriber ACL must
  grant `SUBSCRIBE`/`UNSUBSCRIBE`,
  `PSUBSCRIBE`/`PUNSUBSCRIBE`, and `SSUBSCRIBE`/`SUNSUBSCRIBE` as pairs. `Close`
  now waits for cleanup and returns any unsubscribe error. `ValkeyMessage` was
  removed; `ReceiveMessage` now returns valkey-go's `vk.PubSubMessage` by value,
  so replace its `Payload` field with `Message`.
- `RawClusterMessage` is now the Streams wire shape `map[string]string`.
  `ValkeyClient.XAdd` accepts it before `maxLen`, and `ValkeyClient.XRead` reads
  one stream per call. The generic multi-stream/map forms were removed because
  the adapter protocol never used them.
- `ValkeyClient.XRange` was removed. Use `XRangeN` with an explicit count.
- Rename the Streams adapter constant `DefaultStreamChannelPrefix` to
  `DefaultChannelPrefix`, matching the Redis Streams adapter API.
- `ValkeyStreamsAdapter.Cleanup` was removed because shared poller cleanup is
  internal. `ValkeyStreamsAdapterOptions` no longer embeds
  `ClusterAdapterOptions`; its heartbeat fields were never consumed by the
  Streams adapter.
- `NewValkeyStreamsEmitter` and `ValkeyStreamsEmitterOptions` add the Streams
  emitter alongside the classic and sharded emitters.
- Emitter and adapter transport settings must match: classic `Key`; sharded
  emitter `Key` with adapter `ChannelPrefix`, plus `SubscriptionMode`; and
  Streams `StreamName` and `StreamCount`. Streams writers should also share the
  same `MaxLen`, which controls retention rather than routing.
- Each active shared poller holds one connection from the `Sub()` blocking
  pool. Classic Pub/Sub uses one additional dedicated connection per
  `ValkeyClient` so normal and pattern messages remain ordered.
  Other sharded and Streams Pub/Sub notifications remain multiplexed. Size the
  pool for these connections and other blocking operations. Mixed Go and
  Node.js deployments must use
  `StreamCount=1`: the Node.js standalone emitter writes only to the base
  stream, and its server adapter does not poll the negative stream suffixes its
  signed namespace hash can produce.
- The classic request/response wire now matches Node.js: request types and
  request-scope fields are explicit, broadcast acknowledgements carry a scalar
  acknowledgement value, zero `clientCount` values are preserved, and required
  arrays/options reject missing or `null` values.
- Dynamic room classification and namespace stream routing use JavaScript
  UTF-16 code-unit semantics. This keeps private-room channel selection and
  signed namespace hashing consistent with Node.js for non-BMP characters.
- Cross-language compound payloads preserve nested binary values only in
  `[]any` and `map[string]any`; typed structs, slices, and maps are not
  recursively inspected.

These API and wire changes require a coordinated restart of Go Valkey nodes.
Mixing nodes that use the earlier Go Valkey wire format is not supported.

### Engine.IO Client

| v1 Import | v3 Import |
|-----------|-----------|
| `github.com/zishang520/engine.io-client-go/engine` | `github.com/zishang520/socket.io/clients/engine/v3` |
| `github.com/zishang520/engine.io-client-go/request` | `github.com/zishang520/socket.io/v3/pkg/request` |
| `github.com/zishang520/engine.io-client-go/transports` | `github.com/zishang520/socket.io/clients/engine/v3/transports` |

### Socket.IO Client

| v1 Import | v3 Import |
|-----------|-----------|
| `github.com/zishang520/socket.io-client-go/socket` | `github.com/zishang520/socket.io/clients/socket/v3` |
| `github.com/zishang520/socket.io-client-go/utils` | `github.com/zishang520/socket.io/v3/pkg/utils` |

### Error Types (New in v3)

| Old Import | v3 Import |
|------------|-----------|
| `clients/socket.ExtendedError` | `github.com/zishang520/socket.io/v3/pkg/types.ExtendedError` |
| `servers/socket.ExtendedError` | `github.com/zishang520/socket.io/v3/pkg/types.ExtendedError` |

> **Tip:** Use `grep -r "github.com/zishang520" .` to find all old imports, then use find-and-replace to update them systematically.

---

## Breaking Changes

### Protocol Compatibility

Socket.IO v3 aligns with the Socket.IO v4+ protocol. Ensure your client-side Socket.IO library is updated to version 4.x or higher.

```bash
npm install socket.io-client@^4.0.0
```

### Redis Adapter Type Updates

If you're using the Redis adapter, you must replace all instances of `types.String` with `types.Atomic[string]`:

```go
// Before
import "github.com/zishang520/socket.io-go-redis/types"

func example() {
    var roomName types.String
    roomName.Store("lobby")
    value := roomName.Load()
}

// After
import "github.com/zishang520/socket.io/v3/pkg/types"

func example() {
    var roomName types.Atomic[string]
    roomName.Store("lobby")
    value := roomName.Load()
}
```

### Adapter Payload Reader Normalization

Adapter payload normalization now consistently reads generic `io.Reader`
values. An additional `Bytes()` method no longer bypasses the reader path, so
the common codec and the Redis, Valkey, and PostgreSQL adapters use the same
best-effort behavior without adding error-return plumbing to their public APIs.
The Redis and Valkey JSON-specific normalization helpers are now internal.

### Socket Handshake Access Patterns

Update code that accesses handshake headers and query parameters:

```go
// Before
func handleConnection(socket *socket.Socket) {
    headers := socket.Handshake().Headers
    userAgent := headers["user-agent"][0]

    query := socket.Handshake().Query
    token := query["token"][0]
}

// After
func handleConnection(socket *socket.Socket) {
    headers := socket.Handshake().Headers.Header()
    userAgent := headers.Get("User-Agent")

    query := socket.Handshake().Query.Query()
    token := query.Get("token")
}
```

### Configuration Method Returns

Update code that uses `GetRaw*` configuration methods:

```go
// Before
func configExample(config ConnectionStateRecoveryInterface) {
    if duration := config.GetRawMaxDisconnectionDuration(); duration != nil {
        fmt.Printf("Duration: %d", *duration)
    }
}

// After
func configExample(config ConnectionStateRecoveryInterface) {
    if duration := config.GetRawMaxDisconnectionDuration(); duration != nil {
        fmt.Printf("Duration: %d", duration.Get())
    }
}
```

### ParameterBag Package Migration

Update `*utils.ParameterBag` to `*types.ParameterBag`:

```go
// Before
import "github.com/zishang520/socket.io/v3/pkg/utils"
var bag *utils.ParameterBag = utils.NewParameterBag(nil)

// After
import "github.com/zishang520/socket.io/v3/pkg/types"
var bag *types.ParameterBag = types.NewParameterBag(nil)
```

### Transport Upgrade Methods

Transport upgrade methods now return `[]string` instead of `*types.Set[string]`:

```go
// Before
upgrades := transport.Upgrades() // *types.Set[string]

// After
upgrades := transport.Upgrades() // []string
```

### HttpContext API Migration

| Before | After |
|--------|-------|
| `ctx.ResponseHeaders` | `ctx.ResponseHeaders()` |
| `ctx.GetHost()` | `ctx.Host()` |
| `ctx.GetMethod()` | `ctx.Method()` |
| `ctx.Gets("key")` | `ctx.Query().Gets("key")` |
| `ctx.Get("key")` | `ctx.Query().Get("key")` |
| `ctx.GetPathInfo()` | `ctx.PathInfo()` |

### Utility Functions Migration

| Before | After |
|--------|-------|
| `adapter.SliceMap(...)` | `slices.Map(...)` |
| `adapter.Tap(...)` | `utils.Tap(...)` |

### ExtendedError API Migration

Server-side `Data()` method is now a field:

```go
// Before
data := err.Data()

// After  
data := err.Data
```

### Redis SubscriptionMode Migration

If using the sharded Redis adapter, update `SubscriptionMode` imports:

```go
// Before
import "github.com/zishang520/socket.io/adapters/redis/v3/adapter"

opts.SetSubscriptionMode(adapter.DynamicSubscriptionMode)

// After
import "github.com/zishang520/socket.io/adapters/redis/v3"

opts.SetSubscriptionMode(redis.DynamicSubscriptionMode)
```

---

## Quick Start Example

Here is a minimal server example after upgrading to v3:

```go
package main

import (
	"fmt"
	"net/http"

	server "github.com/zishang520/socket.io/servers/socket/v3"
)

func main() {
	io := server.NewServer(nil, nil)

	io.On("connection", func(args ...any) {
		socket := args[0].(*server.Socket)
		fmt.Printf("connected: %s\n", socket.Id())

		socket.On("message", func(args ...any) {
			fmt.Printf("received: %v\n", args)
			socket.Emit("message", args...)
		})

		socket.On("disconnect", func(args ...any) {
			fmt.Printf("disconnected: %s\n", socket.Id())
		})
	})

	http.Handle("/socket.io/", io.ServeHandler(nil))
	fmt.Println("server listening on :3000")
	http.ListenAndServe(":3000", nil)
}
```

---

## Testing Your Upgrade

After completing the upgrade, thoroughly test your application:

### 1. Run Test Suite

```bash
go test ./...
```

### 2. Test Core Functionality

- Client connections and disconnections
- Event emission and reception
- Namespace and room operations
- Redis adapter broadcasting (if applicable)

### 3. Enable Debug Logging

Set the `DEBUG` environment variable to enable verbose logging:

```bash
# Linux / macOS
DEBUG=socket.io:* go run main.go

# Windows PowerShell
$env:DEBUG="socket.io:*"; go run main.go
```

### 4. Verify Client Compatibility

Ensure your frontend uses Socket.IO client v4.x or higher.

```bash
npm install socket.io-client@^4.0.0
```

### 5. Run Benchmarks (Optional)

The `examples/benchmark` module provides a built-in benchmark test for validating performance:

```bash
cd examples/benchmark
go run main.go
```

---

## Common Issues

### Import Resolution Errors

```bash
go mod tidy
go clean -modcache
go mod download
```

### Connection Protocol Mismatches

```bash
npm install socket.io-client@^4.0.0
```

### ExtendedError API Changes

If you encounter errors with `Data()` method calls on `ExtendedError`:

```go
// Before (server-side)
data := err.Data()

// After
data := err.Data
```

---

## Need Help?

- [GitHub Issues](https://github.com/zishang520/socket.io/issues) — for confirmed bugs or feature requests
- [GitHub Discussions](https://github.com/zishang520/socket.io/discussions/new?category=q-a) — for general questions and help
- [Go Package Documentation](https://pkg.go.dev/github.com/zishang520/socket.io/v3) — API reference
- [Socket.IO Protocol Documentation](https://socket.io/docs/v4/) — protocol specification
- [Socket.IO Go Repository](https://github.com/zishang520/socket.io) — source code and examples

---

## Release Notes

### v3.0.0

> Released on 2026-04-13

This is the **first stable release** of Socket.IO for Go v3. It includes all changes from the alpha, beta, and RC phases.

#### Highlights Since v2

- **Monorepo consolidation**: 6 separate repositories merged into one monorepo with 9 versioned Go submodules
- **Unified versioning**: Single version source at `pkg/version/version.go` shared by all modules
- **Go 1.26.0 minimum**: Takes advantage of the latest Go features
- **Protocol alignment**: Compatible with Socket.IO v4+ JavaScript clients
- **Thread safety overhaul**: Atomic socket flags (copy-on-write), mutex-protected middleware, `sync.OnceValue` for lazy initialization, goroutine leak prevention via `runtime.SetFinalizer`
- **Type safety improvements**: Generic `types.Atomic[T]`, `types.Optional[T]` for null safety, strongly typed `Handshake` fields (`IncomingHttpHeaders`, `ParsedUrlQuery`)
- **New packages**: `pkg/slices` (safe slice operations), `pkg/queue` (sequential task queue for message ordering), `pkg/request` (HTTP client)
- **Redis Cluster support**: Sharded broadcast operator, CROSSSLOT error fixes, dynamic channel subscriptions, pagination for session restoration
- **Security hardening**: HTTP body size limits on polling (DoS prevention), configurable attachment count limits (default 10), immutable packet encoding
- **Code quality**: golangci-lint integration, `errcheck` violations resolved, magic numbers replaced with named constants, standardized debug logging

#### Migrating

For a complete migration guide from v1/v2, see [Upgrading from v1/v2 to v3](#upgrading-from-v1v2-to-v3).

#### Full Changelog

See the individual RC/beta/alpha release notes below for detailed per-release changes.

---

### v3.0.0-rc.14

> Released from commit [`cc50fc2`](https://github.com/zishang520/socket.io/commit/cc50fc2)

#### Breaking Changes and Behavior Updates

<details>
<summary>Parser: ERROR_PACKET Removed from Public API</summary>

**Likelihood Of Impact: Low (only if directly referencing ERROR_PACKET)**

The shared mutable `ERROR_PACKET` singleton has been removed from the public API to prevent data race conditions. It has been replaced with an internal `newErrorPacket()` factory function that creates a fresh instance each time, avoiding shared mutable state across goroutines.

```go
// Before (no longer works)
import "github.com/zishang520/socket.io/parsers/engine/v3/parser"
var errPkt = parser.ERROR_PACKET

// After (use alternatives)
// If you need error packet creation, use the public parser APIs
// that internally create error packets as needed
```

**Impact:** This is unlikely to affect most applications since `ERROR_PACKET` was an internal constant. If you were using it directly, you should rely on the public parser API methods instead.
</details>

<details>
<summary>Socket Packet Encoder: Encode() No Longer Mutates Input</summary>

**Likelihood Of Impact: Low**

The `Encode()` method in the Socket.IO packet encoder now creates a copy of the packet before mutation, preventing unintended side effects on the caller's packet object.

```go
// Before - Encode() modified the input packet's Type field
import "github.com/zishang520/socket.io/parsers/socket/v3/parser"

pkt := &packet.Packet{Type: parser.EVENT, Data: binaryData}
encoded := encoder.Encode(pkt)
// pkt.Type would now be BINARY_EVENT (mutated!)

// After - Input packet is not modified
pkt := &packet.Packet{Type: parser.EVENT, Data: binaryData}
encoded := encoder.Encode(pkt)
// pkt.Type remains EVENT (not mutated)
```

**Impact:** This is a behavior fix that makes code more predictable. If your code was relying on the side effect of `Encode()` mutating the input packet, you need to update it to handle packets immutably.
</details>

<details>
<summary>Socket.IO Parser: Configurable Attachment Count Limit</summary>

**Likelihood Of Impact: Low**

The attachment limit has been reduced from a hardcoded 1000 to a configurable per-decoder instance default of 10 (aligned with the upstream Node.js implementation). The limit is now controlled via `DecoderOptions` instead of a package-level constant.

```go
import "github.com/zishang520/socket.io/parsers/socket/v3/parser"

// Default - limited to 10 attachments per packet
decoder := parser.NewDecoder()

// Custom limit for applications that need more attachments
decoder := parser.NewDecoder(&parser.DecoderOptions{
    MaxAttachments: 50,
})
```

Packets exceeding the limit will be rejected with `parser.ErrTooManyAttachments`.

**Impact:** Applications sending more than 10 attachments in a single packet will now be rejected. If you encounter this error, split large payloads into multiple packets or configure a higher limit.
</details>

<details>
<summary>Engine.IO Polling: HTTP Body Size Limit</summary>

**Likelihood Of Impact: Medium (only if sending very large payloads via polling)**

The polling transport now enforces `MaxHttpBufferSize` limit on request body reads to prevent unbounded memory consumption (DoS prevention).

```go
// Before - No limit on body size
// Large payloads could cause excessive memory usage

// After - Limited by MaxHttpBufferSize (default 1 MB)
// Large payloads exceeding the limit are truncated/rejected
```

**Impact:** If you're sending payloads larger than `MaxHttpBufferSize` (default 1 MB) via polling transport, they will be truncated or rejected. Use WebSocket/WebTransport for larger messages or increase the limit:

```go
import "github.com/zishang520/socket.io/servers/engine/v3/config"

opts := config.DefaultServerOptions()
opts.SetMaxHttpBufferSize(10 * 1024 * 1024) // 10 MB
```
</details>

#### Bug Fixes

<details>
<summary>WebSocket/WebTransport: Send Loop Behavior</summary>

**Likelihood Of Impact: Very Low**

Fixed send loop early return bug that was previously dropping remaining packets in queue when an encoded frame was sent successfully.

```go
// Before - Send loop would return after first packet, dropping queue
// Packet 1: sent
// Packet 2, 3, ...: dropped (never sent)

// After - Send loop continues processing all queued packets
// All packets in queue are sent correctly
```

**Impact:** This is a bug fix that improves reliability. Previously, only the first queued packet would be sent; now all queued packets are sent as expected. No code changes required.
</details>

<details>
<summary>Middleware Thread Safety</summary>

**Likelihood Of Impact: Very Low (only if modifying middleware during runtime)**

Engine.IO base server now protects middleware slice with `sync.RWMutex` for concurrent-safe reading and modification.

```go
// Before - Unsafe concurrent middleware modification
go server.Use(middleware1) // Racing writes
go server.Use(middleware2) // Could panic or miss middleware

// After - Thread-safe middleware operations
go server.Use(middleware1) // Safe
go server.Use(middleware2) // Safe
```

**Impact:** This is a thread safety fix. No code changes required.
</details>

<details>
<summary>Socket Flags: Concurrent Mutation Safety</summary>

**Likelihood Of Impact: Very Low**

Socket flag mutations (Compress, Volatile, Timeout) now use `atomic.Pointer` with copy-on-write to prevent race conditions.

```go
// Before - Racing flag mutations could cause data races
go socket.Compress(true)
go socket.Volatile()
// Data race condition possible

// After - All flag mutations are thread-safe
go socket.Compress(true)
go socket.Volatile()
// Safe concurrent mutations
```

**Impact:** This is a thread safety fix. No code changes required.
</details>

<details>
<summary>Queue: Goroutine Leak Prevention</summary>

**Likelihood Of Impact: Very Low**

The task queue now uses `runtime.SetFinalizer()` to prevent goroutine leaks when queue instances are garbage collected.

**Impact:** This is a resource leak fix. Applications with long-running queues may see reduced goroutine count. No code changes required.
</details>

<details>
<summary>Message Ordering and OOM Prevention</summary>

**Likelihood Of Impact: Very Low**

Resolves [#116](https://github.com/zishang520/socket.io/issues/116). A new sequential task queue (`pkg/queue`) preserves message ordering and prevents OOM under high concurrency. Both client and server transports now use this queue for send operations.

**Impact:** This is a reliability fix. No code changes required.
</details>

#### Internal Improvements

- Debug logging standardized across all packages using `pkg/log`
- Magic numbers replaced with named constants throughout the codebase
- Client constants extracted and network monitoring leak fixed
- Go minimum version is now 1.26.0

---

### v3.0.0-rc.13

> Released from commit [`5b988b6`](https://github.com/zishang520/socket.io/commit/5b988b6)

#### Highlights

- **Go 1.26.0 required**: Minimum Go version bumped to 1.26.0
- **golangci-lint integration**: Linting is now integrated into the build system via `Makefiles`
- **Improved error handling**: `errcheck` violations resolved across the entire codebase, replacing error suppression with proper handling or explicit `io.Closer` patterns

#### Redis Adapter Improvements

- Enhanced polling mechanism and added pagination for session restoration
- Improved dynamic channel subscription management in the sharded Redis adapter
- Added `MessageType` validation and improved error handling

#### Bug Fixes

- Fixed nil pointer dereference caused by race condition in Engine.IO (`76a0015`)
- Fixed `Peek` method added to `Buffer` type with integer overflow protection (`ef32276`, `5d3ea31`)

---

### v3.0.0-rc.12

> Released from commit [`e854211`](https://github.com/zishang520/socket.io/commit/e854211)

#### Highlights

<details>
<summary>ExtendedError Type Consolidation</summary>

**Likelihood Of Impact: Medium**

The `ExtendedError` type has been consolidated from separate implementations in `clients/socket` and `servers/socket` packages into a single shared implementation in `pkg/types`. This eliminates code duplication and provides a consistent error type across the entire codebase.

```go
// Before (client-side)
import "github.com/zishang520/socket.io-client-go/socket"

err := socket.NewExtendedError("connection failed", nil)

// Before (server-side)
import "github.com/zishang520/socket.io/v2/socket"

err := socket.NewExtendedError("middleware error", map[string]any{"code": 401})
data := err.Data()  // Note: server-side had Data() method

// After (unified)
import "github.com/zishang520/socket.io/v3/pkg/types"

err := types.NewExtendedError("error message", map[string]any{"code": 401})
data := err.Data  // Now uses direct field access
```

**Key changes:**

- `clients/socket.ExtendedError` → `types.ExtendedError`
- `servers/socket.ExtendedError` → `types.ExtendedError` (type alias maintained for backward compatibility)
- Server-side `Data()` method replaced with `Data` field for consistency
- Both client and server now share the same `ExtendedError` implementation

**Note:** The server-side `socket` package retains a type alias for `ExtendedError` and a wrapper function `NewExtendedError` for backward compatibility, so existing server code may continue to work without changes. However, client-side code must update imports.
</details>

<details>
<summary>Redis SubscriptionMode Type Migration</summary>

**Likelihood Of Impact: Medium (if using Redis sharded adapter)**

The `SubscriptionMode` type has been moved from `adapters/redis/adapter` package to the root `adapters/redis` package for better organization and sharing between adapter and emitter.

```go
// Before
import "github.com/zishang520/socket.io/adapters/redis/v3/adapter"

opts := adapter.NewShardedRedisAdapterOptions()
opts.SetSubscriptionMode(adapter.DynamicSubscriptionMode)

// After
import (
    "github.com/zishang520/socket.io/adapters/redis/v3"
    "github.com/zishang520/socket.io/adapters/redis/v3/adapter"
)

opts := adapter.DefaultShardedRedisAdapterOptions()
opts.SetSubscriptionMode(redis.DynamicSubscriptionMode)
```

**Key changes:**

| Before | After |
|--------|-------|
| `adapter.SubscriptionMode` | `redis.SubscriptionMode` |
| `adapter.StaticSubscriptionMode` | `redis.StaticSubscriptionMode` |
| `adapter.DynamicSubscriptionMode` | `redis.DynamicSubscriptionMode` |
| `adapter.DynamicPrivateSubscriptionMode` | `redis.DynamicPrivateSubscriptionMode` |

**New additions:**

- `redis.DefaultSubscriptionMode` - Default mode constant
- `redis.PrivateRoomIdLength` - Length constant for private room detection
- `redis.ShouldUseDynamicChannel(mode, room)` - Shared helper function

**Emitter options extended:**

```go
emitterOpts := emitter.DefaultEmitterOptions()
emitterOpts.SetSharded(true)
emitterOpts.SetSubscriptionMode(redis.DynamicSubscriptionMode)
```
</details>

#### Redis Adapter Improvements

- Added sharded broadcast operator for Redis Cluster support (`d83b4db`)
- Fixed timeout when fetching sockets from empty rooms (`d5cfa20`)
- Fixed Redis Cluster CROSSSLOT errors by managing separate PubSub clients per channel (`2629cc1`)
- Improved binary packet handling and code organization

---

### v3.0.0-rc.8

> Released from commit [`b2f5457`](https://github.com/zishang520/socket.io/commit/b2f5457)

#### Highlights

<details>
<summary>Adapter Utility Functions Reorganization</summary>

**Likelihood Of Impact: Medium**

Utility functions `SliceMap` and `Tap` have been moved from the `adapter` package to dedicated `pkg` subpackages.

```go
// Before
import "github.com/zishang520/socket.io/adapters/adapter/v3"

func example() {
    adapter.SliceMap(/**/)
    adapter.Tap(/**/)
}

// After
import (
    "github.com/zishang520/socket.io/v3/pkg/slices"
    "github.com/zishang520/socket.io/v3/pkg/utils"
)

func example() {
    slices.Map(/**/)
    utils.Tap(/**/)
}
```

**Changes summary:**

- `adapter.SliceMap` → `slices.Map` (moved to `pkg/slices`)
- `adapter.Tap` → `utils.Tap` (moved to `pkg/utils`)

**New functions in `pkg/slices`:**

The new `pkg/slices` package provides additional utility functions:

| Function | Description |
|----------|-------------|
| `Get(s, idx)` | Safely retrieves an element with bounds checking |
| `GetAny[O](vals, idx)` | Retrieves and type-asserts from `[]any` |
| `TryGet(s, idx)` | Returns zero value if out of bounds |
| `TryGetAny[O](vals, idx)` | Type-asserts from `[]any` or returns zero |
| `GetWithDefault(s, idx, def)` | Returns default value if out of bounds |
| `GetPtr(s, idx)` | Returns pointer to element or nil |
| `Slice(s, start)` | Safe sub-slice with bounds checking |
| `First(s)` / `Last(s)` | Get first/last element safely |
| `Filter(s, predicate)` | Filter elements by predicate |
| `Map(vals, transform)` | Transform each element |
| `Reduce(vals, initial, reducer)` | Reduce to single value |
| `IsEmpty(s)` | Check if slice is nil or empty |
| `IsValidIndex(s, idx)` | Check if index is valid |
</details>

<details>
<summary>HttpContext API Refactoring</summary>

**Likelihood Of Impact: Medium**

Several methods and properties of `*types.HttpContext` have been renamed or refactored from properties to methods. All lazy-loaded methods now use `sync.OnceValue` for thread safety.

```go
// Before
func example(ctx *types.HttpContext) {
    headers := ctx.ResponseHeaders
    host := ctx.GetHost()
    method := ctx.GetMethod()
    values := ctx.Gets("foo")
    value := ctx.Get("bar")
    path := ctx.GetPathInfo()
}

// After
func example(ctx *types.HttpContext) {
    headers := ctx.ResponseHeaders()
    host := ctx.Host()
    method := ctx.Method()
    values, _ := ctx.Query().Gets("foo")
    value, _ := ctx.Query().Get("bar")
    path := ctx.PathInfo()
}
```

**Changes summary:**

- `ResponseHeaders` → `ResponseHeaders()` (property to method)
- `GetHost()` → `Host()`
- `GetMethod()` → `Method()`
- `Gets(key)` → `Query().Gets(key)`
- `Get(key)` → `Query().Get(key)`
- `GetPathInfo()` → `PathInfo()`

**New/Updated methods:**

| Method | Description |
|--------|-------------|
| `Path()` | Returns cleaned path (without leading/trailing slashes) |
| `UserAgent()` | Returns User-Agent header value |
| `Secure()` | Returns `true` if TLS connection |
| `SetStatusCode(code)` | Now returns `error` for validation |
| `IsDone()` | Check if response has been written |
| `Done()` | Returns `<-chan struct{}` instead of `<-chan Void` |
</details>

<details>
<summary>ParameterBag Package Migration</summary>

**Likelihood Of Impact: Medium**

`ParameterBag` has been moved from the `utils` package to the `types` package.

```go
// Before
import "github.com/zishang520/socket.io/v3/pkg/utils"

func example() {
    var bag *utils.ParameterBag
    bag = utils.NewParameterBag(nil)
}

// After
import "github.com/zishang520/socket.io/v3/pkg/types"

func example() {
    var bag *types.ParameterBag
    bag = types.NewParameterBag(nil)
}
```
</details>

---

### v3.0.0-rc.4

> Released from commit [`d7c93b5`](https://github.com/zishang520/socket.io/commit/d7c93b5)

#### Highlights

<details>
<summary>Socket Handshake Type Updates</summary>

**Likelihood Of Impact: Medium**

The `socket.Handshake` structure now uses more strongly typed fields:

```go
// Before
type Handshake struct {
    Headers map[string][]string
    Query   map[string][]string
    Auth    any
}

// After
type Handshake struct {
    Headers types.IncomingHttpHeaders  // provides Header() method
    Query   types.ParsedUrlQuery       // provides Query() method
    Auth    map[string]any
}
```

Access patterns must be updated:

```go
// Before
headers := socket.Handshake().Headers
userAgent := headers["user-agent"][0]

// After
headers := socket.Handshake().Headers.Header()
userAgent := headers.Get("User-Agent")
```
</details>

<details>
<summary>Auth Parameter Standardization</summary>

**Likelihood Of Impact: Medium**

The `Auth` field in `Handshake` is now standardized to `map[string]any` instead of `any`. This provides a consistent type for authentication data.

```go
// Before
auth := socket.Handshake().Auth // type: any
if authMap, ok := auth.(map[string]any); ok {
    token := authMap["token"]
}

// After
auth := socket.Handshake().Auth // type: map[string]any
token := auth["token"]
```
</details>

<details>
<summary>Optional[T] Enhancements</summary>

**Likelihood Of Impact: Low**

The `Optional[T]` interface now includes `IsPresent()` and `IsEmpty()` methods, and `Some.Get()` handles nil receiver gracefully.

```go
if duration := config.GetRawMaxDisconnectionDuration(); duration != nil && duration.IsPresent() {
    fmt.Printf("Duration: %d", duration.Get())
}
```
</details>

---

### v3.0.0-rc.2

> Released from commit [`540c239`](https://github.com/zishang520/socket.io/commit/540c239)

#### Bug Fixes

- Fixed panic when client sends nil payload in Socket.IO parser (`80fe0b9`)

#### Internal Changes

- Replaced `GetRaw*` method calls with direct property access for better readability (`ce8f623`)

---

### v3.0.0-beta.1

> Released from commit [`01f5eca`](https://github.com/zishang520/socket.io/commit/01f5eca)

#### Highlights

<details>
<summary>Config GetRaw* Method Changes</summary>

**Likelihood Of Impact: Medium**

All `GetRaw*` methods now return `types.Optional[T]` instead of pointer types for better null safety:

```go
// Before
func configExample(config ConnectionStateRecoveryInterface) {
    if duration := config.GetRawMaxDisconnectionDuration(); duration != nil {
        fmt.Printf("Duration: %d", *duration)
    }
}

// After
func configExample(config ConnectionStateRecoveryInterface) {
    if duration := config.GetRawMaxDisconnectionDuration(); duration != nil {
        fmt.Printf("Duration: %d", duration.Get())
    }
}
```
</details>

#### Bug Fixes

- Fixed HTTP/2 connection goroutine leaks in `HTTPClient.Close()` (`069619b`)
- Fixed timer goroutine leaks adapted from upstream (`ff5d935`)

---

### v3.0.0-alpha.0 ~ alpha.4

> Alpha releases covering the initial v3 restructuring

#### Highlights

- **Dependency Consolidation**: All previously separate repositories (`engine.io-go-parser`, `engine.io`, `socket.io-go-parser`, `socket.io-client-go`, `socket.io-go-redis`) have been merged into a single monorepo with versioned submodules
- **Import Path Restructuring**: All package import paths updated to the new `github.com/zishang520/socket.io/` namespace (see [Import Path Updates](#import-path-updates))
- **Type-safe Atomic Types**: `atomic.Value` replaced with generic `types.Atomic[T]` for type safety (`7389549`)
- **Redis Adapter Type Updates**: `types.String` replaced with `types.Atomic[string]`
- **Server Options Refactoring**: Consolidated server options interfaces and structures for improved clarity (`a396fef`)
- **Transport Upgrade Methods**: Updated to return `[]string` instead of `*types.Set[string]` (`f3c4cd8`)
- **Version Management**: Added `cmd/socket.io` module with version command and per-module version files

---

## Additional Notes

| Recommendation | Details |
|----------------|---------|
| **Backup First** | Always backup your codebase before upgrading |
| **Go Version** | Ensure you're using Go 1.26.0 or higher |
| **Staged Rollout** | Consider upgrading non-critical components first |
| **Client Coordination** | Coordinate with frontend team for Socket.IO client v4.x+ compatibility |
| **Security Updates** | v3.0.0 includes important DoS prevention and data race fixes |
| **Vendor Directory** | If using `go mod vendor`, run `go mod vendor` after updating dependencies |
| **IDE Support** | Restart your IDE/language server after updating imports for accurate code completion |
