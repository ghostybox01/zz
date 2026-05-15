# 📁 RAVEN X 2.0 - Files Explained

## 🚀 **LAUNCHER FILES** (Start Here!)

### **START.bat** ⭐ WINDOWS USERS START HERE
**What:** Windows batch file launcher  
**Purpose:** One-click startup for Windows  
**Usage:** Double-click this file  
**What it does:**
- Creates Python virtual environment
- Installs dependencies
- Runs `start.py`

### **START.sh** ⭐ LINUX/MAC USERS START HERE
**What:** Bash script launcher  
**Purpose:** One-click startup for Linux/macOS  
**Usage:** `chmod +x START.sh && ./START.sh`  
**What it does:**
- Creates Python virtual environment
- Installs dependencies
- Runs `start.py`

### **START.ps1**
**What:** PowerShell script launcher  
**Purpose:** Alternative Windows launcher with better error handling  
**Usage:** Right-click → "Run with PowerShell"

---

## 🎮 **CORE AUTOMATION FILES**

### **start.py** 🎯 MAIN ORCHESTRATOR
**What:** Master automation script  
**Purpose:** Coordinates entire workflow  
**What it does:**
1. Runs `auto_setup.py` to build scanner
2. Checks SSH configuration
3. Auto-deploys to all VPS
4. Starts dashboard

**Command options:**
```bash
python start.py                    # Full auto
python start.py --no-deploy        # Skip deployment
python start.py --dashboard-only   # Only dashboard
python start.py --rebuild          # Force rebuild
python start.py --help             # Show help
```

### **auto_setup.py** 🔨 BUILD AUTOMATION
**What:** Automated scanner builder  
**Purpose:** Builds Go binary and creates deployment package  
**What it does:**
1. Checks if Go installed (auto-installs on Linux)
2. Initializes Go module
3. Downloads Go dependencies
4. Builds `raven-scanner` Linux binary
5. Creates `scanner_package.tar.gz`

**Runs automatically via `start.py`**

### **install_cron.sh** ⏰ SYSTEM SERVICE
**What:** Systemd service installer  
**Purpose:** Install as system service with auto-restart  
**Usage:** `sudo bash install_cron.sh`  
**What it does:**
- Creates systemd service
- Auto-starts on boot
- Auto-redeploys when targets.txt changes
- Runs every 5 minutes

---

## 🖥️ **CORE APPLICATION FILES**

### **app.py** 📊 DASHBOARD
**What:** Flask web application  
**Purpose:** Real-time dashboard and VPS control panel  
**Features:**
- Real-time credential monitoring
- WebSocket updates
- VPS status display
- Start/Stop/Restart controls
- Results collection

**Access:** http://localhost:5000

### **ssh_manager.py** 🔐 VPS MANAGER
**What:** SSH deployment and control manager  
**Purpose:** Manage remote VPS servers  
**Features:**
- SSH connection testing
- File upload/download
- Remote command execution
- Target distribution
- Auto-upload scanner binary
- Progress monitoring

**Modified:** Now auto-uploads `raven-scanner` binary!

---

## 🔧 **SCANNER FILES**

### **main.go** 💻 SCANNER SOURCE
**What:** Go source code for scanner  
**Purpose:** High-performance credential scanner  
**Compiled to:** `raven-scanner` (Linux binary)

### **main.py** 🐍 SCANNER WRAPPER
**What:** Python wrapper for Go scanner  
**Purpose:** Bridge between Python system and Go binary  
**What it does:**
- Checks if `raven-scanner` exists
- Makes it executable
- Runs scanner on targets
- Handles errors

### **batch_runner.sh** 📦 BATCH PROCESSOR
**What:** Bash script for batch processing  
**Purpose:** Process target batches one by one  
**What it does:**
- Finds batch files (batch_*.txt)
- Runs scanner on each batch
- Marks batches as done/failed
- Logs progress

