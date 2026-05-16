#!/bin/bash
###############################################################################
# RAVEN X 2.0 - ONE COMMAND AUTO-INSTALLER FOR VPS
# 
# Usage: curl -sSL https://raw.githubusercontent.com/your-repo/main/install.sh | sudo bash
# Or:    wget -qO- https://your-server/install.sh | sudo bash
#
# This script will:
# - Install all dependencies
# - Download dashboard files
# - Configure systemd service
# - Setup Nginx
# - Start dashboard
###############################################################################

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

# Configuration
INSTALL_DIR="/opt/raven-dashboard"
SERVICE_NAME="raven-dashboard"
PORT=5000

echo -e "${CYAN}"
echo "============================================================"
echo "         RAVEN X 2.0 - AUTO INSTALLER (VPS)"
echo "============================================================"
echo -e "${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}[ERROR] Please run with sudo${NC}"
    echo "Usage: sudo bash install.sh"
    exit 1
fi

echo -e "${YELLOW}[1/12] Updating system...${NC}"
apt-get update -qq

echo -e "${YELLOW}[2/12] Installing dependencies...${NC}"
apt-get install -y python3 python3-pip python3-venv nginx sqlite3 curl wget git > /dev/null 2>&1
echo -e "${GREEN}✓ Dependencies installed${NC}"

echo -e "${YELLOW}[3/12] Creating installation directory...${NC}"
mkdir -p $INSTALL_DIR
cd $INSTALL_DIR
echo -e "${GREEN}✓ Directory created: $INSTALL_DIR${NC}"

echo -e "${YELLOW}[4/12] Creating directory structure...${NC}"
mkdir -p templates static/css static/js config scripts ResultJS logs backups

echo -e "${YELLOW}[5/12] Creating app.py...${NC}"
cat > app.py << 'EOFAPP'
#!/usr/bin/env python3
from flask import Flask, render_template, jsonify, send_file
from flask_socketio import SocketIO, emit
import sqlite3
import threading
import time
from datetime import datetime
import os

app = Flask(__name__)
app.config['SECRET_KEY'] = 'raven-x-secret-change-in-production'
socketio = SocketIO(app, cors_allowed_origins="*", async_mode='threading')

DB_PATH = 'raven_results.db'
RESULTS_DIR = 'ResultJS'

FILE_MAPPING = {
    'aws_valid.txt': 'AWS', 'aws_credentials.txt': 'AWS',
    'valid_github_token.txt': 'GitHub', 'valid_sendgrid.txt': 'SendGrid',
    'valid_stripe.txt': 'Stripe', 'valid_openai.txt': 'OpenAI',
    'valid_anthropic.txt': 'Anthropic', 'smtp_valid.txt': 'SMTP',
    'smtp_found.txt': 'SMTP', 'valid_mailgun.txt': 'Mailgun',
    'valid_twilio.txt': 'Twilio', 'valid_nexmo.txt': 'Nexmo',
    'valid_telnyx.txt': 'Telnyx', 'valid_messagebird.txt': 'MessageBird',
    'valid_brevo.txt': 'Brevo', 'valid_mandrill.txt': 'Mandrill',
    'valid_mailersend.txt': 'MailerSend', 'valid_gcp_key.txt': 'GCP',
    'mnemonic_seed_phrases.txt': 'Mnemonic', 'trufflehog_secrets.txt': 'TruffleHog',
    'gitleaks_secrets.txt': 'GitLeaks',
}

file_mtimes = {}

def init_db():
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    cursor.execute('''CREATE TABLE IF NOT EXISTS credentials (
        id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL,
        key_value TEXT NOT NULL, source_url TEXT, status TEXT DEFAULT 'valid',
        metadata TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
        UNIQUE(type, key_value))''')
    cursor.execute('''CREATE TABLE IF NOT EXISTS statistics (
        id INTEGER PRIMARY KEY, total_urls_scanned INTEGER DEFAULT 0,
        total_credentials_found INTEGER DEFAULT 0, total_validated INTEGER DEFAULT 0,
        last_update DATETIME DEFAULT CURRENT_TIMESTAMP)''')
    cursor.execute('INSERT OR IGNORE INTO statistics (id) VALUES (1)')
    conn.commit()
    conn.close()

