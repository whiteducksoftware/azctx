package redact_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/redact"
)

func TestStreamingRedactionAcrossEverySplit(t *testing.T) {
	const redactedAccess = "access_token=[REDACTED]\n"
	jwt := strings.Join([]string{"eyJhbGciOiJub25lIn0", "eyJzdWIiOiJmaXh0dXJlIn0", "signature"}, ".")
	tests := []struct{ input, want string }{
		{"Use device code SYNTHETIC\nEnter code: ", "Use device code SYNTHETIC\nEnter code: "},
		{"/synthetic/" + strings.Repeat("segment/", 20) + ".azctx.yaml: line 2", "/synthetic/" + strings.Repeat("segment/", 20) + ".azctx.yaml: line 2"},
		{"access_token=synthetic-secret\n", redactedAccess},
		{`access_token=synthetic:secret=with/padding\suffix` + "\n", redactedAccess},
		{`{"refreshToken":"synthetic secret with spaces"}`, `{"refreshToken":"[REDACTED]"}`},
		{`{"password":"synthetic\"secret\\value"}`, `{"password":"[REDACTED]"}`},
		{"Authorization: " + "Bearer" + " opaque-secret\n", "Authorization: " + "Bearer" + " [REDACTED]\n"},
		{"jwt=" + jwt, "jwt=[REDACTED]"},
		{"token=" + strings.Repeat("x", 1<<20) + "\nready: ", "token=[REDACTED]\nready: "},
		{strings.Repeat("Z", 200) + "\n", "[REDACTED]\n"},
		{redactedAccess, redactedAccess},
	}
	for index, test := range tests {
		t.Run(string(rune('A'+index)), func(t *testing.T) {
			// The long input exercises bounded buffering without quadratic work.
			step := max(1, len(test.input)/64)
			for split := 0; split <= len(test.input); split += step {
				var output bytes.Buffer
				writer := redact.NewWriter(&output)
				for _, part := range []string{test.input[:split], test.input[split:]} {
					if n, err := writer.Write([]byte(part)); err != nil || n != len(part) {
						t.Fatalf("write %d: n=%d error=%v", split, n, err)
					}
				}
				if err := writer.Flush(); err != nil || output.String() != test.want {
					t.Fatalf("split %d: got %q, want %q, error=%v", split, output.String(), test.want, err)
				}
			}
		})
	}
}

func TestStreamingPromptAndWriterFailures(t *testing.T) {
	var output bytes.Buffer
	writer := redact.NewWriter(&output)
	for _, value := range []byte("Enter code: ") {
		if _, err := writer.Write([]byte{value}); err != nil {
			t.Fatal(err)
		}
	}
	if output.String() != "Enter code: " {
		t.Fatalf("prompt was not visible before input: %q", output.String())
	}
	failure := errors.New("writer failed")
	writer = redact.NewWriter(failedWriter{failure})
	if _, err := writer.Write([]byte("message\n")); !errors.Is(err, failure) {
		t.Fatalf("write failure was lost: %v", err)
	}
}

type failedWriter struct{ err error }

func (w failedWriter) Write([]byte) (int, error) { return 0, w.err }
