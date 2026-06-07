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

type VisualGroup = { key: string; label: string; cats: AddonCategory[] }
const VISUAL_GROUPS: VisualGroup[] = [
  { key: 'ai',    label: 'AI',          cats: ['ai'] },
  { key: 'email', label: 'Email & SMTP', cats: ['email-api', 'smtp'] },
  { key: 'cloud', label: 'Cloud',        cats: ['cloud'] },
  { key: 'other', label: 'Pay · SMS · Other', cats: ['payment', 'sms', 'database', 'web-panels', 'infrastructure'] },
]
import {
  BrandLogo,
  GlyphAI,
  GlyphAwsDeep,
  GlyphAwsSes,
  GlyphBrevo,
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
}

function fallbackGlyphForCategory(category: AddonCategory): Glyph {
  switch (category) {
    case 'ai':             return GlyphAI
    case 'cloud':          return GlyphAwsDeep
    case 'email-api':      return GlyphSendGrid
    case 'smtp':           return GlyphSmtp
    case 'payment':        return GlyphStripe
    case 'sms':            return GlyphTwilio
    case 'database':       return GlyphSmtp
    case 'web-panels':     return GlyphSendGrid
    case 'infrastructure': return GlyphAwsDeep
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
  const [enabledMap, setEnabledMap] = useState<CrackerAddonEnabledMap | null>(null)
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => {
    const small = typeof window !== 'undefined' && window.innerWidth < 1024
    return small ? Object.fromEntries(VISUAL_GROUPS.map((g) => [g.key, true])) : {}
  })

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

  return (
    <section className="cw-addons">
      <header className="cw-addons__head">
        <div>
          <h3>Addons</h3>
          <p className="muted">Click any tile to flip the matching <code>config.json</code> flag — workers consume it on next deploy.</p>
        </div>
        <span className="cw-addons__count">{selected} / {visible.length} ACTIVE</span>
      </header>

      {VISUAL_GROUPS.map((group) => {
        const groupStates = states.filter((s) => group.cats.includes(s.entry.category))
        if (groupStates.length === 0) return null
        const isCollapsed = !!collapsed[group.key]
        const onCount = groupStates.filter((s) => s.on).length
        return (
          <div
            key={group.key}
            className="cw-addons__group"
            data-collapsed={isCollapsed ? 'true' : undefined}
          >
            <button
              type="button"
              className="cw-addons__group-head"
              onClick={() => setCollapsed((s) => ({ ...s, [group.key]: !isCollapsed }))}
              aria-expanded={!isCollapsed}
            >
              <span className="cw-addons__group-label">{group.label}</span>
              <span className="cw-addons__group-count">{onCount}/{groupStates.length}</span>
              <span className="cw-addons__group-sep" aria-hidden />
              <span className="cw-addons__group-chevron" aria-hidden>{isCollapsed ? '▸' : '▾'}</span>
            </button>
            <div className="cw-addons__row">
              {groupStates.map(({ entry, on }) => {
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
          </div>
        )
      })}
    </section>
  )
}
