# Upgrade Guide

## Table of Contents

- [Upgrading from v1/v2 to v3](#upgrading-from-v1v2-to-v3)
  - [Estimated Upgrade Time](#estimated-upgrade-time-30---60-minutes)
  - [High Impact Changes](#high-impact-changes)
  - [Medium Impact Changes](#medium-impact-changes)
  - [Low Impact Changes](#low-impact-changes)
  - [Updating Dependencies](#updating-dependencies)
  - [Import Path Updates](#import-path-updates)
  - [Breaking Changes](#breaking-changes)
  - [Testing Your Upgrade](#testing-your-upgrade)
  - [Common Issues](#common-issues)
  - [Need Help?](#need-help)

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

### Medium Impact Changes

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

opts := adapter.NewShardedRedisAdapterOptions()
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
emitterOpts := emitter.NewEmitterOptions()
emitterOpts.SetSharded(true)
emitterOpts.SetSubscriptionMode(redis.DynamicSubscriptionMode)
```
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

```bash
DEBUG=socket.io:* go run main.go
```

### 4. Verify Client Compatibility

Ensure your frontend uses Socket.IO client v4.x or higher.

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

- [GitHub Issues](https://github.com/zishang520/socket.io/issues)
- [Socket.IO Go Repository](https://github.com/zishang520/socket.io)
- [Socket.IO Protocol Documentation](https://socket.io/docs/v4/)

---

## Additional Notes

| Recommendation | Details |
|----------------|---------|
| **Backup First** | Always backup your codebase before upgrading |
| **Go Version** | Ensure you're using Go 1.24 or higher |
| **Staged Rollout** | Consider upgrading non-critical components first |
| **Client Coordination** | Coordinate with frontend team for client compatibility |
