package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	rawDisplayName       = "Pro\tduction\n界\x1b[31m\a\u0085"
	safeDisplayName      = "Pro duction 界 [31m  "
	rawTenantLabel       = "Dir\rName\x1b[0m"
	safeTenantLabel      = "Dir Name [0m"
	rawDomainLabel       = "domain\tname.invalid"
	safeDomainLabel      = "domain name.invalid"
	protectedDisplayCase = "protected display"
)

func displaySubscription() profile.Subscription {
	return profile.Subscription{
		Id: productionID, Name: rawDisplayName, Tenant: tenantID,
		TenantName: rawTenantLabel, TenantDomain: rawDomainLabel,
		Environment: azureCloudName, IsDefault: true,
	}
}

func TestHumanSubscriptionDisplaysFlattenControls(t *testing.T) {
	for _, tc := range []struct {
		name, out, message string
		args               []string
		status, calls      int
	}{
		{name: "list", args: []string{listCommand}, out: safeDisplayName},
		{name: "bare table", args: []string{noInputFlag}, out: safeDisplayName},
		{name: "current", args: []string{currentFlag}, out: "Active subscription: " + safeTenantLabel + "/" + safeDisplayName},
		{name: "domain fallback", args: []string{currentFlag}, out: "Active subscription: " + safeDomainLabel + "/" + safeDisplayName},
		{
			name: protectedDisplayCase, args: []string{rawDisplayName}, status: 1,
			message: "Switch to protected subscription " + safeTenantLabel + "/" + safeDisplayName + " (" + productionID + ")? [y/N] " + protectedDeclined,
		},
		{name: "ambiguity", args: []string{noInputFlag, rawTenantLabel}, status: 2, message: "  " + tenantID + "/" + safeDisplayName + " (" + productionID + ")"},
		{
			name: "verbose switch", args: []string{verboseFlag, rawDisplayName}, calls: 1,
			message: "Setting active subscription to " + safeTenantLabel + "/" + safeDisplayName,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := displaySubscription()
			if tc.name == "domain fallback" {
				sub.TenantName = ""
			}
			subs := []profile.Subscription{sub}
			if tc.name == "ambiguity" {
				other := sub
				other.Id, other.Name, other.IsDefault = developmentID, developmentName, false
				subs = append(subs, other)
			}
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := tenantDependencies(t, fake, subs)
			if tc.name == protectedDisplayCase {
				writeSelectionConfig(t, "protected: ["+productionID+"]\n")
			}
			var out, messages bytes.Buffer
			status := cmd.Run(tc.args, deps, strings.NewReader(selectionDecline), &out, &messages)
			if status != tc.status || len(fake.Calls) != tc.calls ||
				!strings.Contains(out.String(), tc.out) || !strings.Contains(messages.String(), tc.message) ||
				strings.ContainsAny(out.String()+messages.String(), "\t\r\x1b\a\u0085") {
				t.Fatalf("status=%d calls=%+v stdout=%q stderr=%q", status, fake.Calls, &out, &messages)
			}
			if tc.name == protectedDisplayCase {
				assertNoSwitchRecorded(t, deps)
			}
		})
	}
}

func TestListTableFlattensAllProfileColumns(t *testing.T) {
	sub := displaySubscription()
	sub.Id, sub.TenantName = "id\tvalue", "tenant\nvalue"
	sub.State, sub.User.Type, sub.Environment = "state\rvalue", "account\x1bvalue", "cloud\u0085value"
	deps := tenantDependencies(t, &testutil.FakeRunner{}, []profile.Subscription{sub})
	got := configCommandOutput(t, deps, listCommand, metadataAllFlag)
	for _, value := range []string{safeDisplayName, "id value", "tenant value", "state value", "account value", "cloud value"} {
		if !strings.Contains(got, value) {
			t.Errorf("table missing safe column %q: %q", value, got)
		}
	}
	if strings.Count(got, "\n") != 2 || strings.ContainsAny(got, "\t\r\x1b\a\u0085") {
		t.Fatalf("profile escaped a table cell: %q", got)
	}
}

func TestSubscriptionDataOutputsPreserveDisplayControls(t *testing.T) {
	deps := tenantDependencies(t, &testutil.FakeRunner{}, []profile.Subscription{displaySubscription()})
	for _, args := range [][]string{{listCommand, outputFlag, jsonFormat}, {currentFlag, outputFlag, jsonFormat}} {
		got := configCommandOutput(t, deps, args...)
		var envelope struct {
			Subscription  cmd.SubscriptionOutput   `json:"subscription"`
			Subscriptions []cmd.SubscriptionOutput `json:"subscriptions"`
		}
		if err := json.Unmarshal([]byte(got), &envelope); err != nil {
			t.Fatal(err)
		}
		sub := envelope.Subscription
		if len(envelope.Subscriptions) != 0 {
			sub = envelope.Subscriptions[0]
		}
		if sub.ID != productionID || sub.TenantID != tenantID || sub.Name != rawDisplayName || sub.TenantName != rawTenantLabel || sub.TenantDomain != rawDomainLabel {
			t.Fatalf("JSON values changed: %+v", sub)
		}
	}
	if got := configCommandOutput(t, deps, currentFlag, outputFlag, "name"); got != rawDisplayName+"\n" {
		t.Fatalf("plain-field name changed: %q", got)
	}
	want := "export ARM_SUBSCRIPTION_ID='" + productionID + "'\nexport ARM_TENANT_ID='" + tenantID +
		"'\nexport AZURE_SUBSCRIPTION_ID='" + productionID + "'\nexport AZURE_TENANT_ID='" + tenantID + "'\n"
	if got := configCommandOutput(t, deps, "env", rawDisplayName); got != want {
		t.Fatalf("raw-name matching or environment changed: %q", got)
	}
}

func TestHumanErrorDisplayFlattensControlsButJSONPreservesValues(t *testing.T) {
	failure := &cmd.ExitError{
		Code: cmd.CodeAmbiguousMatch, Status: cmd.ExitNoMatch, Message: rawTenantLabel,
		Candidates: []cmd.SubscriptionOutput{{ID: productionID, TenantID: tenantID, Name: rawDisplayName}},
	}
	var out bytes.Buffer
	if code := cmd.WriteError(&out, "", failure); code != 2 ||
		out.String() != "Error: "+safeTenantLabel+"\n  "+tenantID+"/"+safeDisplayName+" ("+productionID+")\n" {
		t.Fatalf("human error code=%d output=%q", code, &out)
	}
	out.Reset()
	if code := cmd.WriteError(&out, jsonFormat, failure); code != 2 {
		t.Fatalf("JSON error code=%d", code)
	}
	var got struct {
		Message    string                   `json:"message"`
		Candidates []cmd.SubscriptionOutput `json:"candidates"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Message != rawTenantLabel || len(got.Candidates) != 1 || got.Candidates[0].Name != rawDisplayName {
		t.Fatalf("JSON error changed values: %s err=%v", &out, err)
	}
	out.Reset()
	if code := cmd.WriteError(&out, "", errors.New(rawDomainLabel)); code != 1 || out.String() != "Error: "+safeDomainLabel+"\n" {
		t.Fatalf("ordinary error code=%d output=%q", code, &out)
	}
}
