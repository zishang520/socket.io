// Package adapter provides configuration options for the MongoDB-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Default configuration values for MongoAdapterOptions.
const (
	// DefaultHeartbeatInterval is the default interval between heartbeats.
	DefaultHeartbeatInterval = 5_000 * time.Millisecond

	// DefaultHeartbeatTimeout is the default timeout for heartbeat responses.
	DefaultHeartbeatTimeout int64 = 10_000

	// DefaultRequestsTimeout is the default timeout for inter-node requests.
	DefaultRequestsTimeout = 5_000 * time.Millisecond
)

type (
	// MongoAdapterOptionsInterface defines the interface for configuring MongoAdapterOptions.
	// It extends ClusterAdapterOptionsInterface with adapter-specific settings.
	MongoAdapterOptionsInterface interface {
		adapter.ClusterAdapterOptionsInterface

		SetUid(adapter.ServerId)
		GetRawUid() types.Optional[adapter.ServerId]
		Uid() adapter.ServerId

		SetRequestsTimeout(time.Duration)
		GetRawRequestsTimeout() types.Optional[time.Duration]
		RequestsTimeout() time.Duration

		SetAddCreatedAtField(bool)
		GetRawAddCreatedAtField() types.Optional[bool]
		AddCreatedAtField() bool

		SetChangeStreamOptions(*options.ChangeStreamOptionsBuilder)
		GetRawChangeStreamOptions() types.Optional[*options.ChangeStreamOptionsBuilder]
		ChangeStreamOptions() *options.ChangeStreamOptionsBuilder
	}

	// MongoAdapterOptions holds configuration for the MongoDB adapter.
	//
	// Fields:
	//   - addCreatedAtField: Whether to add a createdAt field to each MongoDB document.
	//     Required when using a TTL index instead of a capped collection.
	//     Default: false.
	MongoAdapterOptions struct {
		adapter.ClusterAdapterOptions

		uid                 types.Optional[adapter.ServerId]
		requestsTimeout     types.Optional[time.Duration]
		addCreatedAtField   types.Optional[bool]
		changeStreamOptions types.Optional[*options.ChangeStreamOptionsBuilder]
	}
)

// DefaultMongoAdapterOptions returns a new MongoAdapterOptions with default values.
func DefaultMongoAdapterOptions() *MongoAdapterOptions {
	return &MongoAdapterOptions{}
}

// Assign copies non-nil fields from another MongoAdapterOptionsInterface.
// This method is useful for merging user-provided options with defaults.
func (s *MongoAdapterOptions) Assign(data MongoAdapterOptionsInterface) MongoAdapterOptionsInterface {
	if utils.IsNil(data) {
		return s
	}

	s.ClusterAdapterOptions.Assign(data)
	if data.GetRawUid() != nil {
		s.SetUid(data.Uid())
	}
	if data.GetRawRequestsTimeout() != nil {
		s.SetRequestsTimeout(data.RequestsTimeout())
	}
	if data.GetRawAddCreatedAtField() != nil {
		s.SetAddCreatedAtField(data.AddCreatedAtField())
	}
	if data.GetRawChangeStreamOptions() != nil {
		s.SetChangeStreamOptions(data.ChangeStreamOptions())
	}

	return s
}

// SetUid sets the identifier shared by every namespace created by this factory.
func (s *MongoAdapterOptions) SetUid(uid adapter.ServerId) {
	s.uid = types.NewSome(uid)
}

// GetRawUid returns the raw Optional value for uid.
func (s *MongoAdapterOptions) GetRawUid() types.Optional[adapter.ServerId] {
	return s.uid
}

// Uid returns the configured server identifier, or an empty string if unset.
func (s *MongoAdapterOptions) Uid() adapter.ServerId {
	if s.uid == nil {
		return ""
	}
	return s.uid.Get()
}

// SetRequestsTimeout sets the timeout for inter-node requests.
func (s *MongoAdapterOptions) SetRequestsTimeout(timeout time.Duration) {
	s.requestsTimeout = types.NewSome(timeout)
}

// GetRawRequestsTimeout returns the raw Optional value for requestsTimeout.
func (s *MongoAdapterOptions) GetRawRequestsTimeout() types.Optional[time.Duration] {
	return s.requestsTimeout
}

// RequestsTimeout returns the configured timeout, or zero if unset.
func (s *MongoAdapterOptions) RequestsTimeout() time.Duration {
	if s.requestsTimeout == nil {
		return 0
	}
	return s.requestsTimeout.Get()
}

// SetAddCreatedAtField sets whether to add a createdAt field to documents.
func (s *MongoAdapterOptions) SetAddCreatedAtField(v bool) {
	s.addCreatedAtField = types.NewSome(v)
}

// GetRawAddCreatedAtField returns the raw Optional value for addCreatedAtField.
func (s *MongoAdapterOptions) GetRawAddCreatedAtField() types.Optional[bool] {
	return s.addCreatedAtField
}

// AddCreatedAtField returns the configured addCreatedAtField value.
// Returns false if not set.
func (s *MongoAdapterOptions) AddCreatedAtField() bool {
	if s.addCreatedAtField == nil {
		return false
	}
	return s.addCreatedAtField.Get()
}

// SetChangeStreamOptions sets the options passed to Collection.Watch.
func (s *MongoAdapterOptions) SetChangeStreamOptions(value *options.ChangeStreamOptionsBuilder) {
	s.changeStreamOptions = types.NewSome(value)
}

// GetRawChangeStreamOptions returns the raw Optional value for changeStreamOptions.
func (s *MongoAdapterOptions) GetRawChangeStreamOptions() types.Optional[*options.ChangeStreamOptionsBuilder] {
	return s.changeStreamOptions
}

// ChangeStreamOptions returns the configured change stream options, or nil if unset.
func (s *MongoAdapterOptions) ChangeStreamOptions() *options.ChangeStreamOptionsBuilder {
	if s.changeStreamOptions == nil {
		return nil
	}
	return s.changeStreamOptions.Get()
}
