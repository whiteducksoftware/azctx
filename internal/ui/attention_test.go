package ui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const (
	pickerCustomer = "Customer"
	attentionID    = "33333333-3333-3333-3333-333333333333"
	pickerTenantID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	pickerHomeID   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func TestPickerAttentionColumns(t *testing.T) {
	for _, width := range []int{60, 100, 160} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			subs := []profile.Subscription{
				{Id: productionID, Name: "Lighthouse", State: "Warned", Tenant: pickerTenantID, HomeTenantId: pickerHomeID, TenantName: pickerCustomer},
				{Id: attentionID, Name: "Billing", State: "PastDue", TenantName: pickerCustomer},
			}
			model := teatest.NewTestModel(t, ui.NewPicker(subs, width, 15), teatest.WithInitialTermSize(width, 15))
			model.Send(tea.KeyMsg{Type: tea.KeyDown})
			model.Send(tea.KeyMsg{Type: tea.KeyEnter})
			final := model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(ui.Picker)
			if index, err := final.Selection(); err != nil || index != 1 || subs[index].Id != productionID {
				t.Fatalf("labeled identity index=%d err=%v", index, err)
			}
			view := final.View()
			for _, marker := range []string{"[delegated]", "[Warned]", "[PastDue]"} {
				if !strings.Contains(view, marker) {
					t.Fatalf("missing %s at %d: %s", marker, width, view)
				}
			}
			lines := strings.Split(view, "\n")
			for i, line := range lines {
				if lipgloss.Width(line) > width {
					t.Fatalf("line exceeds %d cells: %q", width, line)
				}
				lines[i] = strings.TrimRight(line, " ")
			}
			teatest.RequireEqualOutput(t, []byte(strings.Join(lines, "\n")+"\n"))
		})
	}
}
