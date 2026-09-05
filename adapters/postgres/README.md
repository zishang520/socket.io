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

    pgClient, err := postgres.NewPostgresClient(context.TODO(), pool)
    if err != nil {
        panic(err)
    }
    defer pgClient.Close()

    io := socket.NewServer(nil, nil)
    defer io.Close(nil)
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

    pgClient, err := postgres.NewPostgresClient(context.TODO(), pool)
    if err != nil {
        panic(err)
    }
    defer pgClient.Close()

    emitter := pgemitter.NewEmitter(pgClient, nil)
    emitter.Emit("hello", "world")
    emitter.To("room1").Emit("hello", "world")
}
```

## Configuration Options

### Adapter Options

```golang
opts := pgadapter.DefaultPostgresAdapterOptions()
opts.SetChannelPrefix("socket.io")
opts.SetTableName("socket_io_attachments")
opts.SetPayloadThreshold(8_000)
opts.SetCleanupInterval(30_000)
opts.SetHeartbeatInterval(5 * time.Second)
opts.SetHeartbeatTimeout(10_000)
opts.SetErrorHandler(func(err error) {
    log.Printf("PostgreSQL adapter: %v", err)
})

builder := &pgadapter.PostgresAdapterBuilder{
    Postgres: pgClient,
    Opts:     opts,
}
```

### Emitter Options

```golang
opts := pgemitter.DefaultEmitterOptions()
opts.SetChannelPrefix("socket.io")
opts.SetTableName("socket_io_attachments")
opts.SetPayloadThreshold(8_000)

emitter := pgemitter.NewEmitter(pgClient, opts)
```

`TableName` accepts unquoted, schema-qualified names such as `public.socket_io_attachments`.

## Architecture

The PostgreSQL adapter uses two mechanisms for inter-node communication:

1. **LISTEN/NOTIFY** — Lightweight pub/sub for messages under the payload threshold
2. **Attachment Table** — Stores large payloads or binary data that exceed the NOTIFY limit

Messages are serialized as JSON for direct NOTIFY, or MessagePack for attachment storage. This ensures compatibility with the Node.js `socket.io-postgres-adapter`, allowing mixed Go/Node.js deployments in the same cluster.

Finite PostgreSQL operations, including publish, attachment access, listener
connection attempts, LISTEN/UNLISTEN updates, and cleanup, use a 5-second I/O
deadline. The steady-state notification wait remains blocking until a message,
connection error, or lifecycle cancellation occurs.

Notifications are processed in arrival order within each namespace, using a
separate receive queue per namespace. An attachment query does not block the
shared listener or another namespace's delivery. Messages in the same namespace
still wait for earlier attachment queries, so slow queries can delay its heartbeats
and responses. This preserves ordering unlike Node's concurrent attachment reads.

Each `PostgresAdapterBuilder` must own a distinct `PostgresClient`, since the
client owns that builder's LISTEN connection and subscriptions. Multiple clients
and emitters may share the same underlying `*pgxpool.Pool`. The builder is the
exclusive owner of its client's listener; do not call `Listen` or `Unlisten`
directly on that client.

For clients used as listeners, leave `ConnConfig.OnNotification` nil and do not
install that callback from `BeforeConnect`: the adapter relies on pgx's default
notification buffer. Opening a listener rejects a statically configured callback
with `postgres.ErrPostgresOnNotificationUnsupported`. Clients used only by an
emitter may use a pool with a custom notification callback.

During shutdown, close the Socket.IO server (and therefore its namespace
adapters) first, then close the `PostgresClient`, and finally close the shared
pool. The adapter example above uses this order through `defer` calls.

### Database Schema

Create the attachment table before starting the adapter. Like the Node.js adapter,
the Go adapter does not execute schema migrations automatically:

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
