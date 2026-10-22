#!/usr/bin/env bash
# Manual live checks put this shim first on PATH as `az`; the CLI runs in Docker
# against a dedicated, separately signed-in config directory. It never pulls.
set -euo pipefail

if [[ ! ${AZCTX_AZURE_CLI_IMAGE:-} =~ ^mcr\.microsoft\.com/azure-cli@sha256:[a-f0-9]{64}$ ]]; then
  echo 'Azure CLI requires a resolved MCR image digest' >&2
  exit 1
fi
config=${AZURE_CONFIG_DIR:-}
if [[ $config != /* || ! -d $config ]]; then
  echo 'AZURE_CONFIG_DIR must be an existing absolute dedicated config directory' >&2
  exit 1
fi
resolved=$(cd "$config" && pwd -P)
host=$(cd "${HOME:-/nonexistent}/.azure" 2>/dev/null && pwd -P || true)
if [[ -n $host && ( $resolved == "$host" || $resolved == "$host"/* ) ]]; then
  echo 'AZURE_CONFIG_DIR must be a dedicated directory, not the host ~/.azure' >&2
  exit 1
fi

exec docker run --rm --interactive --pull=never --cap-drop=ALL --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --volume "$AZURE_CONFIG_DIR:$AZURE_CONFIG_DIR" \
  --env "HOME=$AZURE_CONFIG_DIR" --env AZURE_CONFIG_DIR --env AZURE_CORE_ONLY_SHOW_ERRORS \
  --env AZURE_CORE_LOGIN_EXPERIENCE_V2 --env AZURE_EXTENSION_USE_DYNAMIC_INSTALL \
  "$AZCTX_AZURE_CLI_IMAGE" az "$@"
