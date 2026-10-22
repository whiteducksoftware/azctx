package redact_test

import (
	"bytes"
	"errors"
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

func TestStreamingKeepsOrdinaryDiagnosticsReadable(t *testing.T) {
	inputs := []string{
		"The Basic tier does not support this feature; choose sku Basic or Standard.\n",
		"Authorization failed. Basic troubleshooting steps are documented.\n",
		"Authorization:\nBasic tier selected\n",
		"(AuthorizationFailed) The client does not have authorization to perform action 'Microsoft.Storage/storageAccounts/listKeys/action' over scope '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/synthetic'.\n",
		"/Users/Alice2/Go/src/Github/Org1/Repo2/Pkg3/Sub4/Dir5/More6/Deeper7/Deepest8/Leaf9/Final0 not found\n",
		"see https://learn.microsoft.com/cli/azure/storage/account/keys for AccountName details [1/2]\n",
		strings.Repeat("abcdefghij/", 20) + "end\n",
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

// assertStreamed checks every two-part split (sampled for large inputs) and
// byte-at-a-time writes for short inputs, so values crossing writes are covered.
func assertStreamed(t *testing.T, input, want string) {
	t.Helper()
	step := max(1, len(input)/64)
	for split := 0; split <= len(input); split += step {
		if got := streamParts(t, input[:split], input[split:]); got != want {
			t.Fatalf("split %d: got %q, want %q", split, got, want)
		}
	}
	if len(input) <= 4096 {
		parts := make([]string, len(input))
		for index := range input {
			parts[index] = input[index : index+1]
		}
		if got := streamParts(t, parts...); got != want {
			t.Fatalf("byte writes: got %q, want %q", got, want)
		}
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
