import { useEffect, useState } from 'react'
import { r2, type R2Config, type R2Usage } from '../lib/reconApi'

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
  const [usage, setUsage] = useState<R2Usage | null>(null)

  useEffect(() => {
    // Poll every 30 s so the usage bar reflects the backend's monitor
    // cycle without an explicit refresh button. The first render uses the
    // same call to populate the form, then the interval keeps usage live.
    let alive = true
    async function load() {
      try {
        const c = await r2.getConfig()
        if (!alive) return
        setConfigured(c.configured)
        setHealthState((c.state ?? 'unknown') as HealthState)
        setLastError(c.last_error ?? null)
        setUsage(c.usage ?? null)
        setCfg((prev) => ({
          account_id: c.account_id,
          access_key_id: c.access_key_id,
          // Don't overwrite the secret if the operator was mid-edit.
          secret_access_key: prev.secret_access_key,
          bucket_name: c.bucket_name,
        }))
      } catch { /* ignore — health probe will retry */ }
    }
    void load()
    const t = window.setInterval(load, 30_000)
    return () => { alive = false; window.clearInterval(t) }
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

  /** Idempotent CORS install — see /api/r2/cors-setup. The Lists panel's
   * browser-direct PUTs fail with "R2 PUT network error" until the bucket
   * has a CORS rule that permits the dashboard's origin. */
  const [corsStatus, setCorsStatus] = useState<'idle' | 'working' | 'ok' | 'err'>('idle')
  const [corsErr, setCorsErr] = useState<string | null>(null)
  async function setupCors() {
    setCorsStatus('working'); setCorsErr(null)
    try {
      const r = await r2.setupCors()
      if (r.ok) {
        setCorsStatus('ok')
        setTimeout(() => setCorsStatus('idle'), 2500)
      } else {
        setCorsStatus('err'); setCorsErr(r.error ?? 'unknown error')
      }
    } catch (e) {
      setCorsStatus('err'); setCorsErr((e as Error).message)
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
      {usage && usage.error == null && (
        <UsageBar usage={usage} />
      )}
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
        <button
          type="button"
          className="btn-glass"
          onClick={() => void setupCors()}
          disabled={corsStatus === 'working' || healthState !== 'connected'}
          title={
            healthState !== 'connected'
              ? 'Connect R2 first (save credentials), then click here'
              : 'Install a CORS rule on the bucket so the Lists panel can upload target lists directly to R2'
          }
        >
          {corsStatus === 'working'
            ? 'Installing CORS…'
            : corsStatus === 'ok'
              ? 'CORS installed ✓'
              : corsStatus === 'err'
                ? 'CORS install failed — retry'
                : 'Allow browser uploads (CORS)'}
        </button>
      </div>
      {corsStatus === 'err' && corsErr && (
        <p className="settings-hint" style={{ color: 'var(--danger)' }}>
          CORS install error: {corsErr}
        </p>
      )}
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

function formatBytes(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 ** 2) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(1)} MB`
  return `${(b / 1024 ** 3).toFixed(2)} GB`
}

function UsageBar({ usage }: { usage: R2Usage }) {
  // Colour stops mirror fleet card severity: < 75% safe (green); 75-95 %
  // warn (orange); >= 95 % danger (red). The bar fills against
  // `counted_bytes / limit_bytes`; hits are excluded by policy.
  const pct = Math.min(100, usage.percent)
  const tone = usage.threshold_95_hit ? 'bad'
    : usage.threshold_75_hit ? 'warn'
    : 'ok'
  const barColour =
    tone === 'bad' ? 'var(--danger, #ff5a5a)' :
    tone === 'warn' ? '#ff8a3d' :
    'var(--accent, #6cc6ff)'
  return (
    <div style={{ marginTop: '.5rem' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', fontSize: '.78rem' }}>
        <span className="muted" style={{ textTransform: 'uppercase', letterSpacing: '.06em' }}>
          R2 usage
        </span>
        <span className="mono" title="Hits are not counted toward the cap">
          {formatBytes(usage.counted_bytes)} / {formatBytes(usage.limit_bytes)}
          {' '}({pct.toFixed(1)}%)
        </span>
      </div>
      <div style={{
        marginTop: '.25rem',
        height: '8px',
        background: 'rgba(255,255,255,.08)',
        borderRadius: '4px',
        overflow: 'hidden',
      }}>
        <div style={{
          width: `${pct}%`,
          height: '100%',
          background: barColour,
          transition: 'width .4s ease',
        }} />
      </div>
      <div style={{ marginTop: '.35rem', display: 'flex', gap: '.65rem', flexWrap: 'wrap', fontSize: '.72rem' }} className="muted">
        <span>WARC {formatBytes(usage.bytes_by.warc)} · {usage.count_by.warc}</span>
        <span>Lists {formatBytes(usage.bytes_by.uploads)} · {usage.count_by.uploads}</span>
        <span>Hits {formatBytes(usage.bytes_by.hits)} · {usage.count_by.hits} (uncapped)</span>
        {usage.bytes_by.other > 0 && <span>Other {formatBytes(usage.bytes_by.other)} · {usage.count_by.other}</span>}
      </div>
      {usage.threshold_95_hit && (
        <p style={{ marginTop: '.4rem', color: 'var(--danger, #ff5a5a)', fontSize: '.78rem' }}>
          ⚠ R2 storage is at {pct.toFixed(1)}% of the 9.5 GB cap. Delete old WARC exports or target lists to free space.
        </p>
      )}
      {!usage.threshold_95_hit && usage.threshold_75_hit && (
        <p style={{ marginTop: '.4rem', color: '#ff8a3d', fontSize: '.78rem' }}>
          R2 storage is at {pct.toFixed(1)}% of the 9.5 GB cap — consider pruning before it fills up.
        </p>
      )}
    </div>
  )
}
