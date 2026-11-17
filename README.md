# <img src='assets/img/DB-WD-Iconsazctx-1200px.png' height=40> &nbsp; `azctx`: Power tool for the Azure CLI 

![azctx promo](assets/img/DB-WD-azctx-TW-440x220px@2x.png)

![Proudly written in Go](https://img.shields.io/badge/written%20in-go-29BEB0.svg)
![Latest GitHub release](https://img.shields.io/github/v/release/whiteducksoftware/azctx.svg)
![GitHub stars](https://img.shields.io/github/stars/whiteducksoftware/azctx.svg?label=github%20stars)
![GitHub contributors](https://img.shields.io/github/contributors/whiteducksoftware/azctx.svg?label=github%20contributors)

**`azctx`** helps you switch between azure cli subscriptions back and forth:

![azctx demo GIF](assets/img/azctx-demo.png)

## Collaborate with us 👋

* Feel free to give us your **feedback**, make **suggestions** or let us discuss in our 📢 [**discussions tab**](../../discussions/).
* To report **issues** or search for existing issues go to the 🔎 [**issues tab**](../../issues/).

_And please star this repo ⭐_

## azctx

```bash
azctx is a CLI tool for managing azure cli subscriptions.
        It is a helper for the azure cli and provides a simple interface for managing subscriptions.
        Pass a subscription name, tenant/subscription or subscription ID to switch.
        Use -- before names that equal a command. Pass - for the previous subscription.
        Without interactive input, the bare command lists subscriptions.

Usage:
  azctx [query | -] [flags]
  azctx [command]

Available Commands:
  auth        Manage Azure CLI authentication
  completion  Generate the autocompletion script for the specified shell
  doctor      Show safe, offline Azure CLI diagnostics
  help        Help about any command
  login       Login to Azure
  list        List subscriptions
  version     Print the CLI version

Flags:
  -c, --current         Display the current active subscription
  -h, --help            help for azctx
      --no-input        Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string   Output format (depends on command)
      --verbose         Enable debug diagnostics

Use "azctx [command] --help" for more information about a command.
```

```bash
Login to Azure (wrapped around 'az login')
        Requires Azure CLI 2.61.0 or newer; tenant metadata comes from its profile.
        Azure CLI's subscription selector is disabled only for this login process.
        All args after -- are directly passed to the 'az login' command.

Usage:
  azctx login [flags]

Flags:
      --force-mfa         force individual authentication for each tenant separately (required for tenants which enforce explicit MFA)
  -h, --help              help for login
      --tenant string     Tenant GUID or fuzzy profile tenant name/domain
      --use-device-code   Sign in using a device code
```

-----

## Installation

See the [2026.11 upgrade guide](docs/upgrade.md) for breaking changes and minimum
operating system requirements. Azure CLI **2.61.0 or newer** is required.

Tenant names and domains come directly from Azure CLI's subscription profile,
even after plain `az login`. Labels prefer display names, then default domains,
then tenant IDs. A displayed record missing both modern fields gets a one-line
hint to refresh with `az login`. azctx never directly writes Azure files or
accesses credentials/token caches. The sole configuration-write exception is
`azctx auth broker enable|disable`: after explicit confirmation it delegates
account clearing, the persisted broker setting and attached sign-in to Azure CLI.
Wrapped login disables Azure CLI's subscription selector only for that child
process, without changing your environment.

Use `azctx login --use-device-code` over SSH or without a display, and
`azctx login --tenant <GUID-or-profile-name>` to choose a tenant. Progress
messages follow the effective browser, Windows WAM or macOS broker setting.
`azctx doctor [-o json]` diagnoses missing/old Azure CLI, missing or broken
profiles, settings and leftover tenant files without signing in or contacting
Azure. See the draft [authentication guide](docs/authentication.md).

`azctx list` shows active-cloud subscriptions by default. Disabled and
tenant-level accounts, and records from other/unknown clouds, are hidden;
`azctx list --all` exposes them for inspection. Change clouds with Azure CLI
before switching to a subscription in another cloud. Tables/pickers label
account types, delegation and attention states. See the
[CLI contract](docs/cli-contract.md) for JSON, matching and per-cloud defaults.

### Homebrew (macOS)

* Install `azctx` with `brew install whiteducksoftware/tap/azctx`

### Chocolatey (Windows)

* Install `azctx` with `choco install azctx`

### Binary (Windows, Linux, macOS)

* Install the `azctx` binary from the [repository](https://github.com/whiteducksoftware/azctx/releases)
* Add the `azctx` binary to your PATH

-----

## Troubleshooting

### Missing Azure configuration or `azureProfile.json`

Run `az login` or `azctx login` to let Azure CLI create the subscription profile,
even when the Azure directory does not exist yet. azctx itself never creates
Azure configuration files. An unset `AZURE_CONFIG_DIR` silently uses the default
`~/.azure` directory.

Check the [Microsoft Documentation](https://docs.microsoft.com/en-us/cli/azure/azure-cli-configuration?view=azure-cli-latest#cli-configuration-file) for the correct path to the azure cli config directory and export it as AZURE_CONFIG_DIR environment variable.  

```bash
export AZURE_CONFIG_DIR=$HOME/.azure
```

-----

## Development

Use the Go version and toolchain declared in `go.mod`, and install
`golangci-lint` v2.15.0 on your PATH.

```bash
make build # Build the azctx binary
make lint  # Run golangci-lint
make test  # Run tests with the race detector and coverage
make check # Run lint and tests
```

### Internal boundaries

`cmd` constructs Cobra commands with `cmd.NewRootCommand(cmd.Dependencies)`.
Tests can supply an `azcli.Runner`, a read-only `io/fs.FS` rooted at the Azure
config directory, a config directory, a picker,
and an update-check function without resolving a real `az` or accessing a tenant.

The internal packages separate Azure CLI execution (`azcli`), the read-only
`io/fs` profile parser (`profile`), subscription search (`match`), the existing
picker (`ui`), update checks (`update`), private previous-subscription state
(`state`) and atomic private-file storage (`storage`). `config` remains a
placeholder for a later release layer.

`azcli.Runner.Run` captures stdout, stderr, and the exit code by default.
Supplying `azcli.Options.Terminal` attaches the supplied streams (or the process
terminal) and tees stderr into the result. `Options.Env` overrides inherited
environment entries. The production runner resolves `az` once with `LookPath`.
Login uses this attached mode: device codes on stdout or stderr are visible
immediately, and stderr remains captured for authentication diagnostics.

Search matches subscription names, profile tenant names/domains/IDs and subscription IDs without
collapsing duplicate names. `azctx -` swaps the current and previous subscription
across processes. The previous ID is stored privately in
`os.UserConfigDir()/azctx/state.json`, never in the Azure configuration directory.
Switch diagnostics and optional update-check failures are silent unless
`--verbose` is supplied. Login progress and actionable warnings remain visible.

Interactive runs check for a stable CalVer update at most once per 24 hours,
with a two-second network deadline and a private cache at
`os.UserCacheDir()/azctx/update.json`. Update failures never fail a switch.
Set `AZCTX_NO_UPDATE_CHECK=1` to disable checks; they are also skipped when `CI`
is set (even to an empty value) or stdout is not a terminal. State and cache
destinations that overlap Azure configuration are rejected, including symlinks.

`internal/testutil.FakeRunner` records argv and environment overrides and consumes
canned responses. Tests also build the portable `internal/testutil/fakeaz`
executable and put it on `PATH`. Its `FAKEAZ_FIXTURES` JSON array contains
`args`, `stdout`, `stderr`, and `exitCode` per call; `FAKEAZ_RECORD` receives JSON
lines with argv and only environment names listed in `FAKEAZ_ENV_KEYS`.
Test scratch files live below the package working directory and are cleaned up.
No real Azure CLI or credentials are needed.

-----

## Migrate from stiviik/azctx

Are you an existing user of `stiviik/azctx`? Use the following commands to migrate from the old `stiviik/azctx` to the new `whiteducksoftware/azctx` repository:

```bash
brew uninstall azctx
brew untap stiviik/tap
brew install whiteducksoftware/tap/azctx
```
