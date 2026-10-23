package cmd_test

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestContractTenantSubscriptionQuery(t *testing.T) {
	for _, query := range []string{tenantID + "/Development", "Acme/Development", tenantID + "/" + developmentID} {
		t.Run(query, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			files, ok := deps.FS.(fstest.MapFS)
			if !ok {
				t.Fatal("fixture is not a MapFS")
			}
			files[profileFile] = &fstest.MapFile{Data: []byte(strings.ReplaceAll(fixtureProfile, `"tenantDisplayName":"`+tenantID+`"`, `"tenantDisplayName":"Acme"`))}
			root := cmd.NewRootCommand(deps)
			root.SetArgs([]string{query})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(fake.Calls) != 1 || fake.Calls[0].Args[3] != developmentID {
				t.Fatalf("calls=%+v", fake.Calls)
			}
		})
	}
}

func TestContractCurrentPlainFormats(t *testing.T) {
	for _, tc := range []struct{ format, want string }{
		{idFormat, productionID + "\n"},
		{nameFormat, productionName + "\n"},
		{"tenant", tenantID + "\n"},
		{textFormat, "Active subscription: " + tenantID + "/" + productionName + " (" + tenantID + "/" + productionID + ")\n"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			root := cmd.NewRootCommand(commandDependencies(t, fake))
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{currentFlag, outputFlag, tc.format})
			if err := root.Execute(); err != nil || stdout.String() != tc.want || stderr.Len() != 0 || len(fake.Calls) != 0 {
				t.Fatalf("err=%v stdout=%q stderr=%q calls=%+v", err, stdout.String(), stderr.String(), fake.Calls)
			}
		})
	}
}

func TestContractListAllAndFormats(t *testing.T) {
	for _, format := range []string{"table", jsonFormat} {
		for _, all := range []bool{false, true} {
			fake := &testutil.FakeRunner{}
			root := cmd.NewRootCommand(commandDependencies(t, fake))
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			args := []string{listCommand, outputFlag, format}
			if all {
				args = append(args, "--all")
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil || strings.Contains(stdout.String(), "N/A(tenant level account)") != all ||
				stderr.Len() != 0 || len(fake.Calls) != 0 {
				t.Fatalf("args=%v err=%v stdout=%q stderr=%q calls=%+v", args, err, stdout.String(), stderr.String(), fake.Calls)
			}
		}
	}
}
