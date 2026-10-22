package config

import (
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

func FuzzConfig(f *testing.F) {
	for _, seed := range []string{
		"version: 1\n", "version: 1\naliases:\n  prod: 11111111-1111-1111-1111-111111111111\n",
		"version: 1\npinned: [11111111-1111-1111-1111-111111111111]\nhidden: [aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa]\nprotected: []\npicker: fzf\nupdateCheck: false\n",
		"version: 1\naliases:\n  bad: '&echo injected'\n",
		"version: 1\nversion: 1\n", "version: 1\n---\n", "version: 1\n---\nversion: 1\n",
		"version: &v 1\npicker: *v\n", "version: 1\nunknown: true\n", "[]", "",
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
		for _, id := range append(append(append([]string{}, value.Pinned...), value.Hidden...), value.Protected...) {
			if profile.ValidateGUID(id) != nil {
				t.Fatal("config accepted a non-GUID identity")
			}
		}
		for _, id := range value.Aliases {
			if profile.ValidateGUID(id) != nil {
				t.Fatal("config accepted a non-GUID alias target")
			}
		}
		encoded, err := yaml.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		again, err := parse(encoded)
		if err != nil || !reflect.DeepEqual(value, again) {
			t.Fatalf("config changed after canonical round trip: %v", err)
		}
	})
}
