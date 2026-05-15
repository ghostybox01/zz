# RavenX Scan Dashboard

Frontend-only dashboard for monitoring scans of **your own infrastructure** run by the RavenX scanner.
Polls the scanner's output directory over HTTP and streams new findings into a sortable, filterable hits table.

> **Authorization context:** This dashboard visualises results from `ravenx.go` / `main_enhanced.go`.
> Only point it at scan output produced from assets you own or are authorised to test.

---

## What it shows

- **Overview** — live-domain count, tested vs. extracted totals, throughput, WARC progress.
- **Fleet** — per-VPS health/CPU/RAM tiles (demo data; replaced by live source when wired into a real fleet exporter).
- **Hits** — sortable, filterable findings table, exportable as JSON.
- **Settings** — live-source config, Telegram prefs, vuln rule picker, target-list upload, JSON snapshot import/export.

## Demo mode vs. live mode

By default the app runs a browser-only simulation (`useScanSimulation`) so the UI is populated without a backend.
Toggle **Settings → Live source → Enable live polling** to switch to real data. While live mode is on, the demo simulator
is paused and the table only shows hits parsed from the configured VPS.

---

## Hooking the dashboard to a real scan

The scanner writes append-only `.txt` files to its working directory (one record per line). The dashboard expects an
HTTP endpoint that serves those files at a base URL. Two easy options:

### Option A — nginx static serve (recommended)

```nginx
# /etc/nginx/sites-available/scan-results
server {
    listen 443 ssl http2;
    server_name vps.example.com;

    ssl_certificate     /etc/letsencrypt/live/vps.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/vps.example.com/privkey.pem;

    location /results/ {
        alias /root/scanner/;                     # the dir where ravenx writes valid_*.txt
        autoindex off;
        types { text/plain txt; }
        default_type text/plain;

        # Bearer-token gate — keep these files off the open internet.
        if ($http_authorization != "Bearer REPLACE_ME_LONG_RANDOM") {
            return 401;
        }

        # CORS so the dashboard (served elsewhere) can fetch.
        add_header Access-Control-Allow-Origin  "*";
        add_header Access-Control-Allow-Headers "Authorization";
    }
}
```

Reload nginx and point the dashboard at `https://vps.example.com/results/` with the same bearer token.

### Option B — caddy (simpler)

```caddy
vps.example.com {
    handle /results/* {
        @noAuth not header Authorization "Bearer REPLACE_ME_LONG_RANDOM"
        respond @noAuth "unauthorized" 401

        header Access-Control-Allow-Origin  "*"
        header Access-Control-Allow-Headers "Authorization"
        header Content-Type "text/plain"
        root * /root/scanner
        rewrite * {path}
        file_server
    }
}
```

### Files the dashboard polls

The dashboard fetches the following from `<base>/<file>` each `pollIntervalMs` (default 5 s) and emits a finding for
each **new** line since the last poll. 404s are tolerated (the scanner only creates a file once that detector triggers).

| File | Provider | Schema |
| --- | --- | --- |
| `aws_valid.txt` | AWS | `ak:sk` |
| `aws_credentials.txt` | AWS | `url:ak:sk:region` |
| `aws_deep_scan.txt` | AWS | raw line (SES/SNS/Fargate dump) |
| `valid_github_token.txt` | GitHub | `url:token` |
| `valid_openai_keys.txt` | OpenAI | `url:token` |
| `valid_anthropic_keys.txt` | Anthropic | `url:token` |
| `valid_datadog_keys.txt` | Datadog | `url:token` |
| `valid_google_keys.txt` | Google | `url:token` |
| `valid_discord_webhooks.txt` | Discord | `url:webhook` |
| `valid_slack_webhooks.txt` | Slack | `url:webhook` |
| `valid_twilio.txt` | Twilio | `url:sid:auth` |
| `valid_sendgrid.txt` | SendGrid | `url:key` |
| `valid_stripe.txt` | Stripe | `url:key` |
| `valid_mailgun.txt` | Mailgun | `url:key` |
| `valid_telnyx.txt` | Telnyx | `url:key` |
| `valid_nexmo.txt` | Nexmo | `url:key:secret` |
| `smtp_found.txt` | SMTP | `url:line` |
| `spring_actuator_found.txt` | Spring | `url:tag` |
| `jwt_tokens_found.txt` | JWT | `url:token` |
| `private_keys_found.txt` | Private Key | `url:tag` |
| `firebase_found.txt` | Firebase | `url:value` |
| `sentry_dsns_found.txt` | Sentry | `url:dsn` |
| `backup_files_found.txt` | Backup | `url` |
| `trufflehog_secrets.txt` | TruffleHog | `url:detector:secret` |
| `crypto_keys_found.txt` | Crypto | raw line |
| `live_domains.txt` | _(counter)_ | one host per line — drives the Live Domains KPI |

Sensitive values are masked in the table (`ghp_xx…abcd`) — the original file on the VPS stays untouched.

---

## Local development

```bash
npm install
npm run dev          # http://localhost:5173 with HMR
```

To work against a local fixture instead of a VPS:

```bash
mkdir /tmp/scan-fixture
printf 'AKIATEST:secretsecret\n' >> /tmp/scan-fixture/aws_valid.txt
printf 'https://example.com/.env:gh_REDACTED_DEMO_PLACEHOLDER\n' \
    >> /tmp/scan-fixture/valid_github_token.txt

# In another tab, serve the fixture with CORS open:
npx http-server /tmp/scan-fixture -p 8088 --cors
```

Then in the dashboard's **Settings → Live source**, set:

- Base URL: `http://localhost:8088/`
- Bearer token: _(empty)_
- Poll interval: `2000`

Tick **Enable live polling**. New lines you append to the fixture files show up in the hits table on the next poll.

## Build / deploy

```bash
npm run build        # outputs to dist/
```

The `dist/` directory is plain static HTML/JS — host it anywhere (S3+CloudFront, Netlify, nginx, etc.). The dashboard
talks to your VPS at the configured base URL, so the dashboard itself can live anywhere.

---

## Frontend stack

- React 19 + TypeScript + Vite
- No backend; live mode is a pure `fetch` poller (`src/hooks/useLiveScan.ts`).
- Per-file ETag/`Last-Modified` short-circuits skip unchanged polls; `emittedLines` count dedupes within a file.
- Config (base URL, token, interval, enabled) persists to `localStorage`. The bearer token is stored locally;
  treat the browser profile like any host that holds the credential.

## File map

```
src/
  App.tsx                      # top-level state, demo vs. live wiring
  components/
    LiveSourceSettings.tsx     # base URL / token / interval form
    FindingsBoard.tsx          # sortable hits table
    FleetPanel.tsx             # VPS fleet view (demo data)
    ...
  hooks/
    useLiveScan.ts             # poller that emits findings from valid_*.txt
    useScanSimulation.ts       # browser-only demo simulator (paused in live mode)
  lib/
    liveSource.ts              # config persistence + URL/header helpers
    parseScanFiles.ts          # schemas + line parser for each scanner output file
  data/
    demoFindings.ts            # sample data shown in demo mode
```
