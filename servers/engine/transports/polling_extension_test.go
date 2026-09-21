package transports_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Override the complete HTTP write using only the public transport interfaces.
// Pause after writing to exercise the scheduling gap before its callback.
type completedPollingWriter struct {
	transports.Polling
	afterWrite func()
}

func (p *completedPollingWriter) DoWrite(ctx *types.HttpContext, data types.BufferInterface, _ *packet.Options, callback func(error)) {
	body, err := io.ReadAll(data)
	if err == nil {
		ctx.ResponseHeaders().Set("Content-Type", "text/plain; charset=UTF-8")
		ctx.ResponseHeaders().Set("Content-Length", strconv.Itoa(len(body)))
		_, err = ctx.Write(body)
	}
	if p.afterWrite != nil {
		p.afterWrite()
	}
	callback(err)
}

func TestPollingOverrideCompletionPreservesNextRequest(t *testing.T) {
	for _, cancelAfterWrite := range []bool{false, true} {
		name := "active_request_context"
		if cancelAfterWrite {
			name = "request_context_canceled_after_response"
		}
		t.Run(name, func(t *testing.T) {
			requestCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			firstRecorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/?EIO=4", nil).WithContext(requestCtx)
			first := types.NewHttpContext(firstRecorder, request)
			secondRecorder := httptest.NewRecorder()
			second := types.NewHttpContext(secondRecorder, httptest.NewRequest(http.MethodGet, "/?EIO=4", nil))
			resume := make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			defer release()
			written := make(chan struct{})
			transport := &completedPollingWriter{
				Polling: transports.MakePolling(),
				afterWrite: sync.OnceFunc(func() {
					close(written)
					<-resume
				}),
			}
			transport.Prototype(transport)
			transport.Construct(first)
			t.Cleanup(func() {
				release()
				transport.SetWritable(false)
				first.Flush()
				second.Flush()
				transport.OnClose()
			})
			drained := make(chan struct{}, 2)
			_ = transport.On("drain", func(...any) { drained <- struct{}{} })
			transport.OnRequest(first)
			transport.Send([]*packet.Packet{{Type: packet.MESSAGE, Data: types.NewStringBufferString("first")}})
			select {
			case <-written:
			case <-time.After(time.Second):
				t.Fatal("custom polling writer did not complete its response")
			}
			if firstRecorder.Code != http.StatusOK || firstRecorder.Body.String() != "4first" {
				t.Fatalf("first response: status=%d body=%q", firstRecorder.Code, firstRecorder.Body.String())
			}
			if cancelAfterWrite {
				// net/http cancels the request context after ServeHTTP returns,
				// even when the response was completed successfully.
				cancel()
			}
			transport.OnRequest(second)
			if second.IsDone() || !transport.Writable() {
				t.Fatalf("completed response blocked its successor: done=%v status=%d writable=%v", second.IsDone(), secondRecorder.Code, transport.Writable())
			}
			release()
			select {
			case <-drained:
			case <-time.After(time.Second):
				t.Fatal("custom writer did not invoke its delayed completion callback")
			}
			if second.IsDone() || !transport.Writable() {
				t.Fatal("late completion callback retired the successor request")
			}
			transport.Send([]*packet.Packet{{Type: packet.MESSAGE, Data: types.NewStringBufferString("second")}})
			select {
			case <-drained:
			case <-time.After(time.Second):
				t.Fatal("successor request did not receive its response")
			}
			if secondRecorder.Code != http.StatusOK || secondRecorder.Body.String() != "4second" {
				t.Fatalf("successor response: status=%d body=%q", secondRecorder.Code, secondRecorder.Body.String())
			}
		})
	}
}

func TestPollingOverrideDoesNotAdoptCanceledRequestBeforeCloseNotification(t *testing.T) {
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/?EIO=4", nil).WithContext(requestCtx)
	first := types.NewHttpContext(httptest.NewRecorder(), request)
	closeEntered, resume := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	_ = first.Once("close", func(...any) {
		close(closeEntered)
		<-resume
	})
	transport := transports.NewPolling(first)
	secondRecorder := httptest.NewRecorder()
	second := types.NewHttpContext(secondRecorder, httptest.NewRequest(http.MethodGet, "/?EIO=4", nil))
	t.Cleanup(func() {
		release()
		transport.SetWritable(false)
		first.Flush()
		second.Flush()
		transport.OnClose()
	})
	transport.OnRequest(first)
	closeFinished := make(chan struct{})
	_ = first.Once("close", func(...any) { close(closeFinished) })
	cancel()
	select {
	case <-closeEntered:
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not enter its close listener")
	}
	transport.OnRequest(second)
	if !second.IsDone() || secondRecorder.Code != http.StatusBadRequest {
		t.Fatal("a canceled request was mistaken for a successfully completed response")
	}
	release()
	select {
	case <-closeFinished:
	case <-time.After(time.Second):
		t.Fatal("polling cancellation did not finish after releasing the close listener")
	}
	if !transport.Discarded() || transport.Writable() {
		t.Fatal("canceled polling request was not discarded")
	}
}

type pollingWriterOverrideBuilder struct{}

func (*pollingWriterOverrideBuilder) Name() string          { return transports.POLLING }
func (*pollingWriterOverrideBuilder) HandlesUpgrades() bool { return false }
func (*pollingWriterOverrideBuilder) UpgradesTo() []string  { return nil }
func (*pollingWriterOverrideBuilder) New(ctx *types.HttpContext) transports.Transport {
	writer := &completedPollingWriter{Polling: transports.MakePolling()}
	writer.Prototype(writer)
	writer.Construct(ctx)
	return writer
}

