# Configuration and subscription selection

`azctx config` prints the effective version-1 YAML configuration on stdout;
`azctx config path` prints its filename, even if the file is absent or malformed.
These commands and `azctx alias set|rm|list` do not need an Azure profile or an
installed Azure CLI. They support default/text output.

The default file is `os.UserConfigDir()/azctx/config.yaml` (the platform's user
configuration directory). `AZCTX_CONFIG_DIR` overrides the **azctx directory
itself**, for `config.yaml`, `state.json` and `history.json`. It does not change Azure's
configuration directory or the update cache location.

```yaml
version: 1
aliases:
  prod: 11111111-1111-1111-1111-111111111111
  dev: 22222222-2222-2222-2222-222222222222
pinned:
  - 22222222-2222-2222-2222-222222222222
hidden: []
protected:
  - 11111111-1111-1111-1111-111111111111
picker: builtin
updateCheck: true
```

Only these keys are accepted. `version: 1` is required in an existing file.
Aliases map nonempty names without `/` to subscription GUIDs; names and GUIDs
are matched ignoring case. Duplicate keys and case-colliding aliases are errors.
`pinned`, `hidden` and `protected` accept subscription **or token tenant** GUIDs.
All three lists and aliases default to empty. `picker` defaults to `builtin`;
`fzf` uses an optional executable on `PATH`, falling back to the built-in picker
with one stderr notice if unavailable. `updateCheck` defaults to `true`; `false` skips all
post-switch update work, including cache/lock/network work.

A missing file uses defaults without creating anything. Malformed YAML,
unknown keys, wrong types, invalid GUIDs and extra YAML documents fail with the
filename and line number. Read/write failures are errors, not silent defaults.
Owned files use the shared atomic writer with `0600` files and `0700`
directories on Unix. Locations within either the resolved or environment Azure
configuration directory are rejected before reads/writes, including symlinks.
No credentials, token caches or Azure files are written.

## Project context

`.azctx.yaml` is a separate, untrusted, read-only project expectation, not an
azctx user configuration file:

```yaml
version: 1
tenant: aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
subscription: 11111111-1111-1111-1111-111111111111
```

All three keys are required. GUIDs are compared case-insensitively; unknown or
duplicate keys, invalid types/GUIDs, unsupported versions and extra documents
are explicit filename/line errors. Only regular files up to 64 KiB are accepted;
symlinks are rejected. `guard` searches from the current directory to the
filesystem root and stops at the nearest file, even if it is malformed.
Missing files exit 1; mismatches exit 4 without switching. Supplied guard flags
replace the file's expectations entirely.

No project file can trigger switching, login, state/history writes or updates.
`env` and `exec` ignore it when resolving their targets. Human current output
warns about mismatches/invalid projects; JSON/plain-field current output does not
read the file. See [CI/CD](ci-cd.md) for exact export/guard contracts and examples.

## Aliases and matching

```sh
azctx alias set prod 11111111-1111-1111-1111-111111111111
azctx alias list
azctx prod
azctx alias rm prod
```

`set` replaces an existing alias ignoring case; `rm` also ignores case and
fails for a missing alias. Updates preserve other settings and atomically
rewrite canonical YAML, not comments or the original formatting.

Resolution first looks for exact subscription IDs, then exact subscription
names **and** exact aliases, then fuzzy subscription names/IDs and profile
tenant names/domains/IDs. Exact name/alias collisions retain every matching
identity: a favorite or recent selection never silently wins an ambiguity.
An exact alias whose target is unavailable does not fall back to an unrelated
fuzzy match.

`azctx TENANT/SUBSCRIPTION` first scopes by an exact token tenant ID, display
name or default domain, ignoring case. Within that scope, subscription IDs,
exact names/aliases and fuzzy names/IDs work as above. Both parts must be
nonempty. Use a subscription ID for names containing a literal `/`.
Names equal to commands (including `tenant` and `history`) require `azctx -- NAME`.

Pinned subscriptions/tenants appear first, followed by recently selected IDs
in most-recent-first order. Otherwise the built-in picker keeps its deterministic
tenant/name/ID order; query candidates retain stable profile order within
equal priority. Non-interactive ambiguities still exit 2 with full candidates.
`state.json` retains the previous ID and adds `recentSubscriptionIds`; failed
Azure switches roll both back. `azctx -` continues to swap previous/current.
Recent IDs are only ranking data; the separate history log is described below.

Configured hidden subscriptions/tenants are excluded from selection, lists and
completion unless `--all` is supplied. Root `--all` exposes these configured
entries; it does not bypass the existing cloud/disabled/tenant-account switch
policy. `list --all` additionally inspects those Azure-filtered records.
Current output still describes the actual current, active-cloud subscription.