def import_from_files():
    if not os.path.exists(RESULTS_DIR):
        os.makedirs(RESULTS_DIR)
        return 0
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    imported = 0
    for filename, cred_type in FILE_MAPPING.items():
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
                    parts = line.split(':', 1)
                    if len(parts) < 2:
                        continue
                    source_url = parts[0].strip()
                    key_and_rest = parts[1].strip()
                    key_parts = key_and_rest.split(':', 1)
                    key_value = key_parts[0].strip()
                    metadata = key_parts[1].strip() if len(key_parts) > 1 else ""
                    cursor.execute('''INSERT OR IGNORE INTO credentials (type, key_value, source_url, metadata)
                        VALUES (?, ?, ?, ?)''', (cred_type, key_value, source_url, metadata))
                    if cursor.rowcount > 0:
                        imported += 1
        except: pass
    cursor.execute('SELECT COUNT(*) FROM credentials WHERE status="valid"')
    total_valid = cursor.fetchone()[0]
    cursor.execute('''UPDATE statistics SET total_validated = ?, total_credentials_found = ?,
        last_update = CURRENT_TIMESTAMP WHERE id = 1''', (total_valid, total_valid))
    conn.commit()
    conn.close()
    return imported

def get_statistics():
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM statistics WHERE id = 1')
    stats = cursor.fetchone()
    cursor.execute('''SELECT type, COUNT(*) as count FROM credentials WHERE status = 'valid'
        GROUP BY type ORDER BY count DESC''')
    type_counts = dict(cursor.fetchall())
    cursor.execute('''SELECT type, key_value, source_url, timestamp, metadata FROM credentials
        ORDER BY timestamp DESC LIMIT 20''')
    recent_findings = cursor.fetchall()
    conn.close()
    return {'total_urls': stats[1] if stats else 0, 'total_found': stats[2] if stats else 0,
        'total_validated': stats[3] if stats else 0, 'type_counts': type_counts,
        'recent_findings': recent_findings, 'last_update': datetime.now().strftime('%Y-%m-%d %H:%M:%S')}

def background_file_monitor():
    last_count = 0
    while True:
        time.sleep(3)
        try:
            imported = import_from_files()
            if imported > 0:
                print(f"📥 Imported {imported} new credentials")
            stats = get_statistics()
            current_count = stats['total_validated']
            if current_count != last_count:
                socketio.emit('stats_update', stats, broadcast=True)
                last_count = current_count
        except: pass

@app.route('/')
def index():
    return render_template('dashboard.html')

@app.route('/api/stats')
def api_stats():
    return jsonify(get_statistics())

@socketio.on('connect')
def handle_connect():
    emit('stats_update', get_statistics())

@socketio.on('disconnect')
def handle_disconnect():
    pass

@socketio.on('request_update')
def handle_request_update():
    import_from_files()
    emit('stats_update', get_statistics())

if __name__ == '__main__':
    init_db()
    import_from_files()
    monitor_thread = threading.Thread(target=background_file_monitor, daemon=True)
    monitor_thread.start()
    socketio.run(app, host='0.0.0.0', port=5000, debug=False, allow_unsafe_werkzeug=True)
EOFAPP

echo -e "${GREEN}✓ app.py created${NC}"

echo -e "${YELLOW}[6/12] Creating requirements.txt...${NC}"
cat > requirements.txt << 'EOFREQ'
Flask==3.0.0
flask-socketio==5.3.5
python-socketio==5.10.0
eventlet==0.35.2
EOFREQ
echo -e "${GREEN}✓ requirements.txt created${NC}"

echo -e "${YELLOW}[7/12] Creating templates and static files...${NC}"
# Dashboard HTML will be created in next step
curl -sSL https://raw.githubusercontent.com/google/fonts/main/ofl/orbitron/Orbitron-Regular.ttf -o static/font.ttf 2>/dev/null || true

