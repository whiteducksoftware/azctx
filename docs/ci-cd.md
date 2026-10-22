# CI/CD and project contexts

Examples document the unreleased 2026.11 contract and assume a trusted
azctx 2026.11 binary and Azure CLI >=2.61.0 are already installed in the runner
image or devcontainer. Replace the synthetic GUIDs and connection names.
No live OIDC, workload-identity or deployment acceptance has been performed.
Identities, federated credentials, permissions and service connections must be
provisioned separately by an authorized maintainer. They were not created by
this stack. [Release readiness remains blocked](releasing.md).

## Context is not authentication

`azctx env [query]` exports exactly these four context variables, in this order:

```text
ARM_SUBSCRIPTION_ID
ARM_TENANT_ID
AZURE_SUBSCRIPTION_ID
AZURE_TENANT_ID
```

Terraform azurerm 4.x requires a subscription ID (provider `subscription_id` or
`ARM_SUBSCRIPTION_ID`). azd and supporting SDK tools can use the `AZURE_*`
context variables. These variables are **not credentials** and do not log in,
grant access, select a cloud or configure a workload-identity token source.
Configure each tool's authentication separately.

**Azure CLI (`az`) ignores these ARM/AZURE context variables.** An `az`
command still uses Azure CLI's shared default unless you pass
`--subscription`. This also applies inside `azctx exec`.

Exports use the actual current subscription when there is no query. A query
uses the same exact ID, name/alias, qualified and fuzzy matching catalog as
selection, without switching. Configured hiding applies to queries (`--all`
exposes it); disabled, tenant-level and other/unknown-cloud accounts remain
ineligible. Ambiguity always exits 2, even on a terminal. Protected targets
do not need switch confirmation because nothing is switched.
Exported subscription and token tenant IDs must both be valid GUIDs.

For the current shell, check export success *before* evaluation:

```bash
set -euo pipefail
context_env="$(azctx env --shell bash)"
eval "$context_env"
az group list --subscription "$AZURE_SUBSCRIPTION_ID"
```

`--shell bash` is the default; `zsh`, `fish` and `powershell` are also supported.
All outputs use LF. Values are quoted for their shell and validated as GUIDs;
profile names, aliases and project contents never become executable shell code.
`--shell` and pipeline `--format` cannot be combined.

## Project guard: fail closed, never auto-switch

Put this reviewed file in a project directory:

```yaml
# .azctx.yaml
version: 1
tenant: aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
subscription: 11111111-1111-1111-1111-111111111111
```

`azctx guard` searches from the current directory through all ancestors,
including the filesystem root, and checks the nearest `.azctx.yaml`.
All three keys above are required. Unknown/duplicate keys, wrong scalar types,
non-GUID IDs, unsupported versions and additional YAML documents are errors.
A nearer malformed file is an error, not permission to use a farther file.
Files must be regular files (not symlinks) and at most 64 KiB.
Diagnostics identify the filename and, for field/syntax errors, the line.

- Match: exit 0, no stdout or stderr.
- Different subscription or token tenant: exit **4**.
- Missing file, unreadable/malformed file or unavailable active context: exit 1.
- `-o json` errors use the existing error envelope on stderr; mismatch code is
  `context_mismatch`. Successful JSON guards are also silent.

Explicit flags replace the project expectation entirely; omitted dimensions
are not checked. GUID flags can name an identity absent from the profile and
still return mismatch 4. Non-GUID subscription queries use the catalog;
tenant queries match GUID/name/domain tiers. No match/ambiguity exits 2.

```bash
azctx guard \
  --subscription 11111111-1111-1111-1111-111111111111 \
  --tenant aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
```

A cloned repository is **untrusted input**. azctx never switches, logs in,
records history, modifies previous/recent state or checks updates because of a
project file. Neither `env` nor `exec` uses it to choose a target.
`azctx -c` warns on stderr for a mismatch or invalid project file only in human
default/text mode; it still prints the actual current context and succeeds.
JSON and `-o id|name|tenant` do not read the project file or emit these warnings.
A guard is a point-in-time check, not a lock against another process changing
Azure CLI's default; pass `--subscription` on every Azure CLI operation.

## GitHub Actions: Azure Login with OIDC

Configure an Entra application or user-assigned managed identity with a
federated identity credential scoped to this repository and protected
branch/environment, plus least-privilege Azure RBAC. Store its identifiers in
the named Actions secrets. No client secret/password is needed.
Use protected environments for deployment approval; pin actions to reviewed
commit SHAs in production (major tags below are illustrative).

```yaml
name: Inspect Azure context
on: workflow_dispatch
permissions:
  contents: read
  id-token: write
jobs:
  inspect:
    runs-on: ubuntu-latest
    environment: production
    env:
      EXPECTED_SUBSCRIPTION_ID: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
      EXPECTED_TENANT_ID: ${{ secrets.AZURE_TENANT_ID }}
    steps:
      - uses: actions/checkout@v4
      - uses: azure/login@v3
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
      - name: Verify and export context
        shell: bash
        run: |
          set -euo pipefail
          azctx guard --subscription "$EXPECTED_SUBSCRIPTION_ID" --tenant "$EXPECTED_TENANT_ID"
          azctx env --format github
      - name: Inspect the explicit subscription
        shell: bash
        run: |
          set -euo pipefail
          az group list --subscription "$AZURE_SUBSCRIPTION_ID"
```

