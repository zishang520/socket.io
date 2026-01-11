module github.com/zishang520/socket.io/parsers/socket/v3

go 1.24.1

require github.com/zishang520/socket.io/v3 v3.0.0-rc.10

require (
	github.com/dunglas/httpsfv v1.1.0 // indirect
	github.com/gookit/color v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/quic-go/webtransport-go v0.9.1-0.20260111114619-128538a14311 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
)

replace (
	github.com/zishang520/socket.io/parsers/engine/v3 => ../../parsers/engine
	github.com/zishang520/socket.io/servers/engine/v3 => ../../servers/engine
	github.com/zishang520/socket.io/v3 => ../../
)
