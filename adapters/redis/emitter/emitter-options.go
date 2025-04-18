// Package emitter provides options and interfaces for configuring the Redis emitter in Socket.IO.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

type (
	// EmitterOptionsInterface defines the interface for configuring emitter options.
	EmitterOptionsInterface interface {
		SetKey(string)
		GetRawKey() *string
		Key() string

		SetParser(redis.Parser)
		GetRawParser() redis.Parser
		Parser() redis.Parser
	}

	// EmitterOptions holds configuration for the Redis emitter.
	//
	// Key: the Redis key prefix (default: "socket.io").
	// Parser: the parser used for encoding messages (default: msgpack).
	EmitterOptions struct {
		// key is the Redis key prefix.
		key *string

		// parser is the parser to use for encoding messages sent to Redis.
		parser redis.Parser
	}
)

// DefaultEmitterOptions returns a new EmitterOptions with default values.
func DefaultEmitterOptions() *EmitterOptions {
	return &EmitterOptions{}
}

// Assign copies non-nil fields from another EmitterOptionsInterface.
func (s *EmitterOptions) Assign(data EmitterOptionsInterface) EmitterOptionsInterface {
	if data == nil {
		return s
	}

	if data.GetRawKey() != nil {
		s.SetKey(data.Key())
	}
	if data.GetRawParser() != nil {
		s.SetParser(data.Parser())
	}

	return s
}

// SetKey sets the Redis key prefix.
func (s *EmitterOptions) SetKey(key string) {
	s.key = &key
}

// GetRawKey returns the raw Redis key pointer.
func (s *EmitterOptions) GetRawKey() *string {
	return s.key
}

// Key returns the Redis key prefix. Default is "socket.io".
func (s *EmitterOptions) Key() string {
	if s.key == nil {
		return ""
	}

	return *s.key
}

// SetParser sets the parser for encoding messages.
func (s *EmitterOptions) SetParser(parser redis.Parser) {
	s.parser = parser
}

// GetRawParser returns the raw parser.
func (s *EmitterOptions) GetRawParser() redis.Parser {
	return s.parser
}

// Parser returns the parser for encoding messages. Default is msgpack.
func (s *EmitterOptions) Parser() redis.Parser {
	return s.parser
}
