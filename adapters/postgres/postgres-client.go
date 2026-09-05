// Package postgres provides a PostgreSQL LISTEN/NOTIFY client for the Socket.IO adapter.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// ErrPostgresPoolRequired is returned when no PostgreSQL connection pool is provided.
var ErrPostgresPoolRequired = errors.New("postgres: pool is required")

// ErrPostgresOnNotificationUnsupported indicates that a listener callback bypasses pgx's notification buffer.
var ErrPostgresOnNotificationUnsupported = errors.New("postgres: custom OnNotification callbacks are not supported")

// PostgresClient wraps a pgxpool.Pool for the Socket.IO PostgreSQL adapter.
//
// The client supports a separate listener connection for LISTEN/NOTIFY operations.
// The Pool is used for write operations (pg_notify, INSERT, DELETE, etc.)
// and the Listener connection is used for LISTEN operations. The zero value is
// not usable; create clients with NewPostgresClient.
type PostgresClient struct {
	pool *pgxpool.Pool
	ctx  context.Context

	listenerConn        *pgx.Conn
	listenerChannels    *types.Set[string]
	listenerOpCancel    context.CancelFunc
	listenerCommandDone chan struct{}
	listenerCommandGate chan struct{}
	listenerOpGate      chan struct{} // serializes access to listenerConn
	listenerClosed      bool
	stopContextClose    func() bool
	listenerMu          sync.Mutex // guards listener state
}

// Pool returns the connection pool used for PostgreSQL operations.
func (c *PostgresClient) Pool() *pgxpool.Pool {
	return c.pool
}

// Context returns the caller-provided context controlling PostgreSQL operations,
// subscriptions, and adapters built from this client.
func (c *PostgresClient) Context() context.Context {
	return c.ctx
}

