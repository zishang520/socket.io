package types

import (
	"net/http/httptest"
	"testing"
)

func BenchmarkCorsPreflight(b *testing.B) {
	request := httptest.NewRequest("OPTIONS", "/", nil)
	request.Header.Set("Origin", "https://example.com")
	request.Header.Set("Access-Control-Request-Headers", "Content-Type")
	middleware := MiddlewareWrapper(&Cors{Credentials: true, MaxAge: "60"})
	next := func(error) {}
	b.ReportAllocs()
	for b.Loop() {
		middleware(NewHttpContext(httptest.NewRecorder(), request), next)
	}
}
