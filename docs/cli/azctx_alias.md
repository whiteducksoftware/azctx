## azctx alias

Manage subscription aliases

### Synopsis

Manage case-insensitive names mapped to subscription GUIDs in private config.yaml. Set and rm atomically rewrite canonical YAML while preserving other settings, not comments. Aliases never bypass ambiguity, cloud, visibility or protected-switch checks. No Azure CLI or profile is needed.

### Examples

```
  azctx alias list
  azctx alias set prod 11111111-1111-1111-1111-111111111111
```

### Options

```
  -h, --help   help for alias
```

### Options inherited from parent commands

```
      --all             Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
      --no-input        Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string   Output format (depends on command)
      --verbose         Enable debug diagnostics
```

### SEE ALSO

* [azctx](azctx.md)	: Switch Azure CLI subscriptions with an identity-aware fuzzy picker
* [azctx alias list](azctx_alias_list.md)	: List configured subscription aliases
* [azctx alias rm](azctx_alias_rm.md)	: Remove a subscription alias
* [azctx alias set](azctx_alias_set.md)	: Create or replace a subscription alias
