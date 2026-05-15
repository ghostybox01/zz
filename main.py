import paramiko
import os
import subprocess
import threading
import time
import sys

# ================= KONFIGURASI =================
# Path ke Private Key RSA Anda di Windows
SSH_KEY_PATH = r"/root/ssh/1" 
REMOTE_USER = "root" # User default DigitalOcean

# Daftar file yang wajib ada di folder yang sama dengan script ini
LOCAL_GO_FILE = "ravenx.go"
LOCAL_CONFIG_FILE = "config.json"
LOCAL_INPUT_FILE = "result_dedup.txt"
SERVER_LIST_FILE = "server_ips.txt"
# ===============================================

def check_local_files():
    """Memastikan semua file yang dibutuhkan ada sebelum mulai."""
    files = [LOCAL_GO_FILE, LOCAL_CONFIG_FILE, LOCAL_INPUT_FILE, SERVER_LIST_FILE, SSH_KEY_PATH]
    missing = []
    for f in files:
        if not os.path.exists(f):
            missing.append(f)
    
    if missing:
        print(f"[!] Error: File berikut tidak ditemukan:\n{chr(10).join(missing)}")
        sys.exit(1)
    print("[+] Semua file lokal ditemukan. Memulai proses...")

def split_file_dynamic(input_file, num_servers):
    """Membagi file input menjadi bagian sesuai jumlah IP server."""
    print(f"[*] Membagi {input_file} menjadi {num_servers} bagian...")
    # Membersihkan sisa split sebelumnya
    if os.name == 'nt': # Jika di Windows
        subprocess.run(f"del target_part_*", shell=True, stderr=subprocess.DEVNULL)
    else:
        subprocess.run("rm -f target_part_*", shell=True)

    # Menjalankan perintah split (memerlukan git bash/linux environment di windows)
    # Jika tidak ada split, script ini akan mencoba menggunakan python untuk membagi
    try:
        subprocess.run(f"split -n l/{num_servers} -d {input_file} target_part_", shell=True, check=True)
    except:
        print("[!] Gagal menjalankan perintah 'split'. Pastikan Git Bash terinstall atau jalankan di Linux.")
        sys.exit(1)
    
    parts = sorted([f for f in os.listdir('.') if f.startswith("target_part_")])
    return parts

def deploy_to_server(ip, part_file):
    """Proses remote deployment ke DigitalOcean."""
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    
    try:
        print(f"[{ip}] Menghubungkan...")
        ssh.connect(ip, username=REMOTE_USER, key_filename=SSH_KEY_PATH, timeout=15)
        
        sftp = ssh.open_sftp()
        
        # 1. Upload ravenx.go, config.json, dan potongan list
        print(f"[{ip}] Mengunggah file...")
        sftp.put(LOCAL_GO_FILE, "ravenx.go")
        sftp.put(LOCAL_CONFIG_FILE, "config.json")
        sftp.put(part_file, "target_list.txt")
        
        # 2. Script Otomatisasi (Install Go, Mod Init, Run)
        setup_commands = """
        # Update path
        export PATH=$PATH:/usr/local/go/bin:/usr/bin
        
        # Cek dan Install Golang jika belum ada
        if ! command -v go &> /dev/null; then
            echo "Installing Go..."
            apt-get update -y && apt-get install golang-go -y
        fi

        # Setup Go Module
        if [ ! -f "go.mod" ]; then
            go mod init ravenx
        fi
        go mod tidy

        # Jalankan di background
        # -hybrid adalah flag sesuai permintaan Anda
        nohup go run ravenx.go -hybrid target_list.txt > output.log 2>&1 &
        echo "Proses dimulai dengan PID: $!"
        """
        
        # Simpan perintah ke file sh di remote
        with sftp.file("start.sh", "w") as f:
            f.write(setup_commands)
        
        sftp.close()

        # Eksekusi script setup
        print(f"[{ip}] Menjalankan instalasi dan program...")
        ssh.exec_command("chmod +x start.sh && ./start.sh")
        
        # Beri waktu 5 detik lalu intip log
        time.sleep(5)
        stdin, stdout, stderr = ssh.exec_command("tail -n 5 output.log")
        print(f"[{ip}] Log Akhir:\n{stdout.read().decode()}")

    except Exception as e:
        print(f"[{ip}] Error: {str(e)}")
    finally:
        ssh.close()

def main():
    check_local_files()

    # Ambil daftar IP
    with open(SERVER_LIST_FILE, "r") as f:
        ips = [line.strip() for line in f if line.strip()]

    if not ips:
        print("[!] File server_ips.txt kosong.")
        return

    # Bagi file sesuai jumlah IP
    parts = split_file_dynamic(LOCAL_INPUT_FILE, len(ips))

    # Eksekusi massal dengan Threading
    threads = []
    print(f"[*] Memulai deployment ke {len(ips)} server...")
    for i, ip in enumerate(ips):
        if i < len(parts):
            t = threading.Thread(target=deploy_to_server, args=(ip, parts[i]))
            t.start()
            threads.append(t)
            time.sleep(0.5) # Jeda sedikit antar thread agar tidak overload lokal
    
    for t in threads:
        t.join()

    print("\n[V] Selesai! Semua server telah diproses.")

if __name__ == "__main__":
    main()