// contextchild is an offline process-boundary fixture, not a shell.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"time"
)

const ignoreMode = "ignore"

func main() {
	switch os.Getenv("AZCTX_TEST_CHILD_MODE") {
	case "exit":
		fmt.Fprintln(os.Stderr, "child stderr")
		os.Exit(37)
	case "trap", "wait", ignoreMode:
		wait()
	default:
		echo()
	}
}

func echo() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}
	result := struct {
		Args  []string          `json:"args"`
		Env   map[string]string `json:"env"`
		Stdin string            `json:"stdin"`
	}{Args: os.Args[1:], Env: map[string]string{}, Stdin: string(input)}
	for _, key := range []string{"ARM_SUBSCRIPTION_ID", "ARM_TENANT_ID", "AZURE_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZCTX_TEST_INHERITED"} {
		result.Env[key] = os.Getenv(key)
	}
	fmt.Fprintln(os.Stderr, "child stderr")
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		panic(err)
	}
}

func wait() {
	signals := make(chan os.Signal, 1)
	mode := os.Getenv("AZCTX_TEST_CHILD_MODE")
	if mode == "trap" || mode == ignoreMode {
		signal.Notify(signals, os.Interrupt)
	}
	if err := os.WriteFile(os.Getenv("AZCTX_TEST_CHILD_READY"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil { //nolint:gosec // Test-only helper: the test owns the readiness path.
		panic(err)
	}
	if mode == ignoreMode {
		signals = nil
	}
	select {
	case <-signals:
		fmt.Fprintln(os.Stderr, "child caught interrupt")
		os.Exit(130)
	case <-time.After(time.Minute):
		os.Exit(99)
	}
}
