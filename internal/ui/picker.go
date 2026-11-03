package ui

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

var (
	ErrInterrupted = errors.New("interrupted")
	ErrCanceled    = errors.New("selection canceled")
)

type subscriptionItem struct {
	subscription profile.Subscription
	index        int
}

func (item subscriptionItem) FilterValue() string { return item.subscription.Name }

// Picker is the subscription list's Bubble Tea model.
type Picker struct {
	list     list.Model
	delegate subscriptionDelegate
	selected int
	err      error
}

// NewPicker sorts subscriptions in place; Selection indexes this sorted slice.
func NewPicker(subscriptions []profile.Subscription, width, height int) Picker {
	slices.SortStableFunc(subscriptions, func(a, b profile.Subscription) int {
		if strings.EqualFold(a.TenantName, b.TenantName) {
			return cmp.Compare(a.Name, b.Name)
		}
		return cmp.Compare(a.TenantName, b.TenantName)
	})
	delegate := subscriptionDelegate{nameWidth: len("Name"), tenantWidth: len("Tenant")}
	items := make([]list.Item, len(subscriptions))
	for index, subscription := range subscriptions {
		items[index] = subscriptionItem{subscription: subscription, index: index}
		delegate.nameWidth = max(delegate.nameWidth, lipgloss.Width(subscription.Name))
		delegate.tenantWidth = max(delegate.tenantWidth, lipgloss.Width(tenantLabel(subscription)))
	}
	picker := Picker{delegate: delegate, selected: -1, err: ErrCanceled}
	picker.list = list.New(items, delegate, max(1, width), max(1, height))
	picker.list.DisableQuitKeybindings()
	picker.list.SetShowStatusBar(false)
	picker.list.Styles.Title = lipgloss.NewStyle().Bold(true)
	picker.list.Styles.TitleBar = lipgloss.NewStyle().PaddingBottom(1)
	picker.resize(width, height)
	return picker
}

func (picker Picker) Init() tea.Cmd { return nil }

func (picker Picker) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		picker.resize(message.Width, message.Height)
	case list.FilterMatchesMsg:
		// Bubbles' asynchronous results carry no query identity and can arrive stale.
		return picker, nil
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c":
			picker.err = ErrInterrupted
			return picker, tea.Quit
		case "esc", "q":
			if picker.list.FilterState() == list.Unfiltered {
				return picker, tea.Quit
			}
		case "enter":
			if picker.list.FilterState() != list.Filtering {
				if item, ok := picker.list.SelectedItem().(subscriptionItem); ok {
					picker.selected, picker.err = item.index, nil
					return picker, tea.Quit
				}
			}
		}
	}
	var command tea.Cmd
	filter := picker.list.FilterValue()
	picker.list, command = picker.list.Update(message)
	if picker.list.FilterValue() != filter && picker.list.FilterState() != list.Unfiltered {
		state := picker.list.FilterState()
		picker.list.SetFilterText(picker.list.FilterValue())
		picker.list.SetFilterState(state)
	}
	return picker, command
}

func (picker *Picker) resize(width, height int) {
	picker.list.SetSize(max(1, width), max(1, height))
	picker.list.Title = "  " + picker.delegate.row(width, "Name", "Subscription ID", "Tenant")
}

func (picker Picker) View() string { return picker.list.View() }

// Selection reports the chosen sorted index, or cancellation/interruption.
func (picker Picker) Selection() (int, error) { return picker.selected, picker.err }

func Select(subscriptions []profile.Subscription) (int, error) {
	width, height, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil {
		width, height = 100, 20
	}
	var input io.Reader = os.Stdin
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Hide the fd so Windows readers consume redirected stdin, not CONIN$.
		input = struct{ io.Reader }{os.Stdin}
	}
	result, err := tea.NewProgram(NewPicker(subscriptions, width, height), tea.WithInput(input), tea.WithOutput(os.Stderr)).Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return -1, ErrInterrupted
	}
	if err != nil {
		return -1, err
	}
	return result.(Picker).Selection()
}

type subscriptionDelegate struct {
	nameWidth   int
	tenantWidth int
}

func (subscriptionDelegate) Height() int                         { return 1 }
func (subscriptionDelegate) Spacing() int                        { return 0 }
func (subscriptionDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (delegate subscriptionDelegate) Render(writer io.Writer, model list.Model, index int, item list.Item) {
	subscription := item.(subscriptionItem).subscription
	line := delegate.row(model.Width(), subscription.Name, subscription.Id, tenantLabel(subscription))
	prefix := "  "
	style := lipgloss.NewStyle()
	if index == model.Index() {
		prefix = "▸ "
		style = style.Bold(true).Foreground(lipgloss.Color("2"))
	}
	_, _ = fmt.Fprint(writer, style.Render(prefix+line))
}

func (delegate subscriptionDelegate) row(width int, name, id, tenant string) string {
	nameWidth := min(delegate.nameWidth, max(1, width-2))
	line := lipgloss.NewStyle().Width(nameWidth).MaxWidth(nameWidth).Render(name)
	tenantWidth := delegate.tenantWidth
	switch {
	case width >= 2+nameWidth+3+36+3+tenantWidth:
		line += " | " + lipgloss.NewStyle().Width(36).MaxWidth(36).Render(id) + " | " + tenant
	case width >= 2+nameWidth+3+tenantWidth:
		line += " | " + tenant
	}
	return lipgloss.NewStyle().MaxWidth(max(1, width-2)).Render(line)
}

func tenantLabel(subscription profile.Subscription) string {
	if subscription.TenantName != "" {
		return subscription.TenantName
	}
	return subscription.Tenant
}
