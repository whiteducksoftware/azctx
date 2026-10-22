package redact_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/redact"
)

func TestStreamingRedactionAcrossEverySplit(t *testing.T) {
	const redactedAccess = "access_token=[REDACTED]\n"
	jwt := strings.Join([]string{"eyJhbGciOiJub25lIn0", "eyJzdWIiOiJmaXh0dXJlIn0", "signature"}, ".")
	tests := []struct{ input, want string }{
		{"Use device code SYNTHETIC\nEnter code: ", "Use device code SYNTHETIC\nEnter code: "},
		{"/synthetic/" + strings.Repeat("segment/", 20) + ".azctx.yaml: line 2", "/synthetic/" + strings.Repeat("segment/", 20) + ".azctx.yaml: line 2"},
		{"access_token=synthetic-secret\n", redactedAccess},
		{`access_token=synthetic:secret=with/padding\suffix` + "\n", redactedAccess},
		{`{"refreshToken":"synthetic secret with spaces"}`, `{"refreshToken":"[REDACTED]"}`},
		{`{"password":"synthetic\"secret\\value"}`, `{"password":"[REDACTED]"}`},
		{"Authorization: " + "Bearer" + " opaque-secret\n", "Authorization: " + "Bearer" + " [REDACTED]\n"},
		{"jwt=" + jwt, "jwt=[REDACTED]"},
		{"token=" + strings.Repeat("x", 1<<20) + "\nready: ", "token=[REDACTED]\nready: "},
		{strings.Repeat("Z", 200) + "\n", "[REDACTED]\n"},
		{redactedAccess, redactedAccess},
	}
	for index, test := range tests {
		t.Run(string(rune('A'+index)), func(t *testing.T) { assertStreamed(t, test.input, test.want) })
	}
}

// syntheticKey is base64-shaped like a storage account key: standard alphabet
// with '/' and '+', mixed case and digits, 84 characters plus padding.
const redactedPasswordArray = `{"password": ["[REDACTED]"]}`

var syntheticKey = strings.Repeat("Sy7n+tH3tIcK/eY0", 5) + "Ab9z=="

func TestStreamingRedactsSlashBase64LabelsAndArrays(t *testing.T) {
	tests := map[string]struct{ input, want string }{
		"unlabelled base64 with slash": {"storage key mismatch: " + syntheticKey + "\nnext", "storage key mismatch: [REDACTED]\nnext"},
		"base64 with slash at start":   {"value /" + syntheticKey + " done", "value [REDACTED] done"},
		"base64 key in path context":   {"blob " + strings.Repeat("Q", 3) + "/" + syntheticKey + "\n", "blob [REDACTED]\n"},
		"storage connection string": {
			"DefaultEndpointsProtocol=https;AccountName=synthetic;AccountKey=c3lu/dGg+ZXRpYw==;EndpointSuffix=core.windows.net\n",
			"DefaultEndpointsProtocol=https;AccountName=synthetic;AccountKey=[REDACTED];EndpointSuffix=core.windows.net\n",
		},
		"service bus connection string": {
			"Endpoint=sb://synthetic.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=c3lu/dGg+ZXRpYw=\n",
			"Endpoint=sb://synthetic.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=[REDACTED]\n",
		},
		"quoted account key":       {`{"accountKey": "c3lu/dGg+ZXRpYw=="}`, `{"accountKey": "[REDACTED]"}`},
		"authorization basic":      {"Authorization: Basic c3ludGhldGljOnNlY3JldA==\n", "Authorization: Basic [REDACTED]\n"},
		"authorization basic json": {`{"authorization":"basic c3ludGhldGljOnNlY3JldA=="}`, `{"authorization":"basic [REDACTED]"}`},
		"password array":           {`{"password": ["synthetic-one"]}`, redactedPasswordArray},
		"password array elements":  {`{"password":["one", "two \"three\""],"user":"visible"}`, `{"password":["[REDACTED]", "[REDACTED]"],"user":"visible"}`},
		"password array multiline": {"{\n  \"password\": [\n    \"one\",\n    \"two\"\n  ],\n  \"user\": \"visible\"\n}", "{\n  \"password\": [\n    \"[REDACTED]\",\n    \"[REDACTED]\"\n  ],\n  \"user\": \"visible\"\n}"},
		"password nested array":    {`password: [["one"], ["two"]] user visible`, `password: [["[REDACTED]"], ["[REDACTED]"]] user visible`},
		"password unquoted array":  {"password=[one two] visible\n", "password=[REDACTED] [REDACTED] visible\n"},
		"password empty array":     {`{"password": [], "user": "visible"}`, `{"password": [], "user": "visible"}`},
		"yaml flow sequence":       {"password: [one, two]\nuser: visible", "password: [REDACTED], [REDACTED]\nuser: visible"},
		"redacted array element":   {redactedPasswordArray, redactedPasswordArray},
		"open bracket at flush":    {"password: [", "password: ["},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) { assertStreamed(t, test.input, test.want) })
	}
}

