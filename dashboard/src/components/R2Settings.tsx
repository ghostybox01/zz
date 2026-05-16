import { useEffect, useState } from 'react'
import { r2, type R2Config } from '../lib/reconApi'

export function R2Settings() {
  const [cfg, setCfg] = useState<Omit<R2Config, 'configured'>>({
    account_id: '', access_key_id: '', secret_access_key: '', bucket_name: '',
  })
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [configured, setConfigured] = useState(false)

  useEffect(() => {
    r2.getConfig().then((c) => {
      setConfigured(c.configured)
      setCfg({ account_id: c.account_id, access_key_id: c.access_key_id, secret_access_key: '', bucket_name: c.bucket_name })
    }).catch(() => {})
  }, [])

  async function save() {
    setStatus('saving')
    try {
      const result = await r2.saveConfig(cfg)
      setConfigured(result.configured)
      setStatus('saved')
      setTimeout(() => setStatus('idle'), 2000)
    } catch {
      setStatus('error')
    }
  }

  return (
    <section className="card-block card-block--tight">
      <div className="card-block__head card-block__head--row">
        <div>
          <h2>Cloudflare R2 storage</h2>
          <p className="card-block__lede card-block__lede--short">
            Direct-to-R2 upload — supports 1–2 GB lists, survives tab switching.{' '}
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
      <p className="settings-hint">
        Create an R2 API token at Cloudflare Dashboard → R2 → Manage API tokens. Grant Object Read &amp; Write on your bucket.
        Account ID is on the R2 overview page (right sidebar).
      </p>
    </section>
  )
}
