package emitter

import (
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const (
	DefaultStreamName   = "socket.io"
	DefaultStreamMaxLen = 10_000
	DefaultStreamCount  = 1
)

type (
	// RedisStreamsEmitterOptionsInterface configures the Redis stream used by the emitter.
	RedisStreamsEmitterOptionsInterface interface {
		SetStreamName(string)
		GetRawStreamName() types.Optional[string]
		StreamName() string

		SetStreamCount(int)
		GetRawStreamCount() types.Optional[int]
		StreamCount() int

		SetMaxLen(int64)
		GetRawMaxLen() types.Optional[int64]
		MaxLen() int64
	}

	// RedisStreamsEmitterOptions holds optional Redis Streams emitter settings.
	RedisStreamsEmitterOptions struct {
		streamName  types.Optional[string]
		streamCount types.Optional[int]
		maxLen      types.Optional[int64]
	}
)

// DefaultRedisStreamsEmitterOptions creates empty Redis Streams emitter options.
func DefaultRedisStreamsEmitterOptions() *RedisStreamsEmitterOptions {
	return new(RedisStreamsEmitterOptions)
}

// Assign copies explicitly set values from data.
func (o *RedisStreamsEmitterOptions) Assign(data RedisStreamsEmitterOptionsInterface) RedisStreamsEmitterOptionsInterface {
	if utils.IsNil(data) {
		return o
	}
	if data.GetRawStreamName() != nil {
		o.SetStreamName(data.StreamName())
	}
	if data.GetRawStreamCount() != nil {
		o.SetStreamCount(data.StreamCount())
	}
	if data.GetRawMaxLen() != nil {
		o.SetMaxLen(data.MaxLen())
	}
	return o
}

func (o *RedisStreamsEmitterOptions) SetStreamCount(streamCount int) {
	o.streamCount = types.NewSome(streamCount)
}

func (o *RedisStreamsEmitterOptions) GetRawStreamCount() types.Optional[int] {
	return o.streamCount
}

func (o *RedisStreamsEmitterOptions) StreamCount() int {
	if o.streamCount == nil {
		return 0
	}
	return o.streamCount.Get()
}

func (o *RedisStreamsEmitterOptions) SetStreamName(streamName string) {
	o.streamName = types.NewSome(streamName)
}

func (o *RedisStreamsEmitterOptions) GetRawStreamName() types.Optional[string] {
	return o.streamName
}

func (o *RedisStreamsEmitterOptions) StreamName() string {
	if o.streamName == nil {
		return ""
	}
	return o.streamName.Get()
}

func (o *RedisStreamsEmitterOptions) SetMaxLen(maxLen int64) {
	o.maxLen = types.NewSome(maxLen)
}

func (o *RedisStreamsEmitterOptions) GetRawMaxLen() types.Optional[int64] {
	return o.maxLen
}

func (o *RedisStreamsEmitterOptions) MaxLen() int64 {
	if o.maxLen == nil {
		return 0
	}
	return o.maxLen.Get()
}
