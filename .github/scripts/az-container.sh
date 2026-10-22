#!/usr/bin/env bash
# azure/login and azctx resolve this host shim; the CLI itself runs in Docker.
set -euo pipefail

if [[ ! ${AZCTX_AZURE_CLI_IMAGE:-} =~ ^mcr\.microsoft\.com/azure-cli@sha256:[a-f0-9]{64}$ ]]; then
  echo 'Azure CLI requires a resolved MCR image digest' >&2
  exit 1
fi
: "${AZURE_CONFIG_DIR:?isolated Azure config directory is required}"

exec docker run --rm --interactive --cap-drop=ALL --security-opt=no-new-privileges \
  --volume "$AZURE_CONFIG_DIR:$AZURE_CONFIG_DIR" \
  --env AZURE_CONFIG_DIR --env AZURE_CORE_OUTPUT --env AZURE_CORE_ONLY_SHOW_ERRORS \
  --env AZURE_CORE_LOGIN_EXPERIENCE_V2 --env AZURE_EXTENSION_USE_DYNAMIC_INSTALL \
  "$AZCTX_AZURE_CLI_IMAGE" az "$@"
