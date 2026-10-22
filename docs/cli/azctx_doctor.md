## azctx doctor

Show safe offline Azure CLI diagnostics

### Synopsis

Inspect allowlisted settings, profile status, account-type counts and cached update status. Invoke only az version; never contact Azure, fetch updates, sign in or switch. Works without a profile or installed az. Findings are not fatal command errors. Supports text and separate schema-1 JSON; review local paths/cloud names before sharing.

```
azctx doctor [flags]
```

### Examples

```
  azctx doctor
  azctx doctor -o json
```

### Options

```
  -h, --help   help for doctor
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
