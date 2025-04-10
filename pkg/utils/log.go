package utils

import (
	"github.com/zishang520/socket.io/v3/pkg/log"
)

var _log = log.NewLog("")

func Log() *log.Log {
	return _log
}
