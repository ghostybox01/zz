#!/bin/bash
# Raven Go Scanner - One-Command Setup
# This makes your main.go work with Raven by creating a Python wrapper

set -e

echo "════════════════════════════════════════════════════"
echo "  🔧 Raven Go Scanner - Auto Fix"
echo "════════════════════════════════════════════════════"
echo ""

# Check location
if [ ! -f "app.py" ]; then
    echo "❌ Run this from /root/ravengui directory"
    exit 1
fi

echo "[1/5] Checking files..."
if [ ! -f "main.go" ]; then
    echo "❌ main.go not found! Upload it first."
    exit 1
fi
if [ ! -f "config.json" ]; then
    echo "❌ config.json not found!"
    exit 1
fi
echo "  ✅ main.go and config.json found"

echo ""
echo "[2/5] Installing Go..."
if ! command -v go &> /dev/null; then
    wget -q https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
    rm go1.21.6.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
fi
export PATH=$PATH:/usr/local/go/bin
echo "  ✅ Go: $(go version)"

echo ""
echo "[3/5] Building raven-scanner..."
go mod init raven-scanner 2>/dev/null || true
go get github.com/aws/aws-sdk-go-v2/aws 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/config 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/credentials 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/service/iam 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/service/s3 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/service/servicequotas 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/service/ses 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/service/sesv2 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/service/sns 2>/dev/null
go get github.com/aws/aws-sdk-go-v2/service/sts 2>/dev/null
go get github.com/pterm/pterm 2>/dev/null

GOOS=linux GOARCH=amd64 go build -o raven-scanner main.go
chmod +x raven-scanner
echo "  ✅ Binary: raven-scanner ($(ls -lh raven-scanner | awk '{print $5}'))"

echo ""
echo "[4/5] Creating Python wrapper..."
cat > main.py << 'WRAPPER'
#!/usr/bin/env python3
"""Raven Go Scanner Wrapper"""
import subprocess, sys, os

def main():
    targets = sys.argv[1] if len(sys.argv) > 1 else "targets.txt"
    
    print(f"🚀 Raven Go Scanner")
    print(f"📋 Targets: {targets}")
    print("─" * 60)
    
    if not os.path.exists(targets):
        print(f"❌ Targets file not found: {targets}")
        sys.exit(1)
    
    if not os.path.exists("raven-scanner"):
        print("❌ raven-scanner binary not found!")
        sys.exit(1)
    
    os.chmod("raven-scanner", 0o755)
    
    try:
        subprocess.run(["./raven-scanner", targets], check=True)
        print("─" * 60)
        print("✅ Scan complete!")
    except subprocess.CalledProcessError as e:
        print(f"❌ Failed: {e.returncode}")
        sys.exit(e.returncode)
    except Exception as e:
        print(f"❌ Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
WRAPPER

chmod +x main.py
echo "  ✅ main.py created"

echo ""
echo "[5/5] Verifying deployment files..."
echo "  Files ready for Raven:"
echo "    ├── main.py ........... $([ -f main.py ] && echo '✅' || echo '❌') (wrapper)"
echo "    ├── raven-scanner ..... $([ -f raven-scanner ] && echo '✅' || echo '❌') (binary)"
echo "    └── config.json ....... $([ -f config.json ] && echo '✅' || echo '❌') (config)"

echo ""
echo "════════════════════════════════════════════════════"
echo "  ✅ SETUP COMPLETE!"
echo "════════════════════════════════════════════════════"
echo ""
echo "🎯 Your Go scanner is ready!"
echo ""
echo "Next steps:"
echo "  1. Open: http://$(hostname -I | awk '{print $1}'):5000"
echo "  2. Upload your target URLs"
echo "  3. Test servers (should all be green)"
echo "  4. Click 'Start Deployment'"
echo ""
echo "Raven will deploy:"
echo "  • main.py → Calls your Go scanner"
echo "  • raven-scanner → Your compiled scanner"
echo "  • config.json → Scanner configuration"
echo ""
echo "🚀 Ready to scan!"
echo ""
