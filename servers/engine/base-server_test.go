package engine

import (
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestApplyMiddlewaresUsesRequestSnapshot(t *testing.T) {
	server := MakeBaseServer()
	calls := 0
	server.Use(func(_ *types.HttpContext, next func(error)) {
		calls++
		server.Use(func(_ *types.HttpContext, next func(error)) {
			calls++
			next(nil)
		})
		next(nil)
	})

	server.ApplyMiddlewares(nil, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	if calls != 1 {
		t.Fatalf("current request executed %d middlewares, want 1", calls)
	}

	server.ApplyMiddlewares(nil, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	if calls != 3 {
		t.Fatalf("next request produced %d total middleware calls, want 3", calls)
	}
}
