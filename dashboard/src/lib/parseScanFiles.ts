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
    file: 'valid_openai.txt',
    provider: 'OpenAI',
    severity: 'high',
    ruleLabel: 'OpenAI API key (sk-…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_anthropic.txt',
    provider: 'Anthropic',
    severity: 'high',
    ruleLabel: 'Anthropic API key (sk-ant-…)',
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
    file: 'valid_discord_webhooks.txt',
    provider: 'Discord',
    severity: 'medium',
    ruleLabel: 'Discord webhook URL',
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
    file: 'valid_messagebird.txt',
    provider: 'MessageBird',
    severity: 'high',
    ruleLabel: 'MessageBird API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_brevo.txt',
    provider: 'Brevo',
    severity: 'high',
    ruleLabel: 'Brevo / Sendinblue API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_xsmtp.txt',
    provider: 'SMTP',
    severity: 'high',
    ruleLabel: 'SMTP API key (validated)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_mandrill.txt',
    provider: 'Mandrill',
    severity: 'high',
    ruleLabel: 'Mandrill / Mailchimp API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_mailersend.txt',
    provider: 'MailerSend',
    severity: 'high',
    ruleLabel: 'MailerSend API token',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_mailjet.txt',
    provider: 'Mailjet',
    severity: 'high',
    ruleLabel: 'Mailjet API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_mailtrap.txt',
    provider: 'Mailtrap',
    severity: 'high',
    ruleLabel: 'Mailtrap API token',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_postmark.txt',
    provider: 'Postmark',
    severity: 'high',
    ruleLabel: 'Postmark server token',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_sparkpost.txt',
    provider: 'SparkPost',
    severity: 'high',
    ruleLabel: 'SparkPost API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_plivo.txt',
    provider: 'Plivo',
    severity: 'high',
    ruleLabel: 'Plivo Auth ID',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_tencent.txt',
    provider: 'Tencent',
    severity: 'high',
    ruleLabel: 'Tencent SES key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  // ── Wave-9: Extended AI providers ─────────────────────────────────
  {
    file: 'valid_gemini.txt',
    provider: 'Gemini',
    severity: 'high',
    ruleLabel: 'Gemini API key (AIzaSy…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_xai.txt',
    provider: 'xAI',
    severity: 'high',
    ruleLabel: 'xAI/Grok API key (xai-…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_mistral.txt',
    provider: 'Mistral',
    severity: 'high',
    ruleLabel: 'Mistral API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_elevenlabs.txt',
    provider: 'ElevenLabs',
    severity: 'high',
    ruleLabel: 'ElevenLabs API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_groq.txt',
    provider: 'Groq',
    severity: 'high',
    ruleLabel: 'Groq API key (gsk_…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_perplexity.txt',
    provider: 'Perplexity',
    severity: 'high',
    ruleLabel: 'Perplexity API key (pplx-…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_openrouter.txt',
    provider: 'OpenRouter',
    severity: 'high',
    ruleLabel: 'OpenRouter API key (sk-or-v1-…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_huggingface.txt',
    provider: 'HuggingFace',
    severity: 'high',
    ruleLabel: 'HuggingFace token (hf_…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_replicate.txt',
    provider: 'Replicate',
    severity: 'high',
    ruleLabel: 'Replicate API token (r8_…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_cohere.txt',
    provider: 'Cohere',
    severity: 'high',
    ruleLabel: 'Cohere API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_togetherai.txt',
    provider: 'TogetherAI',
    severity: 'high',
    ruleLabel: 'TogetherAI API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_fireworks.txt',
    provider: 'Fireworks',
    severity: 'high',
    ruleLabel: 'Fireworks AI API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  // ── Wave-9: Extended email providers ──────────────────────────────
  {
    file: 'valid_mailchimp.txt',
    provider: 'Mailchimp',
    severity: 'high',
    ruleLabel: 'Mailchimp API key',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
  },
  {
    file: 'valid_resend.txt',
    provider: 'Resend',
    severity: 'high',
    ruleLabel: 'Resend API key (re_…)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
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
    file: 'jwt_tokens_found.txt',
    provider: 'JWT',
    severity: 'medium',
    ruleLabel: 'JWT in response body',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [t]) => maskOne(t),
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
    file: 'backup_files_found.txt',
    provider: 'Backup',
    severity: 'medium',
    ruleLabel: 'Exposed backup / config file',
    hasSourceUrl: true,
    trailingFields: 0,
    toDetail: (u) => u,
  },
  {
    file: 'aws_secrets.txt',
    provider: 'AWS',
    severity: 'critical',
    ruleLabel: 'AWS Secrets Manager secret',
    hasSourceUrl: false,
    trailingFields: 0,
    toDetail: (_u, _p, raw) => raw,
  },
  {
    file: 'aws_ssm.txt',
    provider: 'AWS',
    severity: 'critical',
    ruleLabel: 'AWS SSM Parameter (decrypted)',
    hasSourceUrl: false,
    trailingFields: 0,
    toDetail: (_u, _p, raw) => raw,
  },
  {
    file: 'smtp_valid.txt',
    provider: 'SMTP',
    severity: 'high',
    ruleLabel: 'SMTP credentials (validated)',
    hasSourceUrl: true,
    trailingFields: 1,
    toDetail: (_u, [line]) => line ?? '',
  },
  // ── Database credentials ─────────────────────────────────────────
  {
    file: 'valid_mysql.txt',
    provider: 'MySQL',
    severity: 'critical',
    ruleLabel: 'MySQL credentials (validated)',
    hasSourceUrl: true,
    trailingFields: 5,
    toDetail: (_u, [host, port, user, _pass, dbname]) =>
      `${host ?? '?'}:${port ?? '3306'} · ${user ?? '?'} · db:${dbname ?? '?'} · ***`,
  },
  {
    file: 'valid_postgresql.txt',
    provider: 'PostgreSQL',
    severity: 'critical',
    ruleLabel: 'PostgreSQL credentials (validated)',
    hasSourceUrl: true,
    trailingFields: 5,
    toDetail: (_u, [host, port, user, _pass, dbname]) =>
      `${host ?? '?'}:${port ?? '5432'} · ${user ?? '?'} · db:${dbname ?? '?'} · ***`,
  },
  {
    file: 'valid_redis.txt',
    provider: 'Redis',
    severity: 'critical',
    ruleLabel: 'Redis (validated)',
    hasSourceUrl: true,
    trailingFields: 2,
    toDetail: (_u, [host, port]) => `${host ?? '?'}:${port ?? '6379'}`,
  },
  // ── Web panel credentials ────────────────────────────────────────
  {
    file: 'valid_cpanel.txt',
    provider: 'cPanel',
    severity: 'critical',
    ruleLabel: 'cPanel credentials (validated)',
    hasSourceUrl: true,
    trailingFields: 3,
    toDetail: (_u, [host, user]) => `${host ?? '?'} · ${user ?? '?'} · ***`,
  },
  {
    file: 'valid_ftp.txt',
    provider: 'FTP',
    severity: 'critical',
    ruleLabel: 'FTP credentials (validated)',
    hasSourceUrl: true,
    trailingFields: 4,
    toDetail: (_u, [host, port, user]) => `${host ?? '?'}:${port ?? '21'} · ${user ?? '?'} · ***`,
  },
  {
    file: 'valid_wordpress.txt',
    provider: 'WordPress',
    severity: 'critical',
    ruleLabel: 'WordPress credentials (validated)',
    hasSourceUrl: true,
    trailingFields: 3,
    toDetail: (_u, [host, user]) => `${host ?? '?'} · ${user ?? '?'} · ***`,
  },
]

