#!/usr/bin/env python3
"""
RAVEN SSH Manager - Full Deployment Orchestration
Manages remote VPS servers via SSH from the web dashboard

Features:
- Test connections before deployment
- Split target lists equally across servers
- Upload scanner files
- Install requirements
- Create batch files
- Start/Stop/Monitor operations

Created by https://t.me/boxxboyy
"""

import paramiko
import os
import threading
import time
import json
import logging
import tempfile
from datetime import datetime
from typing import Dict, List, Optional, Callable
from dataclasses import dataclass, asdict, field

# Suppress paramiko logging
logging.getLogger("paramiko").setLevel(logging.CRITICAL)

# Sidecar written by /api/fleet/bulk-creds and /api/fleet/install-keys.
# Maps `ip → {user, port, auth_kind}` so the SSH manager (which runs as
# root by default) can still log into workers that authenticated as a
# non-root user during bootstrap.
FLEET_CREDS_FILE = 'fleet_creds.json'


# ================= CONFIGURATION =================
DEFAULT_CONFIG = {
    "ssh_key_path": "/root/ssh/1",
    "remote_user": "root",
    "server_list_file": "server_ips.txt",
    "work_dir": "/root/python_job",
    "results_dir": "./collected_results",
    "result_subdir": "ResultJS",
    "dedup_file": "dedup_log.txt",
    "batch_size": 100000,
    "ssh_timeout": 5,  # Reduced from 10
    "monitor_interval": 30,  # seconds between fleet probes — was 5s; bumped to cut reconnect churn
    "ssh_keepalive": 30,     # paramiko transport keepalive (seconds)
    "scanner_file": "main.py",
    "runner_file": "batch_runner.sh",
    "target_file": "targets.txt"
}

@dataclass
class ServerStatus:
    ip: str
    status: str = "UNKNOWN"
    scanned: int = 0
    targets: int = 0
    hits: int = 0
    speed: float = 0
    uptime: str = "-"
    batch_info: str = "-"
    batches_done: int = 0
    batches_total: int = 0
    current_batch_progress: int = 0
    last_update: str = ""
    error: Optional[str] = None
    # Live machine metrics — added so the dashboard cards show real CPU%,
    # RAM used/total, disk used/total, and system uptime. Populated by the
    # probe script from /proc/{stat,meminfo,uptime} and df -Pk. Defaults
    # are zeros so cards in UNREACHABLE state don't crash the serializer.
    cpu_percent: float = 0.0
    ram_used_gb: float = 0.0
    ram_total_gb: float = 0.0
    disk_used_gb: float = 0.0
    disk_total_gb: float = 0.0
    sys_uptime_sec: int = 0
    # Last time we got a successful probe — lets the card show "last seen
    # 4 min ago" when the box is currently UNREACHABLE but was healthy
    # recently. Distinct from last_update which is just the heartbeat.
    last_good_update: str = ""

    def to_dict(self):
        return asdict(self)


@dataclass
class DeploymentResult:
    ip: str
    success: bool
    targets_assigned: int = 0
    message: str = ""
    steps: List[str] = field(default_factory=list)


