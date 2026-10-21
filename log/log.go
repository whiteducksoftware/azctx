// Package log presents plain diagnostics through slog on stderr.
package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

var (
	logger = newLogger(os.Stderr)
	level  slog.LevelVar
)

func newLogger(output io.Writer) *slog.Logger {
	return slog.New(&handler{output: output, renderer: lipgloss.NewRenderer(output), mutex: &sync.Mutex{}})
}

func InitLogger(output io.Writer) { logger = newLogger(output) }
func SetVerbose(verbose bool) {
	level.Set(slog.LevelInfo)
	if verbose {
		level.Set(slog.LevelDebug)
	}
}

func Debug(format string, values ...any) { logger.Debug(fmt.Sprintf(format, values...)) }
func Info(format string, values ...any)  { logger.Info(fmt.Sprintf(format, values...)) }
func Warn(format string, values ...any)  { logger.Warn(fmt.Sprintf(format, values...)) }
func Error(format string, values ...any) { logger.Error(fmt.Sprintf(format, values...)) }

type handler struct {
	output   io.Writer
	renderer *lipgloss.Renderer
	mutex    *sync.Mutex
	attrs    []slog.Attr
	group    string
}

func (*handler) Enabled(_ context.Context, recordLevel slog.Level) bool {
	return recordLevel >= level.Level()
}

func (h *handler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	switch {
	case record.Level >= slog.LevelError:
		line.WriteString(h.renderer.NewStyle().Foreground(lipgloss.Color("1")).Render("Error: "))
	case record.Level >= slog.LevelWarn:
		line.WriteString(h.renderer.NewStyle().Foreground(lipgloss.Color("3")).Render("Warning: "))
	}
	line.WriteString(record.Message)
	writeAttr := func(attr slog.Attr) bool {
		_, _ = fmt.Fprintf(&line, " %s%s=%s", h.group, attr.Key, attr.Value.Resolve())
		return true
	}
	for _, attr := range h.attrs {
		writeAttr(attr)
	}
	record.Attrs(writeAttr)
	h.mutex.Lock()
	defer h.mutex.Unlock()
	_, err := fmt.Fprintln(h.output, line.String())
	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *handler) WithGroup(name string) slog.Handler {
	clone := *h
	if name != "" {
		clone.group += name + "."
	}
	return &clone
}
