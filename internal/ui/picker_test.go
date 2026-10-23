package ui_test

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

// Lip Gloss v2 leaves color downsampling to the terminal renderer. Screen
// assertions compare the same plain text that a non-color terminal displays.
func pickerView(model tea.Model) string { return ansi.Strip(model.View().Content) }

func requirePicker(t *testing.T, value tea.Model) ui.Picker {
	t.Helper()
	picker, ok := value.(ui.Picker)
	if !ok {
		t.Fatal("final model is not a picker")
	}
	return picker
}

const productionID = "11111111-1111-1111-1111-111111111111"

type replayFilterMsg list.FilterMatchesMsg

type filterReplayModel struct {
	ui.Picker
	delayed chan list.FilterMatchesMsg
	applied chan struct{}
}

func (model filterReplayModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	notify := false
	if matches, ok := message.(list.FilterMatchesMsg); ok {
		if len(matches) > 1 {
			model.delayed <- matches
			return model, nil
		}
		notify = true
	}
	if replay, ok := message.(replayFilterMsg); ok {
		message = list.FilterMatchesMsg(replay)
	}
	updated, command := model.Picker.Update(message)
	picker, ok := updated.(ui.Picker)
	if !ok {
		panic("picker fixture returned an unexpected model")
	}
	model.Picker = picker
	if notify {
		model.applied <- struct{}{}
	}
	return model, command
}

func TestPickerIgnoresOutOfOrderFilterResults(t *testing.T) {
	delayed := make(chan list.FilterMatchesMsg, 1)
	applied := make(chan struct{}, 1)
	model := teatest.NewTestModel(t, filterReplayModel{
		Picker: ui.NewPicker(pickerSubscriptions(), 100, 15), delayed: delayed, applied: applied,
	}, teatest.WithInitialTermSize(100, 15))
	model.Type("/P")
	var stale list.FilterMatchesMsg
	select {
	case stale = <-delayed:
	case <-time.After(3 * time.Second):
		t.Fatal("the first filter did not finish")
	}
	model.Type("RD")
	select {
	case <-applied:
	case <-time.After(3 * time.Second):
		t.Fatal("the specific filter did not finish")
	}
	// Complete the old "P" request after the specific "PRD" request.
	model.Send(replayFilterMsg(stale))
	model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	replayed, ok := model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(filterReplayModel)
	if !ok {
		t.Fatal("final model is not the replay fixture")
	}
	final := replayed.Picker
	index, err := final.Selection()
	if err != nil || index != 1 || strings.Contains(pickerView(final), "Development Engineering Platform") {
		t.Fatalf("stale filter replaced the current query: selection=%d, %v; view=%s", index, err, pickerView(final))
	}
}

