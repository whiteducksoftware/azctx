package log

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const (
	diagnosticSecret        = "synthetic-secret"
	redactedAssignment      = "token=[REDACTED]"
	redactedOtherAssignment = "password=[REDACTED]"
	redactedLabel           = "token: [REDACTED]"
)

func captureLog(t *testing.T, color bool) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	original, originalLevel := logger, level.Level()
	logger = slog.New(&handler{output: &output, color: color, mutex: &sync.Mutex{}})
	SetVerbose(true)
	t.Cleanup(func() { logger = original; level.Set(originalLevel) })
	return &output
}

func TestRedactionPrecedesStatusStylesAcrossFormatBoundaries(t *testing.T) {
	for _, color := range []bool{false, true} {
		for _, method := range []struct {
			name, prefix string
			emit         func(string, ...any)
		}{
			{"info", "", Info},
			{"success", "✓ ", Success},
			{"warning", "Warning: ", Warn},
			{"error", "Error: ", Error},
			{"debug", "", Debug},
		} {
			t.Run(method.name+map[bool]string{false: "/plain", true: "/color"}[color], func(t *testing.T) {
				output := captureLog(t, color)
				for _, message := range []struct {
					format, want string
					values       []any
				}{
					{"token: %s", redactedLabel, []any{diagnosticSecret}},
					{"token: %s", redactedLabel, []any{Identity(diagnosticSecret)}},
					{"%s%s=%s", redactedOtherAssignment, []any{"pass", "word", diagnosticSecret}},
					{"token=%s%s", redactedAssignment, []any{"synthetic-", "secret"}},
					{"%s", redactedAssignment, []any{"token=" + diagnosticSecret}},
					{"password=" + diagnosticSecret, redactedOtherAssignment, nil},
				} {
					output.Reset()
					method.emit(message.format, message.values...)
					got := ansi.Strip(output.String())
					if got != method.prefix+message.want+"\n" || strings.Contains(output.String(), diagnosticSecret) {
						t.Fatalf("format=%q output=%q stripped=%q", message.format, output, got)
					}
				}
			})
		}
	}
}

func TestRedactionIncludesAttributesAndMessageBoundaries(t *testing.T) {
	for _, color := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "color"}[color], func(t *testing.T) {
			output := captureLog(t, color)
			logger.With("token", diagnosticSecret).Warn("warning", "password", diagnosticSecret)
			logger.Error("token:", "detail", diagnosticSecret)
			logger.WithGroup("request").With("detail", "token="+diagnosticSecret).Info("message")
			want := "Warning: warning token=[REDACTED] password=[REDACTED]\n" +
				"Error: token: [REDACTED]\nmessage request.detail=token=[REDACTED]\n"
			if got := ansi.Strip(output.String()); got != want || strings.Contains(output.String(), diagnosticSecret) {
				t.Fatalf("attribute/boundary redaction: output=%q stripped=%q", output, got)
			}
		})
	}
}

func TestIdentityPartsKeepProseRedactionAndDisplayBoundaries(t *testing.T) {
	for _, color := range []bool{false, true} {
		for _, method := range []struct {
			name, prefix string
			emit         func(...any)
		}{
			{"info", "", InfoParts},
			{"success", "✓ ", SuccessParts},
		} {
			t.Run(method.name+map[bool]string{false: "/plain", true: "/color"}[color], func(t *testing.T) {
				output := captureLog(t, color)
				for _, message := range []struct {
					parts []any
					want  string
				}{
					{[]any{"to", "ken: ", diagnosticSecret}, redactedLabel},
					{[]any{"token: ", Identity(diagnosticSecret)}, redactedLabel},
					{[]any{"token=synthetic-", Identity("secret")}, redactedAssignment},
					{[]any{"ey", Identity("Jsynthetic")}, "[REDACTED]"},
					{[]any{"Name ", Identity("Token"), " (", Identity("11111111-1111-1111-1111-111111111111"), ")"}, "Name Token (11111111-1111-1111-1111-111111111111)"},
					{[]any{Identity("password=" + diagnosticSecret)}, redactedOtherAssignment},
				} {
					output.Reset()
					method.emit(message.parts...)
					if got := ansi.Strip(output.String()); got != method.prefix+message.want+"\n" {
						t.Fatalf("parts=%v output=%q stripped=%q", message.parts, output, got)
					}
				}
			})
		}
	}
}
