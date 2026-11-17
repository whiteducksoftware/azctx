package match_test

import (
	"reflect"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/match"
	"github.com/whiteducksoftware/azctx/internal/profile"
)

func TestFindPreservesLegacyMatching(t *testing.T) {
	const firstID = "one"
	subscriptions := []profile.Subscription{
		{Id: firstID, Name: "Production"}, {Id: "two", Name: "Production"}, {Id: "three", Name: "Development"},
	}
	for _, tc := range []struct {
		query string
		ids   []string
	}{
		{query: "PRD", ids: []string{firstID, firstID}},
		{query: "dev", ids: []string{"three"}},
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
