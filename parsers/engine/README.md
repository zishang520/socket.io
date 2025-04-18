# engine.io-go-parser

[![GoDoc](https://pkg.go.dev/badge/github.com/zishang520/socket.io/parsers/engine/v3?utm_source=godoc)](https://pkg.go.dev/github.com/zishang520/socket.io/parsers/engine/v3)

## Description

A Go implementation of the Engine.IO protocol parser. This package is used by both [engine.io-client-go](https://github.com/zishang520/socket.io/tree/v3/clients/engine) and [engine.io](https://github.com/zishang520/socket.io/tree/v3/servers/engine) for protocol encoding and decoding.

## Installation

```bash
go get github.com/zishang520/socket.io/parsers/engine/v3
```

## Features

- Packet encoding/decoding
- Payload encoding/decoding
- Binary data support
- Protocol v3 and v4 support
- UTF-8 encoding support

## How to use

### Basic Usage

```go
package main

import (
    "bytes"
    "fmt"

    "github.com/zishang520/socket.io/parsers/engine/v3/packet"
    "github.com/zishang520/socket.io/v3/pkg/types"
)

func main() {
    // Initialize parser
    parser := packet.Parserv4()

    // Encode a packet
    encodedData, err := parser.EncodePacket(&packet.Packet{
        Type: packet.MESSAGE,
        Data: bytes.NewBuffer([]byte("Hello World")),
    }, true)
    if err != nil {
        panic(err)
    }

    // Decode a packet
    decodedPacket, err := parser.DecodePacket(encodedData)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Decoded message: %s\n", decodedPacket.Data)
}
```

### Working with Payloads

```go
func handlePayload() {
    parser := packet.Parserv4()

    packets := []*packet.Packet{
        {
            Type: packet.MESSAGE,
            Data: bytes.NewBuffer([]byte("First message")),
        },
        {
            Type: packet.MESSAGE,
            Data: bytes.NewBuffer([]byte("Second message")),
        },
    }

    // Encode payload
    encoded, err := parser.EncodePayload(packets)
    if err != nil {
        panic(err)
    }

    // Decode payload
    decoded, err := parser.DecodePayload(encoded)
    if err != nil {
        panic(err)
    }
}
```

## API Reference

### Parser Interface

#### EncodePacket

```go
EncodePacket(packet *packet.Packet, supportsBinary bool) (types.BufferInterface, error)
```

- `packet`: The packet to encode
- `supportsBinary`: Enable binary support
- Returns: Encoded packet and error if any

#### DecodePacket

```go
DecodePacket(data types.BufferInterface) (*packet.Packet, error)
```

- `data`: The data to decode
- Returns: Decoded packet and error if any

#### EncodePayload

```go
EncodePayload(packets []*packet.Packet) (types.BufferInterface, error)
```

- `packets`: Array of packets to encode
- Returns: Encoded payload and error if any

#### DecodePayload

```go
DecodePayload(data types.BufferInterface) ([]*packet.Packet, error)
```

- `data`: The payload to decode
- Returns: Array of decoded packets and error if any

## Development

### Prerequisites

- Go 1.24.1 or higher
- Make

### Testing

Run the test suite:

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

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
