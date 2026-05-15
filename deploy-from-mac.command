#!/usr/bin/env bash
# deploy-from-mac.command — double-click this in Finder to ship the project
# to a fresh Ubuntu/Debian VPS and run install-controller.sh on the remote.
#
# Prompts for: VPS IP, SSH user, SSH key path. Then it:
#   1. rsyncs the project to /root/reconx-src/
#   2. SSHes in and runs install-controller.sh as root
#   3. prints the final dashboard URL
#
# Re-running this is safe — the installer is idempotent.

set -euo pipefail

# Always cd to the script's directory so rsync uses the project root.
cd "$(dirname "${BASH_SOURCE[0]}")"

if ! command -v rsync >/dev/null 2>&1; then
  echo "rsync is not installed. Install with: brew install rsync"
  read -n 1 -s -r -p "Press any key to close…"
  exit 1
fi

clear
cat <<'BANNER'
╔═══════════════════════════════════════════════════════════════════╗
║                                                                   ║
║   ReconX — deploy controller to remote VPS                        ║
║                                                                   ║
║   This will:                                                      ║
║     1. rsync this project tree to your VPS                        ║
║     2. run install-controller.sh on the VPS (as root)             ║
║     3. set up nginx, Flask, fleet API, scanner binary             ║
║                                                                   ║
║   The VPS must be Ubuntu 22.04+/24.04 or Debian 12+, with root    ║
║   SSH access from this Mac.                                       ║
║                                                                   ║
╚═══════════════════════════════════════════════════════════════════╝

BANNER

read -p "Controller VPS IP or host:    " HOST
[[ -z "${HOST:-}" ]] && { echo "No host given. Aborting."; exit 1; }

read -p "SSH user [default: root]:     " SSH_USER
SSH_USER=${SSH_USER:-root}

DEFAULT_KEY="$HOME/.ssh/id_ed25519"
[[ ! -f "$DEFAULT_KEY" ]] && DEFAULT_KEY="$HOME/.ssh/id_rsa"
read -p "SSH key path [$DEFAULT_KEY]:  " KEY
KEY=${KEY:-$DEFAULT_KEY}

if [[ ! -f "$KEY" ]]; then
  echo "SSH key not found at $KEY. Aborting."
  read -n 1 -s -r -p "Press any key to close…"
  exit 1
fi

echo
echo "Will deploy to ${SSH_USER}@${HOST} using key ${KEY}"
read -p "Continue? [y/N]: " yn
case "$yn" in
  [Yy]*) ;;
  *) echo "Aborted."; read -n 1 -s -r -p "Press any key to close…"; exit 0;;
esac

REMOTE_DIR="/root/reconx-src"

echo
echo "→ Verifying SSH connectivity…"
ssh -o BatchMode=yes -o ConnectTimeout=8 -i "$KEY" "${SSH_USER}@${HOST}" "echo connected" \
  || { echo "SSH connection failed."; read -n 1 -s -r -p "Press any key to close…"; exit 1; }

echo "→ Rsyncing project to ${SSH_USER}@${HOST}:${REMOTE_DIR} …"
rsync -az --delete \
  --exclude='node_modules' \
  --exclude='dist' \
  --exclude='.git' \
  --exclude='.nelson' \
  --exclude='venv' \
  --exclude='.venv' \
  --exclude='__pycache__' \
  --exclude='ResultJS' \
  --exclude='*.log' \
  -e "ssh -i $KEY -o StrictHostKeyChecking=accept-new" \
  ./ "${SSH_USER}@${HOST}:${REMOTE_DIR}/"

echo "→ Running installer on the VPS…"
ssh -t -i "$KEY" "${SSH_USER}@${HOST}" "bash ${REMOTE_DIR}/install-controller.sh"

echo
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  ✓ Deploy complete"
echo "  Open: http://${HOST}/"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo
read -n 1 -s -r -p "Press any key to close…"
