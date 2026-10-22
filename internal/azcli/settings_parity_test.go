package azcli_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
)

func TestSettingsINISectionHeaderTrailingTextMatchesConfigParser(t *testing.T) {
	const customCloud = "CustomCloud"
	// Literal expectations verified with Python's ConfigParser(interpolation=None).
	for _, tc := range []struct {
		name, data, section, cloud string
	}{
		{"hash comment", "[cloud] # active cloud\nname=CustomCloud\n", "cloud", customCloud},
		{"semicolon comment", "[AzureCloud] ; comment\nname=CustomCloud\n", publicCloud, publicCloud},
		{"trailing text", "[core] trailing text\nname=CustomCloud\n", "core", publicCloud},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte(tc.data)}}
			settings, err := azcli.ReadSettings(files, func(string) (string, bool) { return "", false })
			if err != nil {
				t.Fatal(err)
			}
			if got := settings.Get(tc.section, "name", "missing"); got != customCloud {
				t.Fatalf("ConfigParser section %q name=%q, want %q", tc.section, got, customCloud)
			}
			if got := settings.ActiveCloud(); got != tc.cloud {
				t.Fatalf("active cloud=%q, want %q", got, tc.cloud)
			}
		})
	}
}

func TestSettingsINISectionHeaderGreedyBracketMatchesConfigParser(t *testing.T) {
	const customCloud = "CustomCloud"
	// ConfigParser greedily includes every character before the last closing bracket.
	for _, tc := range []struct {
		name, data, section string
	}{
		{"embedded closing bracket", "[a]b]\nname=CustomCloud\n", "a]b"},
		{"closing bracket in comment", "[cloud] # active [cloud]\nname=CustomCloud\n", "cloud] # active [cloud"},
		{"closing bracket as section name", "[]]\nname=CustomCloud\n", "]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte(tc.data)}}
			settings, err := azcli.ReadSettings(files, func(string) (string, bool) { return "", false })
			if err != nil {
				t.Fatal(err)
			}
			if got := settings.Get(tc.section, "name", "missing"); got != customCloud {
				t.Fatalf("ConfigParser section %q name=%q, want %q", tc.section, got, customCloud)
			}
			if got := settings.ActiveCloud(); got != publicCloud {
				t.Fatalf("active cloud=%q, want %q", got, publicCloud)
			}
		})
	}
}

func TestSettingsINISectionHeaderValidation(t *testing.T) {
	for _, tc := range []struct {
		name, data string
	}{
		{"unclosed section", "[cloud"},
		{"empty section", "[] # no section name\n"},
		{"duplicate section with comments", "[cloud] # first\n[cloud] ; duplicate\n"},
		{"duplicate greedy section", "[a]b]\n[a]b] # duplicate\n"},
		{"oversized section", "[" + strings.Repeat("a", 1024*1024) + "]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte(tc.data)}}
			if _, err := azcli.ReadSettings(files, func(string) (string, bool) { return "", false }); err == nil {
				t.Fatal("accepted invalid section header")
			}
		})
	}
}

func TestSettingsINIIndentationMatchesConfigParser(t *testing.T) {
	const customCloud = "CustomCloud"
	// Expected values verified with Python's stdlib ConfigParser(interpolation=None).
	for _, tc := range []struct {
		name, data, option, coreName, cloud string
	}{
		{"indented option is continuation", "[core]\noption=value\n  name=CustomCloud\n", "value\nname=CustomCloud", "", publicCloud},
		{"indented section is continuation", "[core]\noption=value\n  [cloud]\nname=CustomCloud\n", "value\n[cloud]", customCloud, publicCloud},
		{"aligned options and sections", "[core]\noption=value\n[cloud]\nname=CustomCloud\n", "value", "", customCloud},
		{"uniformly indented options and sections", " [core]\n option=value\n [cloud]\n name=CustomCloud\n", "value", "", customCloud},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte(tc.data)}}
			settings, err := azcli.ReadSettings(files, func(string) (string, bool) { return "", false })
			if err != nil {
				t.Fatal(err)
			}
			if settings.Get("core", "option", "") != tc.option || settings.Get("core", "name", "") != tc.coreName || settings.ActiveCloud() != tc.cloud {
				t.Fatalf("ConfigParser parity: option=%q core.name=%q cloud=%q", settings.Get("core", "option", ""), settings.Get("core", "name", ""), settings.ActiveCloud())
			}
		})
	}
}
