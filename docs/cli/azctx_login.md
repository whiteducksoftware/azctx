## azctx login

Sign in through Azure CLI with attached streams

### Synopsis

Require Azure CLI >=2.61.0 and wrap az login. Device-code instructions stay visible. --tenant accepts a GUID or one profile tenant name/domain. Other Azure CLI arguments go after --; never put secrets in command lines.

Ordinary user login offers one confirmation to retry only reported MFA failures. --all-tenants explicitly discovers tenants for individual sign-ins. Workload-identity passthrough skips orchestration. --no-input gates azctx prompts only, not Azure CLI authentication. Supports default/text output; unresolved sign-ins exit 3.

```
azctx login [flags]
```

### Examples

```
  azctx login
  azctx login --use-device-code --tenant aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
  azctx login -- --identity
```

### Options

```
      --all-tenants       Discover Azure tenants and sign in to each separately
  -h, --help              help for login
      --tenant string     Tenant GUID or fuzzy profile tenant name/domain
      --use-device-code   Sign in using a device code
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
