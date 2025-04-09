package utils

import (
	"github.com/zishang520/socket.io/servers/engine/v3/log"
)

var _log = log.NewLog("")

func Log() *log.Log {
	return _log
}
