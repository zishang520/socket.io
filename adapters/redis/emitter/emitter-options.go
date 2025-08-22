// Package emitter provides options and interfaces for configuring the Redis emitter in Socket.IO.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	// EmitterOptionsInterface defines the interface for configuring emitter options.
	EmitterOptionsInterface interface {
		SetKey(string)
		GetRawKey() types.Optional[string]
		Key() string

		SetParser(redis.Parser)
		GetRawParser() types.Optional[redis.Parser]
		Parser() redis.Parser
	}

	// EmitterOptions holds configuration for the Redis emitter.
	//
	// Key: the Redis key prefix (default: "socket.io").
	// Parser: the parser used for encoding messages (default: msgpack).
	EmitterOptions struct {
		// key is the Redis key prefix.
		key types.Optional[string]

		// parser is the parser to use for encoding messages sent to Redis.
		parser types.Optional[redis.Parser]
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
	s.key = types.NewSome(key)
}
func (s *EmitterOptions) GetRawKey() types.Optional[string] {
	return s.key
}
func (s *EmitterOptions) Key() string {
	if s.key == nil {
		return ""
	}

	return s.key.Get()
}

// SetParser sets the parser for encoding messages.
func (s *EmitterOptions) SetParser(parser redis.Parser) {
	s.parser = types.NewSome(parser)
}
func (s *EmitterOptions) GetRawParser() types.Optional[redis.Parser] {
	return s.parser
}
func (s *EmitterOptions) Parser() redis.Parser {
	if s.parser == nil {
		return nil
	}

	return s.parser.Get()
}
