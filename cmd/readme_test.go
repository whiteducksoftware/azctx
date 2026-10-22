package cmd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const currentJSONInvocation = "azctx -c -o json"

func TestREADMEParserRejectsBrokenInvocation(t *testing.T) {
	markdown := string(readAgentFile(t, "../README.md"))
	const original = "azctx list --all -o json"
	if !strings.Contains(markdown, original) {
		t.Fatal("README lost its structured list example")
	}
	markdown = strings.Replace(markdown, original, original+" --invented", 1)
	rejected := false
	for _, snippet := range agentCommandSnippets(markdown) {
		for _, args := range expandAgentSynopsis(snippet.text) {
			if slices.Contains(args, "--invented") {
				rejected = parseAgentExample(args, snippet.invocation) != nil
			}
		}
	}
	if !rejected {
		t.Fatal("README extraction/parse boundary accepted an invented flag")
	}
}

func TestBinaryREADMEQuickstart(t *testing.T) {
	markdown := string(readAgentFile(t, "../README.md"))
	_, section, ok := strings.Cut(markdown, "## 5. Quickstart\n")
	if !ok {
		t.Fatal("README quickstart missing")
	}
	section, _, _ = strings.Cut(section, "## 6.")
	snippets := agentCommandSnippets(section)
	var invocations []string
	for _, snippet := range snippets {
		if snippet.invocation {
			invocations = append(invocations, snippet.text)
		}
	}
	const productionInvocation = "azctx prod"
	want := []string{"azctx login", "azctx", productionInvocation, "azctx -", currentJSONInvocation, "azctx list --all -o json", "azctx help"}
	if !slices.Equal(invocations, want) {
		t.Fatalf("review quickstart execution coverage: %q", invocations)
	}
	binary, azureDirectory, record := fakeBinary(t, `[
		{"args":["version","-o","json"],"stdout":"{\"azure-cli\":\"2.61.0\"}"},
		{"args":["login"]},
		{"args":["account","set","--subscription","`+productionID+`"]},
		{"args":["account","set","--subscription","`+developmentID+`"]}
	]`)
	profilePath := filepath.Join(azureDirectory, profileFile)
	developmentProfile := strings.ReplaceAll(
		strings.ReplaceAll(fixtureProfile, `"isDefault":true`, `"isDefault":false`),
		`"name":"Development"`, `"isDefault":true,"name":"Development"`)
	writeFixture(t, profilePath, developmentProfile)
	for _, invocation := range invocations {
		args := expandAgentSynopsis(invocation)[0]
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("%s: %v\n%s", invocation, err, output)
		}
		if invocation == productionInvocation || invocation == "azctx -" {
			if len(output) != 0 {
				t.Fatalf("%s should stay quiet: %s", invocation, output)
			}
			if invocation == productionInvocation {
				// The fixture simulates Azure CLI's profile refresh, never azctx.
				writeFixture(t, profilePath, fixtureProfile)
			} else {
				writeFixture(t, profilePath, developmentProfile)
			}
		} else if len(output) == 0 {
			t.Fatalf("%s lost documented output", invocation)
		}
		assertREADMECurrentIdentity(t, invocation, output)
	}

	calls, err := os.ReadFile(record)
	if err != nil || strings.Count(string(calls), "\n") != 4 {
		t.Fatalf("quickstart Azure calls: %s, %v", calls, err)
	}
}

func assertREADMECurrentIdentity(t *testing.T, invocation string, output []byte) {
	t.Helper()
	if invocation == currentJSONInvocation && !strings.Contains(string(output), `"id":"`+developmentID+`"`) {
		t.Fatalf("previous did not restore the documented current identity: %s", output)
	}
}
