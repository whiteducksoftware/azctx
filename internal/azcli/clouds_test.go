package azcli_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	publicID    = "11111111-1111-1111-1111-111111111111"
	privateID   = "22222222-2222-2222-2222-222222222222"
	publicCloud = "AzureCloud"
)

const cloudProfile = `{"subscriptions":[
	{"id":"` + publicID + `","name":"Public","environmentName":"AzureCloud","isDefault":true},
	{"id":"` + privateID + `","name":"Private","environmentName":"CustomCloud"},
	{"id":"33333333-3333-3333-3333-333333333333","name":"Disabled","environmentName":"CustomCloud","state":"Disabled"},
	{"id":"44444444-4444-4444-4444-444444444444","name":"Warning","environmentName":"CustomCloud","state":"Warned"},
	{"id":"55555555-5555-5555-5555-555555555555","name":"Past due","environmentName":"CustomCloud","state":"PastDue"},
	{"id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","name":"N/A(tenant level account)","environmentName":"CustomCloud"},
	{"id":"66666666-6666-6666-6666-666666666666","name":"Unknown cloud"}
]}`

func TestCloudVisibilityAndPerCloudDefault(t *testing.T) {
	t.Setenv("AZURE_CLOUD_NAME", "CustomCloud")
	files := fstest.MapFS{
		profileFilename:  &fstest.MapFile{Data: []byte(cloudProfile)},
		settingsFilename: &fstest.MapFile{Data: []byte("[cloud]\nname=AzureCloud")},
		cloudsFilename:   &fstest.MapFile{Data: []byte("[AzureCloud]\nsubscription=" + publicID + "\n[CustomCloud]\nSuBsCrIpTiOn=" + strings.ToUpper(privateID))},
	}
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	cli, err := azcli.New(files, fake, "")
	if err != nil {
		t.Fatal(err)
	}
	subscriptions := cli.Subscriptions()
	if len(subscriptions) != 3 || subscriptions[0].Id != privateID || !subscriptions[0].IsDefault {
		t.Fatalf("active-cloud visible subscriptions=%+v", subscriptions)
	}
	current, err := cli.ActiveSubscription()
	if err != nil || current.Id != privateID {
		t.Fatalf("per-cloud current=%+v, %v", current, err)
	}
	cli.IncludeAll = true
	all := cli.Subscriptions()
	if len(all) != 7 || all[0].IsDefault || !all[1].IsDefault {
		t.Fatalf("all entries / current flags=%+v", all)
	}
	if err := cli.SetSubscription(all[0]); err == nil {
		t.Fatal("foreign cloud must never reach az account set")
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("foreign-cloud az invocation: %+v", fake.Calls)
	}
	if _, err := cli.TryFindSubscription("Public"); err == nil {
		t.Fatal("--all must not make a foreign cloud selectable")
	}
}

func TestCloudDefaultsRespectProfileAndNeverSelectForeignID(t *testing.T) {
	for _, tc := range []struct {
		name, data, clouds, want string
	}{
		{"profile default", `{"subscriptions":[{"id":"` + privateID + `","environmentName":"CustomCloud","isDefault":true}]}`, "", privateID},
		{"profile takes precedence", `{"subscriptions":[{"id":"` + privateID + `","environmentName":"CustomCloud","isDefault":true}]}`, "[CustomCloud]\nsubscription=" + publicID, privateID},
		{"foreign remembered ID", cloudProfile, "[CustomCloud]\nsubscription=" + publicID, ""},
		{"missing remembered ID", cloudProfile, "[CustomCloud]\nsubscription=77777777-7777-7777-7777-777777777777", ""},
		{"multiple profile defaults", `{"subscriptions":[{"id":"` + privateID + `","environmentName":"CustomCloud","isDefault":true},{"id":"` + publicID + `","environmentName":"CustomCloud","isDefault":true}]}`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AZURE_CLOUD_NAME", "CustomCloud")
			files := fstest.MapFS{
				profileFilename: &fstest.MapFile{Data: []byte(tc.data)},
				cloudsFilename:  &fstest.MapFile{Data: []byte(tc.clouds)},
			}
			cli, err := azcli.New(files, &testutil.FakeRunner{}, "")
			if err != nil {
				t.Fatal(err)
			}
			got, err := cli.ActiveSubscription()
			if tc.want == "" {
				if err == nil {
					t.Fatalf("misleading active subscription: %+v", got)
				}
				for _, sub := range cli.Subscriptions() {
					if sub.IsDefault {
						t.Fatalf("ambiguous/stale default marked current: %+v", sub)
					}
				}
			} else if err != nil || got.Id != tc.want {
				t.Fatalf("current=%+v, %v", got, err)
			}
		})
	}
}

func TestMalformedCloudFiles(t *testing.T) {
	for _, name := range []string{settingsFilename, cloudsFilename} {
		if _, err := azcli.New(fstest.MapFS{
			profileFilename: &fstest.MapFile{Data: []byte(cloudProfile)},
			name:            &fstest.MapFile{Data: []byte("[broken")},
		}, &testutil.FakeRunner{}, ""); err == nil {
			t.Fatalf("accepted malformed %s", name)
		}
	}
	t.Setenv("AZURE_CLOUD_NAME", "CustomCloud")
	if _, err := azcli.New(fstest.MapFS{
		profileFilename: &fstest.MapFile{Data: []byte(cloudProfile)},
		cloudsFilename:  &fstest.MapFile{Data: []byte("[CustomCloud]\nsubscription=not-a-guid")},
	}, &testutil.FakeRunner{}, ""); err == nil || !strings.Contains(err.Error(), cloudsFilename) {
		t.Fatalf("invalid per-cloud GUID: %v", err)
	}
}
