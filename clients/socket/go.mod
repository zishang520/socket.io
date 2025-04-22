module github.com/zishang520/socket.io/clients/socket/v3

go 1.24.1

require (
	github.com/zishang520/socket.io/clients/engine/v3 v3.0.0-alpha.0
	github.com/zishang520/socket.io/parsers/engine/v3 v3.0.0-alpha.0
	github.com/zishang520/socket.io/parsers/socket/v3 v3.0.0-alpha.0
	github.com/zishang520/socket.io/servers/engine/v3 v3.0.0-alpha.0
	github.com/zishang520/socket.io/servers/socket/v3 v3.0.0-alpha.0
	github.com/zishang520/socket.io/v3 v3.0.0-alpha.0
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/google/pprof v0.0.0-20230821062121-407c9e7a662f // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/onsi/ginkgo/v2 v2.12.0 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/quic-go v0.51.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20210125001918-ca9a967f8778 // indirect
	github.com/zishang520/webtransport-go v0.8.7 // indirect
	go.uber.org/mock v0.5.1 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/mod v0.18.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.22.0 // indirect
	resty.dev/v3 v3.0.0-beta.2 // indirect
)

replace (
	github.com/zishang520/socket.io/clients/engine/v3 => ../../clients/engine
	github.com/zishang520/socket.io/parsers/engine/v3 => ../../parsers/engine
	github.com/zishang520/socket.io/parsers/socket/v3 => ../../parsers/socket
	github.com/zishang520/socket.io/servers/engine/v3 => ../../servers/engine
	github.com/zishang520/socket.io/servers/socket/v3 => ../../servers/socket
	github.com/zishang520/socket.io/v3 => ../../
)
