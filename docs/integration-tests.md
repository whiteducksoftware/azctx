# Integration tests

## Credential-free command suite

`make e2e` builds and runs the real azctx binary with the existing portable Go
fake `az` on PATH. It selects **all `TestBinary*` tests in `cmd/`**, not a second
harness. Each process has isolated Azure/profile, user/state and fixture paths,
bounded execution, synthetic GUIDs and disabled update checks. No installed
Azure CLI, sign-in or tenant is needed.

The CI `test` matrix runs `make test` and `make e2e` natively on Linux, macOS and
Windows. Compilation for another OS is not a substitute for those native jobs.

| Command surface | Real-binary coverage |
| --- | --- |
| Bare command, list and current (`-c`) | `TestBinaryContractMatrix`, `TestBinaryContractNoninteractiveVariants`, `TestBinaryFakeAZRoundTrip` |
| Query and previous (`-`) | `TestBinaryPreviousSubscriptionAcrossProcesses`, `TestBinaryContractCommandNamesAndDelimiter` |
| Tenant and history | `TestBinaryTenantSwitchAndHistoryIsolation` |
| Login | `TestBinaryDeviceCodeVisibleBeforeAZExits` (live attached synthetic prompts) |
| Auth broker status, enable and disable | `TestBinaryDiagnosticsAndBroker` (confirmed fake calls on macOS/Windows; rejection on Linux) |
| Doctor text/JSON | `TestBinaryDiagnosticsAndBroker` |
| Config and alias set/list/rm | `TestBinaryConfigAliasRoundTrip`, `TestBinaryConfigAliasProtectedSelection` |
| Env, exec and guard | `TestBinaryEnvironmentExecGuardNeverMutateContext`, `TestBinaryExecRawStreamsAndStatus` |
| Version | `TestBinaryVersionLDFlags` |

The exec test deliberately preserves arbitrary child stdout/stderr bytes,
including token-shaped synthetic strings and NULs, and status 37 even with
JSON output requested. Read-only env/exec/guard tests snapshot Azure and azctx
files and require no Azure runner invocation. Existing agent hook, manifest,
documented-command, signal and PTY tests remain part of `make check`.

## Coverage policy

`make test` writes `coverage.out` with the race-enabled Go suite and runs
`.github/scripts/check_coverage.py`. Cross-package instrumentation (`-coverpkg=./...`)
counts internal code exercised by command tests as well as its own package tests;
separately built subprocess binaries are not instrumented. The gate is **70% of statements across all
packages below this module's `internal/`**, weighted by each block's statement
count, not an average of package percentages. A block is covered if its
execution count is nonzero. All internal packages, including test helpers, are
included. Repeated blocks from cross-package test binaries union their hits and
are weighted only once; inconsistent statement counts fail. There is **no
`cmd/` gate**. An empty or malformed profile fails closed, and rounding cannot
turn a value below 70% into a pass.

CI publishes the raw `coverage.out` per OS (seven-day retention); locally use
`go tool cover -func=coverage.out` or `go tool cover -html=coverage.out`.
`make ci-check` runs offline gate, schema, container-shim, workflow and
vulnerability-policy regressions, including negative cases.
`make check` also runs the nightly script through the portable fake executable:
success, invalid schema, foreign tenant and a failing guard, including context
restoration after failure. Those adapter checks are not live Azure acceptance.

## Opt-in real Azure CLI nightly

`.github/workflows/integration.yaml` schedules 03:37 UTC daily and supports a
manual dispatch **on the upstream main branch only**. It never handles PRs,
forks, arbitrary image/ref inputs or dispatch-supplied identities. Disabled
integration is explicitly **SKIPPED / UNVERIFIED**, not a successful live test.
No dedicated tenant was supplied during implementation, so neither real matrix
leg nor failure notification delivery has been verified.

A maintainer must provision and review these prerequisites separately:

1. A **dedicated non-production AzureCloud test tenant**, never a customer or
   production tenant, with at least two enabled subscriptions in that tenant.
2. A federated service principal with **Reader only** on both subscriptions.
   No client secret or password is needed. Configure GitHub OIDC trust for
   `repo:whiteducksoftware/azctx:environment:azure-integration`, issuer
   `https://token.actions.githubusercontent.com`, audience
   `api://AzureADTokenExchange`.
3. A GitHub environment named `azure-integration`, restricted to the upstream
   `main` branch (and reviewers if required by the organization). Do not broaden
   federation to PRs/forks or use an identity with resource write permissions.
