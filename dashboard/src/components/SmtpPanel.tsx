import { useCallback, useEffect, useState } from 'react'
import { findings as findingsApi, credentials as credApi, type DiscoveredKey } from '../lib/reconApi'

type Props = {
  onToast: (title: string, message?: string, kind?: 'error' | 'info') => void
}

type SmtpCred = { host: string; port: string; user: string; pass: string; from: string }

function parseSmtpCred(keyValue: string): SmtpCred | null {
  const parts = keyValue.split(':')
  if (parts.length < 3) return null
  const [host, port, user, ...rest] = parts
  const lastPart = rest[rest.length - 1] ?? ''
  const hasFrom = lastPart.includes('@')
  const from = hasFrom ? lastPart : ''
  const pass = hasFrom ? rest.slice(0, -1).join(':') : rest.join(':')
  return { host: host ?? '', port: port ?? '', user: user ?? '', pass, from }
}

function parseRecheckMeta(raw: string | null): Record<string, string> {
  if (!raw) return {}
  try {
    const p = JSON.parse(raw)
    if (p?.extra && Array.isArray(p.extra)) {
      return Object.fromEntries(p.extra.map((e: { key: string; value: string }) => [e.key, e.value]))
    }
    return p as Record<string, string>
  } catch { return {} }
}

function RecheckBadges({ raw }: { raw: string | null }) {
  const meta = parseRecheckMeta(raw)
  const keys = Object.keys(meta).filter(k => !['error', 'source'].includes(k))
  if (keys.length === 0) return null
  return (
    <div style={{ marginTop: '.3rem', display: 'flex', flexWrap: 'wrap', gap: '.25rem' }}>
      {keys.slice(0, 6).map(k => (
        <span key={k} style={{ fontSize: '.68rem', background: 'rgba(255,255,255,.06)', borderRadius: 3, padding: '1px 5px', color: '#aaa' }}>
          <span style={{ color: '#666' }}>{k}: </span>{String(meta[k]).slice(0, 40)}
        </span>
      ))}
    </div>
  )
}