# Create minimal but functional dashboard HTML
cat > templates/dashboard.html << 'EOFHTML'
<!DOCTYPE html>
<html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>RAVEN X 2.0</title>
<link href="https://fonts.googleapis.com/css2?family=Orbitron:wght@400;700;900&family=Rajdhani:wght@300;400;600;700&display=swap" rel="stylesheet">
<script src="https://cdn.socket.io/4.5.4/socket.io.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
<style>
:root{--bg-dark:#0a0e27;--cyber-blue:#00f3ff;--cyber-purple:#b700ff;--cyber-pink:#ff006e;--cyber-green:#00ff9f;--text-primary:#e0e7ff;--text-secondary:#8b92b8;--card-bg:rgba(16,24,48,0.8);--border-color:rgba(0,243,255,0.3)}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Rajdhani',sans-serif;background:var(--bg-dark);color:var(--text-primary);min-height:100vh}
.container{max-width:1800px;margin:0 auto;padding:20px}
.header{display:flex;justify-content:space-between;align-items:center;padding:20px 30px;background:var(--card-bg);border:1px solid var(--border-color);border-radius:10px;margin-bottom:30px;backdrop-filter:blur(10px)}
.logo-text{font-family:'Orbitron',sans-serif;font-size:2rem;font-weight:900;background:linear-gradient(135deg,var(--cyber-blue),var(--cyber-purple));-webkit-background-clip:text;-webkit-text-fill-color:transparent;letter-spacing:2px}
.stats-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:20px;margin-bottom:30px}
.stat-card{background:var(--card-bg);border:1px solid var(--border-color);border-radius:12px;padding:25px;backdrop-filter:blur(10px)}
.stat-label{font-size:0.85rem;color:var(--text-secondary);letter-spacing:2px;margin-bottom:10px}
.stat-value{font-family:'Orbitron',sans-serif;font-size:2.5rem;font-weight:900;color:var(--cyber-blue);text-shadow:0 0 20px rgba(0,243,255,0.5)}
.credentials-chart{background:var(--card-bg);border:1px solid var(--border-color);border-radius:12px;padding:25px;backdrop-filter:blur(10px);margin-bottom:30px}
.section-title{font-family:'Orbitron',sans-serif;font-size:1.1rem;letter-spacing:2px;margin-bottom:20px}
.findings-container{display:flex;flex-direction:column;gap:12px;max-height:500px;overflow-y:auto}
.finding-item{background:rgba(0,243,255,0.05);border:1px solid rgba(0,243,255,0.2);border-radius:8px;padding:15px;display:grid;grid-template-columns:auto 1fr auto;gap:15px;align-items:center}
.finding-type{font-family:'Orbitron',sans-serif;font-weight:700;color:var(--cyber-blue);padding:5px 12px;background:rgba(0,243,255,0.1);border-radius:15px;font-size:0.85rem}
</style>
</head><body>
<div class="container">
<header class="header"><h1 class="logo-text">RAVEN X <span>2.0</span></h1><div id="status">LOADING...</div><div id="last-update">--:--:--</div></header>
<div class="stats-grid">
<div class="stat-card"><h3 class="stat-label">URLS SCANNED</h3><p class="stat-value" id="total-urls">0</p></div>
<div class="stat-card"><h3 class="stat-label">CREDENTIALS FOUND</h3><p class="stat-value" id="total-found">0</p></div>
<div class="stat-card"><h3 class="stat-label">VALIDATED</h3><p class="stat-value" id="total-validated">0</p></div>
<div class="stat-card"><h3 class="stat-label">SMTP SERVERS</h3><p class="stat-value" id="total-smtp">0</p></div>
</div>
<div class="credentials-chart"><h3 class="section-title">CREDENTIALS BREAKDOWN</h3><canvas id="chart"></canvas></div>
<div class="credentials-chart"><h3 class="section-title">RECENT FINDINGS</h3><div class="findings-container" id="findings"></div></div>
</div>
<script>
let socket=io();let chart;
socket.on('connect',()=>document.getElementById('status').textContent='CONNECTED');
socket.on('stats_update',data=>{
document.getElementById('total-urls').textContent=data.total_urls;
document.getElementById('total-found').textContent=data.total_found;
document.getElementById('total-validated').textContent=data.total_validated;
document.getElementById('total-smtp').textContent=data.type_counts['SMTP']||0;
document.getElementById('last-update').textContent=data.last_update;
updateChart(data.type_counts);updateFindings(data.recent_findings);});
function updateChart(counts){
const ctx=document.getElementById('chart').getContext('2d');
const labels=Object.keys(counts);const data=Object.values(counts);
if(chart){chart.data.labels=labels;chart.data.datasets[0].data=data;chart.update();}
else{chart=new Chart(ctx,{type:'doughnut',data:{labels:labels,datasets:[{data:data,backgroundColor:['#00f3ff','#b700ff','#ff006e','#00ff9f','#ffd600','#ff6b00']}]},options:{responsive:true}});}}
function updateFindings(findings){
const container=document.getElementById('findings');container.innerHTML='';
findings.forEach(([type,key,src,ts])=>{
const div=document.createElement('div');div.className='finding-item';
div.innerHTML=`<div class="finding-type">${type}</div><div><div>${key.substring(0,20)}...</div><div style="font-size:0.85rem;color:var(--text-secondary)">${src||'N/A'}</div></div><div>${new Date(ts).toLocaleTimeString()}</div>`;
container.appendChild(div);});}
setInterval(()=>socket.emit('request_update'),5000);
</script>
</body></html>
EOFHTML
echo -e "${GREEN}✓ Dashboard HTML created${NC}"

