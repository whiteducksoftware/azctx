package prompt

import "testing"

func TestPad(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		width    int
		expected string
	}{
		{
			name:     "pads ASCII string",
			value:    "azctx",
			width:    8,
			expected: "azctx   ",
		},
		{
			name:     "handles multibyte characters without shifting",
			value:    "Bezahlung über ADN",
			width:    22,
			expected: "Bezahlung über ADN    ",
		},
		{
			name:     "truncates without breaking utf8",
			value:    "überraschung",
			width:    4,
			expected: "über",
		},
		{
			name:     "returns empty string for non-positive widths",
			value:    "foo",
			width:    0,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pad(tt.value, tt.width); got != tt.expected {
				t.Fatalf("pad(%q, %d) = %q, want %q", tt.value, tt.width, got, tt.expected)
			}
		})
	}
}
