package cmd_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func fakeBinary(t *testing.T, fixtures string) (string, string, string) {
	t.Helper()
	dir := testutil.BuildFakeAZ(t)
	binary := filepath.Join(dir, testutil.ExecutableName("azctx"))
	testutil.Build(t, binary, "github.com/whiteducksoftware/azctx")
	testutil.IsolateUserDirs(t)
	config := filepath.Join(dir, "azure")
	if err := os.Mkdir(config, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(config, "azureProfile.json"), fixtureProfile)
	fixturePath, record := filepath.Join(dir, "fixtures.json"), filepath.Join(dir, "calls.jsonl")
	writeFixture(t, fixturePath, fixtures)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZURE_CONFIG_DIR", config)
	t.Setenv("FAKEAZ_FIXTURES", fixturePath)
	t.Setenv("FAKEAZ_RECORD", record)
	t.Setenv("FAKEAZ_ENV_KEYS", "")
	t.Setenv("AZCTX_NO_UPDATE_CHECK", "1")
	return binary, config, record
}

func TestBinaryPreviousSubscriptionAcrossProcesses(t *testing.T) {
	fixtures := `[
		{"args":["account","set","--subscription","` + developmentID + `"]},
		{"args":["account","set","--subscription","` + productionID + `"]},
		{"args":["account","set","--subscription","` + developmentID + `"]}
	]`
	binary, config, record := fakeBinary(t, fixtures)
	profilePath := filepath.Join(config, "azureProfile.json")
	for i, args := range [][]string{{argumentDelimiter, "Development"}, {"-"}, {"-"}} {
		before, err := os.ReadFile(profilePath)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		command := exec.CommandContext(ctx, binary, args...)
		output, err := command.CombinedOutput()
		cancel()
		if err != nil || len(output) != 0 {
			t.Fatalf("switch %d: %v, %s", i, err, output)
		}
		after, err := os.ReadFile(profilePath)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("azctx wrote Azure profile: %q, %v", after, err)
		}
		// Simulate Azure CLI's external profile refresh, not an azctx write.
		var parsed profile.Profile
		if err := json.Unmarshal(after, &parsed); err != nil {
			t.Fatal(err)
		}
		for j := range parsed.Subscriptions {
			parsed.Subscriptions[j].IsDefault = !parsed.Subscriptions[j].IsDefault && j < 2
		}
		data, err := json.Marshal(parsed)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, profilePath, string(data))
	}
	calls, err := os.ReadFile(record)
	if err != nil || strings.Count(string(calls), "\n") != 3 {
		t.Fatalf("toggle calls=%s error=%v", calls, err)
	}
}

func cleanProcess(t *testing.T, command *exec.Cmd, cancel context.CancelFunc) {
	t.Helper()
	t.Cleanup(func() {
		cancel()
		if command.ProcessState == nil {
			_ = command.Wait()
		}
	})
}

func TestBinaryDeviceCodeVisibleBeforeAZExits(t *testing.T) {
	binary, _, _ := fakeBinary(t, `[
		{"args":["rest","--method","get","--url","/tenants?api-version=2020-01-01","--output","json"],"stdout":"{\"value\":[]}"},
		{"args":["login","--use-device-code"],"stdout":"stdout device code ABCD\n","stderr":"stderr device code EFGH\n","waitForInput":true}
	]`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, loginCommand, argumentDelimiter, deviceCodeFlag)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	cleanProcess(t, command, cancel)
	visible := make(chan string, 2)
	finished := make(chan struct{}, 2)
	readPrompt := func(reader io.Reader, expected string) {
		defer func() { finished <- struct{}{} }()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			if scanner.Text() == expected {
				visible <- expected
			}
		}
	}
	go readPrompt(stdout, "stdout device code ABCD")
	go readPrompt(stderr, "stderr device code EFGH")
	for range 2 {
		select {
		case <-visible:
		case <-ctx.Done():
			_ = stdin.Close()
			_ = command.Wait()
			t.Fatal("device code was not streamed before az waited for input")
		}
	}
	if _, err := io.WriteString(stdin, "acknowledge\n"); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	for range 2 {
		select {
		case <-finished:
		case <-ctx.Done():
			_ = command.Wait()
			t.Fatal("login did not finish after acknowledging the streamed code")
		}
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}
