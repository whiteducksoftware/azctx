package cmd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/whiteducksoftware/azctx/cmd"
)

const (
	agentPlugin          = "../plugins/azctx/"
	agentName            = "azctx"
	agentContextMismatch = "exit 4"
	agentCLIRelease      = "2026.11"
	agentVersion         = agentCLIRelease + ".0"
)

func TestAgentSkillAndManifests(t *testing.T) {
	skill := readAgentFile(t, "../skills/azctx/SKILL.md")
	if !bytes.Equal(skill, readAgentFile(t, agentPlugin+"skills/azctx/SKILL.md")) {
		t.Fatal("standalone and cached plugin skills must remain identical")
	}
	parts := strings.SplitN(string(skill), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatal("skill needs YAML frontmatter")
	}
	var metadata struct {
		Name          string            `yaml:"name"`
		Description   string            `yaml:"description"`
		Compatibility string            `yaml:"compatibility"`
		Metadata      map[string]string `yaml:"metadata"`
	}
	decoder := yaml.NewDecoder(strings.NewReader(parts[1]))
	decoder.KnownFields(true)
	if err := decoder.Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Name != agentName || !strings.HasPrefix(metadata.Description, "Use when ") ||
		len(metadata.Description) > 1024 || len(metadata.Compatibility) > 500 ||
		metadata.Metadata["version"] != agentVersion || metadata.Metadata["contract-version"] != "1" {
		t.Fatalf("invalid or unversioned skill metadata: %+v", metadata)
	}
	for _, required := range []string{
		currentJSONInvocation, "azctx list -o json", "--subscription", "azctx exec",
		"azctx guard", agentContextMismatch, "azctx login", "azctx auth broker", "--yes",
		"schemaVersion", "exit_code", "stderr", "RBAC", "PIM", "Conditional Access",
	} {
		if !strings.Contains(parts[2], required) {
			t.Errorf("skill safety contract missing %q", required)
		}
	}
	validateAgentManifests(t, metadata.Metadata["version"])
}

func validateAgentManifests(t *testing.T, version string) {
	t.Helper()
	var marketplace struct {
		Name  string `json:"name"`
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	decodeAgentJSON(t, "../.claude-plugin/marketplace.json", &marketplace)
	if marketplace.Name != agentName || marketplace.Owner.Name == "" || len(marketplace.Plugins) != 1 ||
		marketplace.Plugins[0].Name != agentName || marketplace.Plugins[0].Source != "./plugins/azctx" {
		t.Fatalf("invalid marketplace: %+v", marketplace)
	}
	var plugin struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      struct {
			Name string `json:"name"`
		} `json:"author"`
	}
	decodeAgentJSON(t, agentPlugin+".claude-plugin/plugin.json", &plugin)
	if plugin.Name != marketplace.Plugins[0].Name || plugin.Version != version ||
		plugin.Description == "" || plugin.Author.Name == "" {
		t.Fatalf("invalid plugin: %+v", plugin)
	}
	validateAgentHooks(t)
}

func validateAgentHooks(t *testing.T) {
	t.Helper()
	var hooks struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	decodeAgentJSON(t, agentPlugin+"hooks/hooks.json", &hooks)
	groups := hooks.Hooks["PreToolUse"]
	if len(hooks.Hooks) != 1 || len(groups) != 1 || groups[0].Matcher != "Bash" || len(groups[0].Hooks) != 1 {
		t.Fatalf("invalid hook groups: %+v", hooks)
	}
	handler := groups[0].Hooks[0]
	if handler.Type != "command" || handler.Command != `python3 "${CLAUDE_PLUGIN_ROOT}/hooks/guard.py"` || handler.Timeout <= 10 {
		t.Fatalf("invalid hook handler: %+v", handler)
	}
	readAgentFile(t, agentPlugin+"hooks/guard.py")
}

func decodeAgentJSON(t *testing.T, name string, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(readAgentFile(t, name)))
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("%s: extra JSON document: %v", name, err)
	}
}

func readAgentFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAgentDocumentedCommandsParseOffline(t *testing.T) {
	files := []string{"../README.md", "../AGENTS.md", "../skills/azctx/SKILL.md", agentPlugin + "skills/azctx/SKILL.md"}
	err := filepath.WalkDir("../docs", func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, file := range files {
		snippets := agentCommandSnippets(string(readAgentFile(t, file)))
		for _, snippet := range snippets {
			for _, args := range expandAgentSynopsis(snippet.text) {
				total++
				if err := parseAgentExample(args, snippet.invocation); err != nil {
					t.Errorf("%s: %q: %v", file, snippet.text, err)
				}
			}
		}
	}
	if total < 100 {
		t.Fatalf("command extraction unexpectedly omitted examples: %d", total)
	}
	t.Logf("parsed %d expanded command examples/references without executing handlers", total)
}

type agentSnippet struct {
	text       string
	invocation bool
}

func agentCommandSnippets(markdown string) []agentSnippet {
	var snippets []agentSnippet
	inline := regexp.MustCompile("`(azctx(?:[ \\t]+[^`\\n]*)?)`")
	command := regexp.MustCompile(`(?:^[ \t]*|[$][(][ \t]*|[:=][ \t]*["'])(azctx(?:[ \t]+.*|$))`)
	fenced := false
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	markdown = strings.ReplaceAll(markdown, "\\\n", " ")
	for line := range strings.SplitSeq(markdown, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if match := command.FindStringSubmatch(line); match != nil {
				snippets = append(snippets, agentSnippet{match[1], true})
			}
			continue
		}
		for _, match := range inline.FindAllStringSubmatch(line, -1) {
			snippets = append(snippets, agentSnippet{match[1], false})
		}
	}
	return snippets
}

// Synopses use brackets/alternatives; shell examples use variables/quotes. None
// of this text is evaluated by a shell, and no Cobra handler is allowed to run.
func expandAgentSynopsis(text string) [][]string {
	text = strings.ReplaceAll(text, `\|`, "|")
	text = strings.NewReplacer("[flags]", "", "[command]", "").Replace(text)
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, "VALUE")
	text = strings.NewReplacer("[", "", "]", "", " | ", "|").Replace(text)
	words := regexp.MustCompile(`'[^']*'|"[^"]*"|[^\s]+`).FindAllString(text, -1)
	redirect := regexp.MustCompile(`^\d*[<>]`)
	results := [][]string{{}}
	for _, word := range words[1:] {
		if strings.HasPrefix(word, "#") || strings.HasPrefix(word, "&&") || strings.HasPrefix(word, "||") || redirect.MatchString(word) {
			break
		}
		word = strings.TrimRight(word, `)",;`)
		word = strings.Trim(word, `"'`)
		if word == "..." {
			continue
		}
		if word == "SHELL" {
			word = "bash"
		}
		var expanded [][]string
		for _, result := range results {
			for alternative := range strings.SplitSeq(word, "|") {
				expanded = append(expanded, append(append([]string{}, result...), alternative))
			}
		}
		results = expanded
	}
	return results
}

func parseAgentExample(args []string, invocation bool) error {
	root := cmd.NewRootCommand(cmd.Dependencies{})
	root.InitDefaultCompletionCmd()
	root.InitDefaultHelpCmd()
	disableAgentHandlers(root, invocation)
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	_, err := root.ExecuteC()
	return err
}

func disableAgentHandlers(command *cobra.Command, invocation bool) {
	command.PreRun, command.PreRunE = nil, nil
	command.PersistentPreRun, command.PersistentPreRunE = nil, nil
	command.PostRun, command.PostRunE = nil, nil
	command.PersistentPostRun, command.PersistentPostRunE = nil, nil
	container := command.Run == nil && command.RunE == nil
	command.Run = nil
	command.RunE = func(command *cobra.Command, args []string) error {
		if container && len(args) != 0 {
			return fmt.Errorf("unknown command for %s: %v", command.CommandPath(), args)
		}
		return nil
	}
	if !invocation && command.Parent() != nil && !container {
		command.Args = nil // Inline references may omit required positional args.
	}
	for _, child := range command.Commands() {
		disableAgentHandlers(child, invocation)
	}
}

