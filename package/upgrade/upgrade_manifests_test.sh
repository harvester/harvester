#!/bin/bash
set -euo pipefail

script="$(dirname "$0")/upgrade_manifests.sh"
function_definition=$(sed -n '/^sync_traefik_default_tls_options() {/,/^}/p' "$script")

if [[ -z "$function_definition" ]]; then
  echo "sync_traefik_default_tls_options is not defined" >&2
  exit 1
fi

kubectl() {
  printf '%s\n' "$*"
}

REPO_HARVESTER_VERSION=v1.9.2
actual=$(eval "$function_definition"; sync_traefik_default_tls_options)
expected='Sync traefik-default-tls-options setting
annotate --overwrite=true setting.harvesterhci.io traefik-default-tls-options harvesterhci.io/upgrade-patched=v1.9.2'

[[ "$actual" == "$expected" ]]
