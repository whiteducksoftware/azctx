package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	listCommand     = "list"
	developmentName = "Development"
	verboseFlag     = "--verbose"
	envCI           = "CI"
	missingQuery    = "missing"
	bogusFlag       = "--bogus"
	gateEmptyCI     = "empty CI"
	goldenCurrent   = "current"
	goldenParse     = "parse-error"
	gateEnvironment = "environment"
	textFormat      = "text"
	nameFormat      = "name"
	idFormat        = "id"
	gateFlag        = "flag"
)

func TestContractBareNonTTYListsWithoutSideEffects(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	deps.StdinTTY = func() bool { return false }
	deps.Select = func([]profile.Subscription) (int, error) {
		t.Fatal("non-TTY input must not open the picker")
		return -1, nil
	}

	deps.CheckUpdates = func(string, string) error {
		t.Fatal("listing must not check for updates")
		return nil
	}
	root := cmd.NewRootCommand(deps)
	var stdout, stderr bytes.Buffer
	root.SetIn(strings.NewReader(""))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(nil)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), productionName) || !strings.Contains(stdout.String(), "Development") ||
		strings.Contains(stdout.String(), "N/A") || stderr.Len() != 0 || len(fake.Calls) != 0 {
		t.Fatalf("stdout=%q stderr=%q calls=%+v", stdout.String(), stderr.String(), fake.Calls)
	}
}

func TestContractJSONGoldens(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{listCommand, []string{listCommand, outputFlag, jsonFormat}},
		{"list-all", []string{listCommand, "--all", outputFlag, jsonFormat}},
		{goldenCurrent, []string{currentFlag, outputFlag, jsonFormat}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			deps.Select = func([]profile.Subscription) (int, error) {
				t.Fatal("data output must not open a picker")
				return -1, nil
			}
			deps.CheckUpdates = func(string, string) error {
				t.Fatal("data output must not check updates")
				return nil
			}
			root := cmd.NewRootCommand(deps)
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			assertContractGolden(t, tc.name, stdout.String())
			if stderr.Len() != 0 || len(fake.Calls) != 0 {
				t.Fatalf("stderr=%q calls=%+v", stderr.String(), fake.Calls)
			}
		})
	}
}

func TestContractGoldensUseCanonicalLF(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "contract", "*.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no contract goldens found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsRune(data, '\r') || bytes.Count(data, []byte{'\n'}) != 1 ||
				!bytes.HasSuffix(data, []byte{'\n'}) {
				t.Fatalf("contract golden must use exactly one trailing LF and no CR: %q", data)
			}
		})
	}
}

func assertContractGolden(t *testing.T, name, got string) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", "contract", name+".golden"))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("%s golden mismatch\ngot:  %q\nwant: %q", name, got, want)
	}
}
