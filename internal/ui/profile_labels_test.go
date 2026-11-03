package ui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

func TestTenantPickerAlwaysShowsGUIDForDuplicateNames(t *testing.T) {
	rows := []profile.Subscription{
		{Id: productionID, Name: pickerCustomer, TenantName: "same.invalid"},
		{Id: attentionID, Name: pickerCustomer, TenantName: "same.invalid"},
	}
	picker := ui.NewTenantPicker(rows, 60, 15)
	for _, row := range rows {
		if !strings.Contains(picker.View(), row.Id) {
			t.Fatalf("duplicate tenant label hides GUID:\n%s", picker.View())
		}
	}
	if !strings.Contains(picker.View(), "Tenant ID") || strings.Contains(picker.View(), "Subscription ID") {
		t.Fatalf("tenant header=%s", picker.View())
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
			if !strings.Contains(picker.View(), "domain-only.invalid") {
				t.Fatalf("domain-only label missing: %s", picker.View())
			}
			updated, _ := picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(query)})
			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
			index, err := updated.(ui.Picker).Selection()
			if err != nil || index < 0 || subscriptions[index].Id != productionID {
				t.Fatalf("tenant filter: index=%d err=%v view=%s", index, err, updated.View())
			}
		})
	}
}
