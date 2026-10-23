package cmd_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/version"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (transport updateTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type versionFeedbackFixture struct {
	deps     cmd.Dependencies
	requests int
	offline  bool
}

func newVersionFeedbackFixture(t *testing.T) *versionFeedbackFixture {
	t.Helper()
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{}, {Result: azcli.Result{Stdout: brokerVersionJSON}},
	}}
	fixture := &versionFeedbackFixture{deps: commandDependencies(t, fake)}
	fixture.deps.CheckUpdates = nil
	originalClient, originalVersion := http.DefaultClient, version.Version
	t.Cleanup(func() { http.DefaultClient, version.Version = originalClient, originalVersion })
	version.Version = "2024.12.5"
	http.DefaultClient = &http.Client{Transport: updateTransport(func(request *http.Request) (*http.Response, error) {
		fixture.requests++
		deadline, ok := request.Context().Deadline()
		if request.Method != http.MethodHead || !ok || time.Until(deadline) > 2*time.Second {
			t.Errorf("fresh lookup lacks HEAD/two-second deadline: %s %v %t", request.Method, deadline, ok)
		}
		if fixture.offline {
			return nil, errors.New("synthetic offline")
		}
		final := request.Clone(request.Context())
		final.URL.Path = "/whiteducksoftware/azctx/releases/tag/v2026.11.0"
		if fixture.requests > 1 {
			final.URL.Path = "/whiteducksoftware/azctx/releases/tag/v2026.12.0"
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: final}, nil
	})}
	return fixture
}

func (fixture *versionFeedbackFixture) run(t *testing.T, args ...string) (string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := cmd.Run(args, fixture.deps, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("%v: code=%d stdout=%q stderr=%q", args, code, &stdout, &stderr)
	}
	return stdout.String(), stderr.String()
}

func TestVersionFreshUpdatesSwitchSilenceAndOfflineDoctor(t *testing.T) {
	fixture := newVersionFeedbackFixture(t)
	_, notice := fixture.run(t, versionCommand)
	if fixture.requests != 1 || strings.Count(notice, "A new version") != 1 || !strings.Contains(notice, "v2026.11.0") {
		t.Fatalf("first text check requests=%d notice=%q", fixture.requests, notice)
	}
	_, notice = fixture.run(t, versionCommand, outputFlag, textFormat)
	if fixture.requests != 2 || strings.Count(notice, "A new version") != 1 || !strings.Contains(notice, "v2026.12.0") {
		t.Fatalf("text reused recent cache: requests=%d notice=%q", fixture.requests, notice)
	}
	if out, messages := fixture.run(t, productionID); out != "" || messages != switchFeedback(productionName, productionID, tenantID, tenantID) || fixture.requests != 2 {
		t.Fatalf("switch checked/noticed cached update: requests=%d stdout=%q stderr=%q", fixture.requests, out, messages)
	}
	doctor, messages := fixture.run(t, "doctor", outputFlag, jsonFormat)
	if messages != "" || fixture.requests != 2 || !strings.Contains(doctor, `"update":{"state":"available","latestVersion":"2026.12.0","stale":false}`) {
		t.Fatalf("doctor cache status requests=%d stdout=%q stderr=%q", fixture.requests, doctor, messages)
	}
}

func TestVersionInvalidUpdateConfigIsDebugOnlyAndJSONOffline(t *testing.T) {
	fixture := newVersionFeedbackFixture(t)
	jsonBefore, _ := fixture.run(t, versionCommand, outputFlag, jsonFormat)
	writeSelectionConfig(t, "unknownKey: true\n")
	for _, verbose := range []bool{false, true} {
		args := []string{versionCommand}
		if verbose {
			args = append(args, verboseFlag)
		}
		stdout, messages := fixture.run(t, args...)
		if !strings.HasPrefix(stdout, "azctx version ") || strings.Contains(messages, "unknownKey") != verbose || fixture.requests != 0 {
			t.Fatalf("bad update config affected version: requests=%d stdout=%q stderr=%q", fixture.requests, stdout, messages)
		}
	}
	if jsonAfter, messages := fixture.run(t, versionCommand, outputFlag, jsonFormat); jsonAfter != jsonBefore || messages != "" || fixture.requests != 0 {
		t.Fatalf("JSON read bad config or went online: requests=%d stdout=%q stderr=%q", fixture.requests, jsonAfter, messages)
	}
}

func TestVersionOfflineFailuresAreDebugOnlyAndJSONUnchanged(t *testing.T) {
	fixture := newVersionFeedbackFixture(t)
	jsonBefore, messages := fixture.run(t, versionCommand, outputFlag, jsonFormat)
	if messages != "" || fixture.requests != 0 {
		t.Fatalf("JSON went online: requests=%d stderr=%q", fixture.requests, messages)
	}
	text, _ := fixture.run(t, versionCommand)
	fixture.offline = true
	for _, verbose := range []bool{false, true} {
		args := []string{versionCommand}
		if verbose {
			args = append(args, verboseFlag)
		}
		out, messages := fixture.run(t, args...)
		if out != text || strings.Contains(messages, "synthetic offline") != verbose || strings.Contains(messages, "A new version") {
			t.Fatalf("offline version changed status/output: stdout=%q stderr=%q", out, messages)
		}
	}
	if fixture.requests != 3 {
		t.Fatalf("offline text reused failure cache: requests=%d", fixture.requests)
	}
	if jsonAfter, messages := fixture.run(t, versionCommand, outputFlag, jsonFormat); jsonAfter != jsonBefore || messages != "" || fixture.requests != 3 {
		t.Fatalf("JSON changed or contacted network: requests=%d stdout=%q stderr=%q", fixture.requests, jsonAfter, messages)
	}
}
