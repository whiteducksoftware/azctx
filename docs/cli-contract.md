# CLI contract

This contract applies to the unreleased azctx 2026.11 source. The
[generated reference](cli/azctx.md) includes per-command help and examples.
Subscription JSON has schema version
**1**. Commands read only `azureProfile.json`, INI `config` and `clouds.config`
in the Azure configuration root. Doctor may stat, but never read, a leftover
`azctxTenants.json`. They never access credentials/MSAL caches or directly write
Azure files. The sole configuration-write exception is an explicitly confirmed
`auth broker` change delegated to Azure CLI. Authentication remains delegated to it.

## Commands and formats

| Invocation | Behavior and output |
| --- | --- |
| `azctx` | Open the subscription picker when input is interactive; otherwise print the same table as `azctx list`. |
| `azctx <query> [--all] [--yes]` | Switch by subscription name, alias, subscription ID or `tenant/subscription`. Successful switches are quiet. Protected targets require confirmation or `--yes`. |
| `azctx -- <query>` | Treat the query literally as a positional argument, even when it equals `list`, `tenant`, `history`, `env`, `exec`, `guard`, `login`, `alias`, `config`, `auth`, `doctor`, `version`, `help`, `completion` or starts with `-`. |
| `azctx -` | Switch to the previous subscription using private state, rolling it back if Azure CLI switching fails. |
| `azctx -c [-o text\|json\|id\|name\|tenant]` | Print the current subscription. Default `text` preserves the human-readable active-subscription line. `id`, `name` and `tenant` print only the corresponding subscription ID, name or tenant ID and a newline. |
| `azctx list [--all] [-o table\|json]` | List subscriptions in profile order. Default: `table`. Disabled, tenant-level and other/unknown-cloud accounts are hidden unless `--all` is present. |
| `azctx tenant [query] [--all] [--yes]` | Select a GUID-distinct tenant, then a subscription within it. A unique tenant with one selectable subscription switches directly. Queries support text/JSON output; no-input/JSON ambiguities exit 2 with subscription candidates. Bare no-input or explicit table/JSON output lists subscriptions, with `--all` enabling inspection only of Azure-filtered entries. |
| `azctx history [-o text\|json]` | Read the last 100 completed switches, newest first, without loading a profile or running Azure CLI. Separate schema-1 JSON; see [configuration](configuration.md#completed-switch-history). |
| `azctx env [query] [--shell bash\|zsh\|fish\|powershell] [--format github\|azure-devops]` | Export exactly four subscription/tenant context variables without switching. Default: current context, bash. Shell and pipeline format flags are mutually exclusive. GitHub appends to existing `GITHUB_ENV`; DevOps prints logging commands. Default/text output only. |
| `azctx exec <query> -- <cmd> [args]` | Resolve one command once, without a shell; attach streams and override only the four child context variables. Preserve child status/signals without extra diagnostics. Default/text/JSON affect azctx preflight errors only; child output is untouched. |
| `azctx guard [--subscription <q>] [--tenant <q>] [-o text\|json]` | Check the actual active context against supplied dimensions or, with neither flag, the nearest strict `.azctx.yaml`. Silent on match; mismatch 4 (`context_mismatch`), no project/malformed project 1, unresolved/ambiguous query 2. No mutation or prompts. |
| `azctx version [-o text\|json]` | Print version information, without loading the Azure profile or checking for updates. Its existing version JSON is unchanged. |
| `azctx login [--use-device-code] [--tenant <name\|id> \| --all-tenants] [-- <Azure CLI arguments>]` | Require Azure CLI >=2.61.0, attach login streams including live stderr capture, then offer one confirmation for failed MFA tenants. `--all-tenants` explicitly discovers tenant GUIDs and signs in individually. Disable Azure CLI's subscription picker only in login children. Supports default/text output, not subscription JSON. |
| `azctx auth broker status` | Read effective platform broker settings and check Azure CLI availability, without a profile or mutations. Default/text output only. |
| `azctx auth broker enable\|disable [--yes]` | Require a supported platform/version and explicit confirmation, then `az account clear`, `az config set` for the platform broker key, attached `az login`. Default/text output only; stops on the first failure. |
| `azctx doctor [-o text\|json]` | Offline, profile-independent diagnostics, including cached update status. Default: text. Missing dependencies/broken configuration are report findings, not fatal command errors. |
| `azctx config [path]` | Print effective version-1 YAML or its path. Default/text output; no Azure installation/profile required. |
| `azctx alias set NAME ID`, `alias rm NAME`, `alias list` | Atomically manage case-insensitive aliases or list them sorted by name. Default/text output; no Azure installation/profile required. |
| `azctx completion bash\|zsh\|fish\|powershell` | Generate shell scripts; argument completion reads the profile/config without running Azure CLI, network calls or state writes. |

`-o`/`--output`, `--verbose`, `--all` and `--no-input` are persistent flags, accepted before
or after a subcommand. Formats are case-sensitive. Unsupported formats and
invalid arguments/flags fail with exit 1 before an Azure subprocess is started.
`-c` does not accept a query. Root `--yes` explicitly confirms a protected switch.
Root/tenant `--all` expose configured hidden records without bypassing Azure cloud/state
switching policy; `list --all` and bare no-input `tenant --all` expose Azure-filtered records for inspection.
`--short`, `-s` and `--filter-tenant-level` are removed, not deprecated.

Unqualified queries first match case-insensitive exact subscription IDs, then
exact names/aliases, then fuzzy subscription names, profile tenant display names,
default domains, tenant IDs and subscription IDs. Exact name/alias collisions
remain ambiguous; unavailable aliases do not fall back to unrelated fuzzy matches.
A qualified query splits at the first `/`: the tenant selector must exactly
match a tenant ID, profile display name or default domain, ignoring case. Within that
tenant, the subscription selector matches names/IDs/aliases, using
the same exact/fuzzy rules (not tenant fields). Both selectors must be nonempty.
Use an ID for names containing literal `/`.
Duplicate names retain their subscription and tenant identities.
Pinned subscription/tenant IDs precede recent IDs (most recent first); ties retain
stable profile order for query candidates and tenant/name/ID order in the built-in
picker. Ranking never auto-resolves ambiguity. Configured hidden entries are
excluded from lists, selection and completion unless `--all` is supplied.
See [configuration](configuration.md) for the exact schema and path rules.
Tenant-first matching uses the same exact-ID/exact-label/fuzzy tiers, with exact
tenant domains also treated as exact labels. Grouping and picker selection use
token tenant GUIDs, never display names. Both picker stages honor configured fzf;
missing fzf emits one fallback line, while cancellation and other failures propagate.

The active cloud comes from `[cloud] name` in Azure CLI's `config`, overridden
by `AZURE_CLOUD_NAME`, with `AzureCloud` as the absent/blank-setting default.
Whitespace-only values are blank. No list of
built-in clouds is assumed. Only exact `environmentName` matches are selectable;
missing environment fields are not inferred. `list --all` is inspection only:
foreign-cloud entries are included but never marked current.

Current selection uses a unique active-cloud profile `isDefault`, falling back
to that cloud's `subscription` GUID in `clouds.config` only when no active-cloud
profile default exists. A stale/foreign remembered ID or multiple defaults
cannot identify a current subscription. Listing, current output, switching and
previous-state capture use the same resolved selection. Current entries hidden
by Azure cloud/state policy remain hidden in ordinary current output.
Configured hiding does not conceal the actual current subscription; protected
current text gains `[protected]`, without changing JSON or plain-field formats.
Human current output also warns about a mismatching or invalid nearest project
file; JSON and plain-field modes do not read the project file.

Environment/exec query resolution uses the pure selection catalog, never the
switch/picker orchestration; ambiguity always fails even on a TTY. Protection
requires no confirmation for a read-only export. `--all` exposes configured
hidden candidates only, not Azure-filtered accounts. An omitted env query uses
the actual current context (including configured hidden entries). Both IDs are
validated as GUIDs before any export/child process. These commands require a
profile but not an installed `az`, never read previous/recent/history state,
and never invoke Azure, login, updates or switch side effects.

Project files are expectations, never commands; they cannot drive target
selection or login. See [project configuration](configuration.md#project-context)
for strict parsing and [CI/CD](ci-cd.md) for exact exports and pipeline timing.
Azure CLI ignores the context variables: use `--subscription` even in `exec`.
Explicitly requested child commands own their output and side effects.

### Executable and diagnostic boundaries

All azctx-derived subscription/tenant IDs are validated as GUIDs before
becoming Azure CLI arguments, including on Windows where `az` may be `az.cmd`.
Only the user's literal login arguments after an explicit `--` are exempt:
Azure CLI, not azctx, interprets them (including tenant domains and other
authentication inputs). Friendly flags, profile metadata, aliases and project
expectations never acquire this exception. Avoid putting secrets in command
lines; shell history and the requested process are outside this protection.

Azure CLI and `exec` executables are resolved once with `exec.LookPath` and
used by absolute path; Go's current-directory executable refusal remains in
place. Doctor reports the same resolved Azure CLI path, without another lookup.

Verbose messages, doctor string diagnostics, command-error messages and Azure
CLI stderr replace recognized token-shaped values with `[REDACTED]`. This covers
JWT-like strings, common provider-token prefixes, long opaque words and values
after token/secret/password labels or `Bearer`. Stderr filtering is streaming,
retains at most one short word and handles split tokens and quoted values.
Whitespace/punctuation-terminated device-code prompts remain visible before
input; a final short word is emitted at the next delimiter or process EOF.
This is conservative heuristic filtering, not a guarantee that arbitrary
unlabelled secrets can be recognized. Azure CLI stdout remains a data stream;
explicit `exec` children keep all their own streams unchanged.

Ctrl+C restores the built-in picker's cursor and exits 130. Switching, login,
MFA retries and confirmation reads use a signal-aware command context; an
interrupted Azure CLI returning 1 still produces azctx status 130, stops tenant
retries and restores azctx's exact previous/MRU state on a failed switch.
Declining or accepting the default `N` at the protected-switch prompt exits 1
with `switch canceled; no changes made`, so `azctx prod && command` never
continues; other declined confirmations retain their existing status.

On Unix, a private supervisor and the attached command share a separate process
group. When stdin is azctx's controlling terminal and azctx owns its foreground
group, the child group temporarily owns the foreground terminal;
stdin/stdout/stderr remain attached, not proxied through a
replacement terminal. The supervisor observes terminal group signals without
forwarding them again. Signals addressed to azctx's PID are forwarded to the
child group once. The original foreground group is restored after the command.
Restoration and parent-side terminal output also work in ordinary shell jobs
with `TOSTOP` enabled, without changing process signal dispositions or existing
signal subscriptions. An inherited noncontrolling terminal or background wrapper
does not claim foreground ownership; the isolated child group and PID-directed
signal forwarding still work. Background programs remain subject to the shell's
normal terminal read restrictions.
An interrupt gets a two-second grace period; a second interrupt kills promptly.
Cancellation also cleans up the interrupted child group. Windows uses a child
console process group and Ctrl+Break, with the same bounded direct-child kill.
Protected, broker and tenant prompts can be canceled without closing stdin,
including native Windows console reads.
Windows confirmation echo follows accepted edits: backspace/delete on an empty
answer does not erase the prompt, and a raw Ctrl+C cancels without being echoed.

A successful child whose descendant holds an inherited stderr pipe is not a
failed login: the two-second pipe-drain timeout is ignored only when the process
succeeded. Real process failures, stderr flush failures and cancellation remain
errors. Ordinary `exec` child exits and Unix signal statuses remain unchanged.
This does not supervise an arbitrary descendant process tree.

Tables and picker labels distinguish user, service principal, system-assigned
identity and user-assigned identity accounts. `Warned`/`PastDue` remain visible
with their state marked. Nonempty different home/token tenant IDs mark delegated
Azure Lighthouse access; `managedByTenants` alone does not imply delegation.
Tenant labels prefer the display name, then default domain, then tenant ID.
Human lists, current text and pickers emit one line suggesting `az login` only
when a displayed record lacks both usable modern tenant fields. Current output
checks only the current record; narrowed pickers check only their candidates.
`list --all` checks every displayed entry, including hidden/foreign records.
JSON, plain-field current formats and quiet direct switches do not emit the hint.

Login initialization permits a missing Azure directory/profile; only Azure CLI
creates them. Existing malformed settings remain errors. After a supported,
attached login succeeds, azctx strictly reloads Azure's profile. Ordinary
list/current/switch initialization still requires a readable profile and reports
missing files with an `az login` hint. `azcli.CheckVersion` itself requires no
profile or Azure configuration directory.

Friendly login tenant selectors resolve case-insensitive fuzzy names/domains
against distinct active-cloud profile tenants (including tenant-level records).
No match or ambiguity fails before any subprocess; a validated GUID can be used
on first login. `--tenant` cannot be combined with `--all-tenants` or another
passthrough tenant flag. Other flags still pass after `--`.

Login classifies documented per-tenant stderr warnings: 50076/50079 can be
retried once, after one default-yes confirmation; 53003 is explained as a
Conditional Access block and never retried; other diagnostics are retained,
subject to heuristic redaction.
Without azctx interactive input, initial failures are listed on stderr and exit
3 without retries/discovery. Partial or declined sign-ins also exit 3, not
success; completed attempts strictly reload the profile for the count summary.
`--all-tenants` uses `az account tenant list -o json`, validates/deduplicates
GUIDs, and does not repeat known non-retryable failures. Explicit non-user
`--identity`/`-i` and `--service-principal` passthrough skip all tenant discovery
and MFA retry logic. `--force-mfa` is removed.

Broker capability and confirmation details, headless hints, the diagnostic
schema and safety boundaries are in [authentication](authentication.md).

## Input, color and streams

- An azctx picker/prompt is permitted only when **stdin is a TTY**, `CI` is
  absent, `--no-input` is false and `AZCTX_NO_INPUT` is not `1`. A set-but-empty
  `CI` also disables input. `--no-input=false` cannot override the environment.
- Bare azctx lists rather than waiting when input is disabled. Ambiguous queries
  fail immediately with exit 2 and candidates instead of opening a picker.
  A unique query still switches without input, except protected targets need
  explicit `--yes`; otherwise they exit 1 without mutation.
- Explicit `-o table` on bare azctx requests a list even with a TTY.
  JSON mode is non-interactive: bare `azctx -o json` lists; ambiguous switches
  fail with candidates even with a TTY. Successful `azctx -o json <query>`
  and `azctx -o json -` remain quiet (there is no switch-success JSON payload).
- Input gating covers azctx-owned pickers/prompts. MFA recovery follows the
  rules above; `login` still attaches Azure CLI to the supplied streams and
  forwards arguments after `--`. Azure CLI owns its authentication interaction;
  `--no-input` does not introduce an Azure CLI unattended-authentication mode.
- Data goes to stdout; diagnostics and picker rendering go to stderr.
  Login retains the streams emitted by Azure CLI, including device codes.
- Colors are disabled on non-TTY output and whenever `NO_COLOR` is present
  (including an empty value), even if `FORCE_COLOR`/`CLICOLOR_FORCE` request color.
  Tables and plain-field output contain no terminal styling.
- `--verbose` enables stderr debug diagnostics. JSON mode suppresses ordinary
  diagnostics, including verbose messages, so failures have exactly one JSON
  error object on stderr and no error text on stdout.
- History recovery/write warnings are explicit one-line stderr notices, including
  on otherwise quiet successful JSON switches; they never contaminate stdout or
  turn a completed switch into a failed command.
- Optional update checks run only after successful switches, with stdout a TTY,
  `CI` absent, `AZCTX_NO_UPDATE_CHECK` not `1` and config `updateCheck: true`.
  JSON mode also suppresses them.
  The inherited daily cache and two-second deadline remain; a check failure
  cannot fail a successful switch. Listing/current/version never check updates.
  Doctor only reads an existing private update cache; it never checks the
  network, creates cache files or acquires update locks.

## Exit statuses

The implementation's single mapping is `cmd.ExitCode`, used by the executable
boundary for handler failures, Cobra flag/argument errors and interruptions.

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Error, including invalid arguments/formats, profile/state failures, a declined or unconfirmed protected switch, login preflight/discovery errors and non-login Azure subprocess errors |
| 2 | No subscription match, unavailable previous subscription, or ambiguous match when input is disabled |
| 3 | Azure sign-in failed, or one or more tenant sign-ins remain unresolved (including partial success, no-input, declined/unreadable confirmation) |
| 4 | Guard found a different subscription or tenant context |
| 130 | Canceled with Ctrl+C or an explicitly canceled command context |

Sign-in and guard failures use the typed `cmd.ExitError` boundary. An attached
`exec` child instead returns its own status (which can be any exit code), or
`128 + signal` on Unix; explicitly canceled execution returns 130.
Missing previous state is an error (1), not a query with no match. Escape/ordinary picker cancellation is
an error (1); Ctrl+C remains 130. A Ctrl+C byte in a redirected input stream is
not a terminal interrupt and never opens a picker.

## Subscription JSON, schema version 1

List envelope:

```json
{"schemaVersion":1,"subscriptions":[]}
```

Current envelope:

```json
{"schemaVersion":1,"subscription":{"id":"11111111-1111-1111-1111-111111111111","name":"Production","tenantId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","tenantName":"Example Directory","tenantDomain":"example.invalid","state":"Enabled","isDefault":true,"user":{"name":"synthetic@example.invalid","type":"user"},"environment":"AzureCloud","delegated":false}}
```

Every `subscriptions` entry, current `subscription` and error `candidates` entry
uses **exactly the same subscription object**:

| Field | JSON type | Meaning |
| --- | --- | --- |
| `id` | string | Subscription ID |
| `name` | string | Subscription name |
| `tenantId` | string | Tenant ID |
| `tenantName` | string | Profile `tenantDisplayName`, falling back to `tenantDefaultDomain`, then `tenantId` |
| `tenantDomain` | string | Profile `tenantDefaultDomain`, empty when absent |
| `state` | string | Profile subscription state |
| `isDefault` | boolean | Resolved current selection in the active cloud only |
| `user` | object | Always includes string `name` and string `type` |
| `environment` | string | Profile `environmentName` |
| `delegated` | boolean | Nonempty `homeTenantId` differs from nonempty `tenantId`, ignoring GUID case |

All fields are present, including empty strings/false. Missing profile strings
become `""` (except the tenant-name ID fallback); missing booleans become `false`.
Empty lists are `[]`, never `null`. All tenant metadata comes from the profile,
without extra tenant API calls or cache files. JSON is one compact object
followed by a newline; list order preserves profile order.
Consumers should tolerate additive fields and inspect `schemaVersion`.

## Error JSON

With `-o json`/`--output=json`, azctx failures, including Cobra parse errors,
are one object on stderr. An attached `exec` child owns its own streams and
nonzero status; no additional JSON/error is appended after child diagnostics.
Preflight exec failures still use this envelope:

```json
{"code":"no_match","message":"no azure subscription found for 'missing'","exit_code":2}
```

`code` is a string, `message` is a human-readable string, and `exit_code` is an
integer from the table above. This error envelope is not a subscription envelope
and does not carry `schemaVersion`. Only ambiguity adds `candidates`, an array
of complete version-1 subscription objects in stable priority order, including IDs and
tenant IDs. Its code is `ambiguous_match` and its exit status is 2.

Codes are `error`, `no_match`, `ambiguous_match`, `interrupted`,
`sign_in_failed` (3) and `context_mismatch` (4).
Messages may give context; scripts should use `code` and
`exit_code`, not parse message text.

The last output flag before `--` selects error presentation, even if Cobra
stops parsing at an earlier invalid flag. Attached values (`-ojson`,
`--output=json`) and grouped boolean shorthands (`-cojson`) follow this rule.
Tokens after `--` are never output flags, including forwarded login arguments.

## Environment variables and private files

| Variable | Effect |
| --- | --- |
| `AZURE_CONFIG_DIR` | Azure configuration root; unset uses `~/.azure`. azctx only reads/stat-checks allowlisted files; explicitly confirmed broker changes delegate persistence to Azure CLI. Credentials/MSAL caches are never read. |
| `AZURE_{SECTION}_{NAME}` | Azure CLI setting overrides, in uppercase; environment wins over INI `config`. Section names are case-sensitive, keys are not. Booleans accept `1/yes/true/on` and `0/no/false/off`, ignoring case. Missing files use defaults; malformed files/booleans are errors when read. |
| `AZURE_CORE_LOGIN_EXPERIENCE_V2` | Caller value is preserved; each wrapped login child alone overrides it to `off`. |
| `AZCTX_NO_INPUT=1` | Disable azctx-owned pickers/prompts. Other values do not enable this gate. |
| `CI` | Presence, even empty, disables picker input and optional update checks. |
| `NO_COLOR` | Presence, even empty, disables azctx colors on all streams. |
| `AZCTX_NO_UPDATE_CHECK=1` | Disable optional update checks. |
| `AZCTX_CONFIG_DIR` | Override the azctx directory for `config.yaml`, `state.json`, `history.json` and their private locks; must remain outside Azure configuration, including symlink targets. |
| `HOME`, `USERPROFILE`, `XDG_CONFIG_HOME`, `APPDATA` | Platform-standard user configuration resolution for private config and previous/recent subscription state. |
| `XDG_CACHE_HOME`, `LOCALAPPDATA` | Platform-standard user cache resolution for update-check data (with the platform's home-directory fallback). |

Private state/cache must stay outside both the resolved and environment Azure
configuration roots, including overlapping directories and symlink targets.
An unsafe location or corrupt previous/default GUID is an explicit error, never
a silent fallback. Failed Azure switches roll back previous/recent subscription state.
Missing YAML uses defaults; existing YAML must declare `version: 1`, rejects unknown
keys and reports malformed values with filename/line diagnostics.
