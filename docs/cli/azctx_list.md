## azctx list

List cached subscriptions without switching

### Synopsis

Read the Azure CLI profile in profile order. Default table output hides configured hidden, disabled, tenant-level and other-cloud accounts. --all includes them for inspection, not switching. Supports table and schema-1 JSON output; no Azure subprocess or network request is made.

```
azctx list [flags]
```

### Examples

```
  azctx list
  azctx list --all -o json
```

### Options

```
  -h, --help   help for list
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
