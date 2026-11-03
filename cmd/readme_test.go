package cmd_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	currentJSONInvocation = "azctx -c -o json"
	productionInvocation  = "azctx prod"
)

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

func readmeQuickstartInvocations(t *testing.T, markdown string) []string {
	t.Helper()
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
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
	want := []string{"azctx login", "azctx", productionInvocation, "azctx -", currentJSONInvocation, "azctx list --all -o json", "azctx help"}
	if !slices.Equal(invocations, want) {
		t.Fatalf("review quickstart execution coverage: %q", invocations)
	}
	return invocations
}

func TestREADMEQuickstartLineEndings(t *testing.T) {
	markdown := strings.ReplaceAll(string(readAgentFile(t, "../README.md")), "\r\n", "\n")
	for _, ending := range []struct {
		name string
		text string
	}{
		{"LF", "\n"},
		{"CRLF", "\r\n"},
	} {
		t.Run(ending.name, func(t *testing.T) {
			invocations := readmeQuickstartInvocations(t, strings.ReplaceAll(markdown, "\n", ending.text))
			for _, invocation := range invocations {
				for _, args := range expandAgentSynopsis(invocation) {
					if err := parseAgentExample(args, true); err != nil {
						t.Fatalf("%s: %v", invocation, err)
					}
				}
			}
		})
	}
}

func TestREADMEQuickstartOnCRLFCheckout(t *testing.T) {
	directory := testutil.WorkDir(t)
	args := []string{
		"-c", "core.autocrlf=true", "-c", "core.eol=crlf",
		"checkout-index", "--prefix=" + filepath.ToSlash(directory) + "/", "--", "README.md",
	}
	command := exec.Command("git", args...)
	command.Dir = ".."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Git CRLF checkout failed: %v\n%s", err, output)
	}
	data := readAgentFile(t, filepath.Join(directory, "README.md"))
	if !bytes.Contains(data, []byte("\r\n")) || bytes.Count(data, []byte("\r\n")) != bytes.Count(data, []byte("\n")) {
		t.Fatal("ordinary README Markdown must exercise Git's CRLF conversion, not an LF attribute pin")
	}
	readmeQuickstartInvocations(t, string(data))
}

func TestBinaryREADMEQuickstart(t *testing.T) {
	invocations := readmeQuickstartInvocations(t, string(readAgentFile(t, "../README.md")))
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
