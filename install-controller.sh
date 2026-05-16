#!/usr/bin/env bash
# install-controller.sh — one-shot installer for the ReconX CONTROLLER VPS.
#
# This box is the HUB. It does not scan. It compiles the Go scanner, stores the
# binary on disk, and SFTPs it to worker VPSes via backend/ssh_manager.py.
#
# Re-running this script is safe — every step is idempotent.
#
# Tested on: Ubuntu 22.04, Ubuntu 24.04, Debian 12.

set -euo pipefail

INSTALL_DIR="${RECONX_INSTALL_DIR:-/opt/reconx}"
SERVICE_USER="${RECONX_USER:-reconx}"
DASH_PORT="${RECONX_DASH_PORT:-5000}"
FLEET_PORT="${RECONX_FLEET_PORT:-8787}"
HTTP_PORT="${RECONX_HTTP_PORT:-80}"

GREEN=$'\033[0;32m'
YELLOW=$'\033[0;33m'
RED=$'\033[0;31m'
RESET=$'\033[0m'

log()  { echo "${GREEN}[reconx-install]${RESET} $*"; }
warn() { echo "${YELLOW}[reconx-install]${RESET} $*"; }
err()  { echo "${RED}[reconx-install] ERROR:${RESET} $*" >&2; exit 1; }

# ── 1. Sanity ──────────────────────────────────────────────────────────────
if [[ "$EUID" -ne 0 ]]; then
  err "Run as root: sudo bash $0"
fi
if [[ ! -f /etc/os-release ]]; then
  err "Cannot detect OS — /etc/os-release missing."
fi
# shellcheck disable=SC1091
. /etc/os-release
case "${ID:-}" in
  ubuntu|debian) ;;
  *) err "Only Ubuntu and Debian are supported. Detected: ${ID:-unknown}." ;;
esac
log "OS: $PRETTY_NAME"

SRC="$(cd "$(dirname "$0")" && pwd)"
log "Source: $SRC"
log "Install target: $INSTALL_DIR (as user $SERVICE_USER)"

# ── 2. System packages ────────────────────────────────────────────────────
log "Installing apt packages…"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq \
  python3 python3-venv python3-pip \
  golang-go \
  git curl wget tar rsync \
  nginx \
  build-essential \
  redis-server \
  ca-certificates

# Node 20 LTS via NodeSource if not already present
if ! command -v node >/dev/null 2>&1 || [[ "$(node -v 2>/dev/null | cut -c2- | cut -d. -f1)" -lt 18 ]]; then
  log "Installing Node.js 20 LTS via NodeSource…"
  curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
  apt-get install -y -qq nodejs
fi
log "Node $(node -v)  ·  npm $(npm -v)  ·  Go $(go version | awk '{print $3}')  ·  Python $(python3 -V | awk '{print $2}')"

# ── 3. Service user ───────────────────────────────────────────────────────
if ! id "$SERVICE_USER" &>/dev/null; then
  log "Creating service user '$SERVICE_USER'"
  useradd --system --shell /bin/bash --home "$INSTALL_DIR" --create-home "$SERVICE_USER"
else
  log "Service user '$SERVICE_USER' already exists"
fi

# ── 4. Sync source ────────────────────────────────────────────────────────
log "Syncing project into $INSTALL_DIR …"
mkdir -p "$INSTALL_DIR"
rsync -a --delete \
  --exclude='node_modules' \
  --exclude='dist' \
  --exclude='.git' \
  --exclude='.nelson' \
  --exclude='venv' \
  --exclude='.venv' \
  --exclude='__pycache__' \
  --exclude='ResultJS' \
  "$SRC/" "$INSTALL_DIR/"
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"

# ── 5. Python venv ────────────────────────────────────────────────────────
log "Creating Python venv + installing requirements…"
if [[ ! -d "$INSTALL_DIR/venv" ]]; then
  sudo -u "$SERVICE_USER" python3 -m venv "$INSTALL_DIR/venv"
fi
sudo -u "$SERVICE_USER" "$INSTALL_DIR/venv/bin/pip" install -q --upgrade pip
if [[ -f "$INSTALL_DIR/backend/requirements.txt" ]]; then
  sudo -u "$SERVICE_USER" "$INSTALL_DIR/venv/bin/pip" install -q -r "$INSTALL_DIR/backend/requirements.txt"
fi
# These are required by the prod service stack regardless of requirements.txt contents.
sudo -u "$SERVICE_USER" "$INSTALL_DIR/venv/bin/pip" install -q \
  flask flask-socketio paramiko gunicorn eventlet

