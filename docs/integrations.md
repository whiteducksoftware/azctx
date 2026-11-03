# Integrations

Install or build reviewed tools separately; azctx does not install integrations,
fetch tokens or grant Azure access. [CI/CD](ci-cd.md) covers pipeline
authentication and export timing.

## env and exec

```bash
set -euo pipefail
context_env="$(azctx env prod --shell bash)"
eval "$context_env"
az group list --subscription "$AZURE_SUBSCRIPTION_ID"
```

Check export success **before** evaluation. The four variables, in fixed order,
are `ARM_SUBSCRIPTION_ID`, `ARM_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` and
`AZURE_TENANT_ID`. They contain validated GUIDs only, never credentials.
Bash is the default; zsh, fish and PowerShell syntax are also available.
Do not evaluate one shell's syntax in another shell.

```fish
set context_env (azctx env prod --shell fish)
and eval $context_env
```

```powershell
$contextEnv = azctx env prod --shell powershell
if ($LASTEXITCODE -ne 0) { throw 'Context resolution failed' }
Invoke-Expression ($contextEnv -join "`n")
```

Export resolution never prompts or switches. An omitted query uses actual
current context; ambiguity exits 2 even on a TTY. `--all` exposes configured
hidden entries, not disabled/tenant-level/foreign-cloud accounts.

```sh
azctx exec prod -- terraform plan
azctx exec dev -- azd env get-values
```

Exec resolves one executable once and attaches streams without an implicit
shell. Only the four child context values change; the parent environment and
Azure CLI default do not. Explicit children own their output, status, network
access and filesystem side effects, even under `-o json`. Context variables
can be overridden by tool-specific settings. Exec is not a sandbox or process
tree supervisor. **Azure CLI ignores these variables**: every `az` resource
command needs an explicit `--subscription`.

## Terraform

For azurerm 4.x, use `ARM_SUBSCRIPTION_ID` or provider `subscription_id`.
Configure authentication independently, for example through reviewed workload
identity or an existing Azure CLI login. Review aliases, provider blocks,
backend configuration and any explicit tenant/subscription overrides.

```sh
azctx guard --no-input &&
azctx exec 11111111-1111-1111-1111-111111111111 -- terraform plan
```

Run subsequent commands only if guard succeeds. Guard checks the current
default, not Terraform's resolved provider target. Review the plan before
mutating infrastructure; this guide does not perform an apply.
Agents must also follow the [two-guard exec preflight](ai-agents.md#exec-preflight)
before each exec call.

## Azure Developer CLI

```sh
azctx env dev --shell bash
azctx exec dev -- azd env get-values
```

azd can consume `AZURE_SUBSCRIPTION_ID` and `AZURE_TENANT_ID`, but selected azd
environment values and tool-specific configuration can take precedence.
Inspect those values before deployment. Context export does not authenticate
azd and does not promise that every azd command uses the exported target.

## Shell prompts

Read current context, never switch from a prompt:

```sh
azctx -c -o name
azctx -c -o id
```

Plain-field output has one LF, no styling or project-file warnings, and no
update request. Avoid JSON parsing of the human active-subscription line.
Profiles can contain private display names; decide what your prompt should show.

### Starship

Starship's built-in [azure module](https://starship.rs/config/#azure) reads Azure
CLI context independently. To display azctx's resolved subscription instead,
use a custom module and do not also enable a duplicate Azure segment:

```toml
[custom.azctx]
command = "azctx -c -o name"
when = "command -v azctx >/dev/null 2>&1"
format = "[$output]($style) "
style = "blue"
```

This POSIX example has normal command-startup cost and reads the local profile
on each prompt. No live Starship acceptance is implied.

### oh-my-posh

The built-in [Azure Subscription segment](https://ohmyposh.dev/docs/segments/cloud/az)
can follow Azure CLI's default after an azctx switch:

```json
{"type":"az","style":"plain","template":" {{ .Name }} ","options":{"source":"cli"}}
```

Merge the segment into your existing theme rather than replacing it blindly.
This segment reads the CLI profile independently; it does not invoke azctx or
replicate azctx's active-cloud fallback, visibility and project checks.
It displays the shared default, not an isolated exec context. This is a
configuration example, not a claim that oh-my-posh was installed or activated.

### Powerlevel10k

For zsh, add a custom segment in your existing `.p10k.zsh`:

```zsh
function prompt_azctx() {
  local context_name
  context_name="$(azctx -c -o name 2>/dev/null)" || return
  p10k segment -f blue -t "${context_name//\%/%%}"
}
```

Add `azctx` to `POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS` in that file. The example
escapes `%` so profile text does not introduce zsh prompt escapes. Do not enable
Powerlevel10k expansion options for untrusted profile labels. See
[Powerlevel10k custom segments](https://github.com/romkatv/powerlevel10k#extensible).

## Completion

Generate scripts without login or network access:

```sh
azctx completion bash
azctx completion zsh
azctx completion fish
azctx completion powershell
```

For example, after creating the destination directory:

```sh
azctx completion bash > "$HOME/.local/share/bash-completion/completions/azctx"
azctx completion fish > "$HOME/.config/fish/completions/azctx.fish"
```

For zsh, put the generated script in an existing `fpath` directory as `_azctx`
and initialize `compinit`. For PowerShell, dot-source a reviewed generated
`.ps1` script from your profile, subject to your execution policy.
Completion suggests visible names, aliases and qualified subscriptions with
case-insensitive prefix matching. It never runs az, writes state or checks
updates. Shell setup/activation remains a manual step.

## fzf

Set `picker: fzf` in the [private configuration](configuration.md) to use an
already installed fzf for both subscription and tenant selection.
azctx supplies indexed, sanitized rows and accepts only an exact supplied row
back. Every row carries GUID identity; labels never become commands/options.
`FZF_*` defaults, option files and execute/preview bindings are removed from the
child environment. Missing fzf falls back once to the built-in picker with a
stderr notice; other failures are not silently ignored.
Protection, input and cloud gates still apply. Escape/no-match exits 1;
Ctrl+C exits 130.