// NewPostgresClient creates a new PostgresClient with the given context and connection pool.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of PostgreSQL operations.
//     When canceled, all subscriptions and pending operations will be terminated.
//   - pool: A pgxpool.Pool instance that handles the actual PostgreSQL communication.
//
// Returns:
//   - A pointer to the initialized PostgresClient instance, or an error when
//     the configuration is invalid.
//
// Example:
//
//	pool, _ := pgxpool.New(context.Background(), "postgres://user:pass@localhost:5432/db")
//	pgClient, err := NewPostgresClient(context.Background(), pool)
func NewPostgresClient(ctx context.Context, pool *pgxpool.Pool) (*PostgresClient, error) {
	if pool == nil {
		return nil, ErrPostgresPoolRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client := &PostgresClient{
		pool:                pool,
		ctx:                 ctx,
		listenerChannels:    types.NewSet[string](),
		listenerCommandGate: make(chan struct{}, 1),
		listenerOpGate:      make(chan struct{}, 1),
	}
	client.listenerMu.Lock()
	client.stopContextClose = context.AfterFunc(ctx, client.Close)
	client.listenerMu.Unlock()
	return client, nil
}

func sanitizeTableName(tableName string) string {
	identifiers := strings.Split(tableName, ".")
	for i, identifier := range identifiers {
		identifiers[i] = strings.ToLower(strings.TrimSpace(identifier))
	}
	return pgx.Identifier(identifiers).Sanitize()
}

// Listen subscribes to the specified PostgreSQL notification channels using LISTEN.
// A dedicated connection is used to ensure notifications are not lost.
//
// Parameters:
//   - ctx: The context for the LISTEN operation.
//   - channels: One or more channel names to listen on.
func (c *PostgresClient) Listen(ctx context.Context, channels ...string) error {
	return c.updateListener(ctx, true, channels)
}

// Unlisten unsubscribes from the specified PostgreSQL notification channels using UNLISTEN.
//
// Parameters:
//   - ctx: The context for the UNLISTEN operation.
//   - channels: One or more channel names to unlisten from.
func (c *PostgresClient) Unlisten(ctx context.Context, channels ...string) error {
	return c.updateListener(ctx, false, channels)
}

func (c *PostgresClient) updateListener(ctx context.Context, listen bool, channels []string) error {
	select {
	case c.listenerCommandGate <- struct{}{}:
		defer func() { <-c.listenerCommandGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	c.listenerMu.Lock()
	if c.listenerClosed {
		c.listenerMu.Unlock()
		return context.Canceled
	}
	var changed []string
	for _, channel := range channels {
		hasChannel := c.listenerChannels.Has(channel)
		if listen == hasChannel {
			continue
		}
		if listen {
			c.listenerChannels.Add(channel)
		} else {
			c.listenerChannels.Delete(channel)
		}
		changed = append(changed, channel)
	}
	needsListener := listen && len(channels) > 0 && c.listenerConn == nil
	if len(changed) == 0 && !needsListener {
		c.listenerMu.Unlock()
		return nil
	}
	noChannels := c.listenerChannels.Len() == 0

	c.listenerCommandDone = make(chan struct{})
	if c.listenerOpCancel != nil {
		c.listenerOpCancel()
	}
	c.listenerMu.Unlock()

	defer func() {
		c.listenerMu.Lock()
		close(c.listenerCommandDone)
		c.listenerCommandDone = nil
		c.listenerMu.Unlock()
	}()

	c.listenerOpGate <- struct{}{}
	defer func() { <-c.listenerOpGate }()

	c.listenerMu.Lock()
	if c.listenerClosed {
		c.listenerMu.Unlock()
		return context.Canceled
	}
	opCtx, opCancel := context.WithCancel(ctx)
	c.listenerOpCancel = opCancel
	conn := c.listenerConn
	c.listenerMu.Unlock()
	defer func() {
		opCancel()
		c.listenerMu.Lock()
		c.listenerOpCancel = nil
		c.listenerMu.Unlock()
	}()

	if conn == nil {
		if !listen {
			return nil
		}
		_, err := c.openListener(opCtx)
		return err
	}
	if !listen && noChannels {
		c.discardListener(conn)
		return nil
	}
	if len(changed) == 0 {
		return nil
	}

	command := "UNLISTEN"
	if listen {
		command = "LISTEN"
	}
	for _, channel := range changed {
		if _, err := conn.Exec(opCtx, fmt.Sprintf("%s %s", command, pgx.Identifier{channel}.Sanitize())); err != nil {
			c.discardListener(conn)
			return fmt.Errorf("failed to %s on channel %q: %w", command, channel, err)
		}
	}
	return nil
}

// WaitForNotification waits for a notification on the listener connection.
// This method blocks until a notification is received or the context is canceled.
//
// Returns the received notification or an error if the wait was interrupted.
func (c *PostgresClient) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	for {
		select {
		case c.listenerOpGate <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		// pgx returns buffered notifications even when the context is canceled.
		if err := ctx.Err(); err != nil {
			<-c.listenerOpGate
			return nil, err
		}
		c.listenerMu.Lock()
		if c.listenerClosed {
			c.listenerMu.Unlock()
			<-c.listenerOpGate
			return nil, context.Canceled
		}
		if c.listenerCommandDone != nil {
			done := c.listenerCommandDone
			c.listenerMu.Unlock()
			<-c.listenerOpGate
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-done:
				continue
			}
		}
		opCtx, opCancel := context.WithCancel(ctx)
		c.listenerOpCancel = opCancel
		conn := c.listenerConn
		c.listenerMu.Unlock()

		var notification *pgconn.Notification
		var waitErr error
		if conn == nil {
			openCtx, openCancel := context.WithTimeout(opCtx, DefaultOperationTimeout)
			conn, waitErr = c.openListener(openCtx)
			openCancel()
		}
		if waitErr == nil {
			notification, waitErr = conn.WaitForNotification(opCtx)
			if notification == nil && waitErr == nil {
				waitErr = ErrPostgresOnNotificationUnsupported
			}
		}
		opCancel()

		c.listenerMu.Lock()
		c.listenerOpCancel = nil
		pending := c.listenerCommandDone != nil
		closed := c.listenerClosed
		contextCanceled := ctx.Err() != nil
		discard := conn != nil && waitErr != nil && !pending && !contextCanceled
		if discard && c.listenerConn == conn {
			c.listenerConn = nil
		}
		c.listenerMu.Unlock()
		if discard {
			c.releaseListener(conn)
		}
		<-c.listenerOpGate
		switch {
		case closed:
			return nil, context.Canceled
		case notification != nil:
			return notification, nil
		case pending && !contextCanceled:
			continue
		default:
			return notification, waitErr
		}
	}
}

func (c *PostgresClient) openListener(ctx context.Context) (*pgx.Conn, error) {
	c.listenerMu.Lock()
	if c.listenerClosed {
		c.listenerMu.Unlock()
		return nil, context.Canceled
	}
	channels := c.listenerChannels.Keys()
	c.listenerMu.Unlock()
	if c.pool.Config().ConnConfig.OnNotification != nil {
		return nil, ErrPostgresOnNotificationUnsupported
	}

	pooledConn, err := c.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire listener connection: %w", err)
	}
	conn := pooledConn.Hijack()
	for _, channel := range channels {
		if _, err := conn.Exec(ctx, fmt.Sprintf("LISTEN %s", pgx.Identifier{channel}.Sanitize())); err != nil {
			c.releaseListener(conn)
			return nil, fmt.Errorf("failed to LISTEN on channel %q: %w", channel, err)
		}
	}

	c.listenerMu.Lock()
	if c.listenerClosed {
		c.listenerMu.Unlock()
		c.releaseListener(conn)
		return nil, context.Canceled
	}
	c.listenerConn = conn
	c.listenerMu.Unlock()
	return conn, nil
}

func (c *PostgresClient) discardListener(conn *pgx.Conn) {
	c.listenerMu.Lock()
	if c.listenerConn == conn {
		c.listenerConn = nil
	}
	c.listenerMu.Unlock()
	c.releaseListener(conn)
}

func (*PostgresClient) releaseListener(conn *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultOperationTimeout)
	defer cancel()
	_ = conn.Close(ctx)
}

