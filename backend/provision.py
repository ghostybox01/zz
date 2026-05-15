#!/usr/bin/env python3
"""
VPS FLEET MANAGER - Multi-Cloud Edition
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Provisions VPS fleets across DigitalOcean AND Linode with unified SSH control.

Features:
  • Support for DigitalOcean and Linode
  • Multiple API tokens per provider
  • Single RSA key for entire fleet
  • Auto-generate server_ips.txt
  • Cross-provider management

Requirements:
  pip install pydo linode-api4 requests

Usage:
  python3 provision_multicloud.py
"""

import os
import sys
import time
import json
import shutil
import subprocess
import requests
import random
import string
from typing import List, Dict, Optional, Tuple
from datetime import datetime

# Try importing cloud provider libraries
PROVIDERS_AVAILABLE = {}

try:
    from pydo import Client as DOClient
    PROVIDERS_AVAILABLE['digitalocean'] = True
except ImportError:
    PROVIDERS_AVAILABLE['digitalocean'] = False

try:
    from linode_api4 import LinodeClient
    PROVIDERS_AVAILABLE['linode'] = True
except ImportError:
    PROVIDERS_AVAILABLE['linode'] = False

if not any(PROVIDERS_AVAILABLE.values()):
    print("[!] No cloud providers available. Install at least one:")
    print("    pip install pydo          # For DigitalOcean")
    print("    pip install linode-api4   # For Linode")
    sys.exit(1)

# ══════════════════════════════════════════════════════════════════════════════
# CONFIG
# ══════════════════════════════════════════════════════════════════════════════

class C:
    R = '\033[91m'
    G = '\033[92m'
    Y = '\033[93m'
    B = '\033[94m'
    M = '\033[95m'
    C = '\033[96m'
    W = '\033[97m'
    D = '\033[90m'
    BOLD = '\033[1m'
    X = '\033[0m'

# Provider configurations
PROVIDERS = {
    'digitalocean': {
        'name': 'DigitalOcean',
        'sizes': {
            '1': {'slug': 's-1vcpu-1gb', 'name': '1 CPU, 1GB RAM', 'cost': 6},
            '2': {'slug': 's-1vcpu-2gb', 'name': '1 CPU, 2GB RAM', 'cost': 12},
            '3': {'slug': 's-2vcpu-2gb', 'name': '2 CPU, 2GB RAM', 'cost': 18},
            '4': {'slug': 's-2vcpu-4gb', 'name': '2 CPU, 4GB RAM', 'cost': 24},
            '5': {'slug': 's-4vcpu-8gb', 'name': '4 CPU, 8GB RAM', 'cost': 48},
        },
        'regions': {
            '1': {'slug': 'nyc1', 'name': 'New York 1'},
            '2': {'slug': 'nyc3', 'name': 'New York 3'},
            '3': {'slug': 'sfo3', 'name': 'San Francisco'},
            '4': {'slug': 'lon1', 'name': 'London'},
            '5': {'slug': 'fra1', 'name': 'Frankfurt'},
            '6': {'slug': 'sgp1', 'name': 'Singapore'},
        },
        'image': 'ubuntu-22-04-x64',
    },
    'linode': {
        'name': 'Linode',
        'sizes': {
            '1': {'slug': 'g6-nanode-1', 'name': '1 CPU, 1GB RAM', 'cost': 5},
            '2': {'slug': 'g6-standard-1', 'name': '1 CPU, 2GB RAM', 'cost': 12},
            '3': {'slug': 'g6-standard-2', 'name': '2 CPU, 4GB RAM', 'cost': 24},
            '4': {'slug': 'g6-standard-4', 'name': '4 CPU, 8GB RAM', 'cost': 48},
            '5': {'slug': 'g6-standard-6', 'name': '6 CPU, 16GB RAM', 'cost': 96},
        },
        'regions': {
            '1': {'slug': 'us-east', 'name': 'Newark, NJ'},
            '2': {'slug': 'us-central', 'name': 'Dallas, TX'},
            '3': {'slug': 'us-west', 'name': 'Fremont, CA'},
            '4': {'slug': 'eu-west', 'name': 'London, UK'},
            '5': {'slug': 'eu-central', 'name': 'Frankfurt, DE'},
            '6': {'slug': 'ap-south', 'name': 'Singapore, SG'},
        },
        'image': 'linode/ubuntu22.04',
    }
}

DEFAULT_SSH_KEY = os.path.expanduser('~/.ssh/vps_fleet_master')
RAVEN_SSH_KEY = '/root/ssh/1'  # Path RAVEN panel expects
FLEET_INVENTORY = 'fleet_inventory.json'
CONFIG_FILE = 'fleet_config.json'
SERVER_IPS_FILE = 'server_ips.txt'

# ══════════════════════════════════════════════════════════════════════════════
# UTILITIES
# ══════════════════════════════════════════════════════════════════════════════

def clear():
    os.system('clear' if os.name != 'nt' else 'cls')

def pause():
    input(f"\n{C.D}Press Enter to continue...{C.X}")

def banner():
    clear()
    providers_text = " • ".join([PROVIDERS[p]['name'] for p in PROVIDERS if PROVIDERS_AVAILABLE.get(p)])
    print(f"""{C.C}
╔═══════════════════════════════════════════════════════════════╗
║           VPS FLEET MANAGER - Multi-Cloud Edition             ║
║              {providers_text:^45}              ║
╚═══════════════════════════════════════════════════════════════╝{C.X}
""")

# ══════════════════════════════════════════════════════════════════════════════
# CONFIG MANAGEMENT
# ══════════════════════════════════════════════════════════════════════════════

def load_config() -> Dict:
    """Load fleet configuration"""
    if os.path.exists(CONFIG_FILE):
        with open(CONFIG_FILE, 'r') as f:
            return json.load(f)
    return {
        'api_tokens': [],
        'ssh_key_path': DEFAULT_SSH_KEY,
        'inventory_path': FLEET_INVENTORY,
        'server_ips_file': SERVER_IPS_FILE
    }

def save_config(config: Dict):
    """Save fleet configuration"""
    with open(CONFIG_FILE, 'w') as f:
        json.dump(config, f, indent=2)

def add_api_token(config: Dict, provider: str, token: str, label: str = None) -> bool:
    """Add and validate API token"""
    try:
        if provider == 'digitalocean':
            if not PROVIDERS_AVAILABLE.get('digitalocean'):
                print(f"{C.R}[✗] DigitalOcean library not installed (pip install pydo){C.X}")
                return False
            
            client = DOClient(token=token)
            response = client.account.get()
            account = response['account']
            
            droplets_response = client.droplets.list()
            current = len(droplets_response.get('droplets', []))
            
            token_info = {
                'provider': 'digitalocean',
                'token': token,
                'label': label or f"DO-{len([t for t in config['api_tokens'] if t['provider'] == 'digitalocean']) + 1}",
                'email': account.get('email', 'N/A'),
                'limit': account.get('droplet_limit', 10),
                'current': current,
                'status': account.get('status', 'N/A'),
                'added': datetime.now().isoformat()
            }
            
        elif provider == 'linode':
            if not PROVIDERS_AVAILABLE.get('linode'):
                print(f"{C.R}[✗] Linode library not installed (pip install linode-api4){C.X}")
                return False
            
            client = LinodeClient(token=token)
            profile = client.profile()
            instances = client.linode.instances()
            current = len(instances)
            
            token_info = {
                'provider': 'linode',
                'token': token,
                'label': label or f"Linode-{len([t for t in config['api_tokens'] if t['provider'] == 'linode']) + 1}",
                'email': profile.email,
                'limit': 100,  # Linode has soft limits
                'current': current,
                'status': 'active',
                'added': datetime.now().isoformat()
            }
        else:
            print(f"{C.R}[✗] Unknown provider: {provider}{C.X}")
            return False
        
        config['api_tokens'].append(token_info)
        return True
        
    except Exception as e:
        print(f"{C.R}[✗] Invalid token: {e}{C.X}")
        return False

# ══════════════════════════════════════════════════════════════════════════════
# SSH KEY MANAGEMENT
# ══════════════════════════════════════════════════════════════════════════════

def ensure_master_ssh_key(config: Dict, force_new: bool = False) -> Tuple[str, str]:
    """Ensure master SSH key exists and copy to RAVEN path"""
    private_key = config['ssh_key_path']
    public_key = f"{private_key}.pub"
    
    ssh_dir = os.path.dirname(private_key)
    os.makedirs(ssh_dir, exist_ok=True)
    os.chmod(ssh_dir, 0o700)
    
    # Check if key exists and we don't want to force new
    if os.path.exists(public_key) and not force_new:
        with open(public_key, 'r') as f:
            pub_content = f.read().strip()
        # Also copy to RAVEN path
        copy_key_to_raven(private_key)
        return private_key, pub_content
    
    print(f"{C.Y}[*] Generating master SSH key (RSA 4096-bit, PEM format)...{C.X}")
    
    try:
        # Remove old keys if force_new
        if force_new:
            for f in [private_key, public_key]:
                if os.path.exists(f):
                    os.remove(f)
        
        # Generate RSA key in PEM format (compatible with paramiko)
        cmd = ['ssh-keygen', '-t', 'rsa', '-b', '4096', '-m', 'PEM', '-f', private_key, '-N', '',
               '-C', 'vps-fleet-master', '-q']
        result = subprocess.run(cmd, capture_output=True, text=True)
        
        if result.returncode != 0:
            raise Exception(f"ssh-keygen failed: {result.stderr}")
        
        os.chmod(private_key, 0o600)
        os.chmod(public_key, 0o644)
        
        with open(public_key, 'r') as f:
            public_key_content = f.read().strip()
        
        print(f"{C.G}[✓] Master SSH key generated (PEM format){C.X}")
        print(f"    Private: {private_key}")
        print(f"    Public:  {public_key}")
        
        # Also copy to RAVEN path
        copy_key_to_raven(private_key)
        
        return private_key, public_key_content
        
    except Exception as e:
        print(f"{C.R}[✗] Failed: {e}{C.X}")
        sys.exit(1)

