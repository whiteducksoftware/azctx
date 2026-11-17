package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestEnvironmentCurrentAndExplicitQueryAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		args         []string
		code         int
	}{
		{"current ignores configured hiding", "hidden: [" + productionID + "]\npinned: [" + developmentID + "]\n", nil, 0},
		{"empty explicit query", "", []string{""}, 1},
		{"blank explicit query", "", []string{" \t "}, 1},
		{"invalid user config", "unexpected: true\n", nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := commandDependencies(t, &testutil.FakeRunner{})
			writeSelectionConfig(t, tc.config)
			var out, messages bytes.Buffer
			code := cmd.Run(append([]string{envCommand}, tc.args...), deps, strings.NewReader(""), &out, &messages)
			if code != tc.code || (tc.code == 0 && !strings.Contains(out.String(), productionID)) ||
				(tc.code != 0 && out.Len() != 0) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &messages)
			}
		})
	}
}

func TestEnvironmentQueryLabelsAreNeverShellCode(t *testing.T) {
	for _, name := range []string{`x'"; $(touch nope); & |`, "line1\nline2", outputFlag + "=" + jsonFormat} {
		for _, shell := range []string{envBashSyntax, envZshSyntax, envFishSyntax, envPSSyntax} {
			t.Run(shell+"/"+name, func(t *testing.T) {
				deps := commandDependencies(t, &testutil.FakeRunner{})
				data, err := json.Marshal(profile.Profile{Subscriptions: []profile.Subscription{{
					Id: productionID, Tenant: tenantID, Name: name, Environment: azureCloudName, IsDefault: true,
				}}})
				if err != nil {
					t.Fatal(err)
				}
				deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: data}}
				var out, messages bytes.Buffer
				code := cmd.Run([]string{envCommand, shellFlag, shell, argumentDelimiter, name}, deps, strings.NewReader(""), &out, &messages)
				if code != 0 || messages.Len() != 0 || strings.Contains(out.String(), name) || strings.Count(out.String(), "\n") != 4 {
					t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &messages)
				}
			})
		}
	}
}

func TestEnvironmentOutputFailuresAndExecErrorPreScan(t *testing.T) {
	for _, format := range []string{envBashSyntax, envDevOpsFormat} {
		t.Run(format, func(t *testing.T) {
			deps := commandDependencies(t, &testutil.FakeRunner{})
			args := []string{envCommand}
			if format == envDevOpsFormat {
				args = append(args, formatFlag, format)
			}
			var messages bytes.Buffer
			if code := cmd.Run(args, deps, strings.NewReader(""), doctorFailedWriter{}, &messages); code != 1 || !strings.Contains(messages.String(), "output failed") {
				t.Fatalf("code=%d stderr=%q", code, &messages)
			}
		})
	}
	deps := commandDependencies(t, &testutil.FakeRunner{})
	var out, messages bytes.Buffer
	args := []string{execSubcommand, bogusFlag, productionID, argumentDelimiter, "not-run", outputFlag + "=" + jsonFormat}
	if code := cmd.Run(args, deps, strings.NewReader(""), &out, &messages); code != 1 ||
		!strings.HasPrefix(messages.String(), "Error: unknown flag:") || out.Len() != 0 {
		t.Fatalf("child output flag changed azctx parse-error mode: %d %q %q", code, &out, &messages)
	}
}
