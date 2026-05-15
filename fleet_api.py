#!/usr/bin/env python3
"""Fleet control plane — SSH enroll + deploy for the scan cockpit dashboard.

Endpoints:
  GET  /health
  POST /api/fleet/enroll   { host, port, user, secret, authType, vpsId }
  POST /api/fleet/deploy   { host, port, user, secret, authType, vpsId, listName, targets }

Run: python fleet_api.py [--port 8787] [--token SECRET]
Requires: paramiko
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any
from urllib.parse import urlparse

try:
    import paramiko
except ImportError:
    print("Install paramiko: pip install paramiko", file=sys.stderr)
    sys.exit(1)

LOCAL_GO_FILE = os.environ.get("RAVENX_GO", "ravenx.go")
LOCAL_CONFIG_FILE = os.environ.get("RAVENX_CONFIG", "config.json")
AUTH_TOKEN = os.environ.get("FLEET_API_TOKEN", "")


def cors_headers(handler: BaseHTTPRequestHandler) -> None:
    handler.send_header("Access-Control-Allow-Origin", "*")
    handler.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
    handler.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")


def read_json(handler: BaseHTTPRequestHandler) -> dict[str, Any]:
    length = int(handler.headers.get("Content-Length", 0))
    raw = handler.rfile.read(length) if length else b"{}"
    try:
        return json.loads(raw.decode("utf-8"))
    except json.JSONDecodeError:
        return {}


def auth_ok(handler: BaseHTTPRequestHandler, token: str) -> bool:
    if not token:
        return True
    auth = handler.headers.get("Authorization", "")
    if auth == f"Bearer {token}":
        return True
    return False


def ssh_connect(host: str, port: int, user: str, secret: str, auth_type: str) -> paramiko.SSHClient:
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    kwargs: dict[str, Any] = {
        "hostname": host,
        "port": port,
        "username": user,
        "timeout": 20,
        "allow_agent": False,
        "look_for_keys": False,
    }
    if auth_type == "key":
        key_file = tempfile.NamedTemporaryFile(mode="w", delete=False, suffix=".pem")
        try:
            key_file.write(secret)
            key_file.close()
            for key_cls in (
                paramiko.RSAKey,
                paramiko.Ed25519Key,
                paramiko.ECDSAKey,
            ):
                try:
                    pkey = key_cls.from_private_key_file(key_file.name)
                    kwargs["pkey"] = pkey
                    break
                except Exception:
                    continue
            else:
                raise ValueError("Could not parse private key material")
            client.connect(**kwargs)
        finally:
            try:
                os.unlink(key_file.name)
            except OSError:
                pass
    else:
        kwargs["password"] = secret
        client.connect(**kwargs)
    return client


def guess_region(host: str) -> str:
    if host.startswith(("104.", "159.")):
        return "NYC3"
    if host.startswith(("128.", "139.")):
        return "SGP1"
    if host.startswith(("188.", "167.")):
        return "AMS3"
    return "DISC"


def remote_hostname(ssh: paramiko.SSHClient) -> str:
    _, stdout, _ = ssh.exec_command("hostname -f 2>/dev/null || hostname", timeout=10)
    out = stdout.read().decode("utf-8", errors="replace").strip()
    return out or "unknown"


def deploy_scanner(ssh: paramiko.SSHClient, targets: str, list_name: str) -> str:
    if not os.path.isfile(LOCAL_GO_FILE):
        raise FileNotFoundError(f"Missing {LOCAL_GO_FILE} beside fleet_api.py")

    sftp = ssh.open_sftp()
    remote_dir = f"/tmp/ravenx-{list_name.replace('/', '_')[:48]}"
    ssh.exec_command(f"mkdir -p {remote_dir}")
    sftp.put(LOCAL_GO_FILE, f"{remote_dir}/ravenx.go")
    if os.path.isfile(LOCAL_CONFIG_FILE):
        sftp.put(LOCAL_CONFIG_FILE, f"{remote_dir}/config.json")
    with sftp.file(f"{remote_dir}/target_list.txt", "w") as f:
        f.write(targets)
    sftp.close()

    setup = f"""