def copy_key_to_raven(source_key: str) -> bool:
    """Copy SSH key to RAVEN panel path (/root/ssh/1)"""
    try:
        # Create RAVEN ssh directory
        raven_dir = os.path.dirname(RAVEN_SSH_KEY)
        os.makedirs(raven_dir, exist_ok=True)
        os.chmod(raven_dir, 0o700)
        
        # Copy private key
        shutil.copy2(source_key, RAVEN_SSH_KEY)
        os.chmod(RAVEN_SSH_KEY, 0o600)
        
        # Copy public key
        source_pub = f"{source_key}.pub"
        raven_pub = f"{RAVEN_SSH_KEY}.pub"
        if os.path.exists(source_pub):
            shutil.copy2(source_pub, raven_pub)
            os.chmod(raven_pub, 0o644)
        
        print(f"{C.G}[✓] Key copied to RAVEN path: {RAVEN_SSH_KEY}{C.X}")
        return True
    except Exception as e:
        print(f"{C.R}[✗] Failed to copy to RAVEN: {e}{C.X}")
        return False

def menu_regenerate_key(config: Dict):
    """Regenerate SSH key (force new RSA key)"""
    banner()
    print(f"{C.M}═══ REGENERATE SSH KEY ═══{C.X}\n")
    
    print(f"{C.Y}⚠️  WARNING: This will generate a NEW SSH key!{C.X}")
    print(f"    You will need to re-sync to all cloud providers.")
    print(f"    Existing VPS will need the new key added.\n")
    
    confirm = input(f"{C.R}Type 'YES' to confirm: {C.X}").strip()
    
    if confirm != 'YES':
        print(f"\n{C.Y}[!] Cancelled{C.X}")
        pause()
        return
    
    print()
    private_key, public_key = ensure_master_ssh_key(config, force_new=True)
    
    print(f"\n{C.G}[✓] New RSA 4096-bit key generated!{C.X}")
    print(f"\n{C.Y}[!] Next steps:{C.X}")
    print(f"    1. Use option [4] to sync key to cloud providers")
    print(f"    2. Or manually add to existing VPS:\n")
    print(f"    {C.C}Public key:{C.X}")
    print(f"    {public_key[:60]}...")
    
    pause()

def menu_copy_to_raven(config: Dict):
    """Copy SSH key to RAVEN panel path"""
    banner()
    print(f"{C.M}═══ COPY KEY TO RAVEN ═══{C.X}\n")
    
    private_key = config['ssh_key_path']
    
    if not os.path.exists(private_key):
        print(f"{C.R}[✗] Master key not found: {private_key}{C.X}")
        print(f"    Run option [4] first to generate a key.")
        pause()
        return
    
    # Check key type
    with open(private_key, 'r') as f:
        first_line = f.readline()
    
    if 'DSA' in first_line:
        print(f"{C.R}[✗] Current key is DSA format (not supported by RAVEN){C.X}")
        print(f"    Use option [6] to regenerate as RSA 4096-bit.")
        pause()
        return
    
    print(f"Source:      {private_key}")
    print(f"Destination: {RAVEN_SSH_KEY}")
    print()
    
    if copy_key_to_raven(private_key):
        print(f"\n{C.G}[✓] RAVEN panel can now use this key!{C.X}")
        print(f"    Make sure RAVEN's SSH Key Path is set to: {RAVEN_SSH_KEY}")
    
    pause()

def menu_convert_to_pem(config: Dict):
    """Convert existing SSH key to PEM format (paramiko-compatible)"""
    banner()
    print(f"{C.M}═══ CONVERT KEY TO PEM FORMAT ═══{C.X}\n")
    
    private_key = config['ssh_key_path']
    
    if not os.path.exists(private_key):
        print(f"{C.R}[✗] Master key not found: {private_key}{C.X}")
        pause()
        return
    
    # Check current format
    with open(private_key, 'r') as f:
        first_line = f.readline()
    
    if 'BEGIN RSA PRIVATE KEY' in first_line:
        print(f"{C.G}[✓] Key is already in PEM format!{C.X}")
        pause()
        return
    
    if 'DSA' in first_line:
        print(f"{C.R}[✗] Cannot convert DSA key. Use option [6] to regenerate.{C.X}")
        pause()
        return
    
    print(f"Current format: OpenSSH (new format)")
    print(f"Target format:  PEM (paramiko-compatible)")
    print(f"\nThis will convert: {private_key}")
    print(f"And also: {RAVEN_SSH_KEY}\n")
    
    confirm = input(f"Convert to PEM? [y/N]: ").strip().lower()
    if confirm != 'y':
        print(f"{C.Y}[!] Cancelled{C.X}")
        pause()
        return
    
    try:
        # Convert master key
        print(f"\n{C.Y}[*] Converting master key...{C.X}")
        cmd = ['ssh-keygen', '-p', '-m', 'PEM', '-f', private_key, '-N', '', '-P', '']
        result = subprocess.run(cmd, capture_output=True, text=True)
        
        if result.returncode != 0:
            # Try without -P flag (older ssh-keygen)
            cmd = ['ssh-keygen', '-p', '-m', 'PEM', '-f', private_key, '-N', '']
            result = subprocess.run(cmd, capture_output=True, text=True, input='\n')
        
        print(f"{C.G}[✓] Master key converted{C.X}")
        
        # Convert RAVEN key if exists
        if os.path.exists(RAVEN_SSH_KEY):
            print(f"{C.Y}[*] Converting RAVEN key...{C.X}")
            cmd = ['ssh-keygen', '-p', '-m', 'PEM', '-f', RAVEN_SSH_KEY, '-N', '', '-P', '']
            result = subprocess.run(cmd, capture_output=True, text=True)
            if result.returncode != 0:
                cmd = ['ssh-keygen', '-p', '-m', 'PEM', '-f', RAVEN_SSH_KEY, '-N', '']
                subprocess.run(cmd, capture_output=True, text=True, input='\n')
            print(f"{C.G}[✓] RAVEN key converted{C.X}")
        else:
            # Copy the converted key to RAVEN path
            copy_key_to_raven(private_key)
        
        # Verify
        with open(private_key, 'r') as f:
            new_first_line = f.readline()
        
        if 'BEGIN RSA PRIVATE KEY' in new_first_line:
            print(f"\n{C.G}[✓] Conversion successful!{C.X}")
            print(f"    Keys are now in PEM format (paramiko-compatible)")
        else:
            print(f"\n{C.Y}[!] Conversion may have failed. First line: {new_first_line[:40]}{C.X}")
        
    except Exception as e:
        print(f"{C.R}[✗] Error: {e}{C.X}")
    
    pause()

def push_key_to_all_vps(config: Dict):
    """Push SSH key to all VPS in inventory or server_ips.txt"""
    banner()
    print(f"{C.M}═══ PUSH SSH KEY TO ALL VPS ═══{C.X}\n")
    
    # Check if key exists
    key_path = config['ssh_key_path']
    pub_key_path = f"{key_path}.pub"
    
    if not os.path.exists(pub_key_path):
        print(f"{C.R}[✗] Public key not found: {pub_key_path}{C.X}")
        print(f"\nGenerate key first with option [6]")
        pause()
        return
    
    # Read public key
    with open(pub_key_path, 'r') as f:
        public_key = ' '.join(f.read().strip().split())
    
    print(f"{C.BOLD}SSH Key:{C.X}")
    print(f"  {pub_key_path}")
    print(f"  {public_key[:60]}...\n")
    
    # Load inventory first (has passwords)
    inventory = load_inventory(config)
    
    # Create IP to password mapping from inventory
    ip_passwords = {}
    for entry in inventory:
        if entry.get('ip') and entry.get('root_password'):
            ip_passwords[entry['ip']] = entry['root_password']
    
    # Load server IPs
    ips_file = config.get('server_ips_file', SERVER_IPS_FILE)
    
    if os.path.exists(ips_file):
        with open(ips_file, 'r') as f:
            ips = [line.strip() for line in f if line.strip() and not line.startswith('#')]
    else:
        # Use inventory IPs
        ips = [e['ip'] for e in inventory if e.get('ip') and e['ip'] != 'pending']
    
    if not ips:
        print(f"{C.Y}No IPs found{C.X}")
        pause()
        return
    
    print(f"{C.BOLD}Found {len(ips)} servers{C.X}")
    print(f"{C.BOLD}Found {len(ip_passwords)} stored passwords{C.X}\n")
    
    # Check if sshpass is installed
    sshpass_installed = shutil.which('sshpass') is not None
    if not sshpass_installed:
        print(f"{C.Y}[!] sshpass not installed. Installing...{C.X}")
        os.system('apt-get update && apt-get install -y sshpass')
        sshpass_installed = shutil.which('sshpass') is not None
    
    # Ask for fallback password
    fallback_password = None
    print(f"\n{C.Y}If a server's password is not stored, you can provide a fallback.{C.X}")
    use_fallback = input(f"Enter fallback root password (or press Enter to skip): ").strip()
    if use_fallback:
        fallback_password = use_fallback
    
    confirm = input(f"\nPush key to {len(ips)} servers? (y/N): ").strip().lower()
    if confirm != 'y':
        print(f"{C.Y}Cancelled{C.X}")
        pause()
        return
    
    print(f"\n{C.C}[*] Pushing SSH key to servers...{C.X}\n")
    
    success = 0
    failed = 0
    
    for ip in ips:
        print(f"  {C.C}[*]{C.X} {ip} ... ", end='', flush=True)
        
        # Get password for this IP
        password = ip_passwords.get(ip) or fallback_password
        
        try:
            # Method 1: Try SSH with current key first
            result = subprocess.run(
                ['ssh', '-i', key_path, 
                 '-o', 'StrictHostKeyChecking=no',
                 '-o', 'UserKnownHostsFile=/dev/null',
                 '-o', 'ConnectTimeout=10',
                 '-o', 'BatchMode=yes',
                 f'root@{ip}', 'echo "OK"'],
                capture_output=True,
                text=True,
                timeout=15
            )
            
            if result.returncode == 0 and 'OK' in result.stdout:
                print(f"{C.G}[✓] Already has key{C.X}")
                success += 1
                continue
            
            # Method 2: Try with password
            if password and sshpass_installed:
                print(f"{C.Y}adding key...{C.X} ", end='', flush=True)
                
                add_key_cmd = f"mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '{public_key}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && sort -u ~/.ssh/authorized_keys -o ~/.ssh/authorized_keys"
                
                result = subprocess.run(
                    ['sshpass', '-p', password, 'ssh',
                     '-o', 'StrictHostKeyChecking=no',
                     '-o', 'UserKnownHostsFile=/dev/null',
                     '-o', 'ConnectTimeout=15',
                     f'root@{ip}', add_key_cmd],
                    capture_output=True,
                    text=True,
                    timeout=30
                )
                
                if result.returncode == 0:
                    print(f"{C.G}[✓] Key added!{C.X}")
                    success += 1
                else:
                    error = result.stderr[:50] if result.stderr else "Unknown error"
                    print(f"{C.R}[✗] {error}{C.X}")
                    failed += 1
            else:
                reason = "no password" if not password else "sshpass not installed"
                print(f"{C.R}[✗] Can't connect ({reason}){C.X}")
                failed += 1
                
        except subprocess.TimeoutExpired:
            print(f"{C.R}[✗] Timeout{C.X}")
            failed += 1
        except Exception as e:
            print(f"{C.R}[✗] {str(e)[:30]}{C.X}")
            failed += 1
        
        time.sleep(0.3)
    
    print()
    print(f"{C.W}{'─'*60}{C.X}")
    print(f"  {C.G}Success:{C.X} {success}")
    print(f"  {C.R}Failed:{C.X}  {failed}")
    print(f"{C.W}{'─'*60}{C.X}\n")
    
    if success > 0:
        print(f"{C.G}[✓] {success} servers now have SSH key!{C.X}")
        print(f"\n{C.BOLD}RAVEN should now work. Test with:{C.X}")
        print(f"  ssh -i /root/ssh/1 root@{ips[0]} hostname")
    
    if failed > 0:
        print(f"\n{C.Y}[!] {failed} servers failed{C.X}")
        print(f"  • Check Linode panel for root passwords")
        print(f"  • VPS might still be booting (wait 1-2 min)")
        print(f"  • Or add key via Linode web console (LISH)")
    
    pause()

