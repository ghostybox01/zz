import { useMemo, useState, type ComponentType, type SVGProps } from 'react'
import type { Finding } from '../types'
import { fmtInt } from '../lib/format'
import {
  GlyphAI,
  GlyphAwsSes,
  GlyphAwsDeep,
  GlyphBrevo,
  GlyphGcp,
  GlyphGitHub,
  GlyphMailgun,
  GlyphMandrill,
  GlyphOpenAI,
  GlyphAnthropic,
  GlyphSendGrid,
  GlyphSmtp,
  GlyphStripe,
  GlyphTwilio,
} from './BrandGlyph'

type IconCmp = ComponentType<SVGProps<SVGSVGElement>>

type HubKey =
  | 'aws-ses' | 'stripe' | 'sendgrid' | 'mailgun' | 'brevo' | 'twilio'
  | 'smtp' | 'github' | 'openai' | 'anthropic' | 'gemini' | 'huggingface'
  | 'replicate' | 'postmark' | 'mandrill' | 'resend' | 'sparkpost'
  | 'cloudflare' | 'digitalocean' | 'slack' | 'gcp' | 'other'

type HubMeta = {
  key: HubKey
  label: string
  accent: string
  match: (f: Finding) => boolean
  headline: (rows: readonly Finding[]) => { k: string; v: string }
}

