# Authentication and diagnostics (draft)

This layer-09 guide covers broker-aware sign-in and offline diagnostics. The
multi-tenant MFA redesign is not included; layer 18 finalizes this guide.

## Sign in

```sh
azctx login
azctx login --use-device-code
azctx login --tenant aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa --use-device-code
azctx login --tenant 'Example Directory'
azctx login -- --allow-no-subscriptions
```

Azure CLI **2.61.0 or newer** is checked before sign-in. A missing Azure directory
or profile is fine: only Azure CLI creates them, and azctx reloads the resulting
profile after login. Malformed existing settings still fail explicitly.

`--tenant` accepts a validated GUID, even on first login. Other selectors match
case-insensitive fuzzy profile tenant display names/default domains and IDs,
deduplicated by tenant ID within the active cloud. Tenant-level accounts are
included. Unknown or ambiguous selectors fail without signing in; use a GUID
when there is no profile, or to disambiguate. Names are never sent to a tenant
API. Do not combine the friendly `--tenant` with `--force-mfa` or a second
passthrough tenant flag. Other Azure CLI arguments still go after `--`.

Login progress describes the effective setting, including `AZURE_CORE_*`
environment overrides:

| Platform/path | Progress and prerequisites |
| --- | --- |
| Windows | WAM account picker by default (`core.enable_broker_on_windows=true`). WAM requires Windows 10+/Server 2019+ and supports Windows Hello/FIDO/Conditional Access. Setting false uses browser sign-in. |
| macOS | Browser by default (`core.enable_broker_on_mac=false`). Azure CLI 2.91.0+ offers an opt-in **preview** broker account picker. It requires an enrolled device, Company Portal and the Microsoft Enterprise SSO plug-in. Without a compatible installed broker, Azure CLI may fall back to the browser. |
| Linux | Browser or device code; no platform broker setting. |
| Explicit `--use-device-code` | Follow the device-code instructions that Azure CLI streams immediately, on either stdout or stderr. No promise of a local browser or account picker. |

A present `SSH_CONNECTION`, `CODESPACES` or `REMOTE_CONTAINERS`, or a Linux
session with neither `DISPLAY` nor `WAYLAND_DISPLAY`, produces a
`--use-device-code` suggestion **before** login starts. The hint does not
silently change authentication mode. An explicitly requested device-code flow
does not repeat the suggestion.

Sign-in remains attached to stdin/stdout/stderr. `--no-input`, `AZCTX_NO_INPUT=1`,
redirected stdin and `CI` suppress **azctx-owned** prompts, not Azure CLI's
authentication interaction. `AZURE_CORE_LOGIN_EXPERIENCE_V2=off` applies only to
the login child, never the caller or the persisted configuration.

## Inspect or change the broker

```sh
azctx auth broker status
azctx auth broker enable
azctx auth broker disable
azctx auth broker enable --yes --no-input
```

| Platform and Azure CLI | Status | Enable/disable |
| --- | --- | --- |
| Windows >=2.61.0 | Effective setting, enabled by default | Supported |
| Windows <2.61.0 | Setting shown, unavailable | Rejected before changes |
| macOS >=2.91.0 | Effective setting, disabled by default; preview/enrollment warning | Supported, with preview/enrollment warning |
| macOS <2.91.0 | Setting shown, unavailable; upgrade hint | Both rejected before changes |
| Linux/other | Unsupported, use browser/device code | Rejected before any Azure subprocess |
| Missing/malformed Azure CLI version | Error, availability unknown | Rejected before changes |

Status needs no profile and never changes accounts/settings. Capability means
platform/version support, **not** verified enrollment, broker installation or
successful authentication. Settings are read through the shared INI reader;
environment overrides win over the file. Invalid authentication booleans fail
without echoing their value.

Enabling/disabling always performs a persisted operation, even if the effective
mode already matches (for example because of an environment override). The
confirmation explains the exact setting, account clearing and required new
sign-in. A conflicting `AZURE_CORE_ENABLE_BROKER_ON_WINDOWS` or
`AZURE_CORE_ENABLE_BROKER_ON_MAC` override is rejected **before** clearing
accounts; unset it and retry. A matching override is permitted.

| Input/confirmation | Result |
| --- | --- |
| Interactive TTY and `y`/`yes` (case-insensitive) | Proceed |
| Interactive empty answer/anything else | Cancel successfully, no changes |
| EOF/read failure or Ctrl+C | Stop, no changes; interruption is exit 130 |
| Non-TTY, `CI` present (even empty), `--no-input` or `AZCTX_NO_INPUT=1` without `--yes` | Fail with an explicit-confirmation hint, no changes |
| Explicit `--yes` | Confirmation granted without an azctx prompt; Azure sign-in is still attached and may require interaction |

