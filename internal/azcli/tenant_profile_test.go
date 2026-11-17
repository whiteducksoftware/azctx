package azcli_test

import (
	"bytes"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestTenantLoginUsesProfileGUIDsNotDisplayNames(t *testing.T) {
	t.Setenv("AZURE_CLOUD_NAME", publicCloud)
	const first = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const second = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{true: "invalid GUID", false: "valid GUIDs"}[invalid], func(t *testing.T) {
			last := second
			if invalid {
				last = "--not-a-guid"
			}
			data := `{"subscriptions":[
				{"tenantId":"` + first + `","tenantDisplayName":"Duplicate","environmentName":"AzureCloud"},
				{"tenantId":"AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA","tenantDisplayName":"Other name","environmentName":"AzureCloud"},
				{"tenantId":"` + last + `","tenantDisplayName":"Duplicate","environmentName":"AzureCloud"},
				{"tenantId":"cccccccc-cccc-cccc-cccc-cccccccccccc","environmentName":"CustomCloud"}]}`
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{Result: azcli.Result{Stdout: `{"azure-cli":"2.61.0"}`}}, {}, {},
			}}
			cli, err := azcli.New(fstest.MapFS{profileFilename: &fstest.MapFile{Data: []byte(data)}}, fake, "")
			if err != nil {
				t.Fatal(err)
			}
			if cli.TenantCount() != 2 {
				t.Fatalf("tenant identity count=%d", cli.TenantCount())
			}
			var terminal bytes.Buffer
			cli.Terminal = &azcli.Terminal{Stdout: &terminal, Stderr: &terminal}
			err = cli.IterativeTenantLogin(nil)
			if invalid {
				if err == nil || len(fake.Calls) != 0 {
					t.Fatalf("invalid profile tenant reached az: %v, %+v", err, fake.Calls)
				}
				return
			}
			if err != nil || len(fake.Calls) != 3 {
				t.Fatalf("profile-only tenant login=%v, %+v", err, fake.Calls)
			}
			for i, id := range []string{first, second} {
				call := fake.Calls[i+1]
				if !reflect.DeepEqual(call.Args, []string{loginCommand, "--tenant", id}) ||
					!reflect.DeepEqual(call.Env, []string{"AZURE_CORE_LOGIN_EXPERIENCE_V2=off"}) || !call.Attached {
					t.Fatalf("tenant login=%+v", call)
				}
			}
		})
	}
}
