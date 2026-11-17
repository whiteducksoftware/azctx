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
	"github.com/whiteducksoftware/azctx/internal/match"
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

func TestCompletion500SubscriptionsIsOffline(t *testing.T) {
	checkCompletion500Subscriptions(t)
}

func checkCompletion500Subscriptions(t *testing.T) []time.Duration {
	t.Helper()
	deps, path, fake := completionDependencies(t)
	root := cmd.NewRootCommand(deps)
	want := []string{productionName, exportAliasName, completionTenant + "/Production", "acme.invalid/Production", tenantID + "/Production"}
	for i := 2; i < 500; i++ {
		name := fmt.Sprintf("Subscription %03d", i)
		want = append(want, name, completionTenant+"/"+name, tenantID+"/"+name)
	}
	slices.Sort(want)
	var durations []time.Duration
	for range 10 {
		start := time.Now()
		got, directive := root.ValidArgsFunction(root, nil, "")
		elapsed := time.Since(start)
		durations = append(durations, elapsed)
		assertCompletion(t, got, directive, []string{productionName, exportAliasName, completionTenant + "/Production", "acme.invalid/Production"}, []string{developmentName})
		if !slices.Equal(got, want) {
			t.Fatalf("500-subscription completion candidates differ: got %d, want %d sorted names, tenant-qualified names and aliases", len(got), len(want))
		}
	}
	t.Logf("500 subscriptions: min=%s max=%s (%d runs)", slices.Min(durations), slices.Max(durations), len(durations))
	got, directive := root.ValidArgsFunction(root, nil, "COMPLETION TENANT/PROD")
	assertCompletion(t, got, directive, []string{completionTenant + "/Production"}, []string{exportAliasName})
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
	return durations
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

func TestCompletionSlashNamesAndTenantLabelsRemainResolvable(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	subs := []profile.Subscription{
		{Id: productionID, Name: "Build/Release", Tenant: tenantID, TenantName: "Engineering/Tools", TenantDomain: "tools.invalid", Environment: azureCloudName},
		{Id: developmentID, Name: "Operations", Tenant: tenantID, TenantName: "Operations/Directory", TenantDomain: "ops/invalid", Environment: azureCloudName},
	}
	data, err := json.Marshal(profile.Profile{Subscriptions: subs})
	if err != nil {
		t.Fatal(err)
	}
	deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: data}}
	root := cmd.NewRootCommand(deps)
	want := []string{productionID, "Operations", tenantID + "/Operations"}
	slices.Sort(want)
	got, directive := root.ValidArgsFunction(root, nil, "")
	assertCompletion(t, got, directive, want, []string{"Build/Release", "Engineering/Tools/Build/Release", "Operations/Directory/Operations", "ops/invalid/Operations"})
	if !slices.Equal(got, want) {
		t.Fatalf("slash completion candidates=%q, want %q", got, want)
	}
	for _, candidate := range got {
		if matches, err := match.Find(subs, candidate); err != nil || len(matches) != 1 {
			t.Fatalf("completion %q does not resolve to one subscription: matches=%+v err=%v", candidate, matches, err)
		}
	}
	got, directive = root.ValidArgsFunction(root, nil, productionID[:8])
	assertCompletion(t, got, directive, []string{productionID}, nil)
	got, directive = root.ValidArgsFunction(root, nil, "Operations/")
	assertCompletion(t, got, directive, nil, []string{"Operations/Directory/Operations"})
	if len(got) != 0 || len(fake.Calls) != 0 {
		t.Fatalf("invalid prefix candidates=%q calls=%+v", got, fake.Calls)
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
