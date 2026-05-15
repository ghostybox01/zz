#!/usr/bin/env python3
"""
RAVEN X 2.0 - ONE-CLICK STARTER
Fully automated: Build → Deploy → Run Dashboard
"""

import os
import sys
import time
import subprocess
from pathlib import Path

class Colors:
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    CYAN = '\033[96m'
    MAGENTA = '\033[95m'
    BOLD = '\033[1m'
    END = '\033[0m'

def banner():
    print(f"""
{Colors.CYAN}{Colors.BOLD}
╔═══════════════════════════════════════════════════════════════════╗
║                   RAVEN X 2.0 - AUTO LAUNCHER                     ║
║              Build → Deploy → Monitor (Fully Automated)           ║
╚═══════════════════════════════════════════════════════════════════╝
{Colors.END}
""")

def log_step(step, total, msg):
    print(f"\n{Colors.BOLD}{Colors.CYAN}[{step}/{total}]{Colors.END} {msg}")

def log_success(msg):
    print(f"{Colors.GREEN}✓{Colors.END} {msg}")

def log_error(msg):
    print(f"{Colors.RED}✗{Colors.END} {msg}")

def log_info(msg):
    print(f"{Colors.CYAN}→{Colors.END} {msg}")

def run_auto_setup():
    """Run auto setup to build scanner"""
    log_step(1, 4, "Auto Setup - Building Scanner Binary")
    
    # Check if auto_setup.py exists
    if not os.path.exists('auto_setup.py'):
        log_error("auto_setup.py not found!")
        return False
    
    # Run auto setup
    try:
        result = subprocess.run(
            [sys.executable, 'auto_setup.py'],
            check=True,
            text=True
        )
        log_success("Scanner built and packaged!")
        return True
    except subprocess.CalledProcessError:
        log_error("Auto setup failed!")
        return False

def check_ssh_config():
    """Check SSH configuration"""
    log_step(2, 4, "Checking SSH Configuration")
    
    required_files = {
        'ssh_config.json': 'SSH configuration',
        'server_ips.txt': 'Server list',
    }
    
    all_ok = True
    for file, desc in required_files.items():
        if os.path.exists(file):
            log_success(f"{desc}: {file}")
        else:
            log_error(f"{desc} not found: {file}")
            all_ok = False
    
    # Check SSH key
    try:
        import json
        with open('ssh_config.json', 'r') as f:
            config = json.load(f)
            ssh_key = config.get('ssh_key_path', '/root/ssh/1')
            
        if os.path.exists(ssh_key):
            log_success(f"SSH key found: {ssh_key}")
        else:
            log_error(f"SSH key not found: {ssh_key}")
            log_info("Generate SSH key with: ssh-keygen -t rsa -b 4096 -f /root/ssh/1")
            all_ok = False
    except:
        pass
    
    # Check servers
    try:
        with open('server_ips.txt', 'r') as f:
            servers = [line.strip() for line in f if line.strip() and not line.startswith('#')]
        
        if servers:
            log_success(f"Found {len(servers)} server(s)")
            for i, ip in enumerate(servers[:3], 1):
                log_info(f"  {i}. {ip}")
            if len(servers) > 3:
                log_info(f"  ... and {len(servers) - 3} more")
        else:
            log_error("No servers configured in server_ips.txt")
            all_ok = False
    except:
        pass
    
    return all_ok

def auto_deploy():
    """Automatically deploy to all VPS"""
    log_step(3, 4, "Auto Deploying to VPS")
    
    try:
        # Import SSH manager
        from ssh_manager import get_manager
        
        manager = get_manager()
        
        # Test SSH key
        log_info("Testing SSH key...")
        key_test = manager.quick_ssh_test()
        if not key_test.get('success'):
            log_error(f"SSH key error: {key_test.get('error')}")
            return False
        log_success(f"SSH key OK: {key_test.get('key_type')}")
        
        # Test connections
        log_info("Testing server connections...")
        conn_results = manager.test_all_connections()
        
        if not conn_results['working']:
            log_error("No servers reachable!")
            for detail in conn_results['details']:
                if not detail['success']:
                    log_error(f"  {detail['ip']}: {detail.get('error', 'Failed')}")
            return False
        
        log_success(f"{len(conn_results['working'])} server(s) reachable")
        for ip in conn_results['working']:
            log_info(f"  ✓ {ip}")
        
        if conn_results['failed']:
            log_error(f"{len(conn_results['failed'])} server(s) unreachable:")
            for ip in conn_results['failed']:
                log_error(f"  ✗ {ip}")
        
        # Check if targets file exists
        target_file = 'targets.txt'
        if not os.path.exists(target_file):
            log_error(f"Targets file not found: {target_file}")
            log_info("Create targets.txt with URLs to scan (one per line)")
            return False
        
        with open(target_file, 'r') as f:
            target_count = sum(1 for line in f if line.strip())
        
        if target_count == 0:
            log_error("targets.txt is empty!")
            return False
        
        log_success(f"Targets file ready: {target_count:,} URLs")
        
        # Deploy
        log_info("Deploying to all servers...")
        print(f"\n{Colors.YELLOW}This may take a few minutes...{Colors.END}\n")
        
        result = manager.deploy_full(
            target_file=target_file,
            scanner_file='main.py',
            runner_file='batch_runner.sh',
            auto_start=True
        )
        
        if result['success']:
            log_success(f"Deployed to {result['deployed']}/{result['total_servers']} servers")
            log_info(f"Total targets distributed: {result['total_targets']:,}")
            return True
        else:
            log_error(f"Deployment failed: {result.get('deployed', 0)}/{result.get('total_servers', 0)} succeeded")
            return False
    
    except ImportError:
        log_error("SSH manager not available!")
        log_info("Make sure ssh_manager.py is present")
        return False
    except Exception as e:
        log_error(f"Deployment error: {e}")
        import traceback
        traceback.print_exc()
        return False

