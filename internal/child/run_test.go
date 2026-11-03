package child_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/child"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestRunRetainsStartupErrors(t *testing.T) {
	for name, data := range map[string][]byte{
		"text":         []byte("not an executable\n"),
		"truncated-pe": {'M', 'Z', 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(testutil.WorkDir(t), testutil.ExecutableName("not-an-executable"))
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if resolved, err := exec.LookPath(path); err != nil || resolved != path {
				t.Fatalf("fixture must resolve before failing to start: path=%q error=%v", resolved, err)
			}
			status, err := child.Run(context.Background(), path, nil, nil, child.Streams{Stdout: io.Discard, Stderr: io.Discard})
			var pathError *os.PathError
			var exitError *exec.ExitError
			if status != 0 || !errors.As(err, &pathError) || pathError.Path != path || errors.As(err, &exitError) {
				t.Fatalf("startup failure became a raw child exit: status=%d error=%v", status, err)
			}
		})
	}
}
