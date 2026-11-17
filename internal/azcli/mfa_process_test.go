package azcli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

type loginFixture struct {
	Args     []string `json:"args"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	ExitCode int      `json:"exitCode"`
}

type loginRecordingRunner struct {
	azcli.Runner
	results []azcli.Result
}

type stderrOnlyLoginRunner struct{ azcli.Runner }

func (r stderrOnlyLoginRunner) Run(ctx context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	options.ObserveTenantFailure = nil
	return r.Runner.Run(ctx, args, options)
}

func TestLoginPreservesInjectedRunnerWithoutTenantObserver(t *testing.T) {
	const id = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: mfaMinimumVersionJSON}},
		{Result: azcli.Result{Stderr: "Authentication failed against tenant " + id + " 'Directory': AADSTS50076: MFA\n"}},
	}}
	cli, err := azcli.NewForLogin(fstest.MapFS{}, stderrOnlyLoginRunner{Runner: fake}, "")
	if err != nil {
		t.Fatal(err)
	}
	cli.Terminal = &azcli.Terminal{Stdout: io.Discard, Stderr: io.Discard}
	result, err := cli.Login(nil)
	if err != nil || !result.Attempted || len(result.Failures) != 1 || result.Failures[0].ID != id ||
		result.Failures[0].Code != "50076" || len(fake.Calls) != 2 {
		t.Fatalf("result=%+v error=%v calls=%+v", result, err, fake.Calls)
	}
}

func (r *loginRecordingRunner) Run(ctx context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	result, err := r.Runner.Run(ctx, args, options)
	r.results = append(r.results, result)
	return result, err
}

func TestRunnerDiscardsOversizedTenantFailureLines(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	const ignoredID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const validID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	const secret = "synthetic-overflow-secret"
	const warning = "WARNING: Authentication failed against tenant " + ignoredID + " 'Token': AADSTS50076: "
	const valid = "WARNING: Authentication failed against tenant " + validID + " 'Directory': AADSTS50079: MFA required"
	long := strings.Repeat("pending ", 160000)
	for _, tc := range []struct {
		name, input string
		want        int
	}{
		{"unterminated overflow", warning + long + "access_token=" + secret, 0},
		{"resume after overflow", warning + long + "access_token=" + secret + "\n" + valid, 1},
		{"ignore valid looking overflow tail", long + warning + "access_token=" + secret + "\n" + valid, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := fakeLoginProcess(t, dir, []loginFixture{{Args: []string{loginCommand}, Stderr: tc.input}})
			var output bytes.Buffer
			var failures []azcli.TenantFailure
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result, err := runner.Run(ctx, []string{loginCommand}, azcli.Options{
				Terminal: &azcli.Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &output},
				ObserveTenantFailure: func(failure azcli.TenantFailure) {
					failures = append(failures, failure)
				},
			})
			if err != nil || result.ExitCode != 0 || len(failures) != tc.want {
				t.Fatalf("exit=%d err=%v failure count=%d; want %d", result.ExitCode, err, len(failures), tc.want)
			}
			if tc.want == 1 && (failures[0].ID != validID || failures[0].Code != "50079" || failures[0].Name != "Directory") {
				t.Fatalf("next unterminated warning not classified: %+v", failures)
			}
			assertRedactedLoginDiagnostics(t, secret, output.String(), result.Stderr)
			if output.String() != result.Stderr || (tc.want == 1 && !strings.HasSuffix(result.Stderr, valid)) {
				t.Fatal("overflow changed redacted forwarding or swallowed the following diagnostic")
			}
		})
	}
}

func TestLoginClassifiesTenantFailuresBeforeDiagnosticRedaction(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	for _, name := range []string{"Password", "Token"} {
		for _, status := range []int{0, 1} {
			t.Run(name+strconv.Itoa(status), func(t *testing.T) {
				const id = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
				const secret = "synthetic-sensitive-value"
				detail := "password=" + secret + "\r\n"
				if name == "Token" && status == 0 {
					// Exceed a pipe write and end without LF to cover chunking/EOF.
					detail = strings.Repeat("MFA required ", 8192) + "password=" + secret
				}
				warning := "WARNING: Authentication failed against tenant " + id + " '" + name +
					"': AADSTS50076: " + detail
				fixtures := []loginFixture{
					{Args: []string{"version", "-o", "json"}, Stdout: mfaMinimumVersionJSON},
					{Args: []string{"login"}, Stderr: warning, ExitCode: status},
				}
				runner := fakeLoginProcess(t, dir, fixtures)
				cli, err := azcli.NewForLogin(fstest.MapFS{}, runner, "")
				if err != nil {
					t.Fatal(err)
				}
				var output bytes.Buffer
				cli.Terminal = &azcli.Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &output}
				result, err := cli.Login(nil)
				if (err != nil) != (status != 0) || !result.Attempted || len(result.Failures) != 1 {
					t.Fatalf("result=%+v error=%v output=%q", result, err, output.String())
				}
				failure := result.Failures[0]
				if failure.ID != id || failure.Name != name || failure.Code != "50076" || !failure.Retryable() {
					t.Fatalf("failure=%+v", failure)
				}
				if len(runner.results) != 2 {
					t.Fatalf("unexpected extra processes: %+v", runner.results)
				}
				assertRedactedLoginDiagnostics(t, secret, failure.Message, output.String(), runner.results[1].Stderr)
			})
		}
	}
}

func assertRedactedLoginDiagnostics(t *testing.T, secret string, diagnostics ...string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, secret) || !strings.Contains(diagnostic, "[REDACTED]") {
			t.Fatalf("unsafe diagnostic: %q", diagnostic)
		}
	}
}

func fakeLoginProcess(t *testing.T, dir string, fixtures []loginFixture) *loginRecordingRunner {
	t.Helper()
	files := testutil.WorkDir(t)
	fixturePath := filepath.Join(files, "fixtures.json")
	data, err := json.Marshal(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixturePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKEAZ_FIXTURES", fixturePath)
	t.Setenv("FAKEAZ_RECORD", filepath.Join(files, "calls.jsonl"))
	t.Setenv("FAKEAZ_ENV_KEYS", "")
	runner, err := azcli.NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	return &loginRecordingRunner{Runner: runner}
}
