package log

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type resolvedLogValue string

func (resolvedLogValue) LogValue() slog.Value { return slog.StringValue("resolved") }

func TestPrefixSimpleHandlerResolvesLogValuer(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewPrefixSimpleHandler(&output, "prefix"))
	logger.With("saved", resolvedLogValue("raw-saved")).Info("message", "record", resolvedLogValue("raw-record"))
	if got, want := output.String(), "prefix message saved=resolved record=resolved\n"; got != want {
		t.Fatalf("output=%q, want %q", got, want)
	}
}

func TestNewPrefixSimpleHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "[TEST]")

	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
	if handler.w != &buf {
		t.Error("Expected writer to be set correctly")
	}
	if handler.prefix != "[TEST]" {
		t.Errorf("Expected prefix '[TEST]', got %q", handler.prefix)
	}
}

func TestPrefixSimpleHandlerEnabled(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "")

	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected handler to be enabled for all levels")
	}
}

func TestPrefixSimpleHandlerHandle(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "[PREFIX]")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)

	err := handler.Handle(context.Background(), record)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[PREFIX]") {
		t.Errorf("Expected output to contain prefix '[PREFIX]', got %q", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain message 'test message', got %q", output)
	}
}

func TestPrefixSimpleHandlerHandleNoPrefix(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)

	err := handler.Handle(context.Background(), record)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got %q", output)
	}
}

func TestPrefixSimpleHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "[PREFIX]")

	newHandler := handler.WithAttrs([]slog.Attr{
		slog.String("key", "value"),
	})

	// WithAttrs should return a new handler
	if newHandler == handler {
		t.Error("Expected WithAttrs to return a new handler")
	}

	// New handler should have same prefix
	psh := newHandler.(*PrefixSimpleHandler)
	if psh.prefix != "[PREFIX]" {
		t.Errorf("Expected prefix to be preserved, got %q", psh.prefix)
	}
}

func TestPrefixSimpleHandlerWithGroup(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "[PREFIX]")

	newHandler := handler.WithGroup("mygroup")

	// WithGroup should return a new handler
	if newHandler == handler {
		t.Error("Expected WithGroup to return a new handler")
	}

	// New handler should have same prefix
	psh := newHandler.(*PrefixSimpleHandler)
	if psh.prefix != "[PREFIX]" {
		t.Errorf("Expected prefix to be preserved, got %q", psh.prefix)
	}
}

func TestPrefixSimpleHandlerSetPrefix(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "[OLD]")

	// Log with old prefix
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "message 1", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Change prefix
	handler.SetPrefix("[NEW]")

	// Log with new prefix
	record = slog.NewRecord(time.Now(), slog.LevelInfo, "message 2", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "[OLD]") {
		t.Errorf("Expected first line to contain '[OLD]', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "[NEW]") {
		t.Errorf("Expected second line to contain '[NEW]', got %q", lines[1])
	}
}

// concurrentWriter wraps bytes.Buffer with a mutex for thread-safe writes
type concurrentWriter struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (w *concurrentWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *concurrentWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestPrefixSimpleHandlerConcurrent(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "[CONCURRENT]")

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
				if err := handler.Handle(context.Background(), record); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()

	if got := strings.Count(buf.String(), "[CONCURRENT] message\n"); got != 1000 {
		t.Fatalf("wrote %d complete records, want 1000", got)
	}
}

func TestPrefixSimpleHandlerDerivedConcurrent(t *testing.T) {
	var buf bytes.Buffer
	parent := NewPrefixSimpleHandler(&buf, "parent")
	child := parent.WithAttrs([]slog.Attr{slog.Int("id", 1)})
	grouped := child.WithGroup("group")
	handlers := []slog.Handler{parent, child, grouped}
	var wg sync.WaitGroup
	for _, handler := range handlers {
		for range 4 {
			wg.Go(func() {
				for range 100 {
					if err := handler.Handle(context.Background(), createTestRecord("message")); err != nil {
						t.Error(err)
					}
				}
			})
		}
	}
	wg.Wait()
	counts := make(map[string]int)
	for line := range strings.SplitSeq(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		counts[line]++
	}
	for _, line := range []string{"parent message", "parent message id=1", "[group] parent message id=1"} {
		if got := counts[line]; got != 400 {
			t.Errorf("%q: wrote %d complete records, want 400", line, got)
		}
	}
}

func TestPrefixSimpleHandlerDerivedPrefixAndFormatting(t *testing.T) {
	var buf bytes.Buffer
	parent := NewPrefixSimpleHandler(&buf, "original")
	child := parent.WithAttrs([]slog.Attr{slog.String("first", "one")}).(*PrefixSimpleHandler)
	grouped := child.WithGroup("outer").WithGroup("inner").WithAttrs([]slog.Attr{slog.Int("second", 2)})
	parent.SetPrefix("parent")
	child.SetPrefix("child")
	for _, handler := range []slog.Handler{parent, child, grouped} {
		record := createTestRecord("message")
		record.AddAttrs(slog.Bool("record", true))
		if err := handler.Handle(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}
	want := "parent message record=true\nchild message first=one record=true\n[outer.inner] original message first=one second=2 record=true\n"
	if got := buf.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func BenchmarkPrefixSimpleHandler(b *testing.B) {
	for _, attributes := range []bool{false, true} {
		name := "plain"
		if attributes {
			name = "attributes"
		}
		b.Run(name, func(b *testing.B) {
			var handler slog.Handler = NewPrefixSimpleHandler(io.Discard, "engine:server")
			record := createTestRecord("connection opened")
			if attributes {
				handler = handler.WithGroup("transport").WithAttrs([]slog.Attr{slog.String("kind", "webtransport")})
				record.AddAttrs(slog.String("remote", "127.0.0.1"), slog.Int("stream", 3))
			}
			b.ReportAllocs()
			for b.Loop() {
				if err := handler.Handle(context.Background(), record); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestPrefixSimpleHandlerSetPrefixConcurrent(t *testing.T) {
	var buf concurrentWriter
	handler := NewPrefixSimpleHandler(&buf, "[INITIAL]")

	done := make(chan bool)

	// Concurrent handlers
	for range 5 {
		go func() {
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
			_ = handler.Handle(context.Background(), record)
			done <- true
		}()
	}

	// Change prefix concurrently
	go func() {
		handler.SetPrefix("[CHANGED]")
		done <- true
	}()

	// Wait for all
	for range 6 {
		<-done
	}

	// Just verify no race condition occurred (run with -race flag)
	output := buf.String()
	if output == "" {
		t.Error("Expected some output")
	}
}
