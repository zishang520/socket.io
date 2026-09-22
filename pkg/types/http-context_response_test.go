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

func TestHttpContextLastResponseRacesFinalization(t *testing.T) {
	for iteration := range 32 {
		parent, cancel := context.WithCancel(context.Background())
		defer cancel()
		ctx := NewHttpContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent))
		if !ctx.BeginResponse() {
			t.Fatal("new context refused response preparation")
		}
		var cleanups, closes atomic.Int32
		var cleanupBeforeDone atomic.Bool
		ctx.Cleanup = func() {
			cleanups.Add(1)
			select {
			case <-ctx.Done():
			default:
				cleanupBeforeDone.Store(true)
			}
		}
		closed := make(chan error, 1)
		_ = ctx.On("close", func(args ...any) {
			closes.Add(1)
			err, _ := args[0].(error)
			select {
			case closed <- err:
			default:
			}
		})
		start := make(chan struct{})
		var workers sync.WaitGroup
		for _, operation := range []func(){ctx.EndResponse, ctx.Flush, cancel} {
			workers.Go(func() {
				<-start
				operation()
			})
		}
		finished := make(chan struct{})
		go func() { workers.Wait(); close(finished) }()
		close(start)
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatalf("iteration %d: response finalizers did not return", iteration)
		}
		select {
		case <-ctx.Done():
		default:
			t.Fatalf("iteration %d: last response did not finalize", iteration)
		}
		select {
		case err := <-closed:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("iteration %d: unexpected close error %v", iteration, err)
			}
			if ctx.ResponseCommitted() != (err == nil) {
				t.Fatalf("iteration %d: response commitment disagrees with close error %v", iteration, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("iteration %d: finalization did not emit close", iteration)
		}
		ctx.Flush()
		if !ctx.IsDone() || cleanupBeforeDone.Load() || cleanups.Load() != 1 || closes.Load() != 1 {
			t.Fatalf("iteration %d: done=%v, cleanup before Done=%v, cleanups=%d, closes=%d",
				iteration, ctx.IsDone(), cleanupBeforeDone.Load(), cleanups.Load(), closes.Load())
		}
	}
}

type reentrantFlushResponseWriter struct {
	http.ResponseWriter
	flush func()
}

func (w *reentrantFlushResponseWriter) Write(data []byte) (int, error) {
	w.flush()
	return w.ResponseWriter.Write(data)
}

func TestHttpContextWriterCanReenterFlush(t *testing.T) {
	writer := &reentrantFlushResponseWriter{ResponseWriter: httptest.NewRecorder()}
	ctx := NewHttpContext(writer, httptest.NewRequest(http.MethodGet, "/", nil))
	finalizedDuringWrite := false
	writer.flush = func() {
		ctx.Flush()
		select {
		case <-ctx.Done():
			finalizedDuringWrite = true
		default:
		}
	}
	cleanups := 0
	ctx.Cleanup = func() { cleanups++ }
	var count int
	var writeErr error
	written := make(chan struct{})
	go func() {
		count, writeErr = ctx.Write([]byte("response"))
		close(written)
	}()
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("response writer could not reenter Flush")
	}
	if writeErr != nil || count != len("response") {
		t.Fatalf("Write returned (%d, %v)", count, writeErr)
	}
	if finalizedDuringWrite || cleanups != 1 {
		t.Fatalf("finalized during Write=%v, cleanups=%d", finalizedDuringWrite, cleanups)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("completed Write did not finalize")
	}
}

type panickingResponseWriter struct {
	http.ResponseWriter
	panicValue any
}

func (w *panickingResponseWriter) Write([]byte) (int, error) {
	panic(w.panicValue)
}

func TestHttpContextWriterPanicFinalizesResponse(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		name := "standalone"
		if scoped {
			name = "preparation_scope"
		}
		t.Run(name, func(t *testing.T) {
			panicValue := errors.New("response writer panic")
			writer := &panickingResponseWriter{ResponseWriter: httptest.NewRecorder(), panicValue: panicValue}
			ctx := NewHttpContext(writer, httptest.NewRequest(http.MethodGet, "/", nil))
			cleanups := 0
			ctx.Cleanup = func() { cleanups++ }
			if scoped && !ctx.BeginResponse() {
				t.Fatal("new context refused response preparation")
			}
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				_, _ = ctx.Write([]byte("response"))
			}()
			if recovered != panicValue {
				t.Fatalf("Write panic = %v, want the original value %v", recovered, panicValue)
			}
			if scoped {
				select {
				case <-ctx.Done():
					t.Error("writer panic finalized an active preparation scope")
				default:
				}
				if cleanups != 0 {
					t.Errorf("Cleanup ran %d times before EndResponse", cleanups)
				}
				ctx.EndResponse()
			}
			select {
			case <-ctx.Done():
			default:
				t.Error("writer panic did not finalize the completed response")
			}
			ctx.Flush()
			if cleanups != 1 {
				t.Errorf("Cleanup ran %d times, want 1", cleanups)
			}
		})
	}
}
