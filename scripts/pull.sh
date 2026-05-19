#!/usr/bin/env bash
# pull.sh — fetch artifacts from the controller VPS back to the repo:
#   - live_domains.txt (latest WARC harvest output)
#   - the reconx-dashboard journal tail
#   - the current /api/warc/status JSON
#
# Default target: root@31.57.219.246:/opt/reconx
# Override per call:
#   VPS_HOST=root@1.2.3.4 INSTALL_DIR=/srv/reconx bash scripts/pull.sh
#
# Optional flags (after the script name):
#   --no-logs       skip the journal tail
#   --lines N       how many journal lines to pull (default 200)

source "$(dirname "$0")/_common.sh"

NO_LOGS=0
LINES=200
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-logs) NO_LOGS=1; shift ;;
    --lines)   LINES="${2:-200}"; shift 2 ;;
    -h|--help) sed -n '2,15p' "$0"; exit 0 ;;
    *)         err pull "unknown flag: $1"; exit 2 ;;
  esac
done

require rsync ssh curl

cd "$REPO_DIR"
show_target

OUT_DIR="$REPO_DIR/.pulled"
mkdir -p "$OUT_DIR"

# ── 1. live_domains.txt ───────────────────────────────────────────────────
log pull "Fetching live_domains.txt → $OUT_DIR/live_domains.txt"
if rsync -av --ignore-missing-args "$VPS_HOST:$INSTALL_DIR/backend/live_domains.txt" "$OUT_DIR/live_domains.txt" 2>/dev/null; then
  if [[ -s "$OUT_DIR/live_domains.txt" ]]; then
    lines=$(wc -l < "$OUT_DIR/live_domains.txt" | tr -d ' ')
    bytes=$(du -h "$OUT_DIR/live_domains.txt" | awk '{print $1}')
    log pull "  $lines lines, $bytes"
    log pull "  preview:"
    head -5 "$OUT_DIR/live_domains.txt" | sed 's/^/    /'
  else
    warn pull "  live_domains.txt exists but is empty — start a harvest first (bash scripts/run.sh)"
  fi
else
  warn pull "  live_domains.txt not present on VPS yet — start a harvest first (bash scripts/run.sh)"
fi

# ── 2. status snapshot ────────────────────────────────────────────────────
log pull "Fetching /api/warc/status snapshot → $OUT_DIR/warc-status.json"
if curl -fsS --max-time 5 "http://$VPS_HTTP/api/warc/status" -o "$OUT_DIR/warc-status.json"; then
  python3 - "$OUT_DIR/warc-status.json" <<'PY'
import json, sys
with open(sys.argv[1]) as f: s = json.load(f)
keys = ['running','binary_present','pid','live','tested','extracted',
        'files_processed','files_total','max_domains','output_bytes',
        'started_at','finished_at','exit_code']
for k in keys:
    print(f"    {k:18}= {s.get(k)}")
last = s.get('last_line') or ''
if last:
    print(f"    last_line          = {last[:120]}")
PY
else
  warn pull "  /api/warc/status not reachable"
fi

# ── 3. journal logs ───────────────────────────────────────────────────────
if [[ "$NO_LOGS" -eq 0 ]]; then
  log pull "Fetching last $LINES journal lines → $OUT_DIR/$SERVICE_NAME.log"
  ssh "$VPS_HOST" "journalctl -u $SERVICE_NAME -n $LINES --no-pager" > "$OUT_DIR/$SERVICE_NAME.log" 2>/dev/null || {
    warn pull "  journalctl returned nothing — service may not be journal-logged or you lack permission"
  }
  if [[ -s "$OUT_DIR/$SERVICE_NAME.log" ]]; then
    log pull "  saved $(wc -l < "$OUT_DIR/$SERVICE_NAME.log" | tr -d ' ') lines"
  fi
fi

log pull "Done. Pulled artifacts are in $OUT_DIR/"
