package profile_test

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

func TestRead(t *testing.T) {
	const tenantID = "tenant"
	for _, tc := range []struct {
		name string
		data string
		want []profile.Subscription
	}{
		{
			name: "normal profile",
			data: `{"installationId":"installation","subscriptions":[{"id":"sub","name":"Production","state":"Enabled","user":{"name":"user@example.test","type":"user"},"isDefault":true,"tenantId":"tenant","environmentName":"AzureCloud","homeTenantId":"home","managedByTenants":[{"tenantId":"manager"}]}]}`,
			want: []profile.Subscription{{
				Id: "sub", Name: "Production", State: "Enabled",
				User:      profile.User{Name: "user@example.test", Type: "user"},
				IsDefault: true, Tenant: tenantID, Environment: "AzureCloud",
				HomeTenantId: "home", ManagedBy: []profile.ManagedTenant{{Id: "manager"}},
			}},
		},
		{
			name: "UTF8 BOM",
			data: "\xef\xbb\xbf" + `{"subscriptions":[{"id":"bom"}]}`,
			want: []profile.Subscription{{Id: "bom"}},
		},
		{
			name: "duplicate names retain distinct subscriptions",
			data: `{"subscriptions":[{"id":"one","name":"prod","tenantId":"first"},{"id":"two","name":"prod","tenantId":"second"}]}`,
			want: []profile.Subscription{{Id: "one", Name: "prod", Tenant: "first"}, {Id: "two", Name: "prod", Tenant: "second"}},
		},
		{
			name: "tenant level account",
			data: `{"subscriptions":[{"id":"tenant","name":"N/A(tenant level account)","tenantId":"tenant","isDefault":true}]}`,
			want: []profile.Subscription{{Id: tenantID, Name: "N/A(tenant level account)", Tenant: tenantID, IsDefault: true}},
		},
		{
			name: "service principal",
			data: `{"subscriptions":[{"id":"sp","user":{"name":"app-id","type":"servicePrincipal"}}]}`,
			want: []profile.Subscription{{Id: "sp", User: profile.User{Name: "app-id", Type: "servicePrincipal"}}},
		},
		{
			name: "unknown and missing fields",
			data: `{"future":true,"subscriptions":[{"newField":{"anything":1},"user":{"future":true}},{}]}`,
			want: []profile.Subscription{{}, {}},
		},
		{name: "missing subscriptions", data: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{"azureProfile.json": &fstest.MapFile{Data: []byte(tc.data)}}
			got, err := profile.Read(files, "azureProfile.json")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Subscriptions, tc.want) {
				t.Fatalf("subscriptions = %#v, want %#v", got.Subscriptions, tc.want)
			}
			if tc.name == "normal profile" && got.InstallationId != "installation" {
				t.Fatalf("installation ID = %q", got.InstallationId)
			}
		})
	}
}

func TestReadErrors(t *testing.T) {
	_, err := profile.Read(fstest.MapFS{}, "azureProfile.json")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing profile error = %v", err)
	}
	_, err = profile.Read(fstest.MapFS{"bad.json": &fstest.MapFile{Data: []byte(`{`)}}, "bad.json")
	if err == nil {
		t.Fatal("invalid JSON must fail")
	}
}
