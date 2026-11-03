# Release configuration (not a go-live procedure)

Layer 17 configures future releases. It does **not** create the GitHub App,
protected environment, tap migration, tag or release. Those are #100 acceptance
work, as are package availability and any later promotion to GitHub's latest
release. Do not run a publishing release to test this configuration.

**RELEASE READINESS: BLOCKED.** #98's native-runner block-egress requirement is
not satisfied. The tag workflow deliberately fails its credential-free
`release-readiness` job before **any** publisher can run. This includes RC
releases. Offline policy tests passing means this boundary is preserved, not
that the full release specification passed. Removing the blocker requires a
reviewed, supported enforcement design and actual native/hosted acceptance;
an environment variable or maintainer approval alone cannot bypass it.

## Packaging contracts

- Linux ships only the existing `azctx_Linux_*.tar.gz` archives. macOS keeps
  `azctx_Darwin_*.tar.gz`; Windows keeps `azctx_Windows_*.zip`.
- The macOS-only cask targets `whiteducksoftware/homebrew-tap/Casks/azctx.rb`,
  depends on the `azure-cli` formula and installs `azctx`. The binaries remain
  unsigned/not notarized. A declarative `postflight_steps` `run` invokes
  `/usr/bin/xattr -dr com.apple.quarantine` on the staged binary; no deprecated
  Ruby `postflight` hook is used. Tap trust and formula-to-cask migration are
  separate #100 work, not performed by this workflow.
- Chocolatey authors/owners are white duck GmbH and its `azure-cli` dependency
  has version floor 2.61.0. winget retains `whiteducksoftware.azctx` and requires
  `Microsoft.AzureCLI` >=2.61.0. Both copyright years use the release date.
  winget updates only the version branch in the `whiteducksoftware/winget-pkgs`
  fork; opening the upstream PR remains manual.
- Both configurations classify prereleases automatically. Cask and winget
  `skip_upload: auto` protect direct GoReleaser use; the workflow additionally
  excludes every hyphenated prerelease tag from the tap job and skips the
  Chocolatey pipe. Snapshots never exercise a real publisher.
- Conventional commit types form changelog groups, with breaking changes first.
  Mark breaking subjects with `!:` (for example `feat(auth)!: ...`):
  GoReleaser 2.18.3 groups commit **subjects**, not `BREAKING CHANGE` body footers.
  Static builds retain `CGO_ENABLED=0`, `-trimpath`, `.CommitTimestamp` and the
  commit-derived binary `BuildDate`.

## Protected future workflow

All publishing jobs use the `release` environment; the native macOS formatter
and the fail-closed readiness job do not. Provisioning that
environment, required reviewers and an App installation is deliberately deferred
to #100. Its expected interface is:

| Name | Kind | Purpose |
| --- | --- | --- |
| `RELEASE_APP_CLIENT_ID` | Environment variable | GitHub App client ID |
| `RELEASE_APP_PRIVATE_KEY` | Environment secret | App signing key |
| `CHOCO_API_KEY` | Environment secret | Chocolatey publishing only, not prereleases |
| `RELEASE_MAKE_LATEST` | Environment variable | Literal `true` or `false`; defaults to `false` |

Short-lived, automatically revoked App tokens request contents-write on only
`homebrew-tap` or only `winget-pkgs`, never the source repository or both taps.
`AUTH_GITHUB` is removed. The normal workflow token uploads source-repository
release assets; only the Linux build/publisher and Ubuntu Windows-assets
publisher have contents-write, OIDC and attestations-write. Native Windows
package generation and the final native Chocolatey publisher have contents-read
only; they cannot upload GitHub assets or mint a winget token.
There is no packages-write permission. Syft scans
the archives and `actions/attest` attests every `.tar.gz`/`.zip` archive without
creating registry storage records. Every portable publisher runs on Ubuntu
with block-mode Harden-Runner and explicit service endpoints. Hosted-runner
allowlist completeness and real artifact-service transfers remain unverified
until their authorized acceptance runs.