class SSHManager:
    def __init__(self, config_path: str = "ssh_config.json"):
        self.config_path = config_path
        self.config = self._load_config()
        self.servers: Dict[str, ServerStatus] = {}
        self.previous_scanned: Dict[str, int] = {}
        self.lock = threading.Lock()
        self.stop_monitor = threading.Event()
        self.monitor_thread: Optional[threading.Thread] = None
        self.event_callback: Optional[Callable] = None
        self.deployment_log: List[str] = []
        # Per-worker SSH connection pool. paramiko Transport is thread-safe for
        # opening independent channels, so multiple ssh_exec calls can share
        # one SSHClient. This avoids the ~9 connect/teardown cycles per probe
        # that previously made every worker show "RECONNECT" in the UI.
        self._ssh_pool: Dict[str, paramiko.SSHClient] = {}
        self._pool_lock = threading.Lock()
        # Per-IP {user, port} overrides written by the bulk-creds /
        # install-keys flows. Lets us SSH as `admin` to workers that
        # authenticated as admin while still defaulting to remote_user for
        # workers added the legacy way (server_ips.txt + controller key).
        self._fleet_creds: Dict[str, dict] = {}
        self._fleet_creds_mtime: float = 0.0
        self._reload_fleet_creds_if_changed()
        # Sticky status — a worker has to miss this many consecutive
        # probes before its card flips to UNREACHABLE. At a 30s monitor
        # interval, threshold=3 means ~90s of solid failure before the UI
        # admits anything's wrong, which masks every kind of transient
        # blip (NAT idle-flush, sshd MaxStartups bursts, fail2ban,
        # half-second packet loss).
        self._consecutive_misses: Dict[str, int] = {}
        # Per-IP last SSH error message. Populated when ssh_exec catches an
        # exception (auth failure, network drop, banner timeout). Consumed
        # by fetch_server_status to replace the bare "ssh probe returned no
        # data" string on the card with the real cause.
        self._last_ssh_error: Dict[str, str] = {}
        # IPs whose sshd refuses `exec` channel requests — only `shell`
        # channels work. Once a host trips Channel-closed-on-exec we mark
        # it here so future probes skip exec_command entirely and run via
        # invoke_shell. Saves a wasted round-trip per cycle.
        self._exec_blocked_ips: set = set()
        self._sticky_threshold = 3

        # ── WARC status cache (Effect 3) ─────────────────────────────
        # Filled each monitor cycle from the worker that's currently
        # running the WARC harvest. api_warc_status reads from this
        # instead of issuing 3+ blocking SSH calls per request, which
        # was costing 5–23 s under load and starving eventlet workers.
        # Keyed by worker IP → {alive, domains_found, log_tail,
        # remote_pid, cached_at}.
        self._warc_status_cache: Dict[str, dict] = {}
        self._warc_status_lock = threading.Lock()

        # ── R2 health cache (Effect 4) ───────────────────────────────
        # Refreshed each monitor cycle via an app-injected probe
        # callable (set via set_r2_health_probe). The probe runs
        # head_bucket + usage breakdown per configured account and
        # reports back the full shape. The /api/upload/r2-config GET
        # endpoint and the dashboard's account cards read this cache
        # so neither pays for an inline round-trip per render.
        #
        # The legacy single-account keys (state/last_error/usage) are
        # kept at the top level and mirror whichever account the
        # priority picker named primary on the last cycle, so old
        # frontends and the existing 75/95% toast watcher still work
        # while the new multi-account dashboard ships.
        self._r2_health_cache: dict = {
            'accounts': [],        # list of {id, label, state, last_error, usage}
            'primary_id': None,
            'all_full': False,
            'state': 'unknown',
            'last_check': None,
            'last_error': None,
            'usage': None,
        }
        self._r2_health_lock = threading.Lock()
        self._r2_health_probe: Optional[Callable[[], dict]] = None

        self.junk_patterns = [
            'mysqli.', 'mysql.', 'pdo_mysql.', 'pdo.', 'session.', 'mail.', 'smtp.',
            'sendmail', 'default_socket', 'default_port', 'allow_persistent',
            'allow_local_infile', 'system,', 'popen,', 'passthru,', 'proc_open,',
            'shell_exec,', 'api_key', 'api_secret', '_path', '_dir', '_file',
            'true', 'false', 'null', 'none'
        ]
    
    def _load_config(self) -> dict:
        if os.path.exists(self.config_path):
            try:
                with open(self.config_path, 'r') as f:
                    loaded = json.load(f)
                    return {**DEFAULT_CONFIG, **loaded}
            except Exception as e:
                print(f"Error loading config: {e}")
        return DEFAULT_CONFIG.copy()
    
    def save_config(self):
        with open(self.config_path, 'w') as f:
            json.dump(self.config, f, indent=2)
    
    def get_config(self) -> dict:
        return self.config.copy()
    
    def update_config(self, updates: dict):
        self.config.update(updates)
        self.save_config()
    
    def load_servers(self) -> List[str]:
        server_file = self.config.get("server_list_file", "server_ips.txt")
        if not os.path.exists(server_file):
            return []
        with open(server_file, 'r') as f:
            return [line.strip() for line in f if line.strip() and not line.startswith('#')]
    
    def save_servers(self, ips: List[str]):
        server_file = self.config.get("server_list_file", "server_ips.txt")
        with open(server_file, 'w') as f:
            f.write('\n'.join(ips))
    
    def list_local_files(self, directory: str = ".", extensions: List[str] = None) -> List[dict]:
        """List files on the local VPS (where this panel runs)"""
        extensions = extensions or ['.txt', '.csv', '.list']
        files = []
        
        # Check current directory and common locations
        search_dirs = [
            directory,
            os.path.expanduser("~"),
            "/root",
            "/tmp",
            self.config.get("work_dir", "/root/python_job")
        ]
        
        seen = set()
        for search_dir in search_dirs:
            if not os.path.exists(search_dir):
                continue
            try:
                for filename in os.listdir(search_dir):
                    filepath = os.path.join(search_dir, filename)
                    if os.path.isfile(filepath):
                        ext = os.path.splitext(filename)[1].lower()
                        if ext in extensions or not extensions:
                            if filepath not in seen:
                                seen.add(filepath)
                                try:
                                    stat = os.stat(filepath)
                                    # Count lines for txt files
                                    line_count = 0
                                    if ext == '.txt' and stat.st_size < 500 * 1024 * 1024:  # < 500MB
                                        try:
                                            with open(filepath, 'r', errors='ignore') as f:
                                                line_count = sum(1 for _ in f)
                                        except:
                                            pass
                                    
                                    files.append({
                                        "name": filename,
                                        "path": filepath,
                                        "size": stat.st_size,
                                        "size_human": self._format_size(stat.st_size),
                                        "modified": datetime.fromtimestamp(stat.st_mtime).strftime('%Y-%m-%d %H:%M'),
                                        "lines": line_count
                                    })
                                except:
                                    pass
            except PermissionError:
                continue
        
        # Sort by modification time, newest first
        files.sort(key=lambda x: x.get('modified', ''), reverse=True)
        return files[:50]  # Limit to 50 files
    
    def _format_size(self, size: int) -> str:
        """Format file size to human readable"""
        for unit in ['B', 'KB', 'MB', 'GB']:
            if size < 1024:
                return f"{size:.1f} {unit}"
            size /= 1024
        return f"{size:.1f} TB"
    
    def get_file_line_count(self, filepath: str) -> dict:
        """Get line count of a file"""
        if not os.path.exists(filepath):
            return {"success": False, "error": "File not found"}
        try:
            with open(filepath, 'r', errors='ignore') as f:
                count = sum(1 for line in f if line.strip())
            return {"success": True, "path": filepath, "lines": count}
        except Exception as e:
            return {"success": False, "error": str(e)}
    
    def _load_private_key(self, key_path: str):
        """Load private key, handling both PEM and OpenSSH formats"""
        if not key_path or not os.path.exists(key_path):
            return None
        
        # Try paramiko's native loading first (works for PEM format)
        key_classes = [
            paramiko.RSAKey,
            paramiko.Ed25519Key,
            paramiko.ECDSAKey,
            paramiko.DSSKey,
        ]
        
        for key_class in key_classes:
            try:
                return key_class.from_private_key_file(key_path)
            except:
                pass
        
        # If all fail, try converting OpenSSH format using cryptography
        try:
            from cryptography.hazmat.primitives import serialization
            import io
            
            with open(key_path, 'rb') as f:
                key_data = f.read()
            
            # Load the OpenSSH format key
            private_key = serialization.load_ssh_private_key(
                key_data,
                password=None
            )
            
            # Convert to PEM format in memory
            pem_data = private_key.private_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PrivateFormat.TraditionalOpenSSL,
                encryption_algorithm=serialization.NoEncryption()
            )
            
            # Load with paramiko from the PEM data
            return paramiko.RSAKey.from_private_key(io.StringIO(pem_data.decode()))
        except Exception:
            pass
        
        return None
    
    def _reload_fleet_creds_if_changed(self) -> None:
        """Pick up any new entries the bulk-creds / install-keys endpoints
        wrote to fleet_creds.json, without re-reading the file on every
        connect."""
        try:
            mtime = os.path.getmtime(FLEET_CREDS_FILE) if os.path.exists(FLEET_CREDS_FILE) else 0.0
            if mtime == self._fleet_creds_mtime:
                return
            if mtime == 0.0:
                self._fleet_creds = {}
            else:
                with open(FLEET_CREDS_FILE, 'r') as f:
                    loaded = json.load(f)
                self._fleet_creds = loaded if isinstance(loaded, dict) else {}
            self._fleet_creds_mtime = mtime
        except Exception:
            pass

    def _user_for(self, ip: str) -> str:
        self._reload_fleet_creds_if_changed()
        entry = self._fleet_creds.get(ip) or {}
        return entry.get('user') or self.config.get('remote_user', 'root')

    def _port_for(self, ip: str) -> int:
        self._reload_fleet_creds_if_changed()
        entry = self._fleet_creds.get(ip) or {}
        try:
            return int(entry.get('port') or 22)
        except (TypeError, ValueError):
            return 22

    def _persist_specs_if_new(self, ip: str, metrics: dict) -> None:
        """Stash the first successful probe's hardware specs into
        fleet_creds.json so deploy_full can size batches per worker without
        re-probing. Idempotent: once `ram_gb` is set for an ip, we bail.

        Uses self.lock to serialise the read-modify-write against the rest
        of SSHManager's mutating paths. Note: app.py's _save_fleet_creds
        also writes this file without going through this lock — that's the
        existing pattern and the mtime-based reload in
        _reload_fleet_creds_if_changed picks up either writer's changes."""
        try:
            self._reload_fleet_creds_if_changed()
            if (self._fleet_creds.get(ip, {}) or {}).get('ram_gb'):
                return
            try:
                ram_gb = round(int(metrics.get('RAM_KB', 0) or 0) / 1024 / 1024, 2)
            except (TypeError, ValueError):
                ram_gb = 0
            try:
                disk_gb = round(int(metrics.get('DISK_KB', 0) or 0) / 1024 / 1024, 2)
            except (TypeError, ValueError):
                disk_gb = 0
            try:
                cpu = int(metrics.get('CPU', 0) or 0)
            except (TypeError, ValueError):
                cpu = 0
            if ram_gb and ram_gb > 0:
                batch_size = min(500_000, max(25_000, int(ram_gb * 20_000)))
            else:
                batch_size = 100_000
            with self.lock:
                try:
                    if os.path.exists(FLEET_CREDS_FILE):
                        with open(FLEET_CREDS_FILE, 'r') as f:
                            disk_creds = json.load(f)
                        if not isinstance(disk_creds, dict):
                            disk_creds = {}
                    else:
                        disk_creds = {}
                except Exception:
                    disk_creds = {}
                existing = disk_creds.get(ip) or {}
                if existing.get('ram_gb'):
                    # Another writer beat us to it; keep their values.
                    return
                merged = {
                    'user': existing.get('user') or self.config.get('remote_user', 'root'),
                    'port': int(existing.get('port') or 22),
                    'auth_kind': existing.get('auth_kind') or 'key',
                }
                # Preserve any other fields a future writer might add.
                for k, v in existing.items():
                    if k not in merged:
                        merged[k] = v
                merged['cpu'] = cpu
                merged['ram_gb'] = ram_gb
                merged['disk_gb'] = disk_gb
                merged['batch_size'] = batch_size
                disk_creds[ip] = merged
                try:
                    with open(FLEET_CREDS_FILE, 'w') as f:
                        json.dump(disk_creds, f, indent=2)
                    self._fleet_creds = disk_creds
                    try:
                        self._fleet_creds_mtime = os.path.getmtime(FLEET_CREDS_FILE)
                    except Exception:
                        pass
                except Exception:
                    pass
        except Exception:
            pass

    # Paramiko 3.x disables `ssh-rsa` host-key signing and several legacy
    # KEX/MAC algorithms by default, which is correct for modern servers
    # but produces a bare "Channel closed" mid-handshake against vintage
    # sshd (OpenSSH ≤ 6.x on Debian wheezy etc.). Passing an EMPTY
    # disabled-algorithms map opts the connection back into the full set
    # so legacy boxes negotiate normally. Modern servers still prefer
    # their stronger algorithms — this is purely additive compatibility,
    # not a security downgrade for healthy fleets.
    _LEGACY_FRIENDLY_DISABLED: Dict[str, list] = {}

    def _ssh_connect(self, ssh: paramiko.SSHClient, **kwargs) -> None:
        """Wrap paramiko.connect with two retries and progressive crypto
        relaxation. First try with modern defaults; on `SSHException`
        (typically the "Channel closed" handshake failure against
        ancient sshd) retry with `disabled_algorithms={}` so paramiko
        keeps every algorithm on the table. The retry is paramiko-only —
        the connection params (host, user, password/pkey) are unchanged."""
        kwargs.setdefault('banner_timeout', 10)
        kwargs.setdefault('auth_timeout', 10)
        kwargs.setdefault('allow_agent', False)
        kwargs.setdefault('look_for_keys', False)
        try:
            ssh.connect(**kwargs)
            return
        except paramiko.SSHException as e:
            msg = str(e).lower()
            # Only retry on the legacy-sshd symptoms — auth-fail / wrong-
            # password should bubble up immediately so the caller can flip
            # to the password fallback path.
            if 'channel closed' not in msg and 'no matching' not in msg \
                    and 'unable to negotiate' not in msg:
                raise
        # Second attempt — same params, looser crypto.
        kwargs['disabled_algorithms'] = self._LEGACY_FRIENDLY_DISABLED
        ssh.connect(**kwargs)

    def _get_ssh_client(self, ip: str, timeout: int = None) -> paramiko.SSHClient:
        timeout = timeout or self.config.get("ssh_timeout", 10)
        ssh = paramiko.SSHClient()
        ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

        username = self._user_for(ip)
        port = self._port_for(ip)
        ssh_key_path = self.config.get("ssh_key_path")
        pkey = self._load_private_key(ssh_key_path)

        # Resolve persisted creds for this ip — we may need the password
        # as primary auth (auth_kind == 'password') or as a fallback when
        # key auth fails because authorized_keys got wiped.
        self._reload_fleet_creds_if_changed()
        creds = self._fleet_creds.get(ip, {}) or {}
        password = creds.get('password')
        auth_kind = creds.get('auth_kind', 'key')

        # Password-primary path: this worker was added with auth_kind=password
        # and we have the secret on file. Try password first, then queue a
        # daemon thread to (re)install the controller's pubkey so the next
        # probe upgrades itself to key auth.
        if auth_kind == 'password' and password:
            self._ssh_connect(
                ssh,
                hostname=ip,
                port=port,
                username=username,
                password=password,
                timeout=timeout,
            )
            threading.Thread(target=self._repair_keys_on_worker,
                             args=(ip, ssh), daemon=True).start()
            return ssh

        # Key-auth path (default). If it fails with AuthenticationException
        # and we have a stored password, fall back to password auth and
        # repair the keys on the worker so we don't burn the password every
        # probe.
        try:
            if pkey:
                self._ssh_connect(
                    ssh,
                    hostname=ip,
                    port=port,
                    username=username,
                    pkey=pkey,
                    timeout=timeout,
                )
            else:
                # Fallback to key_filename if _load_private_key fails
                self._ssh_connect(
                    ssh,
                    hostname=ip,
                    port=port,
                    username=username,
                    key_filename=ssh_key_path,
                    timeout=timeout,
                )
        except paramiko.AuthenticationException:
            if not password:
                raise
            # authorized_keys probably got nuked (cloud-init, fail2ban,
            # rebuild). Reconnect with the stored password, then heal.
            try: ssh.close()
            except Exception: pass
            ssh = paramiko.SSHClient()
            ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            self._ssh_connect(
                ssh,
                hostname=ip,
                port=port,
                username=username,
                password=password,
                timeout=timeout,
            )
            threading.Thread(target=self._repair_keys_on_worker,
                             args=(ip, ssh), daemon=True).start()
        return ssh

    def _repair_keys_on_worker(self, ip: str, ssh) -> None:
        """Re-install the controller's pubkey on a worker we just connected
        to via password. Idempotent — grep -qxF guards against duplicates."""
        try:
            pub_path = os.path.expanduser(self.config.get('ssh_key_path', '') + '.pub') if self.config.get('ssh_key_path') else '/opt/reconx/.ssh/id_ed25519.pub'
            if not os.path.exists(pub_path):
                return
            with open(pub_path, 'r') as f:
                pubkey = f.read().strip()
            if not pubkey:
                return
            install_cmd = (
                'umask 077 && mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh" && '
                'touch "$HOME/.ssh/authorized_keys" && chmod 600 "$HOME/.ssh/authorized_keys" && '
                'KEY=$(cat) && '
                'grep -qxF "$KEY" "$HOME/.ssh/authorized_keys" || echo "$KEY" >> "$HOME/.ssh/authorized_keys"'
            )
            stdin, stdout, _ = ssh.exec_command(install_cmd, timeout=10)
            stdin.write(pubkey + '\n')
            stdin.channel.shutdown_write()
            stdout.channel.recv_exit_status()
        except Exception:
            pass
    
    def _get_pooled_ssh(self, ip: str, timeout: int) -> paramiko.SSHClient:
        """Return a cached SSHClient for ip, opening one if absent or stale.

        Sets a TCP keepalive on new connections so middleboxes/NAT don't
        silently drop the session between probes."""
        with self._pool_lock:
            ssh = self._ssh_pool.get(ip)
            if ssh is not None:
                tr = ssh.get_transport()
                if tr is not None and tr.is_active():
                    return ssh
                try: ssh.close()
                except Exception: pass
                self._ssh_pool.pop(ip, None)

        new_ssh = self._get_ssh_client(ip, timeout)
        try:
            tr = new_ssh.get_transport()
            if tr is not None:
                # Adaptive keepalive: if this worker has been missing probes
                # we shorten the heartbeat to 15s so NAT/middlebox idle
                # timeouts can't shave another cycle off recovery.
                keepalive_base = int(self.config.get("ssh_keepalive", 30))
                tighter = self._consecutive_misses.get(ip, 0) > 0
                tr.set_keepalive(15 if tighter else keepalive_base)
        except Exception:
            pass

        # Warm the channel — a `true` exec primes the SSH transport so the
        # next ssh_exec on this freshly-opened client doesn't pay an extra
        # round-trip's worth of channel setup latency. Failures here are
        # silent; the caller will discover any real problem on its own exec.
        try:
            _, _stdout, _ = new_ssh.exec_command('true', timeout=3)
            _stdout.channel.recv_exit_status()
        except Exception:
            pass

        with self._pool_lock:
            # Lose any race against another thread that also opened a conn.
            existing = self._ssh_pool.get(ip)
            if existing is not None:
                etr = existing.get_transport()
                if etr is not None and etr.is_active():
                    try: new_ssh.close()
                    except Exception: pass
                    return existing
            self._ssh_pool[ip] = new_ssh
        return new_ssh

    def _evict_pooled_ssh(self, ip: str) -> None:
        with self._pool_lock:
            ssh = self._ssh_pool.pop(ip, None)
        if ssh is not None:
            try: ssh.close()
            except Exception: pass

    def _ssh_exec_via_shell(self, ssh: paramiko.SSHClient, cmd: str,
                            timeout: int) -> str:
        """Run `cmd` inside an interactive shell channel. Used when the
        server refuses `exec` channel requests (some hardened sshd
        configs, ForceCommand, restricted-environment chroots). We wrap
        the command in sentinel markers so the output we read back can
        be reliably trimmed of MOTDs, prompt strings, and ANSI escapes."""
        import time, re
        chan = ssh.get_transport().open_session()
        chan.get_pty('xterm-256color', 200, 50)
        chan.invoke_shell()
        chan.settimeout(timeout)
        # Drain whatever the shell emits on attach (motd, prompt, etc).
        time.sleep(0.4)
        try:
            while chan.recv_ready():
                chan.recv(65536)
        except Exception:
            pass
        # Sentinel-wrapped command. `unset PROMPT_COMMAND PS1` calms down
        # interactive shells that would otherwise smear escape codes into
        # the output. The trailing `exit` ensures the channel closes on
        # its own so we have a clean read termination.
        sentinel = '___RCNX_OUT_END___'
        wrapped = (
            f"unset PROMPT_COMMAND PS1 2>/dev/null; "
            f"{cmd}; echo '{sentinel}'\n"
        )
        chan.send(wrapped)
        # Read until we see the sentinel or the timeout fires.
        deadline = time.monotonic() + timeout
        buf = ''
        while time.monotonic() < deadline:
            if chan.recv_ready():
                chunk = chan.recv(65536).decode('utf-8', errors='replace')
                buf += chunk
                if sentinel in buf:
                    break
            elif chan.closed:
                break
            else:
                time.sleep(0.08)
        try: chan.send('exit\n')
        except Exception: pass
        try: chan.close()
        except Exception: pass
        # Strip everything before the sentinel marker on its own line.
        # The exec'd command's output sits between the echoed command and
        # the sentinel — we keep that slice and drop the rest.
        ansi = re.compile(r'\x1B\[[0-?]*[ -/]*[@-~]|\x1B\][^\x07]*\x07')
        cleaned = ansi.sub('', buf)
        # Find the sentinel and snip up to it; before that, drop the echo
        # of the wrapped command itself by removing the first line that
        # contains '{sentinel}\''.
        idx = cleaned.find(sentinel)
        if idx < 0:
            return cleaned.strip()
        head = cleaned[:idx]
        # Discard the echoed command line (the one containing the
        # literal `echo '___RCNX_OUT_END___'` we just sent).
        lines = head.splitlines()
        out_lines: list = []
        seen_cmd = False
        for ln in lines:
            if not seen_cmd and sentinel in ln:
                seen_cmd = True; continue
            out_lines.append(ln)
        return '\n'.join(out_lines).strip()

    def ssh_exec(self, ip: str, cmd: str, timeout: int = None) -> str:
        timeout = timeout or self.config.get("ssh_timeout", 10)
        try:
            ssh = self._get_pooled_ssh(ip, timeout)
            # Fast-path for hosts known to refuse exec channels.
            if ip in self._exec_blocked_ips:
                try:
                    out = self._ssh_exec_via_shell(ssh, cmd, timeout)
                    self._last_ssh_error.pop(ip, None)
                    return out
                except (paramiko.SSHException, EOFError, OSError):
                    self._evict_pooled_ssh(ip)
                    ssh = self._get_pooled_ssh(ip, timeout)
                    out = self._ssh_exec_via_shell(ssh, cmd, timeout)
                    self._last_ssh_error.pop(ip, None)
                    return out
            try:
                stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
                out = stdout.read().decode('utf-8', errors='replace').strip()
                # Successful round-trip clears any prior remembered error so
                # the card doesn't keep showing yesterday's auth failure
                # after the operator fixed the creds.
                self._last_ssh_error.pop(ip, None)
                return out
            except (paramiko.SSHException, EOFError, OSError) as e:
                msg = str(e).lower()
                # If the server explicitly refuses exec — common on hardened
                # sshd configs that only allow interactive shells — flip
                # this IP to invoke_shell mode permanently and retry.
                if 'channel closed' in msg or 'administratively prohibited' in msg:
                    self._evict_pooled_ssh(ip)
                    ssh = self._get_pooled_ssh(ip, timeout)
                    try:
                        out = self._ssh_exec_via_shell(ssh, cmd, timeout)
                        self._exec_blocked_ips.add(ip)
                        self._last_ssh_error.pop(ip, None)
                        return out
                    except Exception:
                        pass  # fall through to the generic retry below
                # Connection died mid-exec — evict and retry once with a fresh one.
                self._evict_pooled_ssh(ip)
                ssh = self._get_pooled_ssh(ip, timeout)
                stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
                out = stdout.read().decode('utf-8', errors='replace').strip()
                self._last_ssh_error.pop(ip, None)
                return out
        except Exception as e:
            # Remember the real cause so fetch_server_status can show
            # "ssh: AuthenticationException — Permission denied" instead of
            # the useless "ssh probe returned no data" string the operator
            # was staring at. Keep it short — the card has limited width.
            kind = type(e).__name__
            msg = str(e)
            if len(msg) > 160:
                msg = msg[:160] + '…'
            self._last_ssh_error[ip] = f'{kind}: {msg}' if msg else kind
            self._evict_pooled_ssh(ip)
            return ""
    
    def scp_upload(self, ip: str, local_path: str, remote_path: str) -> bool:
        try:
            ssh = self._get_ssh_client(ip, 30)
            sftp = ssh.open_sftp()
            # Apply a per-operation socket timeout so a hung SFTP channel
            # fails in bounded time instead of blocking the dispatch thread
            # forever. 120 s is generous for files up to a few hundred KB.
            sftp.get_channel().settimeout(120)
            sftp.put(local_path, remote_path)
            sftp.close()
            ssh.close()
            self._last_ssh_error.pop(ip, None)
            return True
        except Exception as e:
            # Stash the paramiko/socket/SFTP error so callers can read the
            # real cause via last_error(ip) instead of staring at a bare
            # "returned False". Keep it short — UI rendering is width-bound.
            kind = type(e).__name__
            msg = str(e)
            if len(msg) > 160:
                msg = msg[:160] + '…'
            self._last_ssh_error[ip] = f'scp: {kind}: {msg}' if msg else f'scp: {kind}'
            return False

    def scp_download(self, ip: str, remote_path: str, local_path: str) -> bool:
        try:
            ssh = self._get_ssh_client(ip, 30)
            sftp = ssh.open_sftp()
            sftp.get(remote_path, local_path)
            sftp.close()
            ssh.close()
            self._last_ssh_error.pop(ip, None)
            return True
        except Exception as e:
            kind = type(e).__name__
            msg = str(e)
            if len(msg) > 160:
                msg = msg[:160] + '…'
            self._last_ssh_error[ip] = f'scp: {kind}: {msg}' if msg else f'scp: {kind}'
            return False

    def last_error(self, ip: str) -> str:
        """Most recent SSH/SFTP error captured for this host (empty if the
        last operation succeeded). Public accessor — callers should use
        this instead of touching _last_ssh_error directly."""
        return self._last_ssh_error.get(ip, '') or ''
    
    # ==================== CONNECTION TESTING ====================
    
    def quick_ssh_test(self) -> dict:
        """Quick test if SSH key exists and is valid format"""
        ssh_key = self.config.get("ssh_key_path", "/root/ssh/1")
        
        # Check if key file exists
        if not os.path.exists(ssh_key):
            return {"success": False, "error": f"SSH key not found: {ssh_key}"}
        
        # Check file permissions
        try:
            import stat
            mode = os.stat(ssh_key).st_mode
            if mode & stat.S_IRWXG or mode & stat.S_IRWXO:
                return {"success": False, "error": "SSH key has too open permissions. Run: chmod 600 " + ssh_key}
        except:
            pass
        
        # Try to load the key
        try:
            key = self._load_private_key(ssh_key)
            
            if key is None:
                return {"success": False, "error": "Cannot parse SSH key - invalid format. Install cryptography: pip install cryptography"}
            
            # Determine key type
            key_type = type(key).__name__.replace("Key", "")
            
            return {"success": True, "key_path": ssh_key, "key_type": key_type}
        except Exception as e:
            return {"success": False, "error": f"Cannot load SSH key: {str(e)}"}
    
    def test_single_connection(self, ip: str, timeout: int = 5) -> dict:
        """Test SSH connection to a single server with short timeout"""
        try:
            ssh = paramiko.SSHClient()
            ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            
            ssh_key_path = self.config.get("ssh_key_path")
            pkey = self._load_private_key(ssh_key_path)
            
            if pkey:
                ssh.connect(
                    ip,
                    username=self.config.get("remote_user", "root"),
                    pkey=pkey,
                    timeout=timeout,
                    banner_timeout=timeout,
                    auth_timeout=timeout,
                    look_for_keys=False,
                    allow_agent=False
                )
            else:
                ssh.connect(
                    ip,
                    username=self.config.get("remote_user", "root"),
                    key_filename=ssh_key_path,
                    timeout=timeout,
                    banner_timeout=timeout,
                    auth_timeout=timeout,
                    look_for_keys=False,
                    allow_agent=False
                )
            
            # Quick command to verify
            stdin, stdout, stderr = ssh.exec_command("hostname", timeout=timeout)
            hostname = stdout.read().decode().strip()
            ssh.close()
            
            return {"success": True, "ip": ip, "hostname": hostname}
        except paramiko.AuthenticationException:
            return {"success": False, "ip": ip, "error": "Authentication failed - check SSH key"}
        except paramiko.SSHException as e:
            return {"success": False, "ip": ip, "error": f"SSH error: {str(e)[:30]}"}
        except Exception as e:
            error = str(e)
            if "timed out" in error.lower():
                return {"success": False, "ip": ip, "error": "Connection timed out"}
            elif "refused" in error.lower():
                return {"success": False, "ip": ip, "error": "Connection refused"}
            elif "no route" in error.lower():
                return {"success": False, "ip": ip, "error": "No route to host"}
            return {"success": False, "ip": ip, "error": str(e)[:40]}
    
    def test_connection(self, ip: str) -> dict:
        try:
            result = self.ssh_exec(ip, "echo OK && hostname && uptime -p 2>/dev/null || uptime", 5)
            if "OK" in result:
                lines = result.split('\n')
                return {
                    "success": True, "ip": ip,
                    "hostname": lines[1] if len(lines) > 1 else "unknown",
                    "uptime": lines[2] if len(lines) > 2 else "unknown"
                }
            return {"success": False, "ip": ip, "error": "Connection failed"}
        except Exception as e:
            return {"success": False, "ip": ip, "error": str(e)}
    
    def test_all_connections(self) -> dict:
        ips = self.load_servers()
        results = {"working": [], "failed": [], "details": []}
        
        def test_one(ip):
            result = self.test_connection(ip)
            with self.lock:
                if result["success"]:
                    results["working"].append(ip)
                else:
                    results["failed"].append(ip)
                results["details"].append(result)
        
        threads = []
        for ip in ips:
            t = threading.Thread(target=test_one, args=(ip,))
            t.start()
            threads.append(t)
        
        for t in threads:
            t.join(timeout=15)
        
        return results
    
    # ==================== DEPLOYMENT ORCHESTRATION ====================
    
    def prepare_deployment(self, target_file: str = None) -> dict:
        """Prepare deployment: test connections, count targets, calculate split"""
        target_file = target_file or self.config.get("target_file", "targets.txt")
        
        if not os.path.exists(target_file):
            return {"success": False, "error": f"Target file not found: {target_file}"}
        
        with open(target_file, 'r') as f:
            targets = [line.strip() for line in f if line.strip()]
        total_targets = len(targets)
        
        if total_targets == 0:
            return {"success": False, "error": "Target file is empty"}
        
        conn_results = self.test_all_connections()
        working_servers = conn_results["working"]
        
        if len(working_servers) == 0:
            return {"success": False, "error": "No servers are reachable"}
        
        per_server = total_targets // len(working_servers)
        remainder = total_targets % len(working_servers)
        
        return {
            "success": True,
            "total_targets": total_targets,
            "working_servers": len(working_servers),
            "failed_servers": len(conn_results["failed"]),
            "per_server": per_server,
            "remainder": remainder,
            "working_ips": working_servers,
            "failed_ips": conn_results["failed"],
            "connection_details": conn_results["details"]
        }
    
    def bootstrap_worker(self, ip: str) -> dict:
        """Minimal single-worker bootstrap: SSH-test, create work_dir, SFTP
        scanner (main.py) and the reconx-scanner binary, chmod +x. NO
        targets.txt, NO batch split, NO start. This is what auto-deploy
        on key-install needs on a fresh install where no target list
        exists yet — see Effect C in the deployment plan.

        Returns {"success": bool, "message": str, "steps": list[str]}.
        Uses scp_upload / ssh_exec which already route through
        _get_ssh_client → _user_for / _port_for, honouring _fleet_creds
        for per-IP user + port."""
        steps: List[str] = []
        scanner_file = self.config.get("scanner_file", "main.py")
        work_dir = self.config.get("work_dir", "/root/python_job")
        result_subdir = self.config.get("result_subdir", "ResultJS")

        # 1. Verify SSH actually works before touching anything else.
        conn = self.test_single_connection(ip)
        if not conn.get("success"):
            return {
                "success": False,
                "message": f"SSH test failed: {conn.get('error', 'unknown error')}",
                "steps": steps,
            }
        steps.append("SSH connection verified")

        # 2. Ensure remote work_dir + result_subdir exist.
        try:
            self.ssh_exec(ip, f"mkdir -p {work_dir}/{result_subdir}", 10)
            steps.append(f"Created {work_dir}/{result_subdir}")
        except Exception as e:
            return {"success": False, "message": f"mkdir failed: {e}", "steps": steps}

        # 3. SFTP main.py. Fall back to common alt names if config'd
        # scanner_file is missing locally — same fallback pattern
        # deploy_full uses.
        local_scanner = scanner_file
        if not os.path.exists(local_scanner):
            for candidate in ('main.py', 'scanner.py', 'gemini.py'):
                if os.path.exists(candidate):
                    local_scanner = candidate
                    break
            else:
                return {"success": False, "message": "Scanner file not found locally", "steps": steps}
        if not self.scp_upload(ip, local_scanner, f"{work_dir}/main.py"):
            return {"success": False, "message": "Failed to upload scanner (main.py)", "steps": steps}
        steps.append("Uploaded main.py")

        # 4. SFTP reconx-scanner binary, then chmod +x. Missing binary
        # is a hard failure here — bootstrap means the worker has the
        # binary on disk.
        if not os.path.exists('reconx-scanner'):
            return {"success": False, "message": "reconx-scanner binary not found locally", "steps": steps}
        # Upload to .new then atomically rename to avoid ETXTBSY if a
        # session is concurrently running the old binary.
        tmp_remote = f"{work_dir}/reconx-scanner.new"
        if not self.scp_upload(ip, 'reconx-scanner', tmp_remote):
            return {"success": False, "message": "Failed to upload reconx-scanner binary", "steps": steps}
        steps.append("Uploaded reconx-scanner")
        try:
            self.ssh_exec(ip, f"chmod +x {tmp_remote} && mv -f {tmp_remote} {work_dir}/reconx-scanner", 5)
            steps.append("chmod +x reconx-scanner")
        except Exception as e:
            return {"success": False, "message": f"chmod/rename failed: {e}", "steps": steps}

        return {"success": True, "message": "Bootstrap complete", "steps": steps}

    def deploy_full(self, target_file: str = None, scanner_file: str = None,
                    runner_file: str = None, auto_start: bool = False,
                    progress_callback: Callable = None,
                    single_ip: str = None) -> dict:
        """
        Full deployment workflow:
        1. Test connections
        2. Split targets equally
        3. Upload files to each server
        4. Install requirements
        5. Create batches
        6. Optionally start
        """
        self.deployment_log = []
        
        def log(msg):
            self.deployment_log.append(f"[{datetime.now().strftime('%H:%M:%S')}] {msg}")
            if progress_callback:
                progress_callback({"type": "log", "message": msg})
        
        target_file = target_file or self.config.get("target_file", "targets.txt")
        scanner_file = scanner_file or self.config.get("scanner_file", "main.py")
        runner_file = runner_file or self.config.get("runner_file", "batch_runner.sh")
        work_dir = self.config.get("work_dir", "/root/python_job")
        batch_size = self.config.get("batch_size", 100000)
        
        results = {
            "success": False, "total_servers": 0, "deployed": 0, "failed": 0,
            "total_targets": 0, "servers": [], "log": self.deployment_log
        }
        
        # Validate files
        log("Checking local files...")
        
        if not os.path.exists(target_file):
            log(f"❌ Target file not found: {target_file}")
            return results
        
        if not os.path.exists(scanner_file):
            for candidate in ['main.py', 'scanner.py', 'gemini.py']:
                if os.path.exists(candidate):
                    scanner_file = candidate
                    break
            else:
                log("❌ Scanner file not found")
                return results
        
        log(f"✓ Scanner: {scanner_file}")
        
        has_runner = os.path.exists(runner_file)
        if has_runner:
            log(f"✓ Runner: {runner_file}")
        
        # Load targets
        log("Loading targets...")
        with open(target_file, 'r') as f:
            all_targets = [line.strip() for line in f if line.strip()]
        
        total_targets = len(all_targets)
        results["total_targets"] = total_targets
        log(f"✓ Total targets: {total_targets:,}")
        
        # Test connections
        log("Testing server connections...")
        conn_results = self.test_all_connections()
        working_servers = conn_results["working"]
        
        if len(working_servers) == 0:
            log("❌ No servers reachable!")
            return results
        
        log(f"✓ Working servers: {len(working_servers)}/{len(self.load_servers())}")
        
        if conn_results["failed"]:
            log(f"⚠ Failed servers: {', '.join(conn_results['failed'])}")
        
        results["total_servers"] = len(working_servers)
        
        # Calculate split
        per_server = total_targets // len(working_servers)
        log(f"Targets per server: ~{per_server:,}")
        
        if progress_callback:
            progress_callback({"type": "status", "phase": "deploying", "total": len(working_servers), "completed": 0})
        
        # Deploy to each server
        for idx, ip in enumerate(working_servers):
            # Effect C seam: when invoked for a single worker (auto-deploy on
            # key-install), skip every IP that isn't the target. Iterating
            # the full list keeps target-splitting logic unchanged.
            if single_ip and ip != single_ip:
                continue
            # Per-worker batch size baked in by the first probe. Falls back
            # to the global config default if specs haven't been captured
            # yet (worker added but never probed).
            per_ip_batch = (self._fleet_creds.get(ip, {}) or {}).get('batch_size')
            batch_size = int(per_ip_batch) if per_ip_batch else int(self.config.get('batch_size', 100000))
            server_result = DeploymentResult(ip=ip, success=False)
            log(f"\n▶ Deploying to {ip} ({idx+1}/{len(working_servers)})...")

            try:
                # 1. Create work directory
                log(f"  Creating {work_dir}...")
                self.ssh_exec(ip, f"mkdir -p {work_dir}/{self.config.get('result_subdir', 'ResultJS')}", 10)
                server_result.steps.append("Created directory")
                
                # 2. Upload scanner (Python wrapper)
                log("  Uploading scanner...")
                if self.scp_upload(ip, scanner_file, f"{work_dir}/main.py"):
                    server_result.steps.append("Uploaded scanner")
                else:
                    raise Exception("Failed to upload scanner")
                
                # 2.5. Upload reconx-scanner binary (Go binary)
                if os.path.exists('reconx-scanner'):
                    log("  Uploading reconx-scanner binary...")
                    if self.scp_upload(ip, 'reconx-scanner', f"{work_dir}/reconx-scanner"):
                        self.ssh_exec(ip, f"chmod +x {work_dir}/reconx-scanner", 5)
                        server_result.steps.append("Uploaded reconx-scanner binary")
                    else:
                        log("  ⚠ Failed to upload binary, trying package...")
                
                # 2.6. Upload package if exists (fallback)
                if os.path.exists('scanner_package.tar.gz'):
                    log("  Uploading scanner package...")
                    if self.scp_upload(ip, 'scanner_package.tar.gz', f"{work_dir}/scanner_package.tar.gz"):
                        # Extract package
                        extract_cmd = f"cd {work_dir} && tar -xzf scanner_package.tar.gz && chmod +x reconx-scanner 2>/dev/null || true"
                        self.ssh_exec(ip, extract_cmd, 10)
                        server_result.steps.append("Uploaded and extracted scanner package")
                
                # 3. Upload runner if exists
                if has_runner:
                    log("  Uploading runner...")
                    if self.scp_upload(ip, runner_file, f"{work_dir}/batch_runner.sh"):
                        self.ssh_exec(ip, f"chmod +x {work_dir}/batch_runner.sh", 5)
                        server_result.steps.append("Uploaded runner")
                
                # 4. Upload requirements if exists
                if os.path.exists("requirements.txt"):
                    log("  Uploading requirements.txt...")
                    self.scp_upload(ip, "requirements.txt", f"{work_dir}/requirements.txt")
                
                # 5. Split and upload targets
                start_idx = idx * per_server
                end_idx = start_idx + per_server if idx < len(working_servers) - 1 else total_targets
                server_targets = all_targets[start_idx:end_idx]
                server_result.targets_assigned = len(server_targets)
                
                log(f"  Uploading {len(server_targets):,} targets...")
                
                with tempfile.NamedTemporaryFile(mode='w', suffix='.txt', delete=False) as tmp:
                    tmp.write('\n'.join(server_targets))
                    tmp_path = tmp.name
                
                if self.scp_upload(ip, tmp_path, f"{work_dir}/server_main_list.txt"):
                    server_result.steps.append(f"Uploaded {len(server_targets):,} targets")
                    os.unlink(tmp_path)
                else:
                    os.unlink(tmp_path)
                    raise Exception("Failed to upload targets")
                
                # 6. Install dependencies
                log("  Installing dependencies...")
                install_cmd = f"""
                    cd {work_dir}
                    apt-get update -qq 2>/dev/null || true
                    apt-get install -y -qq python3-pip redis-server 2>/dev/null || true
                    systemctl start redis-server 2>/dev/null || redis-server --daemonize yes 2>/dev/null || true
                    pip3 install urllib3 requests httpx tqdm colorama redis aiohttp fake-useragent dnspython --break-system-packages -q 2>/dev/null || true
                    [ -f requirements.txt ] && pip3 install -r requirements.txt --break-system-packages -q 2>/dev/null || true
                """
                self.ssh_exec(ip, install_cmd, 120)
                server_result.steps.append("Installed dependencies")
                
                # 7. Create config and batches
                log(f"  Creating batches (size: {batch_size:,})...")
                config_cmd = f"""
                    cd {work_dir}
                    echo '#!/bin/bash
BATCH_SIZE={batch_size}
MAIN_LIST=server_main_list.txt' > server_config.sh
                    chmod +x server_config.sh
                    rm -f batch_*.txt batch_*.txt.done batch_*.txt.failed 2>/dev/null
                    split -l {batch_size} -d --additional-suffix=.txt server_main_list.txt batch_
                    ls batch_*.txt 2>/dev/null | wc -l
                """
                batch_count = self.ssh_exec(ip, config_cmd, 30)
                server_result.steps.append(f"Created {batch_count} batches")
                
                # 8. Clear old data
                log("  Clearing old data...")
                self.ssh_exec(ip, f"rm -f {work_dir}/output.log {work_dir}/{self.config.get('dedup_file', 'dedup_log.txt')}", 5)
                self.ssh_exec(ip, "redis-cli FLUSHALL 2>/dev/null || true", 5)
                server_result.steps.append("Cleared old data")
                
                server_result.success = True
                server_result.message = f"Deployed {len(server_targets):,} targets in {batch_count} batches"
                results["deployed"] += 1
                log(f"  ✓ Success: {len(server_targets):,} targets")
                
            except Exception as e:
                server_result.success = False
                server_result.message = str(e)
                results["failed"] += 1
                log(f"  ❌ Failed: {str(e)}")
            
            results["servers"].append(asdict(server_result))
            
            if progress_callback:
                progress_callback({
                    "type": "status", "phase": "deploying",
                    "total": len(working_servers), "completed": idx + 1,
                    "current_server": ip, "server_result": asdict(server_result)
                })
        
        # Auto-start if requested
        if auto_start and results["deployed"] > 0:
            log("\nStarting all servers...")
            start_result = self.start_all()
            log(f"Started: {start_result['success']}, Failed: {start_result['failed']}")
        
        results["success"] = results["deployed"] > 0
        results["log"] = self.deployment_log
        
        log(f"\n{'='*50}")
        log(f"Deployment complete: {results['deployed']}/{results['total_servers']} servers")
        
        return results
    
    # ==================== SERVER STATUS ====================
    
    def _build_probe_script(self, work_dir: str, result_dir: str, dedup_file: str) -> str:
        """One bash script that collects every metric fetch_server_status
        used to gather across 9–15 separate ssh_exec calls. Running it as
        a single exec_command means one channel, one round-trip, and one
        chance to fail — instead of nine — which dramatically cuts the
        odds of any given probe blipping the worker into RECONNECT."""
        return f"""set +e
cd "{work_dir}" 2>/dev/null
MAIN_PID=$(pgrep -f 'python.*main.py' 2>/dev/null | head -1)
BATCH_PID=$(pgrep -f 'batch_runner.sh' 2>/dev/null | head -1)
PID="${{MAIN_PID:-$BATCH_PID}}"
UPTIME=""
if [ -n "$PID" ]; then UPTIME=$(ps -o etime= -p "$PID" 2>/dev/null | tr -d ' '); fi
COMPLETE=$(grep -c 'ALL COMPLETE' output.log 2>/dev/null || echo 0)
PENDING=$(ls batch_*.txt 2>/dev/null | wc -l)
DONE=$(ls batch_*.txt.done 2>/dev/null | wc -l)
FAILED=$(ls batch_*.txt.failed 2>/dev/null | wc -l)
TARGETS=$(wc -l < server_main_list.txt 2>/dev/null || echo 0)
SCANNED=0
KEYS=$(redis-cli KEYS '*hash*' 2>/dev/null | head -10)
for k in $KEYS; do
  c=$(redis-cli SCARD "$k" 2>/dev/null)
  if [ -n "$c" ] && [ "$c" -gt "$SCANNED" ] 2>/dev/null; then SCANNED=$c; fi
done
if [ "$SCANNED" -eq 0 ]; then
  for p in 'gemini:dedup:hashes' 'gemini:hashes' 'dedup:hashes'; do
    c=$(redis-cli SCARD "$p" 2>/dev/null)
    if [ -n "$c" ] && [ "$c" -gt 0 ] 2>/dev/null; then SCANNED=$c; break; fi
  done
fi
if [ "$SCANNED" -eq 0 ]; then
  c=$(wc -l < "{dedup_file}" 2>/dev/null | tr -d ' ')
  if [ -n "$c" ] && [ "$c" -gt 0 ] 2>/dev/null; then SCANNED=$c; fi
fi
CPU=$(nproc 2>/dev/null || echo 0)
RAM_KB=$(grep MemTotal /proc/meminfo 2>/dev/null | awk '{{print $2}}')
DISK_KB=$(df -Pk / 2>/dev/null | awk 'NR==2{{print $2}}')
# Live metrics — added so the dashboard cards show real CPU%, RAM used,
# disk used, and system uptime instead of zeros. All fast (no second
# sampling window): we read /proc once.
RAM_AVAIL_KB=$(grep MemAvailable /proc/meminfo 2>/dev/null | awk '{{print $2}}')
RAM_USED_KB=0
if [ -n "$RAM_KB" ] && [ -n "$RAM_AVAIL_KB" ]; then
  RAM_USED_KB=$((RAM_KB - RAM_AVAIL_KB))
fi
DISK_USED_KB=$(df -Pk / 2>/dev/null | awk 'NR==2{{print $3}}')
# CPU% via /proc/stat one-shot: read once, sleep, read again, delta.
# 200 ms sample keeps the probe under its 15 s budget.
read -r _ u1 n1 s1 i1 _ < /proc/stat 2>/dev/null
sleep 0.2
read -r _ u2 n2 s2 i2 _ < /proc/stat 2>/dev/null
CPU_PCT=0
if [ -n "$u1" ] && [ -n "$u2" ]; then
  busy=$((u2 + n2 + s2 - u1 - n1 - s1))
  total=$((busy + i2 - i1))
  if [ "$total" -gt 0 ] 2>/dev/null; then CPU_PCT=$(( 100 * busy / total )); fi
fi
SYS_UPTIME_SEC=$(awk '{{print int($1)}}' /proc/uptime 2>/dev/null)
echo "PROBE_BEGIN"
echo "MAIN_PID=$MAIN_PID"
echo "BATCH_PID=$BATCH_PID"
echo "UPTIME=$UPTIME"
echo "COMPLETE=$COMPLETE"
echo "PENDING=$PENDING"
echo "DONE=$DONE"
echo "FAILED=$FAILED"
echo "TARGETS=$TARGETS"
echo "SCANNED=$SCANNED"
echo "CPU=$CPU"
echo "RAM_KB=$RAM_KB"
echo "DISK_KB=$DISK_KB"
echo "CPU_PCT=$CPU_PCT"
echo "RAM_USED_KB=$RAM_USED_KB"
echo "DISK_USED_KB=$DISK_USED_KB"
echo "SYS_UPTIME_SEC=$SYS_UPTIME_SEC"
echo "HITS_BEGIN"
cat "{result_dir}"/*.txt 2>/dev/null | head -100
echo "HITS_END"
echo "PROBE_END"
"""

    @staticmethod
    def _parse_probe_output(raw: str) -> dict:
        """Pull the key=value lines and the HITS_BEGIN…HITS_END block out
        of the combined probe script's stdout. Returns {} if the sentinels
        aren't present (which means the exec didn't actually run)."""
        if 'PROBE_BEGIN' not in raw or 'PROBE_END' not in raw:
            return {}
        metrics: dict = {}
        hits_lines: list = []
        in_hits = False
        for line in raw.split('\n'):
            if line in ('PROBE_BEGIN', 'PROBE_END'):
                continue
            if line == 'HITS_BEGIN':
                in_hits = True
                continue
            if line == 'HITS_END':
                in_hits = False
                continue
            if in_hits:
                hits_lines.append(line)
                continue
            if '=' in line:
                k, _, v = line.partition('=')
                metrics[k] = v
        metrics['HITS'] = '\n'.join(hits_lines)
        return metrics

    def fetch_server_status(self, ip: str) -> ServerStatus:
        status = ServerStatus(ip=ip)
        work_dir = self.config.get("work_dir", "/root/python_job")
        result_dir = self.config.get("result_subdir", "ResultJS")
        dedup_file = self.config.get("dedup_file", "dedup_log.txt")
        batch_size = self.config.get("batch_size", 100000)

        # One exec for the whole probe. ssh_exec uses the pooled SSHClient
        # so a healthy worker pays no per-probe handshake cost — the same
        # socket stays warm between monitor cycles via the 30s keepalive.
        probe_script = self._build_probe_script(work_dir, result_dir, dedup_file)
        raw = self.ssh_exec(ip, probe_script, timeout=15)
        metrics = self._parse_probe_output(raw)

        if not metrics:
            # Probe didn't run. Sticky-status: keep the previous snapshot
            # until we've missed _sticky_threshold cycles in a row. That
            # masks NAT timeouts, fail2ban hiccups, single dropped packets,
            # etc., so the card doesn't flap.
            with self.lock:
                misses = self._consecutive_misses.get(ip, 0) + 1
                self._consecutive_misses[ip] = misses
                if misses < self._sticky_threshold and ip in self.servers:
                    prev = self.servers[ip]
                    prev.last_update = datetime.now().strftime('%H:%M:%S')
                    return prev
                status.status = "UNREACHABLE"
                # Surface the real cause if ssh_exec remembered one (auth
                # failure, network drop, banner timeout). Operators staring
                # at "ssh probe returned no data" had no way to triage; now
                # the card shows "ssh: AuthenticationException — Permission
                # denied" or similar so they know to re-paste creds vs.
                # check the network.
                real = self._last_ssh_error.get(ip)
                # Distinguish "host is up but auth is failing" from "host is
                # down" so the operator knows whether to re-paste creds or
                # check the box itself. Quick non-SSH TCP-22 probe (~1s).
                tcp_hint = ''
                try:
                    port = self._port_for(ip) if hasattr(self, '_port_for') else 22
                    import socket as _s
                    with _s.socket(_s.AF_INET, _s.SOCK_STREAM) as sk:
                        sk.settimeout(1.5)
                        sk.connect((ip, port))
                    tcp_hint = ' (TCP up — host alive, auth path failing)'
                except Exception:
                    tcp_hint = ' (TCP down — host unreachable or firewalled)'
                status.error = (f'ssh: {real}{tcp_hint}' if real
                                else f'ssh probe returned no data{tcp_hint}')
                # Carry over the last known-good metrics so the operator can
                # still see "last known CPU was 8%, last known uptime was
                # 4h" with a stale marker, instead of a card with zeroed
                # fields. The status flips to UNREACHABLE so the colour
                # is still red — only the numbers are stale.
                prev = self.servers.get(ip)
                if prev is not None:
                    for fld in ('cpu_percent', 'ram_used_gb', 'ram_total_gb',
                                'disk_used_gb', 'disk_total_gb',
                                'sys_uptime_sec', 'uptime', 'speed',
                                'targets', 'scanned', 'hits'):
                        if hasattr(prev, fld) and hasattr(status, fld):
                            setattr(status, fld, getattr(prev, fld))
                    if getattr(prev, 'last_good_update', '') or prev.last_update:
                        status.last_good_update = (getattr(prev, 'last_good_update', '')
                                                   or prev.last_update)
                status.last_update = datetime.now().strftime('%H:%M:%S')
                self.servers[ip] = status
            return status

        # Probe came back clean — reset the miss counter for this host.
        with self.lock:
            self._consecutive_misses[ip] = 0

        # First-probe spec capture: writes CPU/RAM/DISK + computed batch_size
        # to fleet_creds.json so deploy_full sizes batches per worker. No-op
        # once the row is populated.
        self._persist_specs_if_new(ip, metrics)

        try:
            main_pid = metrics.get('MAIN_PID', '').strip()
            batch_pid = metrics.get('BATCH_PID', '').strip()
            if main_pid or batch_pid:
                status.status = "RUNNING"
                status.uptime = metrics.get('UPTIME', '').strip() or '-'
            else:
                complete_check = metrics.get('COMPLETE', '0').strip()
                if complete_check.isdigit() and int(complete_check) > 0:
                    status.status = "COMPLETED"
                else:
                    # Reachable + no active scanner = IDLE (renders healthy
                    # via the frontend's mapStatus). The legacy "STOPPED"
                    # value used to fall through to RECONNECT and made every
                    # healthy idle worker look broken.
                    status.status = "IDLE"

            def _as_int(v, default=0):
                v = (v or '').strip()
                return int(v) if v.lstrip('-').isdigit() else default

            batches_pending = _as_int(metrics.get('PENDING'))
            batches_done = _as_int(metrics.get('DONE'))
            batches_failed = _as_int(metrics.get('FAILED'))
            total_batches = batches_pending + batches_done + batches_failed
            batch_str = f"{batches_done}/{total_batches}"
            if batches_failed > 0:
                batch_str += f" ({batches_failed}!)"
            status.batch_info = batch_str
            status.batches_done = batches_done
            status.batches_total = total_batches
            status.targets = _as_int(metrics.get('TARGETS'))

            scanned_from_done = batches_done * batch_size
            current_scanned = _as_int(metrics.get('SCANNED'))
            status.scanned = scanned_from_done + current_scanned
            status.current_batch_progress = current_scanned

            prev_scanned = self.previous_scanned.get(ip, 0)
            if prev_scanned > 0 and status.scanned > prev_scanned:
                status.speed = (status.scanned - prev_scanned) / 5
            self.previous_scanned[ip] = status.scanned

            # Live machine metrics from the probe — feeds the dashboard
            # card's CPU/RAM/disk/uptime widgets. Tolerate missing or
            # malformed fields (older workers, race conditions) by falling
            # back to 0 / the static capacity captured in fleet_creds.
            def _f(key: str) -> float:
                try: return float((metrics.get(key) or '0').strip() or 0)
                except (TypeError, ValueError): return 0.0
            status.cpu_percent = _f('CPU_PCT')
            ram_total_kb = _f('RAM_KB')
            ram_used_kb = _f('RAM_USED_KB')
            disk_total_kb = _f('DISK_KB')
            disk_used_kb = _f('DISK_USED_KB')
            status.ram_total_gb = round(ram_total_kb / (1024 * 1024), 2)
            status.ram_used_gb = round(ram_used_kb / (1024 * 1024), 2)
            status.disk_total_gb = round(disk_total_kb / (1024 * 1024), 2)
            status.disk_used_gb = round(disk_used_kb / (1024 * 1024), 2)
            sys_up = _f('SYS_UPTIME_SEC')
            if sys_up > 0:
                status.sys_uptime_sec = int(sys_up)
                # System uptime trumps the per-process etime when the
                # scanner isn't running — operator wants "is the box on?"
                # not "is the scanner running?". The latter is in MAIN_PID.
                if not status.uptime or status.uptime == '-':
                    h = status.sys_uptime_sec // 3600
                    m = (status.sys_uptime_sec % 3600) // 60
                    status.uptime = f'{h}h {m:02d}m' if h else f'{m}m'
            status.last_good_update = datetime.now().strftime('%H:%M:%S')

            valid_count = 0
            for line in metrics.get('HITS', '').split('\n'):
                if ' | ' in line:
                    parts = line.split(' | ', 1)
                    if len(parts) == 2:
                        cred = parts[1].strip()
                        cred_lower = cred.lower()
                        if any(j in cred_lower for j in self.junk_patterns):
                            continue
                        if len(cred) < 15:
                            continue
                        if cred.startswith(('http', '/', 'www.')):
                            continue
                        if cred.replace('_', '').islower() and '_' in cred:
                            continue
                        valid_count += 1
            status.hits = valid_count

        except Exception as e:
            status.status = "ERROR"
            status.error = str(e)[:50]
        
        status.last_update = datetime.now().strftime('%H:%M:%S')
        
        with self.lock:
            self.servers[ip] = status
        
        return status
    
    def get_cached_status(self) -> List[dict]:
        """Return the most recent status snapshot for every server in the
        roster, without issuing a fresh SSH probe.

        Used by /api/vps/status — the monitor thread (running every
        monitor_interval seconds) keeps self.servers warm, so this is just
        dict serialisation. For workers the monitor hasn't probed yet,
        return a UNKNOWN-status stub so the row still appears in the UI."""
        ips = self.load_servers()
        with self.lock:
            results = []
            for ip in ips:
                if ip in self.servers:
                    results.append(self.servers[ip].to_dict())
                else:
                    results.append(ServerStatus(ip=ip).to_dict())
        return results

    def get_all_status(self) -> List[dict]:
        ips = self.load_servers()
        results = []

        threads = []
        for ip in ips:
            t = threading.Thread(target=self.fetch_server_status, args=(ip,))
            t.start()
            threads.append(t)

        for t in threads:
            t.join(timeout=15)
        
        with self.lock:
            for ip in ips:
                if ip in self.servers:
                    results.append(self.servers[ip].to_dict())
                else:
                    results.append(ServerStatus(ip=ip, status="TIMEOUT").to_dict())
        
        return results
    
    def get_global_stats(self) -> dict:
        with self.lock:
            total_servers = len(self.servers)
            running = sum(1 for s in self.servers.values() if s.status == "RUNNING")
            stopped = sum(1 for s in self.servers.values() if s.status in ("STOPPED", "IDLE"))
            completed = sum(1 for s in self.servers.values() if s.status == "COMPLETED")
            errors = sum(1 for s in self.servers.values() if s.status in ["ERROR", "TIMEOUT", "UNREACHABLE"])
            total_scanned = sum(s.scanned for s in self.servers.values())
            total_hits = sum(s.hits for s in self.servers.values())
            total_targets = sum(s.targets for s in self.servers.values())
            total_speed = sum(s.speed for s in self.servers.values())
            batches_done = sum(s.batches_done for s in self.servers.values())
            batches_total = sum(s.batches_total for s in self.servers.values())
            
            percent = 0
            if batches_total > 0:
                percent = (batches_done / batches_total) * 100
            elif total_targets > 0:
                percent = (total_scanned / total_targets) * 100
            
            return {
                "total_servers": total_servers, "running": running, "stopped": stopped,
                "completed": completed, "errors": errors, "total_scanned": total_scanned,
                "total_hits": total_hits, "total_targets": total_targets,
                "total_speed": round(total_speed, 1), "batches_done": batches_done,
                "batches_total": batches_total, "percent": round(percent, 1),
                "last_update": datetime.now().strftime('%Y-%m-%d %H:%M:%S')
            }
    
    # ==================== WARC STATUS CACHE (Effect 3) ====================

    # File written by backend/app.py with the current WARC run snapshot.
    # We read it directly (rather than importing app) to avoid a circular
    # import between this module and app.py.
    _WARC_STATE_FILE = os.path.join(
        os.path.dirname(os.path.abspath(__file__)), 'warc_state.json'
    )

    def _read_warc_state(self) -> dict:
        """Best-effort load of the WARC sidecar app.py persists. Empty dict
        if the file is missing or malformed — callers handle 'no current
        run' that way too."""
        try:
            if not os.path.exists(self._WARC_STATE_FILE):
                return {}
            with open(self._WARC_STATE_FILE, 'r') as f:
                data = json.load(f) or {}
            return data if isinstance(data, dict) else {}
        except Exception:
            return {}

    def _refresh_warc_status_cache(self) -> None:
        """Each monitor cycle: if a remote WARC harvest is recorded as
        active, fire a single batched SSH command that gathers PID
        liveness, domain count, and log tail. Stash into the cache so
        api_warc_status can return in microseconds instead of spending
        3+ blocking round-trips per request."""
        state = self._read_warc_state()
        run_on = state.get('run_on')
        if not run_on or run_on == 'controller':
            return
        remote_pid = state.get('remote_pid')
        output_path = state.get('output_path')
        log_path = state.get('log_path')
        # Worker path must have at least a PID to probe; if missing,
        # skip — api_warc_status's auto-heal pgrep adoption will pick it
        # up on the next sync call.
        if not remote_pid:
            return

        # Single batched script: one channel, one round-trip.
        pid_int = int(remote_pid) if str(remote_pid).strip().lstrip('-').isdigit() else 0
        out_path = output_path or ''
        log_p = log_path or ''
        script = (
            f"kill -0 {pid_int} 2>/dev/null && echo ALIVE=alive || echo ALIVE=dead; "
            f"echo COUNT=$(wc -l < {out_path} 2>/dev/null || echo 0); "
            f"echo LOG_BEGIN; "
            f"tail -20 {log_p} 2>/dev/null; "
            f"echo LOG_END"
        )
        try:
            raw = self.ssh_exec(run_on, script, timeout=10)
        except Exception:
            raw = ''
        if not raw:
            # Probe failed — keep the previous cache entry intact so
            # callers don't see a transient blip flip the cockpit to
            # "dead". The cached_at stamp will go stale and the status
            # endpoint's sync-fallback path can re-probe if needed.
            return

        alive = False
        count = 0
        log_tail: list = []
        in_log = False
        for line in raw.split('\n'):
            if line.startswith('ALIVE='):
                alive = (line[len('ALIVE='):].strip() == 'alive')
                continue
            if line.startswith('COUNT='):
                try:
                    count = int((line[len('COUNT='):].strip().split() or ['0'])[0])
                except (ValueError, IndexError):
                    count = 0
                continue
            if line == 'LOG_BEGIN':
                in_log = True
                continue
            if line == 'LOG_END':
                in_log = False
                continue
            if in_log and line.strip():
                # warc.go writes progress with \r so a single "line" returned
                # by `tail -20` can balloon to ~16 KB once concatenated.
                # Cap each entry to 400 chars (keeps the human-readable
                # tail intact) and the whole tail to 20 entries so polling
                # the status endpoint stays under a couple of KB instead of
                # the 195 KB we saw in the wild.
                if len(line) > 400:
                    line = line[:200] + ' …[truncated]… ' + line[-180:]
                log_tail.append(line)
        if len(log_tail) > 20:
            log_tail = log_tail[-20:]

        snapshot = {
            'alive': alive,
            'domains_found': count,
            'log_tail': log_tail,
            'remote_pid': pid_int,
            'output_path': out_path,
            'log_path': log_p,
            'cached_at': datetime.now().isoformat(),
        }
        with self._warc_status_lock:
            self._warc_status_cache[run_on] = snapshot

    def get_warc_status_cache(self, ip: str) -> Optional[dict]:
        """Return a snapshot of the cached WARC probe for ip, or None if
        the monitor hasn't filled the cache for this worker yet. The
        snapshot includes a `cached_at` ISO timestamp so callers can
        decide whether to trust it or fall back to a sync probe."""
        with self._warc_status_lock:
            entry = self._warc_status_cache.get(ip)
            return dict(entry) if entry else None

    # ==================== R2 HEALTH CACHE (Effect 4) ====================

    def set_r2_health_probe(self, probe: Optional[Callable[[], dict]]) -> None:
        """Inject the R2 health probe from app.py. Decoupled so this
        module never imports the boto3/app surface. The probe must
        return a dict with keys: state ('connected'|'misconfigured'|
        'unreachable'|'unknown') and last_error (str|None). If the
        probe is None (or raises), the monitor cycle just skips."""
        self._r2_health_probe = probe

    def _refresh_r2_health_cache(self) -> None:
        probe = self._r2_health_probe
        if probe is None:
            return
        try:
            result = probe() or {}
        except Exception as e:
            result = {'state': 'unreachable', 'last_error': f'probe raised: {e}', 'accounts': []}
        if not isinstance(result, dict):
            result = {'accounts': [], 'state': 'unknown'}
        # New multi-account fields.
        raw_accounts = result.get('accounts') if isinstance(result.get('accounts'), list) else []
        accounts: list = []
        for entry in raw_accounts:
            if not isinstance(entry, dict):
                continue
            est = entry.get('state')
            if est not in ('connected', 'misconfigured', 'unreachable', 'unknown'):
                est = 'unknown'
            accounts.append({
                'id': entry.get('id'),
                'label': entry.get('label'),
                'state': est,
                'last_error': entry.get('last_error'),
                # `usage` is the bucket inventory dict produced by
                # app.py's _r2_usage_breakdown — None until the bucket
                # is reachable.
                'usage': entry.get('usage'),
            })
        primary_id = result.get('primary_id')
        all_full = bool(result.get('all_full'))
        # Legacy mirror — the picker's primary determines the
        # `state/last_error/usage` triple so the existing single-account
        # frontend still has something to render.
        primary = next((a for a in accounts if a.get('id') == primary_id), None)
        state = (primary or {}).get('state') if primary else result.get('state')
        if state not in ('connected', 'misconfigured', 'unreachable', 'unknown'):
            state = 'unknown'
        last_error = (primary or {}).get('last_error') if primary else result.get('last_error')
        usage = (primary or {}).get('usage') if primary else result.get('usage')
        with self._r2_health_lock:
            self._r2_health_cache = {
                'accounts': accounts,
                'primary_id': primary_id,
                'all_full': all_full,
                'state': state,
                'last_check': datetime.now().isoformat(),
                'last_error': last_error,
                'usage': usage,
            }

    def get_r2_health(self) -> dict:
        """Snapshot of the R2 health cache. Always returns the dict
        shape callers expect, even when the probe has never run."""
        with self._r2_health_lock:
            return dict(self._r2_health_cache)

    # ==================== CONTROL OPERATIONS ====================

    def start_server(self, ip: str) -> dict:
        work_dir = self.config.get("work_dir", "/root/python_job")
        try:
            running = self.ssh_exec(ip, "pgrep -f 'batch_runner.sh' && echo RUNNING", 5)
            if "RUNNING" in running:
                return {"success": False, "error": "Already running"}
            
            self.ssh_exec(ip, f"cd {work_dir} && nohup ./batch_runner.sh >> output.log 2>&1 &", 10)
            time.sleep(2)
            
            verify = self.ssh_exec(ip, "pgrep -f 'batch_runner.sh' && echo OK", 5)
            if verify:
                return {"success": True, "message": f"Started on {ip}"}
            else:
                return {"success": False, "error": "Failed to start"}
        except Exception as e:
            return {"success": False, "error": str(e)}
    
    def stop_server(self, ip: str) -> dict:
        try:
            self.ssh_exec(ip, "pkill -f 'python.*main.py'; pkill -f 'batch_runner.sh'", 10)
            time.sleep(2)
            check = self.ssh_exec(ip, "pgrep -f 'batch_runner.sh' || echo STOPPED", 5)
            if "STOPPED" in check:
                return {"success": True, "message": f"Stopped on {ip}"}
            else:
                return {"success": False, "error": "Process may still be running"}
        except Exception as e:
            return {"success": False, "error": str(e)}
    
    def restart_server(self, ip: str) -> dict:
        self.stop_server(ip)
        time.sleep(2)
        return self.start_server(ip)
    
    def start_all(self) -> dict:
        ips = self.load_servers()
        results = {"success": 0, "failed": 0, "details": []}
        for ip in ips:
            result = self.start_server(ip)
            if result["success"]:
                results["success"] += 1
            else:
                results["failed"] += 1
            results["details"].append({"ip": ip, **result})
        return results
    
    def stop_all(self) -> dict:
        ips = self.load_servers()
        results = {"success": 0, "failed": 0, "details": []}
        for ip in ips:
            result = self.stop_server(ip)
            if result["success"]:
                results["success"] += 1
            else:
                results["failed"] += 1
            results["details"].append({"ip": ip, **result})
        return results
    
    def restart_all(self) -> dict:
        ips = self.load_servers()
        results = {"success": 0, "failed": 0, "details": []}
        for ip in ips:
            result = self.restart_server(ip)
            if result["success"]:
                results["success"] += 1
            else:
                results["failed"] += 1
            results["details"].append({"ip": ip, **result})
        return results
    
    # ==================== RESULTS COLLECTION ====================
    
    def collect_results(self, ip: str) -> dict:
        work_dir = self.config.get("work_dir", "/root/python_job")
        result_dir = self.config.get("result_subdir", "ResultJS")
        local_results_dir = self.config.get("results_dir", "./collected_results")
        os.makedirs(local_results_dir, exist_ok=True)
        
        try:
            ssh = self._get_ssh_client(ip, 30)
            sftp = ssh.open_sftp()
            remote_result_path = f"{work_dir}/{result_dir}"
            
            try:
                files = sftp.listdir(remote_result_path)
            except:
                ssh.close()
                return {"success": False, "error": "No results directory", "files": 0}
            
            collected = 0
            for filename in files:
                if filename.endswith('.txt'):
                    remote_file = f"{remote_result_path}/{filename}"
                    local_file = os.path.join(local_results_dir, f"{ip.replace('.', '_')}_{filename}")
                    try:
                        sftp.get(remote_file, local_file)
                        collected += 1
                    except:
                        pass
            
            sftp.close()
            ssh.close()
            return {"success": True, "files": collected, "ip": ip}
        except Exception as e:
            return {"success": False, "error": str(e), "files": 0}
    
    def collect_all_results(self) -> dict:
        ips = self.load_servers()
        results = {"total_files": 0, "servers_success": 0, "servers_failed": 0, "details": []}
        for ip in ips:
            result = self.collect_results(ip)
            results["total_files"] += result.get("files", 0)
            if result["success"]:
                results["servers_success"] += 1
            else:
                results["servers_failed"] += 1
            results["details"].append({"ip": ip, **result})
        return results
    
    def get_server_logs(self, ip: str, lines: int = 50) -> dict:
        work_dir = self.config.get("work_dir", "/root/python_job")
        try:
            logs = self.ssh_exec(ip, f"tail -{lines} {work_dir}/output.log 2>/dev/null", 15)
            return {"success": True, "logs": logs, "ip": ip}
        except Exception as e:
            return {"success": False, "error": str(e), "logs": ""}
    
    # ==================== DIAGNOSTICS ====================
    
    def diagnose_server(self, ip: str) -> dict:
        work_dir = self.config.get("work_dir", "/root/python_job")
        diagnostics = {
            "ip": ip, "connection": False, "work_dir_exists": False,
            "redis_running": False, "python_available": False,
            "batch_files": 0, "batch_files_done": 0, "batch_files_failed": 0,
            "results_files": 0, "targets_count": 0, "disk_usage": "", "memory": "", "issues": []
        }
        
        try:
            conn_test = self.ssh_exec(ip, "echo OK", 5)
            diagnostics["connection"] = "OK" in conn_test
            
            if not diagnostics["connection"]:
                diagnostics["issues"].append("Cannot connect via SSH")
                return diagnostics
            
            dir_check = self.ssh_exec(ip, f"[ -d {work_dir} ] && echo YES || echo NO", 5)
            diagnostics["work_dir_exists"] = "YES" in dir_check
            if not diagnostics["work_dir_exists"]:
                diagnostics["issues"].append(f"Work directory {work_dir} does not exist")
            
            redis_check = self.ssh_exec(ip, "pgrep redis-server && echo YES || echo NO", 5)
            diagnostics["redis_running"] = "YES" in redis_check
            if not diagnostics["redis_running"]:
                diagnostics["issues"].append("Redis is not running")
            
            python_check = self.ssh_exec(ip, "python3 --version && echo OK", 5)
            diagnostics["python_available"] = "OK" in python_check
            
            diagnostics["batch_files"] = int(self.ssh_exec(ip, f"ls {work_dir}/batch_*.txt 2>/dev/null | wc -l", 5) or 0)
            diagnostics["batch_files_done"] = int(self.ssh_exec(ip, f"ls {work_dir}/batch_*.txt.done 2>/dev/null | wc -l", 5) or 0)
            diagnostics["batch_files_failed"] = int(self.ssh_exec(ip, f"ls {work_dir}/batch_*.txt.failed 2>/dev/null | wc -l", 5) or 0)
            diagnostics["results_files"] = int(self.ssh_exec(ip, f"ls {work_dir}/Result/*.txt 2>/dev/null | wc -l", 5) or 0)
            diagnostics["targets_count"] = int(self.ssh_exec(ip, f"wc -l < {work_dir}/server_main_list.txt 2>/dev/null || echo 0", 5) or 0)
            diagnostics["disk_usage"] = self.ssh_exec(ip, "df -h / | tail -1 | awk '{print $5}'", 5)
            diagnostics["memory"] = self.ssh_exec(ip, "free -h | grep Mem | awk '{print $3\"/\"$2}'", 5)
            
            if diagnostics["batch_files_failed"] > 0:
                diagnostics["issues"].append(f"{diagnostics['batch_files_failed']} failed batches")
        except Exception as e:
            diagnostics["issues"].append(f"Error: {str(e)}")
        
        return diagnostics
    
    def fix_server(self, ip: str) -> dict:
        work_dir = self.config.get("work_dir", "/root/python_job")
        results = []
        
        try:
            results.append("Stopping processes...")
            self.ssh_exec(ip, "pkill -f 'python.*main.py'; pkill -f 'batch_runner.sh'", 10)
            time.sleep(2)
            
            results.append("Starting Redis...")
            self.ssh_exec(ip, "systemctl start redis-server 2>/dev/null || redis-server --daemonize yes", 10)
            
            results.append("Installing dependencies...")
            self.ssh_exec(ip, f"""
                cd {work_dir}
                pip3 install urllib3 requests httpx tqdm colorama redis aiohttp fake-useragent dnspython --break-system-packages 2>/dev/null
                [ -f requirements.txt ] && pip3 install -r requirements.txt --break-system-packages 2>/dev/null
            """, 120)
            
            results.append("Resetting failed batches...")
            self.ssh_exec(ip, f"""
                cd {work_dir}
                for f in batch_*.txt.failed; do [ -f "$f" ] && mv "$f" "${{f%.failed}}"; done
            """, 30)
            
            results.append("Clearing Redis cache...")
            self.ssh_exec(ip, "redis-cli FLUSHALL 2>/dev/null || true", 5)
            
            results.append("Starting scanner...")
            self.ssh_exec(ip, f"cd {work_dir} && nohup ./batch_runner.sh >> output.log 2>&1 &", 10)
            time.sleep(3)
            
            verify = self.ssh_exec(ip, "pgrep -f 'batch_runner.sh' && echo OK", 5)
            if verify:
                results.append("✅ Server fixed and running")
                return {"success": True, "steps": results}
            else:
                results.append("❌ Failed to start after fixes")
                return {"success": False, "steps": results}
        except Exception as e:
            results.append(f"❌ Error: {str(e)}")
            return {"success": False, "steps": results}
    
    # ==================== MONITORING ====================
    
    def start_monitoring(self, callback: Callable = None, interval: int = None):
        if self.monitor_thread and self.monitor_thread.is_alive():
            return False

        if interval is None:
            interval = int(self.config.get("monitor_interval", 30))

        self.stop_monitor.clear()
        self.event_callback = callback

        def monitor_loop():
            while not self.stop_monitor.is_set():
                try:
                    self.get_all_status()
                    if self.event_callback:
                        stats = self.get_global_stats()
                        servers = [s.to_dict() for s in self.servers.values()]
                        self.event_callback({"type": "status_update", "stats": stats, "servers": servers})
                except Exception as e:
                    print(f"Monitor error: {e}")
                # Piggyback the WARC status + R2 health refreshes onto
                # the same cadence so we don't introduce a second thread.
                try:
                    self._refresh_warc_status_cache()
                except Exception as e:
                    print(f"WARC status cache refresh error: {e}")
                try:
                    self._refresh_r2_health_cache()
                except Exception as e:
                    print(f"R2 health cache refresh error: {e}")
                self.stop_monitor.wait(interval)

        self.monitor_thread = threading.Thread(target=monitor_loop, daemon=True)
        self.monitor_thread.start()
        return True

    def stop_monitoring(self):
        self.stop_monitor.set()
        if self.monitor_thread:
            self.monitor_thread.join(timeout=5)
        # Close every pooled SSH session so the workers don't see stranded
        # half-open channels after a controller restart.
        with self._pool_lock:
            for ssh in self._ssh_pool.values():
                try: ssh.close()
                except Exception: pass
            self._ssh_pool.clear()


_manager_instance = None

def get_manager() -> SSHManager:
    global _manager_instance
    if _manager_instance is None:
        _manager_instance = SSHManager()
    return _manager_instance
