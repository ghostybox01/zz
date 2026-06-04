# Self-Healing Scanner Watchdog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the scanner on each worker VPS self-healing — it restarts automatically on crash, never loses counter state, and uses kernel-level cgroup limits instead of fragile userspace tools.

**Architecture:** Three-layer fix: (1) a systemd watchdog unit deployed once per worker that monitors and auto-restarts the scanner with hard cgroup memory/CPU caps; (2) persistent counters in the Go scanner that seed from the last `stats.json` on startup so `valid_hosts`/`invalid_hosts` survive restarts; (3) the controller writes a session config file before launching the scanner so the watchdog always knows what to run, and clears it on stop.

**Tech Stack:** Python (app.py, paramiko SSH), Go (main.go), systemd (worker VPS), bash (watchdog script)

---

## File Map

| File | Change |
|------|--------|
| `backend/app.py` | Add `_deploy_watchdog()`, call from `_dispatch_crack_worker`, call cleanup from `api_crack_stop` |
| `backend/main.go` | Seed `ValidHosts`/`InvalidHosts` from existing `stats.json` at startup in `startRateTracker` |

---

## Task 1: Persistent counters in the Go scanner

**Files:**
- Modify: `backend/main.go` — `startRateTracker()` function (line ~5767)

The problem: every time the scanner binary restarts, `globalCounters.ValidHosts` and `globalCounters.InvalidHosts` reset to 0. This makes the dashboard show the counter going backwards.

The fix: on startup, read `ResultJS/stats.json` if it exists and seed the counters from it. The file is written every second, so the seed is at most 1 second stale.

- [ ] **Step 1: Add counter seeding to `startRateTracker`**

In `backend/main.go`, replace the `startRateTracker` function (currently at line ~5767):

```go
func startRateTracker() {
	// Seed ValidHosts/InvalidHosts from the last stats.json written by a
	// previous run. This means counters survive a scanner restart — they
	// continue from the last saved value instead of resetting to 0.
	if data, err := os.ReadFile(filepath.Join("ResultJS", "stats.json")); err == nil {
		var prev map[string]interface{}
		if json.Unmarshal(data, &prev) == nil {
			globalCounters.mu.Lock()
			if v, ok := prev["valid_hosts"].(float64); ok && v > 0 {
				globalCounters.ValidHosts = int(v)
			}
			if v, ok := prev["invalid_hosts"].(float64); ok && v > 0 {
				globalCounters.InvalidHosts = int(v)
			}
			globalCounters.mu.Unlock()
		}
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			globalCounters.mu.Lock()
			newReq := globalCounters.URLsProcessed
			newParse := globalCounters.APIsFoundTotal
			globalCounters.RequestsPerSec = float64(newReq - globalCounters.requestSnapshot)
			globalCounters.ParsesPerSec = float64(newParse - globalCounters.parseSnapshot)
			globalCounters.requestSnapshot = newReq
			globalCounters.parseSnapshot = newParse
			globalCounters.rpsTotal += globalCounters.RequestsPerSec
			globalCounters.ppsTotal += globalCounters.ParsesPerSec
			globalCounters.rpsCount++
			globalCounters.ppsCount++
			if globalCounters.rpsCount > 0 {
				globalCounters.AvgRps = globalCounters.rpsTotal / float64(globalCounters.rpsCount)
				globalCounters.AvgPps = globalCounters.ppsTotal / float64(globalCounters.ppsCount)
			}
			rps := globalCounters.RequestsPerSec
			pps := globalCounters.ParsesPerSec
			avgRps := globalCounters.AvgRps
			avgPps := globalCounters.AvgPps
			processed := globalCounters.URLsProcessed
			found := globalCounters.APIsFoundTotal
			validated := globalCounters.APIsValidated
			loaded := globalCounters.URLsLoaded
			validHosts := globalCounters.ValidHosts
			invalidHosts := globalCounters.InvalidHosts
			globalCounters.mu.Unlock()

			var progression float64
			if loaded > 0 {
				progression = float64(processed) / float64(loaded)
			}

			statsData := map[string]interface{}{
				"urls_processed": processed,
				"apis_found":     found,
				"apis_validated": validated,
				"rps":            rps,
				"pps":            pps,
				"avg_rps":        avgRps,
				"avg_pps":        avgPps,
				"valid_hosts":    validHosts,
				"invalid_hosts":  invalidHosts,
				"progression":    progression,
				"urls_loaded":    loaded,
			}
			if b, err := json.Marshal(statsData); err == nil {
				_ = os.WriteFile(filepath.Join("ResultJS", "stats.json"), b, 0644)
			}
		}
	}()
}
```

