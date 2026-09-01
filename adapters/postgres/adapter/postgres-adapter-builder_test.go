package adapter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestPostgresAdapterBuilderListenErrorUsesErrorHandler(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()

	client := mustNewPostgresClient(t, context.Background(), pool)
	options := DefaultPostgresAdapterOptions()
	options.SetCleanupInterval(0)
	reportedErrors := make(chan error, 2)
	options.SetErrorHandler(func(err error) {
		reportedErrors <- err
	})

	builder := &PostgresAdapterBuilder{Postgres: client, Opts: options}
	adapterInstance := builder.New(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
	t.Cleanup(adapterInstance.Close)

	select {
	case err := <-reportedErrors:
		if !strings.Contains(err.Error(), "failed to acquire listener connection") {
			t.Fatalf("unexpected listener error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Listen() error was not sent to ErrorHandler")
	}
}

func TestPostgresAdapterBuilderWaitErrorUsesErrorHandler(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()

	client := mustNewPostgresClient(t, context.Background(), pool)
	options := DefaultPostgresAdapterOptions()
	options.SetCleanupInterval(0)
	reportedErrors := make(chan error, 1)
	options.SetErrorHandler(func(err error) {
		reportedErrors <- err
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	builder := &PostgresAdapterBuilder{Postgres: client}
	done := make(chan struct{})
	go func() {
		builder.startListening(ctx, options)
		close(done)
	}()

	select {
	case err := <-reportedErrors:
		if !strings.Contains(err.Error(), "failed to acquire listener connection") {
			t.Fatalf("unexpected listener error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForNotification() error was not sent to ErrorHandler")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("startListening did not stop after cancellation")
	}
}

func TestPostgresAdapterBuilderStartListeningStopsWhenClientClosed(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	client := mustNewPostgresClient(t, context.Background(), pool)
	client.Close()

	options := DefaultPostgresAdapterOptions()
	options.SetCleanupInterval(0)
	reportedErrors := make(chan error, 1)
	options.SetErrorHandler(func(err error) {
		reportedErrors <- err
	})

	done := make(chan struct{})
	builder := &PostgresAdapterBuilder{Postgres: client}
	go func() {
		builder.startListening(t.Context(), options)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("startListening did not stop after the PostgreSQL client was closed")
	}

	select {
	case err := <-reportedErrors:
		t.Fatalf("unexpected retry error: %v", err)
	default:
	}
}

func TestPostgresAdapterBuilderStopsCleanupWithListener(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()

	client := mustNewPostgresClient(t, t.Context(), pool)
	client.Close()
	options := DefaultPostgresAdapterOptions()
	options.SetCleanupInterval(1)
	reportedErrors := make(chan error, 1)
	options.SetErrorHandler(func(err error) {
		reportedErrors <- err
	})

	builder := &PostgresAdapterBuilder{Postgres: client}
	builder.startListening(t.Context(), options)

	select {
	case err := <-reportedErrors:
		t.Fatalf("cleanup continued after the listener stopped: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestPostgresAdapterBuilderSerializesListenerGenerations(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	client := mustNewPostgresClient(t, context.Background(), pool)
	client.Close()
	previousDone := make(chan struct{})
	builder := &PostgresAdapterBuilder{
		Postgres:     client,
		listenerDone: previousDone,
	}

	builder.New(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
	currentDone := builder.listenerDone
	select {
	case <-currentDone:
		t.Fatal("new listener started before the previous generation stopped")
	default:
	}

	close(previousDone)
	select {
	case <-currentDone:
	case <-time.After(time.Second):
		t.Fatal("new listener did not start after the previous generation stopped")
	}
}

func TestPostgresAdapterBuilderClientContextCancellationClosesAdapter(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := mustNewPostgresClient(t, ctx, pool)
	options := DefaultPostgresAdapterOptions()
	options.SetCleanupInterval(0)
	options.SetErrorHandler(func(error) {})
	builder := &PostgresAdapterBuilder{Postgres: client, Opts: options}
	adapterInstance := builder.New(
		socket.NewNamespace(socket.NewServer(nil, nil), "/test"),
	).(*postgresAdapter)
	listenerDone := builder.listenerDone

	cancel()
	select {
	case <-listenerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not stop after PostgreSQL client context cancellation")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		builder.mu.Lock()
		listenerCancel := builder.cancel
		builder.mu.Unlock()
		if adapterInstance.isClosed.Load() && builder.namespaces.Len() == 0 && listenerCancel == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("adapter lifecycle was not cleaned up after PostgreSQL client context cancellation")
}
