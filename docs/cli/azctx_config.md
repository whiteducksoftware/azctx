## azctx config

Print effective configuration or its path

### Synopsis

Print strict version-1 YAML with defaults, or the configuration filename with path. AZCTX_CONFIG_DIR overrides the azctx directory, never the Azure directory. Missing files use defaults; malformed files fail. The path form works even when the configuration is malformed. No Azure CLI or profile is needed.

```
azctx config [path] [flags]
```

### Examples

```
  azctx config
  azctx config path
```

### Options

```
  -h, --help   help for config
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
