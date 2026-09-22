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

func TestHttpContextResponseCommitSurvivesRequestCancellation(t *testing.T) {
	for _, mode := range []string{"write", "flush", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			requestCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := NewHttpContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(requestCtx))
			if ctx.ResponseCommitted() {
				t.Fatal("new context has a committed response")
			}
			switch mode {
			case "write":
				if _, err := ctx.Write(nil); err != nil {
					t.Fatal(err)
				}
			case "flush":
				ctx.Flush()
			}
			// net/http also cancels the request after a successful response.
			cancel()
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("context did not finalize")
			}
			want := mode != "cancel"
			if got := ctx.ResponseCommitted(); got != want {
				t.Fatalf("response committed = %v, want %v", got, want)
			}
			ctx.Flush()
			if got := ctx.ResponseCommitted(); got != want {
				t.Fatalf("repeated Flush changed response committed to %v", got)
			}
		})
	}
}

func TestHttpContextCleanupCanReenterFlush(t *testing.T) {
	ctx := NewHttpContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	ctx.Cleanup = ctx.Flush
	finished := make(chan struct{})
	go func() {
		ctx.Flush()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Cleanup could not reenter Flush")
	}
}

func TestHttpContextConcurrentFlushFinalizesOnce(t *testing.T) {
	ctx := NewHttpContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	const callers = 16
	var cleanups atomic.Int32
	cleanupEntered, cleanupFinished := make(chan struct{}), make(chan struct{})
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	ctx.Cleanup = func() {
		cleanups.Add(1)
		if !ctx.IsDone() {
			t.Error("Cleanup ran before the context was marked done")
		}
		select {
		case <-ctx.Done():
		default:
			t.Error("Cleanup ran before Done was closed")
		}
		close(cleanupEntered)
		<-resume
		close(cleanupFinished)
	}
	closed := make(chan struct{}, callers)
	_ = ctx.On("close", func(...any) {
		select {
		case <-cleanupFinished:
		default:
			t.Error("close event ran before Cleanup completed")
		}
		closed <- struct{}{}
	})

	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Go(func() {
			<-start
			ctx.Flush()
			if !ctx.IsDone() {
				t.Error("Flush returned before the context was marked done")
			}
			select {
			case <-ctx.Done():
			default:
				t.Error("Flush returned before Done was closed")
			}
		})
	}
	close(start)
	select {
	case <-cleanupEntered:
	case <-time.After(time.Second):
		t.Fatal("concurrent Flush did not begin Cleanup")
	}
	release()
	workers.Wait()
	if count := cleanups.Load(); count != 1 {
		t.Fatalf("Cleanup ran %d times, want 1", count)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Flush did not emit close")
	}
	ctx.Flush()
	select {
	case <-closed:
		t.Fatal("Flush emitted close more than once")
	case <-time.After(25 * time.Millisecond):
	}
}

type blockedHttpContextWriter struct {
	http.ResponseWriter
	entered chan struct{}
	resume  <-chan struct{}
	err     error
}

func (w *blockedHttpContextWriter) Write([]byte) (int, error) {
	close(w.entered)
	<-w.resume
	return 0, w.err
}

func TestHttpContextFinalizationDuringWriteWaitsForWriter(t *testing.T) {
	for _, fails := range []bool{false, true} {
		name := "success"
		var writeErr error
		if fails {
			name = "write_error"
			writeErr = errors.New("response write failed")
		}
		for _, finalizer := range []string{"cancel", "flush", "cancel_then_flush"} {
			t.Run(name+"/"+finalizer, func(t *testing.T) {
				parent, cancel := context.WithCancel(context.Background())
				defer cancel()
				resume := make(chan struct{})
				release := sync.OnceFunc(func() { close(resume) })
				defer release()
				writer := &blockedHttpContextWriter{
					ResponseWriter: httptest.NewRecorder(), entered: make(chan struct{}),
					resume: resume, err: writeErr,
				}
				ctx := NewHttpContext(writer, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent))
				var cleanups atomic.Int32
				ctx.Cleanup = func() { cleanups.Add(1) }
				closed := make(chan struct{})
				_ = ctx.Once("close", func(...any) { close(closed) })
				written := make(chan error, 1)
				go func() { _, err := ctx.Write([]byte("response")); written <- err }()
				select {
				case <-writer.entered:
				case <-time.After(time.Second):
					t.Fatal("Write did not reach the response writer")
				}
				finalized := make(chan struct{})
				go func() {
					if finalizer != "flush" {
						cancel()
					}
					if finalizer != "cancel" {
						ctx.Flush()
					}
					close(finalized)
				}()
				select {
				case <-finalized:
				case <-time.After(time.Second):
					t.Fatal("finalization blocked on the active writer")
				}
				select {
				case <-ctx.Done():
					t.Fatal("finalization released a response whose writer was still active")
				case <-time.After(30 * time.Millisecond):
				}
				if cleanups.Load() != 0 || !ctx.ResponseCommitted() {
					t.Fatal("finalization cleaned up the active response")
				}
				release()
				select {
				case err := <-written:
					if !errors.Is(err, writeErr) {
						t.Fatalf("Write returned %v, want %v", err, writeErr)
					}
				case <-time.After(time.Second):
					t.Fatal("Write did not finish after releasing its writer")
				}
				select {
				case <-ctx.Done():
				default:
					t.Fatal("finished Write did not finalize its response")
				}
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Fatal("finished Write did not emit close")
				}
				ctx.Flush()
				if cleanups.Load() != 1 {
					t.Fatal("response cleanup did not run exactly once")
				}
			})
		}
	}
}

