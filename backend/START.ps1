# RAVEN X 2.0 - PowerShell Launcher for Windows
# Right-click and "Run with PowerShell" or run: powershell -ExecutionPolicy Bypass -File START.ps1

$ErrorActionPreference = "Stop"

# Colors
function Write-Success { param($msg) Write-Host "[✓] $msg" -ForegroundColor Green }
function Write-Info { param($msg) Write-Host "[→] $msg" -ForegroundColor Cyan }
function Write-Warning { param($msg) Write-Host "[!] $msg" -ForegroundColor Yellow }
function Write-Error { param($msg) Write-Host "[✗] $msg" -ForegroundColor Red }

# Banner
Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "         RAVEN X 2.0 - ONE-CLICK STARTER" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""

# Check Python
try {
    $pythonVersion = python --version 2>&1
    Write-Success "Python found: $pythonVersion"
} catch {
    Write-Error "Python not found!"
    Write-Info "Install Python 3.7+ from https://python.org"
    Write-Host ""
    Read-Host "Press Enter to exit"
    exit 1
}

# Change to script directory
Set-Location -Path $PSScriptRoot

# Check/Create virtual environment
if (-not (Test-Path "venv")) {
    Write-Info "[1/2] Creating Python virtual environment..."
    python -m venv venv
    if (-not $?) {
        Write-Error "Failed to create virtual environment"
        Read-Host "Press Enter to exit"
        exit 1
    }
    Write-Success "Virtual environment created"
}

# Activate virtual environment
Write-Info "[2/2] Activating virtual environment..."
& "venv\Scripts\Activate.ps1"

# Install dependencies
Write-Host ""
Write-Info "Installing Python dependencies..."
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt

# Run auto starter
Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "Starting RAVEN X 2.0..." -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""

try {
    python start.py $args
} catch {
    Write-Error "Failed to start RAVEN X 2.0"
    Write-Host $_.Exception.Message -ForegroundColor Red
}

# Cleanup
Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "RAVEN X 2.0 stopped" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""

Read-Host "Press Enter to exit"
