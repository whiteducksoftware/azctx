package cmd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	completionTenant      = "Completion Tenant"
	completionCommandName = "completion"
)

func completionDependencies(t *testing.T) (cmd.Dependencies, string, *testutil.FakeRunner) {
	t.Helper()
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	path := writeSelectionConfig(t, "aliases:\n  prod: "+productionID+"\nhidden: ["+developmentID+"]\n")
	subs := []profile.Subscription{
		{Id: productionID, Name: productionName, Tenant: tenantID, TenantName: completionTenant, TenantDomain: "acme.invalid", Environment: azureCloudName},
		{Id: developmentID, Name: developmentName, Tenant: tenantID, TenantName: completionTenant, Environment: azureCloudName},
	}
	for i := 2; i < 500; i++ {
		subs = append(subs, profile.Subscription{
			Id: fmt.Sprintf("%08x-1111-1111-1111-111111111111", i), Name: fmt.Sprintf("Subscription %03d", i),
			Tenant: tenantID, TenantName: completionTenant, Environment: azureCloudName,
		})
	}
	data, err := json.Marshal(profile.Profile{Subscriptions: subs})
	if err != nil {
		t.Fatal(err)
	}
	deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: data}}
	deps.CheckUpdates = func(string, string) error {
		t.Fatal("completion must not check for updates")
		return nil
	}
	return deps, path, fake
}

func TestCompletion500SubscriptionsIsOfflineAndFast(t *testing.T) {
	deps, path, fake := completionDependencies(t)
	root := cmd.NewRootCommand(deps)
	var durations []time.Duration
	for range 10 {
		start := time.Now()
		got, directive := root.ValidArgsFunction(root, nil, "")
		elapsed := time.Since(start)
		durations = append(durations, elapsed)
		assertCompletion(t, got, directive, []string{productionName, "prod", completionTenant + "/Production", "acme.invalid/Production"}, []string{developmentName})
		if elapsed >= 50*time.Millisecond {
			t.Fatalf("500-subscription completion took %s (limit 50ms)", elapsed)
		}
	}
	t.Logf("500 subscriptions: min=%s max=%s (%d runs)", slices.Min(durations), slices.Max(durations), len(durations))
	got, directive := root.ValidArgsFunction(root, nil, "COMPLETION TENANT/PROD")
	assertCompletion(t, got, directive, []string{completionTenant + "/Production"}, []string{"prod"})
	if err := root.ParseFlags([]string{metadataAllFlag}); err != nil {
		t.Fatal(err)
	}
	got, directive = root.ValidArgsFunction(root, nil, "Dev")
	assertCompletion(t, got, directive, []string{developmentName}, nil)
	deps.Runner = nil
	var out, messages bytes.Buffer
	status := cmd.Run([]string{"__complete", "pro"}, deps, strings.NewReader(""), &out, &messages)
	if status != 0 || !strings.Contains(out.String(), "prod\n") {
		t.Fatalf("completion resolved az: status=%d, out=%q, messages=%q", status, out.String(), messages.String())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "config.yaml" || len(fake.Calls) != 0 {
		t.Fatalf("completion side effects: files=%v err=%v calls=%+v", entries, err, fake.Calls)
	}
}

func assertCompletion(t *testing.T, got []string, directive cobra.ShellCompDirective, want, excluded []string) {
	t.Helper()
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("completion directive=%v candidates=%v", directive, got)
	}
	for _, value := range want {
		if !slices.Contains(got, value) {
			t.Fatalf("completion missing %q: %v", value, got)
		}
	}
	for _, value := range excluded {
		if slices.Contains(got, value) {
			t.Fatalf("completion unexpectedly includes %q", value)
		}
	}
}

func TestCompletionScriptsDoNotInvokeAzure(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		var out, messages bytes.Buffer
		args := []string{completionCommandName, shell}
		status := cmd.Run(args, deps, strings.NewReader(""), &out, &messages)
		if status != 0 || out.Len() == 0 || messages.Len() != 0 || len(fake.Calls) != 0 {
			t.Fatalf("%s completion generation status=%d, messages=%q, calls=%+v", shell, status, messages.String(), fake.Calls)
		}
	}
}
