## azctx guard

Check the active context against reviewed expectations

### Synopsis

Compare the actual active subscription and token tenant against explicit flags or the nearest strict .azctx.yaml. Flags replace project expectations entirely. Match is silent exit 0; mismatch exits 4; missing/malformed project exits 1; unresolved queries exit 2. Never switch, authenticate or prompt. Guard is a point-in-time check, not an authorization boundary.

```
azctx guard [flags]
```

### Examples

```
  azctx guard
  azctx guard -o json
  azctx guard --subscription 11111111-1111-1111-1111-111111111111
```

### Options

```
  -h, --help                  help for guard
      --subscription string   Expected subscription query or GUID
      --tenant string         Expected tenant query or GUID
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
