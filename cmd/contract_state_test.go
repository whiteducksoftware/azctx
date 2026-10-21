package cmd_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestContractSwitchWithoutDefaultAndCorruptDefault(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
		deps := commandDependencies(t, fake)
		data := strings.ReplaceAll(fixtureProfile, `"isDefault":true`, `"isDefault":false`)
		if corrupt {
			data = strings.ReplaceAll(fixtureProfile, productionID, "corrupt-default-id")
		}
		deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(data)}}
		root := cmd.NewRootCommand(deps)
		root.SetArgs([]string{developmentName})
		err := root.Execute()
		if corrupt {
			if err == nil || !strings.Contains(err.Error(), "save previous subscription") || len(fake.Calls) != 0 {
				t.Fatalf("corrupt default error=%v calls=%+v", err, fake.Calls)
			}
		} else if err != nil || len(fake.Calls) != 1 || fake.Calls[0].Args[3] != developmentID {
			t.Fatalf("no default error=%v calls=%+v", err, fake.Calls)
		}
		if previous, err := state.Load(deps.ConfigDir); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("unexpected state=%q error=%v", previous, err)
		}
	}
}
