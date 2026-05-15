# Wiring the dashboard to a live RavenX scanner — Compartment 1

This is the minimal integration: the dashboard polls the **append-only `*.txt`
files** that `ravenx3/main_enhanced.go` already writes, plus `live_domains.txt`
from `warc.go`. No code change to the scanner is required for this compartment.

```
┌────────────────────┐    HTTP GET    ┌────────────────────┐
│  RavenX scanner    │ ─────────────► │  nginx / caddy     │
│  writes valid_*.txt│                │  serves /results/  │
│  + *_found.txt to  │                │  as static text    │
│  /root/scanner/    │                └──────────┬─────────┘
└────────────────────┘                           │
                                                 │  fetch(...) every 5s
                                                 ▼
                                       ┌────────────────────┐
                                       │  Dashboard (React) │
                                       │  useLiveScan hook  │
                                       └────────────────────┘
```

## What the scanner currently emits (confirmed against `main_enhanced.go`)

8 files wired to detectors the dashboard renders out-of-the-box:

| File | Schema | Provider |
|---|---|---|
| `aws_valid.txt` | `accessKey:secretKey` | AWS |
| `aws_credentials.txt` | `url:accessKey:secretKey:region` | AWS |
| `aws_deep_scan.txt` | `AWS ak:sk SES: {…} SNS: {…} Fargate: {…}` (raw) | AWS deep |
| `valid_sendgrid.txt` | `url:SG…` | SendGrid |
| `valid_mailgun.txt` | `url:key-…` | Mailgun |
| `valid_stripe.txt` | `url:sk_live_…` | Stripe |
| `valid_twilio.txt` | `url:AC…:authToken` | Twilio |
| `valid_telnyx.txt` | `url:KEY…` | Telnyx |
| `valid_nexmo.txt` | `url:key:secret` | Vonage |
| `smtp_found.txt` | `url:smtp.user:pass` | Generic SMTP |

Plus generic catch-alls (`trufflehog_secrets.txt`, `jwt_tokens_found.txt`,
`private_keys_found.txt`, `firebase_found.txt`, `sentry_dsns_found.txt`,
`spring_actuator_found.txt`, `backup_files_found.txt`, `crypto_keys_found.txt`).

The WARC counter: `live_domains.txt` (one host per line).

## Patch-needed (regex defined but no `saveIntoFile()`)

These are detected in memory but **lost** today — patch the scanner to
`saveIntoFile()` for each, then the dashboard will start picking them up
automatically:

- Brevo (`xkeysib-…`) — line 412
- Mandrill (`md-…`) — line 416
- MailerSend (`mlsn.…`) — line 417
- Tencent SES (`AKID…`) — line 414
- Postmark (UUID server token) — line 457
- Plivo (Auth ID + token) — lines 458–459
- MessageBird (`AccessKey…`) — line 438

The Detectors tab in the dashboard marks each as **patch needed** and disables
them by default to avoid implying they work.

## VPS-side setup

Assumes `ravenx` is running in `/root/scanner/` and writing the output files
there. Adjust paths as needed.

### Option A — nginx

```nginx
# /etc/nginx/sites-available/scan-results
server {
    listen 443 ssl http2;
    server_name vps.example.com;

    ssl_certificate     /etc/letsencrypt/live/vps.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/vps.example.com/privkey.pem;

    # Output dir → /results/*.txt
    location /results/ {
        alias /root/scanner/;
        autoindex off;
        types { text/plain txt; }
        default_type text/plain;

        # Block anything that's not a known scanner output file.
        location ~* "^/results/(aws_(valid|credentials|deep_scan)|valid_[a-z_]+|.*_found|live_domains|crypto_keys_found|trufflehog_secrets)\.txt$" {
            # Bearer-token auth — keep secrets off the open internet.
            if ($http_authorization != "Bearer REPLACE_ME_LONG_RANDOM") {
                return 401;
            }
            add_header Access-Control-Allow-Origin  "*";
            add_header Access-Control-Allow-Headers "Authorization";
            add_header Cache-Control               "no-cache";
        }
    }
}
```

