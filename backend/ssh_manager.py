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
        # Sticky status — a worker has to miss two consecutive probes before
        # its card flips to UNREACHABLE, so a single transient blip doesn't
        # make every card flap RECONNECT↔HEALTHY.
        self._consecutive_misses: Dict[str, int] = {}
        self._sticky_threshold = 2

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

    def _get_ssh_client(self, ip: str, timeout: int = None) -> paramiko.SSHClient:
        timeout = timeout or self.config.get("ssh_timeout", 10)
        ssh = paramiko.SSHClient()
        ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

        username = self._user_for(ip)
        port = self._port_for(ip)
        ssh_key_path = self.config.get("ssh_key_path")
        pkey = self._load_private_key(ssh_key_path)

        if pkey:
            ssh.connect(
                ip,
                port=port,
                username=username,
                pkey=pkey,
                timeout=timeout,
                banner_timeout=10,
                auth_timeout=10,
                look_for_keys=False,
                allow_agent=False
            )
        else:
            # Fallback to key_filename if _load_private_key fails
            ssh.connect(
                ip,
                port=port,
                username=username,
                key_filename=ssh_key_path,
                timeout=timeout,
                banner_timeout=10,
                auth_timeout=10,
                look_for_keys=False,
                allow_agent=False
            )
        return ssh
    
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
                tr.set_keepalive(int(self.config.get("ssh_keepalive", 30)))
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

    def ssh_exec(self, ip: str, cmd: str, timeout: int = None) -> str:
        timeout = timeout or self.config.get("ssh_timeout", 10)
        try:
            ssh = self._get_pooled_ssh(ip, timeout)
            try:
                stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
                return stdout.read().decode('utf-8', errors='replace').strip()
            except (paramiko.SSHException, EOFError, OSError):
                # Connection died mid-exec — evict and retry once with a fresh one.
                self._evict_pooled_ssh(ip)
                ssh = self._get_pooled_ssh(ip, timeout)
                stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
                return stdout.read().decode('utf-8', errors='replace').strip()
        except Exception:
            self._evict_pooled_ssh(ip)
            return ""
    
    def scp_upload(self, ip: str, local_path: str, remote_path: str) -> bool:
        try:
            ssh = self._get_ssh_client(ip, 30)
            sftp = ssh.open_sftp()
            sftp.put(local_path, remote_path)
            sftp.close()
            ssh.close()
            return True
        except:
            return False
    
    def scp_download(self, ip: str, remote_path: str, local_path: str) -> bool:
        try:
            ssh = self._get_ssh_client(ip, 30)
            sftp = ssh.open_sftp()
            sftp.get(remote_path, local_path)
            sftp.close()
            ssh.close()
            return True
        except:
            return False
    
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
    
    def deploy_full(self, target_file: str = None, scanner_file: str = None, 
                    runner_file: str = None, auto_start: bool = False,
                    progress_callback: Callable = None) -> dict:
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
                
                # 2.5. Upload raven-scanner binary (Go binary)
                if os.path.exists('raven-scanner'):
                    log("  Uploading raven-scanner binary...")
                    if self.scp_upload(ip, 'raven-scanner', f"{work_dir}/raven-scanner"):
                        self.ssh_exec(ip, f"chmod +x {work_dir}/raven-scanner", 5)
                        server_result.steps.append("Uploaded raven-scanner binary")
                    else:
                        log("  ⚠ Failed to upload binary, trying package...")
                
                # 2.6. Upload package if exists (fallback)
                if os.path.exists('scanner_package.tar.gz'):
                    log("  Uploading scanner package...")
                    if self.scp_upload(ip, 'scanner_package.tar.gz', f"{work_dir}/scanner_package.tar.gz"):
                        # Extract package
                        extract_cmd = f"cd {work_dir} && tar -xzf scanner_package.tar.gz && chmod +x raven-scanner 2>/dev/null || true"
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
    
    def fetch_server_status(self, ip: str) -> ServerStatus:
        status = ServerStatus(ip=ip)
        work_dir = self.config.get("work_dir", "/root/python_job")
        result_dir = self.config.get("result_subdir", "ResultJS")
        dedup_file = self.config.get("dedup_file", "dedup_log.txt")
        batch_size = self.config.get("batch_size", 100000)

        # Reachability gate — try a real SSH session before issuing the
        # nine status queries. If this fails, we know the box is
        # unreachable and can apply the sticky-status rule (one miss is
        # kept quiet; two consecutive misses flip the card to UNREACHABLE).
        try:
            ssh = self._get_pooled_ssh(ip, self.config.get("ssh_timeout", 5))
            stdin, stdout, _ = ssh.exec_command("true", timeout=5)
            stdout.channel.recv_exit_status()
        except Exception as e:
            with self.lock:
                misses = self._consecutive_misses.get(ip, 0) + 1
                self._consecutive_misses[ip] = misses
                if misses < self._sticky_threshold and ip in self.servers:
                    prev = self.servers[ip]
                    prev.last_update = datetime.now().strftime('%H:%M:%S')
                    return prev
                status.status = "UNREACHABLE"
                status.error = str(e)[:80] or 'ssh probe failed'
                status.last_update = datetime.now().strftime('%H:%M:%S')
                self.servers[ip] = status
            return status

        # Probe succeeded — reset the miss counter for this host.
        with self.lock:
            self._consecutive_misses[ip] = 0

        try:
            main_pid = self.ssh_exec(ip, "pgrep -f 'python.*main.py' | head -1", 5)
            batch_pid = self.ssh_exec(ip, "pgrep -f 'batch_runner.sh' | head -1", 5)

            if main_pid or batch_pid:
                status.status = "RUNNING"
                pid = main_pid or batch_pid
                status.uptime = self.ssh_exec(ip, f"ps -o etime= -p {pid} 2>/dev/null | tr -d ' '", 5) or "-"
            else:
                complete_check = self.ssh_exec(ip, f"grep -c 'ALL COMPLETE' {work_dir}/output.log 2>/dev/null", 3)
                if complete_check and complete_check.isdigit() and int(complete_check) > 0:
                    status.status = "COMPLETED"
                else:
                    # Reachable but no scanner — surface as IDLE so the UI
                    # shows healthy (frontend mapStatus treats IDLE as
                    # healthy). The legacy "STOPPED" string used to fall
                    # through mapStatus's default and render as RECONNECT,
                    # which made every healthy idle worker look broken.
                    status.status = "IDLE"
            
            batches_pending = int(self.ssh_exec(ip, f"ls {work_dir}/batch_*.txt 2>/dev/null | wc -l", 5) or 0)
            batches_done = int(self.ssh_exec(ip, f"ls {work_dir}/batch_*.txt.done 2>/dev/null | wc -l", 5) or 0)
            batches_failed = int(self.ssh_exec(ip, f"ls {work_dir}/batch_*.txt.failed 2>/dev/null | wc -l", 5) or 0)
            total_batches = batches_pending + batches_done + batches_failed
            
            batch_str = f"{batches_done}/{total_batches}"
            if batches_failed > 0:
                batch_str += f" ({batches_failed}!)"
            status.batch_info = batch_str
            status.batches_done = batches_done
            status.batches_total = total_batches
            
            status.targets = int(self.ssh_exec(ip, f"wc -l < {work_dir}/server_main_list.txt 2>/dev/null || echo 0", 5) or 0)
            
            scanned_from_done = batches_done * batch_size
            current_scanned = 0
            
            redis_keys = self.ssh_exec(ip, "redis-cli KEYS '*hash*' 2>/dev/null", 5)
            if redis_keys:
                for key in redis_keys.split('\n'):
                    key = key.strip()
                    if key:
                        count = self.ssh_exec(ip, f"redis-cli SCARD {key} 2>/dev/null", 5)
                        if count and count.isdigit() and int(count) > current_scanned:
                            current_scanned = int(count)
            
            if current_scanned == 0:
                for pattern in ['gemini:dedup:hashes', 'gemini:hashes', 'dedup:hashes']:
                    count = self.ssh_exec(ip, f"redis-cli SCARD {pattern} 2>/dev/null", 5)
                    if count and count.isdigit() and int(count) > 0:
                        current_scanned = int(count)
                        break
            
            if current_scanned == 0:
                dedup_count = self.ssh_exec(ip, f"wc -l < {work_dir}/{dedup_file} 2>/dev/null", 5)
                if dedup_count and dedup_count.isdigit():
                    current_scanned = int(dedup_count)
            
            status.scanned = scanned_from_done + current_scanned
            status.current_batch_progress = current_scanned
            
            prev_scanned = self.previous_scanned.get(ip, 0)
            if prev_scanned > 0 and status.scanned > prev_scanned:
                status.speed = (status.scanned - prev_scanned) / 5
            self.previous_scanned[ip] = status.scanned
            
            hits_content = self.ssh_exec(ip, f"cat {work_dir}/{result_dir}/*.txt 2>/dev/null | head -100", 8)
            valid_count = 0
            if hits_content:
                for line in hits_content.split('\n'):
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
