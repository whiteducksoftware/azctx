package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const (
	columns    = 92
	pickerRows = 16
	command    = "azctx"
)

type caption struct {
	keys []string
	text string
}

type step struct {
	keys    []tea.KeyPressMsg
	caption caption
	// Delay in GIF units of 10 ms.
	delay int
}

type frame struct {
	content string
	caption caption
	delay   int
}

const (
	contosoTenant   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	fabrikamTenant  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	northwindTenant = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	enabled         = "Enabled"
)

// demoSubscriptions are synthetic identities; none refers to a real account.
func demoSubscriptions() []profile.Subscription {
	row := func(id, name, tenant, tenantName string) profile.Subscription {
		return profile.Subscription{Id: id, Name: name, State: enabled, Tenant: tenant, TenantName: tenantName}
	}
	return []profile.Subscription{
		row("10000000-0000-4000-8000-000000000001", "Web Platform Dev", contosoTenant, "Contoso"),
		row("10000000-0000-4000-8000-000000000002", "Web Platform Staging", contosoTenant, "Contoso"),
		row("10000000-0000-4000-8000-000000000003", "Web Platform Production", contosoTenant, "Contoso"),
		row("10000000-0000-4000-8000-000000000004", "Connectivity Hub", contosoTenant, "Contoso"),
		row("20000000-0000-4000-8000-000000000001", "Web Platform Dev", fabrikamTenant, "Fabrikam"),
		row("20000000-0000-4000-8000-000000000002", "Web Platform Staging", fabrikamTenant, "Fabrikam"),
		row("20000000-0000-4000-8000-000000000003", "Web Platform Production", fabrikamTenant, "Fabrikam"),
		row("20000000-0000-4000-8000-000000000004", "Data Lake Production", fabrikamTenant, "Fabrikam"),
		row("30000000-0000-4000-8000-000000000001", "ML Sandbox Dev", northwindTenant, "Northwind Traders"),
		row("30000000-0000-4000-8000-000000000002", "ML Training Staging", northwindTenant, "Northwind Traders"),
	}
}

func text(value string) []tea.KeyPressMsg {
	keys := make([]tea.KeyPressMsg, 0, len(value))
	for _, r := range value {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

func key(code rune) []tea.KeyPressMsg { return []tea.KeyPressMsg{{Code: code}} }

func typed(value string, last int, explanation string) []step {
	steps := make([]step, 0, len(value))
	for index, keys := range text(value) {
		delay := 35
		if index == len(value)-1 {
			delay = last
		}
		steps = append(steps, step{keys: []tea.KeyPressMsg{keys}, caption: caption{[]string{value}, explanation}, delay: delay})
	}
	return steps
}

func scenario() []step {
	move := caption{[]string{"↓"}, "move the highlight"}
	steps := []step{
		{caption: caption{nil, "10 synthetic subscriptions in 3 tenants: dev, staging and production"}, delay: 260},
		{keys: key(tea.KeyDown), caption: move, delay: 60},
		{keys: key(tea.KeyDown), caption: move, delay: 100},
		{keys: text("/"), caption: caption{[]string{"/"}, "filter by subscription name, tenant or GUID"}, delay: 100},
	}
	steps = append(steps, typed("prod", 220, "the same name in two tenants stays distinct by tenant")...)
	steps = append(steps,
		step{keys: key(tea.KeyEscape), caption: caption{[]string{"esc"}, "cancel the filter"}, delay: 110},
		step{keys: text("/"), caption: caption{[]string{"/"}, "filter again, this time by tenant"}, delay: 70},
	)
	steps = append(steps, typed("fab", 180, "tenant names match too")...)
	choose := caption{[]string{"↓"}, "choose a subscription"}
	return append(steps,
		step{keys: key(tea.KeyEnter), caption: caption{[]string{"enter"}, "apply the filter"}, delay: 110},
		step{keys: key(tea.KeyDown), caption: choose, delay: 45},
		step{keys: key(tea.KeyDown), caption: choose, delay: 45},
		step{keys: key(tea.KeyDown), caption: choose, delay: 60},
		step{keys: key(tea.KeyEnter), caption: caption{[]string{"enter"}, "select Fabrikam / Web Platform Staging; successful switches are quiet"}, delay: 320},
	)
}

// captureFrames drives the production picker model with real key messages.
func captureFrames() ([]frame, ui.Picker, error) {
	var model tea.Model = ui.NewPicker(demoSubscriptions(), columns, pickerRows)
	// The same reply a dark terminal gives to the picker's background query.
	model, _ = model.Update(tea.BackgroundColorMsg{Color: terminalBackground})
	var frames []frame
	for _, step := range scenario() {
		for _, key := range step.keys {
			model, _ = model.Update(key)
		}
		frames = append(frames, frame{content: model.View().Content, caption: step.caption, delay: step.delay})
	}
	picker, ok := model.(ui.Picker)
	if !ok {
		return nil, ui.Picker{}, fmt.Errorf("unexpected picker model %T", model)
	}
	return frames, picker, nil
}
