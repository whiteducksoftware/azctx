## azctx help

Show help for any command

### Synopsis

Read usage, supported flags and examples for azctx or a named command without reading Azure configuration, starting Azure CLI or accessing the network.

```
azctx help [command] [flags]
```

### Examples

```
  azctx help
  azctx help auth broker
```

### Options

```
  -h, --help   help for help
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
