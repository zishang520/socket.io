package types

import (
	"github.com/gorilla/websocket"
)

type WebSocketConn struct {
	EventEmitter

	*websocket.Conn
}

func (t *WebSocketConn) Close() error {
	defer t.Emit("close")
	return t.Conn.Close()
}
