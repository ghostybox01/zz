## 🚀 VPS DEPLOYMENT GUIDE - RAVEN X 2.0

### Complete step-by-step guide untuk deploy ke VPS

---

## 📋 BEFORE YOU START

### What You Need:
- ✅ VPS with Ubuntu 20.04+ (DigitalOcean, Linode, AWS EC2, dll)
- ✅ Root/sudo access
- ✅ SSH access ke VPS
- ✅ Domain name (optional)

### VPS Recommendations:
```
Minimum ($5/month):
- 1 vCPU
- 512 MB RAM  
- 10 GB SSD
- Provider: DigitalOcean, Vultr, Linode

Recommended ($10/month):
- 2 vCPU
- 1 GB RAM
- 25 GB SSD
- Better performance for multiple users
```

---

## 🎯 DEPLOYMENT OPTIONS

### Choose Your Method:

| Method | Time | Difficulty | Best For |
|--------|------|------------|----------|
| **Quick Deploy** | 2 min | Easy | Fast setup |
| **Docker** | 3 min | Easy | Containerization |
| **Manual** | 5 min | Medium | Full control |

---

## 🚀 METHOD 1: QUICK DEPLOY (Recommended)

### Step 1: Upload Files to VPS

**From your PC (Windows):**
```bash
# Using SCP
scp -r c:\Users\User\Desktop\SUPATOOLS\raven\raven-dashboard user@your-vps-ip:/tmp/
```

**Or using WinSCP / FileZilla:**
- Upload entire `raven-dashboard` folder to `/tmp/`

### Step 2: SSH to VPS
```bash
ssh user@your-vps-ip
```

### Step 3: Run Quick Deploy
```bash
cd /tmp/raven-dashboard
sudo chmod +x scripts/*.sh
sudo bash scripts/quick-deploy.sh
```

### Step 4: Access Dashboard
```
http://your-vps-ip:5000
```

**Done! Dashboard is live!** ✅

---

## 🐳 METHOD 2: DOCKER DEPLOYMENT

### Step 1: Upload Files
```bash
scp -r raven-dashboard user@your-vps:/opt/
```

### Step 2: SSH to VPS
```bash
ssh user@your-vps
cd /opt/raven-dashboard
```

### Step 3: Install Docker (if needed)
```bash
# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Install Docker Compose
sudo apt-get install docker-compose
```

### Step 4: Deploy with Docker
```bash
sudo chmod +x scripts/*.sh
sudo bash scripts/docker-deploy.sh
```

### Step 5: Verify
```bash
docker-compose ps
docker-compose logs
```

**Dashboard running in container!** 🐳

---

## 🛠️ METHOD 3: MANUAL DEPLOYMENT

### Step 1: Prepare VPS
```bash
# Update system
sudo apt-get update
sudo apt-get upgrade -y

# Install dependencies
sudo apt-get install -y python3 python3-pip python3-venv nginx sqlite3
```

### Step 2: Create Directory
```bash
sudo mkdir -p /opt/raven-dashboard
sudo chown $USER:$USER /opt/raven-dashboard
```

### Step 3: Upload Files
```bash
# From your PC
scp -r raven-dashboard/* user@your-vps:/opt/raven-dashboard/
```

### Step 4: Setup Python Environment
```bash
cd /opt/raven-dashboard
python3 -m venv venv
source venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
```

### Step 5: Create Directories
```bash
mkdir -p ResultJS
mkdir -p backups
```

### Step 6: Test Run
```bash
# Run manually first to test
python app.py

# If working, press Ctrl+C to stop
```

### Step 7: Setup Systemd Service
```bash
# Copy service file
sudo cp config/raven-dashboard.service /etc/systemd/system/

# Edit paths if needed
sudo nano /etc/systemd/system/raven-dashboard.service

# Reload systemd
sudo systemctl daemon-reload

# Enable and start
sudo systemctl enable raven-dashboard
sudo systemctl start raven-dashboard

# Check status
sudo systemctl status raven-dashboard
```

### Step 8: Configure Nginx (Optional)
```bash
sudo bash scripts/setup-nginx.sh
```

