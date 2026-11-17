## azctx env

Export context variables without changing the Azure CLI default

### Synopsis

Export ARM_SUBSCRIPTION_ID, ARM_TENANT_ID, AZURE_SUBSCRIPTION_ID and AZURE_TENANT_ID for the current context or one unambiguous query. Choose bash (default), zsh, fish or powershell syntax, or a mutually exclusive github/azure-devops pipeline format. GitHub appends to existing GITHUB_ENV for later steps.

These variables are not credentials. Azure CLI ignores them; pass --subscription to az. Check export success before evaluating shell output.

```
azctx env [query] [flags]
```

### Examples

```
  azctx env --shell bash
  azctx env prod --shell powershell
  azctx env --format github
```

### Options

```
      --format string   Pipeline format: github (append GITHUB_ENV), azure-devops
  -h, --help            help for env
      --shell string    Shell syntax: bash, zsh, fish, powershell (default bash) (default "bash")
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
