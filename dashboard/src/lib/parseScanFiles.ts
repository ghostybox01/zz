/** Parsers mapping scanner output files → Finding rows.
 *  Schemas observed in ravenx (main_enhanced.go) + warc.go. Trailing fields rarely contain ':',
 *  so we split from the RIGHT, keeping the URL intact on the left.
 */
import type { Finding, FindingSeverity } from '../types'

export type ScanFileSchema = {
  /** File on the VPS (relative to live-source base URL). */
  file: string
  /** Display addon / provider. */
  provider: string
  /** Default severity for new findings parsed from this file. */
  severity: FindingSeverity
  /** Rule label shown in the table. */
  ruleLabel: string
  /** Trailing colon-separated fields after the source URL. */
  trailingFields: number
  /** Format the detail column from extracted parts. `url` is the source URL (or '' if not present). `raw` is the original line. */
  toDetail: (url: string, parts: string[], raw: string) => string
  /** False if the file does NOT lead with a source URL (e.g. aws_valid.txt is just ak:sk). */
  hasSourceUrl: boolean
}

export const SCAN_FILES: readonly ScanFileSchema[] = [
  {
    file: 'aws_valid.txt',
    provider: 'AWS',
    severity: 'critical',
    ruleLabel: 'AWS access key + secret',
    hasSourceUrl: false,
    trailingFields: 2,
    toDetail: (_u, [ak, sk]) => maskPair(ak, sk),
  },
  {
    file: 'aws_credentials.txt',
    provider: 'AWS',
    severity: 'critical',
    ruleLabel: 'AWS credentials with region',
    hasSourceUrl: true,
    trailingFields: 3,
    toDetail: (_u, [ak, sk, region]) => `${maskPair(ak, sk)} · region ${region ?? '?'}`,
  },
  {
    file: 'aws_deep_scan.txt',
    provider: 'AWS',
    severity: 'critical',
    ruleLabel: 'AWS SES/SNS/Fargate deep scan',
    hasSourceUrl: false,
    trailingFields: 0,
    toDetail: (_u, _p, raw) => raw ?? '',
  },
  {
    file: 'valid_github_token.txt',
    provider: 'GitHub',
    severity: 'critical',
    ruleLabel: 'GitHub personal/app token',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_openai_keys.txt',
    provider: 'OpenAI',
    severity: 'high',
    ruleLabel: 'OpenAI API key (sk-…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_anthropic_keys.txt',
    provider: 'Anthropic',
    severity: 'high',
    ruleLabel: 'Anthropic API key (sk-ant-…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_datadog_keys.txt',
    provider: 'Datadog',
    severity: 'high',
    ruleLabel: 'Datadog API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_google_keys.txt',
    provider: 'Google',
    severity: 'high',
    ruleLabel: 'Google API key (AIza…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_slack_webhooks.txt',
    provider: 'Slack',
    severity: 'medium',
    ruleLabel: 'Slack webhook URL',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [w]) => w ?? '',
  },
  {
    file: 'valid_twilio.txt',
    provider: 'Twilio',
    severity: 'high',
    ruleLabel: 'Twilio Account SID + Auth Token',
    hasSourceUrl: true,
    trailingFields: 2,
    toDetail: (_u, [sid, auth]) => `${sid} · ${maskOne(auth)}`,
  },
  {
    file: 'valid_sendgrid.txt',
    provider: 'SendGrid',
    severity: 'high',
    ruleLabel: 'SendGrid API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_stripe.txt',
    provider: 'Stripe',
    severity: 'critical',
    ruleLabel: 'Stripe secret key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_mailgun.txt',
    provider: 'Mailgun',
    severity: 'high',
    ruleLabel: 'Mailgun API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_telnyx.txt',
    provider: 'Telnyx',
    severity: 'high',
    ruleLabel: 'Telnyx API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_nexmo.txt',
    provider: 'Nexmo',
    severity: 'high',
    ruleLabel: 'Nexmo API key + secret',
    hasSourceUrl: true,
    trailingFields: 2,
    toDetail: (_u, [k, s]) => `${k} · ${maskOne(s)}`,
  },
  {
    file: 'smtp_found.txt',
    provider: 'SMTP',
    severity: 'high',
    ruleLabel: 'SMTP creds in plaintext',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [line]) => line ?? '',
  },
  {
    file: 'spring_actuator_found.txt',
    provider: 'Spring',
    severity: 'medium',
    ruleLabel: 'Spring Boot Actuator exposed',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [tag]) => tag ?? 'ACTUATOR_EXPOSED',
  },
  {
    file: 'private_keys_found.txt',
    provider: 'Private Key',
    severity: 'critical',
    ruleLabel: 'RSA/EC/OPENSSH private key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [tag]) => tag ?? 'PRIVATE_KEY_FOUND',
  },
  {
    file: 'ssh_valid.txt',
    provider: 'SSH',
    severity: 'critical',
    ruleLabel: 'Verified SSH login (host:user:secret)',
    hasSourceUrl: true,
    trailingFields: 3,
    toDetail: (_u, [host, user]) => `${host} · ${user} · ***`,
  },
  {
    file: 'vps_ssh_found.txt',
    provider: 'VPS',
    severity: 'critical',
    ruleLabel: 'VPS root / deploy SSH material',
    hasSourceUrl: true,
    trailingFields: 3,
    toDetail: (_u, [host, user]) => `${host} · ${user} · ***`,
  },
  {
    file: 'ssh_credentials.txt',
    provider: 'SSH',
    severity: 'critical',
    ruleLabel: 'SSH host:user:password or key',
    hasSourceUrl: false,
    trailingFields: 3,
    toDetail: (_u, [host, user]) => `${host ?? '?'} · ${user ?? '?'} · ***`,
  },
  {
    file: 'firebase_found.txt',
    provider: 'Firebase',
    severity: 'medium',
    ruleLabel: 'Firebase config / DB URL',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [v]) => v ?? '',
  },
  {
    file: 'sentry_dsns_found.txt',
    provider: 'Sentry',
    severity: 'low',
    ruleLabel: 'Sentry DSN',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [v]) => v ?? '',
  },
  {
    file: 'backup_files_found.txt',
    provider: 'Backup',
    severity: 'medium',
    ruleLabel: 'Exposed backup / config file',
    hasSourceUrl: true,
    trailingFields: 0,
    toDetail: (u) => u,
  },
  {
    file: 'trufflehog_secrets.txt',
    provider: 'TruffleHog',
    severity: 'high',
    ruleLabel: 'TruffleHog verified secret',
    hasSourceUrl: true,
    trailingFields: 2,
    toDetail: (_u, [det, sec]) => `${det} · ${maskOne(sec)}`,
  },
  {
    file: 'crypto_keys_found.txt',
    provider: 'Crypto',
    severity: 'high',
    ruleLabel: 'Crypto / wallet key material',
    hasSourceUrl: false,
    trailingFields: 0,
    toDetail: (_u, _p, raw) => raw ?? '',
  },
]

