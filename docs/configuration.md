# Configuration and subscription selection

`azctx config` prints the effective version-1 YAML configuration on stdout;
`azctx config path` prints its filename, even if the file is absent or malformed.
These commands and `azctx alias set|rm|list` do not need an Azure profile or an
installed Azure CLI. They support default/text output.

The default file is `os.UserConfigDir()/azctx/config.yaml` (the platform's user
configuration directory). `AZCTX_CONFIG_DIR` overrides the **azctx directory
itself**, for both `config.yaml` and `state.json`. It does not change Azure's
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
`fzf` is accepted and preserved for the next release layer, but this layer still
uses the built-in picker. `updateCheck` defaults to `true`; `false` skips all
post-switch update work, including cache/lock/network work.

A missing file uses defaults without creating anything. Malformed YAML,
unknown keys, wrong types, invalid GUIDs and extra YAML documents fail with the
filename and line number. Read/write failures are errors, not silent defaults.
Owned files use the shared atomic writer with `0600` files and `0700`
directories on Unix. Locations within either the resolved or environment Azure
configuration directory are rejected before reads/writes, including symlinks.
No credentials, token caches or Azure files are written.

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
Names equal to commands (including `alias` and `config`) require `azctx -- NAME`.

Pinned subscriptions/tenants appear first, followed by recently selected IDs
in most-recent-first order. Otherwise the built-in picker keeps its deterministic
tenant/name/ID order; query candidates retain stable profile order within
equal priority. Non-interactive ambiguities still exit 2 with full candidates.
`state.json` retains the previous ID and adds `recentSubscriptionIds`; failed
Azure switches roll both back. `azctx -` continues to swap previous/current.
Recent IDs are only ranking data, not a history command or an audit log.

Configured hidden subscriptions/tenants are excluded from selection, lists and
completion unless `--all` is supplied. Root `--all` exposes these configured
entries; it does not bypass the existing cloud/disabled/tenant-account switch
policy. `list --all` additionally inspects those Azure-filtered records.
Current output still describes the actual current, active-cloud subscription.

## Protected subscriptions and completion

A protected subscription or token tenant requires a default-no confirmation
on stderr before **any** switch route (query, alias, picker or previous).
Without interactive input, or with JSON output, use `azctx --yes QUERY`.
Declining leaves Azure and private state unchanged; Ctrl+C remains exit 130.
`azctx -c` text marks the current subscription `[protected]`; JSON schema 1
and the `id`, `name` and `tenant` formats remain unchanged.

Generate scripts with `azctx completion bash|zsh|fish|powershell` and install
them using your shell's completion setup. Completion reads only allowlisted
profile/settings files and azctx configuration, never starts `az`, never checks
updates and never writes state. It suggests subscription names, available
aliases and profile tenant/name-qualified subscriptions, with case-insensitive
prefix matching and deterministic, deduplicated output.
