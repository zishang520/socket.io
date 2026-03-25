package utils

import (
	"github.com/zishang520/socket.io/v3/pkg/log"
)

func Log() *log.Log {
	return log.Default()
}
