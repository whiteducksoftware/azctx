package ui_test

import (
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestPickerMarksProfileStateAndIdentity(t *testing.T) {
	subscriptions := []profile.Subscription{{
		Id: "11111111-1111-1111-1111-111111111111", Name: "Synthetic", State: "Warned",
		Tenant: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", TenantName: "Example",
		HomeTenantId: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		User:         profile.User{Name: "systemAssignedIdentity", Type: "servicePrincipal"},
	}}
	picker := ui.NewPicker(subscriptions, 160, 20)
	view := picker.View()
	for _, label := range []string{"Synthetic [Warned]", "system-assigned identity", "delegated", "Example", subscriptions[0].Id} {
		if !strings.Contains(view, label) {
			t.Fatalf("picker missing %q:\n%s", label, view)
		}
	}
	if subscriptions[0].Name != "Synthetic" {
		t.Fatal("display label replaced subscription identity")
	}
}
