## azctx alias set

Create or replace a subscription alias

### Synopsis

Map NAME to a validated subscription GUID, replacing an existing name ignoring case. Names must be nonempty and cannot contain /. Other settings are retained; YAML comments and formatting are not. This edits private azctx configuration only and does not switch.

```
azctx alias set NAME SUBSCRIPTION_ID [flags]
```

### Examples

```
  azctx alias set prod 11111111-1111-1111-1111-111111111111
  azctx alias set dev 22222222-2222-2222-2222-222222222222
```

### Options

```
  -h, --help   help for set
```

### Options inherited from parent commands

```
      --all             Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
      --no-input        Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string   Output format (depends on command)
      --verbose         Enable debug diagnostics
```

### SEE ALSO

* [azctx alias](azctx_alias.md)	: Manage subscription aliases