// A path whose 80-byte window is not key-shaped must release only its oldest
// segment: a key starting later in the window stays hidden as a whole, at the
// cost of hiding the path segments that share its first key-shaped window.
func TestStreamingRedactsWholeSlashBase64KeyAfterPathPrefix(t *testing.T) {
	exact := map[string]struct{ input, want string }{
		"short segments": {"/a/b/c/d/e/f/" + syntheticKey + "\nnext", "/a/b/[REDACTED]\nnext"},
		"long path": {
			"/Users/Alice/work/azure/rg1/storage/accounts/myacct/keys/" + syntheticKey + "\nnext",
			"/Users/Alice/work/[REDACTED]\nnext",
		},
		"repeated segments": {strings.Repeat("abcdefg/", 8) + syntheticKey + " done", "abcdefg/[REDACTED] done"},
	}
	for name, test := range exact {
		t.Run(name, func(t *testing.T) { assertStreamed(t, test.input, test.want) })
	}
	prefixes := []string{
		"/", "x/", "/a/b/c/d/e/f/", strings.Repeat("a/", 40), strings.Repeat("a/", 41),
		strings.Repeat("abcdefg/", 8), strings.Repeat("Seg9/", 30), "/Ab1/Cd2/Ef3/Gh4/Ij5/Kl6/Mn7/Op8/Qr9/",
		"/Users/Alice/work/azure/rg1/storage/accounts/myacct/keys/",
		"C:/Users/Alice/storage/accounts/myacct/keys/", "path/" + strings.Repeat("q", 75) + "/",
	}
	fragments := keyFragments(t, prefixes)
	for _, prefix := range prefixes {
		for _, suffix := range []string{"\nnext", " done", ";next=1", "/tail\n"} {
			input := "key " + prefix + syntheticKey + suffix
			t.Run(prefix+suffix, func(t *testing.T) {
				eachStreamed(t, input, func(how, got string) {
					if !strings.HasPrefix(got, "key ") || !strings.HasSuffix(got, "[REDACTED]"+strings.TrimPrefix(suffix, "/tail")) {
						t.Fatalf("%s: got %q", how, got)
					}
					for _, fragment := range fragments {
						if strings.Contains(got, fragment) {
							t.Fatalf("%s: key fragment %q leaked in %q", how, fragment, got)
						}
					}
				})
			})
		}
	}
}

// keyFragments lists every 4-byte run of syntheticKey's base64 body; none
// appears in the prefixes, so any of them in output is leaked key material.
func keyFragments(t *testing.T, prefixes []string) []string {
	t.Helper()
	body := strings.TrimRight(syntheticKey, "=")
	var fragments []string
	for index := 0; index+4 <= len(body); index++ {
		fragment := body[index : index+4]
		for _, prefix := range prefixes {
			if strings.Contains("key "+prefix, fragment) {
				t.Fatalf("prefix %q contains key fragment %q", prefix, fragment)
			}
		}
		fragments = append(fragments, fragment)
	}
	return fragments
}

