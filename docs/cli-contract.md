# CLI contract

This contract applies to azctx 2026.11. Subscription JSON has schema version
**1**. Commands read only `azureProfile.json`, INI `config` and `clouds.config`
in the Azure configuration root. They never read credentials/MSAL caches or
write anything there. Authentication remains delegated to Azure CLI.

## Commands and formats

| Invocation | Behavior and output |
| --- | --- |
| `azctx` | Open the subscription picker when input is interactive; otherwise print the same table as `azctx list`. |
| `azctx <query>` | Switch by subscription name, subscription ID or `tenant/subscription`. Successful switches are quiet. |
| `azctx -- <query>` | Treat the query literally as a positional argument, even when it equals `list`, `login`, `version`, `help`, `completion` or starts with `-`. |
| `azctx -` | Switch to the previous subscription, preserving the layer-06 private-state/rollback behavior. |
| `azctx -c [-o text\|json\|id\|name\|tenant]` | Print the current subscription. Default `text` preserves the human-readable active-subscription line. `id`, `name` and `tenant` print only the corresponding subscription ID, name or tenant ID and a newline. |
| `azctx list [--all] [-o table\|json]` | List subscriptions in profile order. Default: `table`. Disabled, tenant-level and other/unknown-cloud accounts are hidden unless `--all` is present. |
| `azctx version [-o text\|json]` | Print version information, without loading the Azure profile or checking for updates. Its existing version JSON is unchanged. |
| `azctx login [--force-mfa] [-- <Azure CLI arguments>]` | Require Azure CLI >=2.61.0, then attach login streams including live stderr capture. Disable Azure CLI's subscription picker only in the login child's environment. Supports default/text output, not subscription JSON. |

`-o`/`--output`, `--verbose` and `--no-input` are persistent flags, accepted before
or after a subcommand. Formats are case-sensitive. Unsupported formats and
invalid arguments/flags fail with exit 1 before an Azure subprocess is started.
`-c` does not accept a query. `--all` belongs only to `list`.
`--short`, `-s` and `--filter-tenant-level` are removed, not deprecated.

Unqualified queries retain case-insensitive fuzzy matching of subscription names,
profile tenant names and subscription IDs; an exact subscription ID wins.
A qualified query splits at the first `/`: the tenant selector must exactly
match a tenant ID or available profile tenant name, ignoring case. Within that
tenant, the subscription selector matches only subscription names/IDs, using
the same fuzzy/exact-ID rules. Both selectors must be nonempty. Tenant domains
are not selectors in this layer. Use an ID for names containing literal `/`.
Duplicate names retain their subscription and tenant identities.

The active cloud comes from `[cloud] name` in Azure CLI's `config`, overridden
by `AZURE_CLOUD_NAME`, with `AzureCloud` as the absent-setting default. No list of
built-in clouds is assumed. Only exact `environmentName` matches are selectable;
missing environment fields are not inferred. `list --all` is inspection only:
foreign-cloud entries are included but never marked current.

Current selection uses a unique active-cloud profile `isDefault`, falling back
to that cloud's `subscription` GUID in `clouds.config` only when no active-cloud
profile default exists. A stale/foreign remembered ID or multiple defaults
cannot identify a current subscription. Listing, current output, switching and
previous-state capture use the same resolved selection. Hidden current entries
remain hidden in ordinary current output.

Tables and picker labels distinguish user, service principal, system-assigned
identity and user-assigned identity accounts. `Warned`/`PastDue` remain visible
with their state marked. Nonempty different home/token tenant IDs mark delegated
Azure Lighthouse access; `managedByTenants` alone does not imply delegation.
Older profiles use tenant IDs for missing display names. Human lists, current
text and pickers emit one line suggesting `az login` to refresh missing tenant
metadata; JSON, plain-field current formats and quiet direct switches do not.

## Input, color and streams

- An azctx picker/prompt is permitted only when **stdin is a TTY**, `CI` is
  absent, `--no-input` is false and `AZCTX_NO_INPUT` is not `1`. A set-but-empty
  `CI` also disables input. `--no-input=false` cannot override the environment.
- Bare azctx lists rather than waiting when input is disabled. Ambiguous queries
  fail immediately with exit 2 and candidates instead of opening a picker.
  A unique query still switches without input.
