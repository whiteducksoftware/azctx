package azcli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Settings is a read-only view of Azure CLI INI settings. Section names are
// case-sensitive; option names and boolean values are case-insensitive.
type Settings struct {
	sections map[string]map[string]string
	lookup   func(string) (string, bool)
}

// ReadSettings reads config relative to the Azure directory. A missing file is
// empty configuration. Nil lookup uses the process environment.
func ReadSettings(files fs.FS, lookup func(string) (string, bool)) (Settings, error) {
	sections, err := readINI(files, "config")
	if err != nil {
		return Settings{}, err
	}
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return Settings{sections: sections, lookup: lookup}, nil
}

// Get applies AZURE_{SECTION}_{NAME} overrides before the file and fallback.
func (s Settings) Get(section, name, fallback string) string {
	if s.lookup != nil {
		if value, ok := s.lookup("AZURE_" + strings.ToUpper(section+"_"+name)); ok {
			return value
		}
	}
	if value, ok := iniValue(s.sections, section, name); ok {
		return value
	}
	return fallback
}

// Bool accepts the same eight boolean spellings as Azure CLI configuration.
func (s Settings) Bool(section, name string, fallback bool) (bool, error) {
	defaultValue := "false"
	if fallback {
		defaultValue = "true"
	}
	switch strings.ToLower(s.Get(section, name, defaultValue)) {
	case "1", "yes", "true", "on":
		return true, nil
	case "0", "no", "false", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid Azure CLI boolean setting %s.%s", section, name)
	}
}

func (s Settings) ActiveCloud() string {
	cloud := strings.TrimSpace(s.Get("cloud", "name", "AzureCloud"))
	if cloud == "" {
		return "AzureCloud"
	}
	return cloud
}

func iniValue(sections map[string]map[string]string, section, name string) (string, bool) {
	name = strings.ToLower(name)
	if value, ok := sections[section][name]; ok {
		return value, true
	}
	value, ok := sections["DEFAULT"][name]
	return value, ok
}

func readINI(files fs.FS, name string) (map[string]map[string]string, error) {
	data, err := fs.ReadFile(files, name)
	if errors.Is(err, fs.ErrNotExist) {
		return make(map[string]map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return parseINI(data, name)
}

func parseINI(data []byte, name string) (map[string]map[string]string, error) {
	sections := make(map[string]map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var section, previous string
	previousIndent := 0
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.ContainsAny(line[:1], "#;") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if previous != "" && indent > previousIndent {
			sections[section][previous] += "\n" + line
			continue
		}
		if strings.HasPrefix(line, "[") {
			// ConfigParser matches greedily through the last closing bracket.
			end := strings.LastIndexByte(line, ']')
			if end > 1 {
				section = line[1:end]
				if _, exists := sections[section]; !exists {
					sections[section] = make(map[string]string)
					previous = ""
					continue
				}
			}
		} else if options, ok := sections[section]; ok {
			if index := strings.IndexAny(line, "=:"); index > 0 {
				key := strings.ToLower(strings.TrimSpace(line[:index]))
				if _, exists := options[key]; !exists && key != "" {
					options[key] = strings.TrimSpace(line[index+1:])
					previous, previousIndent = key, indent
					continue
				}
			}
		}
		return nil, fmt.Errorf("malformed %s at line %d", name, lineNumber)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return sections, nil
}
