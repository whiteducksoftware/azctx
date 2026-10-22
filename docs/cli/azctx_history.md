## azctx history

Show the last 100 completed subscription switches

### Synopsis

Read private identity-only switch history, newest first, without Azure CLI or a profile. Supports text and separate schema-1 JSON output. Missing history is empty; corrupt history warns. This best-effort log records azctx switches only, not login or external changes.

```
azctx history [flags]
```

### Examples

```
  azctx history
  azctx history -o json
```

### Options

```
  -h, --help   help for history
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
