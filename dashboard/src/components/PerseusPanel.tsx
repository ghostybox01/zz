import { useEffect, useRef, useState } from 'react'
import { perseus, type PerseusStatus } from '../lib/reconApi'

const POLL_MS = 3000

const VALIDATORS = [
  'aws', 'stripe', 'twilio', 'sendgrid', 'mailgun', 'brevo', 'telnyx',
  'github', 'gitlab', 'bitbucket', 'slack', 'discord', 'openai',
  'anthropic', 'resend', 'mailjet', 'sparkpost', 'sinch', 'nexmo',
  'plivo', 'messagebird', 'mandrill', 'mailchimp', 'postmark',
  'elasticemail', 'paypal',
]

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div style={{
      background: 'rgba(255,255,255,.04)',
      border: '1px solid var(--hairline)',
      borderRadius: '.5rem',
      padding: '.55rem .75rem',
    }}>
      <div style={{ fontSize: '.7rem', color: 'var(--text-muted, var(--muted))', textTransform: 'uppercase', letterSpacing: '.04em' }}>{label}</div>
      <div className="mono" style={{ fontSize: '1rem', marginTop: '.15rem', color: 'var(--text)' }}>{value}</div>
    </div>
  )
}

type ToastFn = (title: string, message: string, kind: 'info' | 'error') => void
type Props = { notify?: ToastFn }

