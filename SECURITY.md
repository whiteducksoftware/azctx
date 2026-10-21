# Security policy

## Reporting a vulnerability

Do not report suspected vulnerabilities, credentials or token-cache contents
in public issues, pull requests or discussions.

Use GitHub's private vulnerability reporting page:

**https://github.com/whiteducksoftware/azctx/security/advisories/new**

Private reporting requires a repository administrator to enable it; enablement
is pending and this policy does not assert that it is already available.
If the page does not offer private reporting, ask the maintainers listed in
[CODEOWNERS](.github/CODEOWNERS) to arrange a private channel, without sharing
vulnerability details publicly.

In the private report, include affected azctx versions, impact, sanitized
reproduction steps and any proposed fix. Use synthetic data; do not attach real
credentials, access tokens, Azure profiles or MSAL token caches. Coordinate
disclosure with the maintainers before publishing details.
