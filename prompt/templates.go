package prompt

var (
	template_Long = promptTemplate{
		Label:       "{{ repeat 4 \" \" }}{{ repeat %[1]d \" \" | print \"Name\" | trunc %[1]d }} {{ repeat 38 \" \" | print \"| SubscriptionId\" | trunc 38 }} {{ repeat %[2]d \" \" | print \"| Tenant\" | trunc %[2]d }}",
		Active:      "▸ {{ repeat %[1]d \" \" | print .Name | trunc %[1]d | green | %[3]s }} | {{ repeat 36 \" \" | print .Id | trunc 36 | cyan | %[3]s }} | {{ repeat %[2]d \" \" | print \")\" | print .Tenant | print \" (\" | print .TenantName | trunc %[2]d | faint | %[3]s }}",
		Inactive:    "{{ repeat 2 \" \" }}{{ repeat %[1]d \" \" | print .Name | trunc %[1]d | green | %[3]s }} | {{ repeat 36 \" \" | print .Id | trunc 36 | cyan | %[3]s }} | {{ repeat %[2]d \" \" | print \")\" | print .Tenant | print \" (\" | print .TenantName | trunc %[2]d | faint | %[3]s }}",
		IncludesIds: true,
	}
	template_Short = promptTemplate{
		Label:       "{{ repeat 4 \" \" }}{{ repeat %[1]d \" \" | print \"Name\" | trunc %[1]d }} {{ repeat %[2]d \" \" | print \"Tenant\" | trunc %[2]d }}",
		Active:      "▸ {{ repeat %[1]d \" \" | print .Name | trunc %[1]d | green | %[3]s }} | {{ repeat %[2]d \" \" | print .TenantName | trunc %[2]d | cyan | %[3]s }}",
		Inactive:    "{{ repeat 2 \" \" }}{{ repeat %[1]d \" \" | print .Name | trunc %[1]d | green | %[3]s }} | {{ repeat %[2]d \" \" | print .TenantName | trunc %[2]d | cyan | %[3]s }}",
		IncludesIds: false,
	}
	template_VeryShort = promptTemplate{
		Label:       "{{ repeat 4 \" \" }}{{ repeat %[1]d \" \" | print \"Name\" | trunc %[1]d }}",
		Active:      "▸ {{ repeat %[1]d \" \" | print .Name | trunc %[1]d | green | %[3]s }}",
		Inactive:    "{{ repeat 2 \" \" }}{{ repeat %[1]d \" \" | print .Name | trunc %[1]d | green | %[3]s }}",
		IncludesIds: false,
	}
)

// templateName returns the template to use
// Todo: verify if the numbers for calculating the template are correct
func template(terminalWidth int, maxSubscriptionsLength, maxTenantsLength, maxTenantsWithIdLength int) promptTemplate {
	// Determine the template based on the terminal width
	switch {

	// +50, subscriptionId is 36 chars, + 4 spaces / seperator, + 10 from the previous case
	case terminalWidth > maxSubscriptionsLength+maxTenantsWithIdLength+36+4+10:
		return template_Long

	// +10, arbitrary / magic number
	case terminalWidth > maxSubscriptionsLength+maxTenantsLength+10:
		return template_Short

	default:
		return template_VeryShort

	}
}
