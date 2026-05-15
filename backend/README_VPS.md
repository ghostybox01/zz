# RAVEN X 2.0 - VPS Control Center

This version includes **remote VPS deployment and management** capabilities via SSH.

## Features

### Local Dashboard (`/`)
- Real-time credential monitoring from `ResultJS/` folder
- WebSocket-based live updates
- Credential type breakdown
- Recent findings list

### VPS Control Center (`/vps`)
- **Remote Server Management**: Start/Stop/Restart scanners on multiple VPS
- **Bulk Operations**: Deploy, start, stop, or restart all servers at once
- **Real-time Monitoring**: See scan progress, hits, and speed across all servers
- **Results Collection**: Pull results from all remote servers to local
- **Diagnostics & Fix**: Auto-diagnose and fix common issues

## Setup

### 1. Install Dependencies

```bash
pip install -r requirements.txt
```

### 2. Configure SSH

Edit `ssh_config.json`:

```json
{
  "ssh_key_path": "/root/ssh/1",          // Path to your SSH private key
  "remote_user": "root",                   // Remote server username
  "server_list_file": "server_ips.txt",    // File containing server IPs
  "work_dir": "/root/python_job",          // Remote working directory
  "results_dir": "./collected_results",    // Local results collection dir
  "result_subdir": "Result",               // Remote results subdirectory
  "batch_size": 100000,                    // Batch size for progress calculation
  "ssh_timeout": 10,                       // SSH connection timeout
  "deploy_package": "scanner_package.tar.gz"  // Deployment package
}
```

### 3. Add Server IPs

Edit `server_ips.txt` (one IP per line):

```
192.168.1.100
192.168.1.101
10.0.0.50
```

### 4. Start the Panel

```bash
python app.py
```

Access:
- **Results Dashboard**: http://localhost:5000
- **VPS Control**: http://localhost:5000/vps

## API Endpoints

### VPS Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/vps/available` | GET | Check if SSH is available |
| `/api/vps/config` | GET/POST | Get or update SSH config |
| `/api/vps/servers` | GET/POST/PUT | Manage server list |
| `/api/vps/status` | GET | Get all servers status |
| `/api/vps/start-all` | POST | Start all servers |
| `/api/vps/stop-all` | POST | Stop all servers |
| `/api/vps/restart-all` | POST | Restart all servers |
| `/api/vps/deploy-all` | POST | Deploy to all servers |
| `/api/vps/collect-all` | POST | Collect results from all |

### Single Server Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/vps/server/<ip>/status` | GET | Get server status |
| `/api/vps/server/<ip>/test` | GET | Test connection |
| `/api/vps/server/<ip>/start` | POST | Start scanner |
| `/api/vps/server/<ip>/stop` | POST | Stop scanner |
| `/api/vps/server/<ip>/restart` | POST | Restart scanner |
| `/api/vps/server/<ip>/logs` | GET | Get server logs |
| `/api/vps/server/<ip>/diagnose` | GET | Run diagnostics |
| `/api/vps/server/<ip>/fix` | POST | Auto-fix issues |
| `/api/vps/server/<ip>/deploy` | POST | Deploy to server |
| `/api/vps/server/<ip>/collect` | POST | Collect results |

## Deployment Package

For the `deploy_package` feature, create a `scanner_package.tar.gz` containing:

```
scanner_package/
├── main.py              # Your scanner script
├── batch_runner.sh      # Batch runner script
├── server_config.sh     # Config (BATCH_SIZE, etc)
├── server_main_list.txt # Target URLs
├── requirements.txt     # Python dependencies
└── ...                  # Other required files
```

## WebSocket Events

### Emit Events
- `vps_request_status` - Request current status
- `vps_start_monitoring` - Start auto-refresh
- `vps_stop_monitoring` - Stop auto-refresh

### Receive Events
- `vps_update` - Status update with servers and stats

## Requirements

- Python 3.8+
- SSH key-based authentication to all VPS
- Redis on remote servers (for progress tracking)
- Flask + Flask-SocketIO
- Paramiko (for SSH)

## Troubleshooting

### SSH Connection Failed
- Check SSH key path in config
- Verify key permissions: `chmod 600 /path/to/key`
- Test manually: `ssh -i /path/to/key root@server_ip`

### No Progress Shown
- Ensure Redis is running on remote servers
- Check if scanner uses Redis for dedup tracking
- Verify `work_dir` path is correct

### Results Not Collecting
- Check `results_dir` and `result_subdir` in config
- Ensure scanner writes to `Result/*.txt` format
- Verify SSH can read remote files
