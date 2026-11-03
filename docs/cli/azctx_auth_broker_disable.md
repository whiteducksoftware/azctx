## azctx auth broker disable

Disable the persisted broker after explicit confirmation

### Synopsis

On a supported platform/version, confirm account clearing and persisted broker configuration, then sign in again. Calls az account clear, az config set for the platform key, then attached az login. Stops on the first failure without automatic rollback. Conflicting environment overrides fail before changes. --yes explicitly confirms the change; it does not make Azure login unattended.

```
azctx auth broker disable [flags]
```

### Examples

```
  azctx auth broker disable
  azctx auth broker disable --yes --no-input
```

### Options

```
  -h, --help   help for disable
      --yes    Confirm account clear, persisted broker setting change and Azure sign-in
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
