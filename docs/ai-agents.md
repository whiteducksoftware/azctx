# AI agents

This guide describes the unreleased azctx 2026.11 source contract. This repository
ships an [Agent Skill](../skills/azctx/SKILL.md), not an MCP server. Agents call
the CLI directly. Resource operations can use the separate
[Azure MCP Server](https://learn.microsoft.com/en-us/azure/developer/azure-mcp-server/overview).
Authentication is configured separately by the user.

## Install

Review the skill and hook before enabling them. Use a reviewed source build
on PATH until a trusted 2026.11 binary is actually published and verified;
[release readiness is blocked](releasing.md).
The plugin does not download it or access credentials.

### Claude Code

In a Claude Code session, after this release is available in the repository:

```text
/plugin marketplace add whiteducksoftware/azctx
/plugin install azctx@azctx
```

The marketplace's local source is `plugins/azctx`. Its self-contained skill and
`hooks/hooks.json` are auto-discovered; no paths escape the cached plugin.
Invoke `/azctx:azctx` or let Claude select the skill for Azure work.
The bundled Bash hook requires **Python 3.10+** as `python3` on PATH (standard
library only). This includes Windows when using Claude's Bash tool; it does
not hook PowerShell or other tools.

For offline development, validate both the marketplace and plugin:

```sh
claude plugin validate .
claude plugin validate ./plugins/azctx
```

The hook is optional: to use only guidance, copy `skills/azctx/` into
`.claude/skills/azctx/` in your target project instead of installing the plugin.
Do not install both copies in the same scope.

### GitHub Copilot

Copy this repository's `skills/azctx/` folder into
`.github/skills/azctx/` in the target repository. Copilot loads the skill when
relevant; ask it to use the azctx skill for Azure work. Commit the project skill
if cloud agents need it; a personal local copy does not travel to the cloud.
Copilot CLI also supports personal skills at `~/.copilot/skills/azctx/`.
No Claude hook is installed by copying the skill.

### Codex

Copy `skills/azctx/` to your target repository's `.agents/skills/azctx/`,
or `~/.agents/skills/azctx/` for local personal use. Invoke `$azctx` or select it
with `/skills`. Restart Codex if discovery has not refreshed.
The skill is portable guidance; the Claude Bash hook is not a Codex hook.

### Cursor

Copy `skills/azctx/` to `.cursor/skills/azctx/` in the target repository, or
`~/.cursor/skills/azctx/` for local personal use. Use `/` in Agent chat and select
azctx, or let Agent discover it. `.agents/skills/azctx/` is also supported;
use one location, not duplicate copies. For remote/cloud agents, commit a
project skill rather than assuming personal files are synchronized.
This does not install a Cursor hook.

## Agent-safe workflow

Before **any Azure action**, inspect actual context and candidate identities:

```sh
azctx -c -o json
azctx list -o json
```

Check status and `schemaVersion: 1`; do not guess from names. Before changes,
check the reviewed nearest project expectation with `azctx guard -o json`.
Stop on exit **4** and every other failure. Without a project file, use reviewed
explicit `--subscription` and `--tenant` GUID expectations. Do not silently
rewrite a project file or change the user's default to make guard pass.

Use `az --subscription <id>` for resource commands and
`azctx exec <query> -- <cmd>` for Terraform, azd and SDK-based tools. Exec only
overrides four child environment variables: it is **not** an Azure CLI
subscription override, authentication mechanism or sandbox. Verify the child's
effective target, including tool-specific overrides, before changes.
Never autonomously run `az account set`, azctx subscription/tenant switching,
`azctx login`, `azctx auth broker`, or protected-subscription `--yes`.
Ask the user to handle authentication and explicit protected switching.

### Exec preflight

**`azctx exec` is not hook-guarded**: its first literal word is `azctx`, not
one of the hook's watched tools. Agents must perform the checks themselves.
Resolve the chosen exec query to one schema-1 list entry and pin its subscription
`id` and `tenantId` GUIDs. Replace the sample GUIDs with that reviewed pair and
reuse the subscription GUID for exec, not an unresolved name or alias.
From the same working directory as the reviewed nearest `.azctx.yaml`, run:

```sh
azctx guard --no-input -o json &&
azctx guard --no-input --subscription 11111111-1111-1111-1111-111111111111 --tenant aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa -o json &&
azctx exec --no-input 11111111-1111-1111-1111-111111111111 -- terraform plan -input=false
```

Execute only if both guards succeed. The first guard has no explicit overrides:
it checks the current default against the reviewed project. The explicit guard
flags replace project expectations: the second checks the current default
against the resolved target, **not** the project file. With unchanged context,
their conjunction establishes that the target meets the project expectations;
the explicit guard alone cannot establish this. Stop on every nonzero status;
never auto-switch or rewrite the project to repair a mismatch.
Repeat both guards for each exec call. For azd, replace only the child command
after `--` with `azd env get-values`.

If no project file is intended, independently compare the resolved pair with
user-reviewed GUID expectations before the explicit target guard and exec.
That is not a project check or a fallback after a failed project guard.
These are point-in-time checks, not a lock: re-inspect and repeat if the profile,
project, default or intended target changes between steps. Child tool-specific
overrides must still be checked independently.

### Statuses

With JSON output, data is on stdout and one azctx error object is on stderr;
read `code` and `exit_code`, not message text. Statuses are 0 success, 1 general
error, 2 no/ambiguous match, 3 sign-in failure, 4 guard mismatch, 130 canceled.
Exec child output/status is passed through, not converted to azctx JSON.
Use `--no-input` for automation and never pick the first ambiguous candidate.
See the [versioned CLI contract](cli-contract.md).

## Project file and optional Bash hook

Review a project's expectations before use:

```yaml
# .azctx.yaml
version: 1
subscription: 11111111-1111-1111-1111-111111111111
tenant: aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
```

The file is inert: it never instructs azctx to select or authenticate a target.
The [strict parser](configuration.md#project-context) requires both GUIDs,
rejects unknown/duplicate fields, additional documents and symlinks, and uses
the nearest file, even when it is invalid. No ancestor fallback on errors.

The plugin's `PreToolUse` matcher is `Bash`. For commands whose first literal
word is `az`, `azd`, `terraform` or `bicep` (including leading whitespace or
quotes), the hook:

1. Uses the event's `cwd`, not the plugin install directory or original project
   root. Detects `.azctx.yaml` presence in that directory and its ancestors.
2. Invokes only `azctx guard --no-input`, with stdin disconnected and input/update
   opt-outs in the child environment. azctx alone parses the project file.
3. Continues silently on match. Any guard failure, including mismatch 4 or
   malformed project 1, becomes **hook exit 2**, with a one-line blocking reason
   on stderr. A ten-second guard timeout or execution failure also blocks.
   Inspect context manually with `azctx guard -o json`; raw child diagnostics
   are intentionally not echoed by the hook.
4. If azctx is not on PATH **or** no `.azctx.yaml` exists in the ancestry,
   continues with a one-line stderr notice. This is deliberate opt-in behavior,
   not a claim that the context was checked.

Malformed hook JSON/required Bash input blocks with exit 2. The hook never
executes the proposed Bash command, reads transcripts, credentials or token
caches, switches context, writes Azure files, or logs in. Unrelated tools and
commands are left to their normal permission flow. Missing Python or a host
that fails to launch the hook can bypass it; verify dependencies yourself.

## Limits and acceptance

Guard, the skill and protected subscriptions reduce mistakes but are **no
security boundary**. Access control stays with Azure RBAC, PIM and Conditional
Access. Guard is point-in-time, not a lock: another terminal can switch later.
It compares the current default, **not** a command's explicit target, Terraform
provider settings or azd environment. A mismatching default can therefore block
even a correctly targeted command; ask the user to resolve it, do not bypass it.

This lightweight hook is not a full shell analyzer. It does not detect wrapped
commands (`azctx exec ... -- terraform ...`, `env ... az`, `bash -c ...`),
absolute executable paths, aliases,
commands after `cd ... &&`, or commands later in a compound shell string.
Keep context-sensitive calls as separate, directly named Bash commands.
For exec, use the explicit [two-guard preflight](#exec-preflight); this guidance
does not add wrapper parsing or automatic hook enforcement.
Other tools, MCP calls, disabled hooks and skill-only installations have no
automatic hook enforcement. Agents can ignore instructions.

`make check` includes offline command/documentation/manifest acceptance and
Python hook tests; CI's existing `make test` runs them too. The standalone and
plugin skill copies must remain byte-identical. Their metadata and the plugin
version advance with the CLI release (currently `2026.11.0`, contract `1`).
Command examples are parsed without executing their handlers; authentication
examples are never run by these tests.

Before publishing, run the two native `claude plugin validate` commands above.
They validate files, not end-to-end skill compliance, hook activation in a
trusted session, marketplace installation, live authentication or deployment.
Manually verify discovery and an allowed/blocked Bash call in a trusted local
Claude session using synthetic context. No live Azure acceptance is implied.

## Format references

- [Agent Skills specification](https://agentskills.io/specification)
- [Claude hooks](https://code.claude.com/docs/en/hooks),
  [plugin manifests](https://code.claude.com/docs/en/plugins/manifest-reference)
  and [marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Copilot skills](https://docs.github.com/en/copilot/concepts/customizations/agent-skills)
- [Codex skills](https://developers.openai.com/codex/skills)
- [Cursor skills](https://cursor.com/docs/skills)
