// Package log presents stream-aware diagnostics through slog on stderr.
package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"

	"github.com/whiteducksoftware/azctx/internal/redact"
)

var (
	logger = newLogger(os.Stderr)
	level  slog.LevelVar
)

func newLogger(output io.Writer) *slog.Logger {
	color := ColorEnabled(output)
	if color {
		output = colorprofile.NewWriter(output, os.Environ())
	}
	return slog.New(&handler{output: output, color: color, mutex: &sync.Mutex{}})
}

func InitLogger(output io.Writer) { logger = newLogger(output) }
func SetVerbose(verbose bool) {
	level.Set(slog.LevelInfo)
	if verbose {
		level.Set(slog.LevelDebug)
	}
}

func Debug(format string, values ...any)   { logger.Debug(fmt.Sprintf(format, values...)) }
func Info(format string, values ...any)    { status(slog.LevelInfo, format, values...) }
func Success(format string, values ...any) { status(successLevel, format, values...) }
func Warn(format string, values ...any)    { logger.Warn(fmt.Sprintf(format, values...)) }
func Error(format string, values ...any)   { logger.Error(fmt.Sprintf(format, values...)) }

const successLevel = slog.LevelInfo + 1

func status(recordLevel slog.Level, format string, values ...any) {
	if h, ok := logger.Handler().(*handler); ok && h.color {
		values = append([]any(nil), values...)
		for i, value := range values {
			if text, ok := value.(string); ok {
				values[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(text)
			}
		}
	}
	logger.Log(context.Background(), recordLevel, fmt.Sprintf(format, values...))
}

// ColorEnabled is the shared per-stream policy. Forced colors never bypass it.
func ColorEnabled(output io.Writer) bool {
	file, terminal := output.(interface{ Fd() uintptr })
	_, noColor := os.LookupEnv("NO_COLOR")
	return terminal && term.IsTerminal(int(file.Fd())) && !noColor
}

// Styles returns presentation styles for this stream, not the process stdout.
func Styles(output io.Writer) (name, id, tenant, header lipgloss.Style) {
	name, id, tenant, header = lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle()
	if ColorEnabled(output) {
		name = name.Foreground(lipgloss.Color("2"))
		id = id.Foreground(lipgloss.Color("6"))
		tenant = tenant.Faint(true)
		header = header.Bold(true)
	}
	return
}

// Yellow accents a notice without changing its plain text or adding a prefix.
func Yellow(output io.Writer, text string) string {
	if ColorEnabled(output) {
		lines := strings.Split(text, "\n")
		for index, line := range lines {
			lines[index] = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(line)
		}
		return strings.Join(lines, "\n")
	}
	return text
}

type handler struct {
	output io.Writer
	color  bool
	mutex  *sync.Mutex
	attrs  []slog.Attr
	group  string
}

func (*handler) Enabled(_ context.Context, recordLevel slog.Level) bool {
	return recordLevel >= level.Level()
}

func (h *handler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	prefix, color := "", lipgloss.Color("3")
	switch {
	case record.Level >= slog.LevelError:
		prefix, color = "Error: ", lipgloss.Color("1")
	case record.Level >= slog.LevelWarn:
		prefix = "Warning: "
	case record.Level == successLevel:
		prefix, color = "✓ ", lipgloss.Color("2")
	}
	if h.color && prefix != "" {
		prefix = lipgloss.NewStyle().Foreground(color).Render(prefix)
	}
	line.WriteString(prefix)
	message := record.Message
	if h.color && record.Level == slog.LevelWarn {
		message = lipgloss.NewStyle().Foreground(color).Render(message)
	}
	line.WriteString(message)
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
	_, err := fmt.Fprintln(h.output, redact.Text(line.String()))
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