- [ ] **Step 2: Build and verify**

```bash
cd /Users/x/Downloads/scan/backend
go build -o reconx-scanner-linux . 2>&1
```

Expected: clean build, no errors.

- [ ] **Step 3: Verify counter seeding works**

```bash
# Create a fake stats.json
mkdir -p /tmp/ResultJS
echo '{"valid_hosts":1234,"invalid_hosts":5678}' > /tmp/ResultJS/stats.json
# Run binary briefly with fake targets (it should seed from the file)
echo "example.com" > /tmp/test_targets.txt
cd /tmp && cp /Users/x/Downloads/scan/backend/reconx-scanner-linux . 
./reconx-scanner-linux -timeout 1 test_targets.txt &
sleep 3
cat ResultJS/stats.json | python3 -c "import sys,json; d=json.load(sys.stdin); print('valid_hosts:', d.get('valid_hosts')); assert d.get('valid_hosts') >= 1234, 'counter not seeded'"
kill %1 2>/dev/null; true
```

Expected: `valid_hosts:` shows 1234 or higher (seeded from the previous fake stats.json).

- [ ] **Step 4: Commit**

```bash
cd /Users/x/Downloads/scan
git add backend/main.go backend/reconx-scanner-linux
git commit -m "fix: seed valid/invalid host counters from stats.json on scanner restart"
```

---

## Task 2: Watchdog script and systemd unit

**Files:**
- Modify: `backend/app.py` — add `_deploy_watchdog()` helper function

This task creates the watchdog that goes on each worker VPS. It's a bash script managed by systemd that:
1. Reads `/tmp/reconx_session.conf` to find the active scan directory and binary
2. Uses `systemd-run --scope` with `MemoryMax` and `CPUQuota` for kernel-level enforcement
3. Starts automatically on boot (survives VPS reboot)

The watchdog is deployed **once per worker** — it's idempotent. Subsequent crack sessions just update `/tmp/reconx_session.conf`.

- [ ] **Step 1: Add `_deploy_watchdog` to `app.py`**

Add this function near the other `_dispatch_*` helpers in `backend/app.py` (after `_extract_remote_pid`, around line ~4050):