### Step 9: Verify
```bash
# Check service
curl http://localhost:5000/api/stats

# Should return JSON with statistics
```

**Manual deployment complete!** ✅

---

## 🌐 DOMAIN SETUP

### If you have a domain:

**Step 1: Point DNS to VPS**
```
A Record: @ → your-vps-ip
A Record: www → your-vps-ip
```

**Step 2: Configure Nginx**
```bash
sudo nano /etc/nginx/sites-available/raven-dashboard
# Change "your-domain.com" to actual domain

sudo nginx -t
sudo systemctl reload nginx
```

**Step 3: Setup SSL**
```bash
sudo certbot --nginx -d your-domain.com -d www.your-domain.com
```

**Access via:**
```
https://your-domain.com
```

---

## 📤 UPLOADING SCANNER RESULTS

### Method 1: SCP (Manual Upload)
```bash
# From PC where scanner is running
scp -r ResultJS/* user@your-vps:/opt/raven-dashboard/ResultJS/
```

### Method 2: Rsync (Continuous Sync)
```bash
# From PC
rsync -avz --progress ResultJS/ user@your-vps:/opt/raven-dashboard/ResultJS/

# Or setup as cron job for auto-sync
*/5 * * * * rsync -az ResultJS/ user@your-vps:/opt/raven-dashboard/ResultJS/ >/dev/null 2>&1
```

### Method 3: Scanner Running on VPS
```bash
# Run scanner directly on VPS
ssh user@your-vps
cd /opt/raven-scanner
./raven urls.txt

# Results automatically available to dashboard!
```

### Method 4: Webhook/API (Advanced)
```bash
# Setup webhook endpoint in app.py
# Scanner POSTs results directly to dashboard API
# Real-time updates without file transfer
```

---

## 🔐 SECURITY BEST PRACTICES

### 1. Change Default Secret Key
```bash
# Generate random key
python3 -c "import secrets; print(secrets.token_hex(32))"

# Set in environment or app.py
export SECRET_KEY="your-generated-key"
```

### 2. Setup Firewall
```bash
sudo ufw enable
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp   # SSH
sudo ufw allow 80/tcp   # HTTP
sudo ufw allow 443/tcp  # HTTPS
```

### 3. Disable Direct Port Access
```bash
# Only allow access via Nginx
sudo ufw delete allow 5000/tcp

# Edit app.py to bind to localhost only
# Change: host='0.0.0.0' to host='127.0.0.1'
```

### 4. Regular Updates
```bash
# Update system
sudo apt-get update && sudo apt-get upgrade -y

# Update Python packages
pip list --outdated
pip install --upgrade package-name
```

### 5. Limit SSH Access
```bash
# Only allow key-based auth
sudo nano /etc/ssh/sshd_config
# Set: PasswordAuthentication no

sudo systemctl restart sshd
```

---

## 📊 MONITORING SETUP

### Setup Monitoring with Uptime Kuma:
```bash
# Install Uptime Kuma (optional)
docker run -d --restart=always \
  -p 3001:3001 \
  -v uptime-kuma:/app/data \
  --name uptime-kuma \
  louislam/uptime-kuma:1

# Monitor: http://your-vps-ip:5000/api/stats
```

### Setup Alerts:
```bash
# Add to cron for health checks
*/5 * * * * curl -f http://localhost:5000/api/stats || echo "Dashboard down!" | mail -s "Alert" admin@example.com
```

---

## 🔧 PERFORMANCE OPTIMIZATION

### 1. Use Gunicorn (Production Server)
```bash
# Install gunicorn (already in requirements.txt)
# Create gunicorn.py config:

cat > /opt/raven-dashboard/gunicorn.conf.py << 'EOF'
bind = "0.0.0.0:5000"
workers = 4
worker_class = "eventlet"
worker_connections = 1000
timeout = 120
keepalive = 5
EOF

# Update systemd service to use gunicorn
# Edit: /etc/systemd/system/raven-dashboard.service
# Change ExecStart to:
# ExecStart=/opt/raven-dashboard/venv/bin/gunicorn -c gunicorn.conf.py app:app
```

