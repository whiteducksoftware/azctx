# Security boundaries and verification

This documents the unreleased 2026.11 source, not a security certification or
go-live approval. Report vulnerabilities privately as described in
[SECURITY.md](../SECURITY.md). Preserve [LICENSE](../LICENSE) and all legally
required attribution when redistributing software or documentation.

## What azctx reads

Inside the Azure root (`AZURE_CONFIG_DIR`, default `~/.azure`), azctx reads only
`azureProfile.json`, INI `config` and `clouds.config`. Doctor may **stat**, never
open, a leftover `azctxTenants.json`. No credentials, passwords, MSAL caches or
arbitrary Azure files are read. Tenant display metadata comes from the profile,
not a tenant REST API or azctx tenant cache.

Outside that root, azctx reads its strict user configuration, previous/recent
state, bounded history and update cache as required by each command.
Guard reads the nearest `.azctx.yaml` as an untrusted expectation, never as a
command. Completion reads profile/settings and configuration only.
See [configuration](configuration.md) for exact schemas, filenames and OS paths.

## What changes

azctx never **directly** writes Azure files. Switching delegates to
`az account set --subscription GUID`; login delegates to `az login`.
The sole persisted **configuration-setting** exception is explicitly confirmed
`azctx auth broker enable` or `azctx auth broker disable`:

1. Azure CLI clears cached accounts with `az account clear`.
2. Azure CLI persists the platform broker boolean with `az config set`.
3. Azure CLI signs in again with attached `az login`.

