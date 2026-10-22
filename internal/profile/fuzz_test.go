package profile_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

func FuzzProfile(f *testing.F) {
	for _, seed := range []string{
		`{}`, `null`, `{"subscriptions":[]}`, `{"subscriptions":[{},{}]}`,
		"\xef\xbb\xbf" + `{"subscriptions":[{"id":"11111111-1111-1111-1111-111111111111","tenantId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","name":"Synthetic","isDefault":true}]}`,
		`{"subscriptions":[{"id":"&echo injected","tenantId":"%COMSPEC%","managedByTenants":[{"tenantId":"invalid"}]}]}`,
		`{"subscriptions":[{"id":"duplicate"},{"id":"duplicate"}],"future":{"key":true}}`,
		`{"subscriptions":[`, `{} {}`, `{"subscriptions":"invalid"}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		read := func(input []byte) (profile.Profile, error) {
			return profile.Read(fstest.MapFS{"azureProfile.json": &fstest.MapFile{Data: input}}, "azureProfile.json")
		}
		value, err := read(data)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		again, err := read(encoded)
		if err != nil || !reflect.DeepEqual(value, again) {
			t.Fatalf("profile lost records or metadata after round trip: %v", err)
		}
	})
}
