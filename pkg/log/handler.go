package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

type PrefixSimpleHandler struct {
	w      io.Writer
	mu     sync.RWMutex
	prefix string
}

func NewPrefixSimpleHandler(w io.Writer, prefix string) *PrefixSimpleHandler {
	return &PrefixSimpleHandler{
		w:      w,
		prefix: prefix,
	}
}

func (h *PrefixSimpleHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *PrefixSimpleHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.RLock()
	prefix := h.prefix
	h.mu.RUnlock()

	msg := r.Message
	if prefix != "" {
		msg = fmt.Sprintf("%s %s", prefix, msg)
	}
	_, err := fmt.Fprintln(h.w, msg)
	return err
}

func (h *PrefixSimpleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.RLock()
	prefix := h.prefix
	h.mu.RUnlock()

	return &PrefixSimpleHandler{
		w:      h.w,
		prefix: prefix,
	}
}

func (h *PrefixSimpleHandler) WithGroup(name string) slog.Handler {
	h.mu.RLock()
	prefix := h.prefix
	h.mu.RUnlock()

	return &PrefixSimpleHandler{
		w:      h.w,
		prefix: prefix,
	}
}

func (h *PrefixSimpleHandler) SetPrefix(prefix string) {
	h.mu.Lock()
	h.prefix = prefix
	h.mu.Unlock()
}
