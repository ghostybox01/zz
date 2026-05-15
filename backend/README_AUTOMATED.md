# 🚀 RAVEN X 2.0 - FULLY AUTOMATED

## ✨ One-Click Deployment System

Everything is **100% automated**. No manual steps required!

---

## 🎯 Quick Start (3 Steps)

### 1️⃣ **Setup Configuration** (One-time)

Edit these 3 files:

```bash
# 1. Add your VPS IPs
nano server_ips.txt
```
```
66.228.42.201
66.228.42.244
69.164.213.38
```

```bash
# 2. Configure SSH (optional, defaults are fine)
nano ssh_config.json
```
```json
{
  "ssh_key_path": "/root/ssh/1",
  "remote_user": "root",
  "work_dir": "/root/python_job",
  "batch_size": 100000
}
```

```bash
# 3. Add your target URLs
nano targets.txt
```
```
https://example.com/config.php
https://site.com/.env
https://app.com/wp-config.php
...
```

### 2️⃣ **One-Click Launch**

**Windows:**
```batch
REM Double-click this file:
START.bat

REM Or from command line:
python start.py
```

**Linux/macOS:**
```bash
# Make executable (one-time)
chmod +x START.sh

# Run
./START.sh

# Or directly
python3 start.py
```

### 3️⃣ **Done!**

The system will **automatically**:

✅ Check if Go is installed (auto-install on Linux)  
✅ Download Go dependencies  
✅ Build `raven-scanner` binary  
✅ Create deployment package  
✅ Test SSH connections  
✅ Upload scanner to all VPS  
✅ Split targets equally  
✅ Start scanning on all VPS  
✅ Launch real-time dashboard  

**Access:**
- Dashboard: http://localhost:5000
- VPS Control: http://localhost:5000/vps

---

## 🎮 Command Options

```bash
# Full auto (default)
python start.py

# Skip deployment, just run dashboard
python start.py --no-deploy

# Only dashboard (skip build and deploy)
python start.py --dashboard-only

# Force rebuild scanner even if exists
python start.py --rebuild

# Show help
python start.py --help
```

---

## 📁 File Structure

```
raven/
├── START.bat              ← Windows: Double-click to start
├── START.sh               ← Linux/macOS: Run to start
├── start.py              ← Main launcher (automated)
├── auto_setup.py         ← Auto-build scanner
├── app.py                ← Dashboard app
├── ssh_manager.py        ← VPS deployment manager
├── main.go               ← Scanner source (Go)
├── main.py               ← Scanner wrapper (Python)
├── config.json           ← Scanner config
├── ssh_config.json       ← SSH config
├── server_ips.txt        ← VPS IP list
├── targets.txt           ← URLs to scan
└── requirements.txt      ← Python dependencies
```

---

## 🔧 What Happens Behind the Scenes

### Auto Setup Flow

```
START.bat / START.sh
    │
    ├──> Create Python venv
    ├──> Install dependencies
    │
    └──> python start.py
            │
            ├──> [1/4] Auto Setup
            │       ├──> Check Go installed
            │       ├──> go mod init
            │       ├──> go get dependencies
            │       ├──> go build -o raven-scanner
            │       └──> tar -czf scanner_package.tar.gz
            │
            ├──> [2/4] Check SSH Config
            │       ├──> Verify ssh_config.json
            │       ├──> Check SSH key exists
            │       └──> Read server_ips.txt
            │
            ├──> [3/4] Auto Deploy
            │       ├──> Test SSH connections
            │       ├──> Count targets
            │       ├──> Split targets per server
            │       ├──> Upload raven-scanner binary
            │       ├──> Upload runner script
            │       ├──> Create batches
            │       └──> Start scanning
            │
            └──> [4/4] Start Dashboard
                    ├──> Initialize database
                    ├──> Start file monitor
                    ├──> Launch Flask app
                    └──> Open http://0.0.0.0:5000
```

---

## 🛠️ Prerequisites

### Required (Auto-checked)