`--format github` appends `KEY=GUID` lines to the runner's existing `$GITHUB_ENV`;
it prints nothing on stdout. Variables become available to **subsequent steps**,
not the shell running the export. A missing/relative/unwritable/nonregular file
or a destination inside Azure configuration is an explicit error; azctx does not
create the file or fall back to printing exports. Symlink/hard-linked files are
rejected. Existing contents are retained, adding a separator LF if needed, by
atomic same-directory replacement with a private file. The parent directory's
permissions are unchanged; the existing file is limited to 8 MiB. Do not share
one environment file between concurrent writers.

## Azure DevOps: workload identity service connection

Create an **Azure Resource Manager service connection using workload identity
federation**, scope its RBAC and authorize only the required pipeline.
`AzureCLI@2` performs federated sign-in; azctx does not retrieve or print its
tokens. Configure the expected GUID pipeline variables independently of the
repository. Run azctx within the task so it sees the task's isolated
`AZURE_CONFIG_DIR`; do not replace it with the user's global Azure directory.

```yaml
pool:
  vmImage: ubuntu-latest
variables:
  expectedSubscriptionId: '11111111-1111-1111-1111-111111111111'
  expectedTenantId: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
steps:
  - checkout: self
  - task: AzureCLI@2
    displayName: Verify workload identity context
    inputs:
      azureSubscription: 'reviewed-workload-identity-connection'
      scriptType: bash
      scriptLocation: inlineScript
      useGlobalConfig: false
      inlineScript: |
        set -euo pipefail
        azctx guard --subscription "$EXPECTED_SUBSCRIPTION_ID" --tenant "$EXPECTED_TENANT_ID"
        azctx env --format azure-devops
        context_env="$(azctx env --shell bash)"
        eval "$context_env"
        az group list --subscription "$AZURE_SUBSCRIPTION_ID"
    env:
      EXPECTED_SUBSCRIPTION_ID: $(expectedSubscriptionId)
      EXPECTED_TENANT_ID: $(expectedTenantId)
```

`--format azure-devops` prints four `##vso[task.setvariable variable=KEY]GUID`
logging commands. They set variables for later tasks in the same job, not the
current shell; the explicit checked shell export above covers that shell.
They do not transport authentication to another task/job. Run later Azure
commands in their own authenticated `AzureCLI@2` task, with `--subscription`.
Cross-job output variables are not emitted by this format.

## Devcontainer: interactive device code

Install the trusted tools in the devcontainer image. Use an interactive
terminal; do not put device-code login in an unattended container lifecycle hook.
Keep Azure configuration out of the image/repository and do not print tokens.
Device code does not bypass Conditional Access or organizational policy.

```bash
set -euo pipefail
EXPECTED_TENANT_ID=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
EXPECTED_SUBSCRIPTION_ID=11111111-1111-1111-1111-111111111111
azctx login --use-device-code --tenant "$EXPECTED_TENANT_ID"
azctx guard --subscription "$EXPECTED_SUBSCRIPTION_ID" --tenant "$EXPECTED_TENANT_ID"
context_env="$(azctx env --shell bash)"
eval "$context_env"
az group list --subscription "$AZURE_SUBSCRIPTION_ID"
```

If guard exits 4, inspect `azctx list` and explicitly choose the intended
subscription yourself, then rerun guard. Do not automatically switch or sign
in based on the cloned `.azctx.yaml`.

## One command without changing the parent shell

```bash
azctx exec 11111111-1111-1111-1111-111111111111 -- terraform plan
azctx exec 11111111-1111-1111-1111-111111111111 -- \
  az group list --subscription 11111111-1111-1111-1111-111111111111
```

`exec` resolves the command once through `exec.LookPath`, never through a shell.
Exactly one query precedes the required `--`; every subsequent argument is
literal. To expand shell variables/pipelines, invoke a shell **explicitly** and
take responsibility for its quoting; azctx never constructs shell code.
The child inherits the environment except the four context values; the parent
is unchanged. Stdin/stdout/stderr stay attached. Child status is passed through,
with no extra azctx error or JSON object after the child's diagnostics.
`-o json` only formats azctx preflight failures, not child output.

Unix INT/TERM/HUP/QUIT are forwarded to the direct child; Unix signal exits are
`128 + signal` (Ctrl+C: 130). Windows Ctrl+C is forwarded to the child's console
process group using Ctrl+Break; no-console cancellation falls back to termination.
Context cancellation requests interruption, with a two-second kill fallback,
and returns 130. No process-tree supervisor or shell job control is promised.
azctx does not run `account set`, mutate its own state/history or check updates.
The explicitly launched command can of course have its own side effects.
Do not run untrusted commands simply because guard passed.

## References

- [Azure Login OIDC prerequisites and workflow](https://github.com/Azure/login#login-with-openid-connect-oidc-recommended)
- [GitHub environment files](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands#setting-an-environment-variable)
- [Azure Resource Manager workload identity service connection](https://learn.microsoft.com/en-us/azure/devops/pipelines/library/connect-to-azure)
- [AzureCLI@2 task and isolated configuration](https://learn.microsoft.com/en-us/azure/devops/pipelines/tasks/reference/azure-cli-v2)
- [Azure Pipelines logging commands](https://learn.microsoft.com/en-us/azure/devops/pipelines/scripts/logging-commands#sets-a-variable-from-a-script)
- [Terraform azurerm 4.0 upgrade guide](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/guides/4.0-upgrade-guide)
