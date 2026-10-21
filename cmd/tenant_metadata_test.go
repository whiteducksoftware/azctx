package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	metadataTenantName   = "Example"
	metadataTenantDomain = "example.invalid"
	metadataAllFlag      = "--all"
)

func tenantMetadataProfile(t *testing.T, name, domain, unrelated string) fstest.MapFS {
	t.Helper()
	old := profile.Subscription{Id: "33333333-3333-3333-3333-333333333333", Name: "Unrelated", Tenant: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Environment: azureCloudName}
	switch unrelated {
	case "disabled":
		old.State = "Disabled"
	case "foreign":
		old.Environment = customCloudName
	case "missing-environment":
		old.Environment = ""
	case "tenant-level":
		old.Name = "N/A(tenant level account)"
	}
	data, err := json.Marshal(profile.Profile{Subscriptions: []profile.Subscription{
		{Id: productionID, Name: productionName, Tenant: tenantID, TenantName: name, TenantDomain: domain, Environment: azureCloudName, IsDefault: true},
		{Id: developmentID, Name: productionName, Tenant: tenantID, TenantName: name, TenantDomain: domain, Environment: azureCloudName},
		old,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{profileFile: &fstest.MapFile{Data: data}}
}

func TestAzureCLITenantHintsUseDisplayedEntries(t *testing.T) {
	for _, metadata := range []struct {
		name, domain string
		legacy       bool
	}{
		{metadataTenantName, metadataTenantDomain, false},
		{metadataTenantName, "", false},
		{"", metadataTenantDomain, false},
		{"", "", true},
		{" \t", " \t", true},
	} {
		for _, unrelated := range []string{"noncurrent", "disabled", "foreign", "missing-environment", "tenant-level"} {
			for _, action := range []struct {
				args   []string
				picker bool
				hint   bool
			}{
				{[]string{currentFlag}, false, metadata.legacy},
				{[]string{productionName}, true, metadata.legacy},
				{[]string{listCommand}, false, metadata.legacy || unrelated == "noncurrent"},
				{[]string{listCommand, metadataAllFlag}, false, true},
			} {
				t.Run(metadata.name+"/"+metadata.domain+"/"+unrelated+"/"+strings.Join(action.args, " "), func(t *testing.T) {
					fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
					deps := commandDependencies(t, fake)
					deps.FS = tenantMetadataProfile(t, metadata.name, metadata.domain, unrelated)
					picked := false
					deps.Select = func(subscriptions []profile.Subscription) (int, error) {
						picked = true
						if len(subscriptions) != 2 || subscriptions[0].Id != productionID || subscriptions[1].Id != developmentID {
							t.Fatalf("narrowed candidates=%+v", subscriptions)
						}
						return 1, nil
					}
					var stdout, stderr bytes.Buffer
					if code := cmd.Run(action.args, deps, bytes.NewReader(nil), &stdout, &stderr); code != 0 {
						t.Fatalf("code=%d stderr=%q", code, stderr.String())
					}
					want := ""
					if action.hint {
						want = "Tenant metadata is missing; run `az login` once with Azure CLI 2.61.0 or newer.\n"
					}
					if stderr.String() != want || picked != action.picker {
						t.Fatalf("stderr=%q want=%q picked=%t want=%t", stderr.String(), want, picked, action.picker)
					}
				})
			}
		}
	}
}

func TestAzureCLITenantMetadataMachineModesStayQuiet(t *testing.T) {
	for _, args := range [][]string{
		{currentFlag, outputFlag, "id"},
		{currentFlag, outputFlag, "name"},
		{currentFlag, outputFlag, "tenant"},
		{currentFlag, outputFlag, jsonFormat},
		{listCommand, metadataAllFlag, outputFlag, jsonFormat},
		{developmentID},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			deps.FS = tenantMetadataProfile(t, "", "", "noncurrent")
			var stdout, stderr bytes.Buffer
			if code := cmd.Run(args, deps, bytes.NewReader(nil), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestAzureCLIDomainOnlyProjectionAndLegacyDetection(t *testing.T) {
	files := tenantMetadataProfile(t, "", metadataTenantDomain, "foreign")
	cli, err := azcli.New(files, &testutil.FakeRunner{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if cli.LegacyProfile() {
		t.Fatal("domain-only profile is modern")
	}
	cli.IncludeAll = true
	if !cli.LegacyProfile() {
		t.Fatal("inspection of displayed legacy foreign records must report missing metadata")
	}
	output := cmd.SubscriptionOutputs(cli.Subscriptions())
	if output[0].TenantName != metadataTenantDomain || output[0].TenantDomain != metadataTenantDomain {
		t.Fatalf("domain-only JSON projection=%+v", output[0])
	}
}
