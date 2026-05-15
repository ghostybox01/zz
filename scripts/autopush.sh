#!/usr/bin/env bash
# Auto-push watcher.
#
# Watches the project tree and, when something changes, debounces 30 s and
# then commits + pushes. Designed to run in a terminal you keep open while
# you work — kill with Ctrl-C.
#
# Setup (one-time, in same shell or your zshrc):
#     export GH_TOKEN='ghp_…your token…'
#     # (install fswatch if missing — macOS only)
#     brew install fswatch
#
# Then:
#     bash scripts/autopush.sh
#
# Each commit message is "auto: 2026-05-15T13:42:18". The script never force-
# pushes and never amends. If your local branch has diverged from the remote,
# the push will fail and you'll see it — resolve manually.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_DIR"

REMOTE_URL="https://github.com/aidanbaker812-prog/scanscanscannn.git"
DEBOUNCE_S=30
BRANCH="${GIT_BRANCH:-main}"

GREEN=$'\033[0;32m'
YELLOW=$'\033[0;33m'
RED=$'\033[0;31m'
RESET=$'\033[0m'

log()  { echo "${GREEN}[autopush]${RESET} $*"; }
warn() { echo "${YELLOW}[autopush]${RESET} $*"; }
err()  { echo "${RED}[autopush] ERROR:${RESET} $*" >&2; }

if ! command -v git >/dev/null 2>&1; then err "git not found"; exit 1; fi
if ! command -v fswatch >/dev/null 2>&1; then
  err "fswatch not found. Install with: brew install fswatch"
  exit 1
fi

# Configure the remote with the token if GH_TOKEN is set, so push doesn't prompt.
if [[ -n "${GH_TOKEN:-}" ]]; then
  AUTH_URL="https://${GH_TOKEN}@github.com/aidanbaker812-prog/scanscanscannn.git"
  if git remote get-url origin >/dev/null 2>&1; then
    git remote set-url origin "$AUTH_URL"
  else
    git remote add origin "$AUTH_URL"
  fi
  log "remote 'origin' configured with token"
else
  warn "GH_TOKEN not set — push will use whatever credentials git already has"
fi

# Confirm git identity exists (commits need author).
if ! git config user.email >/dev/null 2>&1; then
  warn "git user.email not set — using a generic identity for this repo only"
  git config user.email "autopush@reconx.local"
  git config user.name  "ReconX Autopush"
fi

log "watching $REPO_DIR (debounce ${DEBOUNCE_S}s, branch '$BRANCH')"
log "press Ctrl-C to stop"

do_commit_push() {
  if [[ -z "$(git status --porcelain)" ]]; then
    return 0
  fi
  log "changes detected — committing"
  git add -A
  TS="$(date +%Y-%m-%dT%H:%M:%S)"
  if git commit -m "auto: $TS" >/dev/null 2>&1; then
    log "committed at $TS"
    if git push origin "$BRANCH" 2>/dev/null; then
      log "pushed to $BRANCH"
    else
      warn "push failed — branch may have diverged or remote rejected. Run 'git pull --rebase' manually."
    fi
  else
    warn "commit produced nothing — skipping"
  fi
}

# Initial push if there are uncommitted changes
do_commit_push

# fswatch -or emits a line per batch of changes
fswatch -or \
  --exclude '\.git/' \
  --exclude 'node_modules/' \
  --exclude 'dist/' \
  --exclude 'venv/' \
  --exclude '__pycache__/' \
  --exclude '\.nelson/' \
  --exclude 'ResultJS/' \
  --exclude 'collected_results/' \
  --exclude '\.DS_Store' \
  . | while read -r _; do
  # Debounce: sleep, then commit+push. If another event arrives during the
  # sleep, fswatch buffers it and we'll handle it on the next iteration.
  sleep "$DEBOUNCE_S"
  do_commit_push
done
