package ui_test

import (
	"os"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

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
				rowsSeen := 0
				for _, line := range strings.Split(view, "\n") {
					if !strings.Contains(ansi.Strip(line), "First") && !strings.Contains(ansi.Strip(line), "Second") {
						continue
					}
					rowsSeen++
					assertPickerRowStyles(t, line, mode == pickerTrueColor)
				}
				if rowsSeen != len(rows) {
					t.Fatalf("checked %d rows, want %d in %q", rowsSeen, len(rows), view)
				}
			})
		}
	}
}

func assertPickerRowStyles(t *testing.T, line string, color bool) {
	t.Helper()
	if !color && ansi.Strip(line) != line {
		t.Fatalf("plain row contains ANSI: %q", line)
	}
	selected := strings.Contains(ansi.Strip(line), "▸")
	var style pickerRowStyle
	column := 0
	for remaining := line; remaining != ""; {
		if parameters, ok := strings.CutPrefix(remaining, "\x1b["); ok {
			codes, rest, found := strings.Cut(parameters, "m")
			if !found {
				t.Fatalf("invalid SGR in row %q", line)
			}
			style.apply(codes)
			remaining = rest
			continue
		}
		glyph, size := utf8.DecodeRuneInString(remaining)
		remaining = remaining[size:]
		if unicode.IsSpace(glyph) {
			continue
		}
		if style.bold != (selected && color) {
			t.Fatalf("glyph %q has bold=%t, selected=%t color=%t in %q", glyph, style.bold, selected, color, line)
		}
		if glyph == '|' {
			column++
		} else if glyph != '▸' && color {
			style.assertColumn(t, column, glyph, line)
		}
	}
}

type pickerRowStyle struct {
	bold, faint bool
	foreground  string
}

func (style *pickerRowStyle) apply(codes string) {
	for _, code := range strings.Split(codes, ";") {
		switch code {
		case "", "0":
			*style = pickerRowStyle{}
		case "1":
			style.bold = true
		case "2":
			style.faint = true
		case "22":
			style.bold, style.faint = false, false
		case "32", "36", "39":
			style.foreground = code
		}
	}
}

func (style pickerRowStyle) assertColumn(t *testing.T, column int, glyph rune, line string) {
	t.Helper()
	if column == 0 && style.foreground != "32" || column == 1 && style.foreground != "36" || column == 2 && !style.faint {
		t.Fatalf("column %d glyph %q has style=%+v in %q", column, glyph, style, line)
	}
}
