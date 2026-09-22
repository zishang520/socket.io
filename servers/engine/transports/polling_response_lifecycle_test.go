package transports_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type responseLifecyclePollingBuilder struct {
	transports.PollingBuilder
	written chan struct{}
	finish  <-chan struct{}
}

func (b *responseLifecyclePollingBuilder) New(ctx *types.HttpContext) transports.Transport {
	p := &responseLifecyclePolling{Polling: transports.MakePolling(), written: b.written, finish: b.finish}
	p.Prototype(p)
	p.Construct(ctx)
	return p
}

// A complete asynchronous override, including its own headers and HTTP write.
type responseLifecyclePolling struct {
	transports.Polling
	written chan struct{}
	finish  <-chan struct{}
}

func (p *responseLifecyclePolling) DoWrite(ctx *types.HttpContext, data types.BufferInterface, _ *packet.Options, callback func(error)) {
	go func() {
		headers := ctx.ResponseHeaders()
		headers.Set("Content-Type", "text/plain; charset=UTF-8")
		headers.Set("Content-Length", strconv.Itoa(data.Len()))
		p.Emit("headers", headers, ctx)
		_, err := io.Copy(ctx, data)
		if ctx.Query().Peek("case") == "active" {
			close(p.written)
			<-p.finish
		}
		callback(err)
	}()
}

// Keep a failing regression from accessing a recycled HTTP/2 writer.
// The handler-lifetime assertions below still report the early return.
type responseLifecycleWriter struct {
	http.ResponseWriter
	returned <-chan struct{}
	late     atomic.Bool
}

func (w *responseLifecycleWriter) ended() bool {
	select {
	case <-w.returned:
		w.late.Store(true)
		return true
	default:
		return false
	}
}

func (w *responseLifecycleWriter) Header() http.Header {
	if w.ended() {
		return make(http.Header)
	}
	return w.ResponseWriter.Header()
}

func (w *responseLifecycleWriter) WriteHeader(status int) {
	if !w.ended() {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *responseLifecycleWriter) Write(data []byte) (int, error) {
	if w.ended() {
		return 0, types.ErrResponseAlreadyWritten
	}
	return w.ResponseWriter.Write(data)
}

func TestPollingResponsePreparationRetainsHTTPHandler(t *testing.T) {
	for _, protocol := range []string{"HTTP1", "HTTP2"} {
		for _, scenario := range []struct {
			name           string
			async, overlap bool
		}{
			{"builtin/cancel", false, false},
			{"builtin/cancel_with_overlap", false, true},
			{"async_override/cancel", true, false},
			{"async_override/cancel_with_overlap", true, true},
		} {
			t.Run(protocol+"/"+scenario.name, func(t *testing.T) {
				entered := make(chan *types.HttpContext, 1)
				resumeHeaders, finishCallback := make(chan struct{}), make(chan struct{})
				releaseHeaders := sync.OnceFunc(func() { close(resumeHeaders) })
				releaseCallback := sync.OnceFunc(func() { close(finishCallback) })
				written, returned := make(chan struct{}), make(chan struct{})
				wait := func(event <-chan struct{}, description string) {
					t.Helper()
					select {
					case <-event:
					case <-time.After(time.Second):
						t.Fatalf("timed out waiting for %s", description)
					}
				}
				assertHeld := func(stage string) {
					t.Helper()
					select {
					case <-returned:
						t.Errorf("HTTP handler returned during %s", stage)
					case <-time.After(30 * time.Millisecond):
					}
				}
				opts := config.DefaultServerOptions()
				opts.SetPingInterval(time.Hour)
				opts.SetPingTimeout(time.Hour)
				if scenario.async {
					opts.SetTransports(types.NewSet[transports.TransportCtor](&responseLifecyclePollingBuilder{
						written: written, finish: finishCallback,
					}))
				}
				server := engine.NewServer(opts)
				connected := make(chan engine.Socket, 1)
				_ = server.Once("connection", func(args ...any) { connected <- args[0].(engine.Socket) })
				_ = server.On("headers", func(args ...any) {
					ctx := args[1].(*types.HttpContext)
					if ctx.Query().Peek("case") == "active" {
						entered <- ctx
						<-resumeHeaders
						args[0].(*types.ParameterBag).Set("X-Prepared", "true")
					}
				})
				cancellation := make(chan context.CancelFunc, 1)
				responseWriters := make(chan *responseLifecycleWriter, 1)
				network := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Query().Get("case") == "active" {
						requestCtx, cancel := context.WithCancel(r.Context())
						defer cancel()
						defer close(returned)
						cancellation <- cancel
						r = r.WithContext(requestCtx)
						guarded := &responseLifecycleWriter{ResponseWriter: w, returned: returned}
						responseWriters <- guarded
						w = guarded
					}
					server.ServeHTTP(w, r)
				}))
				network.EnableHTTP2 = protocol == "HTTP2"
				network.StartTLS()
				t.Cleanup(func() {
					releaseHeaders()
					releaseCallback()
					server.Close()
					network.Close()
				})
				client := network.Client()
				client.Timeout = 3 * time.Second
				endpoint := network.URL + "/engine.io/?EIO=4&transport=polling"
				fetch := func(url string) (*http.Response, error) {
					response, err := client.Get(url)
					if err == nil {
						_, err = io.Copy(io.Discard, response.Body)
						_ = response.Body.Close()
					}
					return response, err
				}
				first, err := fetch(endpoint)
				if err != nil {
					t.Fatal(err)
				}
				if first.StatusCode != http.StatusOK || (first.ProtoMajor == 2) != (protocol == "HTTP2") {
					t.Fatalf("handshake: status=%d protocol=%s", first.StatusCode, first.Proto)
				}
				socket := <-connected
				closed := make(chan struct{})
				_ = socket.Once("close", func(...any) { close(closed) })
				endpoint += "&sid=" + socket.Id()
				result := make(chan struct{})
				go func() { _, _ = fetch(endpoint + "&case=active"); close(result) }()
				socket.Send(types.NewStringBufferString("payload"), nil, nil)
				var ctx *types.HttpContext
				select {
				case ctx = <-entered:
				case <-time.After(time.Second):
					t.Fatal("polling response did not enter headers preparation")
				}
				(<-cancellation)()
				wait(ctx.Context().Done(), "request cancellation")
				assertHeld("headers preparation after cancellation")
				if ctx.ResponseCommitted() {
					t.Error("headers preparation marked the response committed")
				}
				if scenario.overlap {
					second, err := fetch(endpoint + "&case=overlap")
					if err != nil {
						t.Fatal(err)
					}
					if second.StatusCode != http.StatusBadRequest {
						t.Errorf("overlapping GET: status=%d, want 400", second.StatusCode)
					}
				}
				releaseHeaders()
				if scenario.async {
					wait(written, "asynchronous response write")
					assertHeld("pending asynchronous DoWrite callback")
					releaseCallback()
				}
				wait(returned, "HTTP handler completion")
				wait(result, "canceled request completion")
				// With no overlapping GET, only cancellation can close the session.
				wait(closed, "canceled polling session closure")
				if (<-responseWriters).late.Load() {
					t.Error("polling accessed ResponseWriter after its HTTP handler returned")
				}
			})
		}
	}
}
