package utils

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// StringSlice is a slice of strings, helper type used for extension methods
type StringSlice []string

// LongestLength returns the length of the longest string in the given slice
func (slice StringSlice) LongestLength() int {
	longestLength := 0
	for _, s := range slice {
		if width := runewidth.StringWidth(s); width > longestLength {
			longestLength = width
		}
	}
	return longestLength
}

// ToLower returns a copy of the given slice of strings with all strings lowercased
func (slice StringSlice) ToLower() StringSlice {
	lowercaseStrings := make(StringSlice, len(slice))
	for i, s := range slice {
		lowercaseStrings[i] = strings.ToLower(s)
	}
	return lowercaseStrings
}
