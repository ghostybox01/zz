import type { ComponentType, SVGProps } from 'react'
import type { ReconScannerConfig, ReconScannerConfigPatch } from '../lib/reconApi'
import {
  BrandLogo,
  GlyphAI,
  GlyphAwsDeep,
  GlyphAwsSes,
  GlyphBrevo,
  GlyphGitHub,
  GlyphMailgun,
  GlyphMandrill,
  GlyphSendGrid,
  GlyphSmtp,
  GlyphStripe,
  GlyphTwilio,
} from './BrandGlyph'

type Glyph = ComponentType<SVGProps<SVGSVGElement>>

type AddonDef = {
  id: string
  label: string
  Glyph: Glyph
  domain: string
  section: keyof ReconScannerConfig
  key: string
}

const ADDONS: readonly AddonDef[] = [
  { id: 'ai',        label: 'AI Keys',     Glyph: GlyphAI,       domain: '',               section: 'api_validation',    key: 'ai_all' },
  { id: 'ses',       label: 'AWS SES',     Glyph: GlyphAwsSes,   domain: 'aws.amazon.com', section: 'aws_checks',        key: 'ses_quota_check' },
  { id: 'aws-deep',  label: 'AWS Deep',    Glyph: GlyphAwsDeep,  domain: 'aws.amazon.com', section: 'scanning_features', key: 'aws_main_scan' },
  { id: 'sendgrid',  label: 'SendGrid',    Glyph: GlyphSendGrid, domain: 'sendgrid.com',   section: 'api_validation',    key: 'sendgrid' },
  { id: 'mailgun',   label: 'Mailgun',     Glyph: GlyphMailgun,  domain: 'mailgun.com',    section: 'api_validation',    key: 'mailgun' },
  { id: 'brevo',     label: 'Brevo',       Glyph: GlyphBrevo,    domain: 'brevo.com',      section: 'features',          key: 'brevo' },
  { id: 'mandrill',  label: 'Mandrill',    Glyph: GlyphMandrill, domain: 'mailchimp.com',  section: 'features',          key: 'mandrill' },
  { id: 'stripe',    label: 'Stripe',      Glyph: GlyphStripe,   domain: 'stripe.com',     section: 'api_validation',    key: 'stripe' },
  { id: 'twilio',    label: 'Twilio',      Glyph: GlyphTwilio,   domain: 'twilio.com',     section: 'api_validation',    key: 'twilio' },
  { id: 'github',    label: 'GitHub',      Glyph: GlyphGitHub,   domain: 'github.com',     section: 'scanning_features', key: 'github_token_deep_scan' },
  { id: 'smtp',      label: 'Random SMTP', Glyph: GlyphSmtp,     domain: '',               section: 'scanning_features', key: 'smtp_credentials_scan' },
]

type Props = {
  config: ReconScannerConfig | null
  onPatch: (patch: ReconScannerConfigPatch) => void
}

export function AddonsStrip({ config, onPatch }: Props) {
  const states = ADDONS.map((a) => ({
    ...a,
    on: !!(config && (config[a.section] as Record<string, boolean>)[a.key]),
  }))
  const selected = states.filter((s) => s.on).length

  return (
    <section className="cw-addons">
      <header className="cw-addons__head">
        <div>
          <h3>Addons</h3>
          <p className="muted">Click any tile to flip the matching <code>config.json</code> flag — workers consume it on next deploy.</p>
        </div>
        <span className="cw-addons__count">{selected} / {ADDONS.length} ACTIVE</span>
      </header>
      <div className="cw-addons__row">
        {states.map(({ id, label, Glyph, domain, section, key, on }) => (
          <button
            key={id}
            type="button"
            className={`cw-addon${on ? ' cw-addon--on' : ''}`}
            onClick={() => onPatch({ [section]: { [key]: !on } } as ReconScannerConfigPatch)}
            title={config ? `${label} — ${section}.${key}` : `Loading config…  ${label}`}
            aria-pressed={on}
          >
            <span className="cw-addon__logo" aria-hidden>
              {domain ? (
                <BrandLogo domain={domain} Fallback={Glyph} alt={label} size={42} />
              ) : (
                <Glyph width={42} height={42} />
              )}
            </span>
            <span className="cw-addon__label">{label}</span>
            <span className={`cw-addon__state cw-addon__state--${on ? 'on' : 'off'}`}>
              {on ? 'ON' : 'OFF'}
            </span>
          </button>
        ))}
      </div>
    </section>
  )
}
