package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

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
	var buf concurrentWriter
	handler := NewPrefixSimpleHandler(&buf, "[CONCURRENT]")

	// Test concurrent writes - just verify no race condition occurs
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
			_ = handler.Handle(context.Background(), record)
		})
	}

	// Wait for all goroutines
	wg.Wait()

	// Just verify some output was written (don't check exact count due to concurrent nature)
	output := buf.String()
	if output == "" {
		t.Error("Expected some output from concurrent writes")
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