```python
# ── Watchdog content strings ─────────────────────────────────────────────────
# The watchdog script is installed once per worker and stays active across
# reboots.  It reads /tmp/reconx_session.conf for the active crack directory
# and binary path, then monitors and restarts the scanner using systemd-run
# so kernel cgroups enforce the memory/CPU limits (more reliable than cpulimit).

_WATCHDOG_SCRIPT = r"""#!/bin/bash
# /usr/local/bin/reconx-watchdog
# Managed by RavenX controller — do not edit manually.
CONF=/tmp/reconx_session.conf
LOG=/tmp/reconx_watchdog.log
STAMP_FILE=/tmp/reconx_watchdog_started

[ -f "$CONF" ] || exit 0
source "$CONF"   # sets CRACK_DIR and SCANNER_BIN (optional)

[ -d "$CRACK_DIR" ] || exit 0
BIN="${SCANNER_BIN:-$CRACK_DIR/reconx-scanner}"
[ -x "$BIN" ] || BIN="$CRACK_DIR/reconx-scanner-linux"
[ -x "$BIN" ] || exit 0

# Already running?
if pgrep -x "$(basename $BIN)" > /dev/null 2>&1; then exit 0; fi

cd "$CRACK_DIR"

# Launch scanner inside a transient systemd scope so cgroups enforce
# MemoryMax and CPUQuota at the kernel level.
systemd-run --scope --unit=reconx-scanner-scope \
    -p MemoryMax=900M -p CPUQuota=90% \
    env GOMEMLIMIT=900MiB \
    ionice -c 2 -n 7 nice -n 15 \
    "$BIN" -timeout 5 -checkpoint checkpoint.txt targets.txt \
    >> "$CRACK_DIR/crack.log" 2>&1 &

echo "$(date '+%Y-%m-%d %H:%M:%S') watchdog: restarted scanner (PID $!)" >> "$LOG"
"""

_WATCHDOG_SERVICE = """[Unit]
Description=RavenX Scanner Watchdog
After=network.target
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=/usr/local/bin/reconx-watchdog-loop
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
"""

_WATCHDOG_LOOP = r"""#!/bin/bash
# /usr/local/bin/reconx-watchdog-loop — called by systemd, loops forever
while true; do
    /usr/local/bin/reconx-watchdog
    sleep 10
done
"""


def _deploy_watchdog(mgr, ip: str, remote_dir: str, binary_name: str = 'reconx-scanner') -> bool:
    """Install the watchdog on `ip` (idempotent) and write the session config.

    Returns True on success, False on any SSH error (non-fatal — the scan
    proceeds without the watchdog rather than blocking dispatch).

    The watchdog consists of:
      /usr/local/bin/reconx-watchdog        — one-shot check+restart script
      /usr/local/bin/reconx-watchdog-loop   — loop wrapper called by systemd
      /etc/systemd/system/reconx-watchdog.service — systemd unit
      /tmp/reconx_session.conf              — active session config (updated each crack)
    """
    try:
        # Write the one-shot watchdog script
        mgr.ssh_exec(ip, f"cat > /usr/local/bin/reconx-watchdog << 'WDEOF'\n{_WATCHDOG_SCRIPT}\nWDEOF", 10)
        mgr.ssh_exec(ip, "chmod +x /usr/local/bin/reconx-watchdog", 5)

        # Write the loop wrapper
        mgr.ssh_exec(ip, f"cat > /usr/local/bin/reconx-watchdog-loop << 'WLEOF'\n{_WATCHDOG_LOOP}\nWLEOF", 10)
        mgr.ssh_exec(ip, "chmod +x /usr/local/bin/reconx-watchdog-loop", 5)

        # Install the systemd service (only if not already present)
        svc_check = (mgr.ssh_exec(ip, "systemctl is-active reconx-watchdog 2>/dev/null || echo inactive", 5) or '').strip()
        if svc_check != 'active':
            mgr.ssh_exec(ip,
                f"cat > /etc/systemd/system/reconx-watchdog.service << 'SVEOF'\n{_WATCHDOG_SERVICE}\nSVEOF",
                10)
            mgr.ssh_exec(ip, "systemctl daemon-reload && systemctl enable reconx-watchdog && systemctl restart reconx-watchdog", 15)

        # Write session config so watchdog knows where to look
        bin_path = f"{remote_dir}/{binary_name}"
        conf = f"CRACK_DIR={remote_dir}\nSCANNER_BIN={bin_path}\n"
        mgr.ssh_exec(ip, f"printf '%s' '{conf}' > /tmp/reconx_session.conf", 5)

        print(f'[watchdog] deployed on {ip} → {remote_dir}')
        return True
    except Exception as e:
        print(f'[watchdog] deploy failed for {ip}: {e}')
        return False
```

- [ ] **Step 2: Commit**

```bash
cd /Users/x/Downloads/scan
git add backend/app.py
git commit -m "feat: add _deploy_watchdog helper — installs systemd watchdog + session config on workers"
```

---

## Task 3: Call `_deploy_watchdog` from `_dispatch_crack_worker`

**Files:**
- Modify: `backend/app.py` — `_dispatch_crack_worker` function (around line ~4385)

After the scanner is successfully spawned (we have a valid PID), deploy the watchdog so it can restart the scanner if it dies.

