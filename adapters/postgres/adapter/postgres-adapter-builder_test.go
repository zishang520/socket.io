package adapter

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestPostgresAdapterBuilderNotificationQueues(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://root@127.0.0.1/socket_io_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	dialStarted := make(chan struct{}, 1)
	dialCtx, releaseDial := context.WithCancel(t.Context())
	defer releaseDial()
	config.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
		select {
		case dialStarted <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-dialCtx.Done():
			return nil, net.ErrClosed
		}
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	clientCtx, cancelClient := context.WithCancel(t.Context())
	client := mustNewPostgresClient(t, clientCtx, pool)
	received := make(chan string, 3)
	opts := DefaultPostgresAdapterOptions()
	opts.SetErrorHandler(func(error) { received <- "attachment" })
	server := socket.NewServer(nil, nil)
	slow := NewPostgresAdapter(socket.NewNamespace(server, "/slow"), client, opts).(*postgresAdapter)
	fast := NewPostgresAdapter(socket.NewNamespace(server, "/fast"), client, nil).(*postgresAdapter)
	t.Cleanup(func() {
		cancelClient()
		releaseDial()
		slow.Close()
		fast.Close()
		client.Close()
		pool.Close()
		slow.messages.Close()
		fast.messages.Close()
	})
	for _, instance := range []*postgresAdapter{slow, fast} {
		if err := instance.Nsp().On("probe", func(...any) { received <- instance.Nsp().Name() }); err != nil {
			t.Fatal(err)
		}
	}
	builder := &PostgresAdapterBuilder{Postgres: client}
	builder.namespaces.Store(slow.channel, slow)
	builder.namespaces.Store(fast.channel, fast)
	builder.onNotification(t.Context(), &pgconn.Notification{
		Channel: slow.channel,
		Payload: `{"uid":"node","type":3,"attachmentId":"1"}`,
	})
	select {
	case <-dialStarted:
	case <-time.After(time.Second):
		t.Fatal("attachment query did not start")
	}
	for _, instance := range []*postgresAdapter{slow, fast} {
		builder.onNotification(t.Context(), &pgconn.Notification{
			Channel: instance.channel,
			Payload: `{"uid":"emitter","type":9,"data":{"packet":["probe"]}}`,
		})
	}
	select {
	case got := <-received:
		if got != "/fast" {
			t.Fatalf("received %q while the attachment query was blocked", got)
		}
	case <-time.After(time.Second):
		t.Fatal("slow attachment blocked another namespace")
	}
	releaseDial()
	for _, want := range []string{"attachment", "/slow"} {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("received %q, want %q: namespace delivery was reordered", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("did not receive %q after releasing the attachment query", want)
		}
	}
}

func TestPostgresAdapterBuilderNotificationCanCloseAdapter(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pool, err := pgxpool.New(ctx, "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	client := mustNewPostgresClient(t, ctx, pool)
	t.Cleanup(client.Close)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	a := NewPostgresAdapter(nsp, client, nil).(*postgresAdapter)
	t.Cleanup(a.Close)
	closed := make(chan struct{})
	calls := 0
	if err := nsp.On("stop", func(...any) {
		calls++
		a.Close()
		close(closed)
	}); err != nil {
		t.Fatal(err)
	}
	builder := &PostgresAdapterBuilder{Postgres: client}
	builder.namespaces.Store(a.channel, a)
	notification := &pgconn.Notification{
		Channel: a.channel,
		Payload: `{"uid":"emitter","type":9,"data":{"packet":["stop"]}}`,
	}
	builder.onNotification(t.Context(), notification)
	builder.onNotification(t.Context(), notification)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("notification callback deadlocked while closing its adapter")
	}
	a.messages.Close()
	if calls != 1 {
		t.Fatalf("received %d callbacks, want one before closing", calls)
	}
}

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

func TestPostgresAdapterBuilderCleanupNormalizesOverflow(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	client := mustNewPostgresClient(t, t.Context(), pool)
	t.Cleanup(client.Close)
	builder := &PostgresAdapterBuilder{Postgres: client}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	cleaned := false
	// A closed pool reports each attempted cleanup without requiring a database.
	builder.cleanupLoop(ctx, 1<<63-1, DefaultTableName, func(error) {
		cleaned = true
		cancel()
	})
	if !cleaned {
		t.Fatal("overflowing cleanup interval did not trigger a cleanup query")
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

	cancel()
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
