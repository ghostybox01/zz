#!/usr/bin/env bash
# run.sh — kick off a WARC harvest on the controller VPS and tail status
# until the target live-domain count is hit (or you Ctrl-C out).
#
# Default target: root@31.57.219.246:/opt/reconx — same env vars as push.sh
#
# Optional flags (after the script name):
#   --max N        target live-domain count (default 10000)
#   --verbose      pass -verbose to warc_live_checker (very chatty stdout)
#   --stop         stop the running harvester instead of starting one
#   --status       print the current status snapshot and exit
#   --no-tail      start the harvest but don't poll afterwards

source "$(dirname "$0")/_common.sh"

MAX=10000
VERBOSE=false
ACTION=start
TAIL=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    --max)     MAX="${2:-10000}"; shift 2 ;;
    --verbose) VERBOSE=true; shift ;;
    --stop)    ACTION=stop; shift ;;
    --status)  ACTION=status; shift ;;
    --no-tail) TAIL=0; shift ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *)         err run "unknown flag: $1"; exit 2 ;;
  esac
done

require curl

show_target
BASE="http://$VPS_HTTP/api/warc"

print_snapshot() {
  local json
  if ! json="$(curl -fsS --max-time 5 "$BASE/status")"; then
    err run "couldn't reach $BASE/status — is reconx-dashboard up?"
    return 1
  fi
  python3 - <<PY
import json, sys
s = json.loads('''$json''')
def n(k): return s.get(k) or 0
print(f"    running={s.get('running')}  binary_present={s.get('binary_present')}  pid={s.get('pid')}")
print(f"    live={n('live'):,}/{n('max_domains'):,}  tested={n('tested'):,}  extracted={n('extracted'):,}  files={n('files_processed'):,}/{n('files_total'):,}  output={n('output_bytes'):,} B")
last = (s.get('last_line') or '')[:140]
if last: print(f"    last: {last}")
PY
}

case "$ACTION" in
  status)
    print_snapshot
    ;;

  stop)
    log run "Stopping harvester…"
    if resp="$(curl -fsS -X POST --max-time 5 "$BASE/stop")"; then
      echo "  $resp"
    else
      err run "stop call failed"
      exit 1
    fi
    print_snapshot
    ;;

  start)
    log run "Starting harvester (max=$MAX, verbose=$VERBOSE)…"
    payload=$(python3 -c "import json,sys; print(json.dumps({'max_domains': int(sys.argv[1]), 'verbose': sys.argv[2]=='true'}))" "$MAX" "$VERBOSE")
    if resp="$(curl -fsS -X POST --max-time 5 -H 'Content-Type: application/json' -d "$payload" "$BASE/start")"; then
      echo "  $resp"
    else
      # bubble up the HTTP body so we can see "binary not found" etc.
      err run "start call failed"
      curl -sS -X POST --max-time 5 -H 'Content-Type: application/json' -d "$payload" "$BASE/start" || true
      echo
      exit 1
    fi

    if [[ "$TAIL" -eq 0 ]]; then
      log run "Started — skipping live tail (--no-tail). Use \`bash scripts/run.sh --status\` to check in."
      exit 0
    fi

    log run "Tailing status — Ctrl-C to detach (the harvester keeps running)."
    PREV=""
    while true; do
      sleep 3
      json="$(curl -fsS --max-time 5 "$BASE/status" || true)"
      [[ -z "$json" ]] && { warn run "lost connection — retrying"; continue; }
      LINE=$(python3 - <<PY
import json
s = json.loads('''$json''')
running = s.get('running')
live    = s.get('live') or 0
maxd    = s.get('max_domains') or 0
tested  = s.get('tested') or 0
files   = s.get('files_processed') or 0
totalf  = s.get('files_total') or 0
last    = (s.get('last_line') or '')[:80]
exit_c  = s.get('exit_code')
state   = 'HARVESTING' if running else (f'EXITED({exit_c})' if exit_c is not None else 'IDLE')
print(f"{state:11} live={live:,}/{maxd:,}  tested={tested:,}  files={files:,}/{totalf:,}  | {last}")
PY
)
      [[ "$LINE" != "$PREV" ]] && { echo "  $LINE"; PREV="$LINE"; }
      DONE=$(python3 -c "import json; s=json.loads('''$json'''); print('yes' if not s.get('running') and s.get('exit_code') is not None else 'no')")
      [[ "$DONE" == "yes" ]] && { log run "Harvester exited."; break; }
    done

    log run "Final snapshot:"
    print_snapshot
    log run "Pull the result with: bash scripts/pull.sh"
    ;;
esac