func TestStreamingKeepsOrdinaryDiagnosticsReadable(t *testing.T) {
	inputs := []string{
		"The Basic tier does not support this feature; choose sku Basic or Standard.\n",
		"Authorization failed. Basic troubleshooting steps are documented.\n",
		"Authorization:\nBasic tier selected\n",
		"(AuthorizationFailed) The client does not have authorization to perform action 'Microsoft.Storage/storageAccounts/listKeys/action' over scope '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/synthetic'.\n",
		"/Users/Alice2/Go/src/Github/Org1/Repo2/Pkg3/Sub4/Dir5/More6/Deeper7/Deepest8/Leaf9/Final0 not found\n",
		"see https://learn.microsoft.com/cli/azure/storage/account/keys for AccountName details [1/2]\n",
		strings.Repeat("abcdefghij/", 20) + "end\n",
		strings.Repeat("Dir1/", 40) + "file.txt\n",
		"/home/Runner2/work/" + strings.Repeat("Pkg3/Sub4/", 12) + "main.go:12: unused\n",
		"Use device code SYNTHETIC [array] and accountKeys list\n",
	}
	for index, input := range inputs {
		t.Run(string(rune('A'+index)), func(t *testing.T) { assertStreamed(t, input, input) })
	}
}

func TestStreamingRedactionLargeAdversarialInputs(t *testing.T) {
	tests := map[string]struct{ input, want string }{
		"slashed base64 secret": {"key " + strings.Repeat("Sy7n+tH3tIcK/eY0", 1<<10) + "\nready: ", "key [REDACTED]\nready: "},
		"slashed readable path": {strings.Repeat("segment/", 1<<10) + "\n", strings.Repeat("segment/", 1<<10) + "\n"},
		"password array":        {"password: [" + strings.Repeat(`"x/y", `, 1<<10) + `"z"] ok`, "password: [" + strings.Repeat(`"[REDACTED]", `, 1<<10) + `"[REDACTED]"] ok`},
		"nested array":          {"password: " + strings.Repeat("[", 1<<10) + "secret" + strings.Repeat("]", 1<<10) + " ok", "password: " + strings.Repeat("[", 1<<10) + "[REDACTED]" + strings.Repeat("]", 1<<10) + " ok"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) { assertStreamed(t, test.input, test.want) })
	}
}

// assertStreamed checks every two-part split (sampled for large inputs),
// varied multi-part splits and byte-at-a-time writes for short inputs, so
// values crossing writes are covered.
func assertStreamed(t *testing.T, input, want string) {
	t.Helper()
	eachStreamed(t, input, func(how, got string) {
		if got != want {
			t.Fatalf("%s: got %q, want %q", how, got, want)
		}
	})
}

func eachStreamed(t *testing.T, input string, check func(how, got string)) {
	t.Helper()
	step := 1
	if len(input) > 4096 {
		step = len(input) / 64
	}
	for split := 0; split <= len(input); split += step {
		check(fmt.Sprintf("split %d", split), streamParts(t, input[:split], input[split:]))
	}
	if len(input) > 4096 {
		return
	}
	parts := make([]string, len(input))
	for index := range input {
		parts[index] = input[index : index+1]
	}
	check("byte writes", streamParts(t, parts...))
	for round := range 16 {
		parts = parts[:0]
		for rest := input; rest != ""; {
			size := min(len(rest), 1+(round*7+len(parts)*13)%24)
			parts, rest = append(parts, rest[:size]), rest[size:]
		}
		check(fmt.Sprintf("varied writes %d", round), streamParts(t, parts...))
	}
}

func streamParts(t *testing.T, parts ...string) string {
	t.Helper()
	var output bytes.Buffer
	writer := redact.NewWriter(&output)
	for _, part := range parts {
		if n, err := writer.Write([]byte(part)); err != nil || n != len(part) {
			t.Fatalf("write: n=%d error=%v", n, err)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	return output.String()
}

func TestStreamingPromptAndWriterFailures(t *testing.T) {
	var output bytes.Buffer
	writer := redact.NewWriter(&output)
	for _, value := range []byte("Enter code: ") {
		if _, err := writer.Write([]byte{value}); err != nil {
			t.Fatal(err)
		}
	}
	if output.String() != "Enter code: " {
		t.Fatalf("prompt was not visible before input: %q", output.String())
	}
	failure := errors.New("writer failed")
	writer = redact.NewWriter(failedWriter{failure})
	if _, err := writer.Write([]byte("message\n")); !errors.Is(err, failure) {
		t.Fatalf("write failure was lost: %v", err)
	}
}

type failedWriter struct{ err error }

func (w failedWriter) Write([]byte) (int, error) { return 0, w.err }
