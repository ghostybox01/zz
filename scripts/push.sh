#!/usr/bin/env bash
# push.sh — build the dashboard, rsync code to the controller VPS, rebuild
# warc_live_checker on the box, restart the Flask service.
#
# Default target: root@31.57.219.246:/opt/reconx
# Override per call:
#   VPS_HOST=root@1.2.3.4 INSTALL_DIR=/srv/reconx bash scripts/push.sh
#
# Optional flags (after the script name):
#   --skip-build       reuse existing dashboard/dist (faster re-pushes)
#   --no-warc          skip the warc_live_checker build on the VPS
#   --no-restart       leave reconx-dashboard alone

source "$(dirname "$0")/_common.sh"

SKIP_BUILD=0
NO_WARC=0
NO_RESTART=0
for arg in "$@"; do
  case "$arg" in
    --skip-build) SKIP_BUILD=1 ;;
    --no-warc)    NO_WARC=1 ;;
    --no-restart) NO_RESTART=1 ;;
    -h|--help)
      sed -n '2,15p' "$0"
      exit 0
      ;;
    *)
      err push "unknown flag: $arg"
      exit 2
      ;;
  esac
done

require rsync ssh

cd "$REPO_DIR"
show_target

# ── 1. Build the dashboard locally ────────────────────────────────────────
if [[ "$SKIP_BUILD" -eq 0 ]]; then
  log push "Building dashboard (npm run build)…"
  ( cd dashboard && npm run build ) >/dev/null
  [[ -f dashboard/dist/index.html ]] || { err push "build did not produce dashboard/dist/index.html"; exit 1; }
  log push "  dist size: $(du -sh dashboard/dist | awk '{print $1}')"
else
  log push "Skipping local build (--skip-build)"
  [[ -f dashboard/dist/index.html ]] || { err push "no existing dashboard/dist — drop --skip-build"; exit 1; }
fi

# ── 2. Push artifacts ─────────────────────────────────────────────────────
log push "Syncing dashboard/dist/ → $VPS_HOST:$INSTALL_DIR/dashboard/dist/"
rsync -av --delete dashboard/dist/ "$VPS_HOST:$INSTALL_DIR/dashboard/dist/" >/dev/null

log push "Syncing backend/app.py"
rsync -av backend/app.py "$VPS_HOST:$INSTALL_DIR/backend/app.py" >/dev/null

log push "Syncing warc.go + install-controller.sh"
rsync -av warc.go                "$VPS_HOST:$INSTALL_DIR/warc.go"               >/dev/null
rsync -av install-controller.sh  "$VPS_HOST:$INSTALL_DIR/install-controller.sh" >/dev/null

# ── 3. Build warc_live_checker and bounce the service on the VPS ──────────
if [[ "$NO_WARC" -eq 1 && "$NO_RESTART" -eq 1 ]]; then
  log push "Skipping remote build + restart"
else
  log push "Running remote build/restart steps…"
  ssh "$VPS_HOST" \
    INSTALL_DIR="$INSTALL_DIR" \
    SERVICE_NAME="$SERVICE_NAME" \
    NO_WARC="$NO_WARC" \
    NO_RESTART="$NO_RESTART" \
    'bash -s' <<'REMOTE'
set -euo pipefail
cd "$INSTALL_DIR/backend"

if [[ "$NO_WARC" -eq 0 ]]; then
  if [[ -f "$INSTALL_DIR/warc.go" ]]; then
    echo "[push:remote] Building warc_live_checker…"
    WARC_BUILD_DIR="$(mktemp -d)"
    cp "$INSTALL_DIR/warc.go" "$WARC_BUILD_DIR/main.go"
    (
      cd "$WARC_BUILD_DIR"
      go mod init warc-live-checker >/dev/null 2>&1 || true
      go get github.com/schollz/progressbar/v3 >/dev/null 2>&1 || true
      go mod tidy >/dev/null 2>&1 || true
      go build -o "$INSTALL_DIR/backend/warc_live_checker" .
    )
    chmod +x "$INSTALL_DIR/backend/warc_live_checker"
    rm -rf "$WARC_BUILD_DIR"
    echo "[push:remote]   binary: $(du -h "$INSTALL_DIR/backend/warc_live_checker" | awk '{print $1}')"
  else
    echo "[push:remote]   warc.go not present — skipping WARC build"
  fi
fi

if [[ "$NO_RESTART" -eq 0 ]]; then
  echo "[push:remote] Restarting $SERVICE_NAME…"
  systemctl restart "$SERVICE_NAME"
  sleep 1
  systemctl is-active --quiet "$SERVICE_NAME" \
    && echo "[push:remote]   $SERVICE_NAME is active" \
    || { echo "[push:remote]   $SERVICE_NAME is NOT active"; systemctl status "$SERVICE_NAME" --no-pager | head -20; exit 1; }
fi
REMOTE
fi

# ── 4. Health check ───────────────────────────────────────────────────────
log push "Health-checking http://$VPS_HTTP/api/warc/status …"
if STATUS_JSON="$(curl -fsS --max-time 5 "http://$VPS_HTTP/api/warc/status")"; then
  python3 - <<PY
import json
s = json.loads('''$STATUS_JSON''')
print(f"  binary_present={s.get('binary_present')} running={s.get('running')} bytes={s.get('output_bytes')} live={s.get('live')}")
PY
else
  warn push "  /api/warc/status not reachable yet (gunicorn may still be booting)."
fi

log push "Done. Hard-refresh the browser (Cmd-Shift-R) to pick up the new bundle."
