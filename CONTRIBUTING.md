# Contributing to azctx

## Setup and checks

Install Git, Make and Go as declared in [go.mod](go.mod): the source minimum is
Go 1.26 and the preferred toolchain is **Go 1.27.3**. Keep automatic toolchain
selection enabled (the default, `GOTOOLCHAIN=auto`). The preferred patch supersedes
the release plan's original 1.27.2 pin for a security fix; see
[the upgrade guide](docs/upgrade.md).

Install golangci-lint v2.15.0 on your `PATH`, for example:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
```

Include your `GOBIN` directory, or `$(go env GOPATH)/bin` when `GOBIN` is unset,
in `PATH`. Confirm `go version` and `golangci-lint version` before starting.
From the repository root:

```sh
make check  # Lint and race-enabled tests with coverage
make test   # Tests alone
make build  # Build the azctx executable
make golden # Update picker screen goldens; review and commit intentional changes
```

The [Makefile](Makefile) is the source of truth for available targets.
Tests use injected dependencies and a portable fake `az`; no Azure CLI
installation, Azure login, real tenant or credentials are needed for checks.
Keep fixtures synthetic and follow the boundaries in [AGENTS.md](AGENTS.md).
Picker tests run real Bubble Tea programs with bounded waits and synthetic key
events at 60, 100 and 160 columns; they do not need an interactive terminal.
After `make golden`, run `make check` and confirm a second `make golden` leaves
the reviewed golden files unchanged.

## Changes and review

- Keep each PR focused; the 2026.11 release uses one layer per PR, based on the
  preceding layer rather than `main`. Confirm the parent branch in the layer's issue.
- Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/),
  for example `docs: add contributor guidance` or `fix: preserve subscription selection`.
- Command names, flags, JSON output and exit codes require a code owner decision
  before changing the public contract.
- Add regression tests for behavior changes and a row to
  [docs/upgrade.md](docs/upgrade.md) for user-visible changes.
- Fill in the PR template, keep the PR in draft until `make check` passes, and
  resolve required CI checks and code owner review before merging.
- Report vulnerabilities through [SECURITY.md](SECURITY.md), not public issues.

## Stacked pull requests

Use the [GitHub CLI](https://cli.github.com/) authenticated to your own account.
The commands below publish branches and PRs; run them only when ready to share.
Coordinate history rewrites with everyone using the affected branches.

### Native GitHub stacks with `gh stack`

The official [gh-stack extension](https://github.com/github/gh-stack) provides
`gh stack`; it may need installing even on a recent `gh`:

```sh
gh extension install github/gh-stack
```

For a new stack, start on an up-to-date `main` with a clean worktree:

```sh
git switch main
git pull --ff-only
gh stack init --base main feature/first-layer
# Implement the first layer, run make check, and commit.
gh stack add feature/next-layer
# Implement the next layer, run make check, and commit.
gh stack view
gh stack submit --auto
```

`submit --auto` pushes the branches and creates draft PRs with the correct
parent bases. Complete each PR's description and issue linkage below. To join
an existing native stack, use `gh stack checkout` with its parent PR number,
then `gh stack top` and `gh stack add` for the new branch.
Use `gh stack sync` to fetch, rebase and push an existing stack; it is not a
read-only check. Rerun `make check` on affected layers after rebasing.

### Classic stacked PRs (current release delivery)

Use this fallback when the extension or native stack access is unavailable.
The 2026.11 release currently uses ordinary branches and PRs with explicit
parent bases; do not migrate an in-flight stack without maintainer coordination.
For example, to create a **new** layer-04 branch on layer 03:

```sh
git fetch origin
git switch -c 2026.11/04-contributing origin/2026.11/03-structure
# Implement only layer 04, run make check, and commit.
git push -u origin 2026.11/04-contributing
gh pr create --draft --base 2026.11/03-structure --head 2026.11/04-contributing
```

Fill in the PR template rather than copying the whole parent diff into the PR.
If the parent changes, save its old fetched tip **before** fetching, then replay
only the child's commits onto the new parent:

```sh
old_parent=$(git rev-parse origin/2026.11/03-structure)
git fetch origin
git rebase --onto origin/2026.11/03-structure "$old_parent" 2026.11/04-contributing
make check
git push --force-with-lease origin 2026.11/04-contributing
```

Keep the saved old parent tip when coordinating a parent squash or rewrite.
After the parent merges, replay only the child commits onto the updated
`origin/main` using `git rebase --onto` with that old parent tip, then check,
push with `--force-with-lease`, and retarget the child's PR with
`gh pr edit --base main`. Repeat bottom-up; never merge a child with an
unmerged prerequisite.

### Explicit issue linkage

Put the issue number and parent PR in every PR description. A mention alone is
not a Development link. **Closing keywords such as `Closes #85` are ignored
when a PR targets a non-default branch.** Someone with repository write access
must also link the PR explicitly from the issue's **Development** sidebar
(select the repository and PR, then **Apply**). For layer 04, link issue #85 and
identify parent PR #103.
Verify the link on the issue; do not assume a native stack creates it.

## AI-assisted contributions

AI-assisted contributions are welcome. The human opening the PR is responsible
for understanding and reviewing every change, verifying its provenance and
license compatibility, and personally running `make check`. Disclose material
AI assistance in the PR summary. Do not provide agents with credentials,
token caches or private tenant data.

The [Copilot setup workflow](.github/workflows/copilot-setup-steps.yml) prepares
Go and golangci-lint, and validates setup changes on PRs. Copilot cloud agent
starts using it only after it reaches the default branch; manual dispatch is
also available once the workflow is on that branch.