func TestPickerEditsFilterAtCursor(t *testing.T) {
	subscriptions := []profile.Subscription{
		{Id: productionID, Name: "Xylophone Yarrow Production", TenantName: acmeTenant},
		{Id: attentionID, Name: "Development Sandbox", TenantName: acmeTenant},
	}
	var model tea.Model = ui.NewPicker(subscriptions, 100, 15)
	for _, key := range []tea.KeyPressMsg{
		{Code: '/', Text: "/"},
		{Code: 'p', Text: "prd"},
		{Code: tea.KeyHome},
		{Code: 'x', Text: "x"},
		{Code: 'y', Text: "y"},
	} {
		model, _ = model.Update(key)
	}
	if view := pickerView(model); !strings.Contains(view, "xyprd") || !strings.Contains(view, subscriptions[1].Name) {
		t.Fatalf("filter editing lost the insertion point:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if index, err := requirePicker(t, model).Selection(); err != nil || index != 1 {
		t.Fatalf("edited filter selection = %d, %v", index, err)
	}
}

func TestPickerAdaptsToTerminalBackground(t *testing.T) {
	var model tea.Model = ui.NewPicker(pickerSubscriptions(), 100, 15)
	model, _ = model.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	model, _ = model.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	plain := pickerView(model)
	for _, palette := range []struct {
		background color.Color
		prompt     string
	}{
		{color.White, "\x1b[38;2;4;181;117m"},
		{color.Black, "\x1b[38;2;236;253;101m"},
		{color.White, "\x1b[38;2;4;181;117m"},
	} {
		model, _ = model.Update(tea.BackgroundColorMsg{Color: palette.background})
		if view := model.View().Content; !strings.Contains(view, palette.prompt+"Filter: ") {
			t.Fatalf("filter prompt did not adapt to background: %q", view)
		}
		if view := pickerView(model); view != plain {
			t.Fatalf("background response changed layout or filter:\n%s\nwant:\n%s", view, plain)
		}
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if index, err := requirePicker(t, model).Selection(); err != nil || index != 1 {
		t.Fatalf("background response changed selection: %d, %v", index, err)
	}
}

func TestPickerEscapeClearsFilterWithoutQuitting(t *testing.T) {
	for _, applied := range []bool{false, true} {
		t.Run(fmt.Sprintf("applied=%t", applied), func(t *testing.T) {
			subscriptions := pickerSubscriptions()
			model := teatest.NewTestModel(t, ui.NewPicker(subscriptions, 100, 15), teatest.WithInitialTermSize(100, 15))
			model.Type("/PRD")
			if applied {
				model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
			}
			model.Send(tea.KeyPressMsg{Code: tea.KeyEscape})
			model.Send(tea.KeyReleaseMsg{Code: 'c', Mod: tea.ModCtrl})
			model.Send(tea.KeyPressMsg{Code: tea.KeyDown})
			model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
			final := requirePicker(t, model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
			if index, err := final.Selection(); err != nil || index != 1 || subscriptions[index].Id != productionID {
				t.Fatalf("escape or key release quit/changed selection: %d, %v", index, err)
			}
			if !strings.Contains(pickerView(final), "Development Engineering Platform") {
				t.Fatalf("escape did not restore the full list:\n%s", pickerView(final))
			}
		})
	}
}

func TestPickerPasteAndUnicodeFilterCursor(t *testing.T) {
	subscriptions := []profile.Subscription{
		{Id: productionID, Name: "界 Production", TenantName: acmeTenant},
		{Id: attentionID, Name: "Development Sandbox", TenantName: acmeTenant},
	}
	var model tea.Model = ui.NewPicker(subscriptions, 100, 15)
	for _, message := range []tea.Msg{
		tea.KeyPressMsg{Code: '/', Text: "/"},
		tea.PasteMsg{Content: "界pd"},
		tea.KeyPressMsg{Code: tea.KeyLeft},
		tea.KeyPressMsg{Code: 'r', Text: "r"},
		tea.KeyPressMsg{Code: tea.KeyHome},
		tea.KeyPressMsg{Code: tea.KeyRight},
		tea.KeyPressMsg{Code: tea.KeyDelete},
		tea.KeyPressMsg{Code: 'p', Text: "p"},
		tea.KeyPressMsg{Code: tea.KeyEnd},
	} {
		model, _ = model.Update(message)
	}
	if view := pickerView(model); !strings.Contains(view, "界prd") || strings.Contains(view, "Development Sandbox") {
		t.Fatalf("paste or Unicode cursor editing changed the query:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if index, err := requirePicker(t, model).Selection(); err != nil || index != 1 {
		t.Fatalf("Unicode query selected %d, %v", index, err)
	}
}

func TestPickerColorQueryIsSilentAndTerminalOnly(t *testing.T) {
	for _, profile := range []colorprofile.Profile{colorprofile.NoTTY, colorprofile.TrueColor} {
		t.Run(profile.String(), func(t *testing.T) {
			model := teatest.NewTestModel(t, ui.NewPicker(pickerSubscriptions(), 100, 15),
				teatest.WithInitialTermSize(100, 15),
				teatest.WithProgramOptions(tea.WithColorProfile(profile)))
			const query = "\x1b]11;?\x1b\\"
			var screen []byte
			teatest.WaitFor(t, model.Output(), func(output []byte) bool {
				screen = append(screen[:0], output...)
				return bytes.Contains(output, []byte("Name")) &&
					(profile == colorprofile.NoTTY || bytes.Contains(output, []byte(query)))
			}, teatest.WithDuration(3*time.Second))
			model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
			rest, err := io.ReadAll(model.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second)))
			if err != nil {
				t.Fatal(err)
			}
			screen = append(screen, rest...)
			if bytes.ContainsRune(screen, '\a') || bytes.Contains(screen, []byte(query)) != (profile == colorprofile.TrueColor) {
				t.Fatalf("unexpected terminal query or bell: %q", screen)
			}
		})
	}
}

func TestPickerCancelKeys(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter string
		key    tea.KeyPressMsg
		want   error
	}{
		{"escape", "", tea.KeyPressMsg{Code: tea.KeyEscape}, ui.ErrCanceled},
		{"quit", "", tea.KeyPressMsg{Code: 'q', Text: "q"}, ui.ErrCanceled},
		{"interrupt while filtering", "/PRD", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, ui.ErrInterrupted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := teatest.NewTestModel(t, ui.NewPicker(pickerSubscriptions(), 100, 15), teatest.WithInitialTermSize(100, 15))
			model.Type(tc.filter)
			model.Send(tc.key)
			final := requirePicker(t, model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
			if index, err := final.Selection(); index != -1 || !errors.Is(err, tc.want) {
				t.Fatalf("cancellation selected %d, %v; want %v", index, err, tc.want)
			}
		})
	}
}

func TestPickerNarrowLongName(t *testing.T) {
	subscriptions := []profile.Subscription{{Id: productionID, Name: strings.Repeat("界", 40), TenantName: acmeTenant}}
	model := teatest.NewTestModel(t, ui.NewPicker(subscriptions, 60, 10), teatest.WithInitialTermSize(60, 10))
	teatest.WaitFor(t, model.Output(), func(output []byte) bool {
		return bytes.Contains(output, []byte("界"))
	}, teatest.WithDuration(3*time.Second))
	model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	final := requirePicker(t, model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
	for _, line := range strings.Split(pickerView(final), "\n") {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("wide name overflows a narrow terminal: %q (%d cells)", line, lipgloss.Width(line))
		}
	}
}

func TestPickerLongNamesStayOnOneRow(t *testing.T) {
	for _, width := range []int{30, 60, 100, 160} {
		for _, tenants := range []bool{false, true} {
			for _, name := range []string{"Production Engineering Platform ", "界 🌍 e\u0301 "} {
				t.Run(fmt.Sprintf("%d/tenants=%t/%s", width, tenants, name), func(t *testing.T) {
					const height = 12
					subscriptions := make([]profile.Subscription, 12)
					for index := range subscriptions {
						subscriptions[index] = profile.Subscription{
							Id: productionID, Name: fmt.Sprintf("Row%02d %s", index, strings.Repeat(name, 10)),
							TenantName: acmeTenant,
						}
					}
					var model tea.Model = ui.NewPicker(subscriptions, width, height)
					if tenants {
						model = ui.NewTenantPicker(subscriptions, width, height)
					}
					for index := range subscriptions {
						requirePickerRowsFit(t, pickerView(model), width, height, index)
						if index < len(subscriptions)-1 {
							model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
						}
					}
					model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
					if index, err := requirePicker(t, model).Selection(); err != nil || index != 11 {
						t.Fatalf("paginated selection = %d, %v", index, err)
					}
				})
			}
		}
	}
}

func requirePickerRowsFit(t *testing.T, view string, width, height, index int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("row %d overflows height %d with %d lines", index, height, len(lines))
	}
	if strings.Count(view, "▸ ") != 1 {
		t.Fatalf("selected row %d is not visible:\n%s", index, view)
	}
	for _, line := range lines {
		if (width >= 60 || strings.HasPrefix(line, "▸ ")) && lipgloss.Width(strings.TrimRight(line, " ")) > width {
			t.Fatalf("row %d exceeds width %d: %q", index, width, line)
		}
	}
}