---

## ⚙️ **CONFIGURATION FILES**

### **config.json** 🎛️ SCANNER CONFIG
**What:** Scanner configuration  
**Purpose:** Configure scanner behavior  
**Contains:**
- AWS regions to test
- Timeout settings
- Validation options

### **ssh_config.json** 🔐 SSH CONFIG
**What:** SSH deployment configuration  
**Purpose:** Configure SSH connections and deployment  
**Contains:**
```json
{
  "ssh_key_path": "/root/ssh/1",
  "remote_user": "root",
  "work_dir": "/root/python_job",
  "batch_size": 100000,
  "ssh_timeout": 10
}
```

### **server_ips.txt** 🌐 VPS LIST
**What:** List of VPS server IPs  
**Purpose:** Define which servers to deploy to  
**Format:**
```
66.228.42.201
66.228.42.244
69.164.213.38
```

### **targets.txt** 🎯 SCAN TARGETS
**What:** List of URLs to scan  
**Purpose:** Define what to scan  
**Format:**
```
https://example.com/config.php
https://site.com/.env
https://app.com/.git/config
```

---

## 📦 **DEPENDENCY FILES**

### **requirements.txt** 📋 PYTHON DEPS
**What:** Python package requirements  
**Purpose:** Define Python dependencies  
**Contains:**
- Flask + Flask-SocketIO
- Paramiko (SSH)
- Cryptography
- Eventlet

**Auto-installed by START scripts**

### **requirements_full.txt** 📋 FULL DEPS
**What:** Complete dependency list with versions  
**Purpose:** Pin exact versions for stability

---

## 📖 **DOCUMENTATION FILES**

### **README_AUTOMATED.md** 📘 FULL GUIDE
**What:** Complete documentation  
**Purpose:** Detailed setup and usage guide  
**Contains:**
- Full setup instructions
- Troubleshooting
- Advanced usage
- System architecture

### **QUICKSTART.txt** ⚡ QUICK START
**What:** 2-minute quick start guide  
**Purpose:** Get started immediately  
**Contains:**
- 3-step setup
- Common commands
- Success indicators
- Troubleshooting

### **FILES_EXPLAINED.md** 📁 THIS FILE
**What:** File structure explanation  
**Purpose:** Understand what each file does

### **VPS_DEPLOYMENT.md** 🚀 DEPLOYMENT GUIDE
**What:** VPS deployment documentation  
**Purpose:** Manual deployment instructions  
**Note:** Now mostly automated by `start.py`!

---

## 🗄️ **DATABASE & RESULTS**

### **raven_results.db** 💾 DATABASE
**What:** SQLite database  
**Purpose:** Store credentials and statistics  
**Auto-created:** Yes, by `app.py`

### **ResultJS/** 📂 RESULTS FOLDER
**What:** Directory for scanner results  
**Purpose:** Store result files  
**Files:**
- `aws_valid.txt` - Valid AWS credentials
- `valid_github_token.txt` - GitHub tokens
- `smtp_valid.txt` - SMTP servers
- etc.

**Auto-monitored:** Dashboard watches this folder

---

## 🏗️ **BUILD ARTIFACTS**

### **raven-scanner** ⚡ SCANNER BINARY
**What:** Compiled Go binary (Linux)  
**Purpose:** Fast credential scanner for VPS  
**Created by:** `auto_setup.py`  
**Size:** ~15MB

### **scanner_package.tar.gz** 📦 DEPLOY PACKAGE
**What:** Deployment archive  
**Purpose:** Package for VPS deployment  
**Created by:** `auto_setup.py`  
**Contains:**
- `raven-scanner` binary
- `config.json`
- `run.sh` script

### **venv/** 🐍 VIRTUAL ENVIRONMENT
**What:** Python virtual environment  
**Purpose:** Isolated Python dependencies  
**Created by:** START scripts

---

## 🔍 **GENERATED FILES**

