
# socket.io-go-parser

[![Build Status](https://github.com/zishang520/socket.io/parsers/socket/v3/workflows/Go/badge.svg?branch=main)](https://github.com/zishang520/socket.io/parsers/socket/v3/actions)
[![GoDoc](https://pkg.go.dev/badge/github.com/zishang520/socket.io/parsers/socket/v3?utm_source=godoc)](https://pkg.go.dev/github.com/zishang520/socket.io/parsers/socket/v3)

This is the golang parser for the socket.io protocol encoding,
shared by both
[socket.io-client-go(not ready)](https://github.com/zishang520/socket.io/clients/socket/v3) and
[socket.io](https://github.com/zishang520/socket.io).

Compatibility table:

| Parser version | Socket.IO server version | Protocol revision |
|----------------| ------------------------ | ----------------- |
| 1.x            | 3.x                      | 5                 |


## Parser API

  socket.io-parser is the reference implementation of socket.io-protocol. Read
  the full API here:
  [socket.io-protocol](https://github.com/socketio/socket.io-protocol).

## Example Usage

### Encoding and decoding a packet

```go
package main

import (
  "github.com/zishang520/socket.io/servers/engine/v3/utils"
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
  utils.Log().Default("encode %v", encodedPackets)

  for _, encodedPacket := range encodedPackets {
    decoder := parser.NewDecoder()
    decoder.On("decoded", func(decodedPackets ...any) {
      utils.Log().Default("decode %v", decodedPackets[0])
      // decodedPackets[0].Type == parser.EVENT
      // decodedPackets[0].Data == []string{"test-packet"}
      // decodedPackets[0].Id == 13
    })

    decoder.Add(encodedPacket)
  }
}

```

### Encoding and decoding a packet with binary data

```go
package main

import (
  "github.com/zishang520/socket.io/servers/engine/v3/utils"
  "github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

func main() {
  encoder := parser.NewEncoder()
  id := uint64(13)
  attachments := uint64(0)
  packet := &parser.Packet{
    Type:        parser.BINARY_EVENT,
    Data:        []any{"test-packet", []byte{1, 2, 3, 4, 5}},
    Id:          &id,
    Attachments: &attachments,
  }
  encodedPackets := encoder.Encode(packet)
  utils.Log().Default("encode %v", encodedPackets)

  for _, encodedPacket := range encodedPackets {
    decoder := parser.NewDecoder()
    decoder.On("decoded", func(decodedPackets ...any) {
      utils.Log().Default("decode %v", decodedPackets[0])
      // decodedPackets[0].Type == parser.BINARY_EVENT
      // decodedPackets[0].Data == []any{"test-packet", []byte{1, 2, 3, 4, 5}}
      // decodedPackets[0].Id == 13
    })

    decoder.Add(encodedPacket)
  }
}
```
See the test suite for more examples of how socket.io-parser is used.

## Tests

Standalone tests can be run with `make test` which will run the golang tests.

You can run the tests locally using the following command:

```
make test
```

## Support

[issues](https://github.com/zishang520/socket.io/parsers/socket/v3/issues)

## Development

To contribute patches, run tests or benchmarks, make sure to clone the
repository:

```bash
git clone git://github.com/zishang520/socket.io/parsers/socket/v3.git
```

Then:

```bash
cd socket.io-go-parser
make test
```

See the `Tests` section above for how to run tests before submitting any patches.

## License

MIT
