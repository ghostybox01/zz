# Gunicorn Configuration for RAVEN X 2.0 Dashboard
# Production-ready settings

import multiprocessing

# Server Socket
bind = "0.0.0.0:5000"
backlog = 2048

# Worker Processes
workers = multiprocessing.cpu_count() * 2 + 1
worker_class = "eventlet"  # For WebSocket support
worker_connections = 1000
max_requests = 1000
max_requests_jitter = 50
timeout = 300
keepalive = 5

# Logging — send to stdout/stderr so systemd-journald captures everything
accesslog = "-"
errorlog = "-"
loglevel = "info"
access_log_format = '%(h)s %(l)s %(u)s %(t)s "%(r)s" %(s)s %(b)s "%(f)s" "%(a)s"'

# Process Naming
proc_name = "reconx-dashboard"

# Server Mechanics
daemon = False
pidfile = None
umask = 0
user = None
group = None
tmp_upload_dir = None

# SSL (uncomment if using HTTPS directly)
# keyfile = "/etc/ssl/private/raven.key"
# certfile = "/etc/ssl/certs/raven.crt"
