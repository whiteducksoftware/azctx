## azctx tenant

Choose a tenant, then a subscription

### Synopsis

Match token tenant GUIDs, profile names or domains, then choose a selectable subscription within that tenant. Duplicate tenant names retain their GUID identities. A unique tenant with one subscription switches directly; non-interactive ambiguity exits 2. Bare no-input or explicit table/JSON output lists subscriptions instead.

```
azctx tenant [query] [flags]
```

### Examples

```
  azctx tenant
  azctx tenant example.invalid
  azctx tenant --all --no-input
```

### Options

```
  -h, --help   help for tenant
      --yes    Confirm switching to a protected subscription without prompting
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
