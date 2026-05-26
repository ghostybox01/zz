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
import re
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
REPO_SLUG      = os.environ.get('RECONX_REPO', 'ghostybox01/zz')
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
    # When we drop to a service user via sudo, sudo strips environment by
    # default — even vars we explicitly set in subprocess.run(env=…). Wrap
    # any caller-supplied env into a `sudo VAR=val` prefix so they actually
    # reach the inner command. (The dashboard rebuild needs RECONX_REPO to
    # bake __RECONX_REPO__ into the Vite bundle; without this, Settings →
    # Updates renders "(repo not set)".)
    if user:
        env_prefix = [f'{k}={v}' for k, v in (env or {}).items()]
        full = ['sudo', '-u', user] + env_prefix + cmd
    else:
        full = cmd
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
GO_VERSION = '1.24.2'  # minimum needed for pterm + modern AWS SDK
GO_INSTALL_PREFIX = Path('/usr/local')


def ensure_modern_go() -> None:
    """Install Go GO_VERSION to /usr/local/go if installed Go is too old."""
    needed = tuple(int(x) for x in GO_VERSION.split('.'))
    go_bin = '/usr/local/go/bin/go'
    have: tuple[int, ...] = ()
    try:
        out = subprocess.check_output([go_bin if os.path.exists(go_bin) else 'go', 'version']).decode()
        m = re.search(r'go(\d+)\.(\d+)(?:\.(\d+))?', out)
        if m:
            have = tuple(int(x or '0') for x in m.groups())
    except Exception:
        pass

    if have and have >= needed:
        log(f'Go {".".join(map(str, have))} already meets >= {GO_VERSION}')
    else:
        log(f'Installing Go {GO_VERSION} to /usr/local/go (have: {have or "none"})…')
        tarball = f'go{GO_VERSION}.linux-amd64.tar.gz'
        url = f'https://go.dev/dl/{tarball}'
        run(['rm', '-rf', '/usr/local/go'])
        subprocess.run(['curl', '-fsSL', '-o', f'/tmp/{tarball}', url], check=True)
        subprocess.run(['tar', '-C', str(GO_INSTALL_PREFIX), '-xzf', f'/tmp/{tarball}'], check=True)
        Path(f'/tmp/{tarball}').unlink(missing_ok=True)

    profile = Path('/etc/profile.d/golang.sh')
    profile.write_text('export PATH=$PATH:/usr/local/go/bin\n')
    profile.chmod(0o644)
    os.environ['PATH'] = f"/usr/local/go/bin:{os.environ.get('PATH', '')}"


def install_system_packages(args: argparse.Namespace) -> None:
    if args.skip_system:
        warn('skipping system packages (--skip-system)')
        ensure_modern_go()
        return
    log('Installing system packages…')
    env = {'DEBIAN_FRONTEND': 'noninteractive'}
    run(['apt-get', 'update', '-qq'], env=env)
    pkgs = [
        'python3', 'python3-venv', 'python3-pip',
        'git', 'curl', 'wget', 'tar', 'rsync',
        'nginx', 'build-essential', 'redis-server', 'ca-certificates',
    ]
    run(['apt-get', 'install', '-y', '-qq', *pkgs], env=env)

    ensure_modern_go()

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
        # Runtime data — never overwrite with rsync
        '--exclude=backend/lists/',
        '--exclude=backend/crack_sessions.json',
        '--exclude=backend/warc_state.json',
        '--exclude=backend/ssh_config.json',
        '--exclude=backend/fleet_creds.json',
        '--exclude=backend/dork_keys.json',
        '--exclude=backend/saved_dorks.json',
        '--exclude=backend/server_ips.txt',
        '--exclude=backend/targets.txt',
        '--exclude=backend/paths.txt',
        '--exclude=backend/dedup_log.txt',
        '--exclude=backend/.ssh/',
        '--exclude=backend/collected_results/',
        '--exclude=backend/valid_*.txt',
        '--exclude=.ssh/',
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
    # Remove stale untracked source files that survive git reset --hard
    # and would cause TypeScript compilation errors.
    _stale = [
        dash / 'src' / 'components' / 'DataImport.tsx',
        dash / 'src' / 'data' / 'demoSnapshot.ts',
    ]
    for _f in _stale:
        if _f.exists():
            _f.unlink()
            log(f'  removed stale file: {_f.relative_to(INSTALL_DIR)}')
    # Clear npm cache and wipe node_modules to avoid stale/corrupted tarballs
    # from prior installs (--prefer-offline can serve broken cached packages).
    run(['npm', 'cache', 'clean', '--force'], user=SERVICE_USER, cwd=dash, check=False)
    nm = dash / 'node_modules'
    if nm.exists():
        shutil.rmtree(nm)
    run(['npm', 'ci', '--no-audit', '--no-fund'], user=SERVICE_USER, cwd=dash)
    run(['npm', 'run', 'build'], user=SERVICE_USER, cwd=dash,
        env={'RECONX_REPO': REPO_SLUG})