def check_key_type(key_path: str) -> str:
    """Check SSH key type"""
    if not os.path.exists(key_path):
        return "not found"
    try:
        with open(key_path, 'r') as f:
            content = f.read()
        if 'RSA' in content or 'OPENSSH' in content:
            return "RSA"
        elif 'DSA' in content:
            return "DSA (incompatible)"
        elif 'EC' in content:
            return "ECDSA"
        elif 'ED25519' in content.upper():
            return "Ed25519"
        return "unknown"
    except:
        return "error"

def upload_key_to_provider(provider: str, token: str, public_key: str, label: str) -> bool:
    """Upload SSH key to provider"""
    try:
        if provider == 'digitalocean':
            client = DOClient(token=token)
            timestamp = datetime.now().strftime('%Y%m%d-%H%M%S')
            key_name = f'fleet-master-{label}-{timestamp}'
            
            req = {'name': key_name, 'public_key': public_key}
            client.ssh_keys.create(body=req)
            return True
            
        elif provider == 'linode':
            client = LinodeClient(token=token)
            
            # Linode requires clean key format - strip any extra whitespace/newlines
            # But keep the key as a single line with proper spacing
            key_parts = public_key.strip().split()
            if len(key_parts) >= 2:
                # Format: ssh-rsa AAAA... comment
                clean_key = f"{key_parts[0]} {key_parts[1]}"
                if len(key_parts) > 2:
                    clean_key += f" {' '.join(key_parts[2:])}"
            else:
                clean_key = public_key.strip()
            
            # Check if key already exists
            try:
                existing_keys = client.profile.ssh_keys()
                for key in existing_keys:
                    # Compare just the key data (second part)
                    existing_parts = key.ssh_key.strip().split()
                    if len(existing_parts) >= 2 and len(key_parts) >= 2:
                        if existing_parts[1] == key_parts[1]:
                            print(f"    Key already exists on Linode")
                            return True
            except:
                pass
            
            # Upload new key
            timestamp = datetime.now().strftime('%Y%m%d-%H%M%S')
            key_label = f'fleet-{timestamp}'
            
            # Validate key format
            if not (clean_key.startswith('ssh-rsa ') or clean_key.startswith('ssh-ed25519 ') or clean_key.startswith('ecdsa-')):
                print(f"    Error: Invalid key format")
                return False
            
            try:
                # Try the standard method
                client.profile.ssh_key_upload(key_label, clean_key)
                return True
            except Exception as upload_err:
                # Try alternative: direct API call
                try:
                    headers = {'Authorization': f'Bearer {token}', 'Content-Type': 'application/json'}
                    data = {'label': key_label, 'ssh_key': clean_key}
                    resp = requests.post('https://api.linode.com/v4/profile/sshkeys', headers=headers, json=data)
                    if resp.status_code in [200, 201]:
                        return True
                    elif 'already' in resp.text.lower():
                        return True
                    else:
                        print(f"    API Error: {resp.text[:100]}")
                        return False
                except Exception as api_err:
                    print(f"    Error: {upload_err}")
                    return False
            
    except Exception as e:
        error_msg = str(e).lower()
        if 'already in use' in error_msg or 'duplicate' in error_msg or 'already exists' in error_msg:
            return True
        print(f"    Error: {e}")
        return False

def sync_master_key(config: Dict):
    """Sync master SSH key to all accounts"""
    banner()
    print(f"{C.M}═══ SYNC MASTER KEY ═══{C.X}\n")
    
    private_key, public_key = ensure_master_ssh_key(config)
    
    if not config['api_tokens']:
        print(f"{C.Y}No API tokens configured{C.X}")
        pause()
        return
    
    print(f"Syncing to {len(config['api_tokens'])} accounts...\n")
    
    for token_info in config['api_tokens']:
        success = upload_key_to_provider(
            token_info['provider'],
            token_info['token'],
            public_key,
            token_info['label']
        )
        
        status = f"{C.G}[✓]{C.X}" if success else f"{C.R}[✗]{C.X}"
        provider_name = PROVIDERS[token_info['provider']]['name']
        print(f"  {status} {token_info['label']} ({provider_name})")
        time.sleep(0.5)
    
    print(f"\n{C.G}[✓] Sync complete{C.X}")
    pause()

# ══════════════════════════════════════════════════════════════════════════════
# INVENTORY MANAGEMENT
# ══════════════════════════════════════════════════════════════════════════════

def load_inventory(config: Dict) -> List[Dict]:
    """Load fleet inventory"""
    if os.path.exists(config['inventory_path']):
        with open(config['inventory_path'], 'r') as f:
            return json.load(f)
    return []

def save_inventory(config: Dict, inventory: List[Dict]):
    """Save fleet inventory"""
    with open(config['inventory_path'], 'w') as f:
        json.dump(inventory, f, indent=2)

def save_server_ips(config: Dict):
    """Save all VPS IPs to server_ips.txt"""
    inventory = load_inventory(config)
    ips_file = config.get('server_ips_file', SERVER_IPS_FILE)
    
    with open(ips_file, 'w') as f:
        for entry in inventory:
            if entry['ip'] != 'pending':
                f.write(f"{entry['ip']}\n")
    
    count = len([e for e in inventory if e['ip'] != 'pending'])
    if count > 0:
        print(f"{C.G}[✓] Saved {count} IPs to {ips_file}{C.X}")

def add_to_inventory(config: Dict, vps_data: Dict):
    """Add VPS to inventory"""
    inventory = load_inventory(config)
    inventory.append(vps_data)
    save_inventory(config, inventory)
    save_server_ips(config)

# ══════════════════════════════════════════════════════════════════════════════
# VPS CREATION
# ══════════════════════════════════════════════════════════════════════════════

def create_vps_digitalocean(token: str, name: str, size_slug: str, region_slug: str,
                           ssh_key_path: str, account_label: str) -> Optional[Dict]:
    """Create VPS on DigitalOcean"""
    try:
        client = DOClient(token=token)
        
        # Get SSH keys
        ssh_keys_response = client.ssh_keys.list()
        ssh_keys = ssh_keys_response.get('ssh_keys', [])
        key_ids = [k['id'] for k in ssh_keys]
        
        if not key_ids:
            return None
        
        req = {
            'name': name,
            'region': region_slug,
            'size': size_slug,
            'image': PROVIDERS['digitalocean']['image'],
            'ssh_keys': key_ids,
            'tags': ['vps-fleet'],
            'backups': False,
            'ipv6': False,
            'monitoring': False
        }
        
        response = client.droplets.create(body=req)
        droplet = response['droplet']
        
        # Wait for IP
        max_wait = 60
        start = time.time()
        ip = 'pending'
        
        while time.time() - start < max_wait:
            time.sleep(3)
            d_response = client.droplets.get(droplet['id'])
            d = d_response['droplet']
            
            if d['status'] == 'active':
                for net in d.get('networks', {}).get('v4', []):
                    if net['type'] == 'public':
                        ip = net['ip_address']
                        break
                if ip != 'pending':
                    break
        
        return {
            'id': droplet['id'],
            'name': name,
            'ip': ip,
            'region': region_slug,
            'size': size_slug,
            'status': 'active' if ip != 'pending' else 'new',
            'provider': 'digitalocean',
            'account': account_label,
            'created': datetime.now().isoformat(),
            'ssh_key': ssh_key_path,
            'ssh_user': 'root'
        }
        
    except Exception as e:
        print(f"  {C.R}[✗]{C.X} Error: {e}")
        return None

