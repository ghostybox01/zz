import { useMemo, useState, type ComponentType, type SVGProps } from 'react'
import type { Finding } from '../types'
import { fmtInt } from '../lib/format'
import {
  GlyphAI,
  GlyphAwsSes,
  GlyphAwsDeep,
  GlyphBrevo,
  GlyphGitHub,
  GlyphMailgun,
  GlyphSendGrid,
  GlyphSmtp,
  GlyphStripe,
  GlyphTwilio,
} from './BrandGlyph'

type IconCmp = ComponentType<SVGProps<SVGSVGElement>>

type HubKey =
  | 'aws-ses' | 'stripe' | 'sendgrid' | 'mailgun' | 'brevo' | 'twilio'
  | 'smtp' | 'github' | 'openai' | 'anthropic' | 'gcp' | 'other'

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
    match: (f) => f.provider === 'AWS',
    headline: (rs) => {
      const max = Math.max(0, ...rs.map((f) => f.details?.sesQuota?.max24h ?? 0))
      return { k: 'MAX 24h QUOTA', v: max > 0 ? fmtInt(max) : '—' }
    },
  },
  {
    key: 'stripe', label: 'Stripe', accent: '#635bff',
    match: (f) => f.provider === 'Stripe',
    headline: (rs) => {
      const live = rs.filter((f) => f.details?.stripe?.livemode).length
      return { k: 'LIVE-MODE KEYS', v: String(live) }
    },
  },
  {
    key: 'sendgrid', label: 'SendGrid', accent: '#1a82e2',
    match: (f) => f.provider === 'SendGrid',
    headline: (rs) => {
      const d = new Set(rs.flatMap((f) => f.details?.senderDomains ?? [])).size
      return { k: 'VERIFIED DOMAINS', v: String(d) }
    },
  },
  {
    key: 'mailgun', label: 'Mailgun', accent: '#f06b66',
    match: (f) => f.provider === 'Mailgun',
    headline: (rs) => ({ k: 'VALIDATED', v: String(rs.filter((f) => f.details?.validated).length) }),
  },
  {
    key: 'brevo', label: 'Brevo', accent: '#0b996e',
    match: (f) => f.provider === 'Brevo',
    headline: (rs) => {
      const credits = rs.reduce((s, f) => s + (f.details?.monthlyCredits ?? 0), 0)
      return { k: 'CREDITS / MO', v: credits > 0 ? fmtInt(credits) : '—' }
    },
  },
  {
    key: 'twilio', label: 'Twilio', accent: '#f22f46',
    match: (f) => f.provider === 'Twilio',
    headline: (rs) => {
      const nums = rs.reduce((s, f) => s + (f.details?.twilio?.numbers ?? 0), 0)
      return { k: 'PHONE NUMBERS', v: String(nums) }
    },
  },
  {
    key: 'github', label: 'GitHub', accent: '#9da5b4',
    match: (f) => f.provider === 'GitHub',
    headline: (rs) => {
      const repos = rs.reduce((s, f) => s + (f.details?.github?.repos ?? 0), 0)
      return { k: 'REPOS', v: fmtInt(repos) }
    },
  },
  {
    key: 'openai', label: 'OpenAI', accent: '#10a37f',
    match: (f) => f.provider === 'OpenAI',
    headline: (rs) => ({ k: 'MODELS', v: String(rs.reduce((s, f) => s + (f.details?.modelsAvailable ?? 0), 0)) }),
  },
  {
    key: 'anthropic', label: 'Anthropic', accent: '#cd9d6c',
    match: (f) => f.provider === 'Anthropic',
    headline: (rs) => ({ k: 'MODELS', v: String(rs.reduce((s, f) => s + (f.details?.modelsAvailable ?? 0), 0)) }),
  },
  {
    key: 'gcp', label: 'GCP', accent: '#4285f4',
    match: (f) => f.provider === 'GCP',
    headline: (rs) => ({ k: 'KEYS', v: String(rs.length) }),
  },
  {
    key: 'smtp', label: 'SMTP', accent: '#fbbf24',
    match: (f) => f.provider === 'SMTP',
    headline: (rs) => ({ k: 'HOSTS', v: String(new Set(rs.map((f) => f.details?.smtp?.host).filter(Boolean)).size) }),
  },
]

const PROVIDER_ICONS: Record<HubKey, IconCmp> = {
  'aws-ses': GlyphAwsSes,
  stripe: GlyphStripe,
  sendgrid: GlyphSendGrid,
  mailgun: GlyphMailgun,
  brevo: GlyphBrevo,
  twilio: GlyphTwilio,
  smtp: GlyphSmtp,
  github: GlyphGitHub,
  openai: GlyphAI,
  anthropic: GlyphAI,
  gcp: GlyphAwsDeep,
  other: GlyphAwsDeep,
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
