package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	view := pickerView(picker)
	for _, label := range []string{"Synthetic [Warned]", "system-assigned identity", "delegated", "Example", subscriptions[0].Id} {
		if !strings.Contains(view, label) {
			t.Fatalf("picker missing %q:\n%s", label, view)
		}
	}
	if subscriptions[0].Name != "Synthetic" {
		t.Fatal("display label replaced subscription identity")
	}
}

func TestTenantPickerAlwaysShowsGUIDForDuplicateNames(t *testing.T) {
	rows := []profile.Subscription{
		{Id: productionID, Name: pickerCustomer, TenantName: "same.invalid"},
		{Id: attentionID, Name: pickerCustomer, TenantName: "same.invalid"},
	}
	picker := ui.NewTenantPicker(rows, 60, 15)
	for _, row := range rows {
		if !strings.Contains(pickerView(picker), row.Id) {
			t.Fatalf("duplicate tenant label hides GUID:\n%s", pickerView(picker))
		}
	}
	if !strings.Contains(pickerView(picker), "Tenant ID") || strings.Contains(pickerView(picker), "Subscription ID") {
		t.Fatalf("tenant header=%s", pickerView(picker))
	}
}

func TestPickerTenantDomainFallbackAndFiltering(t *testing.T) {
	const tenantName = "Example"
	const tenantID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	for _, query := range []string{"example.invalid", tenantID} {
		t.Run(query, func(t *testing.T) {
			subscriptions := []profile.Subscription{
				{Id: productionID, Name: "Selected subscription", Tenant: tenantID, TenantName: tenantName, TenantDomain: "example.invalid"},
				{Id: "22222222-2222-2222-2222-222222222222", Name: "Development", TenantDomain: "domain-only.invalid"},
			}
			picker := ui.NewPicker(subscriptions, 160, 15)
			if !strings.Contains(pickerView(picker), "domain-only.invalid") {
				t.Fatalf("domain-only label missing: %s", pickerView(picker))
			}
			updated, _ := picker.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			updated, _ = updated.Update(tea.KeyPressMsg{Code: []rune(query)[0], Text: query})
			updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			index, err := requirePicker(t, updated).Selection()
			if err != nil || index < 0 || subscriptions[index].Id != productionID {
				t.Fatalf("tenant filter: index=%d err=%v view=%s", index, err, pickerView(updated))
			}
		})
	}
}