const HUBS: readonly HubMeta[] = [
  {
    key: 'aws-ses', label: 'AWS SES', accent: '#ff9900',
    match: (f) => f.provider === 'AWS' || f.provider === 'AWS SNS',
    headline: (rs) => {
      const max = Math.max(0, ...rs.map((f) => f.details?.sesQuota?.max24h ?? 0))
      if (max > 0) return { k: 'MAX 24h QUOTA', v: fmtInt(max) }
      return { k: 'ACCOUNTS', v: String(rs.length) }
    },
  },
  {
    key: 'stripe', label: 'Stripe', accent: '#635bff',
    match: (f) => f.provider === 'Stripe',
    headline: (rs) => {
      const live = rs.filter((f) => f.details?.stripe?.livemode).length
      if (live > 0) return { k: 'LIVE-MODE KEYS', v: String(live) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'sendgrid', label: 'SendGrid', accent: '#1a82e2',
    match: (f) => f.provider === 'SendGrid',
    headline: (rs) => {
      const d = new Set(rs.flatMap((f) => f.details?.senderDomains ?? [])).size
      if (d > 0) return { k: 'VERIFIED DOMAINS', v: String(d) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'mailgun', label: 'Mailgun', accent: '#f06b66',
    match: (f) => f.provider === 'Mailgun',
    headline: (rs) => {
      const validated = rs.filter((f) => f.details?.validated).length
      if (validated > 0) return { k: 'VALIDATED', v: String(validated) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'brevo', label: 'Brevo', accent: '#0b996e',
    match: (f) => f.provider === 'Brevo',
    headline: (rs) => {
      const credits = rs.reduce((s, f) => s + (f.details?.monthlyCredits ?? 0), 0)
      if (credits > 0) return { k: 'CREDITS / MO', v: fmtInt(credits) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'twilio', label: 'Twilio', accent: '#f22f46',
    match: (f) => f.provider === 'Twilio',
    headline: (rs) => {
      const nums = rs.reduce((s, f) => s + (f.details?.twilio?.numbers ?? 0), 0)
      if (nums > 0) return { k: 'PHONE NUMBERS', v: String(nums) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'github', label: 'GitHub', accent: '#9da5b4',
    match: (f) => f.provider === 'GitHub',
    headline: (rs) => {
      const repos = rs.reduce((s, f) => s + (f.details?.github?.repos ?? 0), 0)
      if (repos > 0) return { k: 'REPOS', v: fmtInt(repos) }
      return { k: 'TOKENS', v: String(rs.length) }
    },
  },
  {
    key: 'openai', label: 'OpenAI', accent: '#10a37f',
    match: (f) => f.provider === 'OpenAI',
    headline: (rs) => {
      const models = rs.reduce((s, f) => s + (f.details?.modelsAvailable ?? 0), 0)
      if (models > 0) return { k: 'MODELS', v: String(models) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'anthropic', label: 'Anthropic', accent: '#cd9d6c',
    match: (f) => f.provider === 'Anthropic',
    headline: (rs) => {
      const models = rs.reduce((s, f) => s + (f.details?.modelsAvailable ?? 0), 0)
      if (models > 0) return { k: 'MODELS', v: String(models) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'gemini', label: 'Gemini', accent: '#4285f4',
    match: (f) => f.provider === 'Gemini',
    headline: (rs) => ({ k: 'KEYS', v: String(rs.length) }),
  },
  {
    key: 'huggingface', label: 'HuggingFace', accent: '#ff9d00',
    match: (f) => f.provider === 'HuggingFace',
    headline: (rs) => {
      const pro = rs.filter((f) => f.details?.hfIsPro).length
      if (pro > 0) return { k: 'PRO ACCOUNTS', v: String(pro) }
      return { k: 'TOKENS', v: String(rs.length) }
    },
  },
  {
    key: 'replicate', label: 'Replicate', accent: '#ee4433',
    match: (f) => f.provider === 'Replicate',
    headline: (rs) => ({ k: 'TOKENS', v: String(rs.length) }),
  },
  {
    key: 'postmark', label: 'Postmark', accent: '#ffbe33',
    match: (f) => f.provider === 'Postmark',
    headline: (rs) => {
      const sent = rs.reduce((s, f) => s + (f.details?.sentLast30d ?? 0), 0)
      if (sent > 0) return { k: 'TOTAL SENT', v: fmtInt(sent) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'mandrill', label: 'Mandrill', accent: '#2a9d8f',
    match: (f) => f.provider === 'Mandrill',
    headline: (rs) => {
      const quota = rs.reduce((s, f) => s + (f.details?.monthlyCredits ?? 0), 0)
      if (quota > 0) return { k: 'HOURLY QUOTA', v: fmtInt(quota) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'resend', label: 'Resend', accent: '#00d084',
    match: (f) => f.provider === 'Resend',
    headline: (rs) => {
      const domains = new Set(rs.flatMap((f) => f.details?.senderDomains ?? [])).size
      if (domains > 0) return { k: 'DOMAINS', v: String(domains) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'sparkpost', label: 'SparkPost', accent: '#fa6423',
    match: (f) => f.provider === 'SparkPost',
    headline: (rs) => {
      const credits = rs.reduce((s, f) => s + (f.details?.monthlyCredits ?? 0), 0)
      if (credits > 0) return { k: 'MONTHLY LIMIT', v: fmtInt(credits) }
      return { k: 'KEYS', v: String(rs.length) }
    },
  },
  {
    key: 'cloudflare', label: 'Cloudflare', accent: '#f48120',
    match: (f) => f.provider === 'Cloudflare',
    headline: (rs) => {
      const zones = rs.reduce((s, f) => s + (f.details?.cfZones ?? 0), 0)
      if (zones > 0) return { k: 'ZONES', v: String(zones) }
      return { k: 'TOKENS', v: String(rs.length) }
    },
  },
  {
    key: 'digitalocean', label: 'DigitalOcean', accent: '#0080ff',
    match: (f) => f.provider === 'DigitalOcean',
    headline: (rs) => ({ k: 'TOKENS', v: String(rs.length) }),
  },
  {
    key: 'slack', label: 'Slack', accent: '#4a154b',
    match: (f) => f.provider === 'Slack',
    headline: (rs) => {
      const teams = new Set(rs.map((f) => f.details?.slackTeam).filter(Boolean)).size
      if (teams > 0) return { k: 'WORKSPACES', v: String(teams) }
      return { k: 'TOKENS', v: String(rs.length) }
    },
  },
  {
    key: 'gcp', label: 'GCP', accent: '#4285f4',
    match: (f) => f.provider === 'GCP',
    headline: (rs) => ({ k: 'KEYS', v: String(rs.length) }),
  },
  {
    key: 'smtp', label: 'SMTP', accent: '#fbbf24',
    match: (f) => f.provider === 'SMTP' || f.provider === 'XSMTP',
    headline: (rs) => {
      const hosts = new Set(rs.map((f) => f.details?.smtp?.host).filter(Boolean)).size
      if (hosts > 0) return { k: 'HOSTS', v: String(hosts) }
      return { k: 'SERVERS', v: String(rs.length) }
    },
  },
  {
    key: 'other', label: 'Other', accent: '#8b5cf6',
    match: (f) => {
      const covered = [
        'AWS', 'AWS SNS', 'Stripe', 'SendGrid', 'Mailgun', 'Brevo', 'Twilio',
        'SMTP', 'XSMTP', 'GitHub', 'OpenAI', 'Anthropic', 'GCP',
        'Gemini', 'HuggingFace', 'Replicate',
        'Postmark', 'Mandrill', 'Resend', 'SparkPost',
        'Cloudflare', 'DigitalOcean', 'Slack',
      ]
      return !covered.includes(f.provider)
    },
    headline: (rs) => ({ k: 'PROVIDERS', v: String(new Set(rs.map((f) => f.provider)).size) }),
  },
]

const PROVIDER_ICONS: Record<HubKey, IconCmp> = {
  'aws-ses':     GlyphAwsSes,
  stripe:        GlyphStripe,
  sendgrid:      GlyphSendGrid,
  mailgun:       GlyphMailgun,
  brevo:         GlyphBrevo,
  twilio:        GlyphTwilio,
  smtp:          GlyphSmtp,
  github:        GlyphGitHub,
  openai:        GlyphOpenAI,
  anthropic:     GlyphAnthropic,
  gemini:        GlyphAI,
  huggingface:   GlyphAI,
  replicate:     GlyphAI,
  postmark:      GlyphSmtp,
  mandrill:      GlyphMandrill,
  resend:        GlyphSmtp,
  sparkpost:     GlyphSmtp,
  cloudflare:    GlyphAwsDeep,
  digitalocean:  GlyphAwsDeep,
  slack:         GlyphAI,
  gcp:           GlyphGcp,
  other:         GlyphAwsDeep,
}

type Props = {
  findings: readonly Finding[]
}

export function DiscoveryHubs({ findings }: Props) {
  const [activeKey, setActiveKey] = useState<HubKey | null>(null)

  const groups = useMemo(() => {
    return HUBS.map((h) => ({ meta: h, rows: findings.filter(h.match) })).filter(
      (g) => g.rows.length > 0,
    )
  }, [findings])

  const active = activeKey ? groups.find((g) => g.meta.key === activeKey) : null

  return (
    <section className="cw-hubs">
      <header className="cw-hubs__head">
        <h3 className="cw-hubs__title">Discovery Hubs</h3>
        <p className="cw-hubs__lede">
          One card per provider with findings. <em>Click any card</em> to drill into the underlying
          finds. Cards light up automatically as scans surface new providers.
        </p>
      </header>

      {groups.length === 0 ? (
        <p className="cw-hubs__empty muted-callout">
          No discoveries yet — hubs will light up as findings stream in.
        </p>
      ) : (
        <div className="cw-hubs__grid">
          {groups.map((g) => {
            const h = g.meta.headline(g.rows)
            return (
              <button
                key={g.meta.key}
                type="button"
                className="cw-hub"
                style={{ borderColor: `color-mix(in srgb, ${g.meta.accent}, transparent 60%)` }}
                onClick={() => setActiveKey(g.meta.key)}
                aria-label={`Open ${g.meta.label} hub`}
              >
                <div className="cw-hub__head">
                  <span
                    className="cw-hub__ico"
                    style={{ background: `color-mix(in srgb, ${g.meta.accent}, transparent 78%)`, color: g.meta.accent }}
                    aria-hidden
                  >
                    {(() => { const Ico = PROVIDER_ICONS[g.meta.key]; return <Ico width={24} height={24} /> })()}
                  </span>
                  <h4 className="cw-hub__label">{g.meta.label}</h4>
                  <span className="cw-hub__count">{g.rows.length}</span>
                </div>
                <div className="cw-hub__metric">
                  <span className="cw-hub__metric-k">{h.k}</span>
                  <strong className="cw-hub__metric-v">{h.v}</strong>
                </div>
                <span className="cw-hub__cta">Click to expand →</span>
              </button>
            )
          })}
        </div>
      )}

      {active && (
        <div
          className="cw-hub-modal__backdrop"
          onClick={() => setActiveKey(null)}
          role="dialog"
          aria-modal="true"
        >
          <div className="cw-hub-modal" onClick={(e) => e.stopPropagation()}>
            <header className="cw-hub-modal__head">
              <div>
                <h3 style={{ margin: 0, color: active.meta.accent }}>{active.meta.label}</h3>
                <p className="muted" style={{ margin: '.2rem 0 0', fontSize: '.8rem' }}>
                  {active.rows.length} finding{active.rows.length === 1 ? '' : 's'}
                </p>
              </div>
              <button type="button" className="btn-glass btn-glass--xs" onClick={() => setActiveKey(null)}>
                Close
              </button>
            </header>
            <ul className="cw-hub-modal__list">
              {active.rows.map((f) => (
                <li key={f.id} className="cw-hub-modal__row">
                  <div className="cw-hub-modal__row-head">
                    <strong>{f.ruleLabel}</strong>
                    <span className={`pill pill--${f.severity === 'critical' || f.severity === 'high' ? 'ok' : 'muted'}`}>
                      {f.severity}
                    </span>
                  </div>
                  <div className="muted mono" style={{ fontSize: '.78rem' }}>{f.hostname}</div>
                  {f.detail && <div className="mono" style={{ fontSize: '.78rem', marginTop: '.25rem' }}>{f.detail}</div>}
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </section>
  )
}
