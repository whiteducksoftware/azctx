## azctx auth

Inspect and explicitly manage Azure CLI authentication

### Synopsis

Authentication stays in Azure CLI. Use broker status for read-only effective settings, or explicitly confirmed broker enable/disable on supported Windows/macOS versions. azctx never reads credentials or MSAL token caches.

### Examples

```
  azctx auth broker status
  azctx auth broker enable
```

### Options

```
  -h, --help[=true|false]   help for auth
```

### Options inherited from parent commands

```
      --all[=true|false]        Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
      --no-input[=true|false]   Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string           Output format (depends on command)
      --verbose[=true|false]    Enable debug diagnostics
```

### SEE ALSO

* [azctx](azctx.md)	: Switch Azure CLI subscriptions with an identity-aware fuzzy picker
* [azctx auth broker](azctx_auth_broker.md)	: Inspect or explicitly change the persisted authentication broker
