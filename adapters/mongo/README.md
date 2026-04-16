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

    mongoClient := mgclient.NewMongoClient(context.TODO(), collection)

    io := socket.NewServer(nil, nil)
    io.SetAdapter(&mgadapter.MongoAdapterBuilder{
        Mongo: mongoClient,
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

    mongoClient := mgclient.NewMongoClient(context.TODO(), collection)

    emitter := mgemitter.NewEmitter(mongoClient, nil)
    emitter.Emit("hello", "world")
    emitter.To("room1").Emit("hello", "world")
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
db.collection("socket.io-adapter-events").createIndex(
    { createdAt: 1 },
    { expireAfterSeconds: 3600 }
)
```

When using a TTL index, set the `AddCreatedAtField` option to `true`:
```golang
opts := &mgadapter.MongoAdapterOptions{}
opts.SetAddCreatedAtField(true)
```

## License

[MIT](LICENSE)
