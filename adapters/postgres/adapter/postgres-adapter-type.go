// Package adapter defines types and interfaces for the PostgreSQL-based Socket.IO adapter implementation.
// It uses PostgreSQL LISTEN/NOTIFY for inter-node communication in a clustered Socket.IO environment.
package adapter

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
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

	namespaces   types.Map[string, PostgresAdapter]
	mu           sync.Mutex
	cancel       context.CancelFunc
	listenerDone chan struct{}
}

// New creates a new PostgresAdapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (pb *PostgresAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	adapterInstance := NewPostgresAdapter(nsp, pb.Postgres, pb.Opts).(*postgresAdapter)
	channel := adapterInstance.channel

	pb.mu.Lock()
	pb.namespaces.Store(channel, adapterInstance)
	var listenerCtx context.Context
	var previousDone, listenerDone chan struct{}
	if pb.cancel == nil {
		listenerCtx, pb.cancel = context.WithCancel(pb.Postgres.Context())
		previousDone = pb.listenerDone
		listenerDone = make(chan struct{})
		pb.listenerDone = listenerDone
	}
	pb.mu.Unlock()

	if err := pb.Postgres.Listen(pb.Postgres.Context(), channel); err != nil {
		postgresLog.Debug("failed to listen on channel %s: %s", channel, err.Error())
	}
	if listenerCtx != nil {
		options := adapterInstance.opts
		go func() {
			defer close(listenerDone)
			if previousDone != nil {
				<-previousDone
			}
			if listenerCtx.Err() == nil {
				pb.startListening(listenerCtx, options)
			}
		}()
	}

	adapterInstance.Cleanup(func() {
		pb.mu.Lock()
		if !pb.namespaces.CompareAndDelete(channel, adapterInstance) {
			pb.mu.Unlock()
			return
		}
		if pb.namespaces.Len() == 0 {
			if pb.cancel != nil {
				pb.cancel()
			}
			pb.cancel = nil
		}
		err := pb.Postgres.Unlisten(pb.Postgres.Context(), channel)
		pb.mu.Unlock()

		if err != nil {
			postgresLog.Debug("failed to unlisten from channel %s: %s", channel, err.Error())
		}
	})

	return adapterInstance
}

// startListening continuously waits for PostgreSQL notifications and dispatches them
// to the appropriate namespace adapter.
func (pb *PostgresAdapterBuilder) startListening(ctx context.Context, options *PostgresAdapterOptions) {
	// Start cleanup timer for old attachments
	cleanupInterval := options.CleanupInterval()
	tableName := options.TableName()

	if cleanupInterval > 0 {
		cleanupCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go pb.cleanupLoop(cleanupCtx, cleanupInterval, tableName, options.ErrorHandler())
	}

	for {
		notification, err := pb.Postgres.WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return
			}
			postgresLog.Debug("listener error: %s", err.Error())

			timer := time.NewTimer(time.Duration(rand.IntN(2_000)+1_000) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}

		if notification == nil {
			continue
		}

		if adapterInstance, ok := pb.namespaces.Load(notification.Channel); ok {
			adapterInstance.OnNotification(notification.Payload)
		}
	}
}

// cleanupLoop periodically cleans up old attachments from the storage table.
func (pb *PostgresAdapterBuilder) cleanupLoop(ctx context.Context, intervalMs int64, tableName string, errorHandler func(error)) {
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := pb.Postgres.CleanupAttachments(ctx, tableName, intervalMs); err != nil && ctx.Err() == nil {
				errorHandler(err)
			}
		}
	}
}