def create_vps_linode(token: str, name: str, size_slug: str, region_slug: str,
                     ssh_key_path: str, account_label: str, root_password: str = None) -> Optional[Dict]:
    """Create VPS on Linode"""
    try:
        client = LinodeClient(token=token)
        
        # Read SSH public key
        with open(f"{ssh_key_path}.pub", 'r') as f:
            ssh_key_raw = f.read().strip()
        
        # Clean the SSH key format - Linode is very strict
        ssh_key = ' '.join(ssh_key_raw.split())
        
        # Validate key format
        if not (ssh_key.startswith('ssh-rsa ') or ssh_key.startswith('ssh-ed25519 ')):
            print(f"  {C.R}[✗]{C.X} Invalid SSH key format")
            return None
        
        # Generate password if not provided
        if not root_password:
            root_password = ''.join(random.choices(string.ascii_letters + string.digits + "!@#$%", k=20))
        
        # Create instance - returns Instance object directly (not a tuple)
        instance = client.linode.instance_create(
            ltype=size_slug,
            region=region_slug,
            image=PROVIDERS['linode']['image'],
            label=name,
            root_pass=root_password,
            authorized_keys=[ssh_key],
            tags=['vps-fleet']
        )
        
        # Wait for IP
        max_wait = 60
        start = time.time()
        ip = 'pending'
        
        while time.time() - start < max_wait:
            time.sleep(3)
            
            # Refresh instance data
            try:
                instance = client.load(instance)
                
                if instance.status == 'running' and instance.ipv4:
                    ip = str(instance.ipv4[0])
                    break
            except:
                # Try direct access
                if hasattr(instance, 'ipv4') and instance.ipv4:
                    ip = str(instance.ipv4[0])
                    break
        
        # If still pending, try to get IP anyway
        if ip == 'pending' and hasattr(instance, 'ipv4') and instance.ipv4:
            ip = str(instance.ipv4[0])
        
        return {
            'id': instance.id,
            'name': name,
            'ip': ip,
            'region': region_slug,
            'size': size_slug,
            'status': instance.status if hasattr(instance, 'status') else 'unknown',
            'provider': 'linode',
            'account': account_label,
            'created': datetime.now().isoformat(),
            'ssh_key': ssh_key_path,
            'ssh_user': 'root',
            'root_password': root_password
        }
        
    except Exception as e:
        print(f"  {C.R}[✗]{C.X} Error: {e}")
        return None

def create_vps(provider: str, token: str, name: str, size_slug: str, region_slug: str,
              ssh_key_path: str, account_label: str, config: Dict, root_password: str = None) -> bool:
    """Create VPS on specified provider"""
    
    if provider == 'digitalocean':
        vps_data = create_vps_digitalocean(token, name, size_slug, region_slug,
                                          ssh_key_path, account_label)
    elif provider == 'linode':
        vps_data = create_vps_linode(token, name, size_slug, region_slug,
                                    ssh_key_path, account_label, root_password)
    else:
        return False
    
    if vps_data:
        add_to_inventory(config, vps_data)
        return True
    
    return False

# ══════════════════════════════════════════════════════════════════════════════
# API TOKEN MANAGEMENT
# ══════════════════════════════════════════════════════════════════════════════

def menu_manage_apis(config: Dict):
    """Manage API tokens"""
    while True:
        banner()
        print(f"{C.M}═══ API TOKEN MANAGEMENT ═══{C.X}\n")
        
        if not config['api_tokens']:
            print(f"{C.Y}No API tokens configured{C.X}\n")
        else:
            print(f"{C.BOLD}Configured Accounts:{C.X}\n")
            
            # Group by provider
            by_provider = {}
            for token in config['api_tokens']:
                p = token['provider']
                if p not in by_provider:
                    by_provider[p] = []
                by_provider[p].append(token)
            
            total_limit = 0
            total_used = 0
            
            for provider, tokens in by_provider.items():
                provider_name = PROVIDERS[provider]['name']
                print(f"  {C.BOLD}{provider_name}:{C.X}")
                
                for token_info in tokens:
                    available = token_info['limit'] - token_info['current']
                    total_limit += token_info['limit']
                    total_used += token_info['current']
                    
                    print(f"    {C.C}•{C.X} {token_info['label']}")
                    print(f"      Email:    {token_info['email']}")
                    print(f"      Capacity: {token_info['current']}/{token_info['limit']} "
                          f"({C.Y}{available} available{C.X})")
                print()
            
            print(f"{C.W}{'─'*60}{C.X}")
            print(f"  {C.BOLD}Fleet Total:{C.X} {total_used}/{total_limit} VPS "
                  f"({C.Y}{total_limit - total_used} available{C.X})")
            print(f"{C.W}{'─'*60}{C.X}\n")
        
        print(f"  {C.C}[1]{C.X} Add DigitalOcean Token" + 
              ("" if PROVIDERS_AVAILABLE.get('digitalocean') else f" {C.D}(not installed){C.X}"))
        print(f"  {C.C}[2]{C.X} Add Linode Token" +
              ("" if PROVIDERS_AVAILABLE.get('linode') else f" {C.D}(not installed){C.X}"))
        print(f"  {C.C}[3]{C.X} Remove Token")
        print(f"  {C.C}[4]{C.X} Refresh Account Info")
        print(f"  {C.C}[0]{C.X} Back\n")
        
        choice = input(f"Select option: ").strip()
        
        if choice == '1':
            if not PROVIDERS_AVAILABLE.get('digitalocean'):
                print(f"\n{C.R}[✗] DigitalOcean library not installed{C.X}")
                print(f"Install with: pip install pydo")
                pause()
                continue
                
            banner()
            print(f"{C.M}═══ ADD DIGITALOCEAN TOKEN ═══{C.X}\n")
            print("Get your API token from:")
            print("  https://cloud.digitalocean.com/account/api/tokens\n")
            
            token = input(f"{C.C}Enter API token: {C.X}").strip()
            if not token:
                continue
            
            label = input(f"{C.C}Enter label (optional): {C.X}").strip()
            
            print(f"\n{C.C}[*] Validating token...{C.X}")
            if add_api_token(config, 'digitalocean', token, label):
                save_config(config)
                print(f"\n{C.G}[✓] Token added successfully{C.X}")
                
                # Auto-sync SSH key
                print(f"\n{C.C}[*] Syncing master SSH key...{C.X}")
                private_key, public_key = ensure_master_ssh_key(config)
                upload_key_to_provider('digitalocean', token, public_key, 
                                      label or f"DO-{len(config['api_tokens'])}")
            
            pause()
            
        elif choice == '2':
            if not PROVIDERS_AVAILABLE.get('linode'):
                print(f"\n{C.R}[✗] Linode library not installed{C.X}")
                print(f"Install with: pip install linode-api4")
                pause()
                continue
                
            banner()
            print(f"{C.M}═══ ADD LINODE TOKEN ═══{C.X}\n")
            print("Get your API token from:")
            print("  https://cloud.linode.com/profile/tokens\n")
            
            token = input(f"{C.C}Enter API token: {C.X}").strip()
            if not token:
                continue
            
            label = input(f"{C.C}Enter label (optional): {C.X}").strip()
            
            print(f"\n{C.C}[*] Validating token...{C.X}")
            if add_api_token(config, 'linode', token, label):
                save_config(config)
                print(f"\n{C.G}[✓] Token added successfully{C.X}")
                
                # Auto-sync SSH key
                print(f"\n{C.C}[*] Syncing master SSH key...{C.X}")
                private_key, public_key = ensure_master_ssh_key(config)
                upload_key_to_provider('linode', token, public_key,
                                      label or f"Linode-{len(config['api_tokens'])}")
            
            pause()
            
        elif choice == '3':
            if not config['api_tokens']:
                continue
            
            banner()
            print(f"{C.M}═══ REMOVE TOKEN ═══{C.X}\n")
            
            for i, token_info in enumerate(config['api_tokens'], 1):
                provider_name = PROVIDERS[token_info['provider']]['name']
                print(f"  {C.C}[{i}]{C.X} {token_info['label']} ({provider_name})")
            
            print()
            idx = input(f"Select token (1-{len(config['api_tokens'])}): ").strip()
            
            if idx.isdigit() and 1 <= int(idx) <= len(config['api_tokens']):
                removed = config['api_tokens'].pop(int(idx) - 1)
                save_config(config)
                print(f"\n{C.G}[✓] Removed {removed['label']}{C.X}")
            
            pause()
            
        elif choice == '4':
            banner()
            print(f"{C.M}═══ REFRESHING INFO ═══{C.X}\n")
            
            for token_info in config['api_tokens']:
                provider = token_info['provider']
                
                try:
                    if provider == 'digitalocean':
                        client = DOClient(token=token_info['token'])
                        droplets = client.droplets.list()
                        token_info['current'] = len(droplets.get('droplets', []))
                        
                    elif provider == 'linode':
                        client = LinodeClient(token=token_info['token'])
                        instances = client.linode.instances()
                        token_info['current'] = len(instances)
                    
                    print(f"  {C.G}[✓]{C.X} {token_info['label']}: "
                          f"{token_info['current']}/{token_info['limit']} VPS")
                    
                except Exception as e:
                    print(f"  {C.R}[✗]{C.X} {token_info['label']}: {e}")
                
                time.sleep(0.3)
            
            save_config(config)
            print(f"\n{C.G}[✓] Info refreshed{C.X}")
            pause()
            
        elif choice == '0':
            break

# Continued in next file part...

# ══════════════════════════════════════════════════════════════════════════════
# FLEET CREATION
# ══════════════════════════════════════════════════════════════════════════════