After the read-only version check and confirmation, the **exact sequence** is:

1. `az account clear`
2. `az config set core.enable_broker_on_windows=true|false` on Windows, or
   `az config set core.enable_broker_on_mac=true|false` on macOS
3. `az login`, with attached streams and the child-only picker override

Only Azure CLI touches its accounts/configuration. Clearing the subscription
cache is not the same as signing out, but Azure CLI commands need a new login
after it. The first failed step stops the sequence; no unsafe automatic
rollback is attempted. If config persistence succeeded but login failed, the
setting stays persisted and the diagnostic tells you to retry `azctx login`.
The setting cannot be a one-child environment-only toggle: subsequent `az`
calls need the same broker mode.

**Safety boundary:** azctx never accesses credentials or MSAL token caches and
never directly writes any Azure file. Explicitly confirmed broker changes are
the only configuration-write exception, implemented solely by the above Azure
CLI calls. Ordinary login, switching, status and doctor do not persist settings.

## Doctor

```sh
azctx doctor
azctx doctor -o json
```

Doctor works without an Azure profile/config directory or installed `az`.
Findings do not fail the diagnostic command; invalid flags/formats, output
failures and cancellation still use the central error/exit contract. JSON is
one compact object plus exactly one LF on stdout, with no verbose/stderr noise.
It does not sign in, switch/clear accounts, change settings or contact Azure.
It invokes only `az version -o json` through the resolved runner.

The separate doctor `schemaVersion: 1` report contains:

| Field | Meaning |
| --- | --- |
| `azctxVersion` | Build/release version |
| `update` | Read-only cached stable-update status: `not_checked`, `check_failed`, `available`, `current`, `unknown_version` or `unavailable`; numeric `latestVersion` and `stale` (24h old/future timestamp). No network request/cache creation/lock; not a claim of live freshness. |
| `azureCli` | Resolved executable `path`, validated `version`, `status` (`ok`, `unsupported`, `unavailable`). Warning below 2.61.0, macOS broker hint below 2.91.0. |
| `configDirectory`, `profile` | Azure directory and independent parse result (`ok`, `missing`, `invalid`, `unavailable`), plus profile record count |
| `legacyTenantFile` | Presence/absence/unavailability of `azctxTenants.json`; shell-quoted `deleteCommand` for manual cleanup (PowerShell on Windows, POSIX elsewhere). Only stat, never open/read/delete. Human output quotes/escapes the command to avoid terminal control characters. |
| `settings` | Status, effective `broker` platform/key/enabled/supported/available/preview/minimum version, and `loginExperienceV2`; invalid/unreadable values are null, not silently assumed valid |
| `activeCloud` | Shared setting/environment resolution, including the `AzureCloud` blank fallback; empty when config cannot be parsed |
| `accountTypes` | Sorted allowlisted account-type counts across **all** profile records/clouds, with unknown types reduced to `unknown` |
| `warnings` | Deterministic, safe findings, without raw subprocess errors or invalid field values |

No token, password, username, tenant/subscription ID/name, arbitrary config entry,
profile payload, MSAL cache or leftover tenant-file content is printed. The
explicitly reported path/cloud fields may reveal local filesystem/cloud names;
review them before sharing a report. Subscriptions' schema-1 DTO is unchanged.

## Live acceptance checklist — unverified

No live identities were available during implementation. Offline fake-Azure
tests are not proof of live broker support. A maintainer must manually verify:

- Windows WAM sign-in and confirmed enable/disable/re-login.
- macOS enrolled-device broker preview on **and** off, account picker/browser
  wording and persistence on subsequent Azure CLI invocations.
- Device-code sign-in over SSH with immediately visible instructions.

## References

- [Azure CLI interactive sign-in, WAM, macOS preview and cache clearing](https://learn.microsoft.com/cli/azure/authenticate-azure-cli-interactively)
- [Azure CLI core 2.91.0 history](https://github.com/Azure/azure-cli/blob/dev/src/azure-cli-core/HISTORY.rst)
- [MSAL Python macOS broker](https://learn.microsoft.com/entra/msal/python/advanced/macos-broker)
- [Microsoft Enterprise SSO plug-in](https://learn.microsoft.com/entra/identity-platform/apple-sso-plugin)
