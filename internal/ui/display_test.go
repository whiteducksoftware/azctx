package ui_test

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestPickerFlattensDisplayControlsWithoutChangingValues(t *testing.T) {
	for _, tenantPicker := range []bool{false, true} {
		for _, domainFallback := range []bool{false, true} {
			sub := profile.Subscription{
				Id: productionID, Name: "Pro\tduction\n界\x1b[31m\a\u0085",
				TenantName: "Dir\rName\x1b[0m", TenantDomain: "domain\tname.invalid",
			}
			clean := profile.Subscription{
				Id: productionID, Name: "Pro duction 界 [31m  ",
				TenantName: "Dir Name [0m", TenantDomain: "domain name.invalid",
			}
			if domainFallback {
				sub.TenantName, clean.TenantName = "", ""
			}
			subs := []profile.Subscription{sub}
			newPicker := ui.NewPicker
			if tenantPicker {
				newPicker = ui.NewTenantPicker
			}
			picker := newPicker(subs, 160, 12)
			want := newPicker([]profile.Subscription{clean}, 160, 12).View()
			if got := picker.View(); got != want {
				t.Errorf("tenant=%t domainFallback=%t: control-bearing view=%q, want %q", tenantPicker, domainFallback, got, want)
			}
			model, _ := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
			index, err := requirePicker(t, model).Selection()
			if err != nil || index != 0 || !reflect.DeepEqual(subs, []profile.Subscription{sub}) {
				t.Fatalf("display changed selection data: index=%d err=%v subscriptions=%+v", index, err, subs)
			}
		}
	}
}
