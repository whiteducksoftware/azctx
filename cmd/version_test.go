package cmd_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	jsonFormat     = "json"
	versionCommand = "version"
	outputFlag     = "-o"
)

func TestBinaryVersionLDFlags(t *testing.T) {
	binary := filepath.Join(testutil.WorkDir(t), testutil.ExecutableName("azctx"))
	flags := "-X github.com/whiteducksoftware/azctx/internal/version.Version=2026.11.0" +
		" -X github.com/whiteducksoftware/azctx/internal/version.BuildDate=2026-10-21T16:00:00Z" +
		" -X github.com/whiteducksoftware/azctx/internal/version.Commit=abc123"
	build := exec.Command("go", "build", "-ldflags", flags, "-o", binary, "github.com/whiteducksoftware/azctx")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build with release ldflags: %v\n%s", err, output)
	}
	// A version query must not resolve az or read an Azure profile.
	t.Setenv("PATH", "")
	t.Setenv("AZURE_CONFIG_DIR", filepath.Join(testutil.WorkDir(t), "absent"))
	command := exec.Command(binary, versionCommand, outputFlag, jsonFormat)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil || stderr.Len() != 0 {
		t.Fatalf("version: %v, stderr=%q", err, stderr.String())
	}
	var info map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info["version"] != "2026.11.0" || info["buildDate"] != "2026-10-21T16:00:00Z" || info["commit"] != "abc123" {
		t.Fatalf("ldflags not reflected by version CLI: %s", stdout.String())
	}
}

func TestVersionOutput(t *testing.T) {
	for _, format := range []string{"", "text", jsonFormat} {
		t.Run(format, func(t *testing.T) {
			root := cmd.NewRootCommand(cmd.Dependencies{})
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{versionCommand, outputFlag, format})
			if format == "" {
				root.SetArgs([]string{versionCommand})
			}
			err := root.Execute()
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("version: err=%v stderr=%q", err, stderr.String())
			}
			if format != jsonFormat {
				if !strings.HasPrefix(stdout.String(), "azctx version ") || !strings.HasSuffix(stdout.String(), "\n") {
					t.Fatalf("text version = %q", stdout.String())
				}
				return
			}
			var info map[string]string
			if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
				t.Fatalf("JSON version = %q: %v", stdout.String(), err)
			}
			for _, key := range []string{"version", "buildDate", "commit", "goVersion"} {
				if info[key] == "" {
					t.Fatalf("missing %s in %s", key, stdout.String())
				}
			}
		})
	}
}

func TestVersionRejectsUnsupportedFormats(t *testing.T) {
	for _, format := range []string{"short", "yaml", "pretty", "unknown"} {
		t.Run(format, func(t *testing.T) {
			root := cmd.NewRootCommand(cmd.Dependencies{})
			var stdout bytes.Buffer
			root.SetOut(&stdout)
			root.SetArgs([]string{versionCommand, outputFlag, format})
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "unsupported version output format") || stdout.Len() != 0 {
				t.Fatalf("unsupported format: err=%v stdout=%q", err, stdout.String())
			}
		})
	}
}