Unsupported platforms/versions, conflicting environment overrides and
unconfirmed non-interactive runs cannot mutate. The first failed step stops
the sequence; a persisted setting is not automatically rolled back after a
failed login. See [authentication](authentication.md#inspect-or-change-the-broker).
This exception must not be described as unrestricted Azure config access.

Private writes include alias edits to `config.yaml`, previous/recent
`state.json`, best-effort bounded `history.json`, and optional update cache
and lock files. Shared atomic writes use `0600` files and `0700` directories
on Unix; existing permissions are tightened where the writer requires it.
Windows ACLs follow the user's filesystem policy, not a POSIX-mode guarantee.
Paths inside either resolved or environment Azure configuration are rejected,
including existing symlink targets.
`env --format github` is not a private azctx write: it appends in place to the
runner-owned `GITHUB_ENV` through one no-follow `O_APPEND` handle (Windows:
append-only, reparse points opened rather than followed, no delete sharing),
preserving its inode, owner and mode and leaving its parent untouched. It
re-checks after opening that the handle is the validated single-link regular
file outside Azure configuration, and reports open, partial-write and close
errors. No tokens are exported.

The private history records UTC completion time, previous/new subscription and
token tenant GUIDs, cloud and terminal/script source, never names, queries or
process output. Busy-lock/write failures warn without undoing a completed
switch, so history is not an authoritative audit ledger.

## Network and subprocesses

| Operation | Boundary |
| --- | --- |
| Switch | `az account set`; Azure CLI owns any behavior beyond the local runner |
| Login | Attached `az login`; explicit all-tenant discovery uses `az account tenant list -o json` |
| Broker change | Confirmed clear/config/login sequence above |
| Doctor | Only local `az version -o json`, never Azure or an update request |
| Optional updates | After an eligible successful switch, an HTTPS request to GitHub's latest-release redirect, at most daily, two-second deadline |
| fzf picker | Optional resolved local fzf with sanitized rows and `FZF_*` removed |
| Exec | One explicitly requested executable with attached raw streams; its network/filesystem operations are its own |

Update requests use Go's normal HTTP/TLS and redirect/proxy behavior. They are
not an Azure endpoint allowlist or a sandbox. Disable them with
`AZCTX_NO_UPDATE_CHECK=1` or `updateCheck: false`. They are also disabled with
`CI` present, non-TTY stdout or JSON switches. Failed updates never fail a
completed switch. Doctor reads cached status only.

Derived tenant/subscription IDs must be GUIDs before becoming Azure arguments.
Only the user's literal login arguments after `--` bypass that GUID rule so
Azure CLI can interpret authentication options. Never place secrets in argv:
shell history, process inspection and the requested child are outside azctx's
control.

Azure CLI and exec executables are resolved once through `exec.LookPath` to an
absolute path. Go's current-directory executable refusal remains enabled;
azctx does not install Azure CLI, account extensions or fzf.
Cancellation forwards interrupts to the direct child with a two-second kill
fallback. Windows uses a console process group and Ctrl+Break. There is no
promise of killing arbitrary descendant processes.

## Redaction is heuristic

Verbose output, doctor string diagnostics, errors and streamed Azure CLI
stderr replace recognized token-shaped values with `[REDACTED]`. Filtering
recognizes JWT-like values, provider prefixes, long opaque words and labelled
token/secret/password/Bearer values, including split writes.
It is not proof that arbitrary unlabelled secrets will be found. Do not use it
as permission to collect private logs.

**Azure CLI stdout is a raw data stream. Explicit exec children keep all their
streams unchanged**, even under JSON output. Device-code instructions remain
visible; protect them like authentication material. Doctor omits profile
identities and raw subprocess diagnostics, but reported filesystem paths and
cloud names can still be sensitive. Review every report before sharing.

Guard, aliases, hidden/protected entries and agent hooks reduce mistakes.
They do not grant or deny Azure access, bind every tool to a target or lock the
shared Azure default. Keep Azure RBAC, PIM and Conditional Access enforced.
Use explicit `--subscription` on az resource commands, including inside exec.

## Verify a release

**2026.11 is not published by this layer.** These commands apply only after an
authorized release actually exists. Obtain the archive and matching checksum
file from the same explicit tag in
[whiteducksoftware/azctx](https://github.com/whiteducksoftware/azctx/releases).
Confirm the tag, repository, expected archive and trusted signer/workflow
identity through an independent channel. Never treat a development snapshot
or a missing attestation as verified provenance.

Linux amd64 example:

```sh
set -eu
gh release download 2026.11.0 --repo whiteducksoftware/azctx \
  --pattern azctx_Linux_x86_64.tar.gz \
  --pattern azctx_2026.11.0_Darwin_Linux_checksums.txt
grep '  azctx_Linux_x86_64.tar.gz$' azctx_2026.11.0_Darwin_Linux_checksums.txt | sha256sum --check --strict -
gh attestation verify azctx_Linux_x86_64.tar.gz --repo whiteducksoftware/azctx
```

For macOS arm64, use `azctx_Darwin_arm64.tar.gz` from that tag and the same
Darwin/Linux checksum file:

```sh
set -eu
grep '  azctx_Darwin_arm64.tar.gz$' azctx_2026.11.0_Darwin_Linux_checksums.txt | shasum -a 256 --check -
gh attestation verify azctx_Darwin_arm64.tar.gz --repo whiteducksoftware/azctx
```

Download both files first. For Windows amd64 use `azctx_Windows_x86_64.zip`
and `azctx_2026.11.0_Windows_checksums.txt`. In PowerShell:

```powershell
$archive = 'azctx_Windows_x86_64.zip'
$lines = @(Get-Content 'azctx_2026.11.0_Windows_checksums.txt' | Where-Object { $_ -match '^[0-9a-fA-F]{64}  azctx_Windows_x86_64\.zip$' })
if ($lines.Count -ne 1) { throw 'Expected exactly one archive checksum' }
$expected = $lines[0].Substring(0, 64)
if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne $expected) { throw 'Checksum mismatch' }
gh attestation verify $archive --repo whiteducksoftware/azctx
if ($LASTEXITCODE -ne 0) { throw 'Attestation verification failed' }
```

Require successful hash **and** provenance verification before extraction.
A checksum detects changed bytes, but a checksum served beside a compromised
archive does not independently prove origin. GitHub attestation verification
binds bytes to a repository/workflow identity; review its reported identity and
ref, not just success. It does not prove the source has no vulnerabilities.
See [GitHub artifact attestation verification](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/verifying-the-provenance-of-binaries).

## Unsigned macOS binaries

macOS binaries are unsigned and not notarized. A GitHub attestation is **not**
Developer ID signing or Apple notarization. The future cask declares removal
of `com.apple.quarantine` on its staged binary; this bypasses that quarantine
check, not a trust review. Manual archive users may need the narrow command in
[troubleshooting](troubleshooting.md#macos-quarantine), only after verification.
Do not globally disable Gatekeeper.

## Release infrastructure is not accepted yet

Portable publishers are configured for Ubuntu block-mode endpoint enforcement.
Pinned Harden-Runner on hosted Windows/macOS supports audit only, including
native Chocolatey push and native Homebrew style verification. The unmet
native enforcement requirement is an explicit **release blocker**; the
credential-free readiness job fails before all publishers, including RCs.
Passing package/schema checks does not remove it.

No App installation, environment reviewer settings, secret rotation, live
attestation, package availability or native network enforcement is established
by these docs. The upstream winget PR remains manual.
See [release configuration](releasing.md) for the exact boundary and required
reviewed enforcement/hosted acceptance before go-live.
