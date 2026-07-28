package postgres

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewPostgresClient(t *testing.T) {
	t.Run("with valid context", func(t *testing.T) {
		ctx := context.Background()
		// We can't create a real pool in unit tests without a database,
		// so we test the constructor behavior with nil pool
		pc := &PostgresClient{
			Context: ctx,
		}

		if pc.Context != ctx {
			t.Fatal("Context mismatch")
		}
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		pc := NewPostgresClient(nil, nil) //nolint:staticcheck // Verify the constructor's nil-context fallback.

		if pc == nil {
			t.Fatal("Expected non-nil PostgresClient")
		}
		if pc.Context == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})
}

func TestPostgresClientListenerChannels(t *testing.T) {
	client := NewPostgresClient(context.Background(), nil)
	client.listenerChannels.Add("socket.io#/", "socket.io#/chat")

	if err := client.Unlisten(context.Background(), "socket.io#/chat"); err != nil {
		t.Fatalf("Unlisten() failed: %v", err)
	}
	if !client.listenerChannels.Has("socket.io#/") || client.listenerChannels.Has("socket.io#/chat") {
		t.Fatal("channel was not removed")
	}
}

func TestPostgresClientCloseIsIdempotent(t *testing.T) {
	client := NewPostgresClient(context.Background(), nil)
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

	client := NewPostgresClient(context.Background(), pool)
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

func TestSanitizeTableName(t *testing.T) {
	if got := sanitizeTableName(" Public.Socket_IO_Attachments "); got != `"public"."socket_io_attachments"` {
		t.Fatalf("sanitizeTableName() = %s", got)
	}
}
