## azctx version

Print azctx version and build information

### Synopsis

Print version, build date, commit and Go version in text (default) or JSON. Missing metadata is unknown. No profile or Azure CLI is needed; version JSON is separate from subscription schema-1 output and stays offline.

Text output makes a fresh stable-release lookup without reading the cache (two-second network deadline), and writes the result to the private cache for offline doctor status. A newer release produces a yellow stderr notice; failures are debug-only and do not fail version output. Skipped with updateCheck: false, AZCTX_NO_UPDATE_CHECK=1, CI present or non-TTY stdout. Switching never checks for updates.

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
