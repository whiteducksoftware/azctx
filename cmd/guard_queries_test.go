package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestGuardQueriesNeverChooseAnAmbiguousIdentity(t *testing.T) {
	for _, tc := range []struct {
		flag, query string
		code        int
	}{
		{tenantFlag, "first.invalid", 0},
		{tenantFlag, "second.invalid", 4},
		{tenantFlag, customerTenant, 2},
		{subscriptionFlag, customerTenant, 2},
		{subscriptionFlag, selectionAlias, 4},
		{subscriptionFlag, missingQuery, 2},
	} {
		t.Run(tc.flag+"/"+tc.query, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := tenantDependencies(t, fake, tenantSubscriptions())
			deps.Select = func([]profile.Subscription) (int, error) {
				t.Fatal("guard must not choose an ambiguous identity")
				return -1, nil
			}
			writeSelectionConfig(t, "aliases:\n  dev: "+developmentID+"\n")
			var out, messages bytes.Buffer
			code := cmd.Run([]string{guardCommand, tc.flag, tc.query, outputFlag, jsonFormat}, deps, strings.NewReader(""), &out, &messages)
			if code != tc.code || out.Len() != 0 || len(fake.Calls) != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, &out, &messages, fake.Calls)
			}
			if tc.query == customerTenant {
				var result struct {
					Code       string                   `json:"code"`
					Candidates []cmd.SubscriptionOutput `json:"candidates"`
				}
				if err := json.Unmarshal(messages.Bytes(), &result); err != nil || result.Code != cmd.CodeAmbiguousMatch || len(result.Candidates) != 2 {
					t.Fatalf("ambiguous identities lost: %q, %v", &messages, err)
				}
			}
		})
	}
}
