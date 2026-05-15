# ReconX — controller install

ReconX has two roles:

| Role           | Runs                                          | Scans? |
|----------------|-----------------------------------------------|--------|
| **Controller** (one box) | Dashboard, Flask API, fleet HTTP API, paramiko SSH manager, scanner-binary builder | **No.** It compiles `reconx-scanner` and SFTPs it to workers. |
| **Workers**    (N boxes) | Receive `reconx-scanner` + `config.json` + their slice of targets via SSH, run the scan, write `ResultJS/*.txt`. | **Yes.** This is where credential discovery happens. |

The controller is the hub. It has the dashboard, the database (`raven_results.db`), the SSH key, and the fleet roster (`server_ips.txt`). It does not run the Go scanner locally.

---

## Install the controller — two ways

### A. Double-click from macOS (deploys to a remote VPS)

1. Provision a fresh Ubuntu 22.04+/24.04 or Debian 12+ VPS with root SSH access.
2. Double-click **`deploy-from-mac.command`** in Finder.
3. Enter the VPS IP, SSH user, and key path when prompted.
4. The script rsyncs the project to `/root/reconx-src/` on the VPS and runs `install-controller.sh` there.
5. Open `http://<vps-ip>/` when it finishes.

On first run, macOS may say "cannot be opened because it is from an unidentified developer." Right-click → **Open** → **Open** dismisses the warning permanently.

### B. Run directly on the VPS

```bash
# On the VPS, as root, with the project already copied to /root/reconx-src/
cd /root/reconx-src
bash install-controller.sh
```

---

## What the installer does

1. Installs apt packages: `python3`, `python3-venv`, `golang-go`, `nginx`, `redis-server`, `build-essential`, `nodejs` (20 LTS via NodeSource).
2. Creates the `reconx` service user with home `/opt/reconx`.
3. Rsyncs the project source into `/opt/reconx/`.
4. Creates a Python venv at `/opt/reconx/venv/` and installs `flask`, `flask-socketio`, `paramiko`, `gunicorn`, `eventlet`, plus anything in `backend/requirements.txt`.
5. Runs `npm ci && npm run build` in `dashboard/` — output goes to `dashboard/dist/`.
6. Builds the Go scanner binary at `/opt/reconx/backend/reconx-scanner` (Linux/amd64). This is the binary that gets uploaded to workers; **it is never executed on the controller**.
7. Generates an ed25519 SSH key at `/opt/reconx/.ssh/id_ed25519` if absent, and patches `backend/ssh_config.json` with the path.
8. Writes two systemd units:
   - `reconx-dashboard.service` — `gunicorn` running `backend/app.py` (Flask + socket.io) on `127.0.0.1:5000`.
   - `reconx-fleet-api.service` — `fleet_api.py` on `127.0.0.1:8787`.
9. Writes an nginx site at `/etc/nginx/sites-available/reconx`:
   - `/` → `dashboard/dist/` (static SPA)
   - `/api/*` and `/socket.io/*` → Flask backend
   - `/fleet-api/*` → fleet API
10. Starts every service and verifies each is `active`.

Re-running the script is safe — every step is idempotent (existing user, existing key, existing services are kept; only configs are rewritten).

---

## After install

The dashboard is at `http://<vps-ip>/`. The installer prints the controller's public SSH key — paste it into each worker's `~/.ssh/authorized_keys` so the fleet manager can connect.

Workers don't need a separate installer. The first time you click **Deploy** from the dashboard, `backend/ssh_manager.py::deploy_full()` runs an apt-install for `python3`, `redis-server`, and a handful of utilities on each worker over SSH, then uploads the scanner binary and target slice. After that, every redeploy just refreshes the target slice.

---

## Logs

```bash
journalctl -u reconx-dashboard -f
journalctl -u reconx-fleet-api -f
tail -F /var/log/nginx/access.log
```

## Re-running / updates

When the source changes, re-run the installer (or `deploy-from-mac.command`). It will rsync, rebuild the dashboard, rebuild the scanner binary, and restart services.

## Architecture diagram

```
                 ┌──────────────────────────────────────────┐
                 │         CONTROLLER VPS (hub only)        │
                 │                                          │
                 │  ┌────────────┐   ┌────────────────────┐ │
                 │  │ Dashboard  │   │ backend/app.py       │ │
                 │  │  (Vite)    │◄──┤   Flask + socket.io│ │
                 │  └────────────┘   │   :5000            │ │
                 │  ┌────────────┐   └─────────┬──────────┘ │
                 │  │ fleet_api  │             │            │
                 │  │  :8787     │             │            │
                 │  └─────┬──────┘             │            │
                 │        │      ┌─────────────┴──────────┐ │
                 │        │      │ backend/ssh_manager.py   │ │
                 │        └──────┤   (paramiko)           │ │
                 │               └──────────┬─────────────┘ │
                 │  reconx-scanner (Linux/amd64 binary)      │
                 │  config.json, paths.txt, server_ips.txt  │
                 │  raven_results.db (SQLite)               │
                 └────────────────────┬─────────────────────┘
                                      │ SSH/SFTP
                ┌─────────────────────┼─────────────────────┐
                ▼                     ▼                     ▼
        ┌───────────────┐     ┌───────────────┐     ┌───────────────┐
        │  Worker VPS 1 │     │  Worker VPS 2 │     │  Worker VPS N │
        │   raven-scan  │     │   raven-scan  │     │   raven-scan  │
        │   ResultJS/*  │     │   ResultJS/*  │     │   ResultJS/*  │
        └───────────────┘     └───────────────┘     └───────────────┘
```
