package postgres

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func mustNewPostgresClient(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *PostgresClient {
	t.Helper()
	client, err := NewPostgresClient(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNewPostgresClient(t *testing.T) {
	t.Run("with valid context", func(t *testing.T) {
		ctx := context.Background()
		pool := newTestPostgresPool(t)
		pc := mustNewPostgresClient(t, ctx, pool)

		if pc.Context() != ctx {
			t.Fatal("Context mismatch")
		}
		if pc.Pool() != pool {
			t.Fatal("Pool mismatch")
		}
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		pc := mustNewPostgresClient(t, nil, newTestPostgresPool(t)) //nolint:staticcheck // Verify the constructor's nil-context fallback.

		if pc.Context() == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})

	t.Run("requires pool", func(t *testing.T) {
		client, err := NewPostgresClient(context.Background(), nil)
		if client != nil {
			t.Fatal("expected nil PostgresClient")
		}
		if !errors.Is(err, ErrPostgresPoolRequired) {
			t.Fatalf("error = %v, want %v", err, ErrPostgresPoolRequired)
		}
	})

	t.Run("defers custom notification callback rejection to listener use", func(t *testing.T) {
		config, err := pgxpool.ParseConfig("postgres://localhost/socket_io_test")
		if err != nil {
			t.Fatal(err)
		}
		config.ConnConfig.OnNotification = func(*pgconn.PgConn, *pgconn.Notification) {}
		pool, err := pgxpool.NewWithConfig(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		client, err := NewPostgresClient(t.Context(), pool)
		if err != nil {
			t.Fatalf("NewPostgresClient() failed for pool-only use: %v", err)
		}
		t.Cleanup(client.Close)
		if err := client.Listen(t.Context(), "socket.io#/"); !errors.Is(err, ErrPostgresOnNotificationUnsupported) {
			t.Fatalf("Listen() error = %v, want %v", err, ErrPostgresOnNotificationUnsupported)
		}
	})
}

func TestPostgresClientListenerChannels(t *testing.T) {
	client := mustNewPostgresClient(t, context.Background(), newTestPostgresPool(t))
	client.listenerChannels.Add("socket.io#/", "socket.io#/chat")

	if err := client.Unlisten(context.Background(), "socket.io#/chat"); err != nil {
		t.Fatalf("Unlisten() failed: %v", err)
	}
	if !client.listenerChannels.Has("socket.io#/") || client.listenerChannels.Has("socket.io#/chat") {
		t.Fatal("channel was not removed")
	}
}

func TestPostgresClientListenerCommandWaitHonorsContext(t *testing.T) {
	client := mustNewPostgresClient(t, context.Background(), newTestPostgresPool(t))
	client.listenerCommandGate <- struct{}{}
	defer func() { <-client.listenerCommandGate }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.Listen(ctx, "socket.io#/"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Listen() error = %v, want context deadline exceeded", err)
	}
	if client.listenerChannels.Has("socket.io#/") {
		t.Fatal("timed-out LISTEN changed the desired channels")
	}
}

func TestPostgresClientCloseIsIdempotent(t *testing.T) {
	client := mustNewPostgresClient(t, context.Background(), newTestPostgresPool(t))
	client.Close()
	client.Close()

	if err := client.Listen(context.Background(), "socket.io#/"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Listen() after Close() returned %v", err)
	}
}

func TestPostgresClientCloseCancelsListenerAcquire(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://root@127.0.0.1/socket_io_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	dialStarted := make(chan struct{}, 1)
	releaseDial := make(chan struct{})
	config.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
		select {
		case dialStarted <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-releaseDial:
			return nil, context.Canceled
		}
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	client := mustNewPostgresClient(t, context.Background(), pool)
	waitDone := make(chan error, 1)
	go func() {
		_, err := client.WaitForNotification(context.Background())
		waitDone <- err
	}()

	select {
	case <-dialStarted:
	case <-time.After(3 * time.Second):
		close(releaseDial)
		t.Fatal("listener connection was not attempted")
	}

	t.Run("queued wait honors context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			_, err := client.WaitForNotification(ctx)
			result <- err
		}()
		select {
		case err := <-result:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("queued WaitForNotification() returned %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("queued WaitForNotification() ignored its context")
		}
	})

	closeDone := make(chan struct{})
	go func() {
		client.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
	case <-time.After(3 * time.Second):
		close(releaseDial)
		<-closeDone
		t.Fatal("Close() did not cancel the blocked listener connection")
	}
	select {
	case err := <-waitDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitForNotification() returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WaitForNotification() did not return after Close()")
	}
}

func TestPostgresClientContextCancellationClosesClient(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://root@127.0.0.1/socket_io_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	dialStarted := make(chan struct{}, 1)
	config.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
		select {
		case dialStarted <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	ctx, cancel := context.WithCancel(context.Background())
	client := mustNewPostgresClient(t, ctx, pool)
	waitDone := make(chan error, 1)
	go func() {
		_, err := client.WaitForNotification(context.Background())
		waitDone <- err
	}()

	select {
	case <-dialStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("listener connection was not attempted")
	}

	cancel()
	select {
	case err := <-waitDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitForNotification() returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("context cancellation did not close the PostgreSQL client")
	}

	if err := client.Listen(context.Background(), "socket.io#/"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Listen() after context cancellation returned %v", err)
	}
}

func TestSanitizeTableName(t *testing.T) {
	if got := sanitizeTableName(" Public.Socket_IO_Attachments "); got != `"public"."socket_io_attachments"` {
		t.Fatalf("sanitizeTableName() = %s", got)
	}
}
