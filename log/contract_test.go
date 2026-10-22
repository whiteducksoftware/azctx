package log_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/log"
)

func TestNoColorWithoutTerminalEvenWhenForced(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("TERM", "xterm-256color")
	for _, noColor := range []string{"", "1"} {
		t.Run(noColor, func(t *testing.T) {
			t.Setenv("NO_COLOR", noColor)
			if noColor == "" {
				if err := os.Unsetenv("NO_COLOR"); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			log.InitLogger(&output)
			log.SetVerbose(false)
			t.Cleanup(func() { log.InitLogger(os.Stderr) })
			log.Warn("warning")
			log.Error("failure")
			if output.String() != "Warning: warning\nError: failure\n" {
				t.Fatalf("redirected diagnostic contains color: %q", output.String())
			}
		})
	}
}

func TestVerboseRedactsTokenShapedDiagnostics(t *testing.T) {
	var output bytes.Buffer
	log.InitLogger(&output)
	log.SetVerbose(true)
	t.Cleanup(func() {
		log.SetVerbose(false)
		log.InitLogger(os.Stderr)
	})
	token := strings.Join([]string{"eyJhbGciOiJub25lIn0", "eyJzdWIiOiJmaXh0dXJlIn0", "signature"}, ".")
	log.Debug("failure access_token=%s %s %s jwt=%s", "synthetic-secret", "Bearer", "opaque-secret", token)
	for _, secret := range []string{"synthetic-secret", "opaque-secret", "eyJhbGci"} {
		if bytes.Contains(output.Bytes(), []byte(secret)) {
			t.Fatalf("verbose leaked %q: %s", secret, output.String())
		}
	}
	if !bytes.Contains(output.Bytes(), []byte("[REDACTED]")) {
		t.Fatalf("redaction marker missing: %s", output.String())
	}
}
