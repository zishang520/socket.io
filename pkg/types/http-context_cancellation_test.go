package types

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Cancel when construction first observes the parent, before it registers its
// cancellation callback. This exercises cancellation during construction without
// depending on the scheduler or on HttpContext's private state.
type cancelDuringConstructionContext struct {
	context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelDuringConstructionContext) Done() <-chan struct{} {
	done := c.Context.Done()
	c.once.Do(c.cancel)
	return done
}

func TestHttpContextCanceledRequestConstruction(t *testing.T) {
	for _, during := range []bool{false, true} {
		name := "already_canceled"
		if during {
			name = "canceled_during_construction"
		}
		t.Run(name, func(t *testing.T) {
			for range 32 {
				parent, cancel := context.WithCancel(context.Background())
				if during {
					parent = &cancelDuringConstructionContext{Context: parent, cancel: cancel}
				} else {
					cancel()
				}
				request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent)
				ctx := NewHttpContext(httptest.NewRecorder(), request)
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("canceled request did not finalize")
				}
				finished := make(chan struct{})
				go func() {
					ctx.Flush()
					close(finished)
				}()
				select {
				case <-finished:
				case <-time.After(time.Second):
					t.Fatal("Flush blocked after construction with a canceled request")
				}
				if !ctx.IsDone() {
					t.Fatal("canceled request was not marked done")
				}
				if _, err := ctx.Write(nil); !errors.Is(err, ErrResponseAlreadyWritten) {
					t.Fatalf("Write after cancellation = %v, want ErrResponseAlreadyWritten", err)
				}
			}
		})
	}
}

func TestHttpContextFlushBeforeRequestCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent)
	ctx := NewHttpContext(httptest.NewRecorder(), request)
	var cleanups atomic.Int32
	ctx.Cleanup = func() { cleanups.Add(1) }
	closed := make(chan error, 2)
	_ = ctx.On("close", func(args ...any) {
		err, _ := args[0].(error)
		closed <- err
	})
	ctx.Flush()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Flush close error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Flush did not emit close")
	}
	cancel()
	ctx.Flush()
	select {
	case <-closed:
		t.Fatal("request cancellation repeated the close event after Flush")
	case <-time.After(25 * time.Millisecond):
	}
	if got := cleanups.Load(); got != 1 {
		t.Fatalf("Cleanup ran %d times, want 1", got)
	}
}

func TestHttpContextCancellationCleanupCanReenterFlush(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent)
	ctx := NewHttpContext(httptest.NewRecorder(), request)
	finished := make(chan struct{})
	ctx.Cleanup = func() {
		ctx.Flush()
		close(finished)
	}
	closed := make(chan error, 1)
	_ = ctx.Once("close", func(args ...any) {
		err, _ := args[0].(error)
		closed <- err
	})
	// Publish the callback before cancellation can execute it.
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cancellation Cleanup could not reenter Flush")
	}
	select {
	case err := <-closed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request cancellation close error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not emit close")
	}
}
