#!/bin/bash
###############################################################################
# Install RAVEN X 2.0 as system service with auto-restart
# Run this on your control server (where dashboard runs)
###############################################################################

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}"
echo "═══════════════════════════════════════════════════════════════"
echo "  RAVEN X 2.0 - System Service Installer"
echo "═══════════════════════════════════════════════════════════════"
echo -e "${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Please run as root: sudo bash install_cron.sh${NC}"
    exit 1
fi

# Get current directory
INSTALL_DIR=$(pwd)

echo -e "${CYAN}[1/4] Creating systemd service...${NC}"

# Create systemd service file
cat > /etc/systemd/system/raven-x.service << EOF
[Unit]
Description=RAVEN X 2.0 - Automated Scanner Dashboard
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
Environment="PATH=$INSTALL_DIR/venv/bin:/usr/local/bin:/usr/bin:/bin"
ExecStart=$INSTALL_DIR/venv/bin/python $INSTALL_DIR/start.py --dashboard-only
Restart=always
RestartSec=10
StandardOutput=append:$INSTALL_DIR/raven.log
StandardError=append:$INSTALL_DIR/raven.error.log

[Install]
WantedBy=multi-user.target
EOF

echo -e "${GREEN}✓ Service file created${NC}"

echo -e "${CYAN}[2/4] Creating auto-deploy cron job...${NC}"

# Create cron script
cat > /usr/local/bin/raven-auto-deploy << 'EOF'
#!/bin/bash
# RAVEN X 2.0 - Auto Deploy Script
# Runs periodically to check for new targets and redeploy

RAVEN_DIR="$INSTALL_DIR_PLACEHOLDER"
cd "$RAVEN_DIR"

# Check if targets.txt updated
if [ "$RAVEN_DIR/targets.txt" -nt "$RAVEN_DIR/.last_deploy" ]; then
    echo "[$(date)] Targets updated, redeploying..." >> "$RAVEN_DIR/auto_deploy.log"
    
    # Activate venv and run deployment
    source "$RAVEN_DIR/venv/bin/activate"
    python -c "from ssh_manager import get_manager; m = get_manager(); result = m.deploy_full(auto_start=True); print(result)" >> "$RAVEN_DIR/auto_deploy.log" 2>&1
    
    # Update timestamp
    touch "$RAVEN_DIR/.last_deploy"
    
    echo "[$(date)] Deployment complete" >> "$RAVEN_DIR/auto_deploy.log"
fi
EOF

# Replace placeholder
sed -i "s|INSTALL_DIR_PLACEHOLDER|$INSTALL_DIR|g" /usr/local/bin/raven-auto-deploy

chmod +x /usr/local/bin/raven-auto-deploy

# Add to crontab (check every 5 minutes)
(crontab -l 2>/dev/null | grep -v "raven-auto-deploy"; echo "*/5 * * * * /usr/local/bin/raven-auto-deploy") | crontab -

echo -e "${GREEN}✓ Auto-deploy cron job created${NC}"

echo -e "${CYAN}[3/4] Enabling and starting service...${NC}"

systemctl daemon-reload
systemctl enable raven-x.service
systemctl start raven-x.service

echo -e "${GREEN}✓ Service started${NC}"

echo -e "${CYAN}[4/4] Verifying installation...${NC}"

sleep 2

if systemctl is-active --quiet raven-x.service; then
    echo -e "${GREEN}✓ Service is running${NC}"
else
    echo -e "${RED}✗ Service failed to start${NC}"
    echo "Check logs with: journalctl -u raven-x.service -f"
    exit 1
fi

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  ✅ RAVEN X 2.0 INSTALLED AS SYSTEM SERVICE${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "Service commands:"
echo "  Status:  systemctl status raven-x"
echo "  Stop:    systemctl stop raven-x"
echo "  Start:   systemctl start raven-x"
echo "  Restart: systemctl restart raven-x"
echo "  Logs:    journalctl -u raven-x -f"
echo ""
echo "Auto-deploy:"
echo "  Edit targets.txt → System auto-redeploys within 5 minutes"
echo "  Manual trigger: /usr/local/bin/raven-auto-deploy"
echo "  Deploy log: $INSTALL_DIR/auto_deploy.log"
echo ""
echo "Dashboard access:"
echo "  Local:  http://localhost:5000"
echo "  Remote: http://$(hostname -I | awk '{print $1}'):5000"
echo ""
echo -e "${CYAN}System will auto-start on reboot!${NC}"
echo ""
