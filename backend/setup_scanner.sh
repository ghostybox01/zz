#!/bin/bash
# Setup Raven Go Scanner for Multi-VPS Deployment
# Run this on your Raven server: /root/ravengui

set -e

echo "════════════════════════════════════════════════════"
echo "  🚀 Raven Go Scanner - Deployment Setup"
echo "════════════════════════════════════════════════════"
echo ""

# Step 1: Check if we're in ravengui folder
if [ ! -f "app.py" ]; then
    echo "❌ Error: Run this from /root/ravengui directory"
    exit 1
fi

echo "[1/6] Checking if main.go exists..."
if [ ! -f "main.go" ]; then
    echo "❌ main.go not found!"
    echo "Please upload main.go to /root/ravengui/"
    exit 1
fi
echo "  ✅ main.go found"

# Step 2: Check if config.json exists
echo ""
echo "[2/6] Checking config.json..."
if [ ! -f "config.json" ]; then
    echo "❌ config.json not found!"
    exit 1
fi
echo "  ✅ config.json found"

# Step 3: Install Go if needed
echo ""
echo "[3/6] Checking Go installation..."
if ! command -v go &> /dev/null; then
    echo "  Go not installed. Installing..."
    wget -q https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
    rm go1.21.6.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
fi
echo "  ✅ Go installed: $(go version)"

# Step 4: Download dependencies
echo ""
echo "[4/6] Installing Go dependencies..."
go mod init raven-scanner 2>/dev/null || true
go get github.com/aws/aws-sdk-go-v2/aws
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/credentials
go get github.com/aws/aws-sdk-go-v2/service/iam
go get github.com/aws/aws-sdk-go-v2/service/s3
go get github.com/aws/aws-sdk-go-v2/service/servicequotas
go get github.com/aws/aws-sdk-go-v2/service/ses
go get github.com/aws/aws-sdk-go-v2/service/sesv2
go get github.com/aws/aws-sdk-go-v2/service/sns
go get github.com/aws/aws-sdk-go-v2/service/sts
go get github.com/pterm/pterm
echo "  ✅ Dependencies installed"

# Step 5: Build the binary
echo ""
echo "[5/6] Building Linux binary..."
GOOS=linux GOARCH=amd64 go build -o raven-scanner main.go
if [ ! -f "raven-scanner" ]; then
    echo "❌ Build failed!"
    exit 1
fi
chmod +x raven-scanner
echo "  ✅ Binary built: raven-scanner"

# Step 6: Create deployment package
echo ""
echo "[6/6] Creating scanner_package.tar.gz..."

# Create package directory
mkdir -p scanner_package
cp raven-scanner scanner_package/
cp config.json scanner_package/

# Create runner script for VPS
cat > scanner_package/run.sh << 'RUNSCRIPT'
#!/bin/bash
# Raven Scanner Runner
# This script runs on each VPS

cd /root/python_job

# Check if targets file exists
if [ ! -f "targets.txt" ]; then
    echo "❌ targets.txt not found!"
    exit 1
fi

# Make scanner executable
chmod +x raven-scanner

# Create Result directory
mkdir -p Result

# Run scanner
echo "🚀 Starting Raven Scanner..."
./raven-scanner targets.txt

echo "✅ Scan complete! Results in Result/ folder"
RUNSCRIPT

chmod +x scanner_package/run.sh

# Create the tar.gz
tar -czf scanner_package.tar.gz -C scanner_package .

# Cleanup
rm -rf scanner_package

# Verify
if [ -f "scanner_package.tar.gz" ]; then
    SIZE=$(ls -lh scanner_package.tar.gz | awk '{print $5}')
    echo "  ✅ Package created: scanner_package.tar.gz ($SIZE)"
else
    echo "❌ Package creation failed!"
    exit 1
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  ✅ SETUP COMPLETE!"
echo "════════════════════════════════════════════════════"
echo ""
echo "📦 Package contents:"
tar -tzf scanner_package.tar.gz
echo ""
echo "🎯 Next steps:"
echo "  1. Open Raven UI: http://$(hostname -I | awk '{print $1}'):5000"
echo "  2. Upload your target URLs"
echo "  3. Click 'Deploy' - it will:"
echo "     • Upload scanner_package.tar.gz to all VPS"
echo "     • Extract and run on each VPS"
echo "     • Start scanning in parallel!"
echo ""
echo "🚀 Ready to deploy!"
echo ""
