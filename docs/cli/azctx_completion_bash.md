## azctx completion bash

Generate bash completion

### Synopsis

Print a bash completion script to stdout for installation through your shell's completion setup. --no-descriptions omits candidate descriptions. Generation and runtime completion never sign in or switch subscriptions.

```
azctx completion bash
```

### Examples

```
  azctx completion bash
  azctx completion bash --no-descriptions
```

### Options

```
  -h, --help[=true|false]              help for bash
      --no-descriptions[=true|false]   disable completion descriptions
```

### Options inherited from parent commands

```
      --all[=true|false]        Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
      --no-input[=true|false]   Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string           Output format (depends on command)
      --verbose[=true|false]    Enable debug diagnostics
```

### SEE ALSO

* [azctx completion](azctx_completion.md)	: Generate shell completion scripts