4. Set these **repository variables**, not secrets or workflow inputs:

   | Variable | Required value |
   | --- | --- |
   | `AZCTX_TEST_CLIENT_ID` | Federated application/client GUID |
   | `AZCTX_TEST_TENANT_ID` | Dedicated test tenant GUID |
   | `AZCTX_TEST_SUBSCRIPTION_ID` | First test subscription GUID, initial default |
   | `AZCTX_TEST_SECOND_SUBSCRIPTION_ID` | Distinct second subscription GUID |
   | `AZCTX_INTEGRATION_DEDICATED_TENANT` | Exact `true`: maintainer attests the dedicated, non-production prerequisite |
   | `AZCTX_INTEGRATION_ENABLED` | Exact `true`: explicit opt-in after reviewing all prerequisites |

When enabled, missing/invalid IDs, duplicate subscriptions or a missing tenant
attestation fail before the OIDC job. The script revalidates the opt-in before
executing even the binary. Disabling the last variable skips all Azure work.
These public identifiers are configuration, not credentials; restrict who can
edit repository variables and the workflow.

### Container and permission boundaries

Both Azure CLI **2.61.0** and **latest** execute inside official
`mcr.microsoft.com/azure-cli` containers. The 2.61.0 manifest is digest-pinned;
latest is intentionally pulled fresh, resolved to an MCR digest and logged
with its actual CLI version in the job summary. Every subsequent invocation
uses that immutable digest, including login, account switching and logout.
Re-review the legacy digest when changing the compatibility baseline.

The old 2.61.0 image is Alpine-based; Microsoft stopped maintaining Alpine CLI
images after 2.63.0. Do not assume it can run the Node24 runtime of current
checkout/login actions or install a different CLI into it. Instead all pinned
JavaScript actions run on the Ubuntu host. A PATH shim invokes **only the
container CLI** for both `azure/login` and azctx, sharing a fresh, dedicated
`AZURE_CONFIG_DIR` mount. No Node runtime is needed inside either CLI image.
Only that Azure directory is mounted, never the host home or Docker socket.
The shim uses the host user's exact UID:GID so the container can access the
host-owned `0700` config directory without creating root-owned files. Container
`HOME` points to that same writable directory. Containers still drop all
capabilities and disallow privilege escalation.

The shim leaves Azure CLI's default JSON output enabled: Azure Login v3.1.0
invokes `az version` without an output flag and parses its JSON stdout. Host
`AZURE_CORE_OUTPUT` overrides are not forwarded; explicit `-o`/`--output` flags
are passed through unchanged. Login/account-set output is suppressed by the
login action itself, not by changing the CLI's default output format. Offline
Docker-recording tests check these boundaries; they do not execute containers
or prove Linux filesystem permissions or live OIDC acceptance.

OIDC `id-token: write` exists only on the `azure` matrix job, which otherwise
has `contents: read`. The prerequisite job has only read access. A separate
failure-only job has only `issues: write`: it opens or comments on a fixed
failure issue and mentions the current CODEOWNERS with a run link, never Azure
output. Every job starts with audit-mode runner hardening; checkouts do not
persist credentials and privileged Go setup disables caching.

### Live assertions and acceptance

The host builds the static azctx binary and the official login action signs
in through the shim. The check script uses azctx's schema-1 JSON to verify both
configured subscriptions and their tenant/cloud before switching. It switches
to each test subscription, confirms current context, verifies all four env
exports, accepts the matching guard and requires exit 4 plus the JSON error
contract for a mismatching guard. The mismatch must not switch. A `finally`
step restores and verifies the initial test context.

Only Azure CLI may manage its own login/configuration files. Neither azctx nor
the check scripts read credentials/MSAL caches, directly write Azure files or
issue Azure resource writes. Azure Login's post-job cleanup logs out through
the same shim. **Never upload `.integration/`, Azure files or login output**;
the disposable hosted runner is the final filesystem cleanup boundary.

Acceptance still requires actual native CI success on all three OSes and a
green, opt-in real run for **both** container versions, plus verified issue
notification permissions/delivery. Interactive MFA stays manual and out of
scope. A skipped nightly does not satisfy these requirements.

Sources:

- [Official Azure CLI Docker images and Alpine retirement](https://learn.microsoft.com/en-us/cli/azure/run-azure-cli-docker)
- [Azure Login v3.1.0 OIDC and action runtime](https://github.com/Azure/login/tree/v3.1.0)
- [GitHub OIDC with Azure](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-azure)
- [Workload identity federation](https://learn.microsoft.com/en-us/entra/workload-id/workload-identity-federation)
