package azcli_test

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestCLIReadsInjectedConfiguration(t *testing.T) {
	const tenant = "tenant-id"
	files := afero.NewMemMapFs()
	configDir := filepath.Join("fixture", "azure")
	if err := files.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	profile := `{"subscriptions":[{"id":"sub","name":"Production","tenantId":"tenant-id"},{"id":"tenant-id","name":"N/A(tenant level account)","isDefault":true}]}`
	if err := afero.WriteFile(files, filepath.Join(configDir, "azureProfile.json"), []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(files, filepath.Join(configDir, "azctxTenants.json"), []byte("\xef\xbb\xbf"+`[{"tenantId":"tenant-id","displayName":"Acme"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AZURE_CONFIG_DIR", configDir)
	t.Setenv("PATH", "")
	fake := &testutil.FakeRunner{}
	cli, err := azcli.New(afero.NewReadOnlyFs(files), fake, "")
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
}

func TestCLIConfigurationErrors(t *testing.T) {
	files := afero.NewMemMapFs()
	if _, err := azcli.New(files, &testutil.FakeRunner{}, "absent"); err == nil {
		t.Fatal("missing configuration directory must fail")
	}
	if err := files.MkdirAll("config", 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := azcli.New(files, &testutil.FakeRunner{}, "config"); err == nil {
		t.Fatal("missing profile must fail")
	}
	if err := afero.WriteFile(files, filepath.Join("config", "azureProfile.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cli, err := azcli.New(files, &testutil.FakeRunner{}, "config")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ActiveSubscription(); err == nil {
		t.Fatal("missing default subscription must fail")
	}
}
