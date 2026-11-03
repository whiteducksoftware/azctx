package cmd_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	t.Setenv("AZURE_CLOUD_NAME", azureCloudName)
	t.Setenv("FAKEAZ_FIXTURES", fixturePath)
	t.Setenv("FAKEAZ_RECORD", record)
	t.Setenv("FAKEAZ_ENV_KEYS", "")
	t.Setenv("AZCTX_NO_UPDATE_CHECK", "1")
	t.Setenv("AZCTX_NO_INPUT", "")
	t.Setenv("CI", "")
	if err := os.Unsetenv("CI"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:0")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:0")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
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
		name, id := developmentName, developmentID
		if i == 1 {
			name, id = productionName, productionID
		}
		if err != nil || string(output) != switchFeedback(name, id, tenantID, tenantID) {
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

func readDevicePrompt(reader io.Reader, expected string, visible chan<- string, finished chan<- struct{}) {
	defer func() { finished <- struct{}{} }()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if scanner.Text() == expected {
			visible <- expected
		}
	}
}

func TestBinaryDeviceCodeVisibleBeforeAZExits(t *testing.T) {
	binary, _, _ := fakeBinary(t, `[
		{"args":["version","-o","json"],"stdout":"{\"azure-cli\":\"2.61.0\"}"},
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
	go readDevicePrompt(stdout, "stdout device code ABCD", visible, finished)
	go readDevicePrompt(stderr, "stderr device code EFGH", visible, finished)
	select {
	case prompt := <-visible:
		if prompt != "stderr device code EFGH" {
			t.Fatalf("login stdout was not suppressed: %q", prompt)
		}
	case <-ctx.Done():
		_ = stdin.Close()
		_ = command.Wait()
		t.Fatal("stderr device code was not streamed before az waited for input")
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
	if len(visible) != 0 {
		t.Fatalf("unexpected login output: %q", <-visible)
	}
}

func TestBinaryLoginRejectsUndelimitedArguments(t *testing.T) {
	binary, _, record := fakeBinary(t, `[
		{"args":["version","-o","json"],"stdout":"{\"azure-cli\":\"2.61.0\"}"},
		{"args":["login","unexpected","--use-device-code"]}
	]`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, binary, loginCommand, "unexpected", argumentDelimiter, deviceCodeFlag)
	var stdout, stderr bytes.Buffer
	process.Stdout, process.Stderr = &stdout, &stderr
	err := process.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Errorf("exit=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if calls, err := os.ReadFile(record); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unexpected Azure calls=%s error=%v", calls, err)
	}
}