- [ ] **Step 1: Add watchdog call after successful scanner spawn**

In `_dispatch_crack_worker`, find the section after the scanner is launched and PID is confirmed. It currently ends with:

```python
        return pid, None
    except Exception as e:
        return None, str(e)
    finally:
        if tf_targets is not None:
            try: os.unlink(tf_targets.name)
            except Exception: pass
        if tf_config is not None:
            try: os.unlink(tf_config.name)
            except Exception: pass
```

Change `return pid, None` to also deploy the watchdog:

```python
        # Deploy watchdog so scanner auto-restarts on crash.
        # Non-fatal: if watchdog deploy fails, the scan still runs.
        _deploy_watchdog(mgr, ip, remote_dir)
        return pid, None
    except Exception as e:
        return None, str(e)
    finally:
        if tf_targets is not None:
            try: os.unlink(tf_targets.name)
            except Exception: pass
        if tf_config is not None:
            try: os.unlink(tf_config.name)
            except Exception: pass
```

- [ ] **Step 2: Commit**

```bash
cd /Users/x/Downloads/scan
git add backend/app.py
git commit -m "feat: deploy watchdog on each worker after scanner is spawned"
```

---

## Task 4: Clean up session config on crack stop

**Files:**
- Modify: `backend/app.py` — `api_crack_stop` function (line ~5323)

When the operator stops a crack, the watchdog session config must be cleared so the watchdog stops trying to restart the scanner. Otherwise the watchdog will keep relaunching a scanner the operator deliberately stopped.

- [ ] **Step 1: Add conf cleanup to `api_crack_stop`**

In `api_crack_stop`, after the `kill -TERM` / `kill -KILL` block, add:

```python
    mgr = get_ssh_manager()
    if mgr is not None:
        for ip, pid in worker_pids.items():
            try:
                # Graceful shutdown: SIGTERM first, SIGKILL after 3s if still running
                _ssh_exec_retry(mgr, ip,
                    f'kill -TERM {int(pid)} 2>/dev/null; '
                    f'sleep 3; '
                    f'kill -0 {int(pid)} 2>/dev/null && kill -KILL {int(pid)} 2>/dev/null; '
                    f'true',
                    15)
                # Clear session config so watchdog stops trying to restart it
                _ssh_exec_retry(mgr, ip, 'rm -f /tmp/reconx_session.conf', 5)
            except Exception as e:
                print(f'[crack] stop failed for {ip} pid {pid}: {e}')
```

- [ ] **Step 2: Commit**

```bash
cd /Users/x/Downloads/scan
git add backend/app.py
git commit -m "fix: clear watchdog session config on crack stop so watchdog doesn't restart a stopped scan"
```

---

## Task 5: Build new scanner binary and deploy everything

**Files:**
- `backend/reconx-scanner-linux` — rebuilt binary with persistent counters

- [ ] **Step 1: Build the new binary**

```bash
cd /Users/x/Downloads/scan/backend
GOOS=linux GOARCH=amd64 go build -o reconx-scanner-linux .
ls -lh reconx-scanner-linux
```

Expected: file exists, size ~15-20MB.

- [ ] **Step 2: Push to GitHub**

```bash
cd /Users/x/Downloads/scan
git add backend/reconx-scanner-linux
git commit -m "build: scanner binary with persistent counter seeding on restart"
git push origin main
```

- [ ] **Step 3: Deploy app.py and new binary to controller VPS**

SSH to the controller VPS (`31.57.219.246`) and run:

```bash
cd /root/reconx-src
git fetch origin main
git checkout origin/main -- backend/app.py backend/reconx-scanner-linux

# Deploy app.py to running location
cp backend/app.py /opt/reconx/backend/app.py
systemctl restart reconx-dashboard.service
sleep 3
systemctl is-active reconx-dashboard.service
```

Expected: `active`

- [ ] **Step 4: Deploy new binary to worker VPSes**

The new scanner binary needs to land on each worker. The controller can SCP it using the stored SSH key:

