package cmd_test

import (
	"bytes"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestContractMetadataAndEmptyList(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		args       []string
	}{
		{"metadata", `{"subscriptions":[{"id":"` + productionID + `","name":"Production","tenantId":"` + tenantID + `","isDefault":true,"state":"Enabled","user":{"name":"synthetic@example.invalid","type":"user"},"environmentName":"AzureCloud","tenantDisplayName":"Not emitted yet","tenantDefaultDomain":"example.invalid","managedByTenants":[{"tenantId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}]}]}`, []string{currentFlag, outputFlag, jsonFormat}},
		{"empty-list", `{"subscriptions":[]}`, []string{listCommand, outputFlag, jsonFormat}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			deps.FS = fstest.MapFS{
				profileFile:     &fstest.MapFile{Data: []byte(tc.data)},
				tenantCacheFile: &fstest.MapFile{Data: []byte(`[{"tenantId":"` + tenantID + `","displayName":"Legacy metadata is not the v1 source"}]`)},
			}
			var stdout, stderr bytes.Buffer
			if code := cmd.Run(tc.args, deps, bytes.NewReader(nil), &stdout, &stderr); code != 0 || stderr.Len() != 0 || len(fake.Calls) != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
			}
			assertContractGolden(t, tc.name, stdout.String())
		})
	}
}
