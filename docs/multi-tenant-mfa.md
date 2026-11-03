# Many tenants and MFA

This guide describes the unreleased source contract. Authentication remains
in Azure CLI and no live multi-tenant acceptance has been verified. Start with
[authentication](authentication.md) for prerequisites, broker settings and the
full failure/confirmation matrix.

## Start with ordinary login

```sh
azctx login
azctx login --use-device-code
```

The initial attached `az login` may authenticate some tenants and report
failures for others. azctx recognizes documented per-tenant stderr warnings,
not a substring search for the word MFA. It validates tenant GUIDs before
using them in a child command and reports failures on stderr.

| Reported AADSTS code | Action |
| --- | --- |
| 50076 | MFA needed, eligible for one targeted retry |
| 50079 | MFA registration needed, eligible for one targeted retry |
| 53003 | Conditional Access blocked issuance, never retried |
| Other | Original diagnostic retained, subject to heuristic redaction; no automatic retry |

With azctx input enabled, one default-yes confirmation covers all eligible
tenants. Retries use `az login --tenant GUID`, preserving the device-code and
other explicit options. A tenant is attempted once; known non-retryable
failures take precedence over duplicate retryable warnings. Already successful
tenants are not retried. Azure CLI merges its own subscription cache.

Initial unresolved failures without azctx input return **3**, without prompting,
retrying or discovering more tenants. Declined, partial and failed sign-ins also
return 3. Ctrl+C returns 130. A completed run reports reloaded profile totals,
not proof that all cached tenants authenticated in that run.

## Explicit tenant choices

```sh
azctx login --tenant aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
azctx login --tenant example.invalid --use-device-code
azctx login --all-tenants --use-device-code
```

A GUID works on first login; a friendly name/domain requires one matching
active-cloud profile tenant. Ambiguity fails before a subprocess starts.
`--all-tenants` explicitly opts into GUID discovery through
`az account tenant list -o json`. That command must already be available,
including its account extension where required. azctx never installs it.
Do not combine discovery with a tenant selector.

After successful initial login without failures, explicit discovery may run
without azctx input; it still uses attached Azure CLI authentication, not a new
unattended login mode. `--no-input`, `CI`, redirected stdin and
`AZCTX_NO_INPUT=1` gate azctx's prompts, not Azure CLI's.

```sh
azctx login -- --identity
azctx login --all-tenants -- --identity
```

Explicit managed-identity (`--identity` or `-i`) and `--service-principal`
passthrough run one Azure login without tenant discovery or MFA recovery,
even if `--all-tenants` is set. Configure workload identity separately; never
put secrets in argv. See [CI/CD](ci-cd.md).

## Select after login

```sh
azctx tenant
azctx tenant example.invalid
azctx aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/11111111-1111-1111-1111-111111111111
```

Tenant selection uses token tenant GUID identity, not a possibly duplicated
display name. A unique tenant with one selectable subscription switches
directly; otherwise use the picker or an exact qualified query.
Non-interactive ambiguity exits 2 with candidates. `--all` reveals hidden
configuration entries, but disabled, tenant-level and foreign-cloud records
remain inspection-only. Different nonempty home/token tenant IDs indicate
Lighthouse delegation; metadata is read only from the Azure CLI profile.

## Remaining manual acceptance

The offline warning fixtures are synthetic. A maintainer still needs three
test tenants, including two enforcing MFA, to verify one confirmation,
targeted sign-ins, subscription merging, policy blocks and final totals.
No identities, real MFA session or broker enrollment was provisioned here.
See [the live checklist](authentication.md#live-acceptance-checklist-unverified).
