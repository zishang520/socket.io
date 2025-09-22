# Upgrade Guide

- [Upgrading from v1/v2 to v3](#upgrading-from-v1v2-to-v3)
  - [Estimated Upgrade Time: 30 - 60 Minutes](#estimated-upgrade-time-30---60-minutes)
  - [High Impact Changes](#high-impact-changes)
  - [Medium Impact Changes](#medium-impact-changes)
  - [Updating Dependencies](#updating-dependencies)
  - [Import Path Updates](#import-path-updates)
  - [Breaking Changes](#breaking-changes)

## Upgrading from v1/v2 to v3

### Estimated Upgrade Time: 30 - 60 Minutes

We recommend reviewing this entire upgrade guide to understand all of the changes. The upgrade process consolidates dependencies and updates import paths to align with the Socket.IO v4+ protocol, introducing improved performance and updated APIs.

### High Impact Changes

<details>
<summary>Dependency Structure Consolidation</summary>

All Socket.IO related packages are now consolidated under the main `github.com/zishang520/socket.io/` repository with versioned submodules. This change affects every import in your application.

**Likelihood Of Impact: Very High**

All imports must be updated to use the new v3 paths.
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
</details>

<details>
<summary>Config GetRaw* Method Changes</summary>

All `GetRaw*` methods now return `types.Optional[T]` instead of pointer types for better null safety:

**Likelihood Of Impact: Medium**

```go
// Before
GetRawMaxDisconnectionDuration() *int64

// After
GetRawMaxDisconnectionDuration() types.Optional[int64]
```
</details>

## Updating Dependencies

You should update your `go.mod` to require the Socket.IO v3 packages:

```bash
go get github.com/zishang520/socket.io/v3@latest
go get github.com/zishang520/socket.io/parsers/engine/v3@latest
go get github.com/zishang520/socket.io/parsers/socket/v3@latest
go get github.com/zishang520/socket.io/servers/engine/v3@latest
go get github.com/zishang520/socket.io/servers/socket/v3@latest
go get github.com/zishang520/socket.io/adapters/redis/v3@latest
go get github.com/zishang520/socket.io/clients/engine/v3@latest
go get github.com/zishang520/socket.io/clients/socket/v3@latest
```

After updating, clean up your dependencies:

```bash
go mod tidy
```

Your `go.mod` should contain entries similar to:

```go
require (
    github.com/zishang520/socket.io/v3 v3.0.0
    github.com/zishang520/socket.io/parsers/engine/v3 v3.0.0
    github.com/zishang520/socket.io/parsers/socket/v3 v3.0.0
    github.com/zishang520/socket.io/servers/engine/v3 v3.0.0
    github.com/zishang520/socket.io/servers/socket/v3 v3.0.0
    github.com/zishang520/socket.io/adapters/redis/v3 v3.0.0
    github.com/zishang520/socket.io/clients/engine/v3 v3.0.0
    github.com/zishang520/socket.io/clients/socket/v3 v3.0.0
)
```

## Import Path Updates

You must update all Socket.IO import paths throughout your application. Use the following reference table to update your imports:

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

> **Tip:** You can use `grep -r "github.com/zishang520" .` to find all old imports in your codebase, then use your editor's find-and-replace functionality to update them systematically.

## Breaking Changes

### Protocol Compatibility

Socket.IO v3 aligns with the Socket.IO v4+ protocol. Ensure your client-side Socket.IO library is updated to version 4.x or higher for compatibility.

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

## Testing Your Upgrade

After completing the upgrade, thoroughly test your application:

1. **Run your test suite:**
   ```bash
   go test ./...
   ```

2. **Test core functionality:**
   - Client connections and disconnections
   - Event emission and reception
   - Namespace and room operations
   - Redis adapter broadcasting (if applicable)

3. **Enable debug logging for troubleshooting:**
   ```bash
   DEBUG=socket.io:* go run main.go
   ```

4. **Verify client compatibility:**
   Ensure your frontend is using Socket.IO client v4.x or higher.

## Common Issues

### Import Resolution Errors

If you encounter import resolution errors after updating:

```bash
go mod tidy
go clean -modcache
go mod download
```

### Connection Protocol Mismatches

Upgrade your client-side Socket.IO library to v4.x:

```bash
npm install socket.io-client@^4.0.0
```

## Need Help?

If you encounter issues during the upgrade:

- Check the [GitHub Issues](https://github.com/zishang520/socket.io/issues) page
- Review the [Socket.IO Go Repository](https://github.com/zishang520/socket.io)
- Consult the [Socket.IO Protocol Documentation](https://socket.io/docs/v4/)

## Additional Notes

- **Backup First:** Always backup your codebase before starting the upgrade process
- **Go Version:** Ensure you're using Go 1.24 or higher
- **Staged Rollout:** Consider upgrading in stages, starting with non-critical components
- **Client Coordination:** Coordinate the upgrade with your frontend team to ensure client compatibility