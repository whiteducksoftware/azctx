// Package redact removes token-shaped values from diagnostics, not data streams.
package redact

import (
	"io"
	"strings"
)

const marker = "[REDACTED]"

// Writer buffers at most one short word. Long words and secret values are
// suppressed as they arrive, including when a token spans process writes.
// Call Flush at EOF to emit a final, unterminated safe word.
type Writer struct {
	output    io.Writer
	word      []byte
	pending   bool
	separator bool
	hiding    bool
	quote     byte
	escaped   bool
}

func NewWriter(output io.Writer) *Writer { return &Writer{output: output} }

func Text(value string) string {
	var output strings.Builder
	writer := NewWriter(&output)
	_, _ = writer.Write([]byte(value))
	_ = writer.Flush()
	return output.String()
}

func (w *Writer) Write(data []byte) (int, error) {
	var output strings.Builder
	for _, value := range data {
		w.consume(&output, value)
	}
	if _, err := io.WriteString(w.output, output.String()); err != nil {
		return 0, err
	}
	return len(data), nil
}

func delimiter(value byte) bool {
	return value <= ' ' || strings.ContainsRune("\"':=,;{}()<>\\", rune(value))
}

func secretEnd(value byte) bool {
	return value <= ' ' || strings.ContainsRune("\"',;{}()<>", rune(value))
}

func (w *Writer) consume(output *strings.Builder, value byte) {
	if w.quote != 0 {
		if w.escaped {
			w.escaped = false
			return
		}
		if value == '\\' {
			w.escaped = true
			return
		}
		if value == w.quote {
			w.quote = 0
			output.WriteByte(value)
		}
		return
	}
	if w.hiding {
		if !secretEnd(value) {
			return
		}
		w.hiding = false
	}
	// Path separators bound ordinary diagnostic words, but must not terminate
	// a value already identified as secret (the hiding branch above).
	if delimiter(value) || value == '/' {
		w.finishWord(output)
		w.consumeDelimiter(output, value)
		return
	}
	if w.pending && w.separator {
		output.WriteString(marker)
		w.pending, w.hiding = false, true
		return
	}
	w.word = append(w.word, value)
	if len(w.word) >= 80 {
		output.WriteString(marker)
		w.word, w.hiding = w.word[:0], true
	}
}

func (w *Writer) consumeDelimiter(output *strings.Builder, value byte) {
	output.WriteByte(value)
	if value == '\n' || value == '\r' {
		w.pending, w.separator = false, false
		return
	}
	if !w.pending {
		return
	}
	if value == ':' || value == '=' || value == ' ' || value == '\t' {
		w.separator = true
	}
	if w.separator && (value == '"' || value == '\'') {
		w.quote, w.pending = value, false
		output.WriteString(marker)
	}
}

func (w *Writer) finishWord(output *strings.Builder) {
	if len(w.word) == 0 {
		return
	}
	word := string(w.word)
	w.word = w.word[:0]
	if tokenShaped(word) {
		output.WriteString(marker)
	} else {
		output.WriteString(word)
	}
	key := strings.ToLower(strings.TrimLeft(word, "-"))
	key = key[strings.LastIndexAny(key, "/?&")+1:]
	switch key {
	case "bearer", "token", "access_token", "accesstoken", "refresh_token", "refreshtoken",
		"id_token", "idtoken", "client_secret", "clientsecret", "client-secret",
		"password", "federated-token", "federated_token", "sig", "sharedaccesssignature":
		w.pending, w.separator = true, false
	}
}

func tokenShaped(word string) bool {
	if strings.Contains(word, "eyJ") {
		return true
	}
	for _, prefix := range []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_", "glpat-", "xoxb-", "xoxp-"} {
		if strings.Contains(word, prefix) {
			return true
		}
	}
	return false
}

func (w *Writer) Flush() error {
	var output strings.Builder
	w.finishWord(&output)
	_, err := io.WriteString(w.output, output.String())
	return err
}
