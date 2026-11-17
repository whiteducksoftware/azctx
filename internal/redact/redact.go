// Package redact removes token-shaped values from diagnostics, not data streams.
package redact

import (
	"bytes"
	"io"
	"strings"
)

const (
	marker = "[REDACTED]"
	// maxWord bounds buffered bytes; longer opaque words are suppressed.
	maxWord = 80
	// maxEncodedSlashes separates slash-containing base64 values from paths:
	// random base64 rarely has more than eight '/' in 80 bytes.
	maxEncodedSlashes = 8
)

// Writer buffers at most one short word. Long words and secret values are
// suppressed as they arrive, including when a token spans process writes.
// A word may span '/' only while it is entirely standard base64, so encoded
// keys are judged whole while ordinary paths are still emitted per segment.
// Call Flush at EOF to emit a final, unterminated safe word.
type Writer struct {
	output    io.Writer
	word      []byte
	pending   bool
	separator bool
	hiding    bool
	quote     byte
	// Visible quotes keep punctuation in ordinary strings from ending a field.
	visibleQuote  byte
	visibleEscape bool
	escaped       bool
	authorization bool
	// depth counts '[' opened after a secret label; every element is a value.
	// The outer '[' is held for one byte: an unquoted first element hides it
	// with the value, so an existing "[REDACTED]" stays unchanged.
	depth     int
	opening   bool
	swallowed bool
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
	for index := 0; index < len(data); index++ {
		if w.hiding {
			index += w.hiddenLength(data[index:])
			if index == len(data) {
				break
			}
		}
		w.consume(&output, data[index])
	}
	if _, err := io.WriteString(w.output, output.String()); err != nil {
		return 0, err
	}
	return len(data), nil
}

// hiddenLength skips a suppressed value in one tight loop; long secrets
// otherwise dominate the per-byte cost. Escapes in visible quotes still need
// consume to update the quote context, even when their bytes are suppressed.
func (w *Writer) hiddenLength(data []byte) int {
	if w.visibleEscape {
		return 0
	}
	for index, value := range data {
		if secretEnds[value] || value == ']' && w.depth > 0 || value == '\\' && w.visibleQuote != 0 {
			return index
		}
	}
	return len(data)
}

var (
	delimiters = byteSet("\"':=,;{}()<>\\")
	secretEnds = byteSet("\"',;{}()<>")
)

// byteSet marks the listed bytes plus ASCII controls and space.
func byteSet(listed string) (set [256]bool) {
	for value := 0; value <= ' '; value++ {
		set[value] = true
	}
	for index := range len(listed) {
		set[listed[index]] = true
	}
	return set
}

func delimiter(value byte) bool { return delimiters[value] }

func secretEnd(value byte) bool { return secretEnds[value] }

func fieldBoundary(value byte) bool {
	return value == ',' || value == ';' || value == '{' || value == '}'
}

func base64Byte(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' ||
		value >= '0' && value <= '9' || value == '+' || value == '/'
}

func base64Word(word []byte) bool {
	for _, value := range word {
		if !base64Byte(value) {
			return false
		}
	}
	return true
}

func (w *Writer) consume(output *strings.Builder, value byte) {
	if w.quote != 0 {
		w.consumeQuoted(output, value)
		return
	}
	escaped := w.visibleEscape
	w.visibleEscape = w.visibleQuote != 0 && !escaped && value == '\\'
	literal := w.literalDelimiter(value, escaped)
	if w.hiding {
		if escaped || !w.valueEnd(value) {
			return
		}
		w.hiding = false
		w.pending = w.pending || w.depth > 0
	}
	if w.consumeBracket(output, value) {
		return
	}
	// Path separators bound ordinary diagnostic words, but must not terminate
	// a value already identified as secret (the hiding branch above) or a
	// word that may still be one standard-base64 value.
	if delimiter(value) || value == '/' && !base64Word(w.word) {
		w.finishWord(output)
		w.consumeDelimiter(output, value, literal)
		return
	}
	if w.pending && w.separator {
		output.WriteString(marker)
		w.pending, w.hiding = false, true
		return
	}
	w.appendWord(output, value)
}

// Apostrophes within unquoted words are prose, not opening quotes. Inspect the
// word and hiding state before either is cleared at the delimiter.
func (w *Writer) literalDelimiter(value byte, escaped bool) bool {
	return escaped || value == '\'' && w.visibleQuote == 0 && (len(w.word) > 0 || w.hiding)
}

func (w *Writer) consumeQuoted(output *strings.Builder, value byte) {
	switch {
	case w.escaped:
		w.escaped = false
	case value == '\\':
		w.escaped = true
	case value == w.quote:
		w.quote = 0
		output.WriteByte(value)
		w.pending = w.pending || w.depth > 0
	}
}

func (w *Writer) valueEnd(value byte) bool {
	return secretEnd(value) || w.depth > 0 && value == ']'
}

// consumeBracket tracks arrays opened after a secret label and reports
// whether value was fully handled.
func (w *Writer) consumeBracket(output *strings.Builder, value byte) bool {
	if w.opening {
		w.openArray(output, value)
	}
	switch {
	case w.depth > 0 && value == ']':
		w.closeArray(output)
	case value == '[' && w.pending && w.separator && w.depth == 0:
		w.opening = true
	case value == '[' && w.pending && w.separator:
		output.WriteByte(value)
		w.depth++
	default:
		return false
	}
	return true
}

