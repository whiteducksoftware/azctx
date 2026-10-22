## azctx version

Print azctx version and build information

### Synopsis

Print version, build date, commit and Go version in text (default) or JSON. Missing metadata is unknown. No profile, Azure CLI or update check is needed; version JSON is separate from subscription schema-1 output.

```
azctx version [flags]
```

### Examples

```
  azctx version
  azctx version -o json
```

### Options

```
  -h, --help   help for version
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
