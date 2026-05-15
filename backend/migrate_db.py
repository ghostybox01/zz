#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Database Migration Script for RAVEN X 2.0
Adds missing columns to existing database
"""

import sqlite3
import os
import sys

DB_PATH = 'raven_results.db'

def migrate_database():
    """Add missing columns to existing database"""
    
    if not os.path.exists(DB_PATH):
        print(f"❌ Database {DB_PATH} not found!")
        print("💡 Run the app normally to create a new database")
        return False
    
    try:
        conn = sqlite3.connect(DB_PATH)
        cursor = conn.cursor()
        
        print(f"📊 Migrating database: {DB_PATH}")
        print("=" * 60)
        
        # Check and add smtp_servers_found column
        try:
            cursor.execute("SELECT smtp_servers_found FROM statistics LIMIT 1")
            print("✅ Column 'smtp_servers_found' already exists")
        except sqlite3.OperationalError:
            print("🔄 Adding column 'smtp_servers_found'...")
            cursor.execute("ALTER TABLE statistics ADD COLUMN smtp_servers_found INTEGER DEFAULT 0")
            print("✅ Column 'smtp_servers_found' added successfully")
        
        conn.commit()
        conn.close()
        
        print("=" * 60)
        print("✅ Database migration completed successfully!")
        print("💡 You can now run the dashboard with: python app_realtime.py")
        return True
        
    except Exception as e:
        print(f"❌ Migration failed: {e}")
        print("\n💡 Quick fix: Delete the old database and start fresh")
        print(f"   rm {DB_PATH}")
        print("   python app_realtime.py")
        return False

if __name__ == '__main__':
    print("=" * 60)
    print("🔧 RAVEN X 2.0 - Database Migration Tool")
    print("=" * 60)
    
    success = migrate_database()
    
    sys.exit(0 if success else 1)
