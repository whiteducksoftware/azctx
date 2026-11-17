package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestBinaryAmbiguousNonTTYJSON(t *testing.T) {
	binary, config, record := fakeBinary(t, `[]`)
	writeFixture(t, filepath.Join(config, profileFile), `{"subscriptions":[
		{"id":"`+productionID+`","name":"Production","tenantId":"`+tenantID+`","isDefault":true},
		{"id":"`+developmentID+`","name":"Production","tenantId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, binary, verboseFlag, outputFlag, jsonFormat, productionName)
	var stdout, stderr bytes.Buffer
	process.Stdout, process.Stderr = &stdout, &stderr
	// Nil stdin is the portable equivalent of /dev/null.
	err := process.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 || stdout.Len() != 0 {
		t.Fatalf("exit=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	assertContractGolden(t, "ambiguous", stderr.String())
	if calls, err := os.ReadFile(record); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected subprocess calls=%s error=%v", calls, err)
	}
}