## Protected subscriptions and completion

A protected subscription or token tenant requires a default-no confirmation
on stderr before **any** switch route (query, alias, picker or previous).
Without interactive input, or with JSON output, use `azctx --yes QUERY`
or `azctx tenant --yes TENANT_QUERY` when that tenant has one subscription.
Declining leaves Azure and private state unchanged; Ctrl+C remains exit 130.
`azctx -c` text marks the current subscription `[protected]`; JSON schema 1
and the `id`, `name` and `tenant` formats remain unchanged.

Generate scripts with `azctx completion bash|zsh|fish|powershell` and install
them using your shell's completion setup. Completion reads only allowlisted
profile/settings files and azctx configuration, never starts `az`, never checks
updates and never writes state. It suggests subscription names, available
aliases and profile tenant/name-qualified subscriptions, with case-insensitive
prefix matching and deterministic, deduplicated output.

## Tenant-first selection and fzf

`azctx tenant [query]` matches token tenant GUIDs first, then exact profile
names/domains, then fuzzy names/domains/GUIDs. Identically named tenants remain
separate by GUID. After selecting a tenant, only that tenant's selectable
subscriptions appear. A unique tenant with one subscription switches directly.
Pinned/recency priorities never resolve ambiguity automatically.

Without input or with JSON, a query with several tenants or subscriptions exits
2 with the existing subscription candidate objects. Use
`azctx TENANT_GUID/SUBSCRIPTION_GUID` to disambiguate. Bare `azctx tenant` without
input (or with explicit table/JSON output) lists subscriptions without switching.
`--all` reveals configured hidden entries; as with root selection, disabled,
tenant-level and other-cloud entries are **inspection only**, visible in
`azctx tenant --all --no-input` or `azctx list --all`, never made selectable.

Both picker stages support `picker: fzf`. azctx supplies ranked, indexed rows
through stdin and fixed arguments; names are never shell commands or options.
Control characters are flattened, every row carries GUID identity, and only an
exact supplied row is accepted back. fzf's `FZF_*` environment defaults are
removed for the child (including default options/options files and execute or
preview bindings); azctx's safety/input/protection gates still apply. Escape/no
match exits 1, Ctrl+C exits 130. Missing fzf falls back once per command with
one line on stderr; other fzf failures do not silently choose a subscription.

## Completed-switch history

`azctx history [-o text|json]` is read-only and works without Azure CLI or a
profile. It shows up to 100 completed switches, newest first, regardless of
current hidden/cloud filters. Missing history is an empty list. A corrupt file
is ignored **with a stderr warning**; the next completed switch replaces it.
Read/permission failures are errors when inspecting history. A history write
failure or busy history lock warns on stderr but never fails a completed switch.
Failed, declined, canceled and protected-unconfirmed switches are not recorded.
Login and switches made outside azctx are not recorded.

The file is `history.json` next to `state.json`, using the same private atomic
writer, `0600` files and `0700` directory on Unix, and the same Azure-isolation
checks. A retained private `history.lock` serializes append attempts using the
shared nonblocking OS lock. A concurrent busy writer may miss a history entry,
but reports that explicitly; the log is best-effort, not a compliance ledger.

Disk and `-o json` share this deterministic schema-1 envelope, compact UTF-8
JSON with exactly one trailing LF:

```json
{"schemaVersion":1,"entries":[{"timestamp":"2026-10-21T22:00:00Z","previous":null,"new":{"id":"11111111-1111-1111-1111-111111111111","tenantId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","environment":"AzureCloud"},"source":"script"}]}
```

`timestamp` is completion time in UTC RFC3339 with optional fractional seconds.
`previous` is the resolved pre-switch identity, or `null` if none was known;
`new` is the selected identity. Each identity contains only subscription `id`,
token `tenantId` and cloud `environment`: no user names, account types, raw
queries, profile labels, credentials or subprocess output. Re-selecting the
current subscription records equal previous/new identities if az succeeds.
`source` is `tty` for terminal stdin and `script` otherwise; it describes the
actual stdin stream, independently of `--no-input`, JSON or `CI` prompt opt-outs.
Empty history is `{"schemaVersion":1,"entries":[]}`, never `null`. JSON remains
noise-free on stdout; history warnings, including recovery after a successful
JSON switch, are explicit plain one-line stderr diagnostics.
Files over 1 MiB are treated as corrupt; recording an oversized log warns and
leaves the previous file intact.
