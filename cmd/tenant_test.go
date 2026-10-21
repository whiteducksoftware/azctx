package cmd_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const (
	secondTenantID      = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	customerTenant      = "Customer"
	otherSubscriptionID = "33333333-3333-3333-3333-333333333333"
)

func tenantDependencies(t *testing.T, fake *testutil.FakeRunner, subs []profile.Subscription) cmd.Dependencies {
	t.Helper()
	deps := commandDependencies(t, fake)
	data, err := json.Marshal(profile.Profile{Subscriptions: subs})
	if err != nil {
		t.Fatal(err)
	}
	deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: data}}
	return deps
}

func TestTenantPickerNarrowsByGUIDAndPreservesMutableIndices(t *testing.T) {
	subs := tenantSubscriptions()
	subs = append(subs, profile.Subscription{Id: otherSubscriptionID, Name: "Other", Tenant: secondTenantID, TenantName: customerTenant, Environment: azureCloudName})
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	deps := tenantDependencies(t, fake, subs)
	stage := 0
	deps.Select = func(rows []profile.Subscription) (int, error) {
		stage++
		if stage == 1 {
			if len(rows) != 2 || rows[0].Id != tenantID || rows[1].Id != secondTenantID {
				t.Fatalf("tenant rows=%+v", rows)
			}
			slices.Reverse(rows)
			return 0, nil
		}
		if len(rows) != 2 || rows[0].Tenant != secondTenantID || rows[1].Tenant != secondTenantID {
			t.Fatalf("subscription scope=%+v", rows)
		}
		return 1, nil
	}
	var out, messages bytes.Buffer
	if code := cmd.Run([]string{tenantSubcommand}, deps, strings.NewReader(""), &out, &messages); code != 0 || stage != 2 || len(fake.Calls) != 1 ||
		fake.Calls[0].Args[3] != subs[2].Id {
		t.Fatalf("code=%d stages=%d calls=%+v messages=%q", code, stage, fake.Calls, &messages)
	}
}

func TestTenantAmbiguousSubscriptionAndCancellationGates(t *testing.T) {
	for _, args := range [][]string{
		{tenantSubcommand, tenantID, noInputFlag},
		{tenantSubcommand, tenantID, outputFlag, jsonFormat},
		{tenantSubcommand, tenantID},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			picked := false
			deps.Select = func(rows []profile.Subscription) (int, error) {
				picked = true
				if len(rows) != 2 {
					t.Fatalf("rows=%+v", rows)
				}
				return -1, ui.ErrInterrupted
			}
			want := 2
			if len(args) == 2 {
				want = 130
			}
			var out, messages bytes.Buffer
			if code := cmd.Run(args, deps, strings.NewReader(""), &out, &messages); code != want || picked != (want == 130) || len(fake.Calls) != 0 {
				t.Fatalf("code=%d picked=%t calls=%+v stderr=%q", code, picked, fake.Calls, &messages)
			}
		})
	}
}

func TestTenantProtectedAndDisabledInspection(t *testing.T) {
	subs := tenantSubscriptions()
	subs[1].State = "Disabled"
	fake := &testutil.FakeRunner{}
	deps := tenantDependencies(t, fake, subs)
	writeSelectionConfig(t, "protected: ["+tenantID+"]\n")
	for _, query := range []string{tenantID, secondTenantID} {
		var out, messages bytes.Buffer
		want := 1
		if query == secondTenantID {
			want = 2
		}
		if code := cmd.Run([]string{tenantSubcommand, query, metadataAllFlag, noInputFlag}, deps, strings.NewReader(""), &out, &messages); code != want || len(fake.Calls) != 0 {
			t.Fatalf("query=%s code=%d calls=%+v messages=%q", query, code, fake.Calls, &messages)
		}
	}
	out := configCommandOutput(t, deps, tenantSubcommand, metadataAllFlag, noInputFlag, outputFlag, jsonFormat)
	if !strings.Contains(out, "Disabled") || !strings.Contains(out, developmentID) || len(fake.Calls) != 0 {
		t.Fatalf("disabled inspection=%s calls=%+v", out, fake.Calls)
	}
}

func tenantSubscriptions() []profile.Subscription {
	return []profile.Subscription{
		{Id: productionID, Name: productionName, Tenant: tenantID, TenantName: customerTenant, TenantDomain: "first.invalid", Environment: azureCloudName, IsDefault: true},
		{Id: developmentID, Name: developmentName, Tenant: secondTenantID, TenantName: customerTenant, TenantDomain: "second.invalid", Environment: azureCloudName},
	}
}

func TestTenantFirstIdentityAndDirectSelection(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  string
		code  int
	}{
		{tenantID, productionID, 0},
		{"SECOND.INVALID", developmentID, 0},
		{customerTenant, "", 2},
		{"not-a-tenant", "", 2},
	} {
		t.Run(tc.query, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := tenantDependencies(t, fake, tenantSubscriptions())
			deps.Select = func([]profile.Subscription) (int, error) {
				t.Fatal("unique/no-input tenant query must not open a picker")
				return -1, nil
			}
			var out, messages bytes.Buffer
			code := cmd.Run([]string{tenantSubcommand, tc.query, noInputFlag, outputFlag, jsonFormat}, deps, strings.NewReader(""), &out, &messages)
			if code != tc.code || out.Len() != 0 {
				t.Fatalf("code=%d out=%q messages=%q", code, out.String(), messages.String())
			}
			if tc.want == "" {
				if len(fake.Calls) != 0 {
					t.Fatalf("unexpected switch: %+v", fake.Calls)
				}
				if tc.query == customerTenant && (!strings.Contains(messages.String(), tenantID) || !strings.Contains(messages.String(), secondTenantID)) {
					t.Fatalf("lost ambiguous tenant identities: %s", &messages)
				}
			} else if len(fake.Calls) != 1 || !reflect.DeepEqual(fake.Calls[0].Args, []string{accountCommand, setCommand, subscriptionFlag, tc.want}) {
				t.Fatalf("calls = %+v", fake.Calls)
			}
		})
	}
}
