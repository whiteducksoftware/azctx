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

func Debug(format string, values ...any) { logger.Debug(fmt.Sprintf(format, values...)) }
func Info(format string, values ...any)  { logger.Info(fmt.Sprintf(format, values...)) }
func Success(format string, values ...any) {
	logger.Log(context.Background(), successLevel, fmt.Sprintf(format, values...))
}
func Warn(format string, values ...any)  { logger.Warn(fmt.Sprintf(format, values...)) }
func Error(format string, values ...any) { logger.Error(fmt.Sprintf(format, values...)) }

const successLevel = slog.LevelInfo + 1

// Identity is a display value with its own redaction boundary and cyan accent.
type Identity string

// InfoParts joins prose and explicit Identity values without printf formatting.
func InfoParts(parts ...any) { status(slog.LevelInfo, parts) }

// SuccessParts adds the success marker to prose and explicit Identity values.
func SuccessParts(parts ...any) { status(successLevel, parts) }

const statusKey = "azctx.display"

type displayParts []any

func status(recordLevel slog.Level, parts displayParts) {
	logger.LogAttrs(context.Background(), recordLevel, "", slog.Any(statusKey, parts))
}

func (parts displayParts) render(color bool) string {
	var output, prose strings.Builder
	for _, part := range parts {
		if identity, ok := part.(Identity); ok {
			output.WriteString(renderIdentity(prose.String(), identity, color))
			prose.Reset()
		} else {
			_, _ = fmt.Fprint(&prose, part)
		}
	}
	output.WriteString(redact.Text(prose.String()))
	return output.String()
}

func renderIdentity(prose string, identity Identity, color bool) string {
	prefix := redact.Text(prose)
	safe := redact.Text(prose + string(identity))
	// Keep leading secret labels effective, but end their context after this
	// complete display value so a name like Token cannot consume the next ID.
	value, ok := strings.CutPrefix(safe, prefix)
	if !ok {
		return safe
	}
	if color && value != "" {
		value = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(value)
	}
	return prefix + value
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
	var display displayParts
	prefix, color := "", lipgloss.Color("3")
	switch {
	case record.Level >= slog.LevelError:
		prefix, color = "Error: ", lipgloss.Color("1")
	case record.Level >= slog.LevelWarn:
		prefix = "Warning: "
	case record.Level == successLevel:
		prefix, color = "✓ ", lipgloss.Color("2")
	}
	line.WriteString(record.Message)
	writeAttr := func(attr slog.Attr) bool {
		if parts, ok := attr.Value.Any().(displayParts); attr.Key == statusKey && ok {
			display = parts
			return true
		}
		_, _ = fmt.Fprintf(&line, " %s%s=%s", h.group, attr.Key, attr.Value.Resolve())
		return true
	}
	for _, attr := range h.attrs {
		writeAttr(attr)
	}
	record.Attrs(writeAttr)
	message := redact.Text(line.String())
	if display != nil {
		message = display.render(h.color) + message
	}
	if h.color && prefix != "" {
		prefix = lipgloss.NewStyle().Foreground(color).Render(prefix)
	}
	if h.color && record.Level == slog.LevelWarn {
		message = lipgloss.NewStyle().Foreground(color).Render(message)
	}
	h.mutex.Lock()
	defer h.mutex.Unlock()
	_, err := fmt.Fprintln(h.output, prefix+message)
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
