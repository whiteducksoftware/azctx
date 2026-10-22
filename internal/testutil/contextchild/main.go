// contextchild is an offline process-boundary fixture, not a shell.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

const (
	ignoreMode  = "ignore"
	azLoginMode = "az-login"
	pipeMode    = "az-pipe"
	mfaMode     = "az-mfa-one"
	oneMode     = "az-one"
	countMode   = "count"
)

func main() {
	mode := os.Getenv("AZCTX_TEST_CHILD_MODE")
	if len(os.Args) > 1 && os.Args[1] == "hold-stderr" {
		holdStderr()
		return
	}
	if (strings.HasPrefix(mode, "az-") || mode == ignoreMode) && len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Fprintln(os.Stdout, `{"azure-cli":"2.91.0"}`)
		return
	}
	switch mode {
	case pipeMode:
		holdPipe()
	case "exit":
		fmt.Fprintln(os.Stderr, "child stderr")
		os.Exit(37)
	case mfaMode:
		if len(os.Args) > 2 && os.Args[1] == "account" {
			fmt.Fprintln(os.Stdout, `[{"tenantId":"11111111-1111-4111-8111-111111111111"},{"tenantId":"22222222-2222-4222-8222-222222222222"}]`)
			return
		}
		record := os.Getenv("AZCTX_TEST_CHILD_RECORD")
		data, _ := os.ReadFile(record)                                         //nolint:gosec // Test-owned record.
		if err := os.WriteFile(record, append(data, 'x'), 0o600); err != nil { //nolint:gosec // Test-owned record.
			panic(err)
		}
		if len(data) == 0 {
			return
		}
		wait()
	case "trap", "wait", ignoreMode, azLoginMode, oneMode, countMode:
		wait()
	default:
		echo()
	}
}

func holdPipe() {
	path, err := os.Executable()
	if err != nil {
		panic(err)
	}
	descendant := exec.Command(path, "hold-stderr")
	descendant.Stderr = os.Stderr
	ready, err := descendant.StdoutPipe()
	if err != nil {
		panic(err)
	}
	if err := descendant.Start(); err != nil {
		_ = ready.Close()
		panic(err)
	}
	// Do not exit until the test owns an exit notification for the live holder.
	_, readErr := io.ReadFull(ready, make([]byte, 1))
	if err := errors.Join(readErr, ready.Close()); err != nil {
		_ = descendant.Process.Kill()
		_ = descendant.Wait()
		panic(err)
	}
	message := "login completed"
	if os.Getenv("AZCTX_TEST_CHILD_NO_NEWLINE") == "" {
		message += "\n"
	}
	fmt.Fprint(os.Stderr, message)
	if err := descendant.Process.Release(); err != nil {
		panic(err)
	}
	if os.Getenv("AZCTX_TEST_CHILD_EXIT") != "" {
		os.Exit(7)
	}
}

func holdStderr() {
	connection, err := net.Dial("tcp4", os.Getenv("AZCTX_TEST_PIPE_ADDRESS")) //nolint:gosec // Test-owned loopback lifetime gate, never a real network service.
	if err != nil {
		panic(err)
	}
	defer func() { _ = connection.Close() }()
	if _, err := fmt.Fprintln(connection, os.Getpid()); err != nil {
		panic(err)
	}
	reader := bufio.NewReader(connection)
	var response string
	if _, err := fmt.Fscanln(reader, &response); err != nil || response != "ack" {
		panic(fmt.Errorf("stderr owner handshake: response=%q error=%w", response, err))
	}
	fmt.Fprint(os.Stdout, "1")
	for {
		_, err := fmt.Fscanln(reader, &response)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil || response != "held" {
			panic(fmt.Errorf("stderr lifetime gate: response=%q error=%w", response, err))
		}
		if _, err := fmt.Fprintln(connection, "held"); err != nil {
			panic(err)
		}
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
	if mode != "wait" {
		signal.Notify(signals, os.Interrupt)
	}
	if os.Getenv("AZCTX_TEST_CHILD_READ_LINE") != "" {
		if err := os.WriteFile(os.Getenv("AZCTX_TEST_CHILD_STARTED"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil { //nolint:gosec // Test-owned pre-input readiness file.
			panic(err)
		}
		var line string
		if _, err := fmt.Fscanln(os.Stdin, &line); err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stderr, "child read "+line)
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
		if mode == countMode {
			select {
			case <-signals:
				fmt.Fprintln(os.Stderr, "child caught duplicate interrupt")
			case <-time.After(400 * time.Millisecond):
			}
		}
		if mode == oneMode || mode == mfaMode {
			os.Exit(1)
		}
		os.Exit(130)
	case <-time.After(time.Minute):
		os.Exit(99)
	}
}
