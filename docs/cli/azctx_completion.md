## azctx completion

Generate shell completion scripts

### Synopsis

Generate completion for bash, zsh, fish or PowerShell. Install the script using your shell's setup. Runtime subscription completion reads only profile/settings and private configuration; it never starts Azure CLI, writes state or checks updates.

### Examples

```
  azctx completion bash
  azctx completion powershell
```

### Options

```
  -h, --help   help for completion
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
* [azctx completion bash](azctx_completion_bash.md)	: Generate bash completion
* [azctx completion fish](azctx_completion_fish.md)	: Generate fish completion
* [azctx completion powershell](azctx_completion_powershell.md)	: Generate powershell completion
* [azctx completion zsh](azctx_completion_zsh.md)	: Generate zsh completion
