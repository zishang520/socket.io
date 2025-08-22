package socket

import (
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
)

type (
	TransportCtor = transports.TransportCtor

	WebSocketBuilder    = transports.WebSocketBuilder
	WebTransportBuilder = transports.WebTransportBuilder
	PollingBuilder      = transports.PollingBuilder
)

var (
	Polling      TransportCtor = &PollingBuilder{}
	WebSocket    TransportCtor = &WebSocketBuilder{}
	WebTransport TransportCtor = &WebTransportBuilder{}
)
