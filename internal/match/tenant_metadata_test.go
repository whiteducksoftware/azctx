package match_test

import (
	"reflect"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/match"
	"github.com/whiteducksoftware/azctx/internal/profile"
)

func TestFindMatchesProfileTenantNamesDomainsAndIDs(t *testing.T) {
	const firstTenant = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const secondTenant = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	const first = "11111111-1111-1111-1111-111111111111"
	const second = "22222222-2222-2222-2222-222222222222"
	const third = "33333333-3333-3333-3333-333333333333"
	const production = "Production"
	subscriptions := []profile.Subscription{
		{Id: first, Name: production, Tenant: firstTenant, TenantName: "Example", TenantDomain: "example.invalid"},
		{Id: second, Name: production, Tenant: secondTenant, TenantDomain: "domain-only.invalid"},
		{Id: third, Name: "Development", Tenant: firstTenant, TenantName: "Example", TenantDomain: "example.invalid"},
	}
	for _, tc := range []struct {
		query string
		ids   []string
	}{
		{"EXAMPLE", []string{first, third}},
		{"EXAMPLE.INVALID", []string{first, third}},
		{"DOMAIN-ONLY.INVALID", []string{second}},
		{firstTenant, []string{first, third}},
		{"EXAMPLE.INVALID/PRD", []string{first}},
		{"DOMAIN-ONLY.INVALID/Production", []string{second}},
		{firstTenant + "/Production", []string{first}},
		{"Example/Production", []string{first}},
		{"EXAMPLE.INVALID/" + second, nil},
		{"DOMAIN-ONLY/Production", nil},
		{"EXAMPLE.INVALID/EXAMPLE.INVALID", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got, err := match.Find(subscriptions, tc.query)
			var ids []string
			for _, sub := range got {
				ids = append(ids, sub.Id)
			}
			if !reflect.DeepEqual(ids, tc.ids) || (err != nil) != (tc.ids == nil) {
				t.Fatalf("IDs=%v err=%v want=%v", ids, err, tc.ids)
			}
		})
	}
}
