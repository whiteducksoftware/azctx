package prompt

var (
	template_Long = promptTemplate{
		Label:       "{{ repeat 4 \" \" }}{{ pad \"Name\" %[1]d }} | {{ pad \"SubscriptionId\" 36 }} | {{ pad \"Tenant\" %[2]d }}",
		Active:      "▸ {{ pad .Name %[1]d | green | %[3]s }} | {{ pad .Id 36 | cyan | %[3]s }} | {{ pad (printf \"%s (%s)\" .Tenant .TenantName) %[2]d | faint | %[3]s }}",
		Inactive:    "{{ repeat 2 \" \" }}{{ pad .Name %[1]d | green | %[3]s }} | {{ pad .Id 36 | cyan | %[3]s }} | {{ pad (printf \"%s (%s)\" .Tenant .TenantName) %[2]d | faint | %[3]s }}",
		IncludesIds: true,
	}
	template_Short = promptTemplate{
		Label:       "{{ repeat 4 \" \" }}{{ pad \"Name\" %[1]d }} {{ pad \"Tenant\" %[2]d }}",
		Active:      "▸ {{ pad .Name %[1]d | green | %[3]s }} | {{ pad .TenantName %[2]d | cyan | %[3]s }}",
		Inactive:    "{{ repeat 2 \" \" }}{{ pad .Name %[1]d | green | %[3]s }} | {{ pad .TenantName %[2]d | cyan | %[3]s }}",
		IncludesIds: false,
	}
	template_VeryShort = promptTemplate{
		Label:       "{{ repeat 4 \" \" }}{{ pad \"Name\" %[1]d }}",
		Active:      "▸ {{ pad .Name %[1]d | green | %[3]s }}",
		Inactive:    "{{ repeat 2 \" \" }}{{ pad .Name %[1]d | green | %[3]s }}",
		IncludesIds: false,
	}
)

// templateName returns the template to use
// Todo: verify if the numbers for calculating the template are correct
func template(terminalWidth int, maxSubscriptionsLength, maxTenantsLength, maxTenantsWithIdLength int) promptTemplate {
	// Determine the template based on the terminal width
	switch {
	// +50, subscriptionId is 36 chars, + 4 spaces / separator, + 10 from the previous case
	case terminalWidth > maxSubscriptionsLength+maxTenantsWithIdLength+36+4+10:
		return template_Long

	// +10, arbitrary / magic number
	case terminalWidth > maxSubscriptionsLength+maxTenantsLength+10:
		return template_Short

	default:
		return template_VeryShort
	}
}
