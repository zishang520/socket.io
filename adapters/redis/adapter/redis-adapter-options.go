// Package adapter provides configuration options for the Redis-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// Default configuration values for RedisAdapterOptions.
const (
	// DefaultRequestsTimeout is the default timeout for inter-node requests.
	DefaultRequestsTimeout = 5000 * time.Millisecond
)

type (
	// RedisAdapterOptionsInterface defines the classic Redis adapter settings.
	RedisAdapterOptionsInterface interface {
		SetKey(string)
		GetRawKey() types.Optional[string]
		Key() string

		SetParser(redis.Parser)
		GetRawParser() types.Optional[redis.Parser]
		Parser() redis.Parser

		SetRequestsTimeout(time.Duration)
		GetRawRequestsTimeout() types.Optional[time.Duration]
		RequestsTimeout() time.Duration

		SetPublishOnSpecificResponseChannel(bool)
		GetRawPublishOnSpecificResponseChannel() types.Optional[bool]
		PublishOnSpecificResponseChannel() bool
	}

	// RedisAdapterOptions holds configuration for the Redis adapter.
	//
	// Fields:
	//   - key: Redis channel prefix. Default: "socket.io".
	//   - parser: Full encoder/decoder used for cluster messages. Default: MessagePack.
	//   - requestsTimeout: Maximum time to wait for responses to inter-node requests.
	//     Default: 5000ms. After this timeout, the adapter stops waiting for responses.
	//   - publishOnSpecificResponseChannel: When true, responses are published to a
	//     channel specific to the requesting node, reducing unnecessary message processing.
	//     Default: false.
	RedisAdapterOptions struct {
		key                              types.Optional[string]
		parser                           types.Optional[redis.Parser]
		requestsTimeout                  types.Optional[time.Duration]
		publishOnSpecificResponseChannel types.Optional[bool]
	}
)

// DefaultRedisAdapterOptions returns raw-empty options; RedisAdapter.Construct applies defaults.
func DefaultRedisAdapterOptions() *RedisAdapterOptions {
	return &RedisAdapterOptions{}
}

// Assign copies non-nil fields from another RedisAdapterOptionsInterface.
// This method is useful for merging user-provided options with defaults.
func (s *RedisAdapterOptions) Assign(data RedisAdapterOptionsInterface) RedisAdapterOptionsInterface {
	if utils.IsNil(data) {
		return s
	}

	if data.GetRawKey() != nil {
		s.SetKey(data.Key())
	}
	if data.GetRawParser() != nil {
		s.SetParser(data.Parser())
	}

	if data.GetRawRequestsTimeout() != nil {
		s.SetRequestsTimeout(data.RequestsTimeout())
	}
	if data.GetRawPublishOnSpecificResponseChannel() != nil {
		s.SetPublishOnSpecificResponseChannel(data.PublishOnSpecificResponseChannel())
	}

	return s
}

// SetKey sets the Redis channel prefix.
func (s *RedisAdapterOptions) SetKey(key string) {
	s.key = types.NewSome(key)
}

// GetRawKey returns the raw Optional wrapper for the key setting.
func (s *RedisAdapterOptions) GetRawKey() types.Optional[string] {
	return s.key
}

// Key returns the configured Redis channel prefix, or an empty string.
func (s *RedisAdapterOptions) Key() string {
	if s.key == nil {
		return ""
	}
	return s.key.Get()
}

// SetParser sets the full encoder/decoder used by the Redis adapter.
func (s *RedisAdapterOptions) SetParser(parser redis.Parser) {
	s.parser = types.NewSome(parser)
}

// GetRawParser returns the raw Optional wrapper for the parser setting.
func (s *RedisAdapterOptions) GetRawParser() types.Optional[redis.Parser] {
	return s.parser
}

// Parser returns the configured parser, or nil if not set.
func (s *RedisAdapterOptions) Parser() redis.Parser {
	if s.parser == nil {
		return nil
	}
	return s.parser.Get()
}

// SetRequestsTimeout sets the timeout duration for inter-node requests.
func (s *RedisAdapterOptions) SetRequestsTimeout(requestsTimeout time.Duration) {
	s.requestsTimeout = types.NewSome(requestsTimeout)
}

// GetRawRequestsTimeout returns the raw Optional value for requestsTimeout.
// Returns nil if not explicitly set.
func (s *RedisAdapterOptions) GetRawRequestsTimeout() types.Optional[time.Duration] {
	return s.requestsTimeout
}

// RequestsTimeout returns the configured requests timeout.
// Returns 0 if not explicitly set; callers should use DefaultRequestsTimeout as fallback.
func (s *RedisAdapterOptions) RequestsTimeout() time.Duration {
	if s.requestsTimeout == nil {
		return 0
	}
	return s.requestsTimeout.Get()
}

// SetPublishOnSpecificResponseChannel sets whether responses should be published
// to a node-specific channel.
func (s *RedisAdapterOptions) SetPublishOnSpecificResponseChannel(publishOnSpecificResponseChannel bool) {
	s.publishOnSpecificResponseChannel = types.NewSome(publishOnSpecificResponseChannel)
}

// GetRawPublishOnSpecificResponseChannel returns the raw Optional value.
// Returns nil if not explicitly set.
func (s *RedisAdapterOptions) GetRawPublishOnSpecificResponseChannel() types.Optional[bool] {
	return s.publishOnSpecificResponseChannel
}

// PublishOnSpecificResponseChannel returns whether responses should be published
// to a node-specific channel. Returns false if not explicitly set.
func (s *RedisAdapterOptions) PublishOnSpecificResponseChannel() bool {
	if s.publishOnSpecificResponseChannel == nil {
		return false
	}
	return s.publishOnSpecificResponseChannel.Get()
}
