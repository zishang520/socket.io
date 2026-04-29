package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

type PrefixSimpleHandler struct {
	w      io.Writer
	mu     sync.RWMutex
	prefix string
	attrs  string // formatted attrs appended after prefix
	group  string // group name appended after prefix
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

func (h *PrefixSimpleHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface requires value receiver
	h.mu.RLock()
	prefix := h.prefix
	attrs := h.attrs
	group := h.group
	h.mu.RUnlock()

	msg := r.Message
	if prefix != "" {
		msg = fmt.Sprintf("%s %s", prefix, msg)
	}
	if group != "" {
		msg = fmt.Sprintf("[%s] %s", group, msg)
	}
	// Append handler-level attrs.
	r.Attrs(func(a slog.Attr) bool {
		attrs += fmt.Sprintf(" %s=%v", a.Key, a.Value)
		return true
	})
	if attrs != "" {
		msg += attrs
	}
	_, err := fmt.Fprintln(h.w, msg)
	return err
}

func (h *PrefixSimpleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.RLock()
	prefix := h.prefix
	existingAttrs := h.attrs
	group := h.group
	h.mu.RUnlock()

	var newAttrs strings.Builder
	newAttrs.WriteString(existingAttrs)
	for _, a := range attrs {
		fmt.Fprintf(&newAttrs, " %s=%v", a.Key, a.Value)
	}
	return &PrefixSimpleHandler{
		w:      h.w,
		prefix: prefix,
		attrs:  newAttrs.String(),
		group:  group,
	}
}

func (h *PrefixSimpleHandler) WithGroup(name string) slog.Handler {
	h.mu.RLock()
	prefix := h.prefix
	attrs := h.attrs
	group := h.group
	h.mu.RUnlock()

	newGroup := group
	if newGroup != "" {
		newGroup += "." + name
	} else {
		newGroup = name
	}
	return &PrefixSimpleHandler{
		w:      h.w,
		prefix: prefix,
		attrs:  attrs,
		group:  newGroup,
	}
}

func (h *PrefixSimpleHandler) SetPrefix(prefix string) {
	h.mu.Lock()
	h.prefix = prefix
	h.mu.Unlock()
}