### 2. Enable Nginx Caching
```nginx
# Add to nginx.conf
proxy_cache_path /var/cache/nginx levels=1:2 keys_zone=raven_cache:10m max_size=100m;
proxy_cache raven_cache;
proxy_cache_valid 200 1m;
```

### 3. Enable Compression
```nginx
# Add to nginx.conf
gzip on;
gzip_types text/plain text/css application/json application/javascript;
```

---

## 🚨 DISASTER RECOVERY

### Backup Strategy:
```bash
# Create backup script
sudo nano /opt/raven-dashboard/scripts/backup.sh

#!/bin/bash
BACKUP_DIR="/opt/raven-dashboard/backups"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# Backup database
cp /opt/raven-dashboard/raven_results.db $BACKUP_DIR/db_$DATE.db

# Backup ResultJS
tar -czf $BACKUP_DIR/results_$DATE.tar.gz /opt/raven-dashboard/ResultJS/

# Delete old backups (keep last 7 days)
find $BACKUP_DIR -name "*.db" -mtime +7 -delete
find $BACKUP_DIR -name "*.tar.gz" -mtime +7 -delete

echo "Backup completed: $DATE"
```

### Setup Cron for Auto Backup:
```bash
# Add to crontab
sudo crontab -e

# Add this line (daily at 2 AM):
0 2 * * * /opt/raven-dashboard/scripts/backup.sh >> /var/log/raven-backup.log 2>&1
```

### Restore from Backup:
```bash
# Stop service
sudo systemctl stop raven-dashboard

# Restore database
sudo cp /opt/raven-dashboard/backups/db_YYYYMMDD_HHMMSS.db \
       /opt/raven-dashboard/raven_results.db

# Restore results
sudo tar -xzf /opt/raven-dashboard/backups/results_YYYYMMDD_HHMMSS.tar.gz \
            -C /

# Start service
sudo systemctl start raven-dashboard
```

---

## ✅ POST-DEPLOYMENT VERIFICATION

### Checklist:

```bash
# 1. Service running
sudo systemctl is-active raven-dashboard
# Expected: active

# 2. Port listening
sudo netstat -tulpn | grep 5000
# Expected: python ... :5000

# 3. HTTP response
curl -s http://localhost:5000/api/stats | jq
# Expected: JSON with stats

# 4. WebSocket test
curl -i -N -H "Connection: Upgrade" \
     -H "Upgrade: websocket" \
     http://localhost:5000/socket.io/
# Expected: 101 Switching Protocols

# 5. Nginx (if configured)
curl -s http://localhost/api/stats
# Expected: JSON via proxy

# 6. From internet
curl -s http://your-vps-ip:5000/api/stats
# Expected: JSON accessible
```

**All pass? Deployment SUCCESS!** 🎉

---

## 🌍 MULTI-SERVER SETUP

### Setup Dashboard on Separate Server from Scanner:

**Server 1 (Scanner):**
```bash
# Run Raven X 2.0 scanner
go run main2.go urls.txt

# Sync results to dashboard server
rsync -avz --progress ResultJS/ \
      user@dashboard-server:/opt/raven-dashboard/ResultJS/
```

**Server 2 (Dashboard):**
```bash
# Dashboard running and receiving results
# No scanner needed, just monitors ResultJS folder
```

**Benefits:**
- 🎯 Separation of concerns
- 🔒 Security (dashboard exposed, scanner hidden)
- ⚡ Performance (dedicated resources)
- 📈 Scalability (multiple scanners → one dashboard)

---

## 🎉 CONGRATULATIONS!

Your Raven X 2.0 dashboard is now running on VPS!

### Final Access Points:
```
Direct:  http://your-vps-ip:5000
Nginx:   http://your-domain.com
HTTPS:   https://your-domain.com (if SSL configured)
```

### Next Steps:
1. ✅ Upload scanner results to ResultJS/
2. ✅ Watch real-time updates
3. ✅ Share URL with team
4. ✅ Setup backups
5. ✅ Configure monitoring

**Enjoy your futuristic dashboard in the cloud!** ☁️✨🚀

---

**Created:** 2026-01-23  
**Version:** VPS Edition 1.0  
**Status:** ✅ PRODUCTION READY
