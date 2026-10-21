package cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestCanceledSelectionAndConfigurationNeverMutate(t *testing.T) {
	for _, args := range [][]string{{"alias", "set", "canceled-alias", productionID}, {"config"}, nil, {productionID}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			path := writeSelectionConfig(t, "")
			deps.Select = func([]profile.Subscription) (int, error) {
				t.Fatal("canceled command opened a picker")
				return -1, nil
			}
			root := cmd.NewRootCommand(deps)
			root.SetArgs(args)
			var out, messages bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&messages)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := root.ExecuteContext(ctx)
			if cmd.ExitCode(err) != 130 || len(fake.Calls) != 0 || out.Len() != 0 {
				t.Fatalf("canceled command error=%v, out=%q, calls=%+v", err, out.String(), fake.Calls)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "version: 1\n" {
				t.Fatalf("canceled command wrote configuration: %q, %v", data, err)
			}
		})
	}
}
