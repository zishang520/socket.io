//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationClient(t *testing.T) (*PostgresClient, *pgxpool.Pool) {
	return integrationClientWithMaxConns(t, 0)
}

func integrationClientWithMaxConns(t *testing.T, maxConns int32) (*PostgresClient, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("SOCKET_IO_POSTGRES_URL")
	if url == "" {
		t.Skip("SOCKET_IO_POSTGRES_URL is not set")
	}

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig() failed: %v", err)
	}
	if maxConns > 0 {
		config.MaxConns = maxConns
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("pgxpool.New() failed: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("PostgreSQL is unavailable: %v", err)
	}
	client := mustNewPostgresClient(t, context.Background(), pool)
	t.Cleanup(func() {
		client.Close()
		pool.Close()
	})
	return client, pool
}

func TestPostgresClientSingleConnectionPool(t *testing.T) {
	client, _ := integrationClientWithMaxConns(t, 1)
	channel := fmt.Sprintf("socket_io_go_single_%d", time.Now().UnixNano())
	if err := client.Listen(client.Context(), channel); err != nil {
		t.Fatalf("Listen() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(client.Context(), 3*time.Second)
	defer cancel()
	if err := client.Notify(ctx, channel, "single"); err != nil {
		t.Fatalf("Notify() with MaxConns=1 failed: %v", err)
	}
	notification, err := client.WaitForNotification(ctx)
	if err != nil {
		t.Fatalf("WaitForNotification() failed: %v", err)
	}
	if notification.Channel != channel || notification.Payload != "single" {
		t.Fatalf("unexpected notification: %#v", notification)
	}
}

func TestPostgresClientRepeatedListenWhileConnecting(t *testing.T) {
	url := os.Getenv("SOCKET_IO_POSTGRES_URL")
	if url == "" {
		t.Skip("SOCKET_IO_POSTGRES_URL is not set")
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	dial := config.ConnConfig.DialFunc
	dialing := make(chan struct{})
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	var dialCount atomic.Int32
	config.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		if dialCount.Add(1) == 1 {
			close(dialing)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
			}
		}
		return dial(ctx, network, address)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	client := mustNewPostgresClient(t, context.Background(), pool)
	t.Cleanup(func() {
		client.Close()
		pool.Close()
	})

	channel := fmt.Sprintf("socket_io_go_connecting_%d", time.Now().UnixNano())
	client.listenerChannels.Add(channel)
	ctx, cancel := context.WithTimeout(client.Context(), 5*time.Second)
	defer cancel()
	waitResult := make(chan error, 1)
	go func() {
		notification, err := client.WaitForNotification(ctx)
		if err == nil && (notification == nil || notification.Channel != channel) {
			err = fmt.Errorf("unexpected notification: %#v", notification)
		}
		waitResult <- err
	}()
	select {
	case <-dialing:
	case <-ctx.Done():
		t.Fatal("listener did not start connecting")
	}

	listenResult := make(chan error, 1)
	go func() {
		listenResult <- client.Listen(ctx, channel)
	}()
	for {
		client.listenerMu.Lock()
		pending := client.listenerCommandDone != nil
		client.listenerMu.Unlock()
		if pending {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("repeated Listen() did not interrupt the pending wait")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	released = true
	if err := <-listenResult; err != nil {
		t.Fatalf("repeated Listen() failed: %v", err)
	}
	if err := client.Notify(ctx, channel, "connected"); err != nil {
		t.Fatal(err)
	}
	if err := <-waitResult; err != nil {
		t.Fatal(err)
	}
}

func TestPostgresClientDynamicChannels(t *testing.T) {
	client, _ := integrationClient(t)
	channelA := fmt.Sprintf("socket_io_go_a_%d", time.Now().UnixNano())
	channelB := fmt.Sprintf("socket_io_go_b_%d", time.Now().UnixNano())
	if err := client.Listen(client.Context(), channelA); err != nil {
		t.Fatalf("Listen(%q) failed: %v", channelA, err)
	}

	for i := range 20 {
		result := make(chan error, 1)
		go func() {
			notification, err := client.WaitForNotification(client.Context())
			if err == nil && (notification == nil || notification.Channel != channelA) {
				err = fmt.Errorf("unexpected notification: %#v", notification)
			}
			result <- err
		}()
		waitForListenerWait(t, client)

		var err error
		if i%2 == 0 {
			err = client.Listen(client.Context(), channelB)
		} else {
			err = client.Unlisten(client.Context(), channelB)
		}
		if err != nil {
			t.Fatalf("updating second channel failed: %v", err)
		}
		if err := client.Notify(client.Context(), channelA, fmt.Sprintf("%d", i)); err != nil {
			t.Fatalf("Notify() failed: %v", err)
		}

		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("notification timed out")
		}
	}
}

func TestPostgresClientLastUnlistenReleasesListener(t *testing.T) {
	client, _ := integrationClient(t)
	channel := fmt.Sprintf("socket_io_go_unlisten_%d", time.Now().UnixNano())
	if err := client.Listen(client.Context(), channel); err != nil {
		t.Fatalf("Listen() failed: %v", err)
	}
	if err := client.Unlisten(client.Context(), channel); err != nil {
		t.Fatalf("Unlisten() failed: %v", err)
	}

	client.listenerMu.Lock()
	conn := client.listenerConn
	client.listenerMu.Unlock()
	if conn != nil {
		t.Fatal("last Unlisten() did not release the listener connection")
	}
}

func TestPostgresClientConcurrentClose(t *testing.T) {
	client, _ := integrationClient(t)
	channel := fmt.Sprintf("socket_io_go_close_%d", time.Now().UnixNano())
	if err := client.Listen(client.Context(), channel); err != nil {
		t.Fatalf("Listen() failed: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := client.WaitForNotification(client.Context())
		result <- err
	}()
	waitForListenerWait(t, client)
	client.Close()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitForNotification() returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close() did not unblock WaitForNotification()")
	}
}

func TestPostgresClientWaitTimeoutKeepsListener(t *testing.T) {
	client, _ := integrationClient(t)
	channel := fmt.Sprintf("socket_io_go_timeout_%d", time.Now().UnixNano())
	if err := client.Listen(client.Context(), channel); err != nil {
		t.Fatalf("Listen() failed: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(client.Context(), time.Millisecond)
	defer cancel()
	if _, err := client.WaitForNotification(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForNotification() returned %v", err)
	}

	client.listenerMu.Lock()
	conn := client.listenerConn
	client.listenerMu.Unlock()
	if conn == nil {
		t.Fatal("a caller timeout discarded the listener connection")
	}

	if err := client.Notify(client.Context(), channel, "after-timeout"); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel = context.WithTimeout(client.Context(), 3*time.Second)
	defer cancel()
	notification, err := client.WaitForNotification(waitCtx)
	if err != nil {
		t.Fatal(err)
	}
	if notification.Payload != "after-timeout" {
		t.Fatalf("unexpected notification: %#v", notification)
	}
}

func TestPostgresClientCanceledWaitKeepsBufferedNotification(t *testing.T) {
	client, _ := integrationClient(t)
	channel := fmt.Sprintf("socket_io_go_buffered_%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := client.Listen(ctx, channel); err != nil {
		t.Fatal(err)
	}

	// A query after a self-notification makes pgx buffer it before the wait.
	conn := client.listenerConn
	if _, err := conn.Exec(ctx, "SELECT pg_notify($1, $2)", channel, "buffered"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	oldCtx, cancelOld := context.WithCancel(ctx)
	cancelOld()
	if notification, err := client.WaitForNotification(oldCtx); notification != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = (%#v, %v), want (nil, context.Canceled)", notification, err)
	}
	notification, err := client.WaitForNotification(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if notification == nil || notification.Channel != channel || notification.Payload != "buffered" {
		t.Fatalf("buffered notification was lost: %#v", notification)
	}
}

func waitForListenerWait(t *testing.T, client *PostgresClient) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		client.listenerMu.Lock()
		waiting := client.listenerOpCancel != nil
		client.listenerMu.Unlock()
		if waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("listener did not start waiting")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPostgresClientReconnectRestoresChannels(t *testing.T) {
	client, pool := integrationClient(t)
	channel := fmt.Sprintf("socket_io_go_reconnect_%d", time.Now().UnixNano())
	if err := client.Listen(client.Context(), channel); err != nil {
		t.Fatalf("Listen() failed: %v", err)
	}

	client.listenerMu.Lock()
	pid := client.listenerConn.PgConn().PID()
	client.listenerMu.Unlock()
	if _, err := pool.Exec(client.Context(), "SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatalf("terminating listener failed: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(client.Context(), 3*time.Second)
	defer cancel()
	if _, err := client.WaitForNotification(waitCtx); err == nil {
		t.Fatal("expected the terminated listener to return an error")
	}
	if err := client.Listen(client.Context(), channel); err != nil {
		t.Fatalf("repeated Listen() did not reconnect: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		notification, err := client.WaitForNotification(client.Context())
		if err == nil && (notification == nil || notification.Channel != channel) {
			err = fmt.Errorf("unexpected notification: %#v", notification)
		}
		result <- err
	}()

	if err := client.Notify(client.Context(), channel, "restored"); err != nil {
		t.Fatalf("Notify() failed: %v", err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("notification after reconnect timed out")
	}
}
