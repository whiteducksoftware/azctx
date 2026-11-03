---
name: Bug report
about: Report an azctx CLI problem with reproduction steps and diagnostics
title: ''
labels: ''
assignees: ''

---

For suspected vulnerabilities, follow [the security policy](https://github.com/whiteducksoftware/azctx/blob/main/SECURITY.md)
instead of filing a public issue.

**Describe the bug**
What happened?

**Reproduce**
1. Exact azctx command and flags:
2. Relevant configuration (sanitized; state whether `AZURE_CONFIG_DIR` is customized):
3. Actual output / error and exit code:

**Expected behavior**
What should have happened?

**Environment**
- azctx version (output of `azctx version`):
- Azure CLI version (output of `az version`):
- OS version and CPU architecture:
- Shell and version (for example Bash, Zsh or PowerShell):
- Installation method (Homebrew, Chocolatey, release binary or source):

**Diagnostics**
`azctx doctor` is planned for release layer 09 and is not available in earlier
layers or older releases. If your build provides it, include its sanitized
output; otherwise write "Not available" and provide the versions above.

```text
Paste sanitized diagnostics here.
```

Remove access tokens, credentials, account identifiers, tenant/subscription
identifiers, private names and paths before posting. Do not upload
`azureProfile.json`, the MSAL token cache or your Azure configuration directory.

**Additional context**
Include relevant sanitized logs and whether this is a regression.
