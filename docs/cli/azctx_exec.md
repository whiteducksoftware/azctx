## azctx exec

Run a command with an isolated context environment

### Synopsis

Resolve one query and run one executable after the mandatory -- delimiter, without a shell. Override only the four ARM/AZURE context variables in the child; do not switch Azure CLI or change the parent environment. Streams and child exit status pass through, including in JSON mode. This is not authentication, a sandbox or supervision of arbitrary descendants. Azure CLI still needs explicit --subscription.

```
azctx exec <query> -- <cmd> [args] [flags]
```

### Examples

```
  azctx exec prod -- terraform plan
  azctx exec dev -- azd env get-values
```

### Options

```
  -h, --help   help for exec
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
