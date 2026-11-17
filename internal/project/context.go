// Package project reads untrusted project expectations without selecting or
// changing a context. Project files are never instructions to switch or log in.
package project

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

const (
	Filename   = ".azctx.yaml"
	versionKey = "version"
)

var ErrNotFound = errors.New("no .azctx.yaml found in the current directory or its ancestors")

type Context struct {
	Subscription string
	Tenant       string
}

func (value Context) Matches(subscription, tenant string) bool {
	return (value.Subscription == "" || strings.EqualFold(value.Subscription, subscription)) &&
		(value.Tenant == "" || strings.EqualFold(value.Tenant, tenant))
}

// Find stops at the nearest file, including when that file is malformed.
// An empty start uses the process working directory.
func Find(start string) (Context, string, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return Context{}, "", err
		}
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return Context{}, "", err
	}
	for {
		path := filepath.Join(dir, Filename)
		info, err := os.Lstat(path)
		if err == nil {
			value, err := load(path, info)
			if err != nil {
				return Context{}, path, fmt.Errorf("%s: %w", path, err)
			}
			return value, path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Context{}, path, fmt.Errorf("%s: %w", path, err)
		}
		if filepath.Dir(dir) == dir {
			return Context{}, "", ErrNotFound
		}
		dir = filepath.Dir(dir)
	}
}

func load(path string, info os.FileInfo) (Context, error) {
	if !info.Mode().IsRegular() {
		return Context{}, errors.New("project context must be a regular file, not a symlink")
	}
	const limit = 64 << 10
	if info.Size() > limit {
		return Context{}, errors.New("project context exceeds 65536 bytes")
	}
	file, err := os.Open(path)
	if err != nil {
		return Context{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return Context{}, err
	}
	if !os.SameFile(info, opened) {
		return Context{}, errors.New("project context changed before reading")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return Context{}, err
	}
	if len(data) > limit {
		return Context{}, errors.New("project context exceeds 65536 bytes")
	}
	return parse(data)
}

func parse(data []byte) (Context, error) {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return Context{}, fmt.Errorf("line 1: invalid project YAML: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode || document.Content[0].Tag != "!!map" {
		return Context{}, errors.New("line 1: expected version-1 project context mapping")
	}
	values, err := fields(document.Content[0])
	if err != nil {
		return Context{}, err
	}
	for _, key := range []string{versionKey, "subscription", "tenant"} {
		if _, ok := values[key]; !ok {
			if key == versionKey {
				return Context{}, errors.New("line 1: version: 1 is required")
			}
			return Context{}, fmt.Errorf("line 1: %s is required", key)
		}
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Context{}, fmt.Errorf("line 1: invalid trailing project YAML (only one YAML document is allowed): %w", err)
		}
		return Context{}, fmt.Errorf("line %d: only one YAML document is allowed", max(1, extra.Line))
	}
	return Context{Subscription: values["subscription"], Tenant: values["tenant"]}, nil
}

func fields(root *yaml.Node) (map[string]string, error) {
	values := map[string]string{}
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if _, seen := values[key.Value]; seen || key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, fmt.Errorf("line %d: duplicate or invalid key %q", key.Line, key.Value)
		}
		switch key.Value {
		case versionKey:
			if value.Kind != yaml.ScalarNode || value.Tag != "!!int" || value.Value != "1" {
				return nil, fmt.Errorf("line %d: version must be 1", value.Line)
			}
		case "subscription", "tenant":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || profile.ValidateGUID(value.Value) != nil {
				return nil, fmt.Errorf("line %d: %s must be a GUID", value.Line, key.Value)
			}
		default:
			return nil, fmt.Errorf("line %d: unknown key %q", key.Line, key.Value)
		}
		values[key.Value] = value.Value
	}
	return values, nil
}
