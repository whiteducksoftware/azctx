package cmd_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const modernProfile = `{"subscriptions":[
	{"id":"` + productionID + `","name":"Production","tenantId":"` + tenantID + `","tenantDisplayName":"Example","tenantDefaultDomain":"example.invalid","environmentName":"AzureCloud","isDefault":true,"state":"Enabled","user":{"name":"synthetic@example.invalid","type":"user"}},
	{"id":"` + developmentID + `","name":"Delegated","tenantId":"` + tenantID + `","tenantDisplayName":"Example","tenantDefaultDomain":"example.invalid","environmentName":"AzureCloud","state":"Warned","homeTenantId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","managedByTenants":[{"tenantId":"` + tenantID + `"}],"user":{"name":"systemAssignedIdentity","type":"servicePrincipal"}},
	{"id":"33333333-3333-3333-3333-333333333333","name":"Hidden","environmentName":"AzureCloud","state":"Disabled"},
	{"id":"44444444-4444-4444-4444-444444444444","name":"Foreign","environmentName":"CustomCloud","isDefault":true},
	{"id":"` + tenantID + `","name":"N/A(tenant level account)","environmentName":"AzureCloud"},
	{"id":"55555555-5555-5555-5555-555555555555","name":"Past due","environmentName":"AzureCloud","state":"PastDue","user":{"name":"userAssignedIdentity","type":"servicePrincipal"}},
	{"id":"66666666-6666-6666-6666-666666666666","name":"Automation","environmentName":"AzureCloud","user":{"name":"synthetic-app","type":"servicePrincipal"}}
]}`

const (
	azureCloudName   = "AzureCloud"
	customCloudName  = "CustomCloud"
	azureConfigFile  = "config"
	cloudsConfigFile = "clouds.config"
	subscriptionFlag = "--subscription"
	tenantFlag       = "--tenant"
	tableFormat      = "table"
	accountCommand   = "account"
	setCommand       = "set"
)

func TestAzureCLIProfileProjectionAndVisibility(t *testing.T) {
	for _, format := range []string{jsonFormat, tableFormat} {
		for _, all := range []bool{false, true} {
			t.Run(format+map[bool]string{true: "-all"}[all], func(t *testing.T) {
				deps := commandDependencies(t, &testutil.FakeRunner{})
				deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(modernProfile)}}
				args := []string{listCommand, outputFlag, format}
				if all {
					args = append(args, "--all")
				}

				var stdout, stderr bytes.Buffer
				if code := cmd.Run(args, deps, bytes.NewReader(nil), &stdout, &stderr); code != 0 {
					t.Fatalf("code=%d stderr=%q", code, stderr.String())
				}
				if format == jsonFormat {
					assertModernJSON(t, stdout.Bytes(), all)
					if stderr.Len() != 0 {
						t.Fatalf("JSON diagnostic noise: %q", stderr.String())
					}
				} else {
					for _, label := range []string{"Example", "Warned", "PastDue", "system-assigned identity", "user-assigned identity", "service principal", "user", "delegated"} {
						if !strings.Contains(stdout.String(), label) {
							t.Fatalf("missing %q label: %s", label, stdout.String())
						}
					}
				}
				if strings.Contains(stdout.String(), "Hidden") != all || strings.Contains(stdout.String(), "Foreign") != all ||
					strings.Contains(stdout.String(), "N/A(tenant level account)") != all {
					t.Fatalf("hidden visibility: %s", stdout.String())
				}
			})
		}
	}
}

