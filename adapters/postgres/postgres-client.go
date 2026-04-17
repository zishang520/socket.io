// Package postgres provides PostgreSQL client wrapper for Socket.IO PostgreSQL adapter.
// This package offers a unified interface for PostgreSQL operations with event handling support
// using LISTEN/NOTIFY for pub/sub communication.
package postgres

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// PostgresClient wraps a pgxpool.Pool and provides context management
// and event emitting capabilities for the Socket.IO PostgreSQL adapter.
//
// The client supports a separate listener connection for LISTEN/NOTIFY operations.
// The Pool is used for write operations (pg_notify, INSERT, DELETE, etc.)
// and the Listener connection is used for LISTEN operations.
//
// The client supports error event emission, which allows higher-level components
// to handle PostgreSQL-related errors gracefully.
type PostgresClient struct {
	types.EventEmitter

	// Pool is the connection pool used for write operations
	// (pg_notify, INSERT, DELETE, SELECT, etc.).
	Pool *pgxpool.Pool

	// Context is the context used for PostgreSQL operations.
	// This context controls the lifecycle of subscriptions and operations.
	Context context.Context

	// listenerConn is a dedicated connection for LISTEN operations.
	// It is lazily acquired from the pool.
	listenerConn *pgx.Conn
	listenerMu   sync.Mutex
}

// NewPostgresClient creates a new PostgresClient with the given context and connection pool.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of PostgreSQL operations.
//     When canceled, all subscriptions and pending operations will be terminated.
//   - pool: A pgxpool.Pool instance that handles the actual PostgreSQL communication.
//
// Returns:
//   - A pointer to the initialized PostgresClient instance.
//
// Example:
//
//	pool, _ := pgxpool.New(context.Background(), "postgres://user:pass@localhost:5432/db")
//	pgClient := NewPostgresClient(context.Background(), pool)
func NewPostgresClient(ctx context.Context, pool *pgxpool.Pool) *PostgresClient {
	if ctx == nil {
		ctx = context.Background()
	}

	return &PostgresClient{
		EventEmitter: types.NewEventEmitter(),
		Pool:         pool,
		Context:      ctx,
	}
}

// getListenerConn returns a dedicated connection for LISTEN/NOTIFY operations.
// The connection is lazily acquired from the pool on first call and reused thereafter.
// This is thread-safe.
func (c *PostgresClient) getListenerConn() (*pgx.Conn, error) {
	c.listenerMu.Lock()
	defer c.listenerMu.Unlock()

	if c.listenerConn != nil {
		return c.listenerConn, nil
	}

	conn, err := pgx.Connect(c.Context, c.Pool.Config().ConnConfig.ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to acquire listener connection: %w", err)
	}

	c.listenerConn = conn
	return conn, nil
}

// Listen subscribes to the specified PostgreSQL notification channels using LISTEN.
// A dedicated connection is used to ensure notifications are not lost.
//
// Parameters:
//   - ctx: The context for the LISTEN operation.
//   - channels: One or more channel names to listen on.
func (c *PostgresClient) Listen(ctx context.Context, channels ...string) error {
	conn, err := c.getListenerConn()
	if err != nil {
		return err
	}

	for _, channel := range channels {
		if _, err := conn.Exec(ctx, fmt.Sprintf("LISTEN %s", pgx.Identifier{channel}.Sanitize())); err != nil {
			return fmt.Errorf("failed to LISTEN on channel %q: %w", channel, err)
		}
	}

	return nil
}

// Unlisten unsubscribes from the specified PostgreSQL notification channels using UNLISTEN.
//
// Parameters:
//   - ctx: The context for the UNLISTEN operation.
//   - channels: One or more channel names to unlisten from.
func (c *PostgresClient) Unlisten(ctx context.Context, channels ...string) error {
	c.listenerMu.Lock()
	conn := c.listenerConn
	c.listenerMu.Unlock()

	if conn == nil {
		return nil
	}

	for _, channel := range channels {
		if _, err := conn.Exec(ctx, fmt.Sprintf("UNLISTEN %s", pgx.Identifier{channel}.Sanitize())); err != nil {
			return fmt.Errorf("failed to UNLISTEN on channel %q: %w", channel, err)
		}
	}

	return nil
}

// WaitForNotification waits for a notification on the listener connection.
// This method blocks until a notification is received or the context is canceled.
//
// Returns the received notification or an error if the wait was interrupted.
func (c *PostgresClient) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	conn, err := c.getListenerConn()
	if err != nil {
		return nil, err
	}

	return conn.WaitForNotification(ctx)
}

// Notify sends a NOTIFY on the specified channel with the given payload.
// Uses pg_notify() to send the notification through the connection pool.
//
// Parameters:
//   - ctx: The context for the notification operation.
//   - channel: The notification channel name.
//   - payload: The notification payload string.
func (c *PostgresClient) Notify(ctx context.Context, channel, payload string) error {
	_, err := c.Pool.Exec(ctx, "SELECT pg_notify($1, $2)", channel, payload)
	return err
}

// EnsureTable creates the attachment table if it does not exist.
// This table is used to store large payloads that exceed the pg_notify limit.
//
// Parameters:
//   - ctx: The context for the operation.
//   - tableName: The name of the table to create.
func (c *PostgresClient) EnsureTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (id bigserial UNIQUE, created_at timestamptz DEFAULT NOW(), payload bytea)",
		pgx.Identifier{tableName}.Sanitize(),
	)
	_, err := c.Pool.Exec(ctx, query)
	return err
}

// InsertAttachment inserts a payload into the attachment table and returns its generated ID.
//
// Parameters:
//   - ctx: The context for the operation.
//   - tableName: The name of the attachment table.
//   - payload: The binary payload to store.
func (c *PostgresClient) InsertAttachment(ctx context.Context, tableName string, payload []byte) (int64, error) {
	var id int64
	query := fmt.Sprintf("INSERT INTO %s (payload) VALUES ($1) RETURNING id", pgx.Identifier{tableName}.Sanitize())
	err := c.Pool.QueryRow(ctx, query, payload).Scan(&id)
	return id, err
}

// GetAttachment retrieves a payload from the attachment table by ID.
//
// Parameters:
//   - ctx: The context for the operation.
//   - tableName: The name of the attachment table.
//   - id: The attachment ID.
func (c *PostgresClient) GetAttachment(ctx context.Context, tableName string, id int64) ([]byte, error) {
	var payload []byte
	query := fmt.Sprintf("SELECT payload FROM %s WHERE id = $1", pgx.Identifier{tableName}.Sanitize())
	err := c.Pool.QueryRow(ctx, query, id).Scan(&payload)
	return payload, err
}

// CleanupAttachments deletes attachments older than the specified interval.
//
// Parameters:
//   - ctx: The context for the operation.
//   - tableName: The name of the attachment table.
//   - cleanupIntervalMs: The age threshold in milliseconds; attachments older than this are deleted.
func (c *PostgresClient) CleanupAttachments(ctx context.Context, tableName string, cleanupIntervalMs int64) error {
	query := fmt.Sprintf(
		"DELETE FROM %s WHERE created_at < now() - interval '%d milliseconds'",
		pgx.Identifier{tableName}.Sanitize(),
		cleanupIntervalMs,
	)
	_, err := c.Pool.Exec(ctx, query)
	return err
}

// Close releases the listener connection if it was acquired.
func (c *PostgresClient) Close() {
	c.listenerMu.Lock()
	defer c.listenerMu.Unlock()

	if c.listenerConn != nil {
		_ = c.listenerConn.Close(c.Context)
		c.listenerConn = nil
	}
}
