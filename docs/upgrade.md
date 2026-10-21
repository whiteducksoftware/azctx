# Upgrading to azctx 2026.11

This guide tracks breaking changes from 2024.12.5. Later release layers add
their changes to the table below.

## Breaking changes

| Area | Change | Who is affected | What to do |
| --- | --- | --- | --- |
| Windows builds | The 32-bit `windows/arm` build is removed; Windows on ARM64 remains supported. | Users of the 32-bit Windows ARM binary. | Move to a supported 64-bit Windows system and use the `windows/arm64` or `windows/amd64` binary. |
| macOS | macOS 13 or later is required by Go 1.27. | Users running macOS 12 or earlier. | Upgrade to macOS 13 or later before installing azctx 2026.11. |
| Windows | Windows 10 or later is required by Go 1.27. | Users running older versions of Windows. | Upgrade to Windows 10 or later before installing azctx 2026.11. |
| Source builds | The minimum Go version is 1.26; the preferred toolchain is Go 1.27.3. | Contributors and users building azctx from source. | Use the Go version and toolchain declared in `go.mod`; allow automatic toolchain selection (the default) rather than setting `GOTOOLCHAIN=local` with an older Go installation. |
| Go security patch | Go 1.27.3 fixes [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) in `crypto/x509.Certificate.Verify`, used by the update check. | Contributors and users building azctx from source. | The preferred toolchain advances from 1.27.2 in the CI layer rather than waiting for a later dependency layer, because the newly published reachable standard-library issue needs a security patch, not a vulnerability waiver. The Go 1.26 source floor is unchanged; Go 1.26 users must use at least 1.26.10 if opting out of automatic toolchain selection. |
| Diagnostic output | Logrus-style timestamps and `INFO[0000]`/`level=info` output are removed. Messages, warnings, login progress and update notices go to stderr as plain text; `azctx -c` data goes to stdout. | Scripts parsing logs or redirecting current-subscription output. | Consume stdout for data and stderr for diagnostics; do not parse logging prefixes. |
| Picker | The picker now uses a fuzzy-filterable Bubble Tea list with name, subscription ID and tenant columns. Narrow terminals drop subscription ID, then tenant; the terminal bell is never used. | Interactive users. | Press `/` to filter, arrows to navigate and Enter to confirm the filter, then select. |
| Picker interruption | Ctrl+C exits with status 130, including when choosing among multiple name matches. | Interactive users and shell scripts. | Treat 130 as interruption rather than successful selection. |
| Login progress | Animated spinners and forced color formatting are removed. | Interactive login users and scripts. | Read the plain progress messages on stderr; Azure login behavior is otherwise unchanged. |
| Version formats | `azctx version` defaults to text and accepts only `-o text` or `-o json`; `-o short`, `-o yaml` and `-o pretty` are removed. The command no longer fetches an upgrade notice. | Users and scripts invoking `version`. | Use text for humans or JSON for automation. JSON reports `version`, `buildDate`, `commit` and `goVersion`; unavailable build metadata is `unknown`. |
| Subscription matching | Names, tenant names and subscription IDs are searched without collapsing equally named subscriptions. Multiple matches show a picker with tenants; sorting is deterministic. | Users with duplicate subscription names or multiple tenants. | Use a unique name, tenant name or ID with `azctx -- QUERY`, or filter the picker by those fields. All subscription IDs passed to Azure CLI must be valid GUIDs. |
| Previous subscription | `azctx -` now swaps the last two subscriptions across processes using private `os.UserConfigDir()/azctx/state.json`. | Users switching repeatedly between subscriptions. | Switch once before using `-`. Keep azctx's user configuration outside `AZURE_CONFIG_DIR`; state files use `0600` permissions and their directory uses `0700` on Unix. |
| Update checks | Stable CalVer checks use the latest-release redirect at most daily, cache attempts/results in `os.UserCacheDir()/azctx/update.json`, and have a two-second network deadline. Failures never change a successful switch's exit status. | Interactive users, offline users and automation. | Set `AZCTX_NO_UPDATE_CHECK=1` to opt out. Checks are also skipped when stdout is not a TTY or `CI` is set. Keep the private cache outside `AZURE_CONFIG_DIR`. |
| Update cache recovery | Malformed update caches are refreshed without failing a successful switch; their JSON error is reported only with `--verbose`. Missing caches remain quiet. | Users troubleshooting update checks. | Use `--verbose` to see malformed-cache diagnostics; no manual cache removal is needed. |
| Login streams | `az login` is attached to stdin/stdout/stderr, with stderr teed into a diagnostic buffer and no spinner. | Device-code login users over SSH, in devcontainers and in Codespaces. | Device codes are shown immediately on the stream Azure CLI uses; `azctx login -- --use-device-code` no longer hides them. |
| Quiet defaults and errors | Unset `AZURE_CONFIG_DIR` silently defaults to `~/.azure`; missing tenant-cache hints and routine switch messages are removed. Picker errors now surface, missing-profile errors recommend `az login`, and wrapped causes are retained. | Interactive users and scripts inspecting errors. | Use `--verbose` for debug diagnostics, including nonfatal update failures. Ctrl+C remains exit 130. |

## Upgrade checklist

- Check the breaking changes above before upgrading.
- Choose a binary for your operating system and architecture: Linux
  (`amd64`, `arm64`, `arm`), macOS (`amd64`, `arm64`), or Windows (`amd64`, `arm64`).
- Follow the [installation instructions](../README.md#installation) for your
  package manager or binary installation.

## Contributor and maintenance changes

| Area | Change | Who is affected | What to do |
| --- | --- | --- | --- |
| CI checks | Every pull request (including stacked branches) and push to `main` runs race-enabled tests on Linux, macOS and Windows, Go/workflow linting, module verification and vulnerability scanning; Linux and Windows snapshot builds never publish. | Contributors. | Run `make check` before opening a PR and resolve failing checks. All reachable vulnerability findings and scanner errors fail; the layer 02 temporary x/text exceptions have been removed after the dependency upgrade. |
| Dependency and build metadata | Legacy UI, logging, filesystem and version dependencies are replaced with Bubble Tea/Bubbles/Lip Gloss, sahilm/fuzzy, x/term and the standard library. Both release configs now set `internal/version.Version`, `BuildDate` and `Commit` under the module import path. | Contributors and custom binary builders. | Use the new ldflags when building release binaries; Go build information supplies fallback metadata. Run `make golden` for intentional picker screen updates. The Go 1.26 language floor and preferred Go 1.27.3 toolchain are unchanged. |
| Dependency updates | Go module and GitHub Actions minor/patch updates are grouped per ecosystem, with a three-day cooldown and at most one open version-update PR per ecosystem. | Maintainers. | Review the weekly grouped updates; major updates remain separate and share the same version-update PR limit. Security updates are not delayed by the cooldown or this limit. |
| Security posture | OpenSSF Scorecard runs on pushes to `main` and weekly, uploading SARIF to GitHub code scanning without publishing externally. Existing CodeQL remains enabled. | Maintainers. | Review the security results. Release-workflow credential-persistence, broad-permission and setup-go cache findings are precisely location-excepted until layer 17 (#98); snapshot jobs disable setup-go caching and all other workflow audits remain active. |
