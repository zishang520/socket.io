// Package adapter defines types and interfaces for the PostgreSQL-based Socket.IO adapter implementation.
// It uses PostgreSQL LISTEN/NOTIFY for inter-node communication in a clustered Socket.IO environment.
package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/postgres/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	// ClusterMessage is an alias for adapter.ClusterMessage.
	ClusterMessage = adapter.ClusterMessage

	// ClusterResponse is an alias for adapter.ClusterResponse.
	ClusterResponse = adapter.ClusterResponse

	// BroadcastMessage is an alias for adapter.BroadcastMessage.
	BroadcastMessage = adapter.BroadcastMessage

	// SocketsJoinLeaveMessage is an alias for adapter.SocketsJoinLeaveMessage.
	SocketsJoinLeaveMessage = adapter.SocketsJoinLeaveMessage

	// DisconnectSocketsMessage is an alias for adapter.DisconnectSocketsMessage.
	DisconnectSocketsMessage = adapter.DisconnectSocketsMessage

	// FetchSocketsMessage is an alias for adapter.FetchSocketsMessage.
	FetchSocketsMessage = adapter.FetchSocketsMessage

	// FetchSocketsResponse is an alias for adapter.FetchSocketsResponse.
	FetchSocketsResponse = adapter.FetchSocketsResponse

	// ServerSideEmitMessage is an alias for adapter.ServerSideEmitMessage.
	ServerSideEmitMessage = adapter.ServerSideEmitMessage

	// ServerSideEmitResponse is an alias for adapter.ServerSideEmitResponse.
	ServerSideEmitResponse = adapter.ServerSideEmitResponse

	// BroadcastClientCount is an alias for adapter.BroadcastClientCount.
	BroadcastClientCount = adapter.BroadcastClientCount

	// BroadcastAck is an alias for adapter.BroadcastAck.
	BroadcastAck = adapter.BroadcastAck

	// NotificationMessage represents a message received via PostgreSQL LISTEN/NOTIFY.
	NotificationMessage = postgres.NotificationMessage

	// PostgresAdapter defines the interface for a PostgreSQL-based Socket.IO adapter.
	// It extends ClusterAdapterWithHeartbeat with PostgreSQL-specific functionality.
	PostgresAdapter interface {
		adapter.ClusterAdapterWithHeartbeat

		// SetPostgres configures the PostgreSQL client for the adapter.
		SetPostgres(*postgres.PostgresClient)

		// Cleanup registers a cleanup callback to be called when the adapter is closed.
		Cleanup(func())

		// SetChannel sets the notification channel for this adapter.
		SetChannel(string)

		// OnNotification processes a raw notification payload from PostgreSQL LISTEN/NOTIFY.
		OnNotification(string)
	}
)

// PostgresAdapterBuilder creates PostgreSQL adapters for Socket.IO namespaces.
// It manages the shared LISTEN connection and notification loop across all namespace adapters.
type PostgresAdapterBuilder struct {
	// Postgres is the PostgreSQL client used for LISTEN/NOTIFY operations.
	Postgres *postgres.PostgresClient
	// Opts contains configuration options for the adapter.
	Opts PostgresAdapterOptionsInterface

	namespaceToAdapters types.Map[string, PostgresAdapter]
	listening           atomic.Bool
}

// New creates a new PostgresAdapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (pb *PostgresAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	options := DefaultPostgresAdapterOptions()
	options.Assign(pb.Opts)

	// Apply defaults
	if options.GetRawKey() == nil {
		options.SetKey(DefaultChannelPrefix)
	}
	if options.GetRawTableName() == nil {
		options.SetTableName(DefaultTableName)
	}
	if options.GetRawPayloadThreshold() == nil {
		options.SetPayloadThreshold(DefaultPayloadThreshold)
	}
	if options.GetRawCleanupInterval() == nil {
		options.SetCleanupInterval(DefaultCleanupInterval)
	}
	if options.GetRawHeartbeatInterval() == nil {
		options.SetHeartbeatInterval(DefaultHeartbeatInterval)
	}
	if options.GetRawHeartbeatTimeout() == nil {
		options.SetHeartbeatTimeout(DefaultHeartbeatTimeout)
	}

	channel := options.Key() + "#" + nsp.Name()

	adapterInstance := NewPostgresAdapter(nsp, pb.Postgres, options)
	adapterInstance.SetChannel(channel)

	pb.namespaceToAdapters.Store(nsp.Name(), adapterInstance)

	// Start listening if not already
	if pb.listening.CompareAndSwap(false, true) {
		// Ensure the attachment table exists
		if err := pb.Postgres.EnsureTable(pb.Postgres.Context, options.TableName()); err != nil {
			pb.Postgres.Emit("error", err)
		}

		// Listen on the channel for this namespace
		if err := pb.Postgres.Listen(pb.Postgres.Context, channel); err != nil {
			pb.Postgres.Emit("error", err)
		}

		go pb.startListening(options)
	} else {
		// Listen on additional channel for new namespace
		if err := pb.Postgres.Listen(pb.Postgres.Context, channel); err != nil {
			pb.Postgres.Emit("error", err)
		}
	}

	// Register cleanup callback
	adapterInstance.Cleanup(func() {
		_ = pb.Postgres.Unlisten(pb.Postgres.Context, channel)
		pb.namespaceToAdapters.Delete(nsp.Name())
	})

	return adapterInstance
}

// startListening continuously waits for PostgreSQL notifications and dispatches them
// to the appropriate namespace adapter.
func (pb *PostgresAdapterBuilder) startListening(options *PostgresAdapterOptions) {
	// Start cleanup timer for old attachments
	cleanupInterval := options.CleanupInterval()
	tableName := options.TableName()

	if cleanupInterval > 0 {
		go pb.cleanupLoop(cleanupInterval, tableName)
	}

	for {
		notification, err := pb.Postgres.WaitForNotification(pb.Postgres.Context)
		if err != nil {
			if pb.Postgres.Context.Err() != nil {
				return // Context canceled, stop listening
			}
			pb.Postgres.Emit("error", err)

			// Brief delay before retrying to avoid tight error loop
			time.Sleep(1 * time.Second)
			continue
		}

		if notification == nil {
			continue
		}

		// Dispatch notification to the matching adapter
		// The channel format is "{prefix}#{nsp}" — extract namespace from the channel
		pb.dispatchNotification(notification.Channel, notification.Payload, options)
	}
}

// dispatchNotification sends a notification payload to the correct adapter based on channel.
func (pb *PostgresAdapterBuilder) dispatchNotification(channel, payload string, options *PostgresAdapterOptions) {
	// Find namespace from channel: "{prefix}#{nsp}"
	prefix := options.Key() + "#"
	if len(channel) <= len(prefix) {
		return
	}
	nspName := channel[len(prefix):]

	if adapterInstance, ok := pb.namespaceToAdapters.Load(nspName); ok {
		adapterInstance.OnNotification(payload)
	}
}

// cleanupLoop periodically cleans up old attachments from the storage table.
func (pb *PostgresAdapterBuilder) cleanupLoop(intervalMs int64, tableName string) {
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-pb.Postgres.Context.Done():
			return
		case <-ticker.C:
			if err := pb.Postgres.CleanupAttachments(pb.Postgres.Context, tableName, intervalMs); err != nil {
				pb.Postgres.Emit("error", err)
			}
		}
	}
}
