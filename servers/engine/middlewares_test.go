package engine

import (
	"errors"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func middlewareContext(t testing.TB) *types.HttpContext {
	t.Helper()
	ctx := types.NewHttpContext(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	t.Cleanup(ctx.Flush)
	return ctx
}

func TestApplyMiddlewaresAsyncOrderAndError(t *testing.T) {
	failure := errors.New("middleware rejected request")
	for _, test := range []struct {
		name string
		err  error
		want []string
	}{
		{"success", nil, []string{"first", "resume", "second", "third", "complete"}},
		{"error", failure, []string{"first", "resume", "second", "complete"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := NewServer(nil)
			ctx := middlewareContext(t)
			events := make(chan string, 5)
			resume, finished := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			t.Cleanup(release)
			server.Use(func(_ *types.HttpContext, next func(error)) {
				events <- "first"
				go func() {
					defer close(finished)
					<-resume
					events <- "resume"
					next(nil)
				}()
			})
			server.Use(func(_ *types.HttpContext, next func(error)) {
				events <- "second"
				next(test.err)
			})
			server.Use(func(_ *types.HttpContext, next func(error)) {
				events <- "third"
				next(nil)
			})
			result := make(chan error, 1)
			server.ApplyMiddlewares(ctx, func(err error) {
				events <- "complete"
				result <- err
			})
			if len(events) != 1 {
				t.Fatal("middleware execution continued before asynchronous next")
			}
			release()
			select {
			case <-finished:
			case <-time.After(2 * time.Second):
				t.Fatal("asynchronous middleware chain did not finish")
			}
			if len(result) != 1 {
				t.Fatal("asynchronous middleware chain did not report its result")
			}
			if err := <-result; err != test.err {
				t.Fatalf("completion error = %v, want %v", err, test.err)
			}
			close(events)
			var got []string
			for event := range events {
				got = append(got, event)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("middleware order = %v, want %v", got, test.want)
			}
		})
	}
}

func TestApplyMiddlewaresKeepsRequestSnapshot(t *testing.T) {
	server := NewServer(nil)
	first := middlewareContext(t)
	continuation := make(chan func(error), 1)
	seen := make(chan string, 3)
	server.Use(func(ctx *types.HttpContext, next func(error)) {
		if ctx == first {
			continuation <- next
			return
		}
		next(nil)
	})
	server.Use(func(_ *types.HttpContext, next func(error)) {
		seen <- "original"
		next(nil)
	})
	completed := make(chan error, 1)
	server.ApplyMiddlewares(first, func(err error) { completed <- err })
	if len(continuation) != 1 {
		t.Fatal("request did not enter its first middleware synchronously")
	}
	next := <-continuation

	// This is a supported live getter mutation, performed while execution is
	// paused and no request is copying the registry. Only later requests see it.
	server.Middlewares()[1] = func(_ *types.HttpContext, next func(error)) {
		seen <- "replacement"
		next(nil)
	}
	server.Use(func(_ *types.HttpContext, next func(error)) {
		seen <- "added"
		next(nil)
	})
	go next(nil)
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("paused middleware request did not finish")
	}
	if len(seen) != 1 {
		t.Fatalf("in-flight request ran %d tail middlewares, want 1", len(seen))
	}
	if got := <-seen; got != "original" {
		t.Fatalf("in-flight request used the changed middleware registry: %q", got)
	}
	server.ApplyMiddlewares(middlewareContext(t), func(err error) { completed <- err })
	if len(completed) != 1 || len(seen) != 2 {
		t.Fatal("next request did not run the replacement and added middleware synchronously")
	}
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"replacement", "added"} {
		if got := <-seen; got != want {
			t.Fatalf("next request middleware = %q, want %q", got, want)
		}
	}
}

func TestApplyMiddlewaresRepeatedNextKeepsItsPosition(t *testing.T) {
	server := NewServer(nil)
	server.Use(func(_ *types.HttpContext, next func(error)) {
		next(nil)
		next(nil)
	})
	tail, completions := 0, 0
	server.Use(func(_ *types.HttpContext, next func(error)) {
		tail++
		next(nil)
	})
	server.ApplyMiddlewares(middlewareContext(t), func(err error) {
		if err != nil {
			t.Fatal(err)
		}
		completions++
	})
	if tail != 2 || completions != 2 {
		t.Fatalf("repeated next ran tail %d times and completed %d times, want 2 each", tail, completions)
	}
}

func BenchmarkApplyMiddlewares(b *testing.B) {
	for _, count := range []int{0, 1, 4, 16} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			server := NewServer(nil)
			for range count {
				server.Use(func(_ *types.HttpContext, next func(error)) { next(nil) })
			}
			ctx := middlewareContext(b)
			completed := func(err error) {
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				server.ApplyMiddlewares(ctx, completed)
			}
		})
	}
}
