package adapter

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

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
