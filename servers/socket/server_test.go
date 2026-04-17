package socket

import (
	"testing"
	"time"
)

func TestNewServerNilArgs(t *testing.T) {
	server := NewServer(nil, nil)
	if server == nil {
		t.Fatal("Expected NewServer(nil, nil) to return non-nil")
	}
}

func TestServerSockets(t *testing.T) {
	server := NewServer(nil, nil)
	nsp := server.Sockets()
	if nsp == nil {
		t.Fatal("Expected Sockets() to return non-nil namespace")
	}
	if nsp.Name() != "/" {
		t.Errorf("Expected default namespace name '/', got %q", nsp.Name())
	}
}

func TestServerEncoder(t *testing.T) {
	server := NewServer(nil, nil)
	if server.Encoder() == nil {
		t.Fatal("Expected Encoder() to return non-nil")
	}
}

func TestServerOf(t *testing.T) {
	server := NewServer(nil, nil)

	nsp := server.Of("/chat", nil)
	if nsp == nil {
		t.Fatal("Expected Of() to return non-nil namespace")
	}
	if nsp.Name() != "/chat" {
		t.Errorf("Expected namespace name '/chat', got %q", nsp.Name())
	}

	// Same name should return same namespace
	nsp2 := server.Of("/chat", nil)
	if nsp != nsp2 {
		t.Error("Expected Of() to return same namespace for same name")
	}
}

func TestServerOfMultiple(t *testing.T) {
	server := NewServer(nil, nil)

	server.Of("/chat", nil)
	server.Of("/admin", nil)

	// Default "/" namespace always exists
	if server.Sockets().Name() != "/" {
		t.Error("Expected default namespace '/' to still exist")
	}
}

func TestServerDefaultPath(t *testing.T) {
	server := NewServer(nil, nil)
	if server.Path() != "/socket.io" {
		t.Errorf("Expected default path '/socket.io', got %q", server.Path())
	}
}

func TestServerCustomPath(t *testing.T) {
	opts := DefaultServerOptions()
	opts.SetPath("/custom")
	server := NewServer(nil, opts)

	if server.Path() != "/custom" {
		t.Errorf("Expected path '/custom', got %q", server.Path())
	}
}

func TestServerDefaultConnectTimeout(t *testing.T) {
	server := NewServer(nil, nil)
	if server.ConnectTimeout() != DefaultConnectTimeout {
		t.Errorf("Expected ConnectTimeout %v, got %v", DefaultConnectTimeout, server.ConnectTimeout())
	}
}

func TestServerCustomConnectTimeout(t *testing.T) {
	opts := DefaultServerOptions()
	opts.SetConnectTimeout(10 * time.Second)
	server := NewServer(nil, opts)

	if server.ConnectTimeout() != 10*time.Second {
		t.Errorf("Expected ConnectTimeout 10s, got %v", server.ConnectTimeout())
	}
}

func TestServerDefaultAdapter(t *testing.T) {
	server := NewServer(nil, nil)
	nsp := server.Sockets()
	adapter := nsp.Adapter()
	if adapter == nil {
		t.Fatal("Expected default adapter to be non-nil")
	}
}

func TestServerSessionAwareAdapter(t *testing.T) {
	opts := DefaultServerOptions()
	opts.SetConnectionStateRecovery(DefaultConnectionStateRecovery())
	server := NewServer(nil, opts)

	adapter := server.Sockets().Adapter()
	if adapter == nil {
		t.Fatal("Expected adapter to be non-nil")
	}
	// SessionAwareAdapter embeds regular Adapter, so it should work
	if _, ok := adapter.(SessionAwareAdapter); !ok {
		t.Error("Expected adapter to implement SessionAwareAdapter when ConnectionStateRecovery is set")
	}
}

func TestServerBroadcastDelegation(t *testing.T) {
	server := NewServer(nil, nil)

	t.Run("To returns non-nil", func(t *testing.T) {
		if op := server.To("room1"); op == nil {
			t.Error("Expected To() to return non-nil BroadcastOperator")
		}
	})

	t.Run("In returns non-nil", func(t *testing.T) {
		if op := server.In("room1"); op == nil {
			t.Error("Expected In() to return non-nil BroadcastOperator")
		}
	})

	t.Run("Except returns non-nil", func(t *testing.T) {
		if op := server.Except("room1"); op == nil {
			t.Error("Expected Except() to return non-nil BroadcastOperator")
		}
	})

	t.Run("Compress returns non-nil", func(t *testing.T) {
		if op := server.Compress(true); op == nil {
			t.Error("Expected Compress() to return non-nil BroadcastOperator")
		}
	})

	t.Run("Volatile returns non-nil", func(t *testing.T) {
		if op := server.Volatile(); op == nil {
			t.Error("Expected Volatile() to return non-nil BroadcastOperator")
		}
	})

	t.Run("Local returns non-nil", func(t *testing.T) {
		if op := server.Local(); op == nil {
			t.Error("Expected Local() to return non-nil BroadcastOperator")
		}
	})

	t.Run("Timeout returns non-nil", func(t *testing.T) {
		if op := server.Timeout(5 * time.Second); op == nil {
			t.Error("Expected Timeout() to return non-nil BroadcastOperator")
		}
	})

	t.Run("Emit returns server", func(t *testing.T) {
		if ret := server.Emit("test", "data"); ret != server {
			t.Error("Expected Emit() to return the server itself")
		}
	})

	t.Run("Send returns server", func(t *testing.T) {
		if ret := server.Send("data"); ret != server {
			t.Error("Expected Send() to return the server itself")
		}
	})

	t.Run("Write returns server", func(t *testing.T) {
		if ret := server.Write("data"); ret != server {
			t.Error("Expected Write() to return the server itself")
		}
	})
}

func TestServerUse(t *testing.T) {
	server := NewServer(nil, nil)
	called := false
	ret := server.Use(func(s *Socket, next func(*ExtendedError)) {
		called = true
		next(nil)
	})
	if ret != server {
		t.Error("Expected Use() to return the server itself")
	}
	// Middleware is registered but not called without actual connections
	if called {
		t.Error("Middleware should not be called immediately")
	}
}

func TestServerClose(t *testing.T) {
	server := NewServer(nil, nil)

	var closeErr error
	closeCalled := false
	server.Close(func(err error) {
		closeCalled = true
		closeErr = err
	})

	if !closeCalled {
		t.Error("Expected Close callback to be called")
	}
	if closeErr != nil {
		t.Errorf("Expected no error from Close, got %v", closeErr)
	}
}