func pickerSubscriptions() []profile.Subscription {
	return []profile.Subscription{
		{Id: productionID, Name: "Production Engineering Platform", TenantName: "White Duck Production Directory"},
		{Id: "22222222-2222-2222-2222-222222222222", Name: "Development Engineering Platform", TenantName: "Acme Engineering Directory"},
	}
}

func TestPickerFuzzyFilterAndInterrupt(t *testing.T) {
	model := teatest.NewTestModel(t, ui.NewPicker(pickerSubscriptions(), 100, 15), teatest.WithInitialTermSize(100, 15))
	teatest.WaitFor(t, model.Output(), func(output []byte) bool {
		return bytes.Contains(output, []byte("Development Engineering Platform"))
	}, teatest.WithDuration(3*time.Second))
	model.Type("/PRD")
	teatest.WaitFor(t, model.Output(), func(output []byte) bool {
		return bytes.Contains(output, []byte("PRD")) && bytes.Contains(output, []byte("Production Engineering Platform"))
	}, teatest.WithDuration(3*time.Second))
	model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	final := requirePicker(t, model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
	index, err := final.Selection()
	if err != nil || index != 1 || strings.Contains(pickerView(final), "Development Engineering Platform") {
		t.Fatalf("fuzzy selection = %d, %v; view=%s", index, err, pickerView(final))
	}

	interrupted := teatest.NewTestModel(t, ui.NewPicker(nil, 60, 10), teatest.WithInitialTermSize(60, 10))
	interrupted.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	final = requirePicker(t, interrupted.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
	if index, err := final.Selection(); index != -1 || !errors.Is(err, ui.ErrInterrupted) {
		t.Fatalf("interrupted selection = %d, %v", index, err)
	}
}

func TestPickerColumns(t *testing.T) {
	for _, width := range []int{60, 100, 160} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			subscriptions := pickerSubscriptions()
			model := teatest.NewTestModel(t, ui.NewPicker(subscriptions, width, 15), teatest.WithInitialTermSize(width, 15))
			teatest.WaitFor(t, model.Output(), func(output []byte) bool {
				if bytes.ContainsRune(output, '\a') {
					t.Error("picker rang the terminal bell")
				}
				return bytes.Contains(output, []byte("Development Engineering Platform"))
			}, teatest.WithDuration(3*time.Second))
			model.Send(tea.KeyPressMsg{Code: tea.KeyDown})
			model.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
			final := requirePicker(t, model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
			index, err := final.Selection()
			if err != nil || index != 1 || subscriptions[index].Id != productionID {
				t.Fatalf("selection = %d, %v; subscriptions=%+v", index, err, subscriptions)
			}
			view := pickerView(final)
			if strings.Contains(view, productionID) != (width == 160) {
				t.Fatalf("subscription ID column at %d: %s", width, view)
			}
			if strings.Contains(view, "White Duck Production Directory") != (width >= 100) {
				t.Fatalf("tenant column at %d: %s", width, view)
			}
			lines := strings.Split(view, "\n")
			for index, line := range lines {
				if lipgloss.Width(line) > width {
					t.Fatalf("line exceeds terminal width %d: %q", width, line)
				}
				lines[index] = strings.TrimRight(line, " ")
			}
			teatest.RequireEqualOutput(t, []byte(strings.Join(lines, "\n")+"\n"))
		})
	}
}
