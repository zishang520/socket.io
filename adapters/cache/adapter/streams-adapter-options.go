// Package adapter provides configuration options for the cache Streams-based adapter.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Default configuration values for CacheStreamsAdapterOptions.
const (
	DefaultStreamName       = "socket.io"
	DefaultStreamMaxLen     = int64(10_000)
	DefaultStreamReadCount  = int64(100)
	DefaultSessionKeyPrefix = "sio:session:"
)

type (
	// CacheStreamsAdapterOptionsInterface is the configuration interface for CacheStreamsAdapterOptions.
	CacheStreamsAdapterOptionsInterface interface {
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

	// CacheStreamsAdapterOptions holds configuration for the cache streams adapter.
	//
	//   - streamName: The stream key. Default: "socket.io".
	//   - maxLen: Approximate maximum stream length. Default: 10 000.
	//   - readCount: Entries fetched per XRead call. Default: 100.
	//   - sessionKeyPrefix: Prefix for session keys. Default: "sio:session:".
	CacheStreamsAdapterOptions struct {
		adapter.ClusterAdapterOptions

		streamName       types.Optional[string]
		maxLen           types.Optional[int64]
		readCount        types.Optional[int64]
		sessionKeyPrefix types.Optional[string]
	}
)

// DefaultCacheStreamsAdapterOptions returns a zero-valued CacheStreamsAdapterOptions.
func DefaultCacheStreamsAdapterOptions() *CacheStreamsAdapterOptions {
	return &CacheStreamsAdapterOptions{}
}

// Assign copies non-nil fields from data into s.
func (s *CacheStreamsAdapterOptions) Assign(data CacheStreamsAdapterOptionsInterface) CacheStreamsAdapterOptionsInterface {
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

func (s *CacheStreamsAdapterOptions) SetStreamName(v string)                   { s.streamName = types.NewSome(v) }
func (s *CacheStreamsAdapterOptions) GetRawStreamName() types.Optional[string] { return s.streamName }
func (s *CacheStreamsAdapterOptions) StreamName() string {
	if s.streamName == nil {
		return ""
	}
	return s.streamName.Get()
}

func (s *CacheStreamsAdapterOptions) SetMaxLen(v int64)                   { s.maxLen = types.NewSome(v) }
func (s *CacheStreamsAdapterOptions) GetRawMaxLen() types.Optional[int64] { return s.maxLen }
func (s *CacheStreamsAdapterOptions) MaxLen() int64 {
	if s.maxLen == nil {
		return 0
	}
	return s.maxLen.Get()
}

func (s *CacheStreamsAdapterOptions) SetReadCount(v int64)                   { s.readCount = types.NewSome(v) }
func (s *CacheStreamsAdapterOptions) GetRawReadCount() types.Optional[int64] { return s.readCount }
func (s *CacheStreamsAdapterOptions) ReadCount() int64 {
	if s.readCount == nil {
		return 0
	}
	return s.readCount.Get()
}

func (s *CacheStreamsAdapterOptions) SetSessionKeyPrefix(v string) {
	s.sessionKeyPrefix = types.NewSome(v)
}
func (s *CacheStreamsAdapterOptions) GetRawSessionKeyPrefix() types.Optional[string] {
	return s.sessionKeyPrefix
}
func (s *CacheStreamsAdapterOptions) SessionKeyPrefix() string {
	if s.sessionKeyPrefix == nil {
		return ""
	}
	return s.sessionKeyPrefix.Get()
}