def menu_create_fleet(config: Dict):
    """Create VPS fleet"""
    banner()
    print(f"{C.M}═══ CREATE VPS FLEET ═══{C.X}\n")
    
    if not config['api_tokens']:
        print(f"{C.Y}No API tokens configured. Add tokens first.{C.X}")
        pause()
        return
    
    # Ensure SSH key
    ensure_master_ssh_key(config)
    
    # Select provider
    available_providers = [p for p in PROVIDERS if any(t['provider'] == p for t in config['api_tokens'])]
    
    if len(available_providers) > 1:
        print(f"{C.BOLD}Select Provider:{C.X}\n")
        for i, provider in enumerate(available_providers, 1):
            count = len([t for t in config['api_tokens'] if t['provider'] == provider])
            print(f"  {C.C}[{i}]{C.X} {PROVIDERS[provider]['name']} ({count} accounts)")
        print(f"  {C.C}[0]{C.X} Auto-distribute\n")
        
        choice = input(f"Select provider [0]: ").strip() or '0'
        
        if choice == '0':
            selected_provider = None  # Auto-distribute
        elif choice.isdigit() and 1 <= int(choice) <= len(available_providers):
            selected_provider = available_providers[int(choice) - 1]
        else:
            return
    else:
        selected_provider = available_providers[0]
    
    # Select size
    banner()
    print(f"{C.M}═══ SELECT VPS SIZE ═══{C.X}\n")
    
    if selected_provider:
        sizes = PROVIDERS[selected_provider]['sizes']
    else:
        # Show all sizes from all providers
        sizes = {}
        for p in available_providers:
            for k, v in PROVIDERS[p]['sizes'].items():
                key = f"{p}_{k}"
                sizes[key] = {**v, 'provider': p}
    
    for key, size in sorted(sizes.items(), key=lambda x: x[1]['cost']):
        if 'provider' in size:
            provider_tag = f" [{PROVIDERS[size['provider']]['name']}]"
        else:
            provider_tag = ""
        print(f"  {C.C}[{key.split('_')[-1] if '_' in key else key}]{C.X} "
              f"{size['name']:<20} ${size['cost']}/mo{provider_tag}")
    
    print()
    size_choice = input(f"Select size [1]: ").strip() or '1'
    
    # Find the size
    selected_size = None
    for key, size in sizes.items():
        if key.endswith(f"_{size_choice}") or key == size_choice:
            selected_size = (key, size)
            break
    
    if not selected_size:
        return
    
    size_key, size_info = selected_size
    
    # Calculate capacity
    total_capacity = sum(t['limit'] - t['current'] for t in config['api_tokens'])
    
    banner()
    print(f"{C.M}═══ CAPACITY CALCULATION ═══{C.X}\n")
    print(f"{C.BOLD}Selected:{C.X} {size_info['name']} @ ${size_info['cost']}/mo\n")
    
    print(f"{C.BOLD}Maximum VPS:{C.X}")
    print(f"  By Capacity: {C.Y}{total_capacity}{C.X} VPS\n")
    
    print(f"{C.BOLD}By Budget:{C.X}")
    for budget in [50, 100, 200, 500]:
        max_vps = min(int(budget / size_info['cost']), total_capacity)
        if max_vps > 0:
            print(f"  ${budget:3}/mo → {C.Y}{max_vps:3}{C.X} VPS (${max_vps * size_info['cost']}/mo)")
    
    print()
    
    # Get count
    count = input(f"How many VPS? [max {total_capacity}]: ").strip()
    count = int(count) if count.isdigit() else min(10, total_capacity)
    
    if count > total_capacity or count <= 0:
        print(f"{C.R}Invalid count{C.X}")
        pause()
        return
    
    # Select region
    banner()
    print(f"{C.M}═══ SELECT REGION ═══{C.X}\n")
    
    if selected_provider:
        regions = PROVIDERS[selected_provider]['regions']
    else:
        regions = PROVIDERS[available_providers[0]]['regions']  # Use first provider's regions
    
    for key, region in regions.items():
        print(f"  {C.C}[{key}]{C.X} {region['name']}")
    
    print()
    region_choice = input(f"Select region [1]: ").strip() or '1'
    
    if region_choice not in regions:
        return
    
    region_info = regions[region_choice]
    
    # Distribute across accounts
    distribution = []
    remaining = count
    
    accounts = [t for t in config['api_tokens'] if not selected_provider or t['provider'] == selected_provider]
    
    for token_info in accounts:
        available = token_info['limit'] - token_info['current']
        allocate = min(remaining, available)
        
        if allocate > 0:
            distribution.append({
                'token_info': token_info,
                'count': allocate
            })
            remaining -= allocate
        
        if remaining == 0:
            break
    
    # Confirm
    banner()
    print(f"{C.M}═══ CONFIRM CREATION ═══{C.X}\n")
    print(f"  Size:   {size_info['name']}")
    print(f"  Region: {region_info['name']}")
    print(f"  Total:  {C.Y}{count}{C.X} VPS")
    print(f"  Cost:   ${size_info['cost'] * count}/month\n")
    
    print(f"{C.BOLD}Distribution:{C.X}\n")
    for dist in distribution:
        provider_name = PROVIDERS[dist['token_info']['provider']]['name']
        print(f"  • {dist['token_info']['label']} ({provider_name}): {C.Y}{dist['count']}{C.X} VPS")
    
    print()
    
    # Ask for optional root password
    print(f"{C.BOLD}Root Password Setup:{C.X}")
    print(f"  {C.Y}Set a root password for all VPS? (recommended){C.X}")
    print(f"  This allows both SSH key AND password access.\n")
    
    set_password = input(f"Set root password? (Y/n): ").strip().lower()
    
    root_password = None
    if set_password != 'n':
        import getpass
        while True:
            root_password = getpass.getpass(f"{C.C}Enter root password (min 6 chars): {C.X}")
            if len(root_password) < 6:
                print(f"{C.R}Password too short! Minimum 6 characters.{C.X}")
                continue
            root_password2 = getpass.getpass(f"{C.C}Confirm password: {C.X}")
            if root_password == root_password2:
                print(f"{C.G}[✓] Password set{C.X}\n")
                break
            else:
                print(f"{C.R}Passwords don't match! Try again.{C.X}")
    else:
        print(f"{C.Y}[!] No password set - SSH key only{C.X}\n")
    
    confirm = input(f"Create fleet? (y/N): ").strip().lower()
    
    if confirm != 'y':
        return
    
    # Create VPS
    banner()
    print(f"{C.M}═══ CREATING FLEET ═══{C.X}\n")
    
    if root_password:
        print(f"{C.G}[✓] Creating VPS with password enabled{C.X}\n")
    
    created = 0
    attempted = 0
    timestamp = datetime.now().strftime('%Y%m%d-%H%M%S')
    
    for dist in distribution:
        token_info = dist['token_info']
        provider = token_info['provider']
        provider_name = PROVIDERS[provider]['name']
        
        print(f"{C.C}[{provider_name}]{C.X} {token_info['label']} - Creating {dist['count']} VPS...\n")
        
        for i in range(dist['count']):
            attempted += 1
            name = f"fleet-{timestamp}-{attempted:03d}"
            
            # Get the correct size slug for this provider
            if provider in size_key:
                size_slug = size_info['slug']
            else:
                # Find equivalent size for this provider
                size_slug = PROVIDERS[provider]['sizes'].get(size_choice, {}).get('slug')
                if not size_slug:
                    size_slug = list(PROVIDERS[provider]['sizes'].values())[0]['slug']
            
            success = create_vps(
                provider,
                token_info['token'],
                name,
                size_slug,
                region_info['slug'],
                config['ssh_key_path'],
                token_info['label'],
                config,
                root_password  # Pass the password
            )
            
            if success:
                print(f"  {C.G}[✓]{C.X} {name}")
                created += 1
                token_info['current'] += 1
            else:
                print(f"  {C.R}[✗]{C.X} {name}")
            
            time.sleep(2)  # Increased delay to avoid rate limits
        
        print()
    
    save_config(config)
    
    print(f"{C.G}[✓] Created {created}/{attempted} VPS{C.X}\n")
    print(f"{C.G}[✓] Fleet deployment complete!{C.X}")
    pause()

def menu_list_fleet(config: Dict):
    """List fleet inventory"""
    banner()
    print(f"{C.M}═══ FLEET INVENTORY ═══{C.X}\n")
    
    inventory = load_inventory(config)
    
    if not inventory:
        print(f"{C.Y}No VPS in fleet{C.X}")
        pause()
        return
    
    # Group by provider and account
    by_provider = {}
    for entry in inventory:
        provider = entry['provider']
        if provider not in by_provider:
            by_provider[provider] = {}
        
        account = entry['account']
        if account not in by_provider[provider]:
            by_provider[provider][account] = []
        
        by_provider[provider][account].append(entry)
    
    total = len(inventory)
    active = sum(1 for e in inventory if e.get('status') == 'active')
    
    print(f"{C.BOLD}Fleet Summary:{C.X}")
    print(f"  Total:  {C.Y}{total}{C.X} VPS")
    print(f"  Active: {C.G}{active}{C.X} VPS")
    print(f"{C.W}{'─'*70}{C.X}\n")
    
    for provider, accounts in by_provider.items():
        provider_name = PROVIDERS[provider]['name']
        provider_total = sum(len(vps) for vps in accounts.values())
        
        print(f"{C.BOLD}{provider_name}{C.X} ({provider_total} VPS)\n")
        
        for account, entries in accounts.items():
            print(f"  {account} ({len(entries)} VPS):")
            for entry in entries:
                status_color = C.G if entry.get('status') == 'active' else C.Y
                print(f"    • {entry['name']:<25} {entry['ip']:<16} "
                      f"{status_color}{entry.get('status', 'unknown')}{C.X}")
            print()
    
    print(f"{C.W}{'─'*70}{C.X}\n")
    print(f"  {C.C}[1]{C.X} Resave server_ips.txt")
    print(f"  {C.C}[0]{C.X} Back\n")
    
    choice = input(f"Select: ").strip()
    
    if choice == '1':
        save_server_ips(config)
        pause()

# ══════════════════════════════════════════════════════════════════════════════
# DELETE VPS
# ══════════════════════════════════════════════════════════════════════════════

def delete_vps_from_provider(provider: str, token: str, vps_id: int) -> bool:
    """Delete VPS from cloud provider"""
    try:
        if provider == 'digitalocean':
            client = DOClient(token=token)
            client.droplets.destroy(vps_id)
            return True
        elif provider == 'linode':
            client = LinodeClient(token=token)
            # Method 1: Get instance and call delete
            try:
                instance = client.linode.instances(vps_id)
                instance.delete()
                return True
            except AttributeError:
                # Method 2: Use direct API call
                import requests
                headers = {
                    'Authorization': f'Bearer {token}',
                    'Content-Type': 'application/json'
                }
                response = requests.delete(
                    f'https://api.linode.com/v4/linode/instances/{vps_id}',
                    headers=headers
                )
                return response.status_code == 200
    except Exception as e:
        return False
    return False

def menu_delete_vps(config: Dict):
    """Delete VPS menu"""
    while True:
        banner()
        print(f"{C.M}═══ DELETE VPS ═══{C.X}\n")
        
        inventory = load_inventory(config)
        
        if not inventory:
            print(f"{C.Y}No VPS in fleet{C.X}")
            pause()
            return
        
        # Show current fleet
        print(f"{C.BOLD}Current Fleet ({len(inventory)} VPS):{C.X}\n")
        
        # Group by provider and account
        by_account = {}
        for entry in inventory:
            key = f"{entry['provider']}:{entry['account']}"
            if key not in by_account:
                by_account[key] = []
            by_account[key].append(entry)
        
        # Display organized view
        for key, entries in by_account.items():
            provider, account = key.split(':')
            provider_name = PROVIDERS[provider]['name']
            print(f"  {C.C}{account}{C.X} ({provider_name}) - {C.Y}{len(entries)}{C.X} VPS")
            for entry in entries[:3]:  # Show first 3
                print(f"    • {entry['name']} ({entry['ip']})")
            if len(entries) > 3:
                print(f"    ... and {len(entries) - 3} more")
            print()
        
        print(f"{C.W}{'─'*60}{C.X}\n")
        print(f"{C.BOLD}Delete Options:{C.X}\n")
        print(f"  {C.C}[1]{C.X} Delete All VPS (wipe entire fleet)")
        print(f"  {C.C}[2]{C.X} Delete by Account (choose which account)")
        print(f"  {C.C}[3]{C.X} Select Specific VPS to Delete")
        print(f"  {C.C}[0]{C.X} Back\n")
        
        choice = input(f"Select option: ").strip()
        
        if choice == '1':
            delete_all_vps(config)
        elif choice == '2':
            delete_by_account(config)
        elif choice == '3':
            delete_specific_vps(config)
        elif choice == '0':
            break