echo -e "${YELLOW}[8/12] Setting up Python environment...${NC}"
python3 -m venv venv
source venv/bin/activate
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt
echo -e "${GREEN}✓ Python environment ready${NC}"

echo -e "${YELLOW}[9/12] Creating systemd service...${NC}"
cat > /etc/systemd/system/$SERVICE_NAME.service << EOFSVC
[Unit]
Description=RAVEN X 2.0 Dashboard
After=network.target

[Service]
Type=simple
User=www-data
Group=www-data
WorkingDirectory=$INSTALL_DIR
Environment="PATH=$INSTALL_DIR/venv/bin"
ExecStart=$INSTALL_DIR/venv/bin/python app.py
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOFSVC

systemctl daemon-reload
systemctl enable $SERVICE_NAME
echo -e "${GREEN}✓ Systemd service created${NC}"

echo -e "${YELLOW}[10/12] Configuring Nginx...${NC}"
SERVER_IP=$(hostname -I | awk '{print $1}')
cat > /etc/nginx/sites-available/$SERVICE_NAME << EOFNGINX
server {
    listen 80;
    server_name $SERVER_IP;
    client_max_body_size 60m;
    location / {
        proxy_pass http://127.0.0.1:$PORT;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }
    location /socket.io {
        proxy_pass http://127.0.0.1:$PORT/socket.io;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
EOFNGINX

ln -sf /etc/nginx/sites-available/$SERVICE_NAME /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
echo -e "${GREEN}✓ Nginx configured${NC}"

echo -e "${YELLOW}[11/12] Setting permissions...${NC}"
chown -R www-data:www-data $INSTALL_DIR
chmod 755 $INSTALL_DIR

echo -e "${YELLOW}[12/12] Starting dashboard...${NC}"
systemctl start $SERVICE_NAME
sleep 3

# Check if service is running
if systemctl is-active --quiet $SERVICE_NAME; then
    echo -e "${GREEN}✓ Service started successfully${NC}"
else
    echo -e "${RED}✗ Service failed to start${NC}"
    systemctl status $SERVICE_NAME
    exit 1
fi

echo ""
echo -e "${GREEN}"
echo "============================================================"
echo "              INSTALLATION COMPLETE!"
echo "============================================================"
echo -e "${NC}"
echo ""
echo -e "${CYAN}Dashboard Access:${NC}"
echo "  Direct:  http://$SERVER_IP:$PORT"
echo "  Nginx:   http://$SERVER_IP"
echo ""
echo -e "${CYAN}Upload scanner results:${NC}"
echo "  scp -r ResultJS/* user@$SERVER_IP:$INSTALL_DIR/ResultJS/"
echo ""
echo -e "${CYAN}Service commands:${NC}"
echo "  Status:  sudo systemctl status $SERVICE_NAME"
echo "  Restart: sudo systemctl restart $SERVICE_NAME"
echo "  Stop:    sudo systemctl stop $SERVICE_NAME"
echo "  Logs:    sudo journalctl -u $SERVICE_NAME -f"
echo ""
echo -e "${CYAN}Files location:${NC}"
echo "  Install: $INSTALL_DIR"
echo "  Results: $INSTALL_DIR/ResultJS/"
echo "  Database: $INSTALL_DIR/raven_results.db"
echo ""
echo -e "${GREEN}🎉 Dashboard is now running!${NC}"
echo -e "${GREEN}🌐 Open http://$SERVER_IP:$PORT in your browser${NC}"
echo ""
echo "============================================================"
