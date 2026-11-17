package cmd_test

import (
	"bytes"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestContractQualifiedQueryNarrowsTenantAndSubscription(t *testing.T) {
	const otherTenant = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	for _, tc := range []struct {
		query string
		code  int
		id    string
	}{
		{"Acme/Production", 0, productionID},
		{otherTenant + "/Production", 0, developmentID},
		{"acme/PRD", 0, productionID},
		{"ACME.INVALID", 0, productionID},
		{"acme.invalid/PRD", 0, productionID},
		{"other.invalid/Production", 0, developmentID},
		{tenantID, 0, productionID},
		{"acme.invalid/" + developmentID, 2, ""},
		{"Acme/Acme", 2, ""},
		{"Acme/" + developmentID, 2, ""},
		{"missing/Production", 2, ""},
		{"/Production", 2, ""},
		{"Acme/", 2, ""},
	} {
		t.Run(tc.query, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			deps.FS = fstest.MapFS{
				profileFile: &fstest.MapFile{Data: []byte(`{"subscriptions":[
					{"id":"` + productionID + `","name":"Production","tenantId":"` + tenantID + `","tenantDisplayName":"Acme","tenantDefaultDomain":"acme.invalid","environmentName":"AzureCloud","isDefault":true},
					{"id":"` + developmentID + `","name":"Production","tenantId":"` + otherTenant + `","tenantDisplayName":"Other","tenantDefaultDomain":"other.invalid","environmentName":"AzureCloud"}]}`)},
			}
			var stdout, stderr bytes.Buffer
			code := cmd.Run([]string{tc.query}, deps, bytes.NewReader(nil), &stdout, &stderr)
			if code != tc.code || stdout.Len() != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if tc.id == "" && len(fake.Calls) != 0 || tc.id != "" && (len(fake.Calls) != 1 || fake.Calls[0].Args[3] != tc.id) {
				t.Fatalf("calls=%+v", fake.Calls)
			}
		})
	}
}
