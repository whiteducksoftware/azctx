package azcli_test

import (
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
)

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
