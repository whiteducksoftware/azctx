package azcli_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
)

const (
	settingsFilename = "config"
	cloudsFilename   = "clouds.config"
	jsonOutput       = "json"
	boolOn           = "on"
)

func TestSettingsINIAndOverrides(t *testing.T) {
	files := fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte("# comment\r\n[cloud]\r\nNaMe = CustomCloud\r\n[core]\r\nOUTPUT: table\r\n; comment\r\nliteral = value # not an inline comment\r\n")}}
	env := map[string]string{"AZURE_CORE_OUTPUT": jsonOutput, "AZURE_CORE_LOGIN_EXPERIENCE_V2": "off"}
	settings, err := azcli.ReadSettings(files, func(key string) (string, bool) { value, ok := env[key]; return value, ok })
	if err != nil {
		t.Fatal(err)
	}
	if settings.ActiveCloud() != "CustomCloud" || settings.Get("core", "OuTpUt", "") != jsonOutput ||
		settings.Get("core", "literal", "") != "value # not an inline comment" {
		t.Fatalf("INI/override settings: %+v", settings)
	}
	value, err := settings.Bool("core", "login_experience_v2", true)
	if err != nil || value {
		t.Fatalf("environment boolean=%v, %v", value, err)
	}
}

func TestSettingsBooleanSpellings(t *testing.T) {
	for _, spelling := range []string{"1", "yes", "true", boolOn, "0", "no", "false", "off", "YeS", "OFF", "invalid", ""} {
		t.Run(spelling, func(t *testing.T) {
			files := fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte("[core]\nvalue=" + spelling)}}
			settings, err := azcli.ReadSettings(files, func(string) (string, bool) { return "", false })
			if err != nil {
				t.Fatal(err)
			}
			got, err := settings.Bool("core", "VALUE", false)
			if spelling == "invalid" || spelling == "" {
				if err == nil {
					t.Fatalf("accepted invalid boolean %q", spelling)
				}
				return
			}
			want := spelling == "1" || strings.EqualFold(spelling, "yes") || spelling == "true" || spelling == boolOn
			if err != nil || got != want {
				t.Fatalf("%q = %v, %v", spelling, got, err)
			}
		})
	}
}

func TestSettingsMissingMalformedAndCase(t *testing.T) {
	noEnv := func(string) (string, bool) { return "", false }
	settings, err := azcli.ReadSettings(fstest.MapFS{}, noEnv)
	if err != nil || settings.ActiveCloud() != "AzureCloud" || settings.Get("core", "missing", "fallback") != "fallback" {
		t.Fatalf("missing config: %+v, %v", settings, err)
	}

	if got, err := settings.Bool("core", "missing", true); err != nil || !got {
		t.Fatalf("missing boolean=%v, %v", got, err)
	}
	for _, data := range []string{"not a section", "[broken", "[core]\nbroken", "[core]\n=empty", "[core]\na=1\nA=2", "[core]\n[core]\na=1"} {
		if _, err := azcli.ReadSettings(fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte(data)}}, noEnv); err == nil {
			t.Fatalf("accepted malformed config %q", data)
		}
	}
	settings, err = azcli.ReadSettings(fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte("[Cloud]\nname=NotActive\n[core]\noutput=table")}}, noEnv)
	if err != nil || settings.ActiveCloud() != "AzureCloud" {
		t.Fatalf("sections must be case sensitive: %+v, %v", settings, err)
	}
	settings, err = azcli.ReadSettings(fstest.MapFS{}, func(key string) (string, bool) { return "EnvCloud", key == "AZURE_CLOUD_NAME" })
	if err != nil || settings.ActiveCloud() != "EnvCloud" {
		t.Fatalf("cloud override=%q, %v", settings.ActiveCloud(), err)
	}
}

type deniedSettingsFS struct{}

func (deniedSettingsFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

func TestSettingsPermissionErrorIsNotMissing(t *testing.T) {
	if _, err := azcli.ReadSettings(deniedSettingsFS{}, nil); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("lost settings read failure: %v", err)
	}
}

func TestSettingsDefaultsContinuationAndLiteralValues(t *testing.T) {
	files := fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte("\xef\xbb\xbf" + "[DEFAULT]\noutput=table\n[core]\nmessage=first\n  second\nliteral=10% # literal\n")}}
	settings, err := azcli.ReadSettings(files, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if settings.Get("core", "output", "") != "table" || settings.Get("core", "message", "") != "first\nsecond" ||
		settings.Get("core", "literal", "") != "10% # literal" {
		t.Fatalf("INI values: %+v", settings)
	}
}
