## azctx completion zsh

Generate zsh completion

### Synopsis

Print a zsh completion script to stdout for installation through your shell's completion setup. --no-descriptions omits candidate descriptions. Generation and runtime completion never sign in or switch subscriptions.

```
azctx completion zsh [flags]
```

### Examples

```
  azctx completion zsh
  azctx completion zsh --no-descriptions
```

### Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --all             Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
      --no-input        Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string   Output format (depends on command)
      --verbose         Enable debug diagnostics
```

### SEE ALSO

* [azctx completion](azctx_completion.md)	: Generate shell completion scripts