Reload: `sudo nginx -t && sudo systemctl reload nginx`

### Option B — caddy (simpler)

```caddy
vps.example.com {
    handle /results/* {
        @noAuth not header Authorization "Bearer REPLACE_ME_LONG_RANDOM"
        respond @noAuth "unauthorized" 401

        header Access-Control-Allow-Origin  "*"
        header Access-Control-Allow-Headers "Authorization"
        header Content-Type "text/plain"
        header Cache-Control "no-cache"
        root * /root/scanner
        rewrite * {path}
        file_server
    }
}
```

## Dashboard configuration

In the dashboard:

1. Open **Settings → Ingest & live HTTP**
2. Set **Base URL**: `https://vps.example.com/results/`
3. Set **Bearer token**: the random value used in the nginx/caddy config
4. Set **Poll interval**: `5000` ms (matches the scanner's natural append cadence)
5. Tick **Enable live polling**

The dashboard will:
- Issue `HEAD`-equivalent ETag checks each tick
- Pull each output file via `GET <base>/<file>`
- Diff against per-file `emittedLines` counter so each row appears once
- Tolerate `404` (file not yet created — the scanner only writes once a
  detector triggers)
- Drop the bearer token + base URL into `localStorage`; nothing leaves the
  browser unless live polling is on

## Smoke test — local fixture

```bash
mkdir -p /tmp/scan-fixture
cd /tmp/scan-fixture

# AWS hit (no URL)
printf 'AKIATEST1234EXAMPLE:wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n' >> aws_valid.txt

# Stripe hit (url:key)
printf 'https://demo.target.test/.env:sk_test_REDACTED_DEMO_PLACEHOLDER_A\n' >> valid_stripe.txt

# SendGrid hit
printf 'https://newsletter.demo.target.test/api/.env:SG.DEMO.tokentokentokentokentokentokentokentoken\n' >> valid_sendgrid.txt

# Generic SMTP
printf 'https://mail.demo.target.test/wp-config.php.bak:user@demo.target.test:s3cretpassword\n' >> smtp_found.txt

# Live domain counter
printf 'demo.target.test\nnewsletter.demo.target.test\nmail.demo.target.test\n' >> live_domains.txt

# Serve with CORS open (no auth so we can test fast)
npx http-server . -p 8088 --cors -s
```

In dashboard Settings → Base URL `http://localhost:8088/`, bearer blank,
toggle live polling on. The 4 fixture hits should appear within one poll cycle.
Append a new line to any file:

```bash
printf 'https://another.demo.target.test/.env:sk_test_REDACTED_DEMO_PLACEHOLDER_B\n' >> /tmp/scan-fixture/valid_stripe.txt
```

…and it'll show up on the next tick (5s default).

## What this compartment does NOT cover

- **Rich detail metadata** (Stripe `balance`, SES `quota`, Brevo `senderDomains`,
  Twilio `account.status`) — the scanner has this in memory and routes it to
  Telegram, but doesn't write it to disk. The expanded **Finding detail view**
  will render mock data until the scanner is patched to emit a
  `findings.jsonl` — that's Compartment 2.
- **Live KPIs** (parsing/sec, requests/sec, valid vs invalid hosts) — scanner
  only prints these at the end. Compartment 3 will patch the existing 5-second
  progress tick to also write `status.json`, which the dashboard will poll.
- **Per-VPS CPU/RAM/uptime** — neither the scanner nor `main.py` reports this.
  Compartment 4 (a tiny `/status` agent).
- **Add/remove VPS, redeploy, prune** — `main.py` is a one-shot deploy
  script. Compartment 5 replaces it with a controller daemon.

Ship compartment 1 first, verify it works against a real scanner, then we tackle the next gap.
