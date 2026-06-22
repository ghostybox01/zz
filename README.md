# RavenX — Developer Reference

Self-hosted web credential scanner and live validator. Crawls targets, extracts credentials and API keys, validates them against provider APIs in real time, and surfaces findings through a React dashboard. Telegram alerts fire on confirmed hits.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  CONTROLLER VPS                      │
│                                                     │
│  nginx (port 80)                                    │
│    ├── /           → dashboard/dist  (React SPA)    │
│    ├── /api/       → Flask :5000     (gunicorn)     │
│    ├── /socket.io/ → Flask :5000     (websocket)    │
│    └── /fleet-api/ → fleet_api.py :8787 (paramiko)  │
│                                                     │
│  reconx-warc  (Go binary, run on demand)            │
│  redis        (job queue / pub-sub)                 │
└──────────────────────┬──────────────────────────────┘
                       │  SSH (paramiko)
          ┌────────────┼────────────┐
          ▼            ▼            ▼
    Worker VPS   Worker VPS   Worker VPS
    reconx-scanner (Go binary)
    python_job/   (target list slice)
```

The **controller** is the hub. It never scans — it manages the fleet, stores results, and serves the UI. **Workers** receive a target list slice and the Go scanner binary via SFTP, run the scan, and write results back.

---

## Repository Layout

```
/
├── backend/                  Go scanner + Python Flask API
│   ├── main.go               Scanner entry point + core engine
│   ├── detectors_*.go        Credential detectors (one file per domain)
│   ├── app.py                Flask API (served by gunicorn)
│   ├── fleet_api.py          Paramiko fleet control HTTP API
│   ├── ssh_manager.py        SSH/SFTP worker dispatch
│   ├── config.json           Scanner runtime config (created at install)
│   ├── ssh_config.json       Fleet SSH settings (created at install)
│   ├── server_ips.txt        Worker IP list (one per line)
│   ├── requirements.txt      Python deps
│   ├── go.mod / go.sum       Go module manifest
│   └── ResultJS/             Scanner output files (all .txt)
│
├── dashboard/                React + TypeScript frontend
│   ├── src/
│   │   ├── components/       One file per UI panel
│   │   ├── hooks/            Data-fetching hooks (useReconStats, etc.)
│   │   ├── lib/reconApi.ts   All /api/ calls in one place
│   │   └── types.ts          Shared TypeScript types
│   ├── dist/                 Vite build output (nginx serves this)
│   └── package.json
│
├── warc.go                   WARC harvester — main + orchestration
├── warc_producers.go         Additional target producers (Wayback, ASN, CIDR, etc.)
│
├── install-controller.sh     Full VPS installer (idempotent)
├── deploy-from-mac.command   Mac deploy script (double-click in Finder)
├── installer/deploy.py       Python deploy helper
│
└── docs/
    ├── ravenx-internals.html  Full engineering reference (open in browser)
    └── ravenx-scanner-deep-dive.html
```

---

## Scanner Pipeline

Every target URL goes through a single function: `checkAndSaveKeys(content, sourceURL string)` in `main.go`. This is the central dispatcher — everything else hangs off it.

```
Target URL
    │
    ▼
HTTP fetch (rotating UA, redirect follow)
    │
    ├── Headers scanned (Authorization, X-API-Key, etc.)
    │
    ├── Body → checkAndSaveKeys()
    │               │
    │               ├── Regex patterns (300+ per file)
    │               ├── Shannon entropy scan (threshold 4.5 bits/char)
    │               ├── Terraform state parser → recurse
    │               ├── DS_Store binary → extract filenames
    │               └── Match → logValid() → saveIntoFile() → Telegram
    │
    ├── JS files extracted from HTML → fetched → checkAndSaveKeys()
    │       └── Source map URLs followed
    │
    ├── Backup file paths probed (/.env, /.git/config, /wp-config.php, ...)
    │
    └── Package manifests → LIB scanner branch
            ├── OSV CVE batch query (api.osv.dev)
            ├── npm supply chain checks (typosquats, new packages, licenses)
            └── JS SAST patterns (eval, innerHTML, child_process, etc.)
