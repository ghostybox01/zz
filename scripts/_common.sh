# Shared helpers for push.sh / pull.sh / run.sh. Source-only — not executable.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")/.." && pwd)"

VPS_HOST="${VPS_HOST:-root@31.57.219.246}"
INSTALL_DIR="${INSTALL_DIR:-/opt/reconx}"
SERVICE_NAME="${SERVICE_NAME:-reconx-dashboard}"
# Hostname portion of VPS_HOST — used for direct HTTP probes
VPS_HTTP="${VPS_HTTP:-${VPS_HOST#*@}}"

GREEN=$'\033[0;32m'
YELLOW=$'\033[0;33m'
RED=$'\033[0;31m'
DIM=$'\033[2m'
RESET=$'\033[0m'

log()  { echo "${GREEN}[$1]${RESET} ${*:2}"; }
warn() { echo "${YELLOW}[$1]${RESET} ${*:2}"; }
err()  { echo "${RED}[$1] ERROR:${RESET} ${*:2}" >&2; }
dim()  { echo "${DIM}${*}${RESET}"; }

require() {
  for cmd in "$@"; do
    command -v "$cmd" >/dev/null 2>&1 || { err common "missing dependency: $cmd"; exit 1; }
  done
}

show_target() {
  dim "  VPS_HOST=$VPS_HOST  INSTALL_DIR=$INSTALL_DIR  SERVICE_NAME=$SERVICE_NAME"
}
