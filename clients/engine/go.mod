module github.com/zishang520/socket.io/clients/engine/v3

go 1.26.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/quic-go/quic-go v0.60.0
	github.com/quic-go/webtransport-go v0.11.1
	github.com/zishang520/socket.io/parsers/engine/v3 v3.0.4
	github.com/zishang520/socket.io/servers/engine/v3 v3.0.4
	github.com/zishang520/socket.io/v3 v3.0.4
)

require (
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/dunglas/httpsfv v1.1.0 // indirect
	github.com/gookit/color v1.6.1 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	resty.dev/v3 v3.0.0-rc.3 // indirect
)

replace (
	github.com/zishang520/socket.io/parsers/engine/v3 => ../../parsers/engine
	github.com/zishang520/socket.io/servers/engine/v3 => ../../servers/engine
	github.com/zishang520/socket.io/v3 => ../../
)
