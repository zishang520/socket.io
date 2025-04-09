// Package engine implements the Engine.IO client transport layer.
package engine

import "github.com/zishang520/socket.io/servers/engine/v3/log"

// Log implements formatted logging for Engine.IO components.
type Log struct {
	*log.Log
}

// NewLog returns a logger that prefixes messages with prefix.
//
// Example:
//
//	log := NewLog("websocket")
//	log.Debugf("state: %s", state) // Output: [websocket] state: connected
func NewLog(prefix string) *Log {
	return &Log{Log: log.NewLog(prefix)}
}

// Debugf logs a debug message using format and args.
//
// Example:
//
//	log.Debugf("bytes: %d", n)
func (l *Log) Debugf(message string, args ...any) {
	l.Debug(message, args...)
}

// Errorf logs an error message using format and args.
//
// Example:
//
//	log.Errorf("failed: %v", err)
func (l *Log) Errorf(message string, args ...any) {
	l.Error(message, args...)
}

// Warnf logs a warning message using format and args.
//
// Example:
//
//	log.Warnf("using: %s", transport)
func (l *Log) Warnf(message string, args ...any) {
	l.Warning(message, args...)
}
