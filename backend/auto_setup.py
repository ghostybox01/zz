#!/usr/bin/env python3
"""
RAVEN AUTO SETUP - Fully Automated Build & Deploy
Auto-detects and builds scanner binary, creates package, and prepares deployment
"""

import subprocess
import os
import sys
import platform
import shutil
from pathlib import Path

class Colors:
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    CYAN = '\033[96m'
    BOLD = '\033[1m'
    END = '\033[0m'

def log_info(msg):
    print(f"{Colors.CYAN}[INFO]{Colors.END} {msg}")

def log_success(msg):
    print(f"{Colors.GREEN}[✓]{Colors.END} {msg}")

def log_warning(msg):
    print(f"{Colors.YELLOW}[!]{Colors.END} {msg}")

def log_error(msg):
    print(f"{Colors.RED}[✗]{Colors.END} {msg}")

def check_command(cmd):
    """Check if command exists"""
    return shutil.which(cmd) is not None

def run_command(cmd, shell=False, cwd=None, check=True):
    """Run shell command"""
    try:
        result = subprocess.run(
            cmd if isinstance(cmd, list) else cmd,
            shell=shell,
            cwd=cwd,
            capture_output=True,
            text=True,
            check=check
        )
        return result.returncode == 0, result.stdout, result.stderr
    except subprocess.CalledProcessError as e:
        return False, e.stdout, e.stderr
    except Exception as e:
        return False, "", str(e)

def install_go():
    """Install Go if not present"""
    if check_command('go'):
        success, stdout, _ = run_command(['go', 'version'])
        if success:
            log_success(f"Go already installed: {stdout.strip()}")
            return True
    
    log_warning("Go not found. Installing Go...")
    
    system = platform.system().lower()
    
    if system == 'linux':
        log_info("Downloading Go for Linux...")
        commands = [
            'wget -q https://go.dev/dl/go1.21.6.linux-amd64.tar.gz -O /tmp/go.tar.gz',
            'sudo rm -rf /usr/local/go',
            'sudo tar -C /usr/local -xzf /tmp/go.tar.gz',
            'rm /tmp/go.tar.gz'
        ]
        
        for cmd in commands:
            success, _, stderr = run_command(cmd, shell=True)
            if not success:
                log_error(f"Failed to install Go: {stderr}")
                return False
        
        # Add to PATH
        go_path = '/usr/local/go/bin'
        os.environ['PATH'] = f"{go_path}:{os.environ.get('PATH', '')}"
        
        log_success("Go installed successfully!")
        return True
    
    elif system == 'windows':
        log_error("Go not installed. Please install from: https://go.dev/dl/")
        log_info("Download: go1.21.6.windows-amd64.msi")
        log_info("After install, restart terminal and run this script again.")
        return False
    
    else:
        log_error(f"Unsupported OS: {system}")
        log_info("Please install Go manually from: https://go.dev/dl/")
        return False

def build_scanner():
    """Build reconx-scanner binary"""
    log_info("Building reconx-scanner binary...")
    
    # Check if main.go exists
    if not os.path.exists('main.go'):
        log_error("main.go not found!")
        return False
    
    # Initialize Go module
    log_info("Initializing Go module...")
    run_command(['go', 'mod', 'init', 'reconx-scanner'], check=False)
    
    # Install dependencies
    log_info("Installing Go dependencies...")
    dependencies = [
        'github.com/aws/aws-sdk-go-v2/aws',
        'github.com/aws/aws-sdk-go-v2/config',
        'github.com/aws/aws-sdk-go-v2/credentials',
        'github.com/aws/aws-sdk-go-v2/service/iam',
        'github.com/aws/aws-sdk-go-v2/service/s3',
        'github.com/aws/aws-sdk-go-v2/service/servicequotas',
        'github.com/aws/aws-sdk-go-v2/service/ses',
        'github.com/aws/aws-sdk-go-v2/service/sesv2',
        'github.com/aws/aws-sdk-go-v2/service/sns',
        'github.com/aws/aws-sdk-go-v2/service/sts',
        'github.com/pterm/pterm',
    ]
    
    for dep in dependencies:
        log_info(f"  Getting {dep}...")
        run_command(['go', 'get', dep], check=False)
    
    run_command(['go', 'mod', 'tidy'], check=False)
    
    # Build for Linux (since VPS are Linux)
    log_info("Building Linux binary...")
    env = os.environ.copy()
    env['GOOS'] = 'linux'
    env['GOARCH'] = 'amd64'
    
    success, stdout, stderr = run_command(
        ['go', 'build', '-o', 'reconx-scanner', 'main.go'],
        check=False
    )
    
    if not success or not os.path.exists('reconx-scanner'):
        log_error(f"Build failed: {stderr}")
        return False
    
    # Make executable
    try:
        os.chmod('reconx-scanner', 0o755)
    except:
        pass
    
    size = os.path.getsize('reconx-scanner')
    log_success(f"Binary built: reconx-scanner ({size // 1024 // 1024}MB)")
    return True

