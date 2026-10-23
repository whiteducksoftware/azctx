package cmd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestAgentExecGuidanceRequiresProjectAndTargetGuards(t *testing.T) {
	for _, file := range []string{"../skills/azctx/SKILL.md", agentPlugin + "skills/azctx/SKILL.md", "../docs/ai-agents.md"} {
		t.Run(file, func(t *testing.T) {
			document := strings.Join(strings.Fields(string(readAgentFile(t, file))), " ")
			for _, required := range []string{
				"`azctx exec` is not hook-guarded",
				"subscription `id` and `tenantId` GUIDs",
				"explicit guard flags replace project expectations",
				"current default against the resolved target",
				"both guards succeed",
			} {
				if !strings.Contains(document, required) {
					t.Errorf("exec safety guidance missing %q", required)
				}
			}
			agentExecPreflight(t, file)
		})
	}
}

func agentExecPreflight(t *testing.T, file string) [][]string {
	t.Helper()
	document := strings.ReplaceAll(string(readAgentFile(t, file)), "\r\n", "\n")
	document = strings.ReplaceAll(document, "\\\n", " ")
	lines := strings.Split(document, "\n")
	for index := range lines {
		lines[index] = strings.TrimLeft(lines[index], " \t")
	}
	document = strings.Join(lines, "\n")
	for _, block := range strings.Split(document, "```sh\n")[1:] {
		block = strings.SplitN(block, "\n```", 2)[0]
		snippets := agentCommandSnippets("```sh\n" + block + "\n```\n")
		if len(snippets) == 0 || !strings.HasPrefix(snippets[len(snippets)-1].text, "azctx exec ") {
			continue
		}
		want := [][]string{
			{guardCommand, noInputFlag, outputFlag, jsonFormat},
			{guardCommand, noInputFlag, subscriptionFlag, productionID, tenantFlag, tenantID, outputFlag, jsonFormat},
			{execSubcommand, noInputFlag, productionID, argumentDelimiter, "terraform", "plan", "-input=false"},
		}
		if len(snippets) != len(want) {
			t.Fatalf("%s: exec example needs project guard, target guard and exec, got %d commands", file, len(snippets))
		}
		var commands [][]string
		for index, snippet := range snippets {
			args := expandAgentSynopsis(snippet.text)
			if len(args) != 1 || !slices.Equal(args[0], want[index]) {
				t.Fatalf("%s: preflight step %d: got %q, want %q", file, index+1, args, want[index])
			}
			if index < 2 && !strings.HasSuffix(strings.TrimSpace(snippet.text), "&&") {
				t.Fatalf("%s: guard step %d must short-circuit on every nonzero status", file, index+1)
			}
			commands = append(commands, args[0])
		}
		return commands
	}
	t.Fatalf("%s: missing guarded exec workflow", file)
	return nil
}

func TestAgentExecDocumentedPreflight(t *testing.T) {
	commands := agentExecPreflight(t, "../skills/azctx/SKILL.md")
	bin := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(bin, testutil.ExecutableName("terraform")), childFixturePackage)
	for _, tc := range []struct {
		name, activeSubscription, activeTenant, project string
		statuses                                        []int
		explicitAlone                                   bool
	}{
		{"all match", productionID, tenantID, projectYAML(productionID, tenantID), []int{0, 0, 0}, false},
		{"active equals target but not project subscription", productionID, tenantID, projectYAML(developmentID, tenantID), []int{4}, true},
		{"active equals target but not project tenant", productionID, tenantID, projectYAML(productionID, secondTenantID), []int{4}, true},
		{"active equals project but not target subscription", developmentID, tenantID, projectYAML(developmentID, tenantID), []int{0, 4}, false},
		{"resolved target tenant is no longer active", productionID, secondTenantID, projectYAML(productionID, secondTenantID), []int{0, 4}, false},
		{"missing project is not a fallback", productionID, tenantID, "", []int{1}, true},
		{"malformed project is not a fallback", productionID, tenantID, "login: true\n", []int{1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			data := fmt.Sprintf(`{"subscriptions":[{"id":%q,"name":"Active","tenantId":%q,"environmentName":"AzureCloud","isDefault":true}]}`, tc.activeSubscription, tc.activeTenant)
			deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(data)}}
			t.Setenv("PATH", bin)
			t.Setenv(childModeEnv, "echo")
			dir := testutil.WorkDir(t)
			if tc.project != "" {
				writeFixture(t, filepath.Join(dir, projectFile), tc.project)
			}
			t.Chdir(dir)
			var out, messages bytes.Buffer
			var statuses []int
			for _, args := range commands {
				status := cmd.Run(args, deps, strings.NewReader(""), &out, &messages)
				statuses = append(statuses, status)
				if status != 0 {
					break // The documented && chain must not execute later steps.
				}
			}
			if !slices.Equal(statuses, tc.statuses) || len(fake.Calls) != 0 {
				t.Fatalf("statuses=%v want=%v Azure calls=%+v stderr=%q", statuses, tc.statuses, fake.Calls, &messages)
			}
			if len(statuses) == 3 {
				assertAgentExecPinnedTarget(t, out.Bytes())
			} else if out.Len() != 0 {
				t.Fatalf("failed preflight ran child: %q", &out)
			}
			if tc.explicitAlone {
				out.Reset()
				messages.Reset()
				if status := cmd.Run(commands[1], deps, strings.NewReader(""), &out, &messages); status != 0 {
					t.Fatalf("explicit guard alone must ignore project: status=%d stderr=%q", status, &messages)
				}
			}
		})
	}
}

//nolint:tagliatelle // The child fixture reports public uppercase environment-variable names.
func assertAgentExecPinnedTarget(t *testing.T, output []byte) {
	t.Helper()
	var result struct {
		Env struct {
			ARMSubscription   string `json:"ARM_SUBSCRIPTION_ID"`
			ARMTenant         string `json:"ARM_TENANT_ID"`
			AzureSubscription string `json:"AZURE_SUBSCRIPTION_ID"`
			AzureTenant       string `json:"AZURE_TENANT_ID"`
		} `json:"env"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	got := []string{result.Env.ARMSubscription, result.Env.ARMTenant, result.Env.AzureSubscription, result.Env.AzureTenant}
	if want := []string{productionID, tenantID, productionID, tenantID}; !slices.Equal(got, want) {
		t.Errorf("child context=%q, want pinned GUIDs %q", got, want)
	}
}
