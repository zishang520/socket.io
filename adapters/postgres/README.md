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

## How it works

This adapter uses PostgreSQL's built-in `LISTEN`/`NOTIFY` mechanism for pub/sub messaging between Socket.IO server instances. When a message payload exceeds the 8000-byte `NOTIFY` limit, the adapter stores the payload in an attachment table and sends only the attachment reference via `NOTIFY`.

### SQL Schema

The adapter automatically creates the following table if it doesn't exist:

```sql
CREATE TABLE IF NOT EXISTS socket_io_attachments (
    id          bigserial UNIQUE,
    created_at  timestamptz DEFAULT NOW(),
    payload     bytea
);
```

## License

[MIT](LICENSE)
