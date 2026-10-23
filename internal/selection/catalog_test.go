package selection_test

import (
	"reflect"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/selection"
)

func TestReadOnlyCatalogVisibilityAndPriority(t *testing.T) {
	const (
		name   = "Shared"
		second = "two"
		third  = "three"
	)
	catalog := selection.Catalog{
		Subscriptions: []profile.Subscription{
			{Id: "one", Name: name, Tenant: "tenant-one"},
			{Id: second, Name: name, Tenant: "tenant-two"},
			{Id: third, Name: name, Tenant: "tenant-three"},
		},
		Config: config.Config{Hidden: []string{"TENANT-ONE"}, Pinned: []string{"TENANT-THREE"}},
		Recent: []string{second},
	}
	original := append([]profile.Subscription(nil), catalog.Subscriptions...)
	for _, tc := range []struct {
		all bool
		ids []string
	}{
		{false, []string{third, second}},
		{true, []string{third, second, "one"}},
	} {
		got, err := catalog.Find(name, tc.all)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, sub := range got {
			ids = append(ids, sub.Id)
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Fatalf("all=%t candidates=%v, want %v", tc.all, ids, tc.ids)
		}
	}
	if !reflect.DeepEqual(original, catalog.Subscriptions) {
		t.Fatal("read-only resolution mutated source profile")
	}
	catalog.Config.Pinned = nil
	got, err := catalog.Find(name, true)
	if err != nil || len(got) != 3 || got[0].Id != second {
		t.Fatalf("recent-only ranking = %v, %v", got, err)
	}
}
