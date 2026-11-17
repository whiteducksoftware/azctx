package profile_test

import (
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

const (
	profileName      = "azureProfile.json"
	userType         = "user"
	spType           = "servicePrincipal"
	enabledState     = "Enabled"
	syntheticSubName = "Synthetic"
)

func TestTenantLabelFallbacks(t *testing.T) {
	const tenantName = "Example"
	const tenantID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const tenantDomain = "example.invalid"
	for _, tc := range []struct {
		name, domain, want string
	}{
		{tenantName, tenantDomain, tenantName},
		{tenantName, "", tenantName},
		{"", tenantDomain, tenantDomain},
		{"", "", tenantID},
		{" \t", tenantDomain, tenantDomain},
		{"", " \t", tenantID},
	} {
		sub := profile.Subscription{Tenant: tenantID, TenantName: tc.name, TenantDomain: tc.domain}
		if got := sub.TenantLabel(); got != tc.want {
			t.Errorf("name=%q domain=%q: label=%q want=%q", tc.name, tc.domain, got, tc.want)
		}
	}
}

func TestModernTenantMetadata(t *testing.T) {
	files := fstest.MapFS{profileName: &fstest.MapFile{Data: []byte("\xef\xbb\xbf" + `{"subscriptions":[{
		"id":"11111111-1111-1111-1111-111111111111",
		"tenantId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"tenantDisplayName":"Example Directory","tenantDefaultDomain":"example.invalid",
		"homeTenantId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		"managedByTenants":[{"tenantId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}],
		"user":{"name":"systemAssignedIdentity","type":"servicePrincipal"}
	}]}`)}}
	got, err := profile.Read(files, profileName)
	if err != nil {
		t.Fatal(err)
	}
	sub := got.Subscriptions[0]
	if sub.TenantName != "Example Directory" || sub.TenantDomain != "example.invalid" || !sub.Delegated() ||
		sub.AccountType() != "system-assigned identity" || len(sub.ManagedBy) != 1 ||
		sub.ManagedBy[0].Id != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("tenant metadata missing: %+v", sub)
	}
}

func TestProfileIdentityLabelsAndDelegation(t *testing.T) {
	for _, tc := range []struct {
		name, kind, label string
	}{
		{"synthetic@example.invalid", userType, userType},
		{"synthetic-app", spType, "service principal"},
		{"systemAssignedIdentity", spType, "system-assigned identity"},
		{"userAssignedIdentity", spType, "user-assigned identity"},
		{"synthetic", "future-type", "future-type"},
		{"", "", ""},
	} {
		sub := profile.Subscription{User: profile.User{Name: tc.name, Type: tc.kind}}
		if sub.AccountType() != tc.label {
			t.Fatalf("account=%+v label=%q want=%q", sub.User, sub.AccountType(), tc.label)
		}
	}
	for _, home := range []string{"", "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"} {
		sub := profile.Subscription{
			Tenant: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", HomeTenantId: home,
			ManagedBy: []profile.ManagedTenant{{Id: "cccccccc-cccc-cccc-cccc-cccccccccccc"}},
		}
		if got := sub.Delegated(); got != (home == "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb") {
			t.Fatalf("home=%q delegated=%v", home, got)
		}
		if sub.TenantLabel() != sub.Tenant {
			t.Fatalf("legacy tenant fallback=%q", sub.TenantLabel())
		}
	}
	for _, status := range []string{enabledState, "Warned", "PastDue"} {
		sub := profile.Subscription{Name: syntheticSubName, State: status}
		want := syntheticSubName
		if status != enabledState {
			want += " [" + status + "]"
		}
		if sub.DisplayName() != want || sub.Name != syntheticSubName {
			t.Fatalf("display label=%q, identity=%q", sub.DisplayName(), sub.Name)
		}
	}
}

func TestProfileMalformedModernFields(t *testing.T) {
	for _, data := range []string{
		`{"subscriptions":"bad"}`, `{"subscriptions":[{"tenantDisplayName":1}]}`,
		`{"subscriptions":[{"tenantDefaultDomain":[]}]}`, `{"subscriptions":[{"managedByTenants":"bad"}]}`,
		`{"subscriptions":[{"homeTenantId":{}}]}`, `{"subscriptions":[{"user":"bad"}]}`,
	} {
		if _, err := profile.Read(fstest.MapFS{profileName: &fstest.MapFile{Data: []byte(data)}}, profileName); err == nil {
			t.Fatalf("accepted malformed profile %s", data)
		}
	}
}
