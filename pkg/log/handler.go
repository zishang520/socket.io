package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

type PrefixSimpleHandler struct {
	w       io.Writer
	writeMu *sync.Mutex // shared by handlers writing to the same output
	mu      sync.RWMutex
	prefix  string
	attrs   string // formatted attrs appended after prefix
	group   string // group name appended after prefix
}

func NewPrefixSimpleHandler(w io.Writer, prefix string) *PrefixSimpleHandler {
	return &PrefixSimpleHandler{
		w:       w,
		writeMu: new(sync.Mutex),
		prefix:  prefix,
	}
}

func (h *PrefixSimpleHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *PrefixSimpleHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface requires value receiver
	h.mu.RLock()
	prefix := h.prefix
	h.mu.RUnlock()

	line := make([]byte, 0, len(prefix)+len(h.group)+len(r.Message)+len(h.attrs)+4)
	if h.group != "" {
		line = append(line, '[')
		line = append(line, h.group...)
		line = append(line, ']', ' ')
	}
	if prefix != "" {
		line = append(line, prefix...)
		line = append(line, ' ')
	}
	line = append(line, r.Message...)
	line = append(line, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		line = appendAttr(line, a)
		return true
	})
	line = append(line, '\n')

	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	_, err := h.w.Write(line)
	return err
}

func (h *PrefixSimpleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	formatted := []byte(h.attrs)
	for _, a := range attrs {
		formatted = appendAttr(formatted, a)
	}
	clone.attrs = string(formatted)
	return clone
}

func (h *PrefixSimpleHandler) WithGroup(name string) slog.Handler {
	clone := h.clone()
	if clone.group != "" {
		clone.group += "." + name
	} else {
		clone.group = name
	}
	return clone
}

// clone snapshots the mutable prefix while retaining the shared output lock.
func (h *PrefixSimpleHandler) clone() *PrefixSimpleHandler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return &PrefixSimpleHandler{
		w:       h.w,
		writeMu: h.writeMu,
		prefix:  h.prefix,
		attrs:   h.attrs,
		group:   h.group,
	}
}

func appendAttr(dst []byte, attr slog.Attr) []byte {
	dst = append(dst, ' ')
	dst = append(dst, attr.Key...)
	dst = append(dst, '=')
	return fmt.Appendf(dst, "%v", attr.Value.Resolve())
}

func (h *PrefixSimpleHandler) SetPrefix(prefix string) {
	h.mu.Lock()
	h.prefix = prefix
	h.mu.Unlock()
}