export function PerseusPanel({ notify }: Props = {}) {
  const [status, setStatus] = useState<PerseusStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [sourceMode, setSourceMode] = useState<'warc' | 'paste'>('warc')
  const [pastedUrls, setPastedUrls] = useState('')
  const [flags, setFlags] = useState<Record<string, boolean>>({ '--crt': false, '--js': false, '--deep': false })
  const [binaryFile, setBinaryFile] = useState<File | null>(null)
  const [binaryUploading, setBinaryUploading] = useState(false)
  const [binaryMsg, setBinaryMsg] = useState<string | null>(null)
  const pollTimer = useRef<number | null>(null)

  async function refresh() {
    try {
      const s = await perseus.status()
      setStatus(s)
    } catch {
      // silent — VPS may be restarting
    }
  }

  useEffect(() => {
    void refresh()
    pollTimer.current = window.setInterval(() => { void refresh() }, POLL_MS)
    return () => { if (pollTimer.current) window.clearInterval(pollTimer.current) }
  }, [])

  async function onUploadBinary() {
    if (!binaryFile) return
    setBinaryUploading(true)
    setBinaryMsg(null)
    try {
      const res = await perseus.uploadBinary(binaryFile)
      setBinaryMsg(`Uploaded ${(res.size / 1024 / 1024).toFixed(1)} MB → ${res.path}`)
      setBinaryFile(null)
    } catch (e: unknown) {
      setBinaryMsg(`Upload failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setBinaryUploading(false)
    }
  }

  async function onStart() {
    setError(null)
    if (sourceMode === 'paste' && !pastedUrls.trim()) {
      setError('Paste at least one URL')
      return
    }
    setBusy(true)
    try {
      const activeFlags = Object.entries(flags).filter(([, v]) => v).map(([k]) => k)
      await perseus.start({
        source: sourceMode,
        urls: sourceMode === 'paste' ? pastedUrls : undefined,
        flags: activeFlags,
      })
      notify?.('Perseus', 'Scan started', 'info')
      void refresh()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setError(msg)
      notify?.('Perseus error', msg, 'error')
    } finally {
      setBusy(false)
    }
  }

  async function onStop() {
    setBusy(true)
    try {
      await perseus.stop()
      notify?.('Perseus', 'Scan stopped', 'info')
      void refresh()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const running = status?.running ?? false
  const finishedAt = status?.finished_at
  const exitCode = status?.last_exit_code

  const validatorHits = VALIDATORS.filter(v => (status?.validator_stats?.[v] ?? 0) > 0)

  return (
    <section className="warc-panel">
      <header className="card-block__head card-block__head--row">
        <div>
          <h2 style={{ margin: 0 }}>Perseus</h2>
          <p className="card-block__lede card-block__lede--short" style={{ margin: '.2rem 0 0' }}>
            Credential exposure scanner — probes live targets for .env files, JS secrets, AWS keys
          </p>
        </div>
        <div className="warc-head-actions">
          <div className="warc-mode">
            <span className={`pill ${running ? 'pill--ok' : finishedAt ? 'pill--muted' : 'pill--muted'}`}>
              {running ? 'Running' : finishedAt ? 'Completed' : 'Idle'}
            </span>
          </div>
          <div className="warc-controls">
            {running ? (
              <button className="btn-danger-outline" onClick={() => void onStop()} disabled={busy}>
                ■ Stop scan
              </button>
            ) : (
              <button className="btn-glass" onClick={() => void onStart()} disabled={busy}>
                ▶ Start scan
              </button>
            )}
          </div>
        </div>
      </header>

      {/* Config form — hidden while running */}
      {!running && (
        <div className="kv kv--form" style={{ marginTop: '1.25rem' }}>
          {/* Binary upload */}
          <div className="kv__row">
            <label className="kv__label">Binary</label>
            <div style={{ display: 'flex', gap: '.5rem', alignItems: 'center', flexWrap: 'wrap' }}>
              <input
                type="file"
                accept=".bin,application/octet-stream,"
                onChange={(e) => setBinaryFile(e.target.files?.[0] ?? null)}
                style={{ flex: 1, minWidth: 0, fontSize: '.82rem' }}
              />
              <button
                type="button"
                className="btn-glass btn-glass--xs"
                onClick={() => void onUploadBinary()}
                disabled={!binaryFile || binaryUploading}
              >
                {binaryUploading ? '…' : 'Upload'}
              </button>
            </div>
          </div>
          {binaryMsg && (
            <div className="kv__row">
              <span className="kv__label" />
              <span className="settings-hint">{binaryMsg}</span>
            </div>
          )}

          {/* Source selector */}
          <div className="kv__row">
            <span className="kv__label">URL source</span>
            <div style={{ display: 'flex', gap: '1.25rem', alignItems: 'center' }}>
              {(['warc', 'paste'] as const).map(m => (
                <label key={m} style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer', fontSize: '.88rem' }}>
                  <input type="radio" checked={sourceMode === m} onChange={() => setSourceMode(m)} />
                  {m === 'warc' ? 'WARC harvest output (env_urls.txt)' : 'Paste URL list'}
                </label>
              ))}
            </div>
          </div>

          {sourceMode === 'paste' && (
            <div className="kv__row" style={{ alignItems: 'flex-start' }}>
              <label className="kv__label" style={{ paddingTop: '.3rem' }}>URLs</label>
              <textarea
                className="kv__input mono"
                rows={6}
                value={pastedUrls}
                onChange={(e) => setPastedUrls(e.target.value)}
                placeholder={'https://example.com\nhttp://1.2.3.4:8080'}
                style={{ resize: 'vertical', fontSize: '.8rem' }}
              />
            </div>
          )}

          {/* Scan flags */}
          <div className="kv__row">
            <span className="kv__label">Flags</span>
            <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
              {Object.entries(flags).map(([flag, checked]) => (
                <label key={flag} style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer', fontSize: '.85rem' }}>
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={(e) => setFlags(prev => ({ ...prev, [flag]: e.target.checked }))}
                  />
                  <code className="inline-code">{flag}</code>
                  <span className="muted" style={{ fontSize: '.78rem' }}>
                    {flag === '--crt' ? 'crt.sh subdomain expansion' : flag === '--js' ? 'JS file scan only' : 'Deep AWS enumeration'}
                  </span>
                </label>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Stats row */}
      <div style={{ marginTop: '1.25rem', display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '.75rem' }}>
        <Stat label="Total hits" value={status?.total_hits ?? 0} />
        <Stat label="Started" value={status?.started_at ? new Date(status.started_at).toLocaleTimeString() : '—'} />
        <Stat label="Status" value={running ? 'Scanning' : finishedAt ? 'Done' : 'Idle'} />
        <Stat label="Exit code" value={exitCode !== null && exitCode !== undefined ? exitCode : '—'} />
      </div>

      {/* Per-validator breakdown */}
      {validatorHits.length > 0 && (
        <>
          <p className="muted" style={{ fontSize: '.78rem', marginTop: '1rem', marginBottom: '.4rem' }}>Live credentials found</p>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '.5rem' }}>
            {validatorHits.map(v => (
              <Stat key={v} label={v} value={status!.validator_stats[v]} />
            ))}
          </div>
        </>
      )}

      {error && <p className="settings-hint" style={{ marginTop: '.75rem', color: 'var(--danger)' }}>{error}</p>}

      {/* Log tail */}
      {status?.log_tail && status.log_tail.length > 0 && (
        <details style={{ marginTop: '1rem' }}>
          <summary className="muted" style={{ cursor: 'pointer', fontSize: '.8rem' }}>
            Last {status.log_tail.length} log lines
          </summary>
          <pre className="mono" style={{
            fontSize: '.72rem', maxHeight: '14rem', overflowY: 'auto',
            background: 'rgba(0,0,0,.35)', padding: '.6rem .8rem',
            borderRadius: '.4rem', marginTop: '.4rem', whiteSpace: 'pre-wrap',
          }}>
            {status.log_tail.join('\n')}
          </pre>
        </details>
      )}
    </section>
  )
}