def delete_all_vps(config: Dict):
    """Delete all VPS"""
    banner()
    print(f"{C.M}═══ DELETE ALL VPS ═══{C.X}\n")
    
    inventory = load_inventory(config)
    
    if not inventory:
        print(f"{C.Y}No VPS to delete{C.X}")
        pause()
        return
    
    print(f"{C.R}⚠️  WARNING: This will DELETE ALL {len(inventory)} VPS! ⚠️{C.X}\n")
    
    # Show exactly what will be deleted
    print(f"{C.BOLD}VPS to be DELETED:{C.X}\n")
    
    by_account = {}
    for entry in inventory:
        acc = entry['account']
        if acc not in by_account:
            by_account[acc] = []
        by_account[acc].append(entry)
    
    for account, entries in by_account.items():
        provider = entries[0]['provider']
        provider_name = PROVIDERS[provider]['name']
        print(f"  {C.R}{account}{C.X} ({provider_name}) - {len(entries)} VPS:")
        for entry in entries:
            print(f"    • {entry['name']:<25} {entry['ip']}")
        print()
    
    total_cost = len(inventory) * 5  # Estimate $5 per VPS
    print(f"  {C.BOLD}Estimated savings:{C.X} ${total_cost}/month\n")
    
    print(f"{C.R}This action CANNOT be undone!{C.X}\n")
    
    confirm = input(f"Type 'DELETE ALL' to confirm: ").strip()
    
    if confirm != 'DELETE ALL':
        print(f"\n{C.Y}Cancelled{C.X}")
        pause()
        return
    
    print(f"\n{C.C}[*] Deleting all VPS (trying all API keys)...{C.X}\n")
    
    deleted = 0
    failed = 0
    
    # Get all tokens by provider
    tokens_by_provider = {}
    for token_info in config['api_tokens']:
        provider = token_info['provider']
        if provider not in tokens_by_provider:
            tokens_by_provider[provider] = []
        tokens_by_provider[provider].append(token_info)
    
    for entry in inventory:
        print(f"  {C.C}[*]{C.X} {entry['name']:<25} {entry['ip']:<16} ... ", end='', flush=True)
        
        provider = entry['provider']
        vps_id = entry['id']
        
        # Try all API keys for this provider until one works
        deleted_success = False
        
        if provider in tokens_by_provider:
            for token_info in tokens_by_provider[provider]:
                try:
                    if delete_vps_from_provider(provider, token_info['token'], vps_id):
                        print(f"{C.G}[✓] ({token_info['label']}){C.X}")
                        deleted += 1
                        deleted_success = True
                        break
                except:
                    continue  # Try next token
        
        if not deleted_success:
            print(f"{C.R}[✗] Not found in any account{C.X}")
            failed += 1
        
        time.sleep(0.5)
    
    # Clear inventory
    save_inventory(config, [])
    save_server_ips(config)
    
    print()
    print(f"{C.W}{'─'*60}{C.X}")
    print(f"  {C.G}Deleted:{C.X} {deleted}")
    if failed > 0:
        print(f"  {C.R}Failed:{C.X}  {failed}")
    print(f"{C.W}{'─'*60}{C.X}\n")
    
    if deleted > 0:
        print(f"{C.G}[✓] Fleet wiped!{C.X}")
        print(f"[✓] Inventory cleared")
        print(f"[✓] server_ips.txt cleared")
        print(f"[✓] Saving ${deleted * 5}/month")
    
    if failed > 0:
        print(f"\n{C.Y}[!] {failed} VPS couldn't be deleted{C.X}")
        print(f"They may have been already deleted manually")
    
    pause()

def delete_by_provider(config: Dict):
    """Delete all VPS from a specific provider"""
    banner()
    print(f"{C.M}═══ DELETE BY PROVIDER ═══{C.X}\n")
    
    inventory = load_inventory(config)
    
    # Group by provider
    by_provider = {}
    for entry in inventory:
        p = entry['provider']
        if p not in by_provider:
            by_provider[p] = []
        by_provider[p].append(entry)
    
    if not by_provider:
        print(f"{C.Y}No VPS to delete{C.X}")
        pause()
        return
    
    print(f"{C.BOLD}Select Provider:{C.X}\n")
    providers_list = list(by_provider.keys())
    
    for i, provider in enumerate(providers_list, 1):
        count = len(by_provider[provider])
        print(f"  {C.C}[{i}]{C.X} {PROVIDERS[provider]['name']} ({count} VPS)")
    
    print()
    choice = input(f"Select provider (1-{len(providers_list)}): ").strip()
    
    if not choice.isdigit() or int(choice) < 1 or int(choice) > len(providers_list):
        return
    
    selected_provider = providers_list[int(choice) - 1]
    vps_to_delete = by_provider[selected_provider]
    
    banner()
    print(f"{C.M}═══ CONFIRM DELETION ═══{C.X}\n")
    print(f"{C.R}Deleting {len(vps_to_delete)} VPS from {PROVIDERS[selected_provider]['name']}{C.X}\n")
    
    for entry in vps_to_delete[:10]:  # Show first 10
        print(f"  • {entry['name']} ({entry['ip']})")
    
    if len(vps_to_delete) > 10:
        print(f"  ... and {len(vps_to_delete) - 10} more")
    
    print()
    confirm = input(f"Type 'DELETE' to confirm: ").strip()
    
    if confirm != 'DELETE':
        print(f"\n{C.Y}Cancelled{C.X}")
        pause()
        return
    
    print(f"\n{C.C}[*] Deleting VPS...{C.X}\n")
    
    deleted = 0
    failed = 0
    tokens_map = {t['label']: t for t in config['api_tokens']}
    
    for entry in vps_to_delete:
        print(f"  {C.C}[*]{C.X} {entry['name']} ... ", end='', flush=True)
        
        token_info = tokens_map.get(entry['account'])
        if token_info and delete_vps_from_provider(entry['provider'], token_info['token'], entry['id']):
            print(f"{C.G}[✓]{C.X}")
            deleted += 1
            inventory.remove(entry)
        else:
            print(f"{C.R}[✗]{C.X}")
            failed += 1
        
        time.sleep(0.5)
    
    save_inventory(config, inventory)
    save_server_ips(config)
    
    print()
    print(f"{C.G}[✓] Deleted {deleted} VPS from {PROVIDERS[selected_provider]['name']}{C.X}")
    pause()

def delete_by_account(config: Dict):
    """Delete all VPS from a specific account"""
    banner()
    print(f"{C.M}═══ DELETE BY ACCOUNT ═══{C.X}\n")
    
    inventory = load_inventory(config)
    
    # Group by account
    by_account = {}
    for entry in inventory:
        acc = entry['account']
        if acc not in by_account:
            by_account[acc] = []
        by_account[acc].append(entry)
    
    if not by_account:
        print(f"{C.Y}No VPS to delete{C.X}")
        pause()
        return
    
    print(f"{C.BOLD}Select Account to DELETE:{C.X}\n")
    accounts_list = list(by_account.keys())
    
    for i, account in enumerate(accounts_list, 1):
        entries = by_account[account]
        provider = entries[0]['provider']
        provider_name = PROVIDERS[provider]['name']
        print(f"  {C.C}[{i}]{C.X} {account} - {provider_name} ({C.Y}{len(entries)} VPS{C.X})")
        
        # Show first 3 VPS in this account
        for entry in entries[:3]:
            print(f"      • {entry['name']} ({entry['ip']})")
        if len(entries) > 3:
            print(f"      ... and {len(entries) - 3} more")
        print()
    
    choice = input(f"Select account (1-{len(accounts_list)}) or 0 to cancel: ").strip()
    
    if choice == '0' or not choice.isdigit() or int(choice) < 1 or int(choice) > len(accounts_list):
        return
    
    selected_account = accounts_list[int(choice) - 1]
    vps_to_delete = by_account[selected_account]
    provider_name = PROVIDERS[vps_to_delete[0]['provider']]['name']
    
    banner()
    print(f"{C.M}═══ CONFIRM DELETION ═══{C.X}\n")
    print(f"{C.R}Deleting {len(vps_to_delete)} VPS from {selected_account} ({provider_name}){C.X}\n")
    
    print(f"{C.BOLD}VPS to be DELETED:{C.X}\n")
    for entry in vps_to_delete:
        print(f"  • {entry['name']:<25} {entry['ip']}")
    
    print()
    confirm = input(f"Type 'DELETE' to confirm: ").strip()
    
    if confirm != 'DELETE':
        print(f"\n{C.Y}Cancelled{C.X}")
        pause()
        return
    
    print(f"\n{C.C}[*] Deleting VPS...{C.X}\n")
    
    deleted = 0
    failed = 0
    tokens_map = {t['label']: t for t in config['api_tokens']}
    token_info = tokens_map.get(selected_account)
    
    if not token_info:
        print(f"{C.R}[✗] No API token found for {selected_account}{C.X}")
        pause()
        return
    
    for entry in vps_to_delete:
        print(f"  {C.C}[*]{C.X} {entry['name']:<25} {entry['ip']:<16} ... ", end='', flush=True)
        
        if delete_vps_from_provider(entry['provider'], token_info['token'], entry['id']):
            print(f"{C.G}[✓]{C.X}")
            deleted += 1
            inventory.remove(entry)
        else:
            print(f"{C.R}[✗]{C.X}")
            failed += 1
        
        time.sleep(0.5)
    
    save_inventory(config, inventory)
    save_server_ips(config)
    
    print()
    print(f"{C.W}{'─'*60}{C.X}")
    print(f"  {C.G}Deleted:{C.X} {deleted}")
    if failed > 0:
        print(f"  {C.R}Failed:{C.X}  {failed}")
    print(f"{C.W}{'─'*60}{C.X}\n")
    
    print(f"{C.G}[✓] Deleted {deleted} VPS from {selected_account}{C.X}")
    print(f"[✓] Updated inventory and server_ips.txt")
    pause()

