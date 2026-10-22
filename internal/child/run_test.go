package child_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/child"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestRunRetainsStartupErrors(t *testing.T) {
	path := filepath.Join(testutil.WorkDir(t), "not-an-executable")
	if err := os.WriteFile(path, []byte("not an executable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	status, err := child.Run(context.Background(), path, nil, nil, child.Streams{Stdout: io.Discard, Stderr: io.Discard})
	var pathError *os.PathError
	if status != 0 || !errors.As(err, &pathError) {
		t.Fatalf("startup failure became a raw child exit: status=%d error=%v", status, err)
	}
}
