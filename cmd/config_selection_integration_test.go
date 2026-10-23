package cmd_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/state"
)

func TestBinaryConfigAliasProtectedSelection(t *testing.T) {
	binary, azure, record := fakeBinary(t, `[{"args":["account","set","--subscription","`+developmentID+`"]}]`)
	path := writeSelectionConfig(t, "hidden: ["+developmentID+"]\nprotected: ["+developmentID+"]\nupdateCheck: false\n")
	for _, tc := range []struct {
		args []string
		code int
		text string
	}{
		{[]string{"alias", "set", selectionAlias, developmentID}, 0, ""},
		{[]string{"config", "path"}, 0, path},
		{[]string{"__complete", "Prod"}, 0, productionName},
		{[]string{selectionAlias}, 2, "no azure subscription"},
		{[]string{metadataAllFlag, selectionAlias}, 1, confirmFlag},
		{[]string{metadataAllFlag, confirmFlag, selectionAlias}, 0, ""},
	} {
		code, out, messages := runContractBinary(t, binary, tc.args, "")
		if code != tc.code || !strings.Contains(out+messages, tc.text) {
			t.Fatalf("%v: exit=%d, stdout=%q, stderr=%q", tc.args, code, out, messages)
		}
	}
	calls, err := os.ReadFile(record)
	if err != nil || strings.Count(string(calls), "\n") != 1 {
		t.Fatalf("unexpected Azure calls = %s, %v", calls, err)
	}
	recent, err := state.Recent(azure)
	if err != nil || !reflect.DeepEqual(recent, []string{developmentID}) {
		t.Fatalf("persistent recent state = %v, %v", recent, err)
	}
	after, err := os.ReadFile(filepath.Join(azure, profileFile))
	if err != nil || string(after) != fixtureProfile {
		t.Fatalf("Azure profile was changed: %q, %v", after, err)
	}
}
