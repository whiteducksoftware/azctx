# Development

Read [CONTRIBUTING](../CONTRIBUTING.md) and [AGENTS](../AGENTS.md) first.
This branch is an unreleased stack layer. Do not merge, tag or publish as part
of documentation validation. No credentials or real tenant are needed.

## Build and checks

Use `go.mod`: Go 1.26 minimum, preferred toolchain Go 1.27.3. Allow automatic
selection unless that exact supported toolchain is already active.
Use golangci-lint v2.15.0 and Python 3.10+ as described in CONTRIBUTING.

```sh
make build
make check
make docs
make docs-check
make e2e
make ci-check
go mod verify
```

`make check` runs strict lint, offline Python hook tests, race-enabled Go tests,
statement-weighted `internal/` coverage >=70%, and the separate uncached native
500-subscription completion gate (<50 ms per call). Do not lower thresholds
or disable scanner failures to make a change pass.
CI also runs native Linux/macOS/Windows tests, strict actionlint/zizmor,
module checks, vulnerability policy and fuzz campaigns.
[Integration tests](integration-tests.md) describes command coverage and the
explicitly opt-in dedicated-tenant nightly. An unconfigured/skipped nightly
is not live acceptance.

## Generated reference and help

Help metadata is in `cmd/help.go`; handlers remain in their command files.
Every public command, including help and shell completion children, has
`Short`, explanatory `Long` and two or three real example invocations.

```sh
make docs
make docs-check
go test ./cmd -run 'TestEveryCommandHasUsefulOfflineHelp|TestAgentDocument'
go test ./internal/clidocs
```

`tools/gendocs` uses `cobra/doc` against an inert command tree with no runner
or profile initialization. Dates/autogeneration tags are disabled.
`make docs` rewrites the owned `docs/cli/` file set; never put hand-written files
there. `make docs-check` compares exact bytes and filenames without rewriting
anything and fails for changed, missing or extra pages, including untracked
ones. CI invokes that check after module tidiness; Go tests repeat it.
Run generation twice and review the first diff; the second must be identical.

The existing agent-document parser covers README, all Markdown under docs,
AGENTS and both skill copies. It expands documented synopsis alternatives and
parses flags/arguments with handlers disabled. Cobra's `[flags]`/`[command]`
metavariables are omitted, not treated as real arguments; invented commands,
unknown/removed flags and malformed exec/alias invocations still fail.
This is parse acceptance, not execution of authentication or infrastructure.
The targeted fake-az README test separately exercises the actual quickstart
and read-only commands against synthetic profiles.

## Portable fake az

`internal/testutil.FakeRunner` records argv/environment overrides and consumes
canned responses. Integration tests also build
`internal/testutil/fakeaz` as an actual executable named `az` on an isolated
PATH. Its `FAKEAZ_FIXTURES` JSON array provides `args`, `stdout`, `stderr` and
`exitCode` for each call. `FAKEAZ_RECORD` receives JSON lines; only environment
keys in `FAKEAZ_ENV_KEYS` are recorded. Never place real credentials in fixtures.

```json
[{"args":["version","-o","json"],"stdout":"{\"azure-cli\":\"2.61.0\"}"},{"args":["login"]}]
```

Use `testutil.WorkDir` and `testutil.IsolateUserDirs` for synthetic private
files under package-local `.test-work/`, cleaned after tests. No real Azure
profile, token cache or Azure CLI is touched. Tests that simulate an Azure
profile refresh do so in the fixture, never through production direct writes.

## Picker capture

The README's static SVG is captured from `ui.NewPicker(...).View()` with
synthetic GUID-distinct subscriptions, a fixed 160 by 15 terminal, no ANSI
styling and no live account. It is a static terminal capture, not a demo GIF.
The wrapper escapes XML and adds only a neutral terminal background.
Regenerate from the repository root:

```sh
go run ./tools/pickercapture > assets/img/picker-2026.11.svg
make golden
```

`make golden` updates real picker screen goldens at 60, 100 and 160 columns.
Review changes, run `make check`, and ensure repeat generation is unchanged.
The capture generator does not change picker production behavior.

## Release process and manual winget PR

The actual configuration and publisher-disabled verification commands are in
[releasing](releasing.md). Both source configs retain CGO disabled, trimpath,
commit-derived timestamps/build date and separate Unix/Windows archives.
Stable/RC snapshot checks generate packages without publication; Windows uses
real native `choco pack` and official winget schemas, macOS normalizes and
checks its generated cask natively.

**Release readiness remains blocked.** Pinned Harden-Runner has no hosted
Windows/macOS block-mode enforcement. The credential-free readiness job
deliberately fails before every publisher, including RCs. Review and actually
accept a supported native enforcement design before changing that blocker.
A green package/schema run, an environment approval or these instructions
cannot substitute for endpoint enforcement.

After that blocker is resolved and a separately authorized release is ready,
the reviewed workflow uses the protected `release` environment and scoped,
short-lived App tokens. Environment reviewers, App installation, secrets,
federated identities, repository settings and tap migration were not provisioned
by this stack. Do not run a direct publishing GoReleaser pass to test them.

The future stable workflow updates only `azctx-<version>` in
`whiteducksoftware/winget-pkgs`. It never opens the upstream PR automatically.
An authorized maintainer must inspect the generated three manifests, hashes,
archive URLs, Azure CLI minimum version and official schema results, then open
a PR from that fork branch to `microsoft/winget-pkgs` manually. For example,
**only after authorized publication and fork inspection**:

```sh
gh pr create --repo microsoft/winget-pkgs \
  --head whiteducksoftware:azctx-2026.11.0 --base master \
  --title 'New version: whiteducksoftware.azctx version 2026.11.0' \
  --body 'Reviewed generated manifests for the published 2026.11.0 release.'
```

This example is not run by any test and no PR was opened here. Wait for upstream
acceptance and real package installation/upgrade checks. Chocolatey moderation,
Homebrew trust/migration, observed native egress, live attestations and broker/
OIDC/MFA acceptance remain separate manual/hosted gates.
Latest promotion remains manual, only after package availability is verified;
`RELEASE_MAKE_LATEST` defaults to `false`. No tag, release, merge or promotion
is part of this documentation change.
