// Created by https://t.me/boxxboyy
//
// Settings governs the canonical addon catalog. The operator's toggle
// state lives in `/api/scanner-config` under the `cracker_addons` key as
// `{ [addonId]: boolean }`. Missing entries fall through to
// `AddonEntry.defaultOn`. The Cracker composer + AddonsStrip both read
// the same map and render only entries the operator has opted in to.

import { useEffect, useMemo, useState } from 'react'
import { scannerConfig } from '../lib/reconApi'
import {
  ADDON_CATALOG,
  isAddonEnabled,
  type AddonCategory,
  type AddonEntry,
  type CrackerAddonEnabledMap,
} from '../data/addonCatalog'

const CATEGORY_LABELS: Record<AddonCategory, string> = {
  ai:         'AI Keys',
  cloud:      'Cloud (AWS)',
  'email-api':'Email APIs',
  smtp:       'SMTP senders',
  payment:    'Payment',
  sms:        'SMS providers',
}

const CATEGORY_ORDER: readonly AddonCategory[] = [
  'ai', 'cloud', 'email-api', 'smtp', 'payment', 'sms',
]

export function CrackerAddonsSettings() {
  const [enabledMap, setEnabledMap] = useState<CrackerAddonEnabledMap | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    scannerConfig.get()
      .then((c) => {
        if (cancelled) return
        const raw = (c as unknown as { cracker_addons?: CrackerAddonEnabledMap }).cracker_addons
        setEnabledMap(raw && typeof raw === 'object' ? { ...raw } : {})
        setError(null)
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message || 'Failed to load addon settings')
      })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [])

  const grouped = useMemo(() => {
    const out: Record<AddonCategory, AddonEntry[]> = {
      ai: [], cloud: [], 'email-api': [], smtp: [], payment: [], sms: [],
    }
    for (const a of ADDON_CATALOG) out[a.category].push(a)
    return out
  }, [])

  function onToggle(id: string, next: boolean) {
    setEnabledMap((prev) => {
      const merged: CrackerAddonEnabledMap = { ...(prev ?? {}), [id]: next }
      // Fire and forget — same shape `R2Settings` uses on patch.
      setSaving(true)
      scannerConfig.update({
        // The scanner-config endpoint silently drops unknown keys via its
        // whitelist schema; HMS Iris extends the schema to include
        // `cracker_addons`. Sending it through the same patch path keeps
        // a single source of truth.
        ...({ cracker_addons: merged } as unknown as Parameters<typeof scannerConfig.update>[0]),
      })
        .then((c) => {
          const raw = (c as unknown as { cracker_addons?: CrackerAddonEnabledMap }).cracker_addons
          if (raw && typeof raw === 'object') {
            setEnabledMap({ ...raw })
          }
          setError(null)
        })
        .catch((e: Error) => setError(e.message || 'Save failed'))
        .finally(() => setSaving(false))
      return merged
    })
  }

  const enabledCount = ADDON_CATALOG.filter((a) => isAddonEnabled(a.id, enabledMap)).length

  return (
    <section className="card-block card-block--tight">
      <header className="card-block__head" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: '.5rem' }}>
        <div>
          <h3 style={{ margin: 0 }}>Cracker addons</h3>
          <p className="card-block__lede card-block__lede--short" style={{ margin: '.2rem 0 0' }}>
            Choose which provider validators the New-Crack composer offers. Toggling here governs visibility everywhere — Composer chips, the addons strip, and per-session config snapshots.
          </p>
        </div>
        <span className="muted" style={{ fontSize: '.7rem', letterSpacing: '.06em', textTransform: 'uppercase' }}>
          {enabledCount} / {ADDON_CATALOG.length} enabled
        </span>
      </header>

      {loading && <p className="muted" style={{ marginTop: '.6rem' }}>Loading…</p>}
      {error && <p className="settings-hint" style={{ color: 'var(--danger)', marginTop: '.5rem' }}>{error}</p>}
      {saving && <p className="muted" style={{ marginTop: '.4rem', fontSize: '.72rem' }}>Saving…</p>}

      {!loading && CATEGORY_ORDER.map((cat) => {
        const entries = grouped[cat]
        if (entries.length === 0) return null
        return (
          <div key={cat} style={{ marginTop: '.85rem' }}>
            <div style={{ fontSize: '.62rem', fontWeight: 700, letterSpacing: '.1em', textTransform: 'uppercase', color: 'var(--muted)', marginBottom: '.35rem' }}>
              {CATEGORY_LABELS[cat]}
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(11rem, 1fr))', gap: '.4rem' }}>
              {entries.map((a) => {
                const on = isAddonEnabled(a.id, enabledMap)
                return (
                  <label
                    key={a.id}
                    className={`cw-toggle cw-toggle--sm${on ? ' cw-toggle--on' : ''}`}
                    style={{ cursor: 'pointer' }}
                    title={`${a.label} — ${a.scannerKey}${a.defaultOn ? ' · default ON' : ' · default OFF'}`}
                  >
                    <input
                      type="checkbox"
                      checked={on}
                      onChange={(e) => onToggle(a.id, e.target.checked)}
                      style={{ display: 'none' }}
                    />
                    <span className="cw-toggle__check" aria-hidden>{on ? '✓' : ''}</span>
                    <span className="cw-toggle__label">{a.label}</span>
                  </label>
                )
              })}
            </div>
          </div>
        )
      })}
    </section>
  )
}