def build_scanner(args: argparse.Namespace) -> None:
    log('building Go scanner binary (linux/amd64)…')
    backend = INSTALL_DIR / "backend"
    if args.dry_run:
        warn('  (dry run, skipping go build)')
        return
    sum_file = backend / 'go.sum'
    if sum_file.exists():
        sum_file.unlink()
    go = '/usr/local/go/bin/go' if Path('/usr/local/go/bin/go').exists() else 'go'
    if not (backend / 'go.mod').exists():
        run([go, 'mod', 'init', 'reconx-scanner'], user=SERVICE_USER, cwd=backend, check=False)
    go_env = {
        'GOOS': 'linux', 'GOARCH': 'amd64', 'GOTOOLCHAIN': 'local',
        'PATH': '/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',
        'HOME': str(INSTALL_DIR),
    }
    run([go, 'mod', 'tidy'], user=SERVICE_USER, cwd=backend, check=False, env=go_env)
    run([go, 'build', '-o', 'reconx-scanner', '.'],
        user=SERVICE_USER, cwd=backend, env=go_env)


def push_binary_to_workers() -> None:
    """After rebuilding reconx-scanner, SCP the new binary to every fleet worker.
    Silently skips workers that are unreachable — they'll get it on next bootstrap."""
    import json, socket
    binary = INSTALL_DIR / 'backend' / 'reconx-scanner'
    if not binary.exists():
        warn('  reconx-scanner binary not found — skipping worker push')
        return
    fleet_creds_path = INSTALL_DIR / 'backend' / 'fleet_creds.json'
    if not fleet_creds_path.exists():
        return
    try:
        fleet = json.loads(fleet_creds_path.read_text())
    except Exception:
        return
    if not fleet:
        return
    ssh_key = INSTALL_DIR / '.ssh' / 'id_ed25519'
    if not ssh_key.exists():
        warn('  SSH key not found — skipping worker push')
        return
    try:
        import paramiko
    except ImportError:
        warn('  paramiko not installed — skipping worker push (pip install paramiko)')
        return
    work_dir = '/root/python_job'
    for ip, creds in fleet.items():
        if (creds.get('role') or 'scanner') == 'warc':
            continue
        port = int(creds.get('port') or 22)
        user = str(creds.get('user') or 'root')
        try:
            sock = socket.create_connection((ip, port), timeout=5)
            sock.close()
        except Exception:
            warn(f'  {ip}: unreachable, skipping binary push')
            continue
        try:
            client = paramiko.SSHClient()
            client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            client.connect(ip, port=port, username=user,
                           key_filename=str(ssh_key), timeout=15, banner_timeout=15)
            client.exec_command(f'mkdir -p {work_dir}')
            sftp = client.open_sftp()
            # Upload to .new then atomically rename to avoid ETXTBSY on a
            # running binary. chmod before rename so the file is never
            # executable-but-partially-written.
            tmp = f'{work_dir}/reconx-scanner.new'
            sftp.get_channel().settimeout(180)
            sftp.put(str(binary), tmp)
            sftp.close()
            _, _, stderr = client.exec_command(f'chmod +x {tmp} && mv -f {tmp} {work_dir}/reconx-scanner')
            stderr.read()
            client.close()
            log(f'  pushed reconx-scanner → {user}@{ip}:{work_dir}/')
        except Exception as e:
            warn(f'  {ip}: binary push failed — {e}')


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
        ExecStart={INSTALL_DIR}/venv/bin/gunicorn --bind 127.0.0.1:{DASH_PORT} --worker-class gthread --workers 4 --threads 8 --timeout 300 --keep-alive 65 app:app
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
                proxy_read_timeout 300s;
                proxy_send_timeout 300s;
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


def install_update_helper() -> None:
    log('installing /usr/local/bin/reconx-update + sudoers rule…')
    # Capture the branch that is currently checked out so the update helper
    # stays pinned to it. Falls back to 'main' if HEAD is detached.
    try:
        import subprocess as _sp
        _branch = _sp.check_output(
            ['git', 'rev-parse', '--abbrev-ref', 'HEAD'],
            cwd=str(INSTALL_DIR), text=True,
        ).strip()
        if _branch == 'HEAD':  # detached HEAD
            _branch = 'main'
    except Exception:
        _branch = 'main'

    script = textwrap.dedent(f'''\
        #!/bin/bash
        set -euo pipefail
        cd {INSTALL_DIR}
        git fetch --quiet origin {_branch}
        git reset --hard origin/{_branch}
        exec /usr/bin/python3 {INSTALL_DIR}/installer/deploy.py --skip-system
    ''')
    target = Path('/usr/local/bin/reconx-update')
    target.write_text(script)
    target.chmod(0o755)

    sudoers = f'{SERVICE_USER} ALL=(root) NOPASSWD: /usr/local/bin/reconx-update\n'
    Path('/etc/sudoers.d/reconx-update').write_text(sudoers)
    Path('/etc/sudoers.d/reconx-update').chmod(0o440)



def start_services(args: argparse.Namespace) -> None:
    log('starting services…')
    if args.dry_run:
        warn('  (dry run, skipping systemctl)')
        return
    run(['systemctl', 'daemon-reload'])
    run(['systemctl', 'enable', '--now', 'redis-server'])
    # Explicit restart for our services so Python code changes are picked up
    # (enable --now does not restart an already-running service).
    for svc in ('reconx-dashboard', 'reconx-fleet-api'):
        run(['systemctl', 'enable', svc])
        run(['systemctl', 'restart', svc])
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
    push_binary_to_workers()
    pub_key = setup_ssh_key()
    write_systemd_units()
    write_nginx_site()
    if not args.dry_run:
        install_update_helper()
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
