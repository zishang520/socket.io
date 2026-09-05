// Package adapter defines types and interfaces for the PostgreSQL-based Socket.IO adapter implementation.
// It uses PostgreSQL LISTEN/NOTIFY for inter-node communication in a clustered Socket.IO environment.
package adapter

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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

		// OnNotification processes a raw notification payload from PostgreSQL LISTEN/NOTIFY.
		OnNotification(string)
	}
)

// PostgresAdapterBuilder creates PostgreSQL adapters for Socket.IO namespaces.
// It manages the shared LISTEN connection and routes notifications to namespace adapters.
type PostgresAdapterBuilder struct {
	// Postgres is the PostgreSQL client used for LISTEN/NOTIFY operations.
	Postgres *postgres.PostgresClient
	// Opts contains configuration options for the adapter.
	Opts PostgresAdapterOptionsInterface

	namespaces types.Map[string, *postgresAdapter]
	mu         sync.Mutex
	cancel     context.CancelFunc
}

// New creates a new PostgresAdapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (pb *PostgresAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	adapterInstance := NewPostgresAdapter(nsp, pb.Postgres, pb.Opts).(*postgresAdapter)
	channel := adapterInstance.channel

	pb.mu.Lock()
	pb.namespaces.Store(channel, adapterInstance)
	var listenerCtx context.Context
	if pb.cancel == nil {
		listenerCtx, pb.cancel = context.WithCancel(pb.Postgres.Context())
	}

	listenCtx, cancelListen := context.WithTimeout(pb.Postgres.Context(), postgres.DefaultOperationTimeout)
	err := pb.Postgres.Listen(listenCtx, channel)
	cancelListen()
	pb.mu.Unlock()
	if err != nil {
		if !errors.Is(err, context.Canceled) && pb.Postgres.Context().Err() == nil {
			adapterInstance.onError(err)
		}
	}
	if listenerCtx != nil {
		go pb.startListening(listenerCtx, adapterInstance.opts)
	}

	stopContextClose := context.AfterFunc(pb.Postgres.Context(), adapterInstance.Close)
	adapterInstance.Cleanup(func() {
		stopContextClose()

		pb.mu.Lock()
		if !pb.namespaces.CompareAndDelete(channel, adapterInstance) {
			pb.mu.Unlock()
			return
		}
		if pb.namespaces.Len() == 0 {
			pb.cancel()
			pb.cancel = nil
		}
		unlistenCtx, cancelUnlisten := context.WithTimeout(pb.Postgres.Context(), postgres.DefaultOperationTimeout)
		err := pb.Postgres.Unlisten(unlistenCtx, channel)
		cancelUnlisten()
		pb.mu.Unlock()

		if err != nil {
			if !errors.Is(err, context.Canceled) && pb.Postgres.Context().Err() == nil {
				adapterInstance.onError(err)
			}
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
			options.ErrorHandler()(err)

			timer := time.NewTimer(time.Duration(rand.IntN(2_000)+1_000) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}

		pb.onNotification(ctx, notification)
	}
}

// onNotification queues delivery per namespace so attachment queries do not block
// the shared listener or reorder subsequent messages in the same namespace.
func (pb *PostgresAdapterBuilder) onNotification(ctx context.Context, notification *pgconn.Notification) {
	adapterInstance, ok := pb.namespaces.Load(notification.Channel)
	if !ok {
		return
	}
	adapterInstance.messages.Enqueue(func() {
		if !pb.isActiveListener(ctx, notification.Channel, adapterInstance) {
			return
		}
		message, err := parseNotification(notification.Payload)
		var clusterMessage *ClusterMessage
		if err == nil {
			clusterMessage, err = adapterInstance.decodeReceivedNotification(ctx, message)
		}
		if !pb.isActiveListener(ctx, notification.Channel, adapterInstance) {
			return
		}
		if err != nil {
			adapterInstance.onError(err)
			return
		}
		if clusterMessage != nil {
			adapterInstance.OnMessage(clusterMessage, "")
		}
	})
}

func (pb *PostgresAdapterBuilder) isActiveListener(
	ctx context.Context,
	channel string,
	adapterInstance *postgresAdapter,
) bool {
	if ctx.Err() != nil || adapterInstance.isClosed.Load() {
		return false
	}
	current, ok := pb.namespaces.Load(channel)
	return ok && current == adapterInstance
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
			cleanupCtx, cancel := context.WithTimeout(ctx, postgres.DefaultOperationTimeout)
			err := pb.Postgres.CleanupAttachments(cleanupCtx, tableName, intervalMs)
			cancel()
			if err != nil && ctx.Err() == nil {
				errorHandler(err)
			}
		}
	}
}
