import { useEffect, useState } from 'react'
import { r2, type R2Config } from '../lib/reconApi'

type HealthState = NonNullable<R2Config['state']>

const STATE_LABELS: Record<HealthState, string> = {
  connected: 'connected',
  misconfigured: 'misconfigured',
  unreachable: 'unreachable',
  unknown: 'unknown',
}

// Mirrors the fleet-card status-pill family in App.css. `connected`
// uses the same green tone as a healthy worker; misconfigured = warn
// (orange); unreachable = bad (red); unknown = muted (gray). The dot
// glyph matches the existing live-pill convention.
const STATE_PILL_CLASS: Record<HealthState, string> = {
  connected: 'status-pill status-pill--ok',
  misconfigured: 'status-pill status-pill--warn',
  unreachable: 'status-pill status-pill--bad',
  unknown: 'status-pill status-pill--muted',
}

type R2Creds = Pick<R2Config, 'account_id' | 'access_key_id' | 'secret_access_key' | 'bucket_name'>

export function R2Settings() {
  const [cfg, setCfg] = useState<R2Creds>({
    account_id: '', access_key_id: '', secret_access_key: '', bucket_name: '',
  })
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [configured, setConfigured] = useState(false)
  const [healthState, setHealthState] = useState<HealthState>('unknown')
  const [lastError, setLastError] = useState<string | null>(null)

  useEffect(() => {
    r2.getConfig().then((c) => {
      setConfigured(c.configured)
      setHealthState((c.state ?? 'unknown') as HealthState)
      setLastError(c.last_error ?? null)
      setCfg({ account_id: c.account_id, access_key_id: c.access_key_id, secret_access_key: '', bucket_name: c.bucket_name })
    }).catch(() => {})
  }, [])

  async function save() {
    setStatus('saving')
    try {
      const result = await r2.saveConfig(cfg)
      setConfigured(result.configured)
      setHealthState((result.state ?? 'unknown') as HealthState)
      setLastError(result.last_error ?? null)
      setStatus('saved')
      setTimeout(() => setStatus('idle'), 2000)
    } catch {
      setStatus('error')
    }
  }

  const pillClass = STATE_PILL_CLASS[healthState] || STATE_PILL_CLASS.unknown
  const pillLabel = STATE_LABELS[healthState] || STATE_LABELS.unknown
  const pillTitle = lastError
    ? `R2 health: ${pillLabel} — ${lastError}`
    : `R2 health: ${pillLabel}`

  return (
    <section className="card-block card-block--tight">
      <div className="card-block__head card-block__head--row">
        <div>
          <h2>Cloudflare R2 storage</h2>
          <p className="card-block__lede card-block__lede--short">
            Direct-to-R2 upload — supports 1–2 GB lists, survives tab switching.{' '}
            <span
              className={pillClass}
              style={{ fontSize: '0.75rem', marginRight: '0.35rem' }}
              title={pillTitle}
            >
              ● {pillLabel}
            </span>
            {configured && <span className="pill pill--ok" style={{fontSize:'0.75rem'}}>Configured</span>}
          </p>
        </div>
      </div>
      <div className="kv kv--form">
        {[
          { key: 'account_id', label: 'Account ID', placeholder: 'a1b2c3d4e5f6...' },
          { key: 'access_key_id', label: 'Access Key ID', placeholder: 'R2 API token key ID' },
          { key: 'secret_access_key', label: 'Secret Access Key', placeholder: configured ? '(unchanged — leave blank to keep)' : 'R2 API token secret', type: 'password' },
          { key: 'bucket_name', label: 'Bucket Name', placeholder: 'my-targets-bucket' },
        ].map(({ key, label, placeholder, type }) => (
          <div key={key} className="kv__row">
            <label className="kv__label" htmlFor={`r2-${key}`}>{label}</label>
            <input
              id={`r2-${key}`}
              type={type ?? 'text'}
              className="kv__input"
              placeholder={placeholder}
              value={cfg[key as keyof typeof cfg]}
              onChange={(e) => setCfg((p) => ({ ...p, [key]: e.target.value }))}
            />
          </div>
        ))}
      </div>
      <div className="settings-btn-row">
        <button type="button" className="btn-primary" onClick={save} disabled={status === 'saving'}>
          {status === 'saving' ? 'Saving…' : status === 'saved' ? 'Saved ✓' : status === 'error' ? 'Error — retry' : 'Save R2 config'}
        </button>
      </div>
      {lastError && (healthState === 'unreachable' || healthState === 'misconfigured') && (
        <p className="settings-hint" style={{ color: 'var(--danger)' }}>
          R2 probe error: {lastError}
        </p>
      )}
      <p className="settings-hint">
        Create an R2 API token at Cloudflare Dashboard → R2 → Manage API tokens. Grant Object Read &amp; Write on your bucket.
        Account ID is on the R2 overview page (right sidebar).
      </p>
    </section>
  )
}
