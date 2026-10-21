package ui_test

import (
	"reflect"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestPickerSortAndSearch(t *testing.T) {
	subscriptions := []profile.Subscription{
		{Id: "three", Name: "Production", TenantName: "Zebra"},
		{Id: "two", Name: "Production", TenantName: "Acme"},
		{Id: "one", Name: "Development", TenantName: "Acme"},
	}
	picker := ui.BuildPrompt(subscriptions)
	got := []string{subscriptions[0].Id, subscriptions[1].Id, subscriptions[2].Id}
	if !reflect.DeepEqual(got, []string{"one", "two", "three"}) {
		t.Fatalf("picker order = %v", got)
	}
	if !picker.Searcher("DEV", 0) || picker.Searcher("dev", 1) {
		t.Fatal("search must use sorted subscription names, ignoring case")
	}
	if picker.Size < 1 || picker.Size > 3 {
		t.Fatalf("picker size = %d", picker.Size)
	}
}
