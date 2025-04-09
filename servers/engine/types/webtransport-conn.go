package types

import (
	"github.com/zishang520/socket.io/servers/engine/v3/webtransport"
	wt "github.com/zishang520/webtransport-go"
)

type WebTransportConn struct {
	EventEmitter

	*webtransport.Conn
}

func (t *WebTransportConn) CloseWithError(code wt.SessionErrorCode, msg string) error {
	defer t.Emit("close")
	return t.Conn.CloseWithError(code, msg)
}