# ── 6. Build the React dashboard ─────────────────────────────────────────
log "Building the React dashboard…"
cd "$INSTALL_DIR/dashboard"
sudo -u "$SERVICE_USER" npm ci --no-audit --no-fund --prefer-offline
sudo -u "$SERVICE_USER" npm run build
log "Dashboard built → $INSTALL_DIR/dashboard/dist"

# ── 7. Build the Go scanner binary (for upload to workers) ───────────────
log "Building the Go scanner binary for worker upload…"
cd "$INSTALL_DIR/backend"
sudo -u "$SERVICE_USER" rm -f go.sum
if [[ ! -f go.mod ]]; then
  sudo -u "$SERVICE_USER" go mod init reconx-scanner >/dev/null 2>&1 || true
fi
sudo -u "$SERVICE_USER" go mod tidy >/dev/null 2>&1 || warn "go mod tidy reported issues — continuing"
sudo -u "$SERVICE_USER" GOOS=linux GOARCH=amd64 go build -o reconx-scanner main.go
chmod +x "$INSTALL_DIR/backend/reconx-scanner"
log "Scanner binary built (Linux/amd64): $(du -h "$INSTALL_DIR/backend/reconx-scanner" | awk '{print $1}')"

# ── 7b. Build the Go warc harvester binary (controller-local) ─────────────
# Built at install time so the gunicorn service (which has no $PATH to
# /usr/bin/go) doesn't need to invoke `go build` at runtime. The build
# happens in a fresh tempdir, NOT in $INSTALL_DIR — `go mod tidy` would
# otherwise descend into $INSTALL_DIR/go/pkg/mod (the SERVICE_USER's
# module cache) and choke on the cached @version dirs. After build the
# binary is moved into place at $INSTALL_DIR/reconx-warc.
log "Building the Go warc harvester binary for controller use…"
WARC_BUILD_DIR=$(sudo -u "$SERVICE_USER" mktemp -d)
sudo -u "$SERVICE_USER" cp "$INSTALL_DIR/warc.go" "$WARC_BUILD_DIR/"
sudo -u "$SERVICE_USER" bash -c "
  cd '$WARC_BUILD_DIR' &&
  /usr/bin/go mod init reconx-warc >/dev/null 2>&1 &&
  /usr/bin/go get github.com/schollz/progressbar/v3 >/dev/null 2>&1 &&
  /usr/bin/go mod tidy >/dev/null 2>&1 &&
  GOOS=linux GOARCH=amd64 /usr/bin/go build -o '$INSTALL_DIR/reconx-warc' warc.go
" || warn "warc build reported issues — check manually"
sudo -u "$SERVICE_USER" rm -rf "$WARC_BUILD_DIR"
chmod +x "$INSTALL_DIR/reconx-warc" 2>/dev/null || true
if [[ -f "$INSTALL_DIR/reconx-warc" ]]; then
  log "WARC binary built (Linux/amd64): $(du -h "$INSTALL_DIR/reconx-warc" | awk '{print $1}')"
else
  warn "reconx-warc binary not produced — WARC tab will return 503 until manually built"
fi

# ── 8. SSH key for fleet ops ──────────────────────────────────────────────
SSH_DIR="$INSTALL_DIR/.ssh"
sudo -u "$SERVICE_USER" mkdir -p "$SSH_DIR"
sudo -u "$SERVICE_USER" chmod 700 "$SSH_DIR"
KEY_PATH="$SSH_DIR/id_ed25519"
if [[ ! -f "$KEY_PATH" ]]; then
  log "Generating SSH keypair for fleet operations…"
  sudo -u "$SERVICE_USER" ssh-keygen -t ed25519 -N '' -f "$KEY_PATH" -C "reconx-controller@$(hostname)" -q
fi
PUB_KEY=$(cat "${KEY_PATH}.pub")

# Patch ssh_config.json with the new key path
cat > "$INSTALL_DIR/backend/ssh_config.json" <<JSON
{
  "ssh_key_path": "$KEY_PATH",
  "remote_user": "root",
  "server_list_file": "$INSTALL_DIR/backend/server_ips.txt",
  "work_dir": "/root/python_job",
  "batch_size": 100000,
  "ssh_timeout": 5
}
JSON
[[ -f "$INSTALL_DIR/backend/server_ips.txt" ]] || sudo -u "$SERVICE_USER" touch "$INSTALL_DIR/backend/server_ips.txt"
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/backend"

