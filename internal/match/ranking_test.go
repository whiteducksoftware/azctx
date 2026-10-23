package match_test

import (
	"reflect"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/match"
	"github.com/whiteducksoftware/azctx/internal/profile"
)

func TestDeterministicSelectionRanking(t *testing.T) {
	const production = "Production"
	subs := []profile.Subscription{
		{Id: "b", Name: "Production East", Tenant: "t2", TenantName: "Beta"},
		{Id: "a", Name: production, Tenant: "t1", TenantName: "Alpha"},
		{Id: "c", Name: production, Tenant: "t2", TenantName: "Beta"},
		{Id: "d", Name: "Dev", Tenant: "t1", TenantName: "Alpha"},
	}
	options := match.Options{Aliases: map[string]string{"prod": "b", production: "d", "a": "d"}, Pinned: []string{"c"}, Recent: []string{"b", "a"}}
	for _, tc := range []struct {
		query string
		ids   []string
	}{
		{production, []string{"c", "a", "d"}},
		{"a", []string{"a"}},
		{"PROD", []string{"b"}},
		{"Alpha/Production", []string{"a", "d"}},
		{"t2/prod", []string{"b"}},
		{"prd", []string{"c", "b", "a"}},
		{"missing", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got, err := match.FindWithOptions(subs, tc.query, options)
			if (err != nil) != (tc.ids == nil) {
				t.Fatalf("error = %v", err)
			}
			var ids []string
			for _, sub := range got {
				ids = append(ids, sub.Id)
			}
			if !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("IDs = %v, want %v", ids, tc.ids)
			}
		})
	}
	duplicates := []profile.Subscription{
		{Id: "same", Tenant: "one", Environment: "AzureCloud"},
		{Id: "same", Tenant: "two", Environment: "AzureCloud"},
	}
	got, err := match.FindWithOptions(duplicates, "same", match.Options{})
	if err != nil || len(got) != 2 {
		t.Fatalf("ID query lost distinct token tenant identities: %+v, %v", got, err)
	}
}

func TestAliasNameCollisionKeepsProfileOrder(t *testing.T) {
	const (
		query   = "Collision"
		aliasID = "alias-target"
		nameID  = "named-target"
	)
	subs := []profile.Subscription{
		{Id: aliasID, Name: "Other"},
		{Id: nameID, Name: query},
	}
	got, err := match.FindWithOptions(subs, query, match.Options{Aliases: map[string]string{query: aliasID}})
	if err != nil || len(got) != 2 || got[0].Id != aliasID || got[1].Id != nameID {
		t.Fatalf("equal-priority exact candidates must retain profile order: %+v, %v", got, err)
	}
}