cd {remote_dir}
export PATH=$PATH:/usr/local/go/bin:/usr/bin
if ! command -v go >/dev/null 2>&1; then
  apt-get update -y && apt-get install -y golang-go
fi
if [ ! -f go.mod ]; then go mod init ravenx; fi
go mod tidy 2>/dev/null || true
nohup go run ravenx.go -hybrid target_list.txt > output.log 2>&1 &
echo started
"""
    _, stdout, stderr = ssh.exec_command(setup, timeout=120)
    msg = stdout.read().decode("utf-8", errors="replace").strip()
    err = stderr.read().decode("utf-8", errors="replace").strip()
    if "started" not in msg.lower():
        raise RuntimeError(err or msg or "deploy script failed")
    return f"Scanner started in {remote_dir}"


class FleetHandler(BaseHTTPRequestHandler):
    server_token: str = ""

    def log_message(self, fmt: str, *args: Any) -> None:
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def _json(self, code: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        cors_headers(self)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_OPTIONS(self) -> None:
        self.send_response(204)
        cors_headers(self)
        self.end_headers()

    def do_GET(self) -> None:
        if self.path.rstrip("/") == "/health":
            self._json(200, {"ok": True, "service": "fleet-api"})
            return
        self._json(404, {"ok": False, "message": "not found"})

    def do_POST(self) -> None:
        if not auth_ok(self, self.server_token):
            self._json(401, {"ok": False, "message": "unauthorized"})
            return

        path = urlparse(self.path).path
        data = read_json(self)

        if path == "/api/fleet/enroll":
            host = str(data.get("host", "")).strip()
            port = int(data.get("port", 22) or 22)
            user = str(data.get("user", "root")).strip()
            secret = str(data.get("secret", ""))
            auth_type = str(data.get("authType", "password"))
            if not host or not secret:
                self._json(400, {"ok": False, "message": "host and secret required"})
                return
            try:
                ssh = ssh_connect(host, port, user, secret, auth_type)
                try:
                    hn = remote_hostname(ssh)
                    region = guess_region(host)
                    self._json(
                        200,
                        {
                            "ok": True,
                            "message": f"SSH session OK as {user}@{host}",
                            "hostname": hn,
                            "region": region,
                        },
                    )
                finally:
                    ssh.close()
            except Exception as exc:
                self._json(502, {"ok": False, "message": str(exc)})
            return

        if path == "/api/fleet/deploy":
            host = str(data.get("host", "")).strip()
            port = int(data.get("port", 22) or 22)
            user = str(data.get("user", "root")).strip()
            secret = str(data.get("secret", ""))
            auth_type = str(data.get("authType", "password"))
            targets = str(data.get("targets", ""))
            list_name = str(data.get("listName", "targets"))
            if not host or not secret or not targets.strip():
                self._json(400, {"ok": False, "message": "host, secret, and targets required"})
                return
            try:
                ssh = ssh_connect(host, port, user, secret, auth_type)
                try:
                    msg = deploy_scanner(ssh, targets, list_name)
                    self._json(200, {"ok": True, "message": msg})
                finally:
                    ssh.close()
            except Exception as exc:
                self._json(502, {"ok": False, "message": str(exc)})
            return

        self._json(404, {"ok": False, "message": "not found"})


def main() -> None:
    parser = argparse.ArgumentParser(description="Fleet control plane API")
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8787)
    parser.add_argument("--token", default=AUTH_TOKEN, help="Bearer token (optional)")
    args = parser.parse_args()

    FleetHandler.server_token = args.token
    server = ThreadingHTTPServer((args.host, args.port), FleetHandler)
    print(f"[*] Fleet API listening on http://{args.host}:{args.port}")
    if args.token:
        print("[*] Bearer auth enabled")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n[*] Stopped")


if __name__ == "__main__":
    main()
