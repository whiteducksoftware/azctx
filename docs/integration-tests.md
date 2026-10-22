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

### Completion performance acceptance

`make test` (and therefore `make check` and every CI test matrix leg) first runs
the full correctness/race/coverage suite, then requires `make completion-perf`.
The latter runs the `completionperf`-tagged test once, uncached, with race and
coverage instrumentation explicitly disabled and inherited `GOFLAGS` cleared.
It uses the same 500-subscription fixture and offline correctness checks as
`TestCompletion500SubscriptionsIsOffline` in the instrumented suite: the Azure
profile/settings filesystem is injected (`fstest.MapFS`), while azctx config
is an isolated real file. This measures native completion callback latency,
not process startup or real Azure profile disk latency.

Every one of ten completion calls, including the first, must take **under
50 ms**. There is no warm-up exclusion, averaging, retry or relaxed threshold.
The timed boundary remains `ValidArgsFunction`: it includes profile/settings
and azctx config reads, profile JSON decoding, visibility filtering, candidate
generation and sorting. Fixture construction and correctness assertions are
outside the timer. All 1,499 expected sorted candidates (499 visible subscriptions'
names and tenant-qualified names, plus the domain-qualified name and alias),
hidden/`--all` behavior, prefix matching, no Azure/update calls and no new
config/state files are checked in both runs.

Race instrumentation substantially increases JSON/profile-read costs; atomic
coverage counters and concurrent package tests can add further overhead. Those
diagnostic costs are not the latency of the shipped binary. Separating the
mandatory native gate keeps that measurement truthful without excluding any
correctness checks from the instrumented suite. It does not eliminate shared
runner scheduling noise: a native call at or above 50 ms still fails CI.

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
`make ci-check` also runs the offline tests of the manual live script and its
container shim with recording fake azctx/az/docker processes, and a workflow
policy test proving that no workflow signs in to Azure or runs the live script.
`make check` runs the live script through the portable fake executable on all
three platforms: success, refusal without opt-in, foreign tenant, invalid
schema, a failing guard with restoration and a failed restore. Those adapter
checks are not live Azure acceptance.

## Manual live Azure test

Live Azure checks are a **manual maintainer action**. No workflow signs in to
Azure, holds an Azure identity or runs `scripts/live-azure-test.py`; the script
refuses to start when `CI`, `GITHUB_ACTIONS` or `TF_BUILD` is set. Native CI
keeps the offline suites above. The script has not been run against a real
tenant during implementation.

### Prerequisites

1. A **dedicated non-production AzureCloud test tenant**, never a customer or
   production tenant, with two enabled subscriptions you may switch between.
2. An existing Azure CLI sign-in on your machine (`az login`) to that tenant,
   with the current subscription set to one of the two test subscriptions.
   The script never signs in, signs out or reads, copies or mounts credentials.

### Run it

```sh
make live-test LIVE_TEST_ARGS='--opt-in-live-azure --tenant <tenant-guid> \
  --subscription <first-guid> --subscription <second-guid>'
```

`make live-test` builds `./azctx`, then runs the script. `--opt-in-live-azure`,
a nonzero tenant GUID and exactly two distinct subscription GUIDs are required;
anything else is refused before a process starts. The script first reads the
current context of every requested Azure CLI and refuses unless it is signed in
to `--tenant` in AzureCloud on one of the two subscriptions. It then lists both
subscriptions, switches to each, checks `-c`, the four `env --shell bash`
exports, a quiet matching guard and a mismatching guard's exit 4 JSON error,
and finally switches back to the initial subscription and verifies it.

Each leg uses a fresh private azctx state directory (`AZCTX_CONFIG_DIR`) with
input and update checks disabled; the work directory is removed afterwards.
Output never echoes Azure CLI or azctx diagnostics: failures name the azctx
command, its exit code and, at most, azctx's JSON error code.

| Exit | Meaning |
| --- | --- |
| 0 | All requested legs passed and were restored |
| 1 | A check failed; the initial context was restored (or never changed) |
| 2 | Refused or usage error; nothing was changed |
| 3 | **RESTORE FAILED**: run the printed `az account set` command yourself |
| 130/143 | Ctrl+C or SIGTERM; the initial context was restored (or never changed) |

Interrupts never abort restoration: a signal is recorded, the current command
finishes, the initial subscription is restored and verified, then the script
exits. A timed-out command (120 s) is killed and counts as a failure.

### Optional Azure CLI 2.61.0 and latest containers

Without `--container`, the summary prints **NOT RUN ... unverified** for the
minimum supported (2.61.0) and latest Azure CLI. To cover them, prepare each
container leg yourself; the script never pulls images or signs in:

```sh
docker pull mcr.microsoft.com/azure-cli@sha256:b83a4620937eccf09244e4dfbf310f774b01b3083b7accf49ab74d77247c8043
docker pull mcr.microsoft.com/azure-cli:latest
mkdir -m 700 ~/azctx-live/cli-2.61.0 ~/azctx-live/cli-latest
AZURE_CONFIG_DIR=$HOME/azctx-live/cli-2.61.0 \
  AZCTX_AZURE_CLI_IMAGE=mcr.microsoft.com/azure-cli@sha256:b83a4620937eccf09244e4dfbf310f774b01b3083b7accf49ab74d77247c8043 \
  scripts/az-container.sh login --tenant <tenant-guid> --use-device-code
# Repeat for latest with its local digest:
# docker image inspect --format '{{json .RepoDigests}}' mcr.microsoft.com/azure-cli:latest
make live-test LIVE_TEST_ARGS='--opt-in-live-azure --tenant <tenant-guid> \
  --subscription <first-guid> --subscription <second-guid> \
  --container 2.61.0=$HOME/azctx-live/cli-2.61.0 --container latest=$HOME/azctx-live/cli-latest'
```

Each `--container VERSION=DIR` must be an absolute, existing, non-symlink
directory owned by you with mode `0700`, different for each leg and neither
inside nor containing `~/.azure` or the host `AZURE_CONFIG_DIR`. The 2.61.0 leg
requires the local image to carry the pinned digest and report exactly 2.61.0;
latest must resolve locally to exactly one MCR digest reporting at least
2.61.0. All image and version checks, then every leg's tenant check, happen
before the first switch. azctx runs on the host; `scripts/az-container.sh` is
put first on its PATH as `az` and runs the CLI with `--pull=never`, all
capabilities dropped, `no-new-privileges`, your UID:GID, only that config
directory mounted and `HOME` set to it. The shim refuses the host `~/.azure`.
The 2.61.0 image is Alpine-based and retired upstream; it only runs the CLI.
Sign out of or delete these dedicated directories when you are done.

### Boundaries and acceptance

Only the Azure CLI manages its own sign-in and config files. The script and
azctx never read credentials or MSAL caches, write Azure files directly or
issue Azure resource writes; `az account set` changes only the local CLI
profile. Live acceptance means a maintainer ran the script successfully for
the host and both container legs against the dedicated tenant. A NOT RUN leg
or offline fake run is not live acceptance. Interactive MFA stays manual.

Sources:

- [Official Azure CLI Docker images and Alpine retirement](https://learn.microsoft.com/en-us/cli/azure/run-azure-cli-docker)
