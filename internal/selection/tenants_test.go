package selection_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/selection"
)

func TestReadOnlyTenantCatalogPreservesGUIDsAndPriorities(t *testing.T) {
	const first, second = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	const one, two = "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"
	const sharedLabel = "Shared"
	subs := []profile.Subscription{
		{Id: one, Tenant: first, TenantName: sharedLabel, TenantDomain: "example.invalid"},
		{Id: two, Tenant: second, TenantName: sharedLabel, TenantDomain: "example-long.invalid"},
		{Id: two, Tenant: strings.ToUpper(second), TenantName: sharedLabel, TenantDomain: "example-long.invalid"},
	}
	before := append([]profile.Subscription(nil), subs...)
	catalog := selection.Catalog{Subscriptions: subs, Config: config.Config{Pinned: []string{second}}}
	got, err := catalog.Tenants(sharedLabel, false)
	if err != nil || len(got) != 2 || got[0].ID != second || len(got[0].Subscriptions) != 1 || got[1].ID != first {
		t.Fatalf("tenant grouping/ranking=%+v err=%v", got, err)
	}
	exact, err := catalog.Tenants("example.invalid", false)
	if err != nil || len(exact) != 1 || exact[0].ID != first {
		t.Fatalf("exact domain should precede fuzzy domain: %+v err=%v", exact, err)
	}
	catalog.Config.Hidden = []string{first}
	if hidden, err := catalog.Tenants(first, false); err == nil || len(hidden) != 0 {
		t.Fatalf("hidden tenant leaked: %+v err=%v", hidden, err)
	}
	if exposed, err := catalog.Tenants(first, true); err != nil || len(exposed) != 1 {
		t.Fatalf("--all hidden tenant=%+v err=%v", exposed, err)
	}
	got[0].Subscriptions[0].Name = "changed"
	if !reflect.DeepEqual(before, subs) {
		t.Fatal("read-only tenant resolution mutated its source")
	}
}

func TestTenantNamesContainingSlashMatchLiterally(t *testing.T) {
	const slashTenant = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const otherTenant = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	slash := profile.Subscription{Id: "11111111-1111-1111-1111-111111111111", Tenant: slashTenant, TenantName: "Ops/Ops"}
	other := profile.Subscription{Id: "22222222-2222-2222-2222-222222222222", Tenant: otherTenant, TenantName: "Ops"}
	for _, test := range []struct {
		name string
		subs []profile.Subscription
	}{
		{name: "coexisting", subs: []profile.Subscription{slash, other}},
		{name: "alone", subs: []profile.Subscription{slash}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := (selection.Catalog{Subscriptions: test.subs}).Tenants("Ops/Ops", false)
			if err != nil || len(got) != 1 || got[0].ID != slashTenant {
				t.Fatalf("literal tenant name selected %+v, err=%v; want tenant %s", got, err, slashTenant)
			}
		})
	}
}