```

### Detector Files

| File | What it finds |
|---|---|
| `main.go` | AWS (4-stage: detect → STS → IAM audit → SES/SNS), core regex engine, backup file scanner |
| `detectors_ai_extended.go` | OpenAI, Anthropic, Gemini, Mistral, Groq, Perplexity, Replicate, Together, Fireworks, ElevenLabs, xAI, HuggingFace, OpenRouter, Cohere |
| `detectors_email_extended.go` | SendGrid, Mailgun, Mandrill, Brevo, Resend, MailerSend, Mailchimp, Postmark |
| `detectors_git_platforms.go` | GitHub (scoped), GitLab, Bitbucket |
| `detectors_mailtrap.go` | Mailtrap SMTP + API |
| `detectors_mailjet.go` | Mailjet |
| `detectors_plivo.go` | Plivo SMS |
| `detectors_postmark.go` | Postmark |
| `detectors_sparkpost.go` | SparkPost |
| `detectors_ssh.go` | SSH private keys, FTP credentials |
| `detectors_webpanels.go` | cPanel, WHM, WordPress admin |
| `detectors_db.go` | MySQL, Postgres, Redis connection strings |
| `detectors_gap_closers.go` | False-positive filter (`isFalsePositiveCred`), test-path detection, entropy hits, generic secret patterns |
| `detectors_osv_cve.go` | CVE lookup via OSV batch API for package manifests |
| `detectors_npm_supply_chain.go` | Typosquats (Levenshtein ≤1), new packages (<30 days), license compliance |
| `detectors_js_sast.go` | JS static analysis: `eval()`, `innerHTML`, `child_process`, `postMessage("*")`, hardcoded PEM keys |

### Output Files

All outputs land in `backend/ResultJS/`. Key ones:

| File | Contents |
|---|---|
| `valid_aws.txt` / `aws_valid.txt` | Confirmed AWS credentials with IAM details |
| `valid_openai.txt`, `valid_anthropic.txt`, ... | Confirmed AI API keys |
| `valid_sendgrid.txt`, `valid_mailgun.txt`, ... | Confirmed email API keys |
| `smtp_valid.txt` | Confirmed SMTP credentials |
| `valid_stripe.txt` | Confirmed Stripe keys |
| `valid_github.txt` | GitHub tokens with scope info |
| `valid_crypto.txt` | Crypto wallet private keys |
| `cve_found.txt` | CVE findings from package manifests |
| `npm_supply_chain.txt` | Typosquat and supply chain hits |
| `js_sast_findings.txt` | JS SAST findings |
| `entropy_found.txt` | High-entropy strings (potential unrecognised secrets) |

---

## Dashboard

React SPA built with Vite. All data comes from polling `/api/` endpoints — no real-time push except for socket.io scan progress events.

**Key panels and their data source:**

| Panel | API endpoint | What it shows |
|---|---|---|
| Findings Board | `/api/findings/hits` | All confirmed hits, filterable by type |
| AI Keys | `/api/findings/hits?type=ai` | Validated AI API keys |
| Email API | `/api/findings/hits?type=email` | Validated email service keys |
| SMTP | `/api/findings/hits?type=smtp` | Validated SMTP credentials |
| Stripe | `/api/findings/hits?type=stripe` | Stripe live/test keys |
| Fleet | `/api/fleet/*` | Worker VPS status, SSH, deploy |
| WARC | `/api/warc/*` | Harvester state, domain counts |
| Logs | `/api/logs` | Live scanner log tail |

**Where to add a new panel:** create a component in `dashboard/src/components/`, add an endpoint handler in `backend/app.py`, register the route in `dashboard/src/lib/reconApi.ts`, add the TypeScript types in `dashboard/src/types.ts`.

---

## WARC Harvester

Separate Go binary (`reconx-warc`) that generates target domain/IP lists for the scanner. Multiple producers run in parallel and write to a shared liveness-tested output file.

**Producers:**

| Flag | Source | How it works |
|---|---|---|
| `-source cc` | Common Crawl | Downloads WARC files, extracts hostnames from `WARC-Target-URI` headers |
| `-source crtsh` | crt.sh | Certificate transparency log — all SANs for a TLD or domain |
| `-source wayback` | Wayback Machine CDX | Searches archived URLs by pattern (`*/.env`, `*/.git/config`, etc.) |
| `-source asn` | BGPView | ASN number → IPv4 prefixes → individual IPs |
| `-source asn-name` | BGPView search | Org keyword → matching ASNs → IPs (e.g. `hostinger`) |
| `-source cidr` | Direct | Expands CIDR range to individual IPs (max /16) |
| `-source ipfile` | File | Reads IPs, FQDNs, or CIDRs from a plain-text file |
| `-crt-org` | crt.sh O= field | Org keyword → all certs ever issued to that org |

**Example invocations:**

```bash
# 10k live domains from Common Crawl
./reconx-warc -max-domains 10000 -output targets.txt

# All Hostinger IPs by name
./reconx-warc -asn-name "hostinger" -max-domains 50000 -insecure -output hostinger.txt

# Search for exposed .env files via Wayback, also scan crt.sh for stripe.com subdomains
./reconx-warc -source "wayback,crtsh" -crt-domain "stripe.com" -max-domains 5000

