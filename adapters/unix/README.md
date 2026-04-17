# Socket.IO Unix Domain Socket Adapter

The package allows broadcasting packets between multiple Socket.IO servers using Unix Domain Sockets as the message broker.

This adapter is suitable for multi-process deployments on the same machine where low-latency IPC is desired without depending on external services like Redis or PostgreSQL.

## Table of Contents

- [Installation](#installation)
- [Usage](#usage)
  - [Adapter](#adapter)
  - [Emitter](#emitter)
- [How It Works](#how-it-works)
- [License](#license)

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/unix/v3
```

## Usage

### Adapter

```go
package main

import (
	"context"
	"net/http"

	sio "github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	unixadapter "github.com/zishang520/socket.io/adapters/unix/v3/adapter"
)

func main() {
	ctx := context.Background()
	socketPath := "/tmp/socket.io.sock"

	client := unix.NewUnixClient(ctx, socketPath)

	opts := unixadapter.DefaultUnixAdapterOptions()
	opts.SetKey("socket.io")

	io := sio.NewServer(nil, nil)
	io.SetAdapter(&unixadapter.UnixAdapterBuilder{
		Unix: client,
		Opts: opts,
	})

	io.On("connection", func(args ...any) {
		socket := args[0].(*sio.Socket)
		socket.On("message", func(args ...any) {
			io.Emit("message", args...)
		})
	})

	http.Handle("/socket.io/", io.ServeHandler(nil))
	http.ListenAndServe(":3000", nil)
}
```

### Emitter

The emitter allows you to send events to connected clients from any process, without running a full Socket.IO server:

```go
package main

import (
	"context"
	"fmt"

	"github.com/zishang520/socket.io/adapters/unix/v3"
	unixemitter "github.com/zishang520/socket.io/adapters/unix/v3/emitter"
)

func main() {
	ctx := context.Background()
	socketPath := "/tmp/socket.io.sock"

	client := unix.NewUnixClient(ctx, socketPath)

	opts := &unixemitter.EmitterOptions{}
	opts.SetKey("socket.io")
	opts.SetSocketPath(socketPath)

	emitter := unixemitter.NewEmitter(client, opts)

	// Emit to all clients
	if err := emitter.Emit("hello", "world"); err != nil {
		fmt.Printf("emit error: %v\n", err)
	}

	// Emit to specific room
	if err := emitter.To("room1").Emit("hello", "room"); err != nil {
		fmt.Printf("emit error: %v\n", err)
	}
}
```

## How It Works

Each Socket.IO server node creates a unique Unix Domain Socket listener file:

```
/tmp/socket.io.sock.{server-uid}
```

When a message needs to be broadcast, the adapter scans the socket directory for all peer listener files matching the base path pattern and sends the message to each peer via Unix datagram sockets.

**Message encoding:**
- JSON for non-binary messages
- MessagePack for binary messages

**Peer discovery:**
- File-system based: each node creates a `{base}.{uid}` socket file
- Peers are discovered by scanning the socket directory

## License

[MIT](LICENSE)