def start_dashboard():
    """Start the dashboard"""
    log_step(4, 4, "Starting Dashboard")
    
    if not os.path.exists('app.py'):
        log_error("app.py not found!")
        return False
    
    log_success("Starting Raven X 2.0 Dashboard...")
    print(f"\n{Colors.BOLD}{'='*70}{Colors.END}")
    print(f"{Colors.GREEN}{Colors.BOLD}🎉 RAVEN X 2.0 READY!{Colors.END}")
    print(f"{Colors.BOLD}{'='*70}{Colors.END}\n")
    print(f"{Colors.CYAN}Dashboard:{Colors.END}    http://0.0.0.0:5000")
    print(f"{Colors.CYAN}VPS Control:{Colors.END}  http://0.0.0.0:5000/vps")
    print(f"\n{Colors.YELLOW}Press Ctrl+C to stop{Colors.END}\n")
    print(f"{Colors.BOLD}{'='*70}{Colors.END}\n")
    
    try:
        # Import and run app
        import app
        # App will run via socketio.run() in app.py
    except KeyboardInterrupt:
        print(f"\n{Colors.YELLOW}Dashboard stopped by user{Colors.END}")
    except Exception as e:
        log_error(f"Dashboard error: {e}")
        return False
    
    return True

def quick_check():
    """Quick check if we can skip setup"""
    # Check if already built and deployed recently
    if os.path.exists('raven-scanner') and os.path.exists('scanner_package.tar.gz'):
        # Check if scanner is recent (less than 1 day old)
        scanner_age = time.time() - os.path.getmtime('raven-scanner')
        if scanner_age < 86400:  # 24 hours
            print(f"{Colors.GREEN}✓{Colors.END} Scanner already built (use --rebuild to force rebuild)")
            return 'skip_build'
    return 'full'

def main():
    banner()
    
    # Parse arguments
    skip_deploy = '--no-deploy' in sys.argv
    force_rebuild = '--rebuild' in sys.argv
    dashboard_only = '--dashboard-only' in sys.argv
    
    if dashboard_only:
        print(f"{Colors.CYAN}Mode:{Colors.END} Dashboard only\n")
        return start_dashboard()
    
    # Quick check
    if not force_rebuild:
        check_result = quick_check()
        if check_result == 'skip_build':
            log_info("Skipping build (scanner already exists)")
            print()
        else:
            # Step 1: Build
            if not run_auto_setup():
                log_error("Setup failed!")
                return False
    else:
        # Force rebuild
        if not run_auto_setup():
            log_error("Setup failed!")
            return False
    
    # Step 2: Check SSH config
    if not skip_deploy:
        if not check_ssh_config():
            log_error("SSH configuration incomplete!")
            log_info("Fix the issues above, then run again")
            return False
        
        # Step 3: Deploy
        if not auto_deploy():
            log_error("Deployment failed!")
            log_info("You can still run dashboard with: python start.py --dashboard-only")
            
            answer = input(f"\n{Colors.YELLOW}Continue to dashboard anyway? (y/N):{Colors.END} ").strip().lower()
            if answer != 'y':
                return False
    else:
        print(f"\n{Colors.YELLOW}Skipping deployment (--no-deploy flag){Colors.END}")
    
    # Step 4: Start dashboard
    return start_dashboard()

if __name__ == '__main__':
    try:
        # Show help
        if '--help' in sys.argv or '-h' in sys.argv:
            print(f"""
{Colors.BOLD}RAVEN X 2.0 - ONE-CLICK STARTER{Colors.END}

Usage:
  python start.py                    Full auto: Build → Deploy → Dashboard
  python start.py --no-deploy        Skip deployment, just build and run dashboard
  python start.py --dashboard-only   Skip build and deploy, just run dashboard
  python start.py --rebuild          Force rebuild even if scanner exists

Requirements:
  - Go installed (auto-installs on Linux)
  - SSH key configured in ssh_config.json
  - Server IPs in server_ips.txt
  - Targets in targets.txt (for deployment)

Files:
  auto_setup.py      - Auto build scanner binary
  ssh_manager.py     - SSH deployment manager
  app.py            - Dashboard application
  config.json       - Scanner configuration
  ssh_config.json   - SSH configuration
  server_ips.txt    - VPS server list
  targets.txt       - URLs to scan
""")
            sys.exit(0)
        
        success = main()
        sys.exit(0 if success else 1)
    
    except KeyboardInterrupt:
        print(f"\n{Colors.YELLOW}Interrupted by user{Colors.END}")
        sys.exit(0)
    except Exception as e:
        print(f"\n{Colors.RED}Error: {e}{Colors.END}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
