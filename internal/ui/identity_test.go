package ui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const acmeTenant = "Acme"

func TestPickerFiltersTenantAndSubscriptionID(t *testing.T) {
	for _, query := range []string{acmeTenant, "22222222-2222"} {
		subscriptions := pickerSubscriptions()
		model := teatest.NewTestModel(t, ui.NewPicker(subscriptions, 100, 15), teatest.WithInitialTermSize(100, 15))
		model.Type("/" + query)
		model.Send(tea.KeyMsg{Type: tea.KeyEnter})
		model.Send(tea.KeyMsg{Type: tea.KeyEnter})
		final := model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(ui.Picker)
		index, err := final.Selection()
		if err != nil || index != 0 || strings.Contains(final.View(), "Production Engineering Platform") {
			t.Fatalf("query=%q selection=%d, %v; view=%s", query, index, err, final.View())
		}
	}
}

func TestPickerSortIsTypedAndDeterministic(t *testing.T) {
	subscriptions := []profile.Subscription{
		{Id: "two", Name: "Production", TenantName: acmeTenant},
		{Id: "one", Name: "Production", TenantName: "ACME"},
		{Id: "three", Name: "Development", TenantName: "Beta"},
	}
	ui.NewPicker(subscriptions, 100, 15)
	if subscriptions[0].Id != "one" || subscriptions[1].Id != "two" || subscriptions[2].Id != "three" {
		t.Fatalf("non-deterministic typed sort: %+v", subscriptions)
	}
}
