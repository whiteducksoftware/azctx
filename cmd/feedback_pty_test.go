//go:build darwin || linux

package cmd_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/log"
)

type feedbackStream struct {
	bytes.Buffer
	fd uintptr
}

func (stream *feedbackStream) Fd() uintptr { return stream.fd }

const greenProduction = "\x1b[32mProduction"

func TestFeedbackUsesTargetStreamTTYAndNoColor(t *testing.T) {
	for _, mode := range []struct {
		name                 string
		stdoutTTY, stderrTTY bool
		noColor              bool
	}{
		{"pipes forced", false, false, false},
		{"stdout only", true, false, false},
		{"stderr only", false, true, false},
		{"both", true, true, false},
		{"empty NO_COLOR forced", true, true, true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{},
				{Result: azcli.Result{Stdout: brokerVersionJSON}},
				{},
				{Result: azcli.Result{Stdout: brokerVersionJSON}},
				{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
				{Result: azcli.Result{Stdout: brokerVersionJSON}},
			}}
			deps := commandDependencies(t, fake)
			deps.Platform = platformWindows
			deps.StdinTTY = func() bool { return false }
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("COLORTERM", "truecolor")
			t.Setenv("FORCE_COLOR", "1")
			t.Setenv("CLICOLOR_FORCE", "1")
			t.Setenv("NO_COLOR", "")
			if !mode.noColor {
				if err := os.Unsetenv("NO_COLOR"); err != nil {
					t.Fatal(err)
				}
			}
			master, slave := openContractPTY(t)
			defer master.Close()
			defer slave.Close()
			stdout, stderr := feedbackStream{fd: ^uintptr(0)}, feedbackStream{fd: ^uintptr(0)}
			if mode.stdoutTTY {
				stdout.fd = slave.Fd()
			}
			if mode.stderrTTY {
				stderr.fd = slave.Fd()
			}
			stdoutColor := mode.stdoutTTY && !mode.noColor
			stderrColor := mode.stderrTTY && !mode.noColor
			run := func(args ...string) (string, string) {
				t.Helper()
				stdout.Reset()
				stderr.Reset()
				if code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr); code != 0 {
					t.Fatalf("%v: code=%d stdout=%q stderr=%q", args, code, &stdout, &stderr)
				}
				return stdout.String(), stderr.String()
			}
			current, messages := run(currentFlag)
			wantCurrent := "Active subscription: " + tenantID + "/Production (" + tenantID + "/" + productionID + ")\n"
			assertFeedbackStyle(t, current, wantCurrent, stdoutColor, []string{greenProduction, "\x1b[36m" + productionID, "\x1b[2m" + tenantID})
			if messages != "" {
				t.Fatalf("current messages=%q", messages)
			}
			table, _ := run(listCommand)
			var plain, plainErr bytes.Buffer
			if code := cmd.Run([]string{listCommand}, deps, strings.NewReader(""), &plain, &plainErr); code != 0 || plainErr.Len() != 0 {
				t.Fatalf("plain table status=%d stderr=%q", code, &plainErr)
			}
			assertFeedbackStyle(t, table, plain.String(), stdoutColor, []string{"\x1b[1mNAME", greenProduction, "\x1b[36m" + productionID, "\x1b[2m" + tenantID})
			out, messages := run(productionID)
			if out != "" {
				t.Fatalf("switch stdout=%q", out)
			}
			assertFeedbackStyle(t, messages, switchFeedback(productionName, productionID, tenantID, tenantID), stderrColor, []string{"\x1b[32m✓", "\x1b[36mProduction", "\x1b[36m" + productionID})
			_, messages = run(loginCommand, deviceCodeFlag)
			assertFeedbackStyle(t, messages,
				"Logging in... Follow the Azure CLI device code instructions.\n✓ Signed in: 1 tenants, 2 subscriptions\n"+wantCurrent,
				stderrColor, []string{"\x1b[32m✓", greenProduction, "\x1b[36m" + productionID})
			assertPartialLoginFeedback(t, deps, &stdout, &stderr, stderrColor)
			doctor, _ := run("doctor")
			if ansi.Strip(doctor) != doctor {
				t.Fatalf("doctor acquired styling: %q", doctor)
			}
			json, _ := run(currentFlag, outputFlag, jsonFormat)
			if ansi.Strip(json) != json {
				t.Fatalf("JSON acquired styling: %q", json)
			}
			stderr.Reset()
			log.InitLogger(&stderr)
			log.Info("Signing in to tenant %s.", tenantID)
			log.Error("failure")
			assertFeedbackStyle(t, stderr.String(), "Signing in to tenant "+tenantID+".\nError: failure\n", stderrColor, []string{"\x1b[36m" + tenantID, "\x1b[31mError:"})
		})
	}
}

func TestVersionUpdateNoticeUsesStderrColorPolicy(t *testing.T) {
	for _, mode := range []string{"terminal notice", "redirected notice", "NO_COLOR"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newVersionFeedbackFixture(t)
			t.Setenv("NO_COLOR", "")
			if mode != "NO_COLOR" {
				if err := os.Unsetenv("NO_COLOR"); err != nil {
					t.Fatal(err)
				}
			}
			master, slave := openContractPTY(t)
			defer master.Close()
			defer slave.Close()
			var stdout bytes.Buffer
			stderr := feedbackStream{fd: slave.Fd()}
			if mode == "redirected notice" {
				stderr.fd = ^uintptr(0)
			}
			code := cmd.Run([]string{versionCommand}, fixture.deps, strings.NewReader(""), &stdout, &stderr)
			notice := "A new version of azctx is available: 2024.12.5 → v2026.11.0\nhttps://github.com/whiteducksoftware/azctx/releases/tag/v2026.11.0\n"
			if code != 0 || fixture.requests != 1 {
				t.Fatalf("version code=%d requests=%d stdout=%q stderr=%q", code, fixture.requests, &stdout, &stderr)
			}
			assertFeedbackStyle(t, stderr.String(), notice, mode == "terminal notice", []string{"\x1b[33mA new version"})
		})
	}
}

func assertPartialLoginFeedback(t *testing.T, deps cmd.Dependencies, stdout, stderr *feedbackStream, color bool) {
	t.Helper()
	stdout.Reset()
	stderr.Reset()
	code := cmd.Run([]string{loginCommand, noInputFlag, deviceCodeFlag}, deps, strings.NewReader(""), stdout, stderr)
	if code != 3 || strings.Contains(stderr.String(), "✓ Signed in:") {
		t.Fatalf("partial login code=%d stderr=%q", code, stderr)
	}
	for _, style := range []string{
		"\x1b[33mFetched 1 tenants and 2 subscriptions; sign-in incomplete.",
		"\x1b[33mWarning:",
	} {
		if strings.Contains(stderr.String(), style) != color {
			t.Fatalf("partial/error color=%t style=%q output=%q", color, style, stderr)
		}
	}
}

func assertFeedbackStyle(t *testing.T, got, plain string, colored bool, styles []string) {
	t.Helper()
	if ansi.Strip(got) != plain || !colored && got != plain {
		t.Fatalf("layout/plain output changed: got=%q want=%q colored=%t", got, plain, colored)
	}
	for _, style := range styles {
		if strings.Contains(got, style) != colored {
			t.Fatalf("color=%t style=%q output=%q", colored, style, got)
		}
	}
}
