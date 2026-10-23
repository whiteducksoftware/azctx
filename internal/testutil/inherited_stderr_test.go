package testutil_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestInheritedStderrFixtureLifetime(t *testing.T) {
	for name, explicitStop := range map[string]bool{"explicit-stop": true, "test-cleanup": false} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.WorkDir(t)
			path := filepath.Join(dir, testutil.ExecutableName("az"))
			testutil.Build(t, path, "github.com/whiteducksoftware/azctx/internal/testutil/contextchild")
			t.Setenv("AZCTX_TEST_CHILD_MODE", "az-pipe")
			t.Cleanup(func() {
				if err := os.Remove(path); err != nil {
					t.Errorf("fixture executable still in use after termination: %v", err)
				}
			})
			fixture := testutil.NewInheritedStderr(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, path, "login")
			command.WaitDelay = 50 * time.Millisecond
			var stderr bytes.Buffer
			command.Stderr = &stderr
			err := command.Run()
			if !errors.Is(err, exec.ErrWaitDelay) || command.ProcessState == nil || !command.ProcessState.Success() || strings.TrimSpace(stderr.String()) != "login completed" {
				t.Fatalf("not a real inherited-pipe timeout: error=%v state=%v stderr=%q", err, command.ProcessState, &stderr)
			}
			fixture.AssertHeld(t)
			if explicitStop {
				for range 2 {
					if err := fixture.Stop(); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestInheritedStderrFixtureCleanupWithoutStart(t *testing.T) {
	fixture := testutil.NewInheritedStderr(t)
	if err := fixture.Stop(); err != nil {
		t.Fatal(err)
	}
}
