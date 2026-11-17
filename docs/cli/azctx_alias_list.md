## azctx alias list

List configured subscription aliases

### Synopsis

Print aliases sorted by name with their subscription GUIDs. Default/text output only; no profile, Azure CLI or network is needed. A missing configuration has no aliases; malformed configuration remains an error.

```
azctx alias list [flags]
```

### Examples

```
  azctx alias list
  azctx alias list -o text
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

* [azctx alias](azctx_alias.md)	: Manage subscription aliases
