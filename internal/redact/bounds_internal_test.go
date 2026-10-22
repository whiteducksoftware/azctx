package redact

import (
	"io"
	"strings"
	"testing"
)

func TestWriterBufferStaysBounded(t *testing.T) {
	inputs := map[string]string{
		"slashed base64":  strings.Repeat("Sy7n+tH3tIcK/eY0", 1<<12),
		"slashed path":    strings.Repeat("segment/", 1<<12),
		"mixed path":      strings.Repeat("Ab1/", 1<<12),
		"long word":       strings.Repeat("Z", 1<<15),
		"password array":  "password: [" + strings.Repeat(`"x/y", `, 1<<12),
		"basic auth":      "Authorization: Basic " + strings.Repeat("c3ludGhldGlj/", 1<<12),
		"short fragments": strings.Repeat("a/b/c/d/e/f/g/h/", 1<<12),
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			writer := NewWriter(io.Discard)
			for start := 0; start < len(input); start += 4093 {
				if _, err := writer.Write([]byte(input[start:min(len(input), start+4093)])); err != nil {
					t.Fatal(err)
				}
				if len(writer.word) > maxWord || cap(writer.word) > 2*maxWord {
					t.Fatalf("word buffer grew to len=%d cap=%d", len(writer.word), cap(writer.word))
				}
			}
		})
	}
}
