package project

import (
	"fmt"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

func FuzzProject(f *testing.F) {
	const valid = "version: 1\nsubscription: 11111111-1111-1111-1111-111111111111\ntenant: aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa\n"
	for _, seed := range []string{
		valid, valid + "---\n", valid + "unknown: true\n", valid + "tenant: bad\n",
		"version: 1\nsubscription: &id 11111111-1111-1111-1111-111111111111\ntenant: *id\n",
		"version: 1\nsubscription: '%COMSPEC%'\ntenant: '&echo injected'\n",
		"version: 1\n", "version: \"1\"\n", "[]", "",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			return
		}
		value, err := parse(data)
		if err != nil {
			return
		}
		if profile.ValidateGUID(value.Subscription) != nil || profile.ValidateGUID(value.Tenant) != nil {
			t.Fatal("project accepted non-GUID identities")
		}
		encoded := fmt.Sprintf("version: 1\nsubscription: %s\ntenant: %s\n", value.Subscription, value.Tenant)
		again, err := parse([]byte(encoded))
		if err != nil || value != again || !again.Matches(value.Subscription, value.Tenant) {
			t.Fatalf("project changed after canonical round trip: %v", err)
		}
	})
}
