// fakeaz replays an ordered JSON fixture array and records calls as JSON lines.
// FAKEAZ_FIXTURES and FAKEAZ_RECORD select the files; FAKEAZ_ENV_KEYS selects
// the comma-separated environment names to record (never the full environment).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

type fixture struct {
	Args     []string `json:"args"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	ExitCode int      `json:"exitCode"`
}

type call struct {
	Args []string `json:"args"`
	Env  []string `json:"env"`
}

func main() {
	code, err := replay()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func replay() (int, error) {
	data, err := os.ReadFile(os.Getenv("FAKEAZ_FIXTURES")) //nolint:gosec // Test-only executable: the test owns the fixture path.
	if err != nil {
		return 0, err
	}
	var fixtures []fixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		return 0, err
	}
	record := os.Getenv("FAKEAZ_RECORD")
	previous, err := os.ReadFile(record) //nolint:gosec // Test-only executable: the test owns the recording path.
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	index := strings.Count(string(previous), "\n")
	invocation := call{Args: os.Args[1:]}
	for _, key := range strings.Split(os.Getenv("FAKEAZ_ENV_KEYS"), ",") {
		if key != "" {
			invocation.Env = append(invocation.Env, key+"="+os.Getenv(key))
		}
	}
	line, err := json.Marshal(invocation)
	if err != nil {
		return 0, err
	}
	file, err := os.OpenFile(record, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // Test-only executable: the test owns the recording path.
	if err != nil {
		return 0, err
	}
	_, writeErr := file.Write(append(line, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return 0, writeErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if index >= len(fixtures) || !slices.Equal(fixtures[index].Args, invocation.Args) {
		return 0, fmt.Errorf("unexpected fake az invocation %d: %q", index, invocation.Args)
	}
	next := fixtures[index]
	fmt.Fprint(os.Stdout, next.Stdout)
	fmt.Fprint(os.Stderr, next.Stderr)
	return next.ExitCode, nil
}