export function SmtpPanel({ onToast }: Props) {
  const [items, setItems] = useState<DiscoveredKey[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<'all' | 'SMTP' | 'XSMTP'>('all')
  const [recheckState, setRecheckState] = useState<Record<number, { loading: boolean; meta?: string }>>({})

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const r = await findingsApi.listSmtp()
      setItems(r.findings)
    } catch (e) {
      onToast('Could not load SMTP findings', (e as Error).message, 'error')
    } finally {
      setLoading(false)
    }
  }, [onToast])

  useEffect(() => { void reload() }, [reload])

  const recheck = useCallback(async (item: DiscoveredKey) => {
    setRecheckState(s => ({ ...s, [item.id]: { loading: true } }))
    try {
      const r = await credApi.recheck(item.id)
      const meta = r.rich ? JSON.stringify({ extra: r.extra, ...r.rich }) : undefined
      setRecheckState(s => ({ ...s, [item.id]: { loading: false, meta } }))
      if (r.live) onToast('SMTP authenticated', r.info || 'Login succeeded', 'info')
      else onToast('SMTP failed', r.info || 'Authentication rejected', 'error')
    } catch (e) {
      setRecheckState(s => ({ ...s, [item.id]: { loading: false } }))
      onToast('Recheck failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const remove = useCallback(async (id: number) => {
    if (!window.confirm('Delete this SMTP credential?')) return
    try {
      await findingsApi.remove(id)
      setItems(prev => prev.filter(k => k.id !== id))
    } catch (e) {
      onToast('Delete failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const smtpCount = items.filter(i => i.type === 'SMTP').length
  const xsmtpCount = items.filter(i => i.type === 'XSMTP').length
  const visible = filter === 'all' ? items : items.filter(i => i.type === filter)

  return (
    <div className="findings-panel">
      <header className="findings-panel__head">
        <div>
          <h2 className="findings-panel__title">SMTP Servers</h2>
          <p className="muted findings-panel__sub">
            Raw SMTP and XSMTP server credentials — authenticated mail relay access.
          </p>
        </div>
        <div style={{ display: 'flex', gap: '.5rem', alignItems: 'center' }}>
          <span className="muted" style={{ fontSize: '.8rem' }}>{items.length} total</span>
          <button type="button" className="btn-glass btn-glass--xs" onClick={() => void reload()} disabled={loading}>
            {loading ? '…' : '↻ Reload'}
          </button>
        </div>
      </header>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: '.25rem', marginBottom: '.75rem' }}>
        <button type="button" className={`btn-glass btn-glass--xs${filter === 'all' ? ' btn-glass--on' : ''}`} onClick={() => setFilter('all')}>
          All {items.length > 0 && <span className="muted">({items.length})</span>}
        </button>
        {smtpCount > 0 && (
          <button type="button" className={`btn-glass btn-glass--xs${filter === 'SMTP' ? ' btn-glass--on' : ''}`} onClick={() => setFilter('SMTP')}>
            <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: '#fbbf24', marginRight: 4 }} />
            SMTP <span className="muted">({smtpCount})</span>
          </button>
        )}
        {xsmtpCount > 0 && (
          <button type="button" className={`btn-glass btn-glass--xs${filter === 'XSMTP' ? ' btn-glass--on' : ''}`} onClick={() => setFilter('XSMTP')}>
            <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: '#f97316', marginRight: 4 }} />
            XSMTP <span className="muted">({xsmtpCount})</span>
          </button>
        )}
      </div>

      {loading && items.length === 0 && <p className="muted-callout">Loading…</p>}
      {!loading && visible.length === 0 && (
        <div className="muted-callout">
          <p style={{ margin: 0, fontWeight: 600 }}>No SMTP credentials yet</p>
          <p className="muted" style={{ margin: '.35rem 0 0', fontSize: '.82rem' }}>
            SMTP credentials appear here when the scanner finds and validates mail server access.
          </p>
        </div>
      )}

      {visible.length > 0 && (
        <div className="findings-table-wrap">
          <table className="findings-table">
            <thead>
              <tr>
                <th>Type</th>
                <th>Host</th>
                <th>User / Pass</th>
                <th>From</th>
                <th>Source</th>
                <th>Status</th>
                <th style={{ width: '5rem' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {visible.map(item => {
                const st = recheckState[item.id]
                const verifyRaw = st?.meta ?? item.verify_meta
                const isValid = item.status === 'valid' || item.status === 'validated'
                const color = item.type === 'XSMTP' ? '#f97316' : '#fbbf24'
                const cred = item.type === 'SMTP' ? parseSmtpCred(item.key_value) : null
                const displayKey = cred ? null : item.key_value
                return (
                  <tr key={item.id} style={isValid ? { background: 'rgba(0,255,100,0.04)' } : undefined}>
                    <td>
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontSize: '.78rem', fontWeight: 600, color }}>
                        <span style={{ width: 6, height: 6, borderRadius: '50%', background: color, display: 'inline-block' }} />
                        {item.type}
                      </span>
                    </td>
                    <td>
                      {cred
                        ? <><code style={{ fontSize: '.72rem', color: '#aaa' }}>{cred.host}</code>
                            {cred.port && <span className="muted" style={{ fontSize: '.68rem' }}>:{cred.port}</span>}</>
                        : displayKey && <code className="findings-key" style={{ fontSize: '.72rem' }}>
                            {displayKey.length > 40 ? `${displayKey.slice(0, 40)}…` : displayKey}
                          </code>
                      }
                    </td>
                    <td>
                      {cred
                        ? <><code style={{ fontSize: '.72rem', display: 'block' }}>{cred.user}</code>
                            {cred.pass && <code style={{ fontSize: '.68rem', color: '#888', display: 'block' }}>{cred.pass.slice(0, 30)}{cred.pass.length > 30 ? '…' : ''}</code>}</>
                        : <RecheckBadges raw={verifyRaw} />
                      }
                    </td>
                    <td>
                      {cred
                        ? <span style={{ fontSize: '.7rem', color: '#888' }}>{cred.from || '—'}</span>
                        : item.metadata && <span className="muted" style={{ fontSize: '.68rem' }}>{item.metadata.slice(0, 30)}</span>
                      }
                    </td>
                    <td>
                      <a href={item.source_url} target="_blank" rel="noopener noreferrer"
                        className="findings-source" title={item.source_url} style={{ fontSize: '.72rem' }}>
                        {item.source_url?.slice(0, 35)}
                      </a>
                    </td>
                    <td>
                      <span className={`findings-mode findings-mode--${isValid ? 'live' : 'unknown'}`}>
                        {isValid ? 'Auth OK' : 'Hit'}
                      </span>
                    </td>
                    <td className="findings-actions">
                      <button type="button" className="btn-glass btn-glass--xs" title="Recheck auth" disabled={st?.loading} onClick={() => void recheck(item)}>
                        {st?.loading ? '…' : '↻'}
                      </button>
                      <button type="button" className="btn-glass btn-glass--xs btn-glass--danger" title="Delete" onClick={() => void remove(item.id)}>
                        ×
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