func assertModernJSON(t *testing.T, data []byte, all bool) {
	t.Helper()
	var result struct {
		SchemaVersion int                      `json:"schemaVersion"`
		Subscriptions []cmd.SubscriptionOutput `json:"subscriptions"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	wantCount := 4
	if all {
		wantCount = 7
	}
	if result.SchemaVersion != 1 || len(result.Subscriptions) != wantCount ||
		result.Subscriptions[0].TenantName != "Example" || result.Subscriptions[0].TenantDomain != "example.invalid" ||
		!result.Subscriptions[1].Delegated || result.Subscriptions[1].User.Name != "systemAssignedIdentity" {
		t.Fatalf("projection=%s", data)
	}
	for _, sub := range result.Subscriptions {
		if sub.Environment != azureCloudName && sub.IsDefault {
			t.Fatalf("foreign cloud marked current: %+v", sub)
		}
	}
}

func TestAzureCLILegacyHintAndStructuredErrors(t *testing.T) {
	data := `{"subscriptions":[{"id":"` + productionID + `","name":"Production","tenantId":"` + tenantID + `","environmentName":"AzureCloud","isDefault":true}]}`
	for _, args := range [][]string{
		{listCommand},
		{currentFlag},
		{listCommand, outputFlag, jsonFormat},
		{currentFlag, outputFlag, jsonFormat},
		{missingQuery, outputFlag, jsonFormat},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			deps := commandDependencies(t, &testutil.FakeRunner{})
			deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(data)}}
			var stdout, stderr bytes.Buffer
			code := cmd.Run(args, deps, bytes.NewReader(nil), &stdout, &stderr)
			switch {
			case len(args) == 1:
				if code != 0 || !strings.Contains(stdout.String(), tenantID) || strings.Count(stderr.String(), "\n") != 1 ||
					!strings.Contains(stderr.String(), "az login") {
					t.Fatalf("legacy fallback: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
				}
			case args[0] == missingQuery:
				if code != 2 || stdout.Len() != 0 || !json.Valid(stderr.Bytes()) || strings.Contains(stderr.String(), "az login") {
					t.Fatalf("structured error polluted: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
				}
			default:
				if code != 0 || stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
					t.Fatalf("structured data polluted: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
				}
			}
		})
	}
}

// Only these three read-only Azure files may be opened, regardless of any
// obsolete cache sentinel in the underlying filesystem.
type azureReadFS struct {
	t     *testing.T
	files fstest.MapFS
}

func (f azureReadFS) Open(name string) (fs.File, error) {
	f.t.Helper()
	if name != profileFile && name != azureConfigFile && name != cloudsConfigFile {
		f.t.Fatalf("unexpected Azure configuration access: %s", name)
	}
	return f.files.Open(name)
}

func TestAzureCLICommandsNeverTouchLegacyCacheOrAzureFiles(t *testing.T) {
	for _, args := range [][]string{
		{listCommand},
		{listCommand, "--all", outputFlag, jsonFormat},
		{currentFlag},
		{developmentID},
		{"-"},
		{loginCommand},
		{loginCommand, allTenantsFlag},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			files := fstest.MapFS{
				profileFile:         &fstest.MapFile{Data: []byte(modernProfile)},
				azureConfigFile:     &fstest.MapFile{Data: []byte("[cloud]\nname=AzureCloud\n[core]\nlogin_experience_v2=on\n")},
				cloudsConfigFile:    &fstest.MapFile{Data: []byte("[AzureCloud]\nsubscription=" + productionID + "\n[CustomCloud]\nsubscription=44444444-4444-4444-4444-444444444444\n")},
				"azctxTenants.json": &fstest.MapFile{Data: []byte("malformed sentinel: never read or modified")},
			}
			before := make(map[string][]byte)
			for name, file := range files {
				before[name] = bytes.Clone(file.Data)
			}
			deps.FS = azureReadFS{t: t, files: files}
			var expected [][]string
			switch args[0] {
			case loginCommand:
				fake.Responses = []testutil.Response{{Result: azcli.Result{Stdout: `{"azure-cli":"2.61.0"}`}}, {}}
				expected = [][]string{{versionCommand, outputFlag, jsonFormat}, {loginCommand}}
				if len(args) > 1 {
					fake.Responses = append(fake.Responses,
						testutil.Response{Result: azcli.Result{Stdout: `[{"tenantId":"` + tenantID + `"}]`}},
						testutil.Response{})
					expected = append(expected, []string{accountCommand, tenantSubcommand, listCommand, outputFlag, jsonFormat},
						[]string{loginCommand, tenantFlag, tenantID})
				}
			case developmentID, "-":
				if args[0] == "-" {
					if err := state.Save(deps.ConfigDir, developmentID); err != nil {
						t.Fatal(err)
					}
				}
				fake.Responses = []testutil.Response{{}}
				expected = [][]string{{accountCommand, setCommand, subscriptionFlag, developmentID}}
			}
			var stdout, stderr bytes.Buffer
			if code := cmd.Run(args, deps, bytes.NewReader(nil), &stdout, &stderr); code != 0 {
				t.Fatalf("code=%d stderr=%q", code, stderr.String())
			}
			var got [][]string
			for _, call := range fake.Calls {
				got = append(got, call.Args)
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("az calls=%q want=%q", got, expected)
			}
			for name, original := range before {
				if !bytes.Equal(files[name].Data, original) {
					t.Fatalf("Azure file bytes changed: %s", name)
				}
			}
			if len(files) != len(before) {
				t.Fatal("Azure files created or deleted")
			}
		})
	}
}
