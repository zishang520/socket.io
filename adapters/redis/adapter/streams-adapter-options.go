package adapter

import "github.com/zishang520/socket.io/adapters/adapter/v3"

type (
	RedisStreamsAdapterOptionsInterface interface {
		adapter.ClusterAdapterOptionsInterface

		SetStreamName(string)
		GetRawStreamName() *string
		StreamName() string

		SetMaxLen(int64)
		GetRawMaxLen() *int64
		MaxLen() int64

		SetReadCount(int64)
		GetRawReadCount() *int64
		ReadCount() int64

		SetSessionKeyPrefix(string)
		GetRawSessionKeyPrefix() *string
		SessionKeyPrefix() string
	}

	RedisStreamsAdapterOptions struct {
		adapter.ClusterAdapterOptions

		// The name of the Redis stream.
		//
		//Default: "socket.io"
		streamName *string

		// The maximum size of the stream. Almost exact trimming (~) is used.
		//
		//Default: 10_000
		maxLen *int64

		// The number of elements to fetch per XREAD call.
		//
		//Default: 100
		readCount *int64

		// The prefix of the key used to store the Socket.IO session, when the connection state recovery feature is enabled.
		//
		//Default: "sio:session:"
		sessionKeyPrefix *string
	}
)

func DefaultRedisStreamsAdapterOptions() *RedisStreamsAdapterOptions {
	return &RedisStreamsAdapterOptions{}
}

func (s *RedisStreamsAdapterOptions) Assign(data RedisStreamsAdapterOptionsInterface) RedisStreamsAdapterOptionsInterface {
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

func (s *RedisStreamsAdapterOptions) SetStreamName(streamName string) {
	s.streamName = &streamName
}
func (s *RedisStreamsAdapterOptions) GetRawStreamName() *string {
	return s.streamName
}
func (s *RedisStreamsAdapterOptions) StreamName() string {
	if s.streamName == nil {
		return ""
	}

	return *s.streamName
}

func (s *RedisStreamsAdapterOptions) SetMaxLen(maxLen int64) {
	s.maxLen = &maxLen
}
func (s *RedisStreamsAdapterOptions) GetRawMaxLen() *int64 {
	return s.maxLen
}
func (s *RedisStreamsAdapterOptions) MaxLen() int64 {
	if s.maxLen == nil {
		return 0
	}

	return *s.maxLen
}

func (s *RedisStreamsAdapterOptions) SetReadCount(readCount int64) {
	s.readCount = &readCount
}
func (s *RedisStreamsAdapterOptions) GetRawReadCount() *int64 {
	return s.readCount
}
func (s *RedisStreamsAdapterOptions) ReadCount() int64 {
	if s.readCount == nil {
		return 0
	}

	return *s.readCount
}

func (s *RedisStreamsAdapterOptions) SetSessionKeyPrefix(sessionKeyPrefix string) {
	s.sessionKeyPrefix = &sessionKeyPrefix
}
func (s *RedisStreamsAdapterOptions) GetRawSessionKeyPrefix() *string {
	return s.sessionKeyPrefix
}
func (s *RedisStreamsAdapterOptions) SessionKeyPrefix() string {
	if s.sessionKeyPrefix == nil {
		return ""
	}

	return *s.sessionKeyPrefix
}