**Unmet egress acceptance:** the pinned
[Harden-Runner compatibility matrix](https://github.com/step-security/harden-runner/blob/v2.22.2/README.md#environment-compatibility-matrix)
supports **audit only** on GitHub-hosted Windows and macOS. Those jobs explicitly
use audit mode, not a fictitious effective block policy. Native Homebrew style
verification remains audit-only but has no publisher credentials. Chocolatey
push is the only remaining native publisher and has **no enforced endpoint
allowlist**. This is a **pre-release blocker**, not an accepted spec waiver.
Full native enforcement from #98 needs reviewed external runner/network
infrastructure or newly supported vendor enforcement, plus actual acceptance
before #100. None is provisioned here; the readiness gate must remain failing
until that requirement is met. No unsupported `block` setting, local firewall
hack, downloaded runner or alternative publishing platform is substituted.

### Why there is a separate native Homebrew job

GoReleaser 2.18.3 supports `custom_block`, but places it before `version`. Its
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
or blocks like `on_linux`, fails closed instead of being ignored. Every stanza
must occur exactly once, each architecture's `url` must name its own release
archive with a lowercase SHA-256, and `postflight_steps` must contain only the
quarantine-removal `run`. All statements, including their architecture block,
must then equal the original Linux artifact's; native style may only reorder or
reformat them. A new GoReleaser or Homebrew output shape therefore needs a
reviewed parser update. Only after that comparison does it mint the
single-tap contents-write App token and run the Python standard-library
publisher. Neither Homebrew nor a build tool runs alongside that token.
The publisher validates the stable tag against the cask
version, performs a SHA-based compare-and-swap, and never removes the old formula
or creates an upstream PR. A conflict or style failure aborts rather than
publishing unverified output.

### Native Windows restriction and portable publishers

The pinned [Chocolatey pipe](https://github.com/goreleaser/goreleaser/blob/v2.18.3/internal/pipe/chocolatey/chocolatey.go)
executes `choco pack` during generation and `choco push` during publishing.
These are native Windows tooling requirements, not requirements of winget's
GitHub fork upload or GitHub release asset upload. The installed Community
GoReleaser has no independent publish/resume command; running its combined
publishing pass natively would unnecessarily expose those portable publishers.

The Windows job therefore builds once with **`--skip=publish`** (also
`--skip=chocolatey` on RC tags). It transfers only ZIP archives, their checksum
file, SBOMs and real GoReleaser winget YAML. On stable tags it also transfers
the same-run `azctx.*.nupkg` as a separate `chocolatey-package` artifact. It
never pushes and receives no Chocolatey secret or App signing key/token.

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

A final, stable-only native `chocolatey-publish` job needs a successful
`windows-publish`, so it never runs after a failed asset upload or for RC tags.
It has contents-read only and downloads only the digest-checked same-run
package. `verify_chocolatey.py` checks the package id/version against the tag
and requires the package to contain only the nuspec, NuGet metadata and
`tools/chocolateyinstall.ps1`; any other script, helper or binary is rejected.
That script must match GoReleaser 2.18.3's rendered template exactly, apart
from this release's `azctx_Windows_x86_64.zip` URL and a SHA-256 checksum, so
other assignments or commands fail closed. It then downloads that
exact public URL and compares the checksum with the uploaded asset's bytes.
Only its final `choco push` step receives `CHOCO_API_KEY`, and only after that
comparison passes. GoReleaser 2.18.3's Chocolatey package embeds only
amd64/386 archives, so the arm64 ZIP is not part of the package.

The remaining unavoidable native `choco push` cannot satisfy #98 enforcement
on the current hosted Windows runner. Both the workflow blocker and policy
regressions explicitly retain that failure. A passing snapshot, schema check
or offline publisher test is not evidence of native network enforcement.

Direct `goreleaser release` does not perform this normalization: use the reviewed
workflow for future publishing. No latest promotion is automated; keep
`RELEASE_MAKE_LATEST=false` until the #100 package-availability acceptance is met.

## Publisher-disabled verification

With the existing Go, GoReleaser 2.18.3, Ruby and Syft installed:

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
`make ci-check` additionally exercises the actual readiness script (expecting
exit 1), rejection mutations for both cask artifact hops, the Chocolatey
package hop, upload idempotency and credential scopes, and the portable winget
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
