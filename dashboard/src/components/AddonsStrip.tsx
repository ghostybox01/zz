// Created by https://t.me/boxxboyy
import { useEffect, useState, type ComponentType, type SVGProps } from 'react'
import type { ReconScannerConfig, ReconScannerConfigPatch } from '../lib/reconApi'
import { scannerConfig } from '../lib/reconApi'
import {
  getEnabledAddons,
  parseScannerKey,
  type AddonCategory,
  type AddonEntry,
  type CrackerAddonEnabledMap,
} from '../data/addonCatalog'
import {
  BrandLogo,
  GlyphAI,
  GlyphAwsDeep,
  GlyphAwsSes,
  GlyphBrevo,
  GlyphCrypto,
  GlyphGitHub,
  GlyphMailgun,
  GlyphMandrill,
  GlyphSendGrid,
  GlyphSmtp,
  GlyphStripe,
  GlyphTwilio,
} from './BrandGlyph'

type Glyph = ComponentType<SVGProps<SVGSVGElement>>

// Per-id brand metadata (domain for BrandLogo lookup + inline-SVG fallback).
// Only the legacy 11 had hand-rolled glyphs; everything else gets a
// category-derived glyph. New entries can graduate to a custom glyph by
// adding a row here.
const BRAND_BY_ID: Record<string, { domain: string; Glyph: Glyph }> = {
  ai:        { domain: '',                Glyph: GlyphAI },
  ses:       { domain: 'aws.amazon.com',  Glyph: GlyphAwsSes },
  'aws-deep':{ domain: 'aws.amazon.com',  Glyph: GlyphAwsDeep },
  'aws-access': { domain: 'aws.amazon.com', Glyph: GlyphAwsDeep },
  sendgrid:  { domain: 'sendgrid.com',    Glyph: GlyphSendGrid },
  mailgun:   { domain: 'mailgun.com',     Glyph: GlyphMailgun },
  brevo:     { domain: 'brevo.com',       Glyph: GlyphBrevo },
  mandrill:  { domain: 'mailchimp.com',   Glyph: GlyphMandrill },
  mailersend:{ domain: 'mailersend.com',  Glyph: GlyphSendGrid },
  postmark:  { domain: 'postmarkapp.com', Glyph: GlyphSendGrid },
  sparkpost: { domain: 'sparkpost.com',   Glyph: GlyphSendGrid },
  mailtrap:  { domain: 'mailtrap.io',     Glyph: GlyphSendGrid },
  mailjet:   { domain: 'mailjet.com',     Glyph: GlyphSendGrid },
  smtp:      { domain: '',                Glyph: GlyphSmtp },
  stripe:    { domain: 'stripe.com',      Glyph: GlyphStripe },
  'tencent-ses':  { domain: 'cloud.tencent.com', Glyph: GlyphSendGrid },
  socketlabs:     { domain: 'socketlabs.com',    Glyph: GlyphSmtp },
  zeptomail:      { domain: 'zoho.com',          Glyph: GlyphSmtp },
  elasticemail:   { domain: 'elasticemail.com',  Glyph: GlyphSmtp },
  twilio:    { domain: 'twilio.com',      Glyph: GlyphTwilio },
  nexmo:     { domain: 'vonage.com',      Glyph: GlyphTwilio },
  telnyx:    { domain: 'telnyx.com',      Glyph: GlyphTwilio },
  plivo:     { domain: 'plivo.com',       Glyph: GlyphTwilio },
  messagebird:    { domain: 'messagebird.com', Glyph: GlyphTwilio },
  github:    { domain: 'github.com',      Glyph: GlyphGitHub },
  heroku:    { domain: 'heroku.com',      Glyph: GlyphGitHub },
  datadog:   { domain: 'datadoghq.com',   Glyph: GlyphGitHub },
  'crypto-wallet': { domain: '', Glyph: GlyphCrypto },
}