def create_deployment_package():
    """Create scanner_package.tar.gz"""
    log_info("Creating deployment package...")
    
    # Check required files
    required = ['reconx-scanner', 'config.json']
    for file in required:
        if not os.path.exists(file):
            log_error(f"Required file missing: {file}")
            return False
    
    # Create temp directory
    pkg_dir = 'scanner_package_tmp'
    os.makedirs(pkg_dir, exist_ok=True)
    
    try:
        # Copy files
        shutil.copy2('reconx-scanner', pkg_dir)
        shutil.copy2('config.json', pkg_dir)
        
        # Copy or create runner script
        if os.path.exists('batch_runner.sh'):
            shutil.copy2('batch_runner.sh', os.path.join(pkg_dir, 'run.sh'))
        else:
            # Create minimal runner
            with open(os.path.join(pkg_dir, 'run.sh'), 'w') as f:
                f.write('''#!/bin/bash
cd /root/python_job
chmod +x reconx-scanner
mkdir -p Result
echo "Starting Raven Scanner..."
./reconx-scanner targets.txt
echo "Scan complete!"
''')
        
        # Make run.sh executable
        try:
            os.chmod(os.path.join(pkg_dir, 'run.sh'), 0o755)
        except:
            pass
        
        # Create tar.gz
        import tarfile
        with tarfile.open('scanner_package.tar.gz', 'w:gz') as tar:
            for item in os.listdir(pkg_dir):
                tar.add(os.path.join(pkg_dir, item), arcname=item)
        
        # Cleanup
        shutil.rmtree(pkg_dir)
        
        size = os.path.getsize('scanner_package.tar.gz')
        log_success(f"Package created: scanner_package.tar.gz ({size // 1024}KB)")
        return True
    
    except Exception as e:
        log_error(f"Failed to create package: {e}")
        shutil.rmtree(pkg_dir, ignore_errors=True)
        return False

def verify_setup():
    """Verify all required files exist"""
    log_info("Verifying setup...")
    
    required_files = {
        'reconx-scanner': 'Scanner binary',
        'scanner_package.tar.gz': 'Deployment package',
        'config.json': 'Configuration',
        'ssh_config.json': 'SSH configuration',
        'server_ips.txt': 'Server list',
        'main.py': 'Python wrapper',
        'app.py': 'Dashboard app',
        'ssh_manager.py': 'SSH manager'
    }
    
    all_ok = True
    for file, desc in required_files.items():
        if os.path.exists(file):
            log_success(f"{desc}: {file}")
        else:
            log_warning(f"{desc}: {file} (missing)")
            if file in ['reconx-scanner', 'scanner_package.tar.gz']:
                all_ok = False
    
    return all_ok

def auto_setup():
    """Main automated setup"""
    print(f"\n{Colors.BOLD}{'='*70}{Colors.END}")
    print(f"{Colors.BOLD}{Colors.CYAN}🚀 RAVEN AUTO SETUP - Fully Automated{Colors.END}")
    print(f"{Colors.BOLD}{'='*70}{Colors.END}\n")
    
    # Check if already built
    if os.path.exists('reconx-scanner') and os.path.exists('scanner_package.tar.gz'):
        log_info("Scanner already built!")
        if verify_setup():
            log_success("All files ready! Skipping build.")
            return True
        else:
            log_warning("Some files missing, rebuilding...")
    
    # Step 1: Check/Install Go
    log_info("Step 1/3: Checking Go installation...")
    if not install_go():
        return False
    
    # Step 2: Build scanner
    log_info("Step 2/3: Building scanner binary...")
    if not build_scanner():
        return False
    
    # Step 3: Create package
    log_info("Step 3/3: Creating deployment package...")
    if not create_deployment_package():
        return False
    
    # Verify everything
    print(f"\n{Colors.BOLD}{'='*70}{Colors.END}")
    if verify_setup():
        print(f"{Colors.GREEN}{Colors.BOLD}✅ AUTO SETUP COMPLETE!{Colors.END}\n")
        log_success("Ready to deploy to VPS!")
        return True
    else:
        log_error("Setup incomplete. Please check errors above.")
        return False

if __name__ == '__main__':
    try:
        success = auto_setup()
        sys.exit(0 if success else 1)
    except KeyboardInterrupt:
        print(f"\n{Colors.YELLOW}Setup interrupted by user{Colors.END}")
        sys.exit(1)
    except Exception as e:
        log_error(f"Unexpected error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