```bash
KEY=/opt/reconx/.ssh/id_ed25519
for IP in 217.145.227.242 185.107.74.92; do
  echo "=== Deploying binary to $IP ==="
  # Find the active crack directory
  CRACK_DIR=$(ssh -i $KEY -o StrictHostKeyChecking=no root@$IP \
    'cat /tmp/reconx_session.conf 2>/dev/null | grep CRACK_DIR | cut -d= -f2' 2>/dev/null)
  if [ -n "$CRACK_DIR" ]; then
    scp -i $KEY /root/reconx-src/backend/reconx-scanner-linux root@$IP:${CRACK_DIR}/reconx-scanner-linux
    echo "Deployed to $IP:$CRACK_DIR"
  else
    echo "$IP: no active session — binary will be deployed on next crack start"
  fi
done
```

- [ ] **Step 5: Start a new crack to trigger watchdog install**

From the dashboard Cracker tab, start a new crack with both workers. The new `_dispatch_crack_worker` will automatically:
1. Upload the new binary (with persistent counters)
2. Start the scanner
3. Deploy the watchdog systemd service
4. Write `/tmp/reconx_session.conf`

- [ ] **Step 6: Verify watchdog is running on both workers**

```bash
KEY=/opt/reconx/.ssh/id_ed25519
for IP in 217.145.227.242 185.107.74.92; do
  echo "=== $IP ==="
  ssh -i $KEY -o StrictHostKeyChecking=no root@$IP \
    'systemctl is-active reconx-watchdog; cat /tmp/reconx_session.conf; tail -5 /tmp/reconx_watchdog.log 2>/dev/null'
done
```

Expected for each worker:
```
active
CRACK_DIR=/root/python_job/crack_<session_id>
SCANNER_BIN=/root/python_job/crack_<session_id>/reconx-scanner-linux
```

- [ ] **Step 7: Verify persistent counters work by simulating a restart**

```bash
KEY=/opt/reconx/.ssh/id_ed25519
IP=185.107.74.92

# Read current valid_hosts from stats.json
ssh -i $KEY root@$IP 'cat $(cat /tmp/reconx_session.conf | grep CRACK_DIR | cut -d= -f2)/ResultJS/stats.json' \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print('valid_hosts:', d.get('valid_hosts'))"

# Kill the scanner
ssh -i $KEY root@$IP 'pkill -x reconx-scanner-linux'

# Wait 15s for watchdog to restart it
sleep 15

# Read valid_hosts again — should be >= the previous value, not 0
ssh -i $KEY root@$IP 'cat $(cat /tmp/reconx_session.conf | grep CRACK_DIR | cut -d= -f2)/ResultJS/stats.json' \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print('valid_hosts after restart:', d.get('valid_hosts'))"
```

Expected: the second `valid_hosts` reading is ≥ the first (counter seeded, not reset to 0).

- [ ] **Step 8: Final commit with all changes together if not already done**

```bash
cd /Users/x/Downloads/scan
git status
git push origin main
```

---

## What This Fixes Permanently

| Problem | Fix |
|---------|-----|
| Scanner OOM killed, counters reset to 0 | Persistent counter seeding from stats.json on restart |
| Scanner crash → scan stops silently | systemd watchdog restarts within 10s |
| cpulimit killed by signal, CPU spikes to 100% | `CPUQuota=90%` in systemd scope (kernel cgroup, unkillable) |
| GOMEMLIMIT soft limit → OOM killer hits anyway | `MemoryMax=900M` in systemd scope (hard kernel limit) |
| VPS reboot → scanner never restarts | `systemctl enable reconx-watchdog` (starts on boot) |
| Auto-reboot on RAM < 128MB reset counters | Removed from poll loop |
| `max()` guard in session prevents display resets | Already shipped in app.py |

## Testing Checklist

- [ ] Kill scanner manually → watchdog restarts within 10s
- [ ] Reboot worker VPS → scanner auto-starts on boot  
- [ ] valid_hosts climbs monotonically (never goes backward)
- [ ] Stopping a crack from dashboard → watchdog stays quiet (no restart)
- [ ] Starting a new crack → watchdog gets updated session config
