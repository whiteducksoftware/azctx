# Release configuration

This page describes how tags become releases and packages. The publishing steps
and acceptance checks for a release are tracked in #100. Do not run a publishing
release to test this configuration.

**Chocolatey is not published for 2026.11.** The Windows build always passes
`--skip=chocolatey` and the workflow has no Chocolatey publisher or
`CHOCO_API_KEY`. The Chocolatey configuration and its snapshot checks stay, so
the channel can return in a later release.

**Native egress (#98):** hosted Windows and macOS support audit-only egress.
Those jobs are accepted because they hold no credentials, have no `release`
environment and only read the repository; a block-mode Ubuntu job re-verifies
their output before any token exists. See
[accepted audit-only native jobs](#accepted-audit-only-native-jobs).

## Packaging contracts

- Linux ships only the existing `azctx_Linux_*.tar.gz` archives. macOS keeps
  `azctx_Darwin_*.tar.gz`; Windows keeps `azctx_Windows_*.zip`.
- The macOS-only cask targets `whiteducksoftware/homebrew-tap/Casks/azctx.rb`,
  depends on the `azure-cli` cask and installs Azure CLI through Homebrew if it
  is missing, alongside `azctx`. The binaries remain
  unsigned/not notarized. A declarative `postflight_steps` `run` invokes
  `/usr/bin/xattr -dr com.apple.quarantine` on the staged binary; no deprecated
  Ruby `postflight` hook is used. Tap trust and formula-to-cask migration are
  separate #100 work, not performed by this workflow.
- Chocolatey authors/owners are white duck GmbH and its `azure-cli` dependency
  has version floor 2.61.0; the release workflow does not build or push it.
  winget retains `whiteducksoftware.azctx` and requires
  `Microsoft.AzureCLI` >=2.61.0. Both copyright years use the release date.
  winget updates only the version branch in the `whiteducksoftware/winget-pkgs`
  fork; opening the upstream PR remains manual.
- Both GoReleaser steps set `GORELEASER_CURRENT_TAG` to the pushed tag, so a
  release candidate and its final release can point to the same commit.
- Both configurations classify prereleases automatically. Cask and winget
  `skip_upload: auto` protect direct GoReleaser use; the workflow additionally
  excludes every hyphenated prerelease tag from the tap and winget publishers.
  Snapshots never exercise a real publisher.
- Conventional commit types form changelog groups, with breaking changes first.
  Mark breaking subjects with `!:` (for example `feat(auth)!: ...`):
  GoReleaser 2.19.0 groups commit **subjects**, not `BREAKING CHANGE` body footers.
  Static builds retain `CGO_ENABLED=0`, `-trimpath`, `.CommitTimestamp` and the
  commit-derived binary `BuildDate`.

## Protected workflow

All publishing jobs use the `release` environment; the native macOS formatter
and native Windows build do not. The environment requires approval from a code
owner and accepts deployments from tags only. The GitHub App
`white-duck-azctx-release` is installed on `homebrew-tap` and the
`winget-pkgs` fork only, with contents read and write. The environment holds:

| Name | Kind | Purpose |
| --- | --- | --- |
| `RELEASE_APP_CLIENT_ID` | Environment variable | GitHub App client ID |
| `RELEASE_APP_PRIVATE_KEY` | Environment secret | App signing key |
| `RELEASE_MAKE_LATEST` | Environment variable | Literal `true` or `false`; defaults to `false` |

Short-lived, automatically revoked App tokens request contents-write on only
`homebrew-tap` or only `winget-pkgs`, never the source repository or both taps.
`AUTH_GITHUB` is removed. The normal workflow token uploads source-repository
release assets; only the Linux build/publisher and Ubuntu Windows-assets
publisher have contents-write, OIDC and attestations-write. Native Windows
package generation has contents-read only; it cannot upload GitHub assets or
mint a winget token.
There is no packages-write permission. Syft scans
the archives and `actions/attest` attests every `.tar.gz`/`.zip` archive without
creating registry storage records. Every portable publisher runs on Ubuntu
with block-mode Harden-Runner and explicit service endpoints. Hosted-runner
allowlist completeness and real artifact-service transfers remain unverified
until their authorized acceptance runs.

### Accepted audit-only native jobs

The pinned
[Harden-Runner compatibility matrix](https://github.com/step-security/harden-runner/blob/v2.22.4/README.md#environment-compatibility-matrix)
supports **audit only** on GitHub-hosted Windows and macOS. Those jobs explicitly
use audit mode, not a fictitious effective block policy. This is an accepted
deviation from #98's block-mode requirement, limited to two jobs:

| Job | Runner | Does | Re-verified by |
| --- | --- | --- | --- |
| `homebrew` | macOS | `brew style --fix` on the rendered cask | `homebrew-publish` compares every stanza with the Linux cask |
| `goreleaser-windows` | Windows | builds ZIPs, SBOMs and winget YAML | `windows-publish` checks manifests against the archive SHA-256s |

`test_release_config.rb` enforces the conditions for every audit-mode job: a
macOS or Windows runner, contents-read only, no `release` environment and no
secret, App token or package credential. A job that needs a credential must run
on Ubuntu in block mode. Restoring Chocolatey would need a native `choco push`
with its API key, so it needs a new decision on #98 first.

### Why there is a separate native Homebrew job

GoReleaser 2.19.0 supports `custom_block`, but places it before `version`. Its
raw cask output is **not** Homebrew-style-clean. The Linux job therefore renders
a derived configuration changing only cask `skip_upload` to `true`. It transfers
only the generated cask, not metadata or environment files, to a stable-only
macOS job. That job runs native `brew style --fix` followed by a strict
`brew style`, then transfers **only** `azctx.rb` as a distinct normalized-cask
artifact. It has no release environment, secret references, App token or write
permissions. The ordinary contents-read checkout credential is not persisted.
Both artifact hops use same-run downloads with `digest-mismatch: error`,
one-day retention and missing-file failure; metadata/workspace directories are
never transferred.

A dedicated Ubuntu job depends only on successful native formatting. Its
allowlist covers GitHub checkout/API and Actions artifact transport, not Go,
Homebrew or package registries. It digest-checks both the original Linux cask
and the normalized cask from the same run, then runs
`publish_cask.py --check` without any token. That check parses both casks with
a restricted grammar covering only the syntax and stanzas that the current
GoReleaser template and `brew style --fix` emit, so unrecognized Ruby, such as
other quoting, interpolation, modifiers, inline comments, calls, extra stanzas
or blocks like `on_linux`, fails closed instead of being ignored. Every core
stanza must occur exactly once, each architecture's `url` must name its own
release archive with a lowercase SHA-256, and `postflight_steps` must contain
only the quarantine-removal `run`. The exact root-level `depends_on :macos`
declaration is permitted because GoReleaser 2.19 emits it for a
macOS-only cask; it may appear at most once and is part of the compared
statements, so native style can neither add nor remove it. The dependency must
be exactly `depends_on cask: "azure-cli"` or its one-element array form;
formula dependencies, other casks and extra dependencies are rejected.
Homebrew 7.0.12 also normalizes GoReleaser's no-zap comment to
`zap :not_required`; this exact root-level no-cleanup marker is accepted at
most once and is semantically equivalent to omitting cleanup. Real cleanup
stanzas, other symbols, platforms and contexts are still rejected.
All actionable statements, including their
architecture block, must then equal the original Linux artifact's; native style
may only reorder or reformat them. The four fixtures are regenerated from real
publisher-disabled GoReleaser 2.19.0 output and current native Homebrew
normalization, not hand-edited.
A new GoReleaser or Homebrew output shape therefore needs a reviewed parser
update. Only after that comparison does it mint the
single-tap contents-write App token and run the Python standard-library
publisher. Neither Homebrew nor a build tool runs alongside that token.
The publisher validates the stable tag against the cask
version, performs a SHA-based compare-and-swap, and never removes the old formula
or creates an upstream PR. A conflict or style failure aborts rather than
publishing unverified output.

### Native Windows restriction and portable publishers

The Windows job runs natively because GoReleaser's Chocolatey pipe needs native
`choco pack`/`choco push`. Chocolatey is skipped for 2026.11, so the native job
only builds; winget's fork upload and GitHub asset upload stay portable. The
installed Community GoReleaser has no independent publish/resume command;
running its combined publishing pass natively would unnecessarily expose those
portable publishers.

The Windows job therefore builds once with **`--skip=publish --skip=chocolatey`**.
It transfers only ZIP archives, their checksum file, SBOMs and real GoReleaser
winget YAML. It never pushes and receives no secret or App signing key/token.

An Ubuntu block/allowlisted job downloads those same-run files with digest
failure enabled, uploads the existing Windows assets to the Linux-created
release, and attests the ZIPs. `upload_release_assets.py` makes the upload
idempotent: it lists the release's assets first and skips a same-name asset
whose GitHub `sha256:` digest matches the local bytes. A different digest, a
missing digest or an incomplete asset fails before any upload; nothing is
overwritten (`--clobber`) or deleted. After a partial failure, rerun the
workflow; a mismatched or incomplete asset needs a reviewed manual cleanup
first. No binary is rebuilt. It verifies the three
stable winget manifests, including exact identity/version, Azure CLI floor,
archive URLs and SHA-256 against the downloaded archives, **before** minting
a fork-only token. `publish_winget.rb` then uses GitHub's existing contents API
to create/update only `azctx-<version>` in `whiteducksoftware/winget-pkgs`,
preserving raw GoReleaser manifests and SHA-based updates. It does not open an
upstream PR. RC tags neither mint the token nor run either package publisher.
Tests use only synthetic credentials and an injected/mock API transport.

Direct `goreleaser release` does not perform this normalization: use the reviewed
workflow for future publishing. No latest promotion is automated; keep
`RELEASE_MAKE_LATEST=false` until the #100 package-availability acceptance is met.

## Publisher-disabled verification

With the existing Go, GoReleaser 2.19.0, Ruby and Syft installed:

```sh
goreleaser check --config .goreleaser.yaml
goreleaser check --config .goreleaser-windows.yaml
make ci-check
ruby .github/scripts/release_snapshot.rb unix stable
ruby .github/scripts/release_snapshot.rb unix rc
HOMEBREW_NO_AUTO_UPDATE=1 brew style --fix .release-check/unix-stable/homebrew/Casks/azctx.rb
HOMEBREW_NO_AUTO_UPDATE=1 brew style .release-check/unix-stable/homebrew/Casks/azctx.rb
```

The helper uses synthetic tags through GoReleaser's environment override, never
creates a Git tag, clears publisher credentials in the child environment, disables
the release, and always passes `--snapshot --skip=publish,announce,before`.
It writes only ignored `.release-check/` outputs. The source configuration is
unchanged. RC snapshots also skip the Chocolatey pipe.

`release-check.yaml` configures stable/RC snapshots on native Linux, macOS and
Windows. Its pinned GoReleaser action uses `install-only: true` to export the
tool on `PATH` before either configuration check or the snapshot helper runs;
the action's normal `args` mode does not export `PATH` for later steps.
Only native Windows with real `choco pack` can satisfy Chocolatey
acceptance: the job requires a nonempty `.nupkg`, inspects the packaged nuspec
and runs `verify_chocolatey.py --archives` to compare its embedded checksum with
the same snapshot's local x86_64 ZIP.
The Windows job also validates the three real GoReleaser winget manifests against
Microsoft's versioned JSON schemas using PowerShell's `Test-Json`; this is schema
validation, not a native winget installation or submission test.
Run `ruby .github/scripts/release_snapshot.rb windows stable` only there.
`windows-render-only stable|rc` is an explicit cross-build alternative for
archives and real GoReleaser winget manifests; it **does not generate or validate
a Chocolatey package**. No replacement Chocolatey command or synthetic package
is used. Native Homebrew normalization/style runs on macOS. CI's existing lint
job scans all workflows using actionlint and strict zizmor with no release-debt
exceptions. A configured check is not evidence of a completed hosted-runner run.
`make ci-check` additionally exercises rejection mutations for the audit-only
job conditions, both cask artifact hops, a returning Chocolatey publisher or
package, upload idempotency and credential scopes, and the portable winget
publisher's mocked create/update/no-op/denial paths. Mocked Python tests cover
cask semantic drift and unrecognized cask syntax, Chocolatey checksum/URL
mismatch, unrecognized install-script content and extra package entries, using
exact real GoReleaser and `brew style --fix` fixtures, and identical, missing
or conflicting release assets.
It never executes a real publisher.

## Authoritative format references

- [GoReleaser casks](https://goreleaser.com/customization/homebrew_casks/),
  [Chocolatey](https://goreleaser.com/customization/chocolatey/),
  [winget](https://goreleaser.com/customization/winget/),
  [release settings](https://goreleaser.com/customization/release/)
- [Homebrew declarative steps](https://docs.brew.sh/Cask-Cookbook#stanza-flight_steps)
  and [tap trust](https://docs.brew.sh/Tap-Trust)
- [App token action](https://github.com/actions/create-github-app-token),
  [attestation action](https://github.com/actions/attest),
  [repository contents API](https://docs.github.com/en/rest/repos/contents#create-or-update-file-contents)
- [NuGet dependency version floors](https://learn.microsoft.com/nuget/concepts/package-versioning#version-ranges)
  and [PowerShell JSON schema validation](https://learn.microsoft.com/powershell/module/microsoft.powershell.utility/test-json)
