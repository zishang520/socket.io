# Socket.IO Unix Domain Socket Adapter

This package connects Socket.IO server processes running on the same machine through Unix domain stream sockets. It is useful when low-latency local IPC is preferable to an external broker such as Redis or PostgreSQL.

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/unix/v3
```

## Adapter

```go
package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/zishang520/socket.io/adapters/unix/v3"
	unixadapter "github.com/zishang520/socket.io/adapters/unix/v3/adapter"
	sio "github.com/zishang520/socket.io/servers/socket/v3"
)

func main() {
	socketDir := filepath.Join(os.TempDir(), "my-service")
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		panic(err)
	}
	socketPath := filepath.Join(socketDir, "socket.io.sock")
	client, err := unix.NewUnixClient(context.Background(), socketPath, nil)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	server := sio.NewServer(nil, nil)
	server.SetAdapter(&unixadapter.UnixAdapterBuilder{Unix: client})

	http.Handle("/socket.io/", server.ServeHandler(nil))
	if err := http.ListenAndServe(":3000", nil); err != nil {
		panic(err)
	}
}
```

Pass the same base socket path to every process in the cluster. It must name a socket base and must not end with a path separator. The client owns the single listener shared by all namespaces; keep it open for the lifetime of the Socket.IO server and close it during process shutdown.

Use a `UnixClient` with at most one `UnixAdapterBuilder`. Once the builder is installed, do not call `Listen` or `ReadMessage` directly on that client: `Listen` may reserve an incompatible listener path, while `ReadMessage` would compete with the builder for cluster frames.

## Emitter

The emitter publishes through the path already configured on `UnixClient`, so there is no second socket-path or codec option to keep in sync.

```go
package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/zishang520/socket.io/adapters/unix/v3"
	unixemitter "github.com/zishang520/socket.io/adapters/unix/v3/emitter"
)

func main() {
	socketDir := filepath.Join(os.TempDir(), "my-service")
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		panic(err)
	}
	client, err := unix.NewUnixClient(
		context.Background(),
		filepath.Join(socketDir, "socket.io.sock"),
		nil,
	)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	emitter := unixemitter.NewEmitter(client)
	if err := emitter.To("room1").Emit("hello", "world"); err != nil {
		panic(err)
	}

	if err := emitter.Of("/admin").Emit("refresh"); err != nil {
		panic(err)
	}
}
```

## Transport and lifecycle

Each server creates a listener named `{base-socket-path}.{listener-id}`. The listener ID is an opaque, non-empty suffix without `.`; the builder generates it automatically. Publishing scans the base path's directory and sends to the matching peer sockets without matching listeners that belong to a longer base name. Failed pooled connections are removed, but a crashed process can leave a stale matching socket file that is rediscovered by later broadcasts. During a coordinated restart, remove such a file only after verifying that no live process owns it.

All platforms use `SOCK_STREAM`. Each payload is framed with a 4-byte big-endian length followed by up to 10 MiB of encoded data. Plain messages use JSON and messages containing binary values use MessagePack.

Peer connection attempts and complete frame writes are bounded to 5 seconds by default, so a process that stops reading cannot block delivery to every later peer indefinitely. Configure these transport-level limits once on the client when needed:

```go
client, err := unix.NewUnixClient(ctx, socketPath, &unix.UnixClientOptions{
	DialTimeout:  time.Second,
	WriteTimeout: 2 * time.Second,
})
```

Non-positive timeout values use the bounded defaults. A timed-out peer connection is discarded; broadcasts report that peer through the client's `"error"` event and continue with the remaining listeners.

Canceling the context passed to `NewUnixClient` has the same effect as calling `Close`: the listener, accepted connections, and pooled outgoing connections are closed. Background transport failures are emitted through the client's `"error"` event. When no application error listener is registered, the client logs a warning instead of silently dropping the failure.

## Security

Unix sockets authenticate through filesystem access, not through the Socket.IO adapter protocol. On Unix-like systems, provision the base-path directory before startup and verify that it is owned by the service account with mode `0700`; `os.MkdirAll(path, 0700)` does not tighten permissions on an existing directory. Listener files are created with mode `0600`. On Windows, provision the directory with an appropriate ACL and verify it before startup. Processes that can connect to the listener are trusted cluster members and can inject cluster operations.

The encoded base path plus the listener-ID suffix must also fit the platform's Unix socket path limit.

## License

[MIT](LICENSE)
