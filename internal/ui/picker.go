package ui

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/sahilm/fuzzy"
	"golang.org/x/term"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/log"
)

var (
	ErrInterrupted = errors.New("interrupted")
	ErrCanceled    = errors.New("selection canceled")
)

type subscriptionItem struct {
	subscription profile.Subscription
	index        int
}

func (item subscriptionItem) FilterValue() string {
	sub := item.subscription
	return strings.Join([]string{sub.Name, sub.TenantName, sub.TenantDomain, sub.Tenant, sub.Id}, "\x00")
}

// Picker is the subscription list's Bubble Tea model.
type Picker struct {
	list     list.Model
	delegate subscriptionDelegate
	selected int
	err      error
}

// NewPicker sorts subscriptions in place; Selection indexes this sorted slice.
func NewPicker(subscriptions []profile.Subscription, width, height int) Picker {
	SortSubscriptions(subscriptions)
	return newOrderedPicker(subscriptions, width, height)
}

// NewTenantPicker preserves ranked rows and prioritizes GUIDs over domain text.
func NewTenantPicker(tenants []profile.Subscription, width, height int) Picker {
	picker := newOrderedPicker(tenants, width, height)
	picker.delegate.tenants = true
	picker.list.SetDelegate(picker.delegate)
	picker.resize(width, height)
	return picker
}

// SortSubscriptions establishes the deterministic base order before priorities.
func SortSubscriptions(subscriptions []profile.Subscription) {
	slices.SortFunc(subscriptions, func(a, b profile.Subscription) int {
		return cmp.Or(
			strings.Compare(strings.ToLower(tenantLabel(a)), strings.ToLower(tenantLabel(b))),
			strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			strings.Compare(strings.ToLower(a.Id), strings.ToLower(b.Id)),
		)
	})
}

func newOrderedPicker(subscriptions []profile.Subscription, width, height int) Picker {
	delegate := subscriptionDelegate{nameWidth: len("Name"), tenantWidth: len("Tenant")}
	items := make([]list.Item, len(subscriptions))
	for index, subscription := range subscriptions {
		items[index] = subscriptionItem{subscription: subscription, index: index}
		delegate.nameWidth = max(delegate.nameWidth, lipgloss.Width(DisplayText(subscription.DisplayName())))
		delegate.tenantWidth = max(delegate.tenantWidth, lipgloss.Width(DisplayText(tenantLabel(subscription))))
	}
	picker := Picker{delegate: delegate, selected: -1, err: ErrCanceled}
	picker.list = list.New(items, delegate, max(1, width), max(1, height))
	picker.list.Filter = filterSubscriptions
	picker.list.DisableQuitKeybindings()
	picker.list.SetShowStatusBar(false)
	picker.setStyles(true)
	picker.resize(width, height)
	return picker
}

func (picker *Picker) setStyles(isDark bool) {
	picker.list.Styles = list.DefaultStyles(isDark)
	picker.list.Styles.Title = lipgloss.NewStyle().Bold(true)
	picker.list.Styles.TitleBar = lipgloss.NewStyle().PaddingBottom(1)
	picker.list.FilterInput.SetStyles(picker.list.Styles.Filter)
	picker.list.Help.Styles = help.DefaultStyles(isDark)
	picker.list.Paginator.ActiveDot = picker.list.Styles.ActivePaginationDot.String()
	picker.list.Paginator.InactiveDot = picker.list.Styles.InactivePaginationDot.String()
}

// Rank each field independently so a query cannot accidentally span a name,
// tenant and ID, then keep each subscription's highest-ranked match.
func filterSubscriptions(query string, targets []string) []list.Rank {
	var fields []string
	var owners []int
	for index, target := range targets {
		for _, field := range strings.Split(target, "\x00") {
			fields = append(fields, field)
			owners = append(owners, index)
		}
	}
	var ranks []list.Rank
	seen := make(map[int]bool)
	for _, match := range fuzzy.Find(query, fields) {
		index := owners[match.Index]
		if !seen[index] {
			ranks = append(ranks, list.Rank{Index: index})
			seen[index] = true
		}
	}
	return ranks
}

func (picker Picker) Init() tea.Cmd { return nil }

func (picker Picker) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		picker.resize(message.Width, message.Height)
	case tea.ColorProfileMsg:
		picker.setColorProfile(message.Profile)
		picker.list.SetDelegate(picker.delegate)
		if picker.delegate.color {
			// ST avoids a BEL byte, including on terminals without OSC support.
			return picker, tea.Raw("\x1b]11;?\x1b\\")
		}
	case tea.BackgroundColorMsg:
		picker.setStyles(message.IsDark())
	case list.FilterMatchesMsg:
		// Bubbles' asynchronous results carry no query identity and can arrive stale.
		return picker, nil
	case tea.KeyPressMsg:
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
		cursor := picker.list.FilterInput.Position()
		picker.list.SetFilterText(picker.list.FilterValue())
		picker.list.SetFilterState(state)
		picker.list.FilterInput.SetCursor(cursor)
	}
	return picker, command
}

func (picker *Picker) setColorProfile(profile colorprofile.Profile) {
	_, noColor := os.LookupEnv("NO_COLOR")
	picker.delegate.color = profile >= colorprofile.ANSI && !noColor
}