function fallbackGlyphForCategory(category: AddonCategory): Glyph {
  switch (category) {
    case 'ai':         return GlyphAI
    case 'cloud':      return GlyphAwsDeep
    case 'email-api':  return GlyphSendGrid
    case 'smtp':       return GlyphSmtp
    case 'payment':    return GlyphStripe
    case 'sms':        return GlyphTwilio
    case 'vcs':        return GlyphGitHub
    case 'dev':        return GlyphGitHub
    case 'crypto':     return GlyphCrypto
  }
}

export function brandFor(entry: AddonEntry): { domain: string; Glyph: Glyph } {
  return BRAND_BY_ID[entry.id] ?? { domain: '', Glyph: fallbackGlyphForCategory(entry.category) }
}

/** True when the scanner config currently has this addon's `scannerKey`
 *  flag set. Returns false for keys outside the typed config schema
 *  (those are valid catalog entries that the backend whitelist doesn't
 *  yet surface — the tile will show OFF). */
function readFlag(entry: AddonEntry, config: ReconScannerConfig | null): boolean {
  if (!config) return false
  const parsed = parseScannerKey(entry.scannerKey)
  if (!parsed) return false
  const [section, key] = parsed
  const block = (config as unknown as Record<string, Record<string, boolean> | undefined>)[section]
  return !!(block && block[key])
}

type Props = {
  config: ReconScannerConfig | null
  onPatch: (patch: ReconScannerConfigPatch) => void
}

export function AddonsStrip({ config, onPatch }: Props) {
  // Operator's catalog-level enabled map. Same surface Settings writes to.
  // If the backend doesn't surface it (whitelist drops the key) we fall
  // through to `defaultOn` per `getEnabledAddons`.
  const [enabledMap, setEnabledMap] = useState<CrackerAddonEnabledMap | null>(null)

  useEffect(() => {
    let cancelled = false
    scannerConfig.get()
      .then((c) => {
        if (cancelled) return
        const raw = (c as unknown as { cracker_addons?: CrackerAddonEnabledMap }).cracker_addons
        setEnabledMap(raw && typeof raw === 'object' ? raw : null)
      })
      .catch(() => { /* leave null → defaults govern */ })
    return () => { cancelled = true }
  }, [])

  const visible = getEnabledAddons(enabledMap)
  const states = visible.map((a) => ({ entry: a, on: readFlag(a, config) }))
  const selected = states.filter((s) => s.on).length
  // Auto-balance into two rows: ceil(N/2) columns gives a near-equal
  // split for any N (15 → 8/7, 14 → 7/7, 16 → 8/8). CSS variable is
  // consumed by `.cw-addons__row` at viewport ≥1024px; narrower screens
  // fall through to the auto-fit minmax layout in App.css.
  const cols = Math.max(1, Math.ceil(visible.length / 2))

  return (
    <section className="cw-addons">
      <header className="cw-addons__head">
        <div>
          <h3>Addons</h3>
          <p className="muted">Click any tile to flip the matching <code>config.json</code> flag — workers consume it on next deploy.</p>
        </div>
        <span className="cw-addons__count">{selected} / {visible.length} ACTIVE</span>
      </header>
      <div
        className="cw-addons__row"
        style={{ ['--cw-addon-cols' as string]: cols }}
      >
        {states.map(({ entry, on }) => {
          const { domain, Glyph } = brandFor(entry)
          const parsed = parseScannerKey(entry.scannerKey)
          return (
            <button
              key={entry.id}
              type="button"
              className={`cw-addon${on ? ' cw-addon--on' : ''}`}
              onClick={() => {
                if (!parsed) return
                const [section, key] = parsed
                onPatch({ [section]: { [key]: !on } } as ReconScannerConfigPatch)
              }}
              title={config ? `${entry.label} — ${entry.scannerKey}` : `Loading config…  ${entry.label}`}
              aria-pressed={on}
            >
              <span className="cw-addon__logo" aria-hidden>
                {domain ? (
                  <BrandLogo domain={domain} Fallback={Glyph} alt={entry.label} size={42} />
                ) : (
                  <Glyph width={42} height={42} />
                )}
              </span>
              <span className="cw-addon__label">{entry.label}</span>
              <span className={`cw-addon__state cw-addon__state--${on ? 'on' : 'off'}`}>
                {on ? 'ON' : 'OFF'}
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}