/** Files the dashboard polls for activity totals only — not findings. */
export const COUNTER_FILES = {
  liveDomains: 'live_domains.txt',
} as const

function maskOne(s: string | undefined): string {
  if (!s) return ''
  if (s.length <= 10) return s.slice(0, 2) + '***'
  return s.slice(0, 6) + '…' + s.slice(-4)
}

function maskPair(a: string | undefined, b: string | undefined): string {
  return `${maskOne(a)} · ${maskOne(b)}`
}

/** Split a line into `[url, ...trailingParts]` from the RIGHT.
 *  trailingFields=N: takes last N `:`-separated chunks as fields, rest is URL.
 */
function splitRight(line: string, trailingFields: number, hasSourceUrl: boolean): { url: string; parts: string[] } {
  if (!hasSourceUrl) {
    const parts = line.split(':')
    return { url: '', parts: parts.slice(0, trailingFields) }
  }
  if (trailingFields <= 0) return { url: line, parts: [] }
  const idxs: number[] = []
  for (let i = line.length - 1; i >= 0 && idxs.length < trailingFields; i--) {
    if (line[i] === ':') idxs.push(i)
  }
  if (idxs.length < trailingFields) return { url: line, parts: [] }
  idxs.reverse()
  const url = line.slice(0, idxs[0])
  const parts: string[] = []
  for (let i = 0; i < idxs.length; i++) {
    const start = idxs[i]! + 1
    const end = i + 1 < idxs.length ? idxs[i + 1]! : line.length
    parts.push(line.slice(start, end))
  }
  return { url, parts }
}

/** Extract a hostname from a URL for the table column. */
function hostnameFromUrl(url: string): string {
  if (!url) return '(no source)'
  try {
    return new URL(url).host
  } catch {
    const m = url.match(/^https?:\/\/([^/?#]+)/)
    return m ? m[1]! : url.slice(0, 64)
  }
}

/** Parse the full body of one scanner output file into Findings.
 *  `reportedByHost` should identify which VPS supplied this batch.
 */
export function parseScanFile(schema: ScanFileSchema, body: string, reportedByHost: string, nowIso: string): Finding[] {
  const out: Finding[] = []
  const lines = body.split('\n')
  let lineNo = 0
  for (const rawLine of lines) {
    lineNo++
    const line = rawLine.trim()
    if (!line) continue
    const { url, parts } = splitRight(line, schema.trailingFields, schema.hasSourceUrl)
    const detail = schema.toDetail(url, parts, line)
    let path: string | undefined
    if (url) {
      try {
        path = new URL(url).pathname
      } catch {
        path = undefined
      }
    }
    // Full scanner line — Hits + exports show this; `detail` may be masked for display.
    const raw = line
    out.push({
      id: `${schema.file}:${reportedByHost}:${lineNo}`,
      at: nowIso,
      provider: schema.provider,
      ruleLabel: schema.ruleLabel,
      hostname: hostnameFromUrl(url) || reportedByHost,
      url: url || undefined,
      path,
      detail: detail.slice(0, 320),
      severity: schema.severity,
      reportedByHost,
      details: {
        raw,
        validated: schema.file === 'ssh_valid.txt',
      },
    })
  }
  return out
}

/** Count non-empty lines in a body (used for live_domains.txt etc). */
export function countLines(body: string): number {
  if (!body) return 0
  let n = 0
  let started = false
  for (let i = 0; i < body.length; i++) {
    const c = body.charCodeAt(i)
    if (c === 10 /* \n */) {
      if (started) n++
      started = false
    } else if (c !== 13 && c !== 32 && c !== 9) {
      started = true
    }
  }
  if (started) n++
  return n
}
