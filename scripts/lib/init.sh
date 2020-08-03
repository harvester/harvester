#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

unset CDPATH

# Set no_proxy for localhost if behind a proxy, otherwise,
# the connections to localhost in scripts will time out
export no_proxy=127.0.0.1,localhost

# The root of the octopus directory
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"

source "${ROOT_DIR}/scripts/lib/log.sh"
source "${ROOT_DIR}/scripts/lib/kubectl.sh"
source "${ROOT_DIR}/scripts/lib/util.sh"
source "${ROOT_DIR}/scripts/lib/version.sh"

vm::log::install_errexit
vm::version::get_version_vars