func TestAgentDocumentParserRejectsBrokenExamples(t *testing.T) {
	for _, text := range []string{
		"azctx list --invented",
		"azctx --filter-tenant-level",
		"azctx auth nonexistent",
		"azctx exec VALUE terraform plan",
		"azctx exec VALUE --",
		"azctx alias set only-one-argument",
		"azctx list [flags] --invented",
		"azctx auth [command] nonexistent",
		"azctx list --invented > output.txt",
	} {
		if err := parseAgentExample(expandAgentSynopsis(text)[0], true); err == nil {
			t.Errorf("invalid example accepted: %s", text)
		}
	}
	snippets := agentCommandSnippets("`azctx list -o json`\n```sh\nx=\"$(azctx env \\\n --shell bash)\"\n```\n")
	if len(snippets) != 2 || !snippets[1].invocation {
		t.Fatalf("lost inline or embedded multiline shell example: %+v", snippets)
	}
}

func TestAgentDocumentParserDistinguishesPackageNamesAndRedirections(t *testing.T) {
	markdown := "```sh\nchoco install azctx --version=2026.11.0\ncommand -v azctx >/dev/null 2>&1\n" +
		"azctx completion bash > completion.sh\ncontext=\"$(azctx env --shell bash)\"\n" +
		"command = \"azctx -c -o name\"\n{\"command\":\"azctx -c -o name\"}\n```\n"
	snippets := agentCommandSnippets(markdown)
	if len(snippets) != 4 {
		t.Fatalf("package names must not become CLI invocations: %+v", snippets)
	}
	for _, snippet := range snippets {
		for _, args := range expandAgentSynopsis(snippet.text) {
			if err := parseAgentExample(args, true); err != nil {
				t.Errorf("shell wrapper handling: %q: %v", snippet.text, err)
			}
		}
	}
}

func TestAgentDocumentParserLineEndings(t *testing.T) {
	for _, ending := range []struct {
		name string
		text string
	}{
		{"LF", "\n"},
		{"CRLF", "\r\n"},
	} {
		t.Run(ending.name, func(t *testing.T) {
			for _, example := range []struct {
				name      string
				text      string
				args      []string
				wantError string
			}{
				{
					name: "continued guard flags",
					text: "azctx guard \\\n  --subscription 11111111-1111-1111-1111-111111111111 \\\n  --tenant aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
					args: []string{"guard", "--subscription", "11111111-1111-1111-1111-111111111111", "--tenant", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
				},
				{
					name: "embedded continued command",
					text: "context_env=\"$(azctx env \\\n  --shell bash)\"",
					args: []string{"env", "--shell", envBashSyntax},
				},
				{
					name:      "invalid continued flag",
					text:      "azctx guard \\\n  --invented",
					args:      []string{"guard", "--invented"},
					wantError: "unknown flag: --invented",
				},
				{
					name:      "invalid continued subcommand",
					text:      "azctx auth \\\n  nonexistent",
					args:      []string{"auth", "nonexistent"},
					wantError: `unknown command "nonexistent" for "azctx auth"`,
				},
			} {
				t.Run(example.name, func(t *testing.T) {
					markdown := strings.ReplaceAll("```sh\n"+example.text+"\n```\n", "\n", ending.text)
					snippets := agentCommandSnippets(markdown)
					if len(snippets) != 1 || !snippets[0].invocation {
						t.Fatalf("expected one executable snippet, got %+v", snippets)
					}
					args := expandAgentSynopsis(snippets[0].text)
					if len(args) != 1 || !slices.Equal(args[0], example.args) {
						t.Fatalf("continued arguments: got %q, want %q", args, example.args)
					}
					err := parseAgentExample(args[0], snippets[0].invocation)
					if example.wantError == "" {
						if err != nil {
							t.Fatal(err)
						}
					} else if err == nil || !strings.Contains(err.Error(), example.wantError) {
						t.Fatalf("got %v, want error containing %q", err, example.wantError)
					}
				})
			}
		})
	}
}
