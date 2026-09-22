package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type requestExtensionServer struct {
	engine.Server
	middlewareCalls int
	verifyCalls     int
	verify          func(*types.HttpContext, bool) (*types.CodeMessage, map[string]any)
}

func (s *requestExtensionServer) ApplyMiddlewares(ctx *types.HttpContext, next func(error)) {
	s.middlewareCalls++
	s.Server.ApplyMiddlewares(ctx, next)
}

func (s *requestExtensionServer) Verify(ctx *types.HttpContext, upgrade bool) (*types.CodeMessage, map[string]any) {
	s.verifyCalls++
	if s.verify != nil {
		return s.verify(ctx, upgrade)
	}
	return s.Server.Verify(ctx, upgrade)
}

func newRequestExtensionServer(t *testing.T, opts *config.ServerOptions) *requestExtensionServer {
	t.Helper()
	if opts == nil {
		opts = config.DefaultServerOptions()
	}
	s := &requestExtensionServer{Server: engine.MakeServer()}
	s.Prototype(s)
	s.Construct(opts)
	t.Cleanup(func() { s.Close() })
	return s
}

func runRequestExtension(t *testing.T, s engine.Server, mode string, parent context.Context) *httptest.ResponseRecorder {
	t.Helper()
	transport := "polling"
	if mode == "upgrade" {
		transport = "websocket"
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=4&transport="+transport, nil).WithContext(parent)
	ctx := types.NewHttpContext(recorder, request)
	t.Cleanup(ctx.Flush)
	if mode == "upgrade" {
		s.HandleUpgrade(ctx)
	} else {
		s.HandleRequest(ctx)
	}
	return recorder
}

func TestRequestPrototypeHooks(t *testing.T) {
	for _, mode := range []string{"http", "upgrade"} {
		for _, rejection := range []string{"verification", "middleware"} {
			t.Run(mode+"/"+rejection, func(t *testing.T) {
				const message = "custom verification rejected the request"
				opts := config.DefaultServerOptions()
				opts.SetAllowRequest(func(*types.HttpContext) error { return errors.New(message) })
				s := newRequestExtensionServer(t, opts)
				wantVerify, wantStatus := 1, http.StatusForbidden
				wantCode, wantMessage := engine.FORBIDDEN.Code, message
				if rejection == "middleware" {
					s.Use(func(_ *types.HttpContext, next func(error)) { next(errors.New("middleware rejected")) })
					wantVerify, wantStatus = 0, http.StatusBadRequest
					wantCode, wantMessage = engine.BAD_REQUEST.Code, engine.BAD_REQUEST.Message
				}
				var reported *types.ErrorMessage
				_ = s.Once("connection_error", func(args ...any) { reported = args[0].(*types.ErrorMessage) })
				recorder := runRequestExtension(t, s, mode, context.Background())
				if s.middlewareCalls != 1 || s.verifyCalls != wantVerify {
					t.Errorf("prototype calls: middleware=%d verify=%d; want 1 and %d", s.middlewareCalls, s.verifyCalls, wantVerify)
				}
				if reported == nil || reported.Code != wantCode {
					t.Errorf("connection error = %+v, want code %d", reported, wantCode)
				} else if rejection == "middleware" && reported.Context["name"] != "MIDDLEWARE_FAILURE" {
					t.Errorf("middleware error context = %v", reported.Context)
				}
				if mode == "upgrade" {
					// Preserve the existing pre-upgrade 400/plain-text rejection.
					if recorder.Code != http.StatusBadRequest || recorder.Body.String() != wantMessage {
						t.Errorf("upgrade response=(%d, %q), want (400, %q)", recorder.Code, recorder.Body.String(), wantMessage)
					}
				} else {
					var body types.CodeMessage
					if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != wantStatus || body.Code != wantCode || body.Message != wantMessage {
						t.Errorf("HTTP rejection=(%d, %q), decode error=%v", recorder.Code, recorder.Body.String(), err)
					}
				}
			})
		}
	}
}

func TestRequestFinishedMiddlewareSkipsVerification(t *testing.T) {
	for _, mode := range []string{"http", "upgrade"} {
		for _, finalization := range []string{"cancel", "respond"} {
			t.Run(mode+"/"+finalization, func(t *testing.T) {
				authCalls := 0
				opts := config.DefaultServerOptions()
				opts.SetAllowRequest(func(*types.HttpContext) error { authCalls++; return nil })
				s := newRequestExtensionServer(t, opts)
				parent, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				s.Use(func(ctx *types.HttpContext, next func(error)) {
					if finalization == "cancel" {
						cancel()
						<-ctx.Done()
					} else {
						_, _ = ctx.Write([]byte("handled by middleware"))
					}
					next(nil)
				})
				recorder := runRequestExtension(t, s, mode, parent)
				if s.middlewareCalls != 1 || s.verifyCalls != 0 || authCalls != 0 || s.ClientsCount() != 0 {
					t.Errorf("finished middleware: middleware=%d verify=%d auth=%d clients=%d", s.middlewareCalls, s.verifyCalls, authCalls, s.ClientsCount())
				}
				wantBody := ""
				if finalization == "respond" {
					wantBody = "handled by middleware"
				}
				if recorder.Body.String() != wantBody {
					t.Errorf("finished request response=%q, want %q", recorder.Body.String(), wantBody)
				}
			})
		}
	}
}

func TestRequestVerificationResponseStopsDispatch(t *testing.T) {
	for _, mode := range []string{"http", "upgrade"} {
		t.Run(mode, func(t *testing.T) {
			s := newRequestExtensionServer(t, nil)
			errorsReported := 0
			_ = s.On("connection_error", func(...any) { errorsReported++ })
			s.verify = func(ctx *types.HttpContext, upgrade bool) (*types.CodeMessage, map[string]any) {
				if upgrade != (mode == "upgrade") {
					t.Error("Verify received the wrong request mode")
				}
				_ = ctx.SetStatusCode(http.StatusAccepted)
				_, _ = ctx.Write([]byte("handled by verification"))
				return engine.FORBIDDEN, nil
			}
			recorder := runRequestExtension(t, s, mode, context.Background())
			if s.middlewareCalls != 1 || s.verifyCalls != 1 || errorsReported != 0 || s.ClientsCount() != 0 {
				t.Errorf("completed verification: middleware=%d verify=%d errors=%d clients=%d", s.middlewareCalls, s.verifyCalls, errorsReported, s.ClientsCount())
			}
			if recorder.Code != http.StatusAccepted || recorder.Body.String() != "handled by verification" {
				t.Errorf("verification response changed to (%d, %q)", recorder.Code, recorder.Body.String())
			}
		})
	}
}
