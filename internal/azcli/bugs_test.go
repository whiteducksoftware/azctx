package azcli_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/log"
)

func TestDefaultAzureDirectoryIsQuiet(t *testing.T) {
	testutil.IsolateUserDirs(t)
	t.Setenv("AZURE_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".azure")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "azureProfile.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	log.InitLogger(&output)
	t.Cleanup(func() { log.InitLogger(os.Stderr) })
	if _, err := azcli.New(nil, &testutil.FakeRunner{}, ""); err != nil || output.Len() != 0 {
		t.Fatalf("default directory: %v output=%q", err, output.String())
	}
	if err := os.Remove(filepath.Join(dir, "azureProfile.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := azcli.New(nil, &testutil.FakeRunner{}, ""); !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "az login") {
		t.Fatalf("missing disk profile: %v", err)
	}
}

func TestInvalidIDsNeverReachAZ(t *testing.T) {
	for _, id := range []string{"", "a-name", "--subscription", "11111111-1111-1111-1111-11111111111g", "111111111111-1111-1111-111111111111"} {
		fake := &testutil.FakeRunner{}
		cli, err := azcli.New(fstest.MapFS{profileFilename: &fstest.MapFile{Data: []byte(`{}`)}}, fake, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := cli.SetSubscription(profile.Subscription{Id: id}); err == nil {
			t.Fatalf("accepted subscription %q", id)
		}
		for _, args := range [][]string{{"--subscription", id}, {"--subscription=" + id}, {"--tenant", id}, {"--tenant=" + id}, {"-t", id}} {
			if err := cli.InteractiveLogin(args); err == nil {
				t.Fatalf("accepted login IDs %q", args)
			}
		}
		if len(fake.Calls) != 0 {
			t.Fatalf("invalid ID reached az: %+v", fake.Calls)
		}
	}
}

func TestLoginKeepsFailureCauseWithoutERRORPrefix(t *testing.T) {
	for _, message := range []string{"", "device code expired", "ERROR: denied\n"} {
		failure := errors.New("process failed")
		fake := &testutil.FakeRunner{Responses: []testutil.Response{
			{Result: azcli.Result{Stdout: `{"azure-cli":"2.61.0"}`}},
			{Result: azcli.Result{Stderr: message}, Err: failure},
		}}
		cli, err := azcli.New(fstest.MapFS{profileFilename: &fstest.MapFile{Data: []byte(`{}`)}}, fake, "")
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		cli.Terminal = &azcli.Terminal{Stderr: &stderr}
		if err := cli.InteractiveLogin(nil); !errors.Is(err, failure) {
			t.Fatalf("lost login failure: %v", err)
		}
		if stderr.String() != message {
			t.Fatalf("login stderr=%q want %q", stderr.String(), message)
		}
	}
}
