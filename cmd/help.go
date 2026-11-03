package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

type commandHelp struct {
	short    string
	long     string
	examples string
}

// Keep help metadata separate from handlers so documentation needs no Azure IO.
func configureHelp(root *cobra.Command) {
	root.InitDefaultHelpCmd()
	// Cobra captures completion output at initialization. Forward it lazily so
	// callers can still SetOut after constructing the tree, as with late setup.
	root.SetOut(commandOutput{root})
	root.InitDefaultCompletionCmd()
	root.SetOut(nil)
	help := map[string]commandHelp{
		"azctx": {
			"Switch Azure CLI subscriptions with an identity-aware fuzzy picker",
			"Choose a subscription by name, alias, GUID or exact tenant/subscription query. Use - for the previous subscription and -- before a name that equals a command.\n\nWith terminal input, the bare command opens a picker on stderr. Without input it lists subscriptions on stdout. Successful switches are quiet; protected targets need confirmation or --yes. Use -c for current context and -o json for structured data/errors.",
			"azctx\nazctx contoso/prod\nazctx -",
		},
		"azctx list": {
			"List cached subscriptions without switching",
			"Read the Azure CLI profile in profile order. Default table output hides configured hidden, disabled, tenant-level and other-cloud accounts. --all includes them for inspection, not switching. Supports table and schema-1 JSON output; no Azure subprocess or network request is made.",
			"azctx list\nazctx list --all -o json",
		},
		"azctx tenant": {
			"Choose a tenant, then a subscription",
			"Match token tenant GUIDs, profile names or domains, then choose a selectable subscription within that tenant. Duplicate tenant names retain their GUID identities. A unique tenant with one subscription switches directly; non-interactive ambiguity exits 2. Bare no-input or explicit table/JSON output lists subscriptions instead.",
			"azctx tenant\nazctx tenant example.invalid\nazctx tenant --all --no-input",
		},
		"azctx history": {
			"Show the last 100 completed subscription switches",
			"Read private identity-only switch history, newest first, without Azure CLI or a profile. Supports text and separate schema-1 JSON output. Missing history is empty; corrupt history warns. This best-effort log records azctx switches only, not login or external changes.",
			"azctx history\nazctx history -o json",
		},
		"azctx env": {
			"Export context variables without changing the Azure CLI default",
			"Export ARM_SUBSCRIPTION_ID, ARM_TENANT_ID, AZURE_SUBSCRIPTION_ID and AZURE_TENANT_ID for the current context or one unambiguous query. Choose bash (default), zsh, fish or powershell syntax, or a mutually exclusive github/azure-devops pipeline format. GitHub appends to existing GITHUB_ENV for later steps.\n\nThese variables are not credentials. Azure CLI ignores them; pass --subscription to az. Check export success before evaluating shell output.",
			"azctx env --shell bash\nazctx env prod --shell powershell\nazctx env --format github",
		},
		"azctx exec": {
			"Run a command with an isolated context environment",
			"Resolve one query and run one executable after the mandatory -- delimiter, without a shell. Override only the four ARM/AZURE context variables in the child; do not switch Azure CLI or change the parent environment. Streams and child exit status pass through, including in JSON mode. This is not authentication, a sandbox or supervision of arbitrary descendants. Azure CLI still needs explicit --subscription.",
			"azctx exec prod -- terraform plan\nazctx exec dev -- azd env get-values",
		},
		"azctx guard": {
			"Check the active context against reviewed expectations",
			"Compare the actual active subscription and token tenant against explicit flags or the nearest strict .azctx.yaml. Flags replace project expectations entirely. Match is silent exit 0; mismatch exits 4; missing/malformed project exits 1; unresolved queries exit 2. Never switch, authenticate or prompt. Guard is a point-in-time check, not an authorization boundary.",
			"azctx guard\nazctx guard -o json\nazctx guard --subscription 11111111-1111-1111-1111-111111111111",
		},
		"azctx config": {
			"Print effective configuration or its path",
			"Print strict version-1 YAML with defaults, or the configuration filename with path. AZCTX_CONFIG_DIR overrides the azctx directory, never the Azure directory. Missing files use defaults; malformed files fail. The path form works even when the configuration is malformed. No Azure CLI or profile is needed.",
			"azctx config\nazctx config path",
		},
		"azctx alias": {
			"Manage subscription aliases",
			"Manage case-insensitive names mapped to subscription GUIDs in private config.yaml. Set and rm atomically rewrite canonical YAML while preserving other settings, not comments. Aliases never bypass ambiguity, cloud, visibility or protected-switch checks. No Azure CLI or profile is needed.",
			"azctx alias list\nazctx alias set prod 11111111-1111-1111-1111-111111111111",
		},
		"azctx alias set": {
			"Create or replace a subscription alias",
			"Map NAME to a validated subscription GUID, replacing an existing name ignoring case. Names must be nonempty and cannot contain /. Other settings are retained; YAML comments and formatting are not. This edits private azctx configuration only and does not switch.",
			"azctx alias set prod 11111111-1111-1111-1111-111111111111\nazctx alias set dev 22222222-2222-2222-2222-222222222222",
		},
		"azctx alias rm": {
			"Remove a subscription alias",
			"Remove NAME ignoring case from private config.yaml without switching. A missing alias is an error. Other settings survive the canonical atomic rewrite; comments and original formatting do not.",
			"azctx alias rm prod\nazctx alias rm dev",
		},
		"azctx alias list": {
			"List configured subscription aliases",
			"Print aliases sorted by name with their subscription GUIDs. Default/text output only; no profile, Azure CLI or network is needed. A missing configuration has no aliases; malformed configuration remains an error.",
			"azctx alias list\nazctx alias list -o text",
		},
		"azctx login": {
			"Sign in through Azure CLI with attached streams",
			"Require Azure CLI >=2.61.0 and wrap az login. Device-code instructions stay visible. --tenant accepts a GUID or one profile tenant name/domain. Other Azure CLI arguments go after --; never put secrets in command lines.\n\nOrdinary user login offers one confirmation to retry only reported MFA failures. --all-tenants explicitly discovers tenants for individual sign-ins. Workload-identity passthrough skips orchestration. --no-input gates azctx prompts only, not Azure CLI authentication. Supports default/text output; unresolved sign-ins exit 3.",
			"azctx login\nazctx login --use-device-code --tenant aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa\nazctx login -- --identity",
		},
		"azctx auth": {
			"Inspect and explicitly manage Azure CLI authentication",
			"Authentication stays in Azure CLI. Use broker status for read-only effective settings, or explicitly confirmed broker enable/disable on supported Windows/macOS versions. azctx never reads credentials or MSAL token caches.",
			"azctx auth broker status\nazctx auth broker enable",
		},
		"azctx auth broker": {
			"Inspect or explicitly change the persisted authentication broker",
			"Windows requires Azure CLI >=2.61.0; macOS requires >=2.91.0 and offers an opt-in enrolled-device preview. Linux has no broker control. Changes require confirmation and delegate account clearing, persisted configuration and a fresh login to Azure CLI. A failed step stops the sequence.",
			"azctx auth broker status\nazctx auth broker disable",
		},
		"azctx auth broker status": {
			"Show effective broker settings without changing accounts",
			"Read effective platform settings and Azure CLI version without needing a profile. Environment overrides win over INI settings. Capability indicates platform/version support only, not enrollment, installed broker or successful sign-in. Supports default/text output.",
			"azctx auth broker status\nazctx auth broker status -o text",
		},
		"azctx doctor": {
			"Show safe offline Azure CLI diagnostics",
			"Inspect allowlisted settings, profile status, account-type counts and cached update status. Invoke only az version; never contact Azure, fetch updates, sign in or switch. Works without a profile or installed az. Findings are not fatal command errors. Supports text and separate schema-1 JSON; review local paths/cloud names before sharing.",
			"azctx doctor\nazctx doctor -o json",
		},
		"azctx version": {
			"Print azctx version and build information",
			"Print version, build date, commit and Go version in text (default) or JSON. Missing metadata is unknown. No profile, Azure CLI or update check is needed; version JSON is separate from subscription schema-1 output.",
			"azctx version\nazctx version -o json",
		},
		"azctx help": {
			"Show help for any command",
			"Read usage, supported flags and examples for azctx or a named command without reading Azure configuration, starting Azure CLI or accessing the network.",
			"azctx help\nazctx help auth broker",
		},
		"azctx completion": {
			"Generate shell completion scripts",
			"Generate completion for bash, zsh, fish or PowerShell. Install the script using your shell's setup. Runtime subscription completion reads only profile/settings and private configuration; it never starts Azure CLI, writes state or checks updates.",
			"azctx completion bash\nazctx completion powershell",
		},
	}
	for _, action := range []string{"enable", "disable"} {
		help["azctx auth broker "+action] = commandHelp{
			strings.ToUpper(action[:1]) + action[1:] + " the persisted broker after explicit confirmation",
			"On a supported platform/version, confirm account clearing and persisted broker configuration, then sign in again. Calls az account clear, az config set for the platform key, then attached az login. Stops on the first failure without automatic rollback. Conflicting environment overrides fail before changes. --yes explicitly confirms the change; it does not make Azure login unattended.",
			"azctx auth broker " + action + "\nazctx auth broker " + action + " --yes --no-input",
		}
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		help["azctx completion "+shell] = commandHelp{
			"Generate " + shell + " completion",
			"Print a " + shell + " completion script to stdout for installation through your shell's completion setup. --no-descriptions omits candidate descriptions. Generation and runtime completion never sign in or switch subscriptions.",
			"azctx completion " + shell + "\nazctx completion " + shell + " --no-descriptions",
		}
	}
	applyCommandHelp(root, help)
}

type commandOutput struct{ command *cobra.Command }

func (output commandOutput) Write(data []byte) (int, error) {
	return output.command.OutOrStdout().Write(data)
}

func applyCommandHelp(command *cobra.Command, help map[string]commandHelp) {
	if value, ok := help[command.CommandPath()]; ok {
		command.Short, command.Long = value.short, value.long
		command.Example = "  " + strings.ReplaceAll(value.examples, "\n", "\n  ")
	}
	for _, child := range command.Commands() {
		applyCommandHelp(child, help)
	}
}
