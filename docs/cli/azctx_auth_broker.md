## azctx auth broker

Inspect or explicitly change the persisted authentication broker

### Synopsis

Windows requires Azure CLI >=2.61.0; macOS requires >=2.91.0 and offers an opt-in enrolled-device preview. Linux has no broker control. Changes require confirmation and delegate account clearing, persisted configuration and a fresh login to Azure CLI. A failed step stops the sequence.

### Examples

```
  azctx auth broker status
  azctx auth broker disable
```

### Options

```
  -h, --help   help for broker
```

### Options inherited from parent commands

```
      --all             Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
      --no-input        Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string   Output format (depends on command)
      --verbose         Enable debug diagnostics
```

### SEE ALSO

* [azctx auth](azctx_auth.md)	: Inspect and explicitly manage Azure CLI authentication
* [azctx auth broker disable](azctx_auth_broker_disable.md)	: Disable the persisted broker after explicit confirmation
* [azctx auth broker enable](azctx_auth_broker_enable.md)	: Enable the persisted broker after explicit confirmation
* [azctx auth broker status](azctx_auth_broker_status.md)	: Show effective broker settings without changing accounts
