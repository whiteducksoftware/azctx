package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	templates "text/template"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/log"

	"github.com/Masterminds/sprig/v3"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/manifoldco/promptui"
	"github.com/olekukonko/ts"
)

// BuildPrompt builds a prompt for the user to select a subscription
func BuildPrompt(subscriptions []profile.Subscription) promptui.Select {
	// Get the terminal dimensions
	var terminalWidth, terminalHeigth int
	if size, err := ts.GetSize(); err != nil {
		terminalWidth = 100 // Default width
		terminalHeigth = 20 // Default height
		log.Warn("Unable to get terminal dimensions, using default values (width: %d, height: %d)", terminalWidth, terminalHeigth)
	} else {
		// Set the terminal dimensions
		terminalWidth = size.Col()
		terminalHeigth = size.Row()
	}

	// Sort the subscriptions by name
	slices.SortFunc(subscriptions, func(a, b profile.Subscription) int {
		if strings.EqualFold(a.TenantName, b.TenantName) {
			return cmp.Compare(a.Name, b.Name)
		}
		return cmp.Compare(a.TenantName, b.TenantName)
	})

	// Prepare the prompt
	subscriptionNames := make([]string, len(subscriptions))
	maxSubscriptionsLength, tenantNamesMaxLength := 0, 0
	for i, subscription := range subscriptions {
		subscriptionNames[i] = subscription.Name
		maxSubscriptionsLength = max(maxSubscriptionsLength, len(subscription.Name))
		tenantNamesMaxLength = max(tenantNamesMaxLength, len(subscription.TenantName))
	}

	// Fetch the correct template                                                subscriptionId is 36 chars, + 3 for () and a space
	tpl := template(terminalWidth, maxSubscriptionsLength, tenantNamesMaxLength, tenantNamesMaxLength+36+3)

	// Determine the max length of the tenants
	maxTenantsLength := 0
	if tpl.IncludesIds {
		maxTenantsLength = tenantNamesMaxLength + 36 + 3
	} else {
		maxTenantsLength = tenantNamesMaxLength
	}

	// Return the prompt
	return promptui.Select{
		Items: subscriptions,
		Templates: &promptui.SelectTemplates{
			Label:    fmt.Sprintf(tpl.Label, maxSubscriptionsLength, maxTenantsLength),
			Inactive: builItemTemplate(tpl.Inactive, maxSubscriptionsLength, maxTenantsLength, ""),
			Active:   builItemTemplate(tpl.Active, maxSubscriptionsLength, maxTenantsLength, "bold"),
			FuncMap:  newTemplateFuncMap(),
		},
		HideSelected: true,
		Searcher: func(input string, index int) bool {
			return fuzzy.MatchNormalized(strings.ToLower(input), strings.ToLower(subscriptionNames[index]))
		},
		Size:   min(len(subscriptions), max(min(terminalHeigth-3, 10), 1)),
		Stdout: &noBellStdout{},
	}
}

// buildItemTemplate builds the item template
func builItemTemplate(template string, maxSubscriptionsLength, maxTenantsLength int, additionalStyle string) string {
	return fmt.Sprintf(template, maxSubscriptionsLength, maxTenantsLength, additionalStyle)
}

// newTemplateFuncMap builds the template function map
func newTemplateFuncMap() templates.FuncMap {
	ret := sprig.TxtFuncMap()
	ret["green"] = promptui.Styler(promptui.FGGreen)
	ret["cyan"] = promptui.Styler(promptui.FGCyan)
	ret["bold"] = promptui.Styler(promptui.FGBold)
	ret["faint"] = promptui.Styler(promptui.FGFaint)
	return ret
}

// Select runs the existing picker and returns an index into the sorted slice.
func Select(subscriptions []profile.Subscription) (int, error) {
	picker := BuildPrompt(subscriptions)
	index, _, err := picker.Run()
	return index, err
}
