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
	const tenant = "tenant-id"
	profile := `{"subscriptions":[{"id":"sub","name":"Production","tenantId":"tenant-id"},{"id":"tenant-id","name":"N/A(tenant level account)","isDefault":true}]}`
	files := fstest.MapFS{
		profileFilename:     &fstest.MapFile{Data: []byte(profile)},
		"azctxTenants.json": &fstest.MapFile{Data: []byte("\xef\xbb\xbf" + `[{"tenantId":"tenant-id","displayName":"Acme"}]`)},
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
	active, err := cli.ActiveSubscription()
	if err != nil || active.Id != tenant {
		t.Fatalf("active tenant-level account = %+v, %v", active, err)
	}
	cli.FilterTenantLevelAccount = false
	if len(cli.Subscriptions()) != 2 {
		t.Fatal("tenant-level filtering cannot be disabled")
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
