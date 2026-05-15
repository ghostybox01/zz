#!/bin/bash
###############################################################################
# RAVEN X 2.0 - Linux/macOS Launcher
# Run this to start everything automatically: bash START.sh
###############################################################################

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

echo ""
echo -e "${CYAN}${BOLD}============================================================${NC}"
echo -e "${CYAN}${BOLD}         RAVEN X 2.0 - ONE-CLICK STARTER${NC}"
echo -e "${CYAN}${BOLD}============================================================${NC}"
echo ""

# Check Python
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}[ERROR] Python3 not found!${NC}"
    echo "Please install Python 3.7+"
    exit 1
fi

# Get script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Create virtual environment if not exists
if [ ! -d "venv" ]; then
    echo -e "${YELLOW}[1/2] Creating Python virtual environment...${NC}"
    python3 -m venv venv
fi

# Activate virtual environment
echo -e "${YELLOW}[2/2] Activating virtual environment...${NC}"
source venv/bin/activate

# Install/upgrade dependencies
echo ""
echo "Installing Python dependencies..."
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt

# Run the auto starter
echo ""
echo -e "${CYAN}${BOLD}============================================================${NC}"
echo -e "${CYAN}${BOLD}Starting RAVEN X 2.0...${NC}"
echo -e "${CYAN}${BOLD}============================================================${NC}"
echo ""

python3 start.py "$@"

# If we reach here, the app has stopped
echo ""
echo -e "${CYAN}${BOLD}============================================================${NC}"
echo -e "${CYAN}${BOLD}RAVEN X 2.0 stopped${NC}"
echo -e "${CYAN}${BOLD}============================================================${NC}"
echo ""