// Notify sends a NOTIFY on the specified channel with the given payload.
// Uses pg_notify() to send the notification through the connection pool.
//
// Parameters:
//   - ctx: The context for the notification operation.
//   - channel: The notification channel name.
//   - payload: The notification payload string.
func (c *PostgresClient) Notify(ctx context.Context, channel, payload string) error {
	_, err := c.pool.Exec(ctx, "SELECT pg_notify($1, $2)", channel, payload)
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
		sanitizeTableName(tableName),
	)
	_, err := c.pool.Exec(ctx, query)
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
	query := fmt.Sprintf("INSERT INTO %s (payload) VALUES ($1) RETURNING id", sanitizeTableName(tableName))
	err := c.pool.QueryRow(ctx, query, payload).Scan(&id)
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
	query := fmt.Sprintf("SELECT payload FROM %s WHERE id = $1", sanitizeTableName(tableName))
	err := c.pool.QueryRow(ctx, query, id).Scan(&payload)
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
		sanitizeTableName(tableName),
		cleanupIntervalMs,
	)
	_, err := c.pool.Exec(ctx, query)
	return err
}

// Close releases the listener connection if it was acquired. It does not cancel
// Context or close the caller-owned pool; close adapters or cancel Context first.
func (c *PostgresClient) Close() {
	c.listenerMu.Lock()
	if c.listenerClosed {
		c.listenerMu.Unlock()
		return
	}
	c.listenerClosed = true
	stopContextClose := c.stopContextClose
	c.stopContextClose = nil
	if c.listenerOpCancel != nil {
		c.listenerOpCancel()
	}
	c.listenerMu.Unlock()
	if stopContextClose != nil {
		stopContextClose()
	}

	c.listenerOpGate <- struct{}{}
	c.listenerMu.Lock()
	conn := c.listenerConn
	c.listenerConn = nil
	c.listenerMu.Unlock()
	if conn != nil {
		c.releaseListener(conn)
	}
	<-c.listenerOpGate
}
