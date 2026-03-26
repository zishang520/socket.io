// Package adapter provides configuration options for the Valkey Streams-based Socket.IO adapter.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	DefaultStreamName       = "socket.io"
	DefaultStreamMaxLen     = 10_000
	DefaultStreamReadCount  = 100
	DefaultSessionKeyPrefix = "sio:session:"
)

type (
	// ValkeyStreamsAdapterOptionsInterface defines the interface for configuring ValkeyStreamsAdapterOptions.
	ValkeyStreamsAdapterOptionsInterface interface {
		adapter.ClusterAdapterOptionsInterface

		SetStreamName(string)
		GetRawStreamName() types.Optional[string]
		StreamName() string

		SetMaxLen(int64)
		GetRawMaxLen() types.Optional[int64]
		MaxLen() int64

		SetReadCount(int64)
		GetRawReadCount() types.Optional[int64]
		ReadCount() int64

		SetSessionKeyPrefix(string)
		GetRawSessionKeyPrefix() types.Optional[string]
		SessionKeyPrefix() string
	}

	// ValkeyStreamsAdapterOptions holds configuration for the Valkey Streams adapter.
	ValkeyStreamsAdapterOptions struct {
		adapter.ClusterAdapterOptions

		streamName       types.Optional[string]
		maxLen           types.Optional[int64]
		readCount        types.Optional[int64]
		sessionKeyPrefix types.Optional[string]
	}
)

// DefaultValkeyStreamsAdapterOptions returns a new ValkeyStreamsAdapterOptions with default values.
func DefaultValkeyStreamsAdapterOptions() *ValkeyStreamsAdapterOptions {
	return &ValkeyStreamsAdapterOptions{}
}

// Assign copies non-nil fields from another ValkeyStreamsAdapterOptionsInterface.
func (s *ValkeyStreamsAdapterOptions) Assign(data ValkeyStreamsAdapterOptionsInterface) ValkeyStreamsAdapterOptionsInterface {
	if data == nil {
		return s
	}

	s.ClusterAdapterOptions.Assign(data)

	if data.GetRawStreamName() != nil {
		s.SetStreamName(data.StreamName())
	}
	if data.GetRawMaxLen() != nil {
		s.SetMaxLen(data.MaxLen())
	}
	if data.GetRawReadCount() != nil {
		s.SetReadCount(data.ReadCount())
	}
	if data.GetRawSessionKeyPrefix() != nil {
		s.SetSessionKeyPrefix(data.SessionKeyPrefix())
	}

	return s
}

func (s *ValkeyStreamsAdapterOptions) SetStreamName(v string) { s.streamName = types.NewSome(v) }
func (s *ValkeyStreamsAdapterOptions) GetRawStreamName() types.Optional[string] {
	return s.streamName
}
func (s *ValkeyStreamsAdapterOptions) StreamName() string {
	if s.streamName == nil {
		return ""
	}
	return s.streamName.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetMaxLen(v int64) { s.maxLen = types.NewSome(v) }
func (s *ValkeyStreamsAdapterOptions) GetRawMaxLen() types.Optional[int64] { return s.maxLen }
func (s *ValkeyStreamsAdapterOptions) MaxLen() int64 {
	if s.maxLen == nil {
		return 0
	}
	return s.maxLen.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetReadCount(v int64) { s.readCount = types.NewSome(v) }
func (s *ValkeyStreamsAdapterOptions) GetRawReadCount() types.Optional[int64] { return s.readCount }
func (s *ValkeyStreamsAdapterOptions) ReadCount() int64 {
	if s.readCount == nil {
		return 0
	}
	return s.readCount.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetSessionKeyPrefix(v string) {
	s.sessionKeyPrefix = types.NewSome(v)
}
func (s *ValkeyStreamsAdapterOptions) GetRawSessionKeyPrefix() types.Optional[string] {
	return s.sessionKeyPrefix
}
func (s *ValkeyStreamsAdapterOptions) SessionKeyPrefix() string {
	if s.sessionKeyPrefix == nil {
		return ""
	}
	return s.sessionKeyPrefix.Get()
}
