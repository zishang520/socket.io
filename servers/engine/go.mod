module github.com/zishang520/socket.io/servers/engine/v3

go 1.24.1

require (
	github.com/andybalholm/brotli v1.2.0
	github.com/gorilla/websocket v1.5.3
	github.com/klauspost/compress v1.18.0
	github.com/zishang520/socket.io/parsers/engine/v3 v3.0.0-rc.4
	github.com/zishang520/socket.io/v3 v3.0.0-rc.4
	github.com/zishang520/webtransport-go v0.9.1
)

require (
	github.com/gookit/color v1.5.4 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/quic-go v0.54.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	go.uber.org/mock v0.6.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
)

replace (
	github.com/zishang520/socket.io/parsers/engine/v3 => ../../parsers/engine
	github.com/zishang520/socket.io/v3 => ../../
)
