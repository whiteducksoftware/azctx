# Troubleshooting

This guide describes the unreleased 2026.11 contract. Start with offline
diagnostics, not a new login or a destructive cache reset:

```sh
azctx doctor
azctx doctor -o json
azctx version -o json
```

Doctor works without Azure CLI or a profile. It runs only `az version` and
reports findings rather than failing for a missing dependency/profile.
Invalid flags, output failures and cancellation still fail normally.
The [doctor schema](authentication.md#doctor) is distinct from subscription
JSON. It omits identities and raw subprocess errors, but local paths/cloud
names may be private. Review the report before sharing it.
Its update status is cached, never a live network freshness claim.

## Missing profile or older Azure CLI

```sh
azctx login
azctx login --use-device-code
```

Login requires Azure CLI >=2.61.0 and can start without an Azure directory.
Only Azure CLI creates its profile. List/current/switch require a readable
profile and suggest login when it is missing. Malformed settings are errors,
not silently replaced defaults. Upgrade Azure CLI through its supported
installation channel rather than editing cached identities.
macOS broker controls additionally require >=2.91.0 and enrolled-device
prerequisites; browser/device code remain alternatives.

`--no-input` does not make Azure CLI authentication unattended. For automation,
configure reviewed workload identity as described in [CI/CD](ci-cd.md).
MFA/Conditional Access failures are not permission to bypass tenant policy.

## Leftover tenant file

Doctor reports whether `azctxTenants.json` exists in the Azure directory and
provides a platform-quoted `deleteCommand`. It only stats that file: it never
reads, repairs or removes it. Review and run the suggested command yourself if
cleanup is desired. Do not remove `azureProfile.json`, `config`, `clouds.config`
or any credential cache as a substitute.

Tenant names now come from `tenantDisplayName` and `tenantDefaultDomain` in the
Azure CLI profile. A human-output login hint appears only for displayed
records missing both fields. Run an ordinary Azure login to refresh metadata;
azctx never calls the tenants REST API to fill gaps.

## Cloud mismatch and missing current context

```sh
azctx list --all
azctx -c -o json
```

Active cloud comes from `AZURE_CLOUD_NAME`, otherwise `[cloud] name` in Azure
CLI config, with `AzureCloud` for absent/blank settings. Subscription
`environmentName` must match exactly; missing/foreign clouds are not guessed.
`list --all` inspects every record but cannot make foreign or disabled accounts
selectable. Configure the intended cloud through Azure CLI and authenticate
there deliberately, not by editing profile JSON.

A unique active-cloud profile default wins; only if absent can its remembered
GUID in `clouds.config` identify current context. Stale remembered IDs or
multiple defaults are not silently resolved. A current subscription hidden by
user configuration is still reported; Azure cloud/state exclusions still apply.

## Ambiguous names, hidden targets and protection

```sh
azctx list -o json
azctx aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/11111111-1111-1111-1111-111111111111
azctx -- list
```

Use exact GUIDs or qualified queries for duplicate names. Aliases and exact
names can collide; ranking/favorites never choose an ambiguous identity for
you. Exit 2 means no match or ambiguity, not a request to choose the first row.
Use `--all` for configured hidden candidates; it never overrides Azure state
or cloud safety. Names equal to commands require `--`.
Protected switches need explicit confirmation or deliberate `--yes`; automation
must not add that approval merely to make a command succeed.

Previous state exists only after a completed azctx switch. Missing state exits
1; an unavailable previous target exits 2. Failed switches roll private
previous/recent state back. Do not copy state into the Azure directory.

## Guard mismatch

```sh
azctx guard -o json
azctx config path
```

Stop on mismatch exit 4 and every other nonzero result. Review both tenant and
subscription expectations; never auto-switch or rewrite `.azctx.yaml`.
A malformed nearer project file is an error, not permission to use an ancestor.
Guard is a point-in-time check, not a lock or validation of another tool's
explicit target. Pass `--subscription` to az even inside exec.
See [configuration](configuration.md#project-context).

## Private files, locks and fzf

`AZCTX_CONFIG_DIR` overrides private config/state/history only. The update
cache uses a separate OS cache directory. Every private destination must stay
outside Azure configuration, including symlink targets.
Existing invalid YAML is an error; missing YAML uses defaults.
Alias edits preserve settings, not YAML comments.

History is best-effort: corrupt files warn and a later completed switch
replaces them; busy/write failures warn but do not undo a successful switch.
Update failures also never fail a completed switch. Do not infer that a
retained lock file is a live lock or manually delete it to break OS locking.

With `picker: fzf`, missing fzf falls back once to the built-in picker.
Other failures propagate. fzf execute/preview bindings and all `FZF_*`
defaults are intentionally ignored, not a configuration bug.

## macOS quarantine

Future macOS binaries are unsigned and not notarized. First verify the exact
archive hash and GitHub attestation using [security](security.md#verify-a-release).
After extracting a reviewed archive and deliberately installing that binary,
remove only its quarantine attribute if macOS blocks it:

```sh
xattr -d com.apple.quarantine "$HOME/.local/bin/azctx"
```

Use the actual reviewed binary path. This is an explicit local security
decision, not an authenticity check; do not disable Gatekeeper globally.
The future cask declares a narrow staged-binary quarantine-removal step.
No cask install, signing/notarization or Gatekeeper acceptance was performed
for this layer. Do not uninstall a working formula until the replacement
cask is actually published and verified.

## Logs and cancellation

Keep data on stdout and diagnostics on stderr. JSON errors are one object on
stderr; successful switches and matching guards are silent. Explicit history
recovery/write warnings can still appear after a successful JSON switch.
Use `--verbose` in text mode, not by parsing log prefixes.

Token-shaped stderr/diagnostics are heuristically redacted; arbitrary secrets
may be missed. Azure CLI stdout and explicit exec streams remain raw.
Never paste raw authentication output into an issue.
Ctrl+C is exit 130; attached children receive interruption with a bounded kill
fallback, not arbitrary descendant-process cleanup. See
[the executable and diagnostic contract](cli-contract.md#executable-and-diagnostic-boundaries).
