#!/usr/bin/env python3
"""
Raven Go Scanner Wrapper
This Python script acts as a bridge between Raven's Python-based
deployment system and your Go scanner binary.
"""

import subprocess
import sys
import os

def main():
    # Get the targets file from command line or use default
    if len(sys.argv) > 1:
        targets_file = sys.argv[1]
    else:
        targets_file = "targets.txt"
    
    # Check if targets file exists
    if not os.path.exists(targets_file):
        print(f"❌ Targets file not found: {targets_file}")
        sys.exit(1)
    
    # Check if reconx-scanner binary exists
    if not os.path.exists("reconx-scanner"):
        print("❌ reconx-scanner binary not found!")
        print("Make sure setup_scanner.sh was run successfully")
        sys.exit(1)
    
    # Make sure binary is executable
    os.chmod("reconx-scanner", 0o755)
    
    print(f"🚀 Starting Raven Go Scanner...")
    print(f"📋 Targets file: {targets_file}")
    print(f"─" * 60)
    
    # Run the Go scanner
    try:
        result = subprocess.run(
            ["./reconx-scanner", targets_file],
            check=True,
            text=True
        )
        
        print(f"─" * 60)
        print(f"✅ Scanner completed successfully!")
        sys.exit(0)
        
    except subprocess.CalledProcessError as e:
        print(f"─" * 60)
        print(f"❌ Scanner failed with exit code: {e.returncode}")
        sys.exit(e.returncode)
        
    except FileNotFoundError:
        print("❌ Could not execute reconx-scanner binary")
        print("Make sure Go scanner is compiled for Linux")
        sys.exit(1)
        
    except Exception as e:
        print(f"❌ Unexpected error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
