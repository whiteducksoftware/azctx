// Package config loads strict, versioned azctx configuration outside Azure files.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/storage"
)

type Config struct {
	Version     int               `yaml:"version"`
	Aliases     map[string]string `yaml:"aliases"`
	Pinned      []string          `yaml:"pinned"`
	Hidden      []string          `yaml:"hidden"`
	Protected   []string          `yaml:"protected"`
	Picker      string            `yaml:"picker"`
	UpdateCheck bool              `yaml:"updateCheck"`
}

const stringTag = "!!str"

func Default() Config {
	return Config{
		Version: 1, Aliases: map[string]string{}, Pinned: []string{},
		Hidden: []string{}, Protected: []string{}, Picker: "builtin", UpdateCheck: true,
	}
}

func Path(azureDir string) (string, error) {
	return storage.ConfigPath(azureDir, "config.yaml")
}

// Load defaults only when the file is absent. Invalid or unreadable files fail.
func Load(azureDir string) (Config, error) {
	path, err := Path(azureDir)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	result, err := parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return result, nil
}

// Save validates before atomically replacing the private file.
func Save(azureDir string, value Config) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := parse(data); err != nil {
		return err
	}
	path, err := Path(azureDir)
	if err != nil {
		return err
	}
	if err := storage.Write(path, azureDir, data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func parse(data []byte) (Config, error) {
	result := Default()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return Config{}, decodeError(err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode || doc.Content[0].Tag != "!!map" {
		return Config{}, errors.New("line 1: expected version-1 configuration mapping")
	}
	root := doc.Content[0]
	seen := map[string]bool{}
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Tag != stringTag || seen[key.Value] {
			return Config{}, lineError(key, "duplicate or invalid key %q", key.Value)
		}
		seen[key.Value] = true
		if err := applyField(&result, key, value); err != nil {
			return Config{}, err
		}
	}
	if !seen["version"] {
		return Config{}, lineError(root, "version: 1 is required")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, decodeError(err)
		}
		return Config{}, lineError(extra.Content[0], "only one YAML document is allowed")
	}
	return result, nil
}

func applyField(result *Config, key, value *yaml.Node) error {
	switch key.Value {
	case "version":
		if value.Tag != "!!int" || value.Value != "1" {
			return lineError(value, "version must be 1")
		}
	case "aliases":
		return readAliases(value, result.Aliases)
	case "pinned", "hidden", "protected":
		ids, err := readIDs(value)
		if err != nil {
			return err
		}
		destination := map[string]*[]string{
			"pinned": &result.Pinned, "hidden": &result.Hidden, "protected": &result.Protected,
		}[key.Value]
		*destination = ids
	case "picker":
		if value.Tag != stringTag || (value.Value != "builtin" && value.Value != "fzf") {
			return lineError(value, "picker must be builtin or fzf")
		}
		result.Picker = value.Value
	case "updateCheck":
		if value.Tag != "!!bool" || (value.Value != "true" && value.Value != "false") {
			return lineError(value, "updateCheck must be true or false")
		}
		result.UpdateCheck = value.Value == "true"
	default:
		return lineError(key, "unknown key %q", key.Value)
	}
	return nil
}

func decodeError(err error) error {
	if strings.Contains(err.Error(), "line ") {
		return err
	}
	return fmt.Errorf("line 1: %w", err)
}

func lineError(node *yaml.Node, format string, args ...any) error {
	return fmt.Errorf("line %d: %s", max(1, node.Line), fmt.Sprintf(format, args...))
}

func readAliases(node *yaml.Node, result map[string]string) error {
	if node.Kind != yaml.MappingNode || node.Tag != "!!map" {
		return lineError(node, "aliases must be a mapping of names to subscription IDs")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Tag != stringTag || strings.TrimSpace(key.Value) == "" || strings.Contains(key.Value, "/") {
			return lineError(key, "alias must be a nonempty name without '/'")
		}
		for alias := range result {
			if strings.EqualFold(alias, key.Value) {
				return lineError(key, "duplicate alias %q", key.Value)
			}
		}
		if value.Tag != stringTag || profile.ValidateGUID(value.Value) != nil {
			return lineError(value, "alias %q must reference a subscription GUID", key.Value)
		}
		result[key.Value] = value.Value
	}
	return nil
}

func readIDs(node *yaml.Node) ([]string, error) {
	if node.Kind != yaml.SequenceNode || node.Tag != "!!seq" {
		return nil, lineError(node, "expected a list of subscription or tenant GUIDs")
	}
	result := make([]string, 0, len(node.Content))
	for _, id := range node.Content {
		if id.Tag != stringTag || profile.ValidateGUID(id.Value) != nil {
			return nil, lineError(id, "expected a subscription or tenant GUID")
		}
		result = append(result, id.Value)
	}
	return result, nil
}

// Contains matches subscription or token tenant IDs case-insensitively.
func Contains(ids []string, subscription profile.Subscription) bool {
	for _, id := range ids {
		if strings.EqualFold(id, subscription.Id) || strings.EqualFold(id, subscription.Tenant) {
			return true
		}
	}
	return false
}
