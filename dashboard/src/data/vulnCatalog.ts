/** Catalogue of vulnerability/detector groups for the Detectors UI.
 *  Scope: email senders + payments + SMS (the providers we actively want to look for).
 *
 *  `wired` reflects what the current `ravenx3/main_enhanced.go` actually emits to disk:
 *  - `wired: true`  → scanner writes a dedicated `*_found.txt` / `valid_*.txt` for it; dashboard renders rows from those files via `parseScanFiles`.
 *  - `wired: false` → scanner has the regex pattern but no `saveIntoFile()` call yet, so hits are detected
 *                     but lost. Patch the scanner to enable.
 */
export type VulnRule = {
  id: string
  provider: string
  label: string
  category: 'email' | 'payments' | 'sms' | 'cloud' | 'generic'
  /** Whether the scanner currently saves matches for this rule. */
  wired: boolean
}

export const VULN_CATALOG: readonly VulnRule[] = [
  // ── AWS (multi-service) ─────────────────────────────────────────
  { id: 'aws-access-key',  provider: 'AWS',        label: 'IAM access keys (AKIA…)',        category: 'cloud',    wired: true  },
  { id: 'aws-ses',         provider: 'AWS',        label: 'SES SMTP + send quota probe',    category: 'email',    wired: true  },
  { id: 'aws-sns',         provider: 'AWS',        label: 'SNS publish ability',            category: 'sms',      wired: true  },
  { id: 'aws-deep',        provider: 'AWS',        label: 'Fargate / federation deep scan', category: 'cloud',    wired: true  },

  // ── Email senders (paid plans / recurring) ───────────────────────
  { id: 'sendgrid-api',    provider: 'SendGrid',   label: 'API key + monthly credits',      category: 'email',    wired: true  },
  { id: 'mailgun-key',     provider: 'Mailgun',    label: 'Private API key (key-…)',        category: 'email',    wired: true  },
  { id: 'mailgun-new',     provider: 'Mailgun',    label: 'New domain key format',          category: 'email',    wired: true  },
  { id: 'brevo-key',       provider: 'Brevo',      label: 'xkeysib API key',                category: 'email',    wired: true  },
  { id: 'mandrill-key',    provider: 'Mandrill',   label: 'md-… API key',                   category: 'email',    wired: true  },
  { id: 'sparkpost-key',   provider: 'SparkPost',  label: 'API key',                        category: 'email',    wired: true  },
  { id: 'mailtrap-key',    provider: 'Mailtrap',   label: 'API key',                        category: 'email',    wired: true  },
  { id: 'postmark',        provider: 'Postmark',   label: 'Server token',                   category: 'email',    wired: true  },
  { id: 'mailersend',      provider: 'MailerSend', label: 'mlsn.… API key',                 category: 'email',    wired: true  },
  { id: 'tencent-ses',     provider: 'Tencent',    label: 'Tencent SES (AKID…)',            category: 'email',    wired: false },

  { id: 'mailjet-key',     provider: 'Mailjet',    label: 'API key + secret key',           category: 'email',    wired: true  },
  { id: 'datadog-key',     provider: 'Datadog',    label: 'API key + app key probe',        category: 'cloud',    wired: true  },
  { id: 'heroku-key',      provider: 'Heroku',     label: 'Heroku API key',                 category: 'cloud',    wired: true  },

  // ── Generic SMTP catch-all ──────────────────────────────────────
  { id: 'smtp-plain',      provider: 'SMTP',       label: 'SMTP user:pass in plaintext',    category: 'email',    wired: true  },

  // ── Payments ────────────────────────────────────────────────────
  { id: 'stripe-sk',       provider: 'Stripe',     label: 'sk_live_ secret key + balance',  category: 'payments', wired: true  },

  // ── SMS / Voice ─────────────────────────────────────────────────
  { id: 'twilio-sid',      provider: 'Twilio',     label: 'Account SID + auth token',       category: 'sms',      wired: true  },
  { id: 'nexmo-key',       provider: 'Nexmo',      label: 'Vonage API key + secret',        category: 'sms',      wired: true  },
  { id: 'telnyx-key',      provider: 'Telnyx',     label: 'API key',                        category: 'sms',      wired: true  },
  { id: 'plivo-auth',      provider: 'Plivo',      label: 'Auth ID + token',                category: 'sms',      wired: true  },
  { id: 'messagebird-key', provider: 'MessageBird', label: 'AccessKey pattern',             category: 'sms',      wired: false },
]

export type VulnSelection = Record<string, boolean>

/** Default selection: turn on everything that's actually wired; mute the patch-needed ones. */
export function defaultVulnSelection(on = true): VulnSelection {
  return Object.fromEntries(VULN_CATALOG.map((r) => [r.id, on ? r.wired : false]))
}
