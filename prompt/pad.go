package prompt

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// pad returns the input string truncated to the given visual width and padded with spaces on the right.
func pad(value string, width int) string {
	if width <= 0 {
		return ""
	}

	truncated := runewidth.Truncate(value, width, "")
	padding := width - runewidth.StringWidth(truncated)
	if padding <= 0 {
		return truncated
	}

	return truncated + strings.Repeat(" ", padding)
}