# ── 9. systemd units ──────────────────────────────────────────────────────
log "Writing systemd units…"
cat > /etc/systemd/system/reconx-dashboard.service <<UNIT
[Unit]
Description=ReconX dashboard backend (Flask + socket.io)
After=network.target redis-server.service

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR/backend
Environment=PYTHONUNBUFFERED=1
ExecStart=$INSTALL_DIR/venv/bin/gunicorn --bind 127.0.0.1:$DASH_PORT --worker-class eventlet --workers 2 --timeout 60 app:app
Restart=on-failure
RestartSec=4
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
UNIT

cat > /etc/systemd/system/reconx-fleet-api.service <<UNIT
[Unit]
Description=ReconX fleet HTTP API (paramiko fleet control plane)
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
Environment=RAVENX_GO=$INSTALL_DIR/backend/main.go
Environment=RAVENX_CONFIG=$INSTALL_DIR/backend/config.json
ExecStart=$INSTALL_DIR/venv/bin/python fleet_api.py --host 127.0.0.1 --port $FLEET_PORT
Restart=on-failure
RestartSec=4
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
UNIT

# ── 10. Nginx reverse proxy ───────────────────────────────────────────────
log "Configuring nginx site…"
cat > /etc/nginx/sites-available/reconx <<NGINX
server {
    listen $HTTP_PORT default_server;
    listen [::]:$HTTP_PORT default_server;
    server_name _;

    # Built dashboard (Vite output)
    root $INSTALL_DIR/dashboard/dist;
    index index.html;
    location / {
        try_files \$uri \$uri/ /index.html;
    }

    # Flask API
    location /api/ {
        proxy_pass         http://127.0.0.1:$DASH_PORT;
        proxy_http_version 1.1;
        proxy_set_header   Host              \$host;
        proxy_set_header   X-Real-IP         \$remote_addr;
        proxy_set_header   X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
        proxy_read_timeout 90s;
    }

    # socket.io
    location /socket.io/ {
        proxy_pass         http://127.0.0.1:$DASH_PORT;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade    \$http_upgrade;
        proxy_set_header   Connection "upgrade";
        proxy_set_header   Host       \$host;
        proxy_read_timeout 86400s;
    }

    # Fleet HTTP API (paramiko control plane)
    location /fleet-api/ {
        rewrite ^/fleet-api/(.*)\$ /\$1 break;
        proxy_pass       http://127.0.0.1:$FLEET_PORT;
        proxy_set_header Host \$host;
    }

    client_max_body_size 64m;
}
NGINX
ln -sf /etc/nginx/sites-available/reconx /etc/nginx/sites-enabled/reconx
rm -f /etc/nginx/sites-enabled/default
nginx -t

# ── 11. Start everything ──────────────────────────────────────────────────
log "Starting services…"
systemctl daemon-reload
systemctl enable --now redis-server
systemctl enable --now reconx-dashboard
systemctl enable --now reconx-fleet-api
systemctl reload nginx

# Brief wait + health check
sleep 2
for svc in reconx-dashboard reconx-fleet-api nginx; do
  if systemctl is-active --quiet "$svc"; then
    log "  ✓ $svc is active"
  else
    warn "  ✗ $svc is NOT active — check journalctl -u $svc"
  fi
done

PUBLIC_IP=$(hostname -I | awk '{print $1}')

# ── 12. Done ──────────────────────────────────────────────────────────────
cat <<EOF

${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}
${GREEN}✓ ReconX controller installed${RESET}

  Dashboard:     http://${PUBLIC_IP}/
  Install dir:   $INSTALL_DIR
  Service user:  $SERVICE_USER

  This box is the HUB. It does not run scans. Workers do.
  ${YELLOW}Scanner binary built and waiting at:${RESET}
    $INSTALL_DIR/backend/reconx-scanner

${YELLOW}Next steps:${RESET}
  1. Add worker IPs to $INSTALL_DIR/backend/server_ips.txt (one per line),
     either by editing the file directly or via the dashboard.
  2. Distribute the controller's public key to each worker:
       cat $KEY_PATH.pub
     Then on each worker:
       echo "<pubkey>" >> ~/.ssh/authorized_keys
  3. Open the dashboard → Fleet tab → 'Test SSH' to verify connectivity.
  4. Upload a target list → Fleet → Deploy.

${YELLOW}Logs:${RESET}
  journalctl -u reconx-dashboard -f
  journalctl -u reconx-fleet-api -f

${YELLOW}Public key for worker authorized_keys:${RESET}
$PUB_KEY

${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}
EOF