func TestHttpContextWriteClaimAndFailureFinalization(t *testing.T) {
	writeErr := errors.New("test response write failed")
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	writer := &blockedHttpContextWriter{
		ResponseWriter: httptest.NewRecorder(),
		entered:        make(chan struct{}),
		resume:         resume,
		err:            writeErr,
	}
	ctx := NewHttpContext(writer, httptest.NewRequest(http.MethodGet, "/", nil))
	closed := make(chan error, 1)
	_ = ctx.Once("close", func(args ...any) {
		closeErr, _ := args[0].(error)
		closed <- closeErr
	})
	written := make(chan error, 1)
	go func() {
		_, err := ctx.Write([]byte("response"))
		written <- err
	}()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("Write did not reach the response writer")
	}
	if !ctx.IsDone() || !ctx.ResponseCommitted() {
		t.Error("Write did not claim the response before writing its body")
	}
	select {
	case <-ctx.Done():
		t.Error("Write finalized before the response writer returned")
	default:
	}
	if _, err := ctx.Write(nil); !errors.Is(err, ErrResponseAlreadyWritten) {
		t.Errorf("concurrent Write error = %v, want ErrResponseAlreadyWritten", err)
	}
	release()
	select {
	case err := <-written:
		if !errors.Is(err, writeErr) {
			t.Errorf("Write error = %v, want %v", err, writeErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Write did not return after the response writer failed")
	}
	select {
	case <-ctx.Done():
	default:
		t.Error("failed Write did not finalize the context")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("completed Write close error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("failed Write did not emit close")
	}
}

func TestHttpContextResponsePreparationDelaysFinalization(t *testing.T) {
	for _, finalizer := range []string{"write", "cancel", "flush"} {
		t.Run(finalizer, func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := NewHttpContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent))
			if !ctx.BeginResponse() {
				t.Fatal("new context refused response preparation")
			}
			if ctx.IsDone() || ctx.ResponseCommitted() {
				t.Fatal("response preparation committed the response")
			}
			ctx.ResponseHeaders().Set("X-Prepared", "true")
			closed := make(chan error, 1)
			_ = ctx.Once("close", func(args ...any) {
				err, _ := args[0].(error)
				closed <- err
			})
			switch finalizer {
			case "write":
				if _, err := ctx.Write([]byte("response")); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancel()
			case "flush":
				ctx.Flush()
			}
			if finalizer != "write" && ctx.ResponseCommitted() {
				t.Error("unfinished response preparation was marked committed")
			}
			select {
			case <-ctx.Done():
				t.Error("response finalized before preparation released its writer")
			default:
			}
			ctx.EndResponse()
			select {
			case err := <-closed:
				var want error
				if finalizer == "cancel" {
					want = context.Canceled
				}
				if !errors.Is(err, want) {
					t.Errorf("close error = %v, want %v", err, want)
				}
			case <-time.After(time.Second):
				t.Fatal("response did not finalize after preparation finished")
			}
			if ctx.BeginResponse() {
				ctx.EndResponse()
				t.Fatal("finalized context accepted new response preparation")
			}
		})
	}
}

func TestHttpContextTransportReadPermissionConcurrentAccess(t *testing.T) {
	ctx := NewHttpContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Cleanup(ctx.Flush)
	permissions := []<-chan bool{nil, make(chan bool), make(chan bool)}
	for _, permission := range permissions {
		ctx.SetTransportReadPermission(permission)
		if ctx.TransportReadPermission() != permission {
			t.Fatal("read permission did not preserve the published channel")
		}
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	for worker := range 16 {
		workers.Go(func() {
			<-start
			for iteration := range 100 {
				ctx.SetTransportReadPermission(permissions[(worker+iteration)%len(permissions)])
				permission := ctx.TransportReadPermission()
				if permission != nil && permission != permissions[1] && permission != permissions[2] {
					t.Error("read permission returned an unpublished channel")
				}
			}
		})
	}
	close(start)
	workers.Wait()
}

func TestHttpContextTransportReadyHandoff(t *testing.T) {
	ctx := NewHttpContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Cleanup(ctx.Flush)
	if ctx.TakeTransportReady() != nil {
		t.Fatal("new context unexpectedly has a readiness callback")
	}
	var calls, taken atomic.Int32
	callback := func() { calls.Add(1) }
	ctx.SetTransportReady(callback)
	if calls.Load() != 0 {
		t.Fatal("registering the callback invoked it")
	}
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			<-start
			if ready := ctx.TakeTransportReady(); ready != nil {
				taken.Add(1)
				ready()
			}
		})
	}
	close(start)
	workers.Wait()
	if taken.Load() != 1 || calls.Load() != 1 {
		t.Fatalf("callback taken %d times and called %d times, want 1 each", taken.Load(), calls.Load())
	}

	ctx.SetTransportReady(callback)
	ctx.SetTransportReady(nil)
	if ctx.TakeTransportReady() != nil || calls.Load() != 1 {
		t.Fatal("clearing readiness did not discard the pending callback without invoking it")
	}

	finished := make(chan struct{})
	go func() {
		ctx.SetTransportReady(func() {
			ctx.SetTransportReady(callback)
			if next := ctx.TakeTransportReady(); next != nil {
				next()
			} else {
				t.Error("callback could not register and take the next callback")
			}
		})
		if ready := ctx.TakeTransportReady(); ready != nil {
			ready()
		} else {
			t.Error("readiness callback was lost before invocation")
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("readiness callback could not reenter SetTransportReady/TakeTransportReady")
	}
	if calls.Load() != 2 || ctx.TakeTransportReady() != nil {
		t.Fatal("reentrant callback did not complete and consume its registration")
	}
}
