package azcli_test

import (
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const profileFilename = "azureProfile.json"

func TestCLIReadsInjectedConfiguration(t *testing.T) {
	t.Setenv("AZURE_CLOUD_NAME", publicCloud)
	const tenant = "tenant-id"
	profile := `{"subscriptions":[{"id":"sub","name":"Production","tenantId":"tenant-id","tenantDisplayName":"Acme","environmentName":"AzureCloud"},{"id":"tenant-id","name":"N/A(tenant level account)","environmentName":"AzureCloud","isDefault":true}]}`
	files := fstest.MapFS{
		profileFilename: &fstest.MapFile{Data: []byte(profile)},
	}
	t.Setenv("AZURE_CONFIG_DIR", filepath.Join("fixture", "azure"))
	t.Setenv("PATH", "")
	fake := &testutil.FakeRunner{}
	cli, err := azcli.New(files, fake, "")
	if err != nil {
		t.Fatal(err)
	}
	subscriptions := cli.Subscriptions()
	if len(subscriptions) != 1 || subscriptions[0].TenantName != "Acme" {
		t.Fatalf("filtered subscriptions = %+v", subscriptions)
	}
	if active, err := cli.ActiveSubscription(); err == nil {
		t.Fatalf("hidden tenant-level account marked current = %+v", active)
	}
	cli.IncludeAll = true
	if len(cli.Subscriptions()) != 2 {
		t.Fatal("tenant-level filtering cannot be disabled")
	}
	active, err := cli.ActiveSubscription()
	if err != nil || active.Id != tenant {
		t.Fatalf("active --all tenant-level account = %+v, %v", active, err)
	}
	if err := cli.Reload(); err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("read-only operations invoked az: %+v", fake.Calls)
	}
	contents, err := fs.ReadFile(files, profileFilename)
	if err != nil || string(contents) != profile {
		t.Fatalf("profile changed: %q, %v", contents, err)
	}
}

func TestCLIConfigurationErrors(t *testing.T) {
	t.Setenv("AZURE_CLOUD_NAME", publicCloud)
	if _, err := (azcli.CLI{}).ActiveSubscription(); err == nil {
		t.Fatal("an unloaded CLI must have no active subscription")
	}
	files := fstest.MapFS{}
	if _, err := azcli.New(files, &testutil.FakeRunner{}, "absent"); err == nil {
		t.Fatal("missing profile must fail")
	}
	files[profileFilename] = &fstest.MapFile{Data: []byte(`{}`)}
	cli, err := azcli.New(files, &testutil.FakeRunner{}, "config")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ActiveSubscription(); err == nil {
		t.Fatal("missing default subscription must fail")
	}
}
