#!/usr/bin/env python3
"""ReconX controller deployer.

One file. Idempotent. Run on a fresh Ubuntu 22.04+/24.04 or Debian 12+ VPS as root.

The controller is the HUB:
  - serves the dashboard (nginx + flask + socket.io)
  - holds the fleet roster and the SSH key
  - compiles the Go scanner and SFTPs it to worker VPSes
  - never executes the scanner locally

Usage:
  sudo python3 installer/deploy.py
  sudo python3 installer/deploy.py --dry-run          # print plan, no changes
  sudo python3 installer/deploy.py --skip-system      # skip apt-get install
  sudo python3 installer/deploy.py --port 8080        # nginx HTTP port
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
import textwrap
from pathlib import Path

# ── Configuration ─────────────────────────────────────────────────────────
INSTALL_DIR    = Path(os.environ.get('RECONX_INSTALL_DIR', '/opt/reconx'))
SERVICE_USER   = os.environ.get('RECONX_USER', 'reconx')
DASH_PORT      = int(os.environ.get('RECONX_DASH_PORT', '5000'))
FLEET_PORT     = int(os.environ.get('RECONX_FLEET_PORT', '8787'))
HTTP_PORT      = int(os.environ.get('RECONX_HTTP_PORT', '80'))
SOURCE_DIR     = Path(__file__).resolve().parent.parent  # project root
GREEN, YELLOW, RED, RESET = '\033[0;32m', '\033[0;33m', '\033[0;31m', '\033[0m'


def log(msg: str) -> None:
    print(f'{GREEN}[reconx]{RESET} {msg}', flush=True)


def warn(msg: str) -> None:
    print(f'{YELLOW}[reconx]{RESET} {msg}', flush=True)


def die(msg: str) -> None:
    print(f'{RED}[reconx] ERROR:{RESET} {msg}', file=sys.stderr)
    sys.exit(1)


def run(cmd: list[str], *, user: str | None = None, cwd: Path | None = None, check: bool = True, env: dict | None = None) -> subprocess.CompletedProcess:
    full = ['sudo', '-u', user] + cmd if user else cmd
    return subprocess.run(full, cwd=str(cwd) if cwd else None, check=check, env={**os.environ, **(env or {})})


def ensure_root() -> None:
    if os.geteuid() != 0:
        die('run as root: sudo python3 installer/deploy.py')


def detect_os() -> str:
    rel = Path('/etc/os-release')
    if not rel.exists():
        die('cannot detect OS — /etc/os-release missing')
    text = rel.read_text()
    if 'ubuntu' in text.lower():
        return 'ubuntu'
    if 'debian' in text.lower():
        return 'debian'
    die(f'only Ubuntu/Debian supported. /etc/os-release:\n{text}')


# ── Step implementations ──────────────────────────────────────────────────
def install_system_packages(args: argparse.Namespace) -> None:
    if args.skip_system:
        warn('skipping system packages (--skip-system)')
        return
    log('Installing system packages…')
    env = {'DEBIAN_FRONTEND': 'noninteractive'}
    run(['apt-get', 'update', '-qq'], env=env)
    pkgs = [
        'python3', 'python3-venv', 'python3-pip',
        'golang-go', 'git', 'curl', 'wget', 'tar', 'rsync',
        'nginx', 'build-essential', 'redis-server', 'ca-certificates',
    ]
    run(['apt-get', 'install', '-y', '-qq', *pkgs], env=env)

    try:
        v = subprocess.check_output(['node', '-v']).decode().strip().lstrip('v')
        major = int(v.split('.')[0])
    except Exception:
        major = 0
    if major < 18:
        log('Installing Node.js 20 LTS via NodeSource…')
        subprocess.run(
            'curl -fsSL https://deb.nodesource.com/setup_20.x | bash -',
            shell=True, check=True
        )
        run(['apt-get', 'install', '-y', '-qq', 'nodejs'], env=env)


def ensure_service_user() -> None:
    try:
        subprocess.check_output(['id', SERVICE_USER])
        log(f'service user {SERVICE_USER!r} exists')
    except subprocess.CalledProcessError:
        log(f'creating service user {SERVICE_USER!r}')
        run(['useradd', '--system', '--shell', '/bin/bash',
             '--home', str(INSTALL_DIR), '--create-home', SERVICE_USER])


def sync_source(args: argparse.Namespace) -> None:
    log(f'syncing project to {INSTALL_DIR}…')
    INSTALL_DIR.mkdir(parents=True, exist_ok=True)
    if args.dry_run:
        warn('  (dry run, skipping rsync)')
        return
    run([
        'rsync', '-a', '--delete',
        '--exclude=node_modules', '--exclude=dist', '--exclude=.git',
        '--exclude=.nelson', '--exclude=venv', '--exclude=.venv',
        '--exclude=__pycache__', '--exclude=ResultJS', '--exclude=installer/cache',
        f'{SOURCE_DIR}/', f'{INSTALL_DIR}/',
    ])
    run(['chown', '-R', f'{SERVICE_USER}:{SERVICE_USER}', str(INSTALL_DIR)])


def setup_python_venv(args: argparse.Namespace) -> None:
    log('creating Python venv + installing requirements…')
    venv = INSTALL_DIR / 'venv'
    if not venv.exists() and not args.dry_run:
        run(['python3', '-m', 'venv', str(venv)], user=SERVICE_USER)
    pip = venv / 'bin' / 'pip'
    if args.dry_run:
        warn('  (dry run, skipping pip install)')
        return
    run([str(pip), 'install', '-q', '--upgrade', 'pip'], user=SERVICE_USER)
    req = INSTALL_DIR / 'backend' / 'requirements.txt'
    if req.exists():
        run([str(pip), 'install', '-q', '-r', str(req)], user=SERVICE_USER)
    run([str(pip), 'install', '-q',
         'flask', 'flask-socketio', 'paramiko', 'gunicorn', 'eventlet'],
        user=SERVICE_USER)


def build_dashboard(args: argparse.Namespace) -> None:
    log('building dashboard (npm)…')
    if args.dry_run:
        warn('  (dry run, skipping npm)')
        return
    dash = INSTALL_DIR / 'dashboard'
    run(['npm', 'ci', '--no-audit', '--no-fund', '--prefer-offline'], user=SERVICE_USER, cwd=dash)
    run(['npm', 'run', 'build'], user=SERVICE_USER, cwd=dash)


def build_scanner(args: argparse.Namespace) -> None:
    log('building Go scanner binary (linux/amd64)…')
    backend = INSTALL_DIR / "backend"
    if args.dry_run:
        warn('  (dry run, skipping go build)')
        return
    sum_file = raven / 'go.sum'
    if sum_file.exists():
        sum_file.unlink()
    if not (raven / 'go.mod').exists():
        run(['go', 'mod', 'init', 'reconx-scanner'], user=SERVICE_USER, cwd=raven, check=False)
    run(['go', 'mod', 'tidy'], user=SERVICE_USER, cwd=raven, check=False)
    run(['go', 'build', '-o', 'reconx-scanner', 'main.go'],
        user=SERVICE_USER, cwd=raven,
        env={'GOOS': 'linux', 'GOARCH': 'amd64'})


def setup_ssh_key() -> str:
    log('ensuring SSH key for fleet ops…')
    ssh_dir = INSTALL_DIR / '.ssh'
    ssh_dir.mkdir(exist_ok=True)
    run(['chmod', '700', str(ssh_dir)])
    run(['chown', '-R', f'{SERVICE_USER}:{SERVICE_USER}', str(ssh_dir)])
    key = ssh_dir / 'id_ed25519'
    if not key.exists():
        run(['ssh-keygen', '-t', 'ed25519', '-N', '', '-f', str(key),
             '-C', f'reconx-controller@{os.uname().nodename}', '-q'],
            user=SERVICE_USER)
    pub = (ssh_dir / 'id_ed25519.pub').read_text().strip()

    cfg = INSTALL_DIR / 'backend' / 'ssh_config.json'
    cfg.write_text(textwrap.dedent(f'''\
        {{
          "ssh_key_path": "{key}",
          "remote_user": "root",
          "server_list_file": "{INSTALL_DIR}/backend/server_ips.txt",
          "work_dir": "/root/python_job",
          "batch_size": 100000,
          "ssh_timeout": 5
        }}
    '''))
    server_ips = INSTALL_DIR / 'backend' / 'server_ips.txt'
    if not server_ips.exists():
        server_ips.touch()
    run(['chown', '-R', f'{SERVICE_USER}:{SERVICE_USER}', str(INSTALL_DIR / 'backend')])
    return pub


def write_systemd_units() -> None:
    log('writing systemd units…')
    dashboard_unit = textwrap.dedent(f'''\
        [Unit]
        Description=ReconX dashboard backend (Flask + socket.io)
        After=network.target redis-server.service

        [Service]
        Type=simple
        User={SERVICE_USER}
        WorkingDirectory={INSTALL_DIR}/backend
        Environment=PYTHONUNBUFFERED=1
        ExecStart={INSTALL_DIR}/venv/bin/gunicorn --bind 127.0.0.1:{DASH_PORT} --worker-class eventlet --workers 2 --timeout 60 app:app
        Restart=on-failure
        RestartSec=4

        [Install]
        WantedBy=multi-user.target
    ''')
    fleet_unit = textwrap.dedent(f'''\
        [Unit]
        Description=ReconX fleet HTTP API (paramiko fleet control plane)
        After=network.target

        [Service]
        Type=simple
        User={SERVICE_USER}
        WorkingDirectory={INSTALL_DIR}
        Environment=RAVENX_GO={INSTALL_DIR}/backend/main.go
        Environment=RAVENX_CONFIG={INSTALL_DIR}/backend/config.json
        ExecStart={INSTALL_DIR}/venv/bin/python fleet_api.py --host 127.0.0.1 --port {FLEET_PORT}
        Restart=on-failure
        RestartSec=4

        [Install]
        WantedBy=multi-user.target
    ''')
    Path('/etc/systemd/system/reconx-dashboard.service').write_text(dashboard_unit)
    Path('/etc/systemd/system/reconx-fleet-api.service').write_text(fleet_unit)


def write_nginx_site() -> None:
    log('configuring nginx site…')
    site = textwrap.dedent(f'''\
        server {{
            listen {HTTP_PORT} default_server;
            listen [::]:{HTTP_PORT} default_server;
            server_name _;

            root {INSTALL_DIR}/dashboard/dist;
            index index.html;
            location / {{
                try_files $uri $uri/ /index.html;
            }}

            location /api/ {{
                proxy_pass         http://127.0.0.1:{DASH_PORT};
                proxy_http_version 1.1;
                proxy_set_header   Host              $host;
                proxy_set_header   X-Real-IP         $remote_addr;
                proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
                proxy_set_header   X-Forwarded-Proto $scheme;
                proxy_read_timeout 90s;
            }}

            location /socket.io/ {{
                proxy_pass         http://127.0.0.1:{DASH_PORT};
                proxy_http_version 1.1;
                proxy_set_header   Upgrade    $http_upgrade;
                proxy_set_header   Connection "upgrade";
                proxy_set_header   Host       $host;
                proxy_read_timeout 86400s;
            }}

            location /fleet-api/ {{
                rewrite ^/fleet-api/(.*)$ /$1 break;
                proxy_pass       http://127.0.0.1:{FLEET_PORT};
                proxy_set_header Host $host;
            }}

            client_max_body_size 64m;
        }}
    ''')
    Path('/etc/nginx/sites-available/reconx').write_text(site)
    avail = Path('/etc/nginx/sites-available/reconx')
    enabled = Path('/etc/nginx/sites-enabled/reconx')
    if enabled.is_symlink() or enabled.exists():
        enabled.unlink()
    enabled.symlink_to(avail)
    default = Path('/etc/nginx/sites-enabled/default')
    if default.exists():
        default.unlink()
    run(['nginx', '-t'])


def start_services(args: argparse.Namespace) -> None:
    log('starting services…')
    if args.dry_run:
        warn('  (dry run, skipping systemctl)')
        return
    run(['systemctl', 'daemon-reload'])
    for svc in ('redis-server', 'reconx-dashboard', 'reconx-fleet-api'):
        run(['systemctl', 'enable', '--now', svc])
    run(['systemctl', 'reload', 'nginx'])

    for svc in ('reconx-dashboard', 'reconx-fleet-api', 'nginx'):
        r = subprocess.run(['systemctl', 'is-active', '--quiet', svc])
        if r.returncode == 0:
            log(f'  ✓ {svc} active')
        else:
            warn(f'  ✗ {svc} NOT active — journalctl -u {svc}')


# ── Main ──────────────────────────────────────────────────────────────────
def main() -> int:
    parser = argparse.ArgumentParser(description='ReconX controller deployer')
    parser.add_argument('--dry-run', action='store_true', help='show plan, no changes')
    parser.add_argument('--skip-system', action='store_true', help='skip apt-get install')
    parser.add_argument('--port', type=int, default=HTTP_PORT, help='nginx HTTP port')
    args = parser.parse_args()

    if args.port != HTTP_PORT:
        globals()['HTTP_PORT'] = args.port

    ensure_root()
    distro = detect_os()
    log(f'OS: {distro}')
    log(f'Install target: {INSTALL_DIR} (user {SERVICE_USER})')

    install_system_packages(args)
    ensure_service_user()
    sync_source(args)
    setup_python_venv(args)
    build_dashboard(args)
    build_scanner(args)
    pub_key = setup_ssh_key()
    write_systemd_units()
    write_nginx_site()
    start_services(args)

    public_ip = subprocess.check_output(['hostname', '-I']).decode().split()[0]
    print()
    print(f'{GREEN}━' * 73 + RESET)
    print(f'{GREEN}✓ ReconX controller installed{RESET}')
    print()
    print(f'  Dashboard:    http://{public_ip}/')
    print(f'  Install dir:  {INSTALL_DIR}')
    print(f'  Service user: {SERVICE_USER}')
    print()
    print(f'  This box is the HUB. It does NOT run scans. Workers do.')
    print(f'  Scanner binary built at {INSTALL_DIR}/backend/reconx-scanner')
    print()
    print(f'{YELLOW}Next steps:{RESET}')
    print(f'  1. From dashboard → Settings → Fleet bootstrap, paste worker IPs and SSH creds.')
    print(f'     (Or edit {INSTALL_DIR}/backend/server_ips.txt directly.)')
    print(f'  2. Distribute the controller pubkey to each worker:')
    print(f'       cat {INSTALL_DIR}/.ssh/id_ed25519.pub')
    print()
    print(f'{YELLOW}Controller public key:{RESET}')
    print(f'  {pub_key}')
    print()
    print(f'  Logs: journalctl -u reconx-dashboard -f')
    print(f'        journalctl -u reconx-fleet-api -f')
    print(f'{GREEN}━' * 73 + RESET)

    return 0


if __name__ == '__main__':
    sys.exit(main())