/** Pre-validation pattern-match files — written before the API call is made.
 *  Credentials here may be expired/revoked but were detected in page content.
 *  Severity is one step lower than the confirmed valid_*.txt equivalents. */
export const FOUND_FILES: readonly ScanFileSchema[] = [
  { file: 'sendgrid_found.txt',    provider: 'SendGrid',    severity: 'medium', ruleLabel: 'SendGrid key detected (unvalidated)',    hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'stripe_found.txt',      provider: 'Stripe',      severity: 'high',   ruleLabel: 'Stripe key detected (unvalidated)',      hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'mailgun_found.txt',     provider: 'Mailgun',     severity: 'medium', ruleLabel: 'Mailgun key detected (unvalidated)',     hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'brevo_found.txt',       provider: 'Brevo',       severity: 'medium', ruleLabel: 'Brevo key detected (unvalidated)',       hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'xsmtp_found.txt',       provider: 'SMTP',        severity: 'medium', ruleLabel: 'SMTP key detected (unvalidated)',        hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'mandrill_found.txt',    provider: 'Mandrill',    severity: 'medium', ruleLabel: 'Mandrill key detected (unvalidated)',    hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'mailersend_found.txt',  provider: 'MailerSend',  severity: 'medium', ruleLabel: 'MailerSend key detected (unvalidated)',  hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'postmark_found.txt',    provider: 'Postmark',    severity: 'medium', ruleLabel: 'Postmark token detected (unvalidated)',  hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'sparkpost_found.txt',   provider: 'SparkPost',   severity: 'medium', ruleLabel: 'SparkPost key detected (unvalidated)',   hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'mailtrap_found.txt',    provider: 'Mailtrap',    severity: 'medium', ruleLabel: 'Mailtrap token detected (unvalidated)',  hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'mailjet_found.txt',     provider: 'Mailjet',     severity: 'medium', ruleLabel: 'Mailjet key detected (unvalidated)',     hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'plivo_found.txt',       provider: 'Plivo',       severity: 'medium', ruleLabel: 'Plivo Auth ID detected (unvalidated)',   hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'twilio_found.txt',      provider: 'Twilio',      severity: 'medium', ruleLabel: 'Twilio SID+token detected (unvalidated)',hasSourceUrl: true, trailingFields: 2, toDetail: (_u, [sid, auth]) => `${sid} · ${maskOne(auth)}` },
  { file: 'nexmo_found.txt',       provider: 'Nexmo',       severity: 'medium', ruleLabel: 'Nexmo key+secret detected (unvalidated)',hasSourceUrl: true, trailingFields: 2, toDetail: (_u, [k, s]) => `${k} · ${maskOne(s)}` },
  { file: 'telnyx_found.txt',      provider: 'Telnyx',      severity: 'medium', ruleLabel: 'Telnyx key detected (unvalidated)',      hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'messagebird_found.txt', provider: 'MessageBird', severity: 'medium', ruleLabel: 'MessageBird key detected (unvalidated)', hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'tencent_found.txt',     provider: 'Tencent',     severity: 'medium', ruleLabel: 'Tencent AKID detected (unvalidated)',    hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'openai_found.txt',      provider: 'OpenAI',      severity: 'medium', ruleLabel: 'OpenAI key detected (unvalidated)',      hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  { file: 'anthropic_found.txt',   provider: 'Anthropic',   severity: 'medium', ruleLabel: 'Anthropic key detected (unvalidated)',   hasSourceUrl: true, trailingFields: 1, toDetail: (_u, [t]) => maskOne(t) },
  // ── Database (unvalidated) ───────────────────────────────────────
  { file: 'database_found.txt',   provider: 'Database',    severity: 'high',   ruleLabel: 'DB credentials detected (unvalidated)',  hasSourceUrl: true, trailingFields: 5, toDetail: (_u, [host, port, user, _p, dbname]) => `${host ?? '?'}:${port ?? '?'} · ${user ?? '?'} · db:${dbname ?? '?'} · ***` },
  { file: 'mysql_found.txt',      provider: 'MySQL',       severity: 'high',   ruleLabel: 'MySQL credentials detected (unvalidated)',hasSourceUrl: true, trailingFields: 5, toDetail: (_u, [host, port, user]) => `${host ?? '?'}:${port ?? '3306'} · ${user ?? '?'} · ***` },
  { file: 'postgresql_found.txt', provider: 'PostgreSQL',  severity: 'high',   ruleLabel: 'PostgreSQL creds detected (unvalidated)', hasSourceUrl: true, trailingFields: 5, toDetail: (_u, [host, port, user]) => `${host ?? '?'}:${port ?? '5432'} · ${user ?? '?'} · ***` },
  { file: 'redis_found.txt',      provider: 'Redis',       severity: 'medium', ruleLabel: 'Redis host detected (unvalidated)',       hasSourceUrl: true, trailingFields: 2, toDetail: (_u, [host, port]) => `${host ?? '?'}:${port ?? '6379'}` },
  // ── Web panels (unvalidated) ─────────────────────────────────────
  { file: 'cpanel_found.txt',     provider: 'cPanel',      severity: 'high',   ruleLabel: 'cPanel credentials detected (unvalidated)',hasSourceUrl: true, trailingFields: 3, toDetail: (_u, [host, user]) => `${host ?? '?'} · ${user ?? '?'} · ***` },
  { file: 'ftp_found.txt',        provider: 'FTP',         severity: 'high',   ruleLabel: 'FTP credentials detected (unvalidated)',  hasSourceUrl: true, trailingFields: 4, toDetail: (_u, [host, port, user]) => `${host ?? '?'}:${port ?? '21'} · ${user ?? '?'} · ***` },
  { file: 'wordpress_found.txt',  provider: 'WordPress',   severity: 'high',   ruleLabel: 'WordPress creds detected (unvalidated)',  hasSourceUrl: true, trailingFields: 3, toDetail: (_u, [host, user]) => `${host ?? '?'} · ${user ?? '?'} · ***` },
  // ── SSH (unvalidated) ────────────────────────────────────────────
  { file: 'ssh_found.txt',        provider: 'SSH',         severity: 'high',   ruleLabel: 'SSH credentials detected (unvalidated)', hasSourceUrl: true, trailingFields: 4, toDetail: (_u, [host, port, user]) => `${host ?? '?'}:${port ?? '22'} · ${user ?? '?'} · ***` },
  { file: 'ssh_keys_found.txt',   provider: 'SSH Key',     severity: 'critical',ruleLabel: 'SSH private key detected',              hasSourceUrl: true, trailingFields: 0, toDetail: (u) => u ? `${u.slice(0, 60)}… [PEM]` : '[SSH KEY]' },
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
