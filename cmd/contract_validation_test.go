package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestContractInvalidFormatsFailBeforeDependencies(t *testing.T) {
	for _, args := range [][]string{
		{currentFlag, outputFlag, "table"},
		{listCommand, outputFlag, nameFormat},
		{productionName, outputFlag, idFormat},
		{outputFlag, textFormat},
		{versionCommand, outputFlag, "yaml"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("PATH", "")
			t.Setenv("AZURE_CONFIG_DIR", testutil.WorkDir(t))
			var stdout, stderr bytes.Buffer
			code := cmd.Run(args, cmd.Dependencies{}, bytes.NewReader(nil), &stdout, &stderr)
			if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "unsupported") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestContractNoInputEnvironmentCannotBeOverridden(t *testing.T) {
	for _, value := range []string{"1", "0"} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
		deps := commandDependencies(t, fake)
		t.Setenv("AZCTX_NO_INPUT", value)
		var stdout, stderr bytes.Buffer
		code := cmd.Run([]string{"--no-input=false"}, deps, bytes.NewReader(nil), &stdout, &stderr)
		messages := ""
		if value == "0" {
			messages = switchFeedback(developmentName, developmentID, tenantID, tenantID)
		}
		if code != 0 || stderr.String() != messages || value == "1" && (len(fake.Calls) != 0 || !strings.Contains(stdout.String(), productionName)) ||
			value == "0" && (len(fake.Calls) != 1 || stdout.Len() != 0) {
			t.Fatalf("value=%q code=%d stdout=%q stderr=%q calls=%+v", value, code, stdout.String(), stderr.String(), fake.Calls)
		}
	}
}
