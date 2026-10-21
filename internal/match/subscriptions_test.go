package match_test

import (
	"reflect"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/match"
	"github.com/whiteducksoftware/azctx/internal/profile"
)

func TestFindUsesSubscriptionIdentity(t *testing.T) {
	const firstID = "one"
	const secondID = "two"
	subscriptions := []profile.Subscription{
		{Id: firstID, Name: "Production", TenantName: "Acme"},
		{Id: secondID, Name: "Production", TenantName: "Contoso"},
		{Id: "three", Name: "Development", TenantName: "Acme"},
	}
	for _, tc := range []struct {
		query string
		ids   []string
	}{
		{query: "PRD", ids: []string{firstID, secondID}},
		{query: "dev", ids: []string{"three"}},
		{query: "Contoso", ids: []string{secondID}},
		{query: "TWO", ids: []string{secondID}},
		{query: "missing"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			results, err := match.Find(subscriptions, tc.query)
			if (err != nil) != (tc.ids == nil) {
				t.Fatalf("error = %v", err)
			}
			var ids []string
			for _, result := range results {
				ids = append(ids, result.Id)
			}
			if !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("IDs = %v, want %v", ids, tc.ids)
			}
		})
	}
}