type flushedExtensionResponseWriter struct {
	http.ResponseWriter
	flushed chan struct{}
	resume  <-chan struct{}
}

func (w *flushedExtensionResponseWriter) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	w.ResponseWriter.(http.Flusher).Flush()
	close(w.flushed)
	<-w.resume
	return n, err
}

func TestPollingOverrideFlushedResponseAdmitsNextRequest(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetPingInterval(time.Hour)
	opts.SetPingTimeout(time.Hour)
	opts.SetTransports(types.NewSet[transports.TransportCtor](&pollingWriterOverrideBuilder{}))
	server := engine.NewServer(opts)
	t.Cleanup(func() { server.Close() })
	firstContexts := make(chan *types.HttpContext, 1)
	server.Use(func(ctx *types.HttpContext, next func(error)) {
		if ctx.Query().Peek("case") == "first" {
			firstContexts <- ctx
		}
		next(nil)
	})
	connected := make(chan engine.Socket, 1)
	_ = server.Once("connection", func(args ...any) { connected <- args[0].(engine.Socket) })
	handshakeDone := make(chan struct{})
	flushed, resume := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("case") {
		case "first":
			w = &flushedExtensionResponseWriter{ResponseWriter: w, flushed: flushed, resume: resume}
		case "":
			defer close(handshakeDone)
		}
		server.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	firstTransport, secondTransport := &http.Transport{}, &http.Transport{}
	defer firstTransport.CloseIdleConnections()
	defer secondTransport.CloseIdleConnections()
	firstClient := &http.Client{Transport: firstTransport, Timeout: 3 * time.Second}
	secondClient := &http.Client{Transport: secondTransport, Timeout: 3 * time.Second}
	endpoint := httpServer.URL + "/engine.io/?EIO=4&transport=polling"
	response, err := firstClient.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("handshake response: status=%d err=%v", response.StatusCode, readErr)
	}
	<-handshakeDone
	socket := <-connected
	t.Cleanup(func() { socket.Transport().SetWritable(false); socket.Transport().OnClose() })
	ready := make(chan struct{}, 2)
	_ = socket.Transport().On("ready", func(...any) { ready <- struct{}{} })
	transportErrors := make(chan any, 2)
	_ = socket.Transport().On("error", func(args ...any) { transportErrors <- args[0] })
	type result struct {
		status int
		body   string
		length int64
		err    error
	}
	fetch := func(client *http.Client, request *http.Request, resultChannel chan<- result) {
		response, requestErr := client.Do(request)
		if requestErr != nil {
			resultChannel <- result{err: requestErr}
			return
		}
		body, bodyErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		resultChannel <- result{status: response.StatusCode, body: string(body), length: response.ContentLength, err: bodyErr}
	}
	firstRequest, err := http.NewRequest(http.MethodGet, endpoint+"&sid="+socket.Id()+"&case=first", nil)
	if err != nil {
		t.Fatal(err)
	}
	firstResult := make(chan result, 1)
	go fetch(firstClient, firstRequest, firstResult)
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("first HTTP poll was not registered")
	}
	socket.Send(types.NewStringBuffer([]byte("first")), nil, nil)
	var first result
	select {
	case first = <-firstResult:
	case <-time.After(time.Second):
		t.Fatal("client could not consume the first flushed polling response")
	}
	if first.err != nil || first.body != "4first" || first.length != int64(len(first.body)) {
		t.Fatalf("first response not fully consumed with known Content-Length: body=%q length=%d err=%v", first.body, first.length, first.err)
	}
	<-flushed
	firstContext := <-firstContexts
	cancellationHandled := make(chan struct{})
	_ = firstContext.Once("close", func(...any) { close(cancellationHandled) })
	// The response is fully consumed, so the client may close its HTTP
	// connection while the writer has not yet returned to the transport.
	firstTransport.CloseIdleConnections()
	select {
	case <-cancellationHandled:
	case <-time.After(time.Second):
		t.Fatal("closing the consumed response connection did not cancel its request")
	}
	if firstContext.Context().Err() == nil || socket.Transport().Discarded() {
		t.Fatal("request cancellation discarded an already committed polling response")
	}
	drained := make(chan struct{})
	_ = socket.Transport().Once("drain", func(...any) { close(drained) })
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	secondRequest, err := http.NewRequestWithContext(secondCtx, http.MethodGet, endpoint+"&sid="+socket.Id()+"&case=second", nil)
	if err != nil {
		t.Fatal(err)
	}
	secondResult := make(chan result, 1)
	go fetch(secondClient, secondRequest, secondResult)
	select {
	case <-ready:
	case early := <-secondResult:
		t.Fatalf("successor was rejected before registration: %+v", early)
	case <-time.After(time.Second):
		t.Fatal("successor HTTP poll was not registered on its independent connection")
	}
	release()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("first polling response did not finish after releasing its writer")
	}
	socket.Send(types.NewStringBuffer([]byte("second")), nil, nil)
	select {
	case failure := <-transportErrors:
		t.Errorf("legal successor GET lost after first response finalization: transport error=%v; first body fully consumed=%q", failure, first.body)
		cancelSecond()
		<-secondResult
	case response := <-secondResult:
		if response.err != nil || response.body != "4second" {
			t.Errorf("successor response=%q err=%v", response.body, response.err)
		}
	case <-time.After(time.Second):
		t.Error("legal successor GET did not receive the queued message")
		cancelSecond()
		<-secondResult
	}
}
