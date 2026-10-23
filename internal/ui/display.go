package ui

import (
	"strings"
	"unicode"
)

// DisplayText flattens controls for human displays, never identity or data output.
func DisplayText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}