func (picker *Picker) resize(width, height int) {
	picker.list.SetSize(max(1, width), max(1, height))
	header := picker.delegate
	header.color = false
	picker.list.Title = "  " + header.row(width, "Name", "Subscription ID", "Tenant", false)
	if picker.delegate.tenants {
		picker.list.Title = "  " + header.row(width, "Tenant", "Tenant ID", "Domain", false)
	}
}

func (picker Picker) View() tea.View { return tea.NewView(picker.list.View()) }

// Selection reports the chosen sorted index, or cancellation/interruption.
func (picker Picker) Selection() (int, error) { return picker.selected, picker.err }

func Select(subscriptions []profile.Subscription) (int, error) {
	SortSubscriptions(subscriptions)
	return SelectOrdered(subscriptions)
}

// SelectOrdered preserves caller ranking and returns an index into that slice.
func SelectOrdered(subscriptions []profile.Subscription) (int, error) {
	return selectOrdered(subscriptions, false)
}

// SelectTenantsOrdered accepts rows whose Id is the tenant GUID and Name its label.
func SelectTenantsOrdered(tenants []profile.Subscription) (int, error) {
	return selectOrdered(tenants, true)
}

func selectOrdered(subscriptions []profile.Subscription, tenants bool) (int, error) {
	options := []tea.ProgramOption{tea.WithOutput(os.Stderr)}
	if !log.ColorEnabled(os.Stderr) {
		options = append(options, tea.WithColorProfile(colorprofile.NoTTY))
	}
	width, height, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil {
		width, height = 100, 20
	}
	var input io.Reader = os.Stdin
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Hide the fd so Windows readers consume redirected stdin, not CONIN$.
		input = struct{ io.Reader }{os.Stdin}
	}
	picker := newOrderedPicker(subscriptions, width, height)
	if tenants {
		picker = NewTenantPicker(subscriptions, width, height)
	}
	options = append(options, tea.WithInput(input))
	result, err := tea.NewProgram(picker, options...).Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return -1, ErrInterrupted
	}
	if err != nil {
		return -1, err
	}
	final, ok := result.(Picker)
	if !ok {
		return -1, errors.New("unexpected picker result")
	}
	return final.Selection()
}

type subscriptionDelegate struct {
	nameWidth   int
	tenantWidth int
	tenants     bool
	color       bool
}

func (subscriptionDelegate) Height() int                         { return 1 }
func (subscriptionDelegate) Spacing() int                        { return 0 }
func (subscriptionDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (delegate subscriptionDelegate) Render(writer io.Writer, model list.Model, index int, item list.Item) {
	row, ok := item.(subscriptionItem)
	if !ok {
		return
	}
	subscription := row.subscription
	selected := index == model.Index()
	line := delegate.row(model.Width(), subscription.DisplayName(), subscription.Id, tenantLabel(subscription), selected)
	prefix := "  "
	style := lipgloss.NewStyle()
	if selected {
		prefix = "▸ "
		if delegate.color {
			style = style.Bold(true)
		}
	}
	_, _ = fmt.Fprint(writer, style.Render(prefix)+line)
}

func (delegate subscriptionDelegate) row(width int, name, id, tenant string, selected bool) string {
	name, id, tenant = DisplayText(name), DisplayText(id), DisplayText(tenant)
	style := lipgloss.NewStyle()
	if delegate.color && selected {
		style = style.Bold(true)
	}
	nameStyle, idStyle, tenantStyle := style, style, style
	if delegate.color {
		nameStyle = nameStyle.Foreground(lipgloss.Color("2"))
		idStyle = idStyle.Foreground(lipgloss.Color("6"))
		tenantStyle = tenantStyle.Faint(true)
	}
	if delegate.tenants {
		nameWidth := min(delegate.nameWidth, max(1, width-2-3-36))
		line := nameStyle.Render(singleLineCell(name, nameWidth)) + style.Render(" | ") + idStyle.Render(id)
		if width >= 2+nameWidth+3+36+3+delegate.tenantWidth {
			line += style.Render(" | ") + tenantStyle.Render(tenant)
		}
		return lipgloss.NewStyle().MaxWidth(max(1, width-2)).Render(line)
	}
	nameWidth := min(delegate.nameWidth, max(1, width-2))
	line := nameStyle.Render(singleLineCell(name, nameWidth))
	tenantWidth := delegate.tenantWidth
	switch {
	case width >= 2+nameWidth+3+36+3+tenantWidth:
		line += style.Render(" | ") + idStyle.Render(singleLineCell(id, 36)) + style.Render(" | ") + tenantStyle.Render(tenant)
	case width >= 2+nameWidth+3+tenantWidth:
		line += style.Render(" | ") + tenantStyle.Render(tenant)
	}
	return lipgloss.NewStyle().MaxWidth(max(1, width-2)).Render(line)
}

func singleLineCell(value string, width int) string {
	// Width wraps text, so truncate before applying column padding.
	value = lipgloss.NewStyle().MaxWidth(width).Render(value)
	return lipgloss.NewStyle().Width(width).Render(value)
}

func tenantLabel(subscription profile.Subscription) string {
	return subscription.TenantLabel()
}