- Explicit `-o table` on bare azctx requests a list even with a TTY.
  JSON mode is non-interactive: bare `azctx -o json` lists; ambiguous switches
  fail with candidates even with a TTY. Successful `azctx -o json <query>`
  and `azctx -o json -` remain quiet (there is no switch-success JSON payload).
- Input gating covers azctx-owned pickers/prompts. This layer does not redesign
  authentication: `login` still attaches Azure CLI to the supplied streams and
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
- Optional update checks run only after successful switches, with stdout a TTY,
  `CI` absent and `AZCTX_NO_UPDATE_CHECK` not `1`. JSON mode also suppresses them.
  The inherited daily cache and two-second deadline remain; a check failure
  cannot fail a successful switch. Listing/current/version never check updates.

## Exit statuses

The implementation's single mapping is `cmd.ExitCode`, used by the executable
boundary for handler failures, Cobra flag/argument errors and interruptions.

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Error, including invalid arguments/formats, profile/state failures and Azure subprocess errors |
| 2 | No subscription match, unavailable previous subscription, or ambiguous match when input is disabled |
| 3 | Reserved: sign-in failed for one or more tenants (layer 10) |
| 4 | Reserved: guard found a different context (layer 13) |
| 130 | Cancelled with Ctrl+C |

Statuses 3 and 4 are supported/tested through the typed `cmd.ExitError` boundary;
this layer does not add future sign-in or guard behavior. Missing previous state
is an error (1), not a query with no match. Escape/ordinary picker cancellation is
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
| `tenantName` | string | Profile `tenantDisplayName`, falling back to `tenantId` |
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

With `-o json`/`--output=json`, every failure, including Cobra parse errors,
is one object on stderr:

```json
{"code":"no_match","message":"no azure subscription found for 'missing'","exit_code":2}
```

`code` is a string, `message` is a human-readable string, and `exit_code` is an
integer from the table above. This error envelope is not a subscription envelope
and does not carry `schemaVersion`. Only ambiguity adds `candidates`, an array
of complete version-1 subscription objects in profile order, including IDs and
tenant IDs. Its code is `ambiguous_match` and its exit status is 2.

Codes in this layer are `error`, `no_match`, `ambiguous_match` and `interrupted`.
The typed boundary reserves `sign_in_failed` (3) and `context_mismatch` (4)
for later layers. Messages may give context; scripts should use `code` and
`exit_code`, not parse message text.

The last output flag before `--` selects error presentation, even if Cobra
stops parsing at an earlier invalid flag. Attached values (`-ojson`,
`--output=json`) and grouped boolean shorthands (`-cojson`) follow this rule.
Tokens after `--` are never output flags, including forwarded login arguments.

## Environment variables and private files

| Variable | Effect |
| --- | --- |
| `AZURE_CONFIG_DIR` | Read-only Azure configuration root; unset uses `~/.azure`. Credentials/MSAL caches are never read. |
| `AZURE_{SECTION}_{NAME}` | Azure CLI setting overrides, in uppercase; environment wins over INI `config`. Section names are case-sensitive, keys are not. Booleans accept `1/yes/true/on` and `0/no/false/off`, ignoring case. Missing files use defaults; malformed files/booleans are errors when read. |
| `AZURE_CORE_LOGIN_EXPERIENCE_V2` | Caller value is preserved; each wrapped login child alone overrides it to `off`. |
| `AZCTX_NO_INPUT=1` | Disable azctx-owned pickers/prompts. Other values do not enable this gate. |
| `CI` | Presence, even empty, disables picker input and optional update checks. |
| `NO_COLOR` | Presence, even empty, disables azctx colors on all streams. |
| `AZCTX_NO_UPDATE_CHECK=1` | Disable optional update checks. |
| `HOME`, `USERPROFILE`, `XDG_CONFIG_HOME`, `APPDATA` | Platform-standard user configuration resolution for private previous-subscription state. |
| `XDG_CACHE_HOME`, `LOCALAPPDATA` | Platform-standard user cache resolution for update-check data (with the platform's home-directory fallback). |

Private state/cache must stay outside both the resolved and environment Azure
configuration roots, including overlapping directories and symlink targets.
An unsafe location or corrupt previous/default GUID is an explicit error, never
a silent fallback. Failed Azure switches roll back previous-subscription state.
