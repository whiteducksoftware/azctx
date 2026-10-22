## azctx auth broker status

Show effective broker settings without changing accounts

### Synopsis

Read effective platform settings and Azure CLI version without needing a profile. Environment overrides win over INI settings. Capability indicates platform/version support only, not enrollment, installed broker or successful sign-in. Supports default/text output.

```
azctx auth broker status [flags]
```

### Examples

```
  azctx auth broker status
  azctx auth broker status -o text
```

### Options

```
  -h, --help   help for status
```

### Options inherited from parent commands

```
      --all             Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
      --no-input        Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string   Output format (depends on command)
      --verbose         Enable debug diagnostics
```

### SEE ALSO

* [azctx auth broker](azctx_auth_broker.md)	: Inspect or explicitly change the persisted authentication broker
