## azctx

Switch Azure CLI subscriptions with an identity-aware fuzzy picker

### Synopsis

Choose a subscription by name, alias, GUID or exact tenant/subscription query. Use - for the previous subscription and -- before a name that equals a command.

With terminal input, the bare command opens a picker on stderr. Without input it lists subscriptions on stdout. Successful switches are quiet; protected targets need confirmation or --yes. Use -c for current context and -o json for structured data/errors.

```
azctx [query | -] [flags]
```

### Examples

```
  azctx
  azctx contoso/prod
  azctx -
```

### Options

```
      --all             Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts
  -c, --current         Display the current active subscription
  -h, --help            help for azctx
      --no-input        Never open a picker or prompt (also AZCTX_NO_INPUT=1)
  -o, --output string   Output format (depends on command)
      --verbose         Enable debug diagnostics
      --yes             Confirm switching to a protected subscription without prompting
```

### SEE ALSO

* [azctx alias](azctx_alias.md)	: Manage subscription aliases
* [azctx auth](azctx_auth.md)	: Inspect and explicitly manage Azure CLI authentication
* [azctx completion](azctx_completion.md)	: Generate shell completion scripts
* [azctx config](azctx_config.md)	: Print effective configuration or its path
* [azctx doctor](azctx_doctor.md)	: Show safe offline Azure CLI diagnostics
* [azctx env](azctx_env.md)	: Export context variables without changing the Azure CLI default
* [azctx exec](azctx_exec.md)	: Run a command with an isolated context environment
* [azctx guard](azctx_guard.md)	: Check the active context against reviewed expectations
* [azctx history](azctx_history.md)	: Show the last 100 completed subscription switches
* [azctx list](azctx_list.md)	: List cached subscriptions without switching
* [azctx login](azctx_login.md)	: Sign in through Azure CLI with attached streams
* [azctx tenant](azctx_tenant.md)	: Choose a tenant, then a subscription
* [azctx version](azctx_version.md)	: Print azctx version and build information
