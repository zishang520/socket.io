package transports

const (
	POLLING      string = "polling"
	WEBSOCKET    string = "websocket"
	WEBTRANSPORT string = "webtransport"
)

var transports map[string]TransportCtor

func init() {
	transports = map[string]TransportCtor{
		POLLING:      &PollingBuilder{},
		WEBSOCKET:    &WebSocketBuilder{},
		WEBTRANSPORT: &WebTransportBuilder{},
	}
}

func Transports() map[string]TransportCtor {
	return transports
}
