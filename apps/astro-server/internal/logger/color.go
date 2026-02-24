package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// ANSI color codes
const (
	reset   = "\033[0m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	gray    = "\033[90m"

	bold = "\033[1m"
)

// colorHandler is a slog.Handler that outputs colorized text logs.
type colorHandler struct {
	w     io.Writer
	level slog.Leveler
	attrs []slog.Attr
	group string
	mu    *sync.Mutex
}

func newColorHandler(w io.Writer, opts *slog.HandlerOptions) *colorHandler {
	level := opts.Level
	if level == nil {
		level = slog.LevelInfo
	}
	return &colorHandler{
		w:     w,
		level: level,
		mu:    &sync.Mutex{},
	}
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	timeStr := r.Time.Format(time.TimeOnly)

	levelColor, levelLabel := levelStyle(r.Level)
	levelTag := fmt.Sprintf("%s%s%-5s%s", bold, levelColor, levelLabel, reset)

	msg := r.Message

	h.mu.Lock()
	defer h.mu.Unlock()

	// timestamp level message
	_, _ = fmt.Fprintf(h.w, "%s%s%s %s %s", gray, timeStr, reset, levelTag, msg)

	// pre-set attrs from With()
	for _, a := range h.attrs {
		writeAttr(h.w, h.group, a)
	}

	// inline attrs from the log call
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(h.w, h.group, a)
		return true
	})

	_, _ = fmt.Fprintln(h.w)
	return nil
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &colorHandler{
		w:     h.w,
		level: h.level,
		attrs: append(cloneAttrs(h.attrs), attrs...),
		group: h.group,
		mu:    h.mu,
	}
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	g := name
	if h.group != "" {
		g = h.group + "." + name
	}
	return &colorHandler{
		w:     h.w,
		level: h.level,
		attrs: cloneAttrs(h.attrs),
		group: g,
		mu:    h.mu,
	}
}

func writeAttr(w io.Writer, group string, a slog.Attr) {
	key := a.Key
	if group != "" {
		key = group + "." + key
	}
	_, _ = fmt.Fprintf(w, " %s%s%s=%v", cyan, key, reset, a.Value)
}

func levelStyle(l slog.Level) (string, string) {
	switch {
	case l >= slog.LevelError:
		return red, "ERROR"
	case l >= slog.LevelWarn:
		return yellow, "WARN"
	case l >= slog.LevelInfo:
		return green, "INFO"
	default:
		return magenta, "DEBUG"
	}
}

func cloneAttrs(attrs []slog.Attr) []slog.Attr {
	if attrs == nil {
		return nil
	}
	c := make([]slog.Attr, len(attrs))
	copy(c, attrs)
	return c
}

// isTerminal reports whether w is a terminal (used to auto-detect color support).
func isTerminal(w io.Writer) bool {
	type fder interface {
		Fd() uintptr
	}
	f, ok := w.(fder)
	if !ok {
		return false
	}
	// On darwin and linux, os.Stdout.Fd() is 1 and isatty can be checked
	// via the runtime. A simple heuristic: fd 1 or 2 on non-windows is likely a tty.
	fd := f.Fd()
	return fd == 1 || fd == 2 || runtime.GOOS != "windows"
}
