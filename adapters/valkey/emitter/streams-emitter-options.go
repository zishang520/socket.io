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
	// ValkeyStreamsEmitterOptionsInterface configures the Valkey stream used by the emitter.
	ValkeyStreamsEmitterOptionsInterface interface {
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

	// ValkeyStreamsEmitterOptions holds optional Valkey Streams emitter settings.
	ValkeyStreamsEmitterOptions struct {
		streamName  types.Optional[string]
		streamCount types.Optional[int]
		maxLen      types.Optional[int64]
	}
)

func DefaultValkeyStreamsEmitterOptions() *ValkeyStreamsEmitterOptions {
	return new(ValkeyStreamsEmitterOptions)
}

func (o *ValkeyStreamsEmitterOptions) Assign(data ValkeyStreamsEmitterOptionsInterface) ValkeyStreamsEmitterOptionsInterface {
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

func (o *ValkeyStreamsEmitterOptions) SetStreamCount(streamCount int) {
	o.streamCount = types.NewSome(streamCount)
}

func (o *ValkeyStreamsEmitterOptions) GetRawStreamCount() types.Optional[int] {
	return o.streamCount
}

func (o *ValkeyStreamsEmitterOptions) StreamCount() int {
	if o.streamCount == nil {
		return 0
	}
	return o.streamCount.Get()
}

func (o *ValkeyStreamsEmitterOptions) SetStreamName(streamName string) {
	o.streamName = types.NewSome(streamName)
}

func (o *ValkeyStreamsEmitterOptions) GetRawStreamName() types.Optional[string] {
	return o.streamName
}

func (o *ValkeyStreamsEmitterOptions) StreamName() string {
	if o.streamName == nil {
		return ""
	}
	return o.streamName.Get()
}

func (o *ValkeyStreamsEmitterOptions) SetMaxLen(maxLen int64) {
	o.maxLen = types.NewSome(maxLen)
}

func (o *ValkeyStreamsEmitterOptions) GetRawMaxLen() types.Optional[int64] {
	return o.maxLen
}

func (o *ValkeyStreamsEmitterOptions) MaxLen() int64 {
	if o.maxLen == nil {
		return 0
	}
	return o.maxLen.Get()
}
