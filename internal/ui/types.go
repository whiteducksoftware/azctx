package ui

// promptTemplate represents the json template for the prompt (see templates/*.json)
type promptTemplate struct {
	Label    string
	Active   string
	Inactive string

	// IncludesIds indicates if the template includes the tenant ids & subscription ids
	IncludesIds bool
}
