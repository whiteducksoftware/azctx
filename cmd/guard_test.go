package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	guardCommand  = "guard"
	projectFile   = ".azctx.yaml"
	guardRequired = "required"
)

func TestGuardFlagsAndProjectContext(t *testing.T) {
	for _, tc := range []struct {
		name, project string
		args          []string
		code          int
		diagnostic    string
	}{
		{"flags match", "", []string{subscriptionFlag, productionName, tenantFlag, "example.invalid"}, 0, ""},
		{"subscription differs", "", []string{subscriptionFlag, developmentID}, 4, guardRequired},
		{"tenant differs", "", []string{tenantFlag, secondTenantID}, 4, guardRequired},
		{"both differ", "", []string{subscriptionFlag, developmentID, tenantFlag, secondTenantID}, 4, guardRequired},
		{"no file", "", nil, 1, "no .azctx.yaml"},
		{"project matches", projectYAML(productionID, tenantID), nil, 0, ""},
		{"project differs", projectYAML(developmentID, tenantID), nil, 4, guardRequired},
		{"flags override untrusted malformed file", "login: true\n", []string{subscriptionFlag, productionID}, 0, ""},
		{"unknown key", projectYAML(productionID, tenantID) + "login: true\n", nil, 1, "line 4: unknown key"},
		{"invalid GUID", projectYAML("$(touch nope)", tenantID), nil, 1, "line 2: subscription must be a GUID"},
		{"invalid tenant", projectYAML(productionID, "bad"), nil, 1, "line 3: tenant must be a GUID"},
		{"wrong version", strings.Replace(projectYAML(productionID, tenantID), "version: 1", "version: 2", 1), nil, 1, "line 1: version must be 1"},
		{"missing version", "subscription: " + productionID + "\ntenant: " + tenantID + "\n", nil, 1, "version: 1 is required"},
		{"missing tenant", "version: 1\nsubscription: " + productionID + "\n", nil, 1, "tenant is required"},
		{"duplicate", projectYAML(productionID, tenantID) + "tenant: " + tenantID + "\n", nil, 1, "line 4: duplicate"},
		{"multiple documents", projectYAML(productionID, tenantID) + "---\n" + projectYAML(developmentID, tenantID), nil, 1, "only one YAML document"},
		{"empty second document", projectYAML(productionID, tenantID) + "---\n", nil, 1, "only one YAML document"},
		{"scalar document", "not-a-mapping\n", nil, 1, "line 1:"},
		{"empty flag", "", []string{subscriptionFlag, ""}, 1, "must not be empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			deps.Runner = nil
			root := testutil.WorkDir(t)
			child := filepath.Join(root, "repo", "nested")
			if err := os.MkdirAll(child, 0o700); err != nil {
				t.Fatal(err)
			}
			if tc.project != "" {
				writeFixture(t, filepath.Join(root, projectFile), tc.project)
			}
			t.Chdir(child)
			var out, messages bytes.Buffer
			code := cmd.Run(append([]string{guardCommand}, tc.args...), deps, strings.NewReader(""), &out, &messages)
			if code != tc.code || out.Len() != 0 || len(fake.Calls) != 0 ||
				(tc.diagnostic != "" && !strings.Contains(messages.String(), tc.diagnostic)) ||
				(tc.code == 0 && messages.Len() != 0) {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, &out, &messages, fake.Calls)
			}
			if tc.code == 1 && tc.project != "" && !strings.Contains(messages.String(), filepath.Join(root, projectFile)) {
				t.Fatalf("project diagnostic has no filename: %q", &messages)
			}
		})
	}
}

func projectYAML(subscription, tenant string) string {
	return "version: 1\nsubscription: " + subscription + "\ntenant: " + tenant + "\n"
}

func TestGuardNearestProjectAndCurrentWarning(t *testing.T) {
	deps := commandDependencies(t, &testutil.FakeRunner{})
	root := testutil.WorkDir(t)
	repo := filepath.Join(root, "repo")
	nested := filepath.Join(repo, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(root, projectFile), projectYAML(developmentID, tenantID))
	writeFixture(t, filepath.Join(repo, projectFile), projectYAML(productionID, tenantID))
	t.Chdir(nested)
	if got := configCommandOutput(t, deps, guardCommand); got != "" {
		t.Fatalf("matching guard output: %q", got)
	}
	writeFixture(t, filepath.Join(repo, projectFile), projectYAML(developmentID, tenantID))
	assertProjectCurrentWarnings(t, deps)
	var out, messages bytes.Buffer
	code := cmd.Run([]string{guardCommand, outputFlag, jsonFormat}, deps, strings.NewReader(""), &out, &messages)
	want := "{\"code\":\"context_mismatch\",\"message\":\"active context does not match the required subscription or tenant\",\"exit_code\":4}\n"
	if code != 4 || messages.String() != want || out.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &messages)
	}
	writeFixture(t, filepath.Join(repo, projectFile), "version: 2\n")
	out.Reset()
	messages.Reset()
	if code := cmd.Run([]string{currentFlag}, deps, strings.NewReader(""), &out, &messages); code != 0 ||
		!strings.Contains(messages.String(), "version must be 1") || out.Len() == 0 {
		t.Fatalf("invalid project current: %d, %q, %q", code, &out, &messages)
	}
	for _, format := range []string{jsonFormat, idFormat, nameFormat, tenantSubcommand} {
		out.Reset()
		messages.Reset()
		if code := cmd.Run([]string{currentFlag, outputFlag, format}, deps, strings.NewReader(""), &out, &messages); code != 0 || messages.Len() != 0 {
			t.Fatalf("invalid project contaminated machine current: %s %d %q", format, code, &messages)
		}
	}
}

func assertProjectCurrentWarnings(t *testing.T, deps cmd.Dependencies) {
	t.Helper()
	for _, format := range []string{"", textFormat, jsonFormat, idFormat, nameFormat, tenantSubcommand} {
		var out, messages bytes.Buffer
		args := []string{currentFlag}
		if format != "" {
			args = append(args, outputFlag, format)
		}
		code := cmd.Run(args, deps, strings.NewReader(""), &out, &messages)
		human := format == "" || format == textFormat
		if code != 0 || out.Len() == 0 || (human != strings.Contains(messages.String(), projectFile)) {
			t.Fatalf("%s code=%d stdout=%q stderr=%q", format, code, &out, &messages)
		}
	}
}