def delete_specific_vps(config: Dict):
    """Delete specific VPS by selection"""
    banner()
    print(f"{C.M}═══ SELECT VPS TO DELETE ═══{C.X}\n")
    
    inventory = load_inventory(config)
    
    if not inventory:
        print(f"{C.Y}No VPS to delete{C.X}")
        pause()
        return
    
    print(f"{C.BOLD}Current Fleet:{C.X}\n")
    
    for i, entry in enumerate(inventory, 1):
        status_color = C.G if entry.get('status') == 'active' else C.Y
        provider_short = entry['provider'][:2].upper()  # LI for Linode, DI for DigitalOcean
        print(f"  {C.C}[{i:2}]{C.X} {entry['name']:<25} {entry['ip']:<16} "
              f"{status_color}{entry.get('status', 'unknown'):<8}{C.X} "
              f"{C.D}({provider_short}/{entry['account']}){C.X}")
    
    print(f"\n{C.W}{'─'*70}{C.X}\n")
    print(f"{C.BOLD}Selection Options:{C.X}")
    print(f"  • Single:   1")
    print(f"  • Multiple: 1,3,5")
    print(f"  • Range:    5-10")
    print(f"  • Mixed:    1,3,5-8,12")
    print(f"  • All:      all\n")
    
    selection = input(f"Select VPS to delete (or 0 to cancel): ").strip()
    
    if not selection or selection == '0':
        return
    
    # Parse selection
    to_delete = []
    
    if selection.lower() == 'all':
        to_delete = list(range(len(inventory)))
    else:
        try:
            for part in selection.split(','):
                part = part.strip()
                if '-' in part:
                    start, end = part.split('-')
                    to_delete.extend(range(int(start) - 1, int(end)))
                else:
                    to_delete.append(int(part) - 1)
        except:
            print(f"\n{C.R}[✗] Invalid selection format{C.X}")
            pause()
            return
    
    # Filter valid indices and remove duplicates
    to_delete = sorted(list(set([i for i in to_delete if 0 <= i < len(inventory)])))
    
    if not to_delete:
        print(f"\n{C.Y}No valid VPS selected{C.X}")
        pause()
        return
    
    # Show confirmation
    banner()
    print(f"{C.M}═══ CONFIRM DELETION ═══{C.X}\n")
    print(f"{C.R}Deleting {len(to_delete)} VPS:{C.X}\n")
    
    for i in to_delete:
        entry = inventory[i]
        print(f"  • {entry['name']:<25} {entry['ip']:<16} ({entry['account']})")
    
    total_cost = len(to_delete) * 5
    print(f"\n  {C.BOLD}Estimated savings:{C.X} ${total_cost}/month\n")
    
    confirm = input(f"Type 'DELETE' to confirm: ").strip()
    
    if confirm != 'DELETE':
        print(f"\n{C.Y}Cancelled{C.X}")
        pause()
        return
    
    print(f"\n{C.C}[*] Deleting selected VPS...{C.X}\n")
    
    deleted = 0
    failed = 0
    tokens_map = {t['label']: t for t in config['api_tokens']}
    
    # Process in reverse order to maintain indices
    entries_to_delete = [inventory[i] for i in sorted(to_delete, reverse=True)]
    
    for entry in entries_to_delete:
        print(f"  {C.C}[*]{C.X} {entry['name']:<25} {entry['ip']:<16} ... ", end='', flush=True)
        
        token_info = tokens_map.get(entry['account'])
        if token_info and delete_vps_from_provider(entry['provider'], token_info['token'], entry['id']):
            print(f"{C.G}[✓]{C.X}")
            deleted += 1
            inventory.remove(entry)
        else:
            print(f"{C.R}[✗]{C.X}")
            failed += 1
        
        time.sleep(0.5)
    
    save_inventory(config, inventory)
    save_server_ips(config)
    
    print()
    print(f"{C.W}{'─'*60}{C.X}")
    print(f"  {C.G}Deleted:{C.X} {deleted}")
    if failed > 0:
        print(f"  {C.R}Failed:{C.X}  {failed}")
    print(f"{C.W}{'─'*60}{C.X}\n")
    
    print(f"{C.G}[✓] Deleted {deleted} VPS{C.X}")
    print(f"[✓] Updated inventory and server_ips.txt")
    pause()

# ══════════════════════════════════════════════════════════════════════════════
# SYNC FROM CLOUD
# ══════════════════════════════════════════════════════════════════════════════

def sync_from_cloud(config: Dict):
    """Sync VPS from all cloud provider accounts"""
    banner()
    print(f"{C.M}═══ SYNC FROM CLOUD ═══{C.X}\n")
    
    if not config['api_tokens']:
        print(f"{C.Y}No API tokens configured{C.X}")
        pause()
        return
    
    print(f"{C.BOLD}Scanning all accounts for existing VPS...{C.X}\n")
    
    found_by_account = {}
    
    for token_info in config['api_tokens']:
        provider = token_info['provider']
        provider_name = PROVIDERS[provider]['name']
        label = token_info['label']
        
        print(f"{C.C}[{provider_name}]{C.X} {label} ... ", end='', flush=True)
        
        vps_list = []
        
        try:
            if provider == 'digitalocean':
                client = DOClient(token=token_info['token'])
                response = client.droplets.list()
                droplets = response.get('droplets', [])
                
                for droplet in droplets:
                    ip = 'pending'
                    for net in droplet.get('networks', {}).get('v4', []):
                        if net['type'] == 'public':
                            ip = net['ip_address']
                            break
                    
                    vps_list.append({
                        'id': droplet['id'],
                        'name': droplet['name'],
                        'ip': ip,
                        'region': droplet['region']['slug'],
                        'size': droplet['size']['slug'],
                        'status': droplet['status'],
                        'provider': 'digitalocean',
                        'account': label,
                        'created': droplet.get('created_at', datetime.now().isoformat()),
                        'ssh_key': config['ssh_key_path'],
                        'ssh_user': 'root'
                    })
                
            elif provider == 'linode':
                client = LinodeClient(token=token_info['token'])
                instances = client.linode.instances()
                
                for instance in instances:
                    ip = str(instance.ipv4[0]) if instance.ipv4 else 'pending'
                    
                    vps_list.append({
                        'id': instance.id,
                        'name': instance.label,
                        'ip': ip,
                        'region': instance.region.id,
                        'size': instance.type.id,
                        'status': instance.status,
                        'provider': 'linode',
                        'account': label,
                        'created': str(instance.created),
                        'ssh_key': config['ssh_key_path'],
                        'ssh_user': 'root'
                    })
            
            print(f"{C.G}[✓] {len(vps_list)} VPS{C.X}")
            
            if vps_list:
                found_by_account[label] = {
                    'token_info': token_info,
                    'vps': vps_list
                }
                
        except Exception as e:
            print(f"{C.R}[✗] {str(e)[:50]}{C.X}")
        
        time.sleep(0.3)
    
    if not found_by_account:
        print(f"\n{C.Y}No VPS found in any account{C.X}")
        pause()
        return
    
    # Show what was found with details
    banner()
    print(f"{C.M}═══ FOUND VPS ═══{C.X}\n")
    
    total_found = sum(len(data['vps']) for data in found_by_account.values())
    
    for account, data in found_by_account.items():
        vps_list = data['vps']
        provider_name = PROVIDERS[vps_list[0]['provider']]['name']
        print(f"{C.BOLD}{account}{C.X} ({provider_name}) - {C.Y}{len(vps_list)} VPS{C.X}:")
        
        for vps in vps_list:
            status_color = C.G if vps['status'] == 'running' or vps['status'] == 'active' else C.Y
            print(f"  • {vps['name']:<30} {vps['ip']:<16} {status_color}{vps['status']}{C.X}")
        print()
    
    print(f"{C.W}{'─'*70}{C.X}")
    print(f"  {C.BOLD}Total Found:{C.X} {total_found} VPS across {len(found_by_account)} accounts")
    print(f"{C.W}{'─'*70}{C.X}\n")
    
    # Show action options
    print(f"{C.BOLD}What would you like to do?{C.X}\n")
    print(f"  {C.C}[1]{C.X} Add ALL to inventory (merge)")
    print(f"  {C.C}[2]{C.X} Replace inventory with ALL found VPS")
    print(f"  {C.C}[3]{C.X} Delete ALL found VPS (from cloud)")
    print(f"  {C.C}[4]{C.X} Choose per account (advanced)")
    print(f"  {C.C}[0]{C.X} Cancel\n")
    
    choice = input(f"Select: ").strip()
    
    if choice == '1':
        # Merge
        current_inventory = load_inventory(config)
        current_ids = {(v['provider'], v['id']) for v in current_inventory}
        
        added = 0
        for data in found_by_account.values():
            for vps in data['vps']:
                if (vps['provider'], vps['id']) not in current_ids:
                    current_inventory.append(vps)
                    added += 1
        
        save_inventory(config, current_inventory)
        save_server_ips(config)
        
        print(f"\n{C.G}[✓] Added {added} new VPS to inventory{C.X}")
        print(f"[✓] Total inventory: {len(current_inventory)} VPS")
        
    elif choice == '2':
        # Replace
        all_vps = []
        for data in found_by_account.values():
            all_vps.extend(data['vps'])
        
        save_inventory(config, all_vps)
        save_server_ips(config)
        
        print(f"\n{C.G}[✓] Inventory replaced with cloud data{C.X}")
        print(f"[✓] Total inventory: {len(all_vps)} VPS")
        
    elif choice == '3':
        # Delete all from cloud
        banner()
        print(f"{C.M}═══ DELETE ALL FROM CLOUD ═══{C.X}\n")
        print(f"{C.R}⚠️  WARNING: This will DELETE {total_found} VPS from cloud! ⚠️{C.X}\n")
        
        confirm = input(f"Type 'DELETE ALL' to confirm: ").strip()
        
        if confirm == 'DELETE ALL':
            print(f"\n{C.C}[*] Deleting VPS from cloud...{C.X}\n")
            
            deleted = 0
            failed = 0
            
            for account, data in found_by_account.items():
                token_info = data['token_info']
                
                for vps in data['vps']:
                    print(f"  {C.C}[*]{C.X} {vps['name']:<30} {vps['ip']:<16} ... ", end='', flush=True)
                    
                    if delete_vps_from_provider(vps['provider'], token_info['token'], vps['id']):
                        print(f"{C.G}[✓]{C.X}")
                        deleted += 1
                    else:
                        print(f"{C.R}[✗]{C.X}")
                        failed += 1
                    
                    time.sleep(0.5)
            
            # Clear inventory
            save_inventory(config, [])
            save_server_ips(config)
            
            print()
            print(f"{C.G}[✓] Deleted {deleted} VPS from cloud{C.X}")
            print(f"[✓] Inventory cleared")
            
        else:
            print(f"\n{C.Y}Cancelled{C.X}")
    
    elif choice == '4':
        # Per account control
        sync_per_account(config, found_by_account)
    else:
        print(f"\n{C.Y}Cancelled{C.X}")
    
    pause()

