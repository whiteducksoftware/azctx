# Synthetic cross-tenant authentication diagnostics

These exact-LF fixtures follow Microsoft's documented
[`Authentication failed against tenant <id> '<name>': AADSTS<code>` format](https://learn.microsoft.com/cli/azure/authenticate-azure-cli-mfa).
All IDs, names, traces and timestamps are synthetic, not captured from identities.
Both bare and `WARNING: `-prefixed lines are intentional.

The [Entra error reference](https://learn.microsoft.com/entra/identity-platform/reference-error-codes)
classifies 50076 as MFA required, 50079 as registration required, and 53003 as
Conditional Access blocking token issuance. The 50105 fixture represents a
different error and must remain verbatim, without an automatic retry.

No live test tenants were available. These fixtures verify parsing and
orchestration only; live acceptance remains unverified.
