package ui_test

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const pickerTrueColor = "truecolor"

func TestPickerColorsEveryRowWithoutChangingPlainLayout(t *testing.T) {
	for _, tenants := range []bool{false, true} {
		for _, mode := range []string{pickerTrueColor, "notty", "no-color"} {
			t.Run(map[bool]string{false: "subscriptions/", true: "tenants/"}[tenants]+mode, func(t *testing.T) {
				t.Setenv("NO_COLOR", "")
				if mode != "no-color" {
					if err := os.Unsetenv("NO_COLOR"); err != nil {
						t.Fatal(err)
					}
				}
				rows := []profile.Subscription{
					{Name: "First", Id: productionID, TenantName: "Acme"},
					{Name: "Second", Id: attentionID, TenantName: "Other"},
				}
				var model tea.Model = ui.NewPicker(rows, 160, 15)
				if tenants {
					model = ui.NewTenantPicker(rows, 160, 15)
				}
				plain := pickerView(model)
				profile := colorprofile.TrueColor
				if mode == "notty" {
					profile = colorprofile.NoTTY
				}
				model, _ = model.Update(tea.ColorProfileMsg{Profile: profile})
				view := model.View().Content
				if ansi.Strip(view) != plain {
					t.Fatalf("style changed layout:\n%s\nwant:\n%s", ansi.Strip(view), plain)
				}
				for _, line := range strings.Split(view, "\n") {
					if !strings.Contains(ansi.Strip(line), "First") && !strings.Contains(ansi.Strip(line), "Second") {
						continue
					}
					assertPickerRowStyles(t, line, mode == pickerTrueColor)
				}
			})
		}
	}
}

func assertPickerRowStyles(t *testing.T, line string, color bool) {
	t.Helper()
	for _, code := range []string{"32", "36", "2"} {
		colored := strings.Contains(line, "\x1b["+code+"m") || strings.Contains(line, ";"+code+"m")
		if colored != color {
			t.Fatalf("color=%t missing/unexpected style %s in row %q", color, code, line)
		}
	}
	if color && strings.Contains(ansi.Strip(line), "▸") && !strings.Contains(line, "\x1b[1m") {
		t.Fatalf("selected row is not bold: %q", line)
	}
}