### **.last_deploy**
**What:** Timestamp marker  
**Purpose:** Track last deployment time  
**Used by:** Auto-deploy cron job

### **auto_deploy.log**
**What:** Auto-deployment log  
**Purpose:** Log automatic deployments  
**Created by:** Cron job

### **raven.log**
**What:** Dashboard log  
**Purpose:** Application logs  
**Created by:** systemd service

### **go.mod / go.sum**
**What:** Go module files  
**Purpose:** Go dependency management  
**Created by:** `auto_setup.py`

---

## 📊 **FILE WORKFLOW**

```
User runs START.bat/START.sh
    │
    ├──> Creates venv/
    ├──> Installs requirements.txt
    │
    └──> Runs start.py
            │
            ├──> Runs auto_setup.py
            │       ├──> Checks main.go
            │       ├──> Builds raven-scanner
            │       └──> Creates scanner_package.tar.gz
            │
            ├──> Checks ssh_config.json
            ├──> Reads server_ips.txt
            ├──> Reads targets.txt
            │
            ├──> ssh_manager.py deploys:
            │       ├──> Uploads raven-scanner to VPS
            │       ├──> Uploads main.py to VPS
            │       ├──> Uploads batch_runner.sh to VPS
            │       ├──> Splits targets.txt per VPS
            │       └──> Starts scanning
            │
            └──> Runs app.py
                    ├──> Creates raven_results.db
                    ├──> Monitors ResultJS/
                    └──> Serves dashboard at :5000
```

---

## 🎯 **USAGE PATTERNS**

### **First Time Setup**
```
1. Edit: server_ips.txt (add VPS IPs)
2. Edit: targets.txt (add URLs)
3. Run: START.bat or START.sh
4. Access: http://localhost:5000
```

### **Daily Usage**
```
1. Update: targets.txt (new URLs)
2. Run: START.bat or START.sh
3. System auto-rebuilds and redeploys
```

### **Production Deployment**
```
1. Run: sudo bash install_cron.sh
2. Service auto-starts on boot
3. Auto-redeploys when targets change
4. Monitor: systemctl status raven-x
```

---

## 🔄 **FILE DEPENDENCIES**

```
START.bat/START.sh
    └──> start.py (required)
            ├──> auto_setup.py (required)
            │       └──> main.go (required)
            ├──> ssh_manager.py (required)
            │       ├──> ssh_config.json (required)
            │       └──> server_ips.txt (required)
            └──> app.py (required)
                    ├──> templates/dashboard.html
                    └──> templates/vps.html
```

**Minimal Required Files:**
- START.bat or START.sh
- start.py
- auto_setup.py
- app.py
- ssh_manager.py
- main.go
- ssh_config.json
- server_ips.txt
- targets.txt

---

## 💡 **KEY IMPROVEMENTS**

### **What's New (Automation)**

**Before:**
```bash
# Manual steps
1. bash setup_scanner.sh
2. Check if built
3. ssh to each VPS
4. Upload files manually
5. Run commands on each VPS
6. Start dashboard
7. Monitor manually
```

**Now:**
```bash
# One command
python start.py

# Or double-click
START.bat
```

**Automated:**
- ✅ Binary building
- ✅ Package creation
- ✅ SSH testing
- ✅ File uploads
- ✅ Target distribution
- ✅ Scanner starting
- ✅ Dashboard launching
- ✅ Auto-restart on failure

---

## 🎉 **SUMMARY**

**To Start:** Just run `START.bat` (Windows) or `START.sh` (Linux)

**Everything else is automatic!**

**Files you edit:**
- `server_ips.txt` - Your VPS IPs
- `targets.txt` - URLs to scan
- `ssh_config.json` - SSH settings (optional)

**Files you never touch:**
- Everything else runs automatically
- No manual builds
- No manual deployments
- No manual SSH commands

**🚀 One click. Full power.**
