// fakefzf records the data/argv boundary and selects a supplied ranked row.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}
	record, err := json.Marshal(struct {
		Args  []string `json:"args"`
		Input string   `json:"input"`
		Opts  string   `json:"opts"`
		File  string   `json:"file"`
	}{os.Args[1:], string(data), os.Getenv("FZF_DEFAULT_OPTS"), os.Getenv("FZF_DEFAULT_OPTS_FILE")})
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Getenv("FAKEFZF_RECORD"), record, 0o600); err != nil { //nolint:gosec // Test-owned recording destination.
		panic(err)
	}
	if os.Getenv("FAKEFZF_WAIT") == "1" {
		time.Sleep(time.Hour)
	}
	code, _ := strconv.Atoi(os.Getenv("FAKEFZF_EXIT"))
	if code != 0 {
		os.Exit(code)
	}
	if output, ok := os.LookupEnv("FAKEFZF_OUTPUT"); ok {
		fmt.Print(output)
		return
	}
	index, _ := strconv.Atoi(os.Getenv("FAKEFZF_INDEX"))
	rows := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if slices.Contains(os.Args[1:], "--expect=esc") {
		fmt.Println()
	}
	fmt.Println(rows[index])
}
