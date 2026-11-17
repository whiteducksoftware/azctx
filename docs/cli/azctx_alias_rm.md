## azctx alias rm

Remove a subscription alias

### Synopsis

Remove NAME ignoring case from private config.yaml without switching. A missing alias is an error. Other settings survive the canonical atomic rewrite; comments and original formatting do not.

```
azctx alias rm NAME [flags]
```

### Examples

```
  azctx alias rm prod
  azctx alias rm dev
```

### Options

```
  -h, --help[=true|false]   help for rm
```

### Options inherited from parent commands

```
      --all[=true|false]        Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
      --no-input[=true|false]   Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string           Output format (depends on command)
      --verbose[=true|false]    Enable debug diagnostics
```

### SEE ALSO

* [azctx alias](azctx_alias.md)	: Manage subscription aliases
