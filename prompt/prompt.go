package prompt

import (
	"fmt"
	"sort"
	"strings"
	templates "text/template"

	"github.com/whiteducksoftware/azctx/azurecli"
	"github.com/whiteducksoftware/azctx/log"
	"github.com/whiteducksoftware/azctx/utils"

	"github.com/Masterminds/sprig/v3"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/manifoldco/promptui"
	"github.com/olekukonko/ts"
)

// BuildPrompt builds a prompt for the user to select a subscription
func BuildPrompt(subscriptions utils.ComparableNamedSlice[azurecli.Subscription]) promptui.Select {
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
	sort.Sort(subscriptions)

	// Prepare the prompt
	subscriptionNames := utils.StringSlice(subscriptions.Names())
	maxSubscriptionsLength := subscriptionNames.LongestLength()
	tenantNames := calculateTenantNames(subscriptions)
	tenantNamesMaxLength := tenantNames.LongestLength()

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
		Size:   utils.Min(len(subscriptions), utils.Max(utils.Min(terminalHeigth-3, 10), 1)),
		Stdout: utils.NoBellStdout,
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

// calculateTenantNames returns the tenant names of the given subscriptions
func calculateTenantNames(subscriptions []azurecli.Subscription) (tenantNames utils.StringSlice) {
	for _, subscription := range subscriptions {
		tenantNames = append(tenantNames, subscription.TenantName)
	}

	return
}