- **Python 3.7+** (install from [python.org](https://python.org))
- **SSH Key** at `/root/ssh/1` or path in `ssh_config.json`
- **VPS servers** with root access

### Optional (Auto-installed on Linux)

- **Go 1.21+** (auto-installs on Linux, manual on Windows from [go.dev](https://go.dev))

---

## 🔐 SSH Key Setup (If Needed)

If you don't have SSH key yet:

```bash
# Generate SSH key
ssh-keygen -t rsa -b 4096 -f /root/ssh/1

# Copy to VPS (for each server)
ssh-copy-id -i /root/ssh/1.pub root@66.228.42.201
ssh-copy-id -i /root/ssh/1.pub root@66.228.42.244
```

Or use the provision script:

```bash
# Auto-provision VPS with SSH keys
python provision.py
```

---

## 📊 Dashboard Features

### Local Results Tab (/)
- Real-time credential updates
- Statistics and charts
- Recent findings
- Hit counts

### VPS Control Tab (/vps)
- Server status monitoring
- Start/Stop/Restart controls
- Live scanning progress
- Target distribution
- Results collection

---

## 🚨 Troubleshooting

### "Go not found" on Windows

**Solution:** Install Go manually
```
Download: https://go.dev/dl/go1.21.6.windows-amd64.msi
Install and restart terminal
```

### "SSH connection failed"

**Check:**
```bash
# 1. Test manual SSH
ssh -i /root/ssh/1 root@66.228.42.201

# 2. Check SSH key permissions
chmod 600 /root/ssh/1

# 3. Verify key in ssh_config.json
cat ssh_config.json
```

### "No servers reachable"

**Fix:**
```bash
# Test each server
python -c "from ssh_manager import get_manager; m = get_manager(); print(m.test_all_connections())"

# Check firewall
# Allow port 22 on VPS
```

### "targets.txt not found"

**Solution:**
```bash
# Create targets file
echo "https://example.com/config.php" > targets.txt
echo "https://site.com/.env" >> targets.txt
```

### Force clean rebuild

```bash
# Remove old builds
rm -f raven-scanner scanner_package.tar.gz

# Rebuild
python start.py --rebuild
```

---

## 🎉 Success Indicators

After running `START.bat` / `START.sh`, you should see:

```
✓ Go installed: go version go1.21.6
✓ Binary built: raven-scanner (15MB)
✓ Package created: scanner_package.tar.gz (12KB)
✓ SSH key OK: RSA
✓ 9 server(s) reachable
✓ Targets file ready: 10,000 URLs
✓ Deployed to 9/9 servers
✓ Total targets distributed: 10,000

🎉 RAVEN X 2.0 READY!

Dashboard:    http://0.0.0.0:5000
VPS Control:  http://0.0.0.0:5000/vps

Press Ctrl+C to stop
```

---

## 📈 Workflow Example

**Complete automated workflow:**

```bash
# Day 1: Initial setup (5 minutes)
1. Edit server_ips.txt → Add your VPS IPs
2. Edit targets.txt → Add URLs to scan
3. Run: START.bat (Windows) or ./START.sh (Linux)
4. Open: http://localhost:5000
5. Watch: Real-time scanning on all VPS!

# Day 2+: Just run
1. Update targets.txt with new URLs
2. Run: START.bat or ./START.sh
3. System auto-deploys and starts scanning
```

**No manual SSH, no manual builds, no manual deployments!**

---

## 💡 Pro Tips

### 1. Auto-restart on reboot (Linux VPS)

```bash
# Add to crontab
@reboot cd /root/raven && ./START.sh --no-deploy
```

### 2. Background mode

```bash
# Run in background
nohup ./START.sh > raven.log 2>&1 &

# Check status
tail -f raven.log
```

### 3. Remote access

```bash
# Access from anywhere
ssh -L 5000:localhost:5000 root@your-vps-ip

# Then open in browser
http://localhost:5000
```

### 4. Update scanner code

```bash
# Edit main.go
nano main.go

# Rebuild and redeploy
python start.py --rebuild
```

---

## 🔄 Update System

```bash
# Pull latest code
git pull

# Rebuild everything
python start.py --rebuild

# Redeploy to all VPS
python -c "from ssh_manager import get_manager; m = get_manager(); m.deploy_full(auto_start=True)"
```

---

## 📞 Support

**Issues?**
1. Check logs: `tail -f raven.log`
2. Test SSH: `ssh -i /root/ssh/1 root@<vps-ip>`
3. Verify files: `ls -lah raven-scanner scanner_package.tar.gz`
4. Force rebuild: `python start.py --rebuild`

**All good?**
- Dashboard: ✅
- VPS scanning: ✅
- Results flowing: ✅
- **You're ready to go!** 🚀

---

**Built with ❤️ for maximum automation**

**One command. Zero hassle. Full power.**
