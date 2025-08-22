# Upgrading from v1/v2 to v3

This guide outlines the steps to upgrade your Go project using the `github.com/zishang520/socket.io` library from version 1.x or 2.x to version 3.x. The upgrade aligns with the Socket.IO v4+ protocol, introducing improved performance, updated APIs, and new package structures. Follow these steps carefully to ensure a smooth transition.

## Introduction

Version 3 of `github.com/zishang520/socket.io` consolidates dependencies and updates import paths to a more modular structure. This upgrade involves updating dependencies, replacing deprecated import paths, and addressing breaking changes, such as changes in the Redis adapter and event handling. This document provides detailed instructions, code examples, and tips to help you upgrade successfully.

## Prerequisites

Before starting, ensure you have:
- Go 1.18 or higher installed.
- A working project using `github.com/zishang520/socket.io` v1.x or v2.x.
- Access to your project’s `go.mod` file.
- A backup of your codebase (**Tip**: Use version control or create a manual backup to avoid data loss).
- The client-side Socket.IO library updated to a version compatible with Socket.IO v4+ (e.g., `socket.io-client` 4.x).

## Upgrade Steps

Follow these steps in order to upgrade your project to v3:

### 1. Update Dependencies in `go.mod`

Update your `go.mod` file to use the latest v3 versions of the Socket.IO library and its dependencies. Run the following commands:

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

Run `go mod tidy` to resolve dependencies:

```bash
go mod tidy
```

**Tip**: Verify the versions in `go.mod` match the v3 releases. Example `go.mod` snippet:

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

### 2. Replace Deprecated Import Paths

Replace all outdated import paths with their v3 equivalents. Below is a comprehensive list of changes for each package. Search your codebase for old import paths starting with `github.com/zishang520/...` and update them as follows.

#### 2.1 `engine.io-go-parser`

| Old Import Path (v1) | New Import Path (v3) |
|----------------------|----------------------|
| `github.com/zishang520/engine.io-go-parser/packet` | `github.com/zishang520/socket.io/parsers/engine/v3/packet` |
| `github.com/zishang520/engine.io-go-parser/parser` | `github.com/zishang520/socket.io/parsers/engine/v3/parser` |
| `github.com/zishang520/engine.io-go-parser/types` | `github.com/zishang520/socket.io/v3/pkg/types` |
| `github.com/zishang520/engine.io-go-parser/utils` | `github.com/zishang520/socket.io/v3/pkg/utils` |

#### 2.2 `socket.io-go-parser`

| Old Import Path (v1/v2) | New Import Path (v3) |
|-------------------------|----------------------|
| `github.com/zishang520/socket.io-go-parser/parser` | `github.com/zishang520/socket.io/parsers/socket/v3/parser` |
| `github.com/zishang520/socket.io-go-parser/v2/parser` | `github.com/zishang520/socket.io/parsers/socket/v3/parser` |

#### 2.3 `engine.io-server`

| Old Import Path (v1) | New Import Path (v3) |
|----------------------|----------------------|
| `github.com/zishang520/engine.io/config` | `github.com/zishang520/socket.io/servers/engine/v3/config` |
| `github.com/zishang520/engine.io/engine` | `github.com/zishang520/socket.io/servers/engine/v3` |
| `github.com/zishang520/engine.io/errors` | `github.com/zishang520/socket.io/servers/engine/v3/errors` |
| `github.com/zishang520/engine.io/events` | `github.com/zishang520/socket.io/v3/pkg/events` |
| `github.com/zishang520/engine.io/log` | `github.com/zishang520/socket.io/v3/pkg/log` |
| `github.com/zishang520/engine.io/transports` | `github.com/zishang520/socket.io/servers/engine/v3/transports` |
| `github.com/zishang520/engine.io/types` | `github.com/zishang520/socket.io/v3/pkg/types` |
| `github.com/zishang520/engine.io/utils` | `github.com/zishang520/socket.io/v3/pkg/utils` |

| Old Import Path (v2) | New Import Path (v3) |
|----------------------|----------------------|
| `github.com/zishang520/engine.io/v2/config` | `github.com/zishang520/socket.io/servers/engine/v3/config` |
| `github.com/zishang520/engine.io/v2/engine` | `github.com/zishang520/socket.io/servers/engine/v3` |
| `github.com/zishang520/engine.io/v2/errors` | `github.com/zishang520/socket.io/servers/engine/v3/errors` |
| `github.com/zishang520/engine.io/v2/events` | `github.com/zishang520/socket.io/v3/pkg/events` |
| `github.com/zishang520/engine.io/v2/log` | `github.com/zishang520/socket.io/v3/pkg/log` |
| `github.com/zishang520/engine.io/v2/transports` | `github.com/zishang520/socket.io/servers/engine/v3/transports` |
| `github.com/zishang520/engine.io/v2/types` | `github.com/zishang520/socket.io/v3/pkg/types` |
| `github.com/zishang520/engine.io/v2/utils` | `github.com/zishang520/socket.io/v3/pkg/utils` |
| `github.com/zishang520/engine.io/v2/webtransport` | `github.com/zishang520/socket.io/v3/pkg/webtransport` |

#### 2.4 `socket.io-server`