# Org keyword across both IP and cert sources
./reconx-warc -asn-name "sendgrid" -crt-org "sendgrid" -max-domains 20000
```

Side-channel output files written alongside the main list:
- `env_urls.txt` — full URLs whose paths matched env/config patterns (probe directly, skip rediscovery)
- `crtsh_domains.txt` — crt.sh sourced domains before liveness test

---

## Telegram Alerts

Configured in `backend/config.json` under `telegram.bot_token` and `telegram.chat_id`. Alerts fire on:
- AWS credential confirmed (with IAM identity and account ID)
- High/Critical CVE found in a package manifest
- Each validated email API key
- SMTP credential confirmed

Deduplication is in-process via `SentTelegrams sync.Map` (hash of message content). Does not persist across scanner restarts — same hit can alert again after a restart.

---

## Configuration

`backend/config.json` — created by the installer, not in source control:

```jsonc
{
  "telegram": {
    "bot_token": "...",
    "chat_id": "..."
  },
  "scanning_features": {
    "lib_scan": true,      // OSV CVE + npm supply chain + JS SAST
    "js_scan": true,       // JS file fetching and scanning
    "entropy_scan": true,  // Shannon entropy pass
    "gpl_scan": false      // License compliance (CPU-heavy, off by default)
  },
  "threads": 50,
  "timeout": 10
}
```

---

## Deployment

**First deploy or full restore:**
```bash
# Mac — double-click in Finder, or:
bash deploy-from-mac.command
# Prompts for: VPS IP, SSH user, SSH key path
# Rsyncs everything, runs install-controller.sh on the VPS
```

**What the installer does (idempotent — safe to re-run):**
1. Installs apt packages (Go, Node 20, Python 3, nginx, redis)
2. Creates service user `reconx`
3. Rsyncs source to `/opt/reconx/` (preserves runtime state files)
4. Creates Python venv, installs requirements
5. Builds React dashboard (`npm ci && npm run build`)
6. Builds Go scanner binary (`go build .` — all detector files)
7. Builds Go warc binary (tempdir build, copies `warc.go` + `warc_producers.go`)
8. Generates SSH keypair for fleet operations
9. Writes systemd units (`reconx-dashboard`, `reconx-fleet-api`)
10. Configures and reloads nginx

**Runtime state files — excluded from rsync delete (survive redeploys):**
- `backend/raven_results.db` — SQLite findings database
- `backend/server_ips.txt` — worker IP list
- `backend/warc_state.json` — harvester checkpoint
- `backend/crack_sessions.json`
- `backend/fleet_creds.json`

**Service management on VPS:**
```bash
systemctl status reconx-dashboard
systemctl status reconx-fleet-api
journalctl -u reconx-dashboard -f    # live Flask logs
journalctl -u reconx-fleet-api -f
nginx -t && systemctl reload nginx
```

---

## Development Workflow

**Backend (Go scanner):**
```bash
cd backend
go mod tidy
go build .                            # local build check
GOOS=linux GOARCH=amd64 go build .    # cross-compile for VPS
```

**Backend (Python Flask):**
```bash
cd backend
python3 -m venv venv && source venv/bin/activate
pip install -r requirements.txt
flask --app app run --port 5000       # local dev server
```

**Dashboard:**
```bash
cd dashboard
npm install
npm run dev                           # Vite dev server on :5173, proxies /api/ to :5000
npm run build                         # production build → dist/
```

**WARC harvester:**
```bash
# At repo root (own go.mod, separate from backend)
go mod tidy
go build -o reconx-warc .
./reconx-warc -max-domains 100 -verbose   # quick smoke test
```

**Deploying a change to the VPS:**
```bash
bash deploy-from-mac.command          # rsyncs + re-runs installer
```

---

## Adding a New Credential Type

1. Add regex patterns in the appropriate `detectors_*.go` file (or create a new one)
2. Call `a.logValid("TypeName", details)` on a match — this writes to `ResultJS/valid_typename.txt` and fires Telegram
3. Add a validator function if the credential can be live-checked (follow the pattern in `detectors_email_extended.go`)
4. Add the output file to `backend/app.py` so the Flask API can serve it
5. Add a panel or column to the dashboard if needed

---

## Known Limitations / Backlog

- No authentication on the dashboard — any IP that can reach port 80 has full access. Add nginx basic-auth as a short-term fix.
- Telegram alert batching not implemented — high-volume scans can hit the Bot API rate limit.
- OSV CVE results are not cached between runs — repeated scans re-query api.osv.dev for the same packages.
- Azure credential detection is minimal (no validator).
- GCP coverage is partial (no IAM or Storage bucket audit).
- JS SAST is regex-based — a Semgrep subprocess integration would reduce false positives.
- Concurrent scan operations from multiple dashboard users can conflict on shared output files.
