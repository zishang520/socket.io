# socket.io-go-parser

[![GoDoc](https://pkg.go.dev/badge/github.com/zishang520/socket.io/parsers/socket/v3?utm_source=godoc)](https://pkg.go.dev/github.com/zishang520/socket.io/parsers/socket/v3)

## Overview

This is the Go parser for the Socket.IO protocol, responsible for encoding and decoding packets. It is shared by both [socket.io-client-go](https://github.com/zishang520/socket.io/clients/socket/v3) and [socket.io](https://github.com/zishang520/socket.io/servers/socket/v3).

### Compatibility Table

| Parser Version | Socket.IO Server Version  | Protocol Revision |
|----------------|---------------------------|-------------------|
| 3.x            | 3.x                       | 5                 |

## Features

- Full support for Socket.IO protocol v5
- Encoding and decoding of packets
- Binary data support
- Event-based decoding
- Extensible and thread-safe implementation

## Installation

To install the package, run:

```bash
go get github.com/zishang520/socket.io/parsers/socket/v3
```

## Example Usage

### Encoding and Decoding a Packet

```go
package main

import (
    "github.com/zishang520/socket.io/v3/pkg/utils"
    "github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

func main() {
    encoder := parser.NewEncoder()
    id := uint64(13)
    packet := &parser.Packet{
        Type: parser.EVENT,
        Data: []string{"test-packet"},
        Id:   &id,
    }
    encodedPackets := encoder.Encode(packet)
    utils.Log().Default("Encoded: %v", encodedPackets)

    for _, encodedPacket := range encodedPackets {
        decoder := parser.NewDecoder()
        decoder.On("decoded", func(decodedPackets ...any) {
            utils.Log().Default("Decoded: %v", decodedPackets[0])
            // decodedPackets[0].Type == parser.EVENT
            // decodedPackets[0].Data == []string{"test-packet"}
            // decodedPackets[0].Id == 13
        })

        decoder.Add(encodedPacket)
    }
}
```

### Encoding and Decoding a Packet with Binary Data

```go
package main

import (
    "github.com/zishang520/socket.io/v3/pkg/utils"
    "github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

func main() {
    encoder := parser.NewEncoder()
    attachments := uint64(0)
    packet := &parser.Packet{
        Type:        parser.BINARY_EVENT,
        Data:        []any{"test-packet", []byte{1, 2, 3, 4, 5}},
        Id:          utils.Ptr(uint64(13)),
        Attachments: &attachments,
    }
    encodedPackets := encoder.Encode(packet)
    utils.Log().Default("Encoded: %v", encodedPackets)

    for _, encodedPacket := range encodedPackets {
        decoder := parser.NewDecoder()
        decoder.On("decoded", func(decodedPackets ...any) {
            utils.Log().Default("Decoded: %v", decodedPackets[0])
            // decodedPackets[0].Type == parser.BINARY_EVENT
            // decodedPackets[0].Data == []any{"test-packet", []byte{1, 2, 3, 4, 5}}
            // decodedPackets[0].Id == 13
        })

        decoder.Add(encodedPacket)
    }
}
```

## API Reference

### Packet Structure

```go
type Packet struct {
    Type        PacketType   // Type of the packet (e.g., EVENT, BINARY_EVENT)
    Data        any          // Packet data
    Id          *uint64      // Packet ID (optional)
    Attachments *uint64      // Number of binary attachments (optional)
}
```

### Encoder Interface

```go
type Encoder interface {
    Encode(packet *Packet) []types.BufferInterface
}
```

### Decoder Interface

```go
type Decoder interface {
    types.EventEmitter
    Add(data any) error
    Destroy()
}
```

## Tests

Run the test suite with:

```bash
make test
```

## Development

To contribute to the project, follow these steps:

1. Fork the repository.
2. Create a feature branch: `git checkout -b feature/amazing-feature`.
3. Commit your changes: `git commit -m 'Add some amazing feature'`.
4. Push to the branch: `git push origin feature/amazing-feature`.
5. Open a Pull Request.

## Support

If you encounter any issues or have questions, please file them in the [issues section](https://github.com/zishang520/socket.io/issues).

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
