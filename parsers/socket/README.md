# socket.io-go-parser

[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/parsers/socket/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/parsers/socket/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/parsers/socket/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/parsers/socket/v3)

## Overview

This is the Go parser for the Socket.IO protocol, responsible for encoding and decoding packets. It is shared by both [socket.io-client-go](https://github.com/zishang520/socket.io/tree/v3/clients/socket) and [socket.io](https://github.com/zishang520/socket.io/tree/v3/servers/socket).

### Compatibility Table

| Parser Version | Socket.IO Server Version  | Protocol Revision |
|----------------|---------------------------|-------------------|
| 3.x            | 3.x                       | 5                 |

## Features

- Full support for Socket.IO protocol v5
- Encoding and decoding of packets
- Binary data support
- Event-based decoding
- Extensible parser interfaces with ordered, stateful decoding

## Installation

To install the package, run:

```bash
go get github.com/zishang520/socket.io/parsers/socket/v3
```

## Example Usage

### Encoding and Decoding a Packet

Use EVENT or ACK for application packets. The encoder detects binary values and produces the header followed by its attachments.

```go
package main

import (
    "fmt"
    "github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

func main() {
    encoder := parser.NewEncoder()
    packet := &parser.Packet{
        Type: parser.EVENT,
        Nsp: "/",
        Data: []any{"upload", []byte{1, 2, 3}, "text"},
    }
    buffers, err := encoder.Encode(packet)
    if err != nil {
        panic(err)
    }

    decoder := parser.NewDecoder()
    defer decoder.Destroy()
    if err := decoder.On("decoded", func(args ...any) {
        decoded := args[0].(*parser.Packet)
        fmt.Println(decoded.Type, decoded.Data) // EVENT, with the reconstructed binary buffer
    }); err != nil {
        panic(err)
    }
    for _, buffer := range buffers {
        if err := decoder.Add(buffer); err != nil {
            panic(err)
        }
    }
}
```

For a text-only event, use `[]any{"message", "hello"}` as Data. Keep one decoder for the entire packet stream and feed buffers in order; do not call Add concurrently.

Encoding leaves the input Packet and containers unchanged. Reader values are consumed, and binary readers implementing io.Closer are closed. Encoding failure returns nil buffers and an error, so callers must not send a partial packet.

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
    Encode(packet *Packet) ([]types.BufferInterface, error)
}
```

`DeconstructPacket(packet)` returns `(*Packet, []types.BufferInterface, error)` and changes the packet only on success. `ReconstructPacket` accepts both its typed placeholders and placeholders decoded from JSON.

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
