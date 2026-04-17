package log

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func createTestRecord(msg string) slog.Record {
	return slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
}

func TestDefaultAndSetDefault(t *testing.T) {
	original := Default()
	if original == nil {
		t.Fatal("Default() returned nil")
	}

	custom := NewLog("custom")
	SetDefault(custom)
	if Default() != custom {
		t.Error("SetDefault did not change default logger")
	}

	// Restore
	SetDefault(original)
	if Default() != original {
		t.Error("Restore original default failed")
	}
}

func TestLogDefaultf(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	logger := NewLog("test")
	logger.SetFlags(0)
	logger.SetOutput(&buf)

	logger.Defaultf("default %s", "message")
	if !strings.Contains(buf.String(), "default message") {
		t.Errorf("Defaultf output = %q, want to contain 'default message'", buf.String())
	}

	buf.Reset()
	logger.Default("default message alias")
	if !strings.Contains(buf.String(), "default message alias") {
		t.Errorf("Default output = %q, want to contain 'default message alias'", buf.String())
	}
}

func TestLogSecondaryf(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	logger := NewLog("test")
	logger.SetFlags(0)
	logger.SetOutput(&buf)

	logger.Secondaryf("secondary %s", "msg")
	if !strings.Contains(buf.String(), "secondary msg") {
		t.Errorf("Secondaryf output = %q, want to contain 'secondary msg'", buf.String())
	}

	buf.Reset()
	logger.Secondary("secondary alias")
	if !strings.Contains(buf.String(), "secondary alias") {
		t.Errorf("Secondary output = %q, want to contain 'secondary alias'", buf.String())
	}
}

func TestLogQuestionf(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	logger := NewLog("test")
	logger.SetFlags(0)
	logger.SetOutput(&buf)

	logger.Questionf("question %s", "msg")
	if !strings.Contains(buf.String(), "question msg") {
		t.Errorf("Questionf output = %q, want to contain 'question msg'", buf.String())
	}

	buf.Reset()
	logger.Question("question alias")
	if !strings.Contains(buf.String(), "question alias") {
		t.Errorf("Question output = %q, want to contain 'question alias'", buf.String())
	}
}

func TestLogSuccessf(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	logger := NewLog("test")
	logger.SetFlags(0)
	logger.SetOutput(&buf)

	logger.Successf("success %s", "formatted")
	if !strings.Contains(buf.String(), "success formatted") {
		t.Errorf("Successf output = %q, want to contain 'success formatted'", buf.String())
	}
}

func TestLogWarningf(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	logger := NewLog("test")
	logger.SetFlags(0)
	logger.SetOutput(&buf)

	logger.Warningf("warning %s", "formatted")
	if !strings.Contains(buf.String(), "warning formatted") {
		t.Errorf("Warningf output = %q, want to contain 'warning formatted'", buf.String())
	}
}

func TestLogDebugf(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	DEBUG.Store(true)
	defer func() { DEBUG.Store(false) }()
	_ = os.Setenv("DEBUG", "test*")
	defer func() { _ = os.Unsetenv("DEBUG") }()

	logger := NewLog("test")
	logger.SetFlags(0)
	logger.SetOutput(&buf)

	logger.Debugf("debug %s", "formatted")
	if !strings.Contains(buf.String(), "debug formatted") {
		t.Errorf("Debugf output = %q, want to contain 'debug formatted'", buf.String())
	}
}

func TestLogPrefixEmpty(t *testing.T) {
	logger := NewLog("")
	if got := logger.Prefix(); got != "" {
		t.Errorf("Prefix() for empty = %q, want empty", got)
	}
}

func TestPrefixSimpleHandler_HandleError(t *testing.T) {
	errWriter := &errorWriter{err: errors.New("write failure")}
	handler := NewPrefixSimpleHandler(errWriter, "[TEST]")

	// Handle should propagate the writer error
	record := createTestRecord("message")
	err := handler.Handle(context.Background(), record)
	// Depending on fmt.Fprintln behavior, the error is from the writer
	if err == nil {
		t.Error("Expected error from Handle when writer fails")
	}
}

func TestPrefixSimpleHandler_WithAttrsPreservesWriter(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "[ORIGINAL]")
	newHandler := handler.WithAttrs(nil)

	record := createTestRecord("test")
	if err := newHandler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if !strings.Contains(buf.String(), "[ORIGINAL]") {
		t.Error("WithAttrs handler should preserve prefix and writer")
	}
}

func TestPrefixSimpleHandler_WithGroupPreservesWriter(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrefixSimpleHandler(&buf, "[ORIGINAL]")
	newHandler := handler.WithGroup("group")

	record := createTestRecord("test")
	if err := newHandler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if !strings.Contains(buf.String(), "[ORIGINAL]") {
		t.Error("WithGroup handler should preserve prefix and writer")
	}
}

type errorWriter struct {
	err error
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	return 0, w.err
}
