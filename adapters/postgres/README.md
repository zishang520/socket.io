# socket.io-go-postgres

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/adapters/postgres/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/adapters/postgres/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/adapters/postgres/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/adapters/postgres/v3)

## Description

A PostgreSQL adapter for Socket.IO server in Go, allowing to scale Socket.IO applications across multiple processes or servers using PostgreSQL's `LISTEN`/`NOTIFY` mechanism.

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/postgres/v3
```

## Features

- Multiple servers support via PostgreSQL LISTEN/NOTIFY
- Automatic large payload handling via attachment table
- Heartbeat-based node failure detection
- Real-time communication between processes
- Custom PostgreSQL configuration

## How to use

### Adapter

```golang
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/zishang520/socket.io/adapters/postgres/v3"
    pgadapter "github.com/zishang520/socket.io/adapters/postgres/v3/adapter"
    "github.com/zishang520/socket.io/servers/socket/v3"
)

func main() {
    pool, err := pgxpool.New(context.Background(), "postgres://user:password@localhost:5432/mydb")
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    pgClient := postgres.NewPostgresClient(context.TODO(), pool)

    io := socket.NewServer(nil, nil)
    io.SetAdapter(&pgadapter.PostgresAdapterBuilder{
        Postgres: pgClient,
    })

    io.On("connection", func(args ...any) {
        s := args[0].(*socket.Socket)
        fmt.Printf("connect %s\n", s.Id())
    })

    exit := make(chan struct{})
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sig
        close(exit)
    }()
    <-exit
}
```

### Emitter

```golang
package main

import (
    "context"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/zishang520/socket.io/adapters/postgres/v3"
    pgemitter "github.com/zishang520/socket.io/adapters/postgres/v3/emitter"
)

func main() {
    pool, err := pgxpool.New(context.Background(), "postgres://user:password@localhost:5432/mydb")
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    pgClient := postgres.NewPostgresClient(context.TODO(), pool)

    emitter := pgemitter.NewEmitter(pgClient, nil)
    emitter.Emit("hello", "world")
    emitter.To("room1").Emit("hello", "world")
}
```

## Configuration Options

### Adapter Options

```golang
type PostgresAdapterOptions struct {
    Key               string        // PostgreSQL channel prefix (default: "socket.io")
    TableName         string        // Attachment storage table name (default: "socket_io_attachments")
    PayloadThreshold  int           // Byte threshold for attachment storage (default: 8000)
    CleanupInterval   int64         // Cleanup interval in milliseconds (default: 30000)
    HeartbeatInterval time.Duration // Interval between heartbeats (default: 5000ms)
    HeartbeatTimeout  int64         // Heartbeat response timeout (default: 10000)
    ErrorHandler      func(error)   // Custom error handler callback
}
```

### Emitter Options

```golang
type EmitterOptions struct {
    Key              string // PostgreSQL channel prefix (default: "socket.io")
    TableName        string // Attachment storage table name (default: "socket_io_attachments")
    PayloadThreshold int    // Byte threshold for attachment storage (default: 8000)
}
```

## Architecture

The PostgreSQL adapter uses two mechanisms for inter-node communication:

1. **LISTEN/NOTIFY** — Lightweight pub/sub for messages under the payload threshold
2. **Attachment Table** — Stores large payloads or binary data that exceed the NOTIFY limit

Messages are serialized as JSON for direct NOTIFY, or MessagePack for attachment storage. This ensures compatibility with the Node.js `socket.io-postgres-adapter`, allowing mixed Go/Node.js deployments in the same cluster.

### Database Schema

The adapter automatically creates the attachment table on startup:

```sql
CREATE TABLE IF NOT EXISTS socket_io_attachments (
    id bigserial UNIQUE,
    created_at timestamptz DEFAULT NOW(),
    payload bytea
);
```

## Mixed Deployment

This Go adapter is wire-compatible with the Node.js [`socket.io-postgres-adapter`](https://github.com/socketio/socket.io-postgres-adapter) and [`socket.io-postgres-emitter`](https://github.com/socketio/socket.io-postgres-emitter). You can mix Go and Node.js servers in the same cluster, as long as:

- Both use the same channel prefix (default: `socket.io`)
- Both use the same attachment table name (default: `socket_io_attachments`)
- Both use the same namespace names

## Testing

Run the test suite with:

```bash
make test
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Support

If you encounter any issues or have questions, please file them in the [issues section](https://github.com/zishang520/socket.io/issues).

## License

This project is licensed under the MIT License - see the LICENSE file for details.