// openArray emits the held '[' unless an unquoted first element follows; that
// element is hidden together with its brackets, matching a "[REDACTED]" value.
func (w *Writer) openArray(output *strings.Builder, next byte) {
	w.opening, w.depth = false, 1
	w.swallowed = !delimiter(next) && next != '[' && next != ']'
	if !w.swallowed {
		output.WriteByte('[')
	}
}

func (w *Writer) closeArray(output *strings.Builder) {
	w.depth--
	if w.depth > 0 || !w.swallowed {
		output.WriteByte(']')
	}
	w.pending, w.separator = w.depth > 0, w.depth > 0
	w.swallowed = w.swallowed && w.depth > 0
}

func (w *Writer) appendWord(output *strings.Builder, value byte) {
	if !base64Byte(value) && bytes.IndexByte(w.word, '/') >= 0 {
		w.emitSegments(output)
	}
	w.word = append(w.word, value)
	if len(w.word) < maxWord {
		return
	}
	if bytes.IndexByte(w.word, '/') >= 0 && !encodedSecret(w.word) {
		w.emitFirstSegment(output)
		return
	}
	output.WriteString(marker)
	w.word, w.hiding = w.word[:0], true
}

// encodedSecret reports whether a full slash-containing base64 window looks
// like random key material rather than a path made of letters and slashes.
func encodedSecret(word []byte) bool {
	var upper, lower, digit bool
	for _, value := range word {
		upper = upper || value >= 'A' && value <= 'Z'
		lower = lower || value >= 'a' && value <= 'z'
		digit = digit || value >= '0' && value <= '9'
	}
	return upper && lower && digit && bytes.Count(word, []byte("/")) <= maxEncodedSlashes
}

// emitSegments releases every complete '/'-separated segment exactly as the
// path-splitting rules would have, keeping only the final partial segment.
func (w *Writer) emitSegments(output *strings.Builder) {
	if bytes.IndexByte(w.word, '/') < 0 {
		return
	}
	buffered := string(w.word)
	w.word = w.word[:0]
	for {
		index := strings.IndexByte(buffered, '/')
		if index < 0 {
			break
		}
		w.word = append(w.word, buffered[:index]...)
		w.finishSegment(output)
		w.consumeDelimiter(output, '/', false)
		buffered = buffered[index+1:]
	}
	w.word = append(w.word, buffered...)
}

// emitFirstSegment slides a non-secret 80-byte window past its oldest
// '/'-segment only. Later complete segments may begin a key whose remainder
// is shorter than maxWord, so they stay buffered until a window judges them.
func (w *Writer) emitFirstSegment(output *strings.Builder) {
	index := bytes.IndexByte(w.word, '/')
	if index < 0 {
		return
	}
	var rest [maxWord]byte
	kept := copy(rest[:], w.word[index+1:])
	w.word = w.word[:index]
	w.finishSegment(output)
	w.consumeDelimiter(output, '/', false)
	w.word = append(w.word[:0], rest[:kept]...)
}

func (w *Writer) consumeDelimiter(output *strings.Builder, value byte, literal bool) {
	output.WriteByte(value)
	if literal {
		return
	}
	if w.consumeQuote(output, value) {
		return
	}
	if w.visibleQuote == 0 && w.depth == 0 && fieldBoundary(value) {
		w.pending, w.separator, w.authorization = false, false, false
		return
	}
	if value == '\n' || value == '\r' {
		w.authorization, w.visibleQuote = false, 0
		if w.depth == 0 {
			w.pending, w.separator = false, false
		}
		return
	}
	if !w.pending {
		return
	}
	if value == ':' || value == '=' || value == ' ' || value == '\t' {
		w.separator = true
	}
}

func (w *Writer) consumeQuote(output *strings.Builder, value byte) bool {
	if value != '"' && value != '\'' {
		return false
	}
	switch {
	case value == w.visibleQuote:
		w.visibleQuote = 0
	case w.pending && w.separator:
		w.quote, w.pending = value, false
		output.WriteString(marker)
	case w.visibleQuote == 0:
		w.visibleQuote = value
	}
	return true
}

func (w *Writer) finishWord(output *strings.Builder) {
	w.emitSegments(output)
	w.finishSegment(output)
}

func (w *Writer) finishSegment(output *strings.Builder) {
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
	// A Basic credential is only marked by the Authorization word before it.
	afterAuthorization := w.authorization
	w.authorization = key == "authorization" || key == "proxy-authorization"
	switch key {
	case "bearer", "token", "access_token", "accesstoken", "refresh_token", "refreshtoken",
		"id_token", "idtoken", "client_secret", "clientsecret", "client-secret",
		"password", "federated-token", "federated_token", "sig", "sharedaccesssignature",
		"accountkey", "sharedaccesskey":
		w.pending, w.separator = true, false
	case "basic":
		if afterAuthorization {
			w.pending, w.separator = true, false
		}
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
	if w.opening {
		output.WriteByte('[')
		w.opening = false
	}
	_, err := io.WriteString(w.output, output.String())
	return err
}
