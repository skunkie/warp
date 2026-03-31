package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/miekg/dns"
)

func Setup(out io.Writer, level slog.Level) {
	slog.SetDefault(slog.New(&colorHandler{out: out, level: level}))
}

func EnableDebug() {
	Setup(os.Stdout, slog.LevelDebug)
}

func Colorize(s string, c int) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", c, s)
}

func DNSAttrs(m *dns.Msg) []any {
	names := make([]string, 0, len(m.Question))
	ips := make([]string, 0, len(m.Answer))

	for _, que := range m.Question {
		names = append(names, que.Name)
	}

	for _, ans := range m.Answer {
		ansA, ok := ans.(*dns.A)
		if !ok {
			continue
		}

		ips = append(ips, ansA.A.String())
	}

	attrs := []any{"names", strings.Join(names, ",")}

	if len(ips) != 0 {
		attrs = append(attrs, "ips", strings.Join(ips, ","))
	}

	return attrs
}

type colorHandler struct {
	out   io.Writer
	level slog.Level
	attrs []slog.Attr
}

func (h *colorHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	ts := Colorize(r.Time.Format("15:04:05"), 7)
	lvl := formatLevel(r.Level)
	msg := r.Message

	var fields []string
	var errStr string

	for _, a := range h.attrs {
		if a.Key == "error" {
			errStr = Colorize(a.Value.String(), 1)
		} else {
			fields = append(fields, Colorize(a.Key+"=", 6)+Colorize(a.Value.String(), 6))
		}
	}

	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "error" {
			errStr = Colorize(a.Value.String(), 1)
		} else {
			fields = append(fields, Colorize(a.Key+"=", 6)+Colorize(a.Value.String(), 6))
		}

		return true
	})

	parts := []string{ts, lvl, msg}

	if errStr != "" {
		parts = append(parts, errStr)
	}

	parts = append(parts, fields...)

	_, err := fmt.Fprintf(h.out, "%s\n", strings.Join(parts, " "))

	return err
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &colorHandler{
		out:   h.out,
		level: h.level,
		attrs: append(h.attrs, attrs...),
	}
}

func (h *colorHandler) WithGroup(_ string) slog.Handler {
	return h
}

func formatLevel(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return Colorize("ERR", 9)
	case l >= slog.LevelWarn:
		return Colorize("WRN", 11)
	case l >= slog.LevelInfo:
		return Colorize("INF", 10)
	default:
		return Colorize("DBG", 10)
	}
}
