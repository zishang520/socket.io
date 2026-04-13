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

```go
emitterOpts := emitter.NewEmitterOptions()
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
