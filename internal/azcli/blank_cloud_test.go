package azcli_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestSettingsBlankCloudFallsBack(t *testing.T) {
	for _, value := range []string{"", " \t "} {
		for _, override := range []bool{false, true} {
			files := fstest.MapFS{settingsFilename: &fstest.MapFile{Data: []byte("[cloud]\nname=" + value)}}
			lookup := func(string) (string, bool) { return "", false }
			if override {
				files[settingsFilename].Data = []byte("[cloud]\nname=CustomCloud")
				lookup = func(key string) (string, bool) { return value, key == "AZURE_CLOUD_NAME" }
			}
			settings, err := azcli.ReadSettings(files, lookup)
			if err != nil || settings.ActiveCloud() != publicCloud {
				t.Errorf("value=%q override=%t: active=%q err=%v", value, override, settings.ActiveCloud(), err)
			}
		}
	}
}

func TestCloudBlankOverrideNeverSelectsMissingEnvironment(t *testing.T) {
	for _, value := range []string{"", " \t "} {
		for _, remembered := range []bool{false, true} {
			t.Run(value+map[bool]string{false: "/profile-default", true: "/remembered-default"}[remembered], func(t *testing.T) {
				t.Setenv("AZURE_CLOUD_NAME", value)
				data := strings.Replace(cloudProfile, `"name":"Unknown cloud"`, `"name":"Unknown cloud","isDefault":true`, 1)
				if remembered {
					data = strings.Replace(data, `"isDefault":true`, `"isDefault":false`, 1)
				}
				fake := &testutil.FakeRunner{}
				cli, err := azcli.New(fstest.MapFS{
					profileFilename:  &fstest.MapFile{Data: []byte(data)},
					settingsFilename: &fstest.MapFile{Data: []byte("[cloud]\nname= \t")},
					cloudsFilename:   &fstest.MapFile{Data: []byte("[AzureCloud]\nsubscription=" + publicID)},
				}, fake, "")
				if err != nil {
					t.Fatal(err)
				}
				visible := cli.Subscriptions()
				current, err := cli.ActiveSubscription()
				if len(visible) != 1 || visible[0].Id != publicID || !visible[0].IsDefault || err != nil || current.Id != publicID {
					t.Fatalf("blank active cloud: visible=%+v current=%+v err=%v", visible, current, err)
				}
				cli.IncludeAll = true
				all := cli.Subscriptions()
				if len(all) != 7 || all[6].IsDefault {
					t.Fatalf("unknown environment marked current: %+v", all)
				}
				if _, err := cli.TryFindSubscription(all[6].Id); err == nil {
					t.Fatal("--all made missing environment selectable")
				}
				if err := cli.SetSubscription(all[6]); err == nil || len(fake.Calls) != 0 {
					t.Fatalf("missing environment reached az: err=%v calls=%+v", err, fake.Calls)
				}
			})
		}
	}
}