def sync_per_account(config: Dict, found_by_account: Dict):
    """Control VPS per account"""
    for account, data in found_by_account.items():
        vps_list = data['vps']
        token_info = data['token_info']
        provider_name = PROVIDERS[vps_list[0]['provider']]['name']
        
        banner()
        print(f"{C.M}═══ {account} ({provider_name}) ═══{C.X}\n")
        
        print(f"{C.BOLD}Found {len(vps_list)} VPS:{C.X}\n")
        for vps in vps_list:
            status_color = C.G if vps['status'] == 'running' or vps['status'] == 'active' else C.Y
            print(f"  • {vps['name']:<30} {vps['ip']:<16} {status_color}{vps['status']}{C.X}")
        
        print(f"\n{C.BOLD}Actions:{C.X}")
        print(f"  {C.C}[1]{C.X} Add to inventory")
        print(f"  {C.C}[2]{C.X} Delete from cloud")
        print(f"  {C.C}[0]{C.X} Skip\n")
        
        choice = input(f"Select: ").strip()
        
        if choice == '1':
            current_inventory = load_inventory(config)
            current_ids = {(v['provider'], v['id']) for v in current_inventory}
            
            added = 0
            for vps in vps_list:
                if (vps['provider'], vps['id']) not in current_ids:
                    current_inventory.append(vps)
                    added += 1
            
            save_inventory(config, current_inventory)
            save_server_ips(config)
            
            print(f"\n{C.G}[✓] Added {added} VPS to inventory{C.X}")
            time.sleep(1)
            
        elif choice == '2':
            print(f"\n{C.R}Delete {len(vps_list)} VPS from {account}?{C.X}")
            confirm = input(f"Type 'DELETE': ").strip()
            
            if confirm == 'DELETE':
                print(f"\n{C.C}[*] Deleting...{C.X}\n")
                
                deleted = 0
                for vps in vps_list:
                    print(f"  {vps['name']} ... ", end='', flush=True)
                    if delete_vps_from_provider(vps['provider'], token_info['token'], vps['id']):
                        print(f"{C.G}[✓]{C.X}")
                        deleted += 1
                    else:
                        print(f"{C.R}[✗]{C.X}")
                    time.sleep(0.5)
                
                print(f"\n{C.G}[✓] Deleted {deleted} VPS{C.X}")
                time.sleep(1)

# ══════════════════════════════════════════════════════════════════════════════
# MAIN MENU
# ══════════════════════════════════════════════════════════════════════════════

def show_main_menu(config: Dict):
    """Display main menu"""
    banner()
    
    inventory = load_inventory(config)
    total_vps = len(inventory)
    
    # Count by provider
    by_provider = {}
    for entry in inventory:
        p = entry['provider']
        by_provider[p] = by_provider.get(p, 0) + 1
    
    if config['api_tokens']:
        total_capacity = sum(t['limit'] for t in config['api_tokens'])
        total_used = sum(t['current'] for t in config['api_tokens'])
        
        # Count accounts by provider
        accounts_by_provider = {}
        for t in config['api_tokens']:
            p = t['provider']
            accounts_by_provider[p] = accounts_by_provider.get(p, 0) + 1
        
        print(f"{C.BOLD}Fleet Status:{C.X}")
        
        provider_status = []
        for provider in PROVIDERS:
            if provider in accounts_by_provider:
                count = accounts_by_provider[provider]
                vps_count = by_provider.get(provider, 0)
                provider_name = PROVIDERS[provider]['name']
                provider_status.append(f"{provider_name} ({count} acct, {vps_count} VPS)")
        
        print(f"  Providers: {', '.join(provider_status)}")
        print(f"  Capacity:  {total_used}/{total_capacity} VPS")
        print(f"  Inventory: {C.Y}{total_vps}{C.X} VPS")
        print(f"  IPs File:  {config.get('server_ips_file', SERVER_IPS_FILE)}")
    else:
        print(f"{C.Y}No API tokens configured{C.X}")
    
    # Show SSH key status
    master_key = config['ssh_key_path']
    master_type = check_key_type(master_key)
    raven_type = check_key_type(RAVEN_SSH_KEY)
    
    print(f"\n{C.BOLD}SSH Keys:{C.X}")
    master_color = C.G if master_type == "RSA" else C.R if "incompatible" in master_type else C.Y
    raven_color = C.G if raven_type == "RSA" else C.R if raven_type == "not found" or "incompatible" in raven_type else C.Y
    print(f"  Master: {master_color}{master_type}{C.X} ({master_key})")
    print(f"  RAVEN:  {raven_color}{raven_type}{C.X} ({RAVEN_SSH_KEY})")
    
    print(f"{C.W}{'─'*60}{C.X}\n")
    
    print(f"{C.BOLD}FLEET MANAGEMENT{C.X}\n")
    print(f"  {C.C}[1]{C.X} Create VPS Fleet")
    print(f"  {C.C}[2]{C.X} List Fleet Inventory")
    print(f"  {C.C}[9]{C.X} Delete VPS (wipe/remove servers)\n")
    
    print(f"{C.BOLD}CONFIGURATION{C.X}\n")
    print(f"  {C.C}[3]{C.X} Manage API Tokens (Add DO/Linode)")
    print(f"  {C.C}[4]{C.X} Sync Master SSH Key to Providers")
    print(f"  {C.C}[5]{C.X} Resave server_ips.txt")
    print(f"  {C.C}[S]{C.X} Sync VPS from Cloud (scan all accounts)\n")
    
    print(f"{C.BOLD}SSH KEY TOOLS{C.X}\n")
    print(f"  {C.C}[6]{C.X} Regenerate SSH Key (new RSA 4096-bit)")
    print(f"  {C.C}[7]{C.X} Copy Key to RAVEN Panel Path")
    print(f"  {C.C}[8]{C.X} Push SSH Key to All VPS (for RAVEN access)")
    print(f"  {C.C}[P]{C.X} Convert Key to PEM Format (fix paramiko)\n")
    
    print(f"  {C.C}[0]{C.X} Exit\n")
    
    return input(f"Select option: ").strip()

# ══════════════════════════════════════════════════════════════════════════════
# MAIN
# ══════════════════════════════════════════════════════════════════════════════

def main():
    """Main program"""
    
    config = load_config()
    
    # First-time setup
    if not config['api_tokens']:
        banner()
        print(f"{C.Y}═══ FIRST-TIME SETUP ═══{C.X}\n")
        print("Welcome to VPS Fleet Manager - Multi-Cloud Edition!\n")
        print("This tool supports DigitalOcean and Linode.\n")
        
        available = []
        if PROVIDERS_AVAILABLE.get('digitalocean'):
            available.append("DigitalOcean")
        if PROVIDERS_AVAILABLE.get('linode'):
            available.append("Linode")
        
        print(f"Available providers: {', '.join(available)}\n")
        
        # Generate SSH key
        print(f"{C.BOLD}Step 1: Generate Master SSH Key{C.X}\n")
        ensure_master_ssh_key(config)
        
        # Add first token
        print(f"\n{C.BOLD}Step 2: Add Your First API Token{C.X}\n")
        
        if len(available) > 1:
            print("Which provider do you want to add first?")
            print(f"  {C.C}[1]{C.X} DigitalOcean")
            print(f"  {C.C}[2]{C.X} Linode\n")
            
            choice = input(f"Select [1]: ").strip() or '1'
            provider = 'digitalocean' if choice == '1' else 'linode'
        else:
            provider = 'digitalocean' if PROVIDERS_AVAILABLE.get('digitalocean') else 'linode'
        
        print(f"\nGet your {PROVIDERS[provider]['name']} API token from:")
        if provider == 'digitalocean':
            print("  https://cloud.digitalocean.com/account/api/tokens")
        else:
            print("  https://cloud.linode.com/profile/tokens")
        print()
        
        token = input(f"{C.C}Enter API token: {C.X}").strip()
        
        if token:
            label = input(f"{C.C}Enter label (optional): {C.X}").strip()
            
            print(f"\n{C.C}[*] Validating...{C.X}")
            if add_api_token(config, provider, token, label):
                save_config(config)
                print(f"\n{C.G}[✓] Setup complete!{C.X}")
                
                # Sync SSH key
                print(f"\n{C.C}[*] Syncing SSH key...{C.X}")
                private_key, public_key = ensure_master_ssh_key(config)
                upload_key_to_provider(provider, token, public_key, label or f"{provider}-1")
                
                pause()
    
    # Main loop
    while True:
        choice = show_main_menu(config)
        
        if choice == '1':
            menu_create_fleet(config)
        elif choice == '2':
            menu_list_fleet(config)
        elif choice == '3':
            menu_manage_apis(config)
        elif choice == '4':
            sync_master_key(config)
        elif choice == '5':
            banner()
            print(f"{C.M}═══ RESAVE FILES ═══{C.X}\n")
            save_server_ips(config)
            print()
            pause()
        elif choice == '6':
            menu_regenerate_key(config)
        elif choice == '7':
            menu_copy_to_raven(config)
        elif choice == '8':
            push_key_to_all_vps(config)
        elif choice == '9':
            menu_delete_vps(config)
        elif choice.lower() == 's':
            sync_from_cloud(config)
        elif choice.lower() == 'p':
            menu_convert_to_pem(config)
        elif choice == '0':
            print(f"\n{C.G}Goodbye!{C.X}\n")
            break

if __name__ == '__main__':
    main()

