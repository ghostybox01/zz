import { useCallback, useEffect, useState } from 'react'
import { findings as findingsApi, credentials as credApi, type DiscoveredKey } from '../lib/reconApi'

type Props = {
  onToast: (title: string, message?: string, kind?: 'error' | 'info') => void
}

const ALL_PROVIDERS = [
  'SendGrid', 'Mailgun', 'Mandrill', 'Postmark', 'Brevo', 'MailerSend',
  'SparkPost', 'Mailtrap', 'Mailjet', 'Resend', 'Twilio',
  'Nexmo', 'Telnyx', 'MessageBird', 'Plivo',
] as const

const PROVIDER_COLOR: Record<string, string> = {
  SendGrid:    '#1a82e2',
  Mailgun:     '#f06b26',
  Mandrill:    '#2a9d8f',
  Postmark:    '#ffbe33',
  Brevo:       '#0092ff',
  MailerSend:  '#6c47ff',
  SparkPost:   '#fa6423',
  Mailtrap:    '#16c79a',
  Mailjet:     '#9b5de5',
  Resend:      '#00d084',
  Twilio:      '#f22f46',
  Nexmo:       '#00b2ff',
  Telnyx:      '#00cc66',
  MessageBird: '#2cb7f0',
  Plivo:       '#e74c3c',
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

export function EmailApiPanel({ onToast }: Props) {
  const [items, setItems] = useState<DiscoveredKey[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('all')
  const [recheckState, setRecheckState] = useState<Record<number, { loading: boolean; meta?: string }>>({})

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const r = await findingsApi.listEmailApi()
      setItems(r.findings)
    } catch (e) {
      onToast('Could not load email API findings', (e as Error).message, 'error')
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
      if (r.live) onToast(`${item.type} live`, r.info || 'Credential valid', 'info')
      else onToast(`${item.type} dead`, r.info || 'Credential invalid', 'error')
    } catch (e) {
      setRecheckState(s => ({ ...s, [item.id]: { loading: false } }))
      onToast('Recheck failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const remove = useCallback(async (id: number) => {
    if (!window.confirm('Delete this credential?')) return
    try {
      await findingsApi.remove(id)
      setItems(prev => prev.filter(k => k.id !== id))
    } catch (e) {
      onToast('Delete failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const providerCounts = ALL_PROVIDERS.reduce<Record<string, number>>((acc, p) => {
    acc[p] = items.filter(i => i.type === p).length
    return acc
  }, {})

  const visible = filter === 'all' ? items : items.filter(i => i.type === filter)

  return (
    <div className="findings-panel">
      <header className="findings-panel__head">
        <div>
          <h2 className="findings-panel__title">Email &amp; SMS APIs</h2>
          <p className="muted findings-panel__sub">
            Transactional email and SMS API keys — SendGrid, Mailgun, Twilio, Brevo, and more.
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
        {ALL_PROVIDERS.filter(p => providerCounts[p] > 0).map(p => (
          <button key={p} type="button"
            className={`btn-glass btn-glass--xs${filter === p ? ' btn-glass--on' : ''}`}
            onClick={() => setFilter(p)}
          >
            <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: PROVIDER_COLOR[p] ?? '#888', marginRight: 4 }} />
            {p} <span className="muted">({providerCounts[p]})</span>
          </button>
        ))}
      </div>

      {loading && items.length === 0 && <p className="muted-callout">Loading…</p>}
      {!loading && visible.length === 0 && (
        <div className="muted-callout">
          <p style={{ margin: 0, fontWeight: 600 }}>No {filter === 'all' ? 'email/SMS API' : filter} credentials yet</p>
          <p className="muted" style={{ margin: '.35rem 0 0', fontSize: '.82rem' }}>
            Enable the relevant scanner addon and start a scan session.
          </p>
        </div>
      )}

      {visible.length > 0 && (
        <div className="findings-table-wrap">
          <table className="findings-table">
            <thead>
              <tr>
                <th>Provider</th>
                <th>Credential</th>
                <th>Source</th>
                <th>Status</th>
                <th>Validated info</th>
                <th style={{ width: '7rem' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {visible.map(item => {
                const st = recheckState[item.id]
                const verifyRaw = st?.meta ?? item.verify_meta
                const isValid = item.status === 'valid' || item.status === 'validated'
                const color = PROVIDER_COLOR[item.type] ?? '#888'
                return (
                  <tr key={item.id} style={isValid ? { background: 'rgba(0,255,100,0.04)' } : undefined}>
                    <td>
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontSize: '.78rem', fontWeight: 600, color }}>
                        <span style={{ width: 6, height: 6, borderRadius: '50%', background: color, display: 'inline-block' }} />
                        {item.type}
                      </span>
                    </td>
                    <td>
                      <code className="findings-key" style={{ fontSize: '.72rem' }}>
                        {item.key_value.length > 60 ? `${item.key_value.slice(0, 60)}…` : item.key_value}
                      </code>
                      {item.metadata && <div className="muted" style={{ fontSize: '.68rem' }}>{item.metadata.slice(0, 50)}</div>}
                      <RecheckBadges raw={verifyRaw} />
                    </td>
                    <td>
                      <a href={item.source_url} target="_blank" rel="noopener noreferrer"
                        className="findings-source" title={item.source_url} style={{ fontSize: '.72rem' }}>
                        {item.source_url?.slice(0, 50)}
                      </a>
                    </td>
                    <td>
                      <span className={`findings-mode findings-mode--${isValid ? 'live' : 'unknown'}`}>
                        {isValid ? 'Valid' : 'Hit'}
                      </span>
                    </td>
                    <td className="muted" style={{ fontSize: '.72rem' }}>
                      {item.last_verified ? new Date(item.last_verified).toLocaleString() : '—'}
                    </td>
                    <td className="findings-actions">
                      <button type="button" className="btn-glass btn-glass--xs" title="Recheck" disabled={st?.loading} onClick={() => void recheck(item)}>
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
