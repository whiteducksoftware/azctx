# Upgrading to azctx 2026.11

This guide tracks breaking changes from 2024.12.5. Later release layers add
their changes to the table below.

## Breaking changes

| Area | Change | Who is affected | What to do |
| --- | --- | --- | --- |
| Windows builds | The 32-bit `windows/arm` build is removed; Windows on ARM64 remains supported. | Users of the 32-bit Windows ARM binary. | Move to a supported 64-bit Windows system and use the `windows/arm64` or `windows/amd64` binary. |
| macOS | macOS 13 or later is required by Go 1.27. | Users running macOS 12 or earlier. | Upgrade to macOS 13 or later before installing azctx 2026.11. |
| Windows | Windows 10 or later is required by Go 1.27. | Users running older versions of Windows. | Upgrade to Windows 10 or later before installing azctx 2026.11. |
| Source builds | The minimum Go version is 1.26; the preferred toolchain is Go 1.27.2. | Contributors and users building azctx from source. | Use the Go version and toolchain declared in `go.mod`. |

## Upgrade checklist

- Check the breaking changes above before upgrading.
- Choose a binary for your operating system and architecture: Linux
  (`amd64`, `arm64`, `arm`), macOS (`amd64`, `arm64`), or Windows (`amd64`, `arm64`).
- Follow the [installation instructions](../README.md#installation) for your
  package manager or binary installation.

## Contributor and maintenance changes

| Area | Change | Who is affected | What to do |
| --- | --- | --- | --- |
| CI checks | Every pull request (including stacked branches) and push to `main` runs race-enabled tests on Linux, macOS and Windows, Go/workflow linting, module verification and vulnerability scanning; Linux and Windows snapshot builds never publish. | Contributors. | Run `make check` before opening a PR and resolve failing checks. Only `GO-2026-5970` and `GO-2026-6629` in `golang.org/x/text` v0.21.0 temporarily warn until the layer 05 dependency upgrade (#86); other reachable findings and scanner errors fail. |
| Dependency updates | Go module and GitHub Actions minor/patch updates are grouped per ecosystem, with a three-day cooldown and at most one open version-update PR per ecosystem. | Maintainers. | Review the weekly grouped updates; major updates remain separate and share the same version-update PR limit. Security updates are not delayed by the cooldown or this limit. |
| Security posture | OpenSSF Scorecard runs on pushes to `main` and weekly, uploading SARIF to GitHub code scanning without publishing externally. Existing CodeQL remains enabled. | Maintainers. | Review the security results. Release-workflow credential-persistence and broad-permission findings are explicitly deferred to layer 17 (#98); all other workflow audits remain active. |
