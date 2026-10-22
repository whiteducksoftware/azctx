# Release configuration (not a go-live procedure)

Layer 17 configures future releases. It does **not** create the GitHub App,
protected environment, tap migration, tag or release. Those are #100 acceptance
work, as are package availability and any later promotion to GitHub's latest
release. Do not run a publishing release to test this configuration.

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

All three release jobs use the `release` environment. Provisioning that
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
release assets; only those two archive-building jobs have contents-write,
OIDC and attestations-write. There is no packages-write permission. Syft scans
the archives and `actions/attest` attests every `.tar.gz`/`.zip` archive without
creating registry storage records. Linux starts with block-mode Harden-Runner
and explicit service endpoints, which still require observed hosted-runner
acceptance before go-live.

**Unmet egress acceptance:** the pinned
[Harden-Runner compatibility matrix](https://github.com/step-security/harden-runner/blob/v2.22.2/README.md#environment-compatibility-matrix)
supports **audit only** on GitHub-hosted Windows and macOS. Those jobs explicitly
use audit mode, not a fictitious effective block policy. Chocolatey publishing
therefore has no enforced endpoint allowlist. Full cross-platform blocking from
#98 needs a supported enforcement design and maintainer acceptance before #100;
this configuration must not be mistaken for completed egress enforcement.

### Why there is a separate native Homebrew job

GoReleaser 2.18.3 supports `custom_block`, but places it before `version`. Its
raw cask output is **not** Homebrew-style-clean. The Linux job therefore renders
a derived configuration changing only cask `skip_upload` to `true`. It transfers
only the generated cask, not metadata or environment files, to a stable-only
macOS job. That job runs native `brew style --fix` followed by a strict
`brew style`, then obtains the tap token and uploads the normalized file through
GitHub's contents API. The publisher validates the stable tag against the cask
version, performs a SHA-based compare-and-swap, and never removes the old formula
or creates an upstream PR. A conflict or style failure aborts rather than
publishing unverified output.

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
Windows. Only native Windows with real `choco pack` can satisfy Chocolatey
acceptance: the job requires a nonempty `.nupkg` and inspects the packaged nuspec.
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
