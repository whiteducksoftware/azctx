# Development

Read [CONTRIBUTING](../CONTRIBUTING.md) and [AGENTS](../AGENTS.md) first.
Local checks need no credentials or real tenant, and validating a change never
requires merging, tagging or publishing.

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
manual dedicated-tenant live test (`make live-test`), which maintainers run
locally and CI never runs. Offline fake runs are not live acceptance.

## Dependency maintenance

[Dependabot](../.github/dependabot.yaml) checks two ecosystems weekly, with up
to 10 open pull requests each:

- `gomod`: the root Go module, direct and indirect dependencies;
- `github-actions`: action references in the workflows.

Minor and patch updates are grouped per ecosystem; each major update arrives as
its own pull request for review. There is no cooldown, ignore rule or
major-version exclusion. Review updates like any other change with `make check`;
regenerate goldens or docs only for intentional output changes.

The terminal UI uses the `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`
and `charm.land/lipgloss/v2` modules. Two Charm modules have no release tags,
so they stay on reviewed pseudo-versions:
`github.com/charmbracelet/x/exp/teatest/v2` (tests only) and the indirect
`github.com/charmbracelet/ultraviolet`. Dependabot cannot offer tagged updates
for them; review their upstream commits together with Bubble Tea upgrades.

Dependabot cannot update tool versions embedded in workflow inputs
(`with: version`, `syft-version`), `go install ...@version` commands or the
preferred Go `toolchain`. Update these seven pins (golangci-lint, actionlint,
govulncheck, zizmor, GoReleaser, Syft and Go) manually. When upgrading one:

1. Change every occurrence in the same change: the workflows under
   `.github/workflows/`, the `toolchain` line in `go.mod`, the contributor
   `go install` command, the GoReleaser version literal in
   `.github/scripts/test_release_config.rb` and exact version literals in
   CONTRIBUTING and the docs.
2. Update version mentions in README, AGENTS, CONTRIBUTING and `docs/` that
   are spelled differently, such as "Go 1.27.3" or "GoReleaser 2.19.0". Leave
   released upgrade-guide history unchanged.
3. Run `make check` and `make ci-check`.

Compatibility floors are not upgrade targets. The Go 1.26.0 `go` directive,
Azure CLI 2.61.0 and its immutable minimum image digest stay fixed for the
manual minimum-versus-latest matrix in [integration tests](integration-tests.md).
The maintained latest pins are the seven tool pins above, including the
preferred Go 1.27.3 toolchain and golangci-lint v2.15.0.
GoReleaser owns the generated package and winget manifest schema versions
(currently 1.12.0); `check_winget.ps1` fetches Microsoft's schema matching each
generated `ManifestVersion`. Runner-provided Python, Ruby, PowerShell,
Homebrew, Chocolatey, Docker and hosted `*-latest` runner images are managed
by the platform and are not pinned in this repository.

## PR validation binaries

The native `test` job on Linux, macOS and Windows uploads one validation
binary per runner after its tests and end-to-end suite pass. Use them to try a
pull request without building it. They contain proposed, unreviewed code: they
are not releases, and their checksums detect corruption but are not
signatures or provenance. Do not expose real credentials or production
tenants to an unreviewed binary.

Each GitHub artifact is named
`azctx-validation-<GOHOSTOS>-<GOHOSTARCH>-<full checkout SHA>`, for example
`azctx-validation-linux-amd64-<sha>`, and holds exactly one
`<artifact name>.tar.gz`. Artifacts are kept for 14 days unless repository
policy is shorter. The tar archive contains three root files:

| File | Mode | Content |
| --- | --- | --- |
| `azctx` (`azctx.exe` on Windows) | 0755 | `CGO_ENABLED=0`, `-trimpath` native build |
| `metadata.json` | 0644 | Schema version 1 build metadata |
| `SHA256SUMS` | 0644 | SHA-256 of the binary and `metadata.json` |

The SHA is the revision that CI actually checked out and tested. **For a pull
request this is GitHub's test merge commit (`GITHUB_SHA`), not the PR head
commit**, so do not look for the head SHA in artifact names. The binary reports
version `0.0.0-validation.<first 12 characters of the SHA>`, the full SHA as its
commit and the commit's UTC timestamp as its build date. `metadata.json`
records `schemaVersion`, `kind` (`validation`), `revision`, `revisionSource`
(`checkout HEAD`), `version`, `buildDate`, `goos`, `goarch`, `binary`,
`goVersion`, `cgoEnabled` and `trimpath`.

List the artifacts of a completed CI run, then download and extract the one for
your platform:

```sh
REPO=whiteducksoftware/azctx
RUN_ID=<completed-ci-run-id>
gh api --paginate "repos/$REPO/actions/runs/$RUN_ID/artifacts?per_page=100" \
  --jq '.artifacts[] | select(.name | startswith("azctx-validation-")) | .name'

# Copy the full matching name from the list, including its tested checkout SHA.
ARTIFACT='azctx-validation-darwin-arm64-<full-tested-checkout-sha>'
gh run download "$RUN_ID" --repo "$REPO" --name "$ARTIFACT" \
  --dir ./azctx-validation-download
tar -xzf "./azctx-validation-download/$ARTIFACT.tar.gz" \
  -C ./azctx-validation-download
cd ./azctx-validation-download
```

`gh run download` only unwraps GitHub's outer artifact archive; extract the tar
before running the binary. Ordinary `tar -xzf` keeps the executable mode, so no
`chmod` is needed unless your extractor drops Unix permissions. Then verify and
inspect offline. On Linux:

```sh
sha256sum -c SHA256SUMS
./azctx version -o json
cat metadata.json
```

On macOS:

```sh
shasum -a 256 -c SHA256SUMS
./azctx version -o json
cat metadata.json
```

On Windows, extract the `windows` artifact's tar with the built-in `tar -xzf`,
enter the extraction directory and run:

```powershell
Get-Content SHA256SUMS | ForEach-Object {
    $expected, $file = $_ -split '  ', 2
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "SHA256 mismatch: $file" }
    Write-Output "$file`: OK"
}
.\azctx.exe version -o json
Get-Content metadata.json
```

Both checksums must pass. The executable's `commit` must equal the metadata
`revision`, and its `version`, `buildDate` and `goVersion` must equal the same
metadata fields. None of these steps needs Azure.

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

`make docs` runs `go run ./internal/cmd/gendocs` and `make docs-check` adds
`-check`. The command uses `cobra/doc` against an inert command tree with no runner
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

## Demo GIF

The README's `assets/img/azctx-demo.gif` is a committed static asset. It shows
the real built-in picker with ten synthetic subscriptions in three tenants
(dev, staging and production names, invented GUIDs) responding to key presses
(down, `/`, typing `prod` and `fab`, Escape, Enter). A caption bar names each
key; the window title labels the demo as synthetic. No Azure CLI call or real
profile was involved.

The repository has no generator for the GIF and no test compares it with
current picker output, so later picker changes are not reflected in it. The
picker screen goldens at 60, 100 and 160 columns (`make golden`) remain the
checked reference for picker output.

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
