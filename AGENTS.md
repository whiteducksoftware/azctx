# Working on azctx

azctx is a Go CLI for switching Azure CLI subscriptions and wrapping Azure login.

## Start and validate

- Read [CONTRIBUTING.md](CONTRIBUTING.md) for setup and stacked PR delivery.
- Use `go.mod`: Go 1.26 minimum, preferred toolchain Go 1.27.3; allow automatic toolchain selection.
- `make check` runs lint and tests; pass it before a PR leaves draft.
- `make test` runs race-enabled tests with coverage; `make build` builds azctx.
- `make golden` updates the picker screens at 60, 100 and 160 columns; review the diff.

## Package map

- `main.go`: executable error presentation and exit boundary.
- `cmd/`: Cobra command construction and orchestration with injected dependencies.
- `internal/azcli/`: Azure CLI runner, version check, login, read-only INI settings and cloud-aware subscriptions.
- `internal/profile/`: read-only `io/fs` Azure profile parser and models.
- `internal/match/`: subscription matching.
- `internal/ui/`: Bubble Tea/Bubbles list picker, Lip Gloss rendering and screen goldens.
- `internal/version/`: release ldflags and Go build metadata for `version -o text|json`.
- `internal/update/`: update checks.
- `internal/state/`: private previous-subscription and recent-ID storage.
- `internal/history/`: bounded identity-only audit of completed switches.
- `internal/storage/`: atomic private files and update-check process locks outside Azure configuration.
- `internal/config/`: strict version-1 YAML and shared private config paths.
- `internal/selection/`: read-only visibility/matching catalog without IO or prompts.
- `internal/testutil/`: recording runner and portable fake `az`.
- `log/`: plain stderr diagnostics through slog and Lip Gloss.

## Invariants and boundaries

- Never read or write credentials or the MSAL token cache.
- Only read Azure `azureProfile.json`, `config` and `clouds.config`; doctor may
  stat (never read) a leftover `azctxTenants.json`. Never directly write Azure
  files. The sole configuration-write exception is an explicitly confirmed
  `auth broker enable|disable` invoking `az account clear`, `az config set` for
  the platform broker key, then attached `az login` through the runner.
- Tenant metadata comes only from the profile; do not add tenant-cache or REST fallbacks.
- Check Azure CLI >=2.61.0 on login, not on switching; disable its picker only in the login child environment.
- Broker changes require >=2.61.0 on Windows or >=2.91.0 on macOS and explicit
  terminal confirmation or `--yes`; unsupported platforms, conflicting
  environment overrides and unconfirmed non-interactive runs never mutate.
- Doctor is profile-independent and offline: only `az version`, allowlisted
  settings and account-type counts; never raw process diagnostics or identities.
- Validate subscription and tenant IDs as GUIDs before passing them to `az`.
- Tests use injected runners or the fake `az`, isolated filesystems and fixtures; never a real tenant.
- Injected `io/fs.FS` values are read-only and rooted at the Azure config directory.
- Keep data on stdout, messages and the picker on stderr; Ctrl+C propagates to exit 130.
- Command names, flags, JSON output and exit codes are public contracts.
  Obtain a code owner decision before changing them.
- Keep changes scoped to the task and preserve unrelated behavior.
- Do not hand-edit generated `docs/cli/` when present.
- Do not touch release workflows, GoReleaser publishers or secrets.

## Delivery

- Use conventional commits and one release layer per PR.
- Add a row to [docs/upgrade.md](docs/upgrade.md) for user-visible changes.
- Follow the contributor guide for explicit issue linkage on stacked PRs.
- Guidance here complements, not replaces, rulesets, code owner review and required checks.
