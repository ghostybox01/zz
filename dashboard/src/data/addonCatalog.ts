// Created by https://t.me/boxxboyy
//
// Canonical addon catalog — single source of truth.
//
// Three UI surfaces (Cracker composer, AddonsStrip, Settings) all import
// from this module. Settings governs which entries the operator has opted
// in to; the Composer renders `defaultOn || enabledMap[id]`. Each entry
// carries a `scannerKey` dotted path that names the flag inside the
// scanner's `config.json` so dispatch can build a per-session config
// snapshot without inventing a second mapping.

export type AddonCategory =
  | 'smtp'
  | 'email-api'
  | 'ai'
  | 'cloud'
  | 'payment'
  | 'sms'
  | 'vcs'
  | 'dev'

export type AddonEntry = {
  /** Stable kebab-case slug — the wire format used in dispatch payloads. */
  id: string
  /** Operator-facing label. */
  label: string
  category: AddonCategory
  /** Dotted path into the scanner's `config.json` (e.g. `api_validation.sendgrid`). */
  scannerKey: string
  /** ON by default per operator policy. */
  defaultOn: boolean
  /** Optional one-liner shown in Settings. */
  note?: string
}

export const ADDON_CATALOG: readonly AddonEntry[] = [
  // ── ON by default ────────────────────────────────────────────────
  { id: 'ai',           label: 'AI Keys',            category: 'ai',        scannerKey: 'api_validation.ai_all',  defaultOn: true },
  { id: 'ses',          label: 'AWS SES',            category: 'cloud',     scannerKey: 'aws_checks.ses',         defaultOn: true },
  { id: 'aws-deep',     label: 'AWS Deep',           category: 'cloud',     scannerKey: 'aws_checks.deep',        defaultOn: true },
  { id: 'aws-access',   label: 'AWS Access Keys',    category: 'cloud',     scannerKey: 'api_validation.aws_access', defaultOn: true },
  { id: 'sendgrid',     label: 'SendGrid',           category: 'email-api', scannerKey: 'api_validation.sendgrid', defaultOn: true },
  { id: 'mailgun',      label: 'Mailgun',            category: 'email-api', scannerKey: 'api_validation.mailgun',  defaultOn: true },
  { id: 'brevo',        label: 'Brevo / Sendinblue', category: 'email-api', scannerKey: 'api_validation.brevo',    defaultOn: true },
  { id: 'mandrill',     label: 'Mandrill',           category: 'email-api', scannerKey: 'api_validation.mandrill', defaultOn: true },
  { id: 'mailersend',   label: 'MailerSend',         category: 'email-api', scannerKey: 'api_validation.mailersend', defaultOn: true },
  { id: 'postmark',     label: 'Postmark',           category: 'email-api', scannerKey: 'api_validation.postmark', defaultOn: true },
  { id: 'sparkpost',    label: 'SparkPost',          category: 'email-api', scannerKey: 'api_validation.sparkpost', defaultOn: true },
  { id: 'mailtrap',     label: 'Mailtrap',           category: 'email-api', scannerKey: 'api_validation.mailtrap', defaultOn: true },
  { id: 'mailjet',      label: 'Mailjet',            category: 'email-api', scannerKey: 'api_validation.mailjet',  defaultOn: true },
  { id: 'smtp',         label: 'Random SMTP',        category: 'smtp',      scannerKey: 'api_validation.smtp',     defaultOn: true },
  { id: 'stripe',       label: 'Stripe',             category: 'payment',   scannerKey: 'api_validation.stripe',   defaultOn: true },

  // ── OFF by default ───────────────────────────────────────────────
  { id: 'tencent-ses',  label: 'Tencent SES',        category: 'email-api', scannerKey: 'api_validation.tencent',  defaultOn: false },
  { id: 'socketlabs',   label: 'SocketLabs',         category: 'smtp',      scannerKey: 'api_validation.socketlabs', defaultOn: false },
  { id: 'zeptomail',    label: 'ZeptoMail',          category: 'smtp',      scannerKey: 'api_validation.zeptomail', defaultOn: false },
  { id: 'elasticemail', label: 'ElasticEmail',       category: 'smtp',      scannerKey: 'api_validation.elasticemail', defaultOn: false },
  { id: 'twilio',       label: 'Twilio',             category: 'sms',       scannerKey: 'api_validation.twilio',   defaultOn: false },
  { id: 'nexmo',        label: 'Nexmo / Vonage',     category: 'sms',       scannerKey: 'api_validation.nexmo',    defaultOn: false },
  { id: 'telnyx',       label: 'Telnyx',             category: 'sms',       scannerKey: 'api_validation.telnyx',   defaultOn: false },
  { id: 'plivo',        label: 'Plivo',              category: 'sms',       scannerKey: 'api_validation.plivo',    defaultOn: false },
  { id: 'messagebird',  label: 'MessageBird',        category: 'sms',       scannerKey: 'api_validation.messagebird', defaultOn: false },
]

/** Operator's persisted toggle dict — `{[id]: boolean}` — overlaid on
 *  `defaultOn`. Persisted in `/api/scanner-config` under the key
 *  `cracker_addons`. When a key is missing for a given id, the entry
 *  falls through to its `defaultOn`. */
export type CrackerAddonEnabledMap = Record<string, boolean>

/** Resolve the operator's effective ON/OFF state for one addon id. */
export function isAddonEnabled(id: string, enabled: CrackerAddonEnabledMap | null | undefined): boolean {
  if (enabled && Object.prototype.hasOwnProperty.call(enabled, id)) {
    return !!enabled[id]
  }
  const entry = ADDON_CATALOG.find((a) => a.id === id)
  return entry ? entry.defaultOn : false
}

/** Filter the catalog to entries the operator has opted in to (or that
 *  default ON). This is what both the Composer and the AddonsStrip render. */
export function getEnabledAddons(enabled: CrackerAddonEnabledMap | null | undefined): readonly AddonEntry[] {
  return ADDON_CATALOG.filter((a) => isAddonEnabled(a.id, enabled))
}

/** Map a `scannerKey` like 'api_validation.sendgrid' into its `[section,
 *  key]` tuple. Used by the per-session config builder. */
export function parseScannerKey(scannerKey: string): [string, string] | null {
  const dot = scannerKey.indexOf('.')
  if (dot <= 0 || dot >= scannerKey.length - 1) return null
  return [scannerKey.slice(0, dot), scannerKey.slice(dot + 1)]
}
