package cmd_test

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
)

func runContractBinary(t *testing.T, binary string, args []string, input string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, binary, args...)
	if input != "" {
		process.Stdin = strings.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	process.Stdout, process.Stderr = &stdout, &stderr
	err := process.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	if ctx.Err() != nil {
		t.Fatalf("command did not finish promptly: %v", args)
	}
	return code, stdout.String(), stderr.String()
}

func TestBinaryContractMatrix(t *testing.T) {
	binary, _, record := fakeBinary(t, `[]`)
	for _, tc := range []struct {
		name, golden string
		args         []string
		code         int
	}{
		{name: listCommand, golden: listCommand, args: []string{listCommand, outputFlag, jsonFormat}},
		{name: goldenCurrent, golden: goldenCurrent, args: []string{currentFlag, outputFlag, jsonFormat}},
		{name: "persistent output before command", golden: listCommand, args: []string{outputFlag, jsonFormat, verboseFlag, listCommand}},
		{name: "no match", golden: "no-match", args: []string{outputFlag, jsonFormat, missingQuery}, code: 2},
		{name: "no match after delimiter", golden: "no-match", args: []string{outputFlag, jsonFormat, argumentDelimiter, missingQuery}, code: 2},
		{name: "parse error", golden: goldenParse, args: []string{bogusFlag, outputFlag, jsonFormat}, code: 1},
		{name: "subcommand parse error", golden: goldenParse, args: []string{listCommand, bogusFlag, outputFlag, jsonFormat}, code: 1},
		{name: "attached output", golden: goldenParse, args: []string{bogusFlag, "-ojson"}, code: 1},
		{name: "equals output", golden: goldenParse, args: []string{bogusFlag, "--output=json"}, code: 1},
		{name: "grouped short output", golden: goldenParse, args: []string{bogusFlag, "-cojson"}, code: 1},
		{name: "removed short", golden: "removed-short", args: []string{"-s", outputFlag, jsonFormat}, code: 1},
		{name: "removed long", golden: "removed-long", args: []string{outputFlag, jsonFormat, "--short"}, code: 1},
		{name: "removed flag on subcommand", golden: "removed-long", args: []string{listCommand, "--short", outputFlag, jsonFormat}, code: 1},
		{name: "removed tenant filter", golden: "removed-filter", args: []string{"--filter-tenant-level=false", outputFlag, jsonFormat}, code: 1},
		{name: "extra arguments", golden: "extra-arguments", args: []string{outputFlag, jsonFormat, "one", "two"}, code: 1},
		{name: "current with query", golden: "current-query", args: []string{currentFlag, outputFlag, jsonFormat, productionName}, code: 1},
		{name: "login JSON rejected before az", golden: "login-json", args: []string{loginCommand, outputFlag, jsonFormat}, code: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runContractBinary(t, binary, tc.args, "")
			if code != tc.code {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			output := stdout
			if code != 0 {
				output = stderr
				if stdout != "" {
					t.Fatalf("error leaked to stdout=%q", stdout)
				}
			} else if stderr != "" {
				t.Fatalf("data command stderr=%q", stderr)
			}
			assertContractGolden(t, tc.golden, output)
		})
	}
	if calls, err := os.ReadFile(record); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected az calls=%s error=%v", calls, err)
	}
}

func TestBinaryContractNoninteractiveVariants(t *testing.T) {
	binary, config, record := fakeBinary(t, `[]`)
	for _, gate := range []string{"devnull", "pipe", envCI, gateEmptyCI, gateFlag, gateEnvironment} {
		t.Run(gate, func(t *testing.T) {
			writeFixture(t, filepath.Join(config, profileFile), fixtureProfile)
			args, input := noninteractiveBinaryArgs(t, gate)
			code, stdout, stderr := runContractBinary(t, binary, args, input)
			if code != 0 || !strings.Contains(stdout, productionName) || stderr != "" {
				t.Fatalf("bare code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			writeFixture(t, filepath.Join(config, profileFile), strings.ReplaceAll(fixtureProfile, "Development", productionName))
			code, stdout, stderr = runContractBinary(t, binary, append(args, productionName), input)
			if code != 2 || stdout != "" || !strings.Contains(stderr, productionID) || !strings.Contains(stderr, developmentID) ||
				strings.Contains(stderr, "\x1b") {
				t.Fatalf("ambiguous code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
	if calls, err := os.ReadFile(record); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected az calls=%s error=%v", calls, err)
	}
}

func noninteractiveBinaryArgs(t *testing.T, gate string) ([]string, string) {
	t.Helper()
	switch gate {
	case "pipe":
		return nil, "\x03\n"
	case envCI:
		t.Setenv(envCI, "1")
	case gateEmptyCI:
		t.Setenv(envCI, "")
	case gateFlag:
		return []string{"--no-input"}, ""
	case gateEnvironment:
		t.Setenv("AZCTX_NO_INPUT", "1")
	}
	return nil, ""
}
