package ui_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

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
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	replayed, ok := model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(filterReplayModel)
	if !ok {
		t.Fatal("final model is not the replay fixture")
	}
	final := replayed.Picker
	index, err := final.Selection()
	if err != nil || index != 1 || strings.Contains(final.View(), "Development Engineering Platform") {
		t.Fatalf("stale filter replaced the current query: selection=%d, %v; view=%s", index, err, final.View())
	}
}

func TestPickerNarrowLongName(t *testing.T) {
	subscriptions := []profile.Subscription{{Id: productionID, Name: strings.Repeat("界", 40), TenantName: acmeTenant}}
	model := teatest.NewTestModel(t, ui.NewPicker(subscriptions, 60, 10), teatest.WithInitialTermSize(60, 10))
	teatest.WaitFor(t, model.Output(), func(output []byte) bool {
		return bytes.Contains(output, []byte("界"))
	}, teatest.WithDuration(3*time.Second))
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	final := requirePicker(t, model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
	for _, line := range strings.Split(final.View(), "\n") {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("wide name overflows a narrow terminal: %q (%d cells)", line, lipgloss.Width(line))
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
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	final := requirePicker(t, model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
	index, err := final.Selection()
	if err != nil || index != 1 || strings.Contains(final.View(), "Development Engineering Platform") {
		t.Fatalf("fuzzy selection = %d, %v; view=%s", index, err, final.View())
	}

	interrupted := teatest.NewTestModel(t, ui.NewPicker(nil, 60, 10), teatest.WithInitialTermSize(60, 10))
	interrupted.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
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
			model.Send(tea.KeyMsg{Type: tea.KeyDown})
			model.Send(tea.KeyMsg{Type: tea.KeyEnter})
			final := requirePicker(t, model.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)))
			index, err := final.Selection()
			if err != nil || index != 1 || subscriptions[index].Id != productionID {
				t.Fatalf("selection = %d, %v; subscriptions=%+v", index, err, subscriptions)
			}
			view := final.View()
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
