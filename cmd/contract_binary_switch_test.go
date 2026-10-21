package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBinaryContractCommandNamesAndDelimiter(t *testing.T) {
	names := []string{listCommand, loginCommand, versionCommand, envCommand, execSubcommand, guardCommand, "help", "completion", verboseFlag, "--output=json"}
	fixture := `{"args":["account","set","--subscription","` + productionID + `"]}`
	binary, config, record := fakeBinary(t, "["+strings.TrimSuffix(strings.Repeat(fixture+",", len(names)), ",")+"]")
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			data := strings.ReplaceAll(fixtureProfile, productionName, name)
			writeFixture(t, filepath.Join(config, profileFile), data)
			code, stdout, stderr := runContractBinary(t, binary, []string{argumentDelimiter, name}, "")
			if code != 0 || stdout != "" || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			after, err := os.ReadFile(filepath.Join(config, profileFile))
			if err != nil || string(after) != data {
				t.Fatalf("profile changed: %q error=%v", after, err)
			}
		})
	}
	if calls, err := os.ReadFile(record); err != nil || strings.Count(string(calls), "\n") != len(names) {
		t.Fatalf("calls=%s error=%v", calls, err)
	}
}

func TestBinaryContractAZFailureJSONIsOneObject(t *testing.T) {
	binary, _, record := fakeBinary(t, `[{"args":["account","set","--subscription","`+developmentID+`"],"stderr":"denied\n","exitCode":7}]`)
	code, stdout, stderr := runContractBinary(t, binary, []string{verboseFlag, outputFlag, jsonFormat, developmentName}, "")
	if code != 1 || stdout != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertContractGolden(t, "az-error", stderr)
	if calls, err := os.ReadFile(record); err != nil || strings.Count(string(calls), "\n") != 1 {
		t.Fatalf("calls=%s error=%v", calls, err)
	}
}
