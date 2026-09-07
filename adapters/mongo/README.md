# socket.io-go-mongo

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/adapters/mongo/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/adapters/mongo/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/adapters/mongo/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/adapters/mongo/v3)

## Description

A MongoDB adapter for Socket.IO server in Go, allowing to scale Socket.IO applications across multiple processes or servers using MongoDB Change Streams.

This adapter is compatible with the Node.js [@socket.io/mongo-adapter](https://github.com/socketio/socket.io-mongo-adapter) package, enabling mixed Go and Node.js deployments.

**Note:** MongoDB must be configured as a Replica Set or Sharded Cluster to support Change Streams.

## Installation

```bash
go get github.com/zishang520/socket.io/adapters/mongo/v3
```

## Features

- Multiple servers support via MongoDB Change Streams
- Compatible with Node.js `@socket.io/mongo-adapter` for mixed deployments
- Heartbeat-based node failure detection
- Real-time communication between processes
- Support for both capped collections and TTL indexes

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

    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/mongo/options"
    mgadapter "github.com/zishang520/socket.io/adapters/mongo/v3/adapter"
    mgclient "github.com/zishang520/socket.io/adapters/mongo/v3"
    "github.com/zishang520/socket.io/servers/socket/v3"
)

func main() {
    client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017/?replicaSet=rs0"))
    if err != nil {
        panic(err)
    }
    defer client.Disconnect(context.Background())

    collection := client.Database("mydb").Collection("socket.io-adapter-events")

    mongoClient, err := mgclient.NewMongoClient(context.TODO(), collection)
    if err != nil {
        panic(err)
    }

    mongoClient.On("error", func(args ...any) {
        fmt.Println("MongoDB error:", args)
    })

    io := socket.NewServer(nil, nil)
    defer io.Close(nil)
    io.SetAdapter(&mgadapter.MongoAdapterBuilder{
        Mongo: mongoClient,
    })

    io.On("connection", func(args ...any) {
        s := args[0].(*socket.Socket)
        fmt.Printf("connect %s\n", s.Id())
    })

    io.Listen(3000, nil)

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

Background publish and Change Stream failures are emitted through the MongoDB
client's `"error"` event. A warning is logged when no application error handler
is registered.

### Emitter

```golang
package main

import (
    "context"

    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/mongo/options"
    mgclient "github.com/zishang520/socket.io/adapters/mongo/v3"
    mgemitter "github.com/zishang520/socket.io/adapters/mongo/v3/emitter"
)

func main() {
    client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017/?replicaSet=rs0"))
    if err != nil {
        panic(err)
    }
    defer client.Disconnect(context.Background())

    collection := client.Database("mydb").Collection("socket.io-adapter-events")

    mongoClient, err := mgclient.NewMongoClient(context.TODO(), collection)
    if err != nil {
        panic(err)
    }

    emitter := mgemitter.NewEmitter(mongoClient, nil)
    if err := emitter.Emit("hello", "world"); err != nil {
        panic(err)
    }
    if err := emitter.To("room1").Emit("hello", "world"); err != nil {
        panic(err)
    }
}
```

## How it works

The adapter uses MongoDB Change Streams to detect new documents inserted into a shared collection. When a Socket.IO server needs to broadcast a message or perform a cross-node operation, it inserts a document into the MongoDB collection. All other servers watching the same collection via Change Streams will receive the notification and process the event accordingly.

### Capped Collection vs TTL Index

You can use either a **capped collection** or a **TTL index** for automatic cleanup:

#### Capped Collection (recommended for most cases)
```javascript
db.createCollection("socket.io-adapter-events", { capped: true, size: 1e6 })
```

#### TTL Index
```javascript
db.getCollection("socket.io-adapter-events").createIndex(
    { createdAt: 1 },
    { expireAfterSeconds: 3600 }
)
```

When using a TTL index, set `AddCreatedAtField` to `true` on every adapter and
emitter that writes to the collection. Documents without `createdAt` will not
expire through this index.

In the adapter example above, pass the configured options to `MongoAdapterBuilder`:

```golang
adapterOpts := mgadapter.DefaultMongoAdapterOptions()
adapterOpts.SetAddCreatedAtField(true)

io.SetAdapter(&mgadapter.MongoAdapterBuilder{
    Mongo: mongoClient,
    Opts:  adapterOpts,
})
```

In the emitter example above, pass the emitter options to `NewEmitter`:

```golang
emitterOpts := mgemitter.DefaultEmitterOptions()
emitterOpts.SetAddCreatedAtField(true)

emitter := mgemitter.NewEmitter(mongoClient, emitterOpts)
if err := emitter.Emit("hello", "world"); err != nil {
    panic(err)
}
```

## Testing

Set `SOCKET_IO_MONGO_TEST_URI` to a MongoDB test instance to include the database
integration tests when running `go test -race ./...` from this module. Each test
creates and removes its own database. Without this variable, database integration
tests are skipped.

## License

[MIT](LICENSE)