| Old Import Path (v1) | New Import Path (v3) |
|----------------------|----------------------|
| `github.com/zishang520/socket.io/socket` | `github.com/zishang520/socket.io/servers/socket/v3` |

| Old Import Path (v2) | New Import Path (v3) |
|----------------------|----------------------|
| `github.com/zishang520/socket.io/v2/socket` | `github.com/zishang520/socket.io/servers/socket/v3` |
| `github.com/zishang520/socket.io/v2/adapter` | `github.com/zishang520/socket.io/adapters/adapter/v3` |

#### 2.5 `socket.io-go-redis`

| Old Import Path (v1) | New Import Path (v3) |
|----------------------|----------------------|
| `github.com/zishang520/socket.io-go-redis/adapter` | `github.com/zishang520/socket.io/adapters/redis/v3/adapter` |
| `github.com/zishang520/socket.io-go-redis/emitter` | `github.com/zishang520/socket.io/adapters/redis/v3/emitter` |
| `github.com/zishang520/socket.io-go-redis/types` | `github.com/zishang520/socket.io/adapters/redis/v3` |

**Note**: If you use `github.com/zishang520/socket.io-go-redis/types.String`, replace it with `github.com/zishang520/socket.io/v3/pkg/types.Atomic`. Example:

```go
// Old (v1)
import "github.com/zishang520/socket.io-go-redis/types"
var s types.String

// New (v3)
import "github.com/zishang520/socket.io/v3/pkg/types"
var s types.Atomic[string]
```

#### 2.6 `engine.io-client`

| Old Import Path (v1) | New Import Path (v3) |
|----------------------|----------------------|
| `github.com/zishang520/engine.io-client-go/engine` | `github.com/zishang520/socket.io/clients/engine/v3` |
| `github.com/zishang520/engine.io-client-go/request` | `github.com/zishang520/socket.io/v3/pkg/request` |
| `github.com/zishang520/engine.io-client-go/transports` | `github.com/zishang520/socket.io/clients/engine/v3/transports` |

#### 2.7 `socket.io-client`

| Old Import Path (v1) | New Import Path (v3) |
|----------------------|----------------------|
| `github.com/zishang520/socket.io-client-go/socket` | `github.com/zishang520/socket.io/clients/socket/v3` |
| `github.com/zishang520/socket.io-client-go/utils` | `github.com/zishang520/socket.io/v3/pkg/utils` |

**Tip**: Use a global search-and-replace tool (e.g., `grep -r "github.com/zishang520" .`) to locate all outdated imports in your codebase. After replacing, run `go mod tidy` again to ensure consistency.

### 3. Update Redis Adapter (if applicable)

If you use the Redis adapter, update the adapter configuration and replace `types.String` with `types.Atomic[string]`:

```go
// Old (v1)
import "github.com/zishang520/socket.io-go-redis/adapter"
var redisAdapter adapter.Adapter
var s types.String

// New (v3)
import "github.com/zishang520/socket.io/adapters/redis/v3/adapter"
import "github.com/zishang520/socket.io/v3/pkg/types"
var redisAdapter adapter.Adapter
var s types.Atomic[string]
```

**Note**: Ensure your Redis client library is compatible with the v3 adapter. Test Redis-specific functionality, such as broadcasting across nodes.

### 4. Test Your Application

After applying the changes, test your application thoroughly:

1. Run your test suite:
   ```bash
   go test ./...
   ```
2. Verify key functionality:
   - Client connections and disconnections.
   - Event emission and reception.
   - Namespace and room handling.
   - Redis adapter broadcasting (if applicable).
3. Enable debug logging to identify issues:
   ```bash
   DEBUG=socket.io:* go run .
   ```

**Tip**: Test with a client using Socket.IO v4+ to ensure protocol compatibility.

## Breaking Changes

- **Dependency Consolidation**: All packages are now under `github.com/zishang520/socket.io/...`, with versioned submodules (e.g., `v3`).
- **Import Paths**: All v1/v2 import paths are deprecated. Use the new paths listed above.
- **Redis Adapter**: `types.String` is replaced by `types.Atomic[string]`. Update all type declarations accordingly.
- **Event Handling**: The `On` and `Emit` methods use a consistent `...interface{}` signature, which may require updating type assertions.
- **Protocol**: v3 aligns with Socket.IO v4+, affecting packet encoding/decoding. Use `github.com/zishang520/socket.io/parsers/socket/v3/parser`.

## Troubleshooting

- **Connection Errors**: Ensure the client-side Socket.IO library is updated to v4+. Mismatched protocols cause connection failures.
- **Import Errors**: Run `go mod tidy` after updating imports to resolve missing dependencies.
- **Redis Issues**: Verify Redis server compatibility and test broadcasting functionality.
- **Debugging**: Use `DEBUG=socket.io:*` for detailed logs to pinpoint issues.

## Additional Resources

- [Socket.IO Go Repository](https://github.com/zishang520/socket.io)
- [Socket.IO Protocol Documentation](https://socket.io/docs/v4/)
- [Engine.IO Go Repository](https://github.com/zishang520/engine.io)

If you encounter issues, open a ticket on the [GitHub issues page](https://github.com/zishang520/socket.io/issues).