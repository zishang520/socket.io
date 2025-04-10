
# engine.io-go-parser

[![Go](https://github.com/zishang520/socket.io/parsers/engine/v3/actions/workflows/go.yml/badge.svg)](https://github.com/zishang520/socket.io/parsers/engine/v3/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/zishang520/socket.io/parsers/engine/v3?utm_source=godoc)](https://pkg.go.dev/github.com/zishang520/socket.io/parsers/engine/v3)

This is the golang parser for the engine.io protocol encoding,
shared by both
[engine.io-client-go(not ready)](https://github.com/zishang520/socket.io/clients/engine/v3) and
[engine.io](https://github.com/zishang520/engine.io).

## How to use

### Standalone

The parser can encode/decode packets, payloads, and payloads as binary
with the following methods: `Parser.EncodePacket`, `Parser.DecodePacket`, `Parser.EncodePayload`,
`Parser.DecodePayload`.

Example:

```go
import (
    "bytes"
    "io"
    "strings"

    "github.com/zishang520/socket.io/parsers/engine/v3/packet"
    "github.com/zishang520/socket.io/v3/pkg/types"
)

func main() {
    p := &packet.Parserv4()

    data, _ := p.EncodePacket(&packet.Packet{
        Type:    packet.MESSAGE,
        Data:    bytes.NewBuffer([]byte{1, 2, 3, 4}),
        Options: nil,
    }, true)
    decodedData, _ := p.DecodePacket(data)
}
```

## API

### Parser interface

- `EncodePacket`
    - Encodes a packet.
    - **Parameters**
      - `*packet.Packet`: the packet to encode.
      - `bool`: binary support.
      - `bool`: utf8 encode, v3 only.
- `DecodePacket`
    - **Parameters**
      - `types.BufferInterface`: the packet to decode.
      - `bool`: utf8 encode, v3 only.

- `EncodePayload`
    - Encodes multiple messages (payload).
    - If any contents are binary, they will be encoded as base64 strings. Base64
      encoded strings are marked with a b before the length specifier
    - **Parameters**
      - `[]*packet.Packet`: an array of packets
      - `bool`: binary support, v3 only.
- `DecodePayload`
    - Decodes data when a payload is maybe expected. Possible binary contents are
      decoded from their base64 representation.
    - **Parameters**
      - `types.BufferInterface`: the payload

## Tests

Standalone tests can be run with `make test` which will run the golang tests.

You can run the tests locally using the following command:

```
make test
```

## Support

[issues](https://github.com/zishang520/socket.io/parsers/engine/v3/issues)

## Development

To contribute patches, run tests or benchmarks, make sure to clone the
repository:

```bash
git clone git://github.com/zishang520/socket.io/parsers/engine/v3.git
```

Then:

```bash
cd engine.io-go-parser
make test
```

See the `Tests` section above for how to run tests before submitting any patches.

## License

MIT
