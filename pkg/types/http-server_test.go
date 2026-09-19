package types

import (
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHttpServerListenReadyBeforeNotifications(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reserved.Addr().String()
	if err := reserved.Close(); err != nil {
		t.Fatal(err)
	}
	s := NewWebServer(http.NotFoundHandler())
	t.Cleanup(func() { _ = s.Close(nil) })
	notifications := 0
	checkReady := func() {
		notifications++
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Errorf("notification arrived before listener was ready: %v", err)
			return
		}
		_ = conn.Close()
	}
	_ = s.On("listening", func(...any) { checkReady() })
	s.Listen(addr, checkReady)
	if notifications != 2 {
		t.Fatalf("notifications = %d, want listening event and callback", notifications)
	}
}

func TestHttpServerListenFailureDoesNotNotifyReady(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reserved.Close() }()
	s := NewWebServer(nil)
	t.Cleanup(func() { _ = s.Close(nil) })
	notified := false
	_ = s.On("listening", func(...any) { notified = true })
	defer func() {
		value := recover()
		err, ok := value.(error)
		var opErr *net.OpError
		if !ok || !errors.As(err, &opErr) || opErr.Op != "listen" {
			t.Errorf("panic = %v, want listen failure", value)
		}
		if notified {
			t.Error("failed bind reported that the server was ready")
		}
	}()
	s.Listen(reserved.Addr().String(), func() { notified = true })
}
