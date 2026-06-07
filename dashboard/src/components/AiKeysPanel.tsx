import { useCallback, useEffect, useState } from 'react'
import { findings as findingsApi, credentials as credApi, type DiscoveredKey } from '../lib/reconApi'

type Props = {
  onToast: (title: string, message?: string, kind?: 'error' | 'info') => void
}

const PROVIDER_COLOR: Record<string, string> = {
  OpenAI:     '#10a37f',
  Anthropic:  '#d4964f',
  HuggingFace:'#ff9d00',
  Gemini:     '#4285f4',
  Replicate:  '#ee4433',
}

const ALL_PROVIDERS = ['OpenAI', 'Anthropic', 'HuggingFace', 'Gemini', 'Replicate'] as const

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

export function AiKeysPanel({ onToast }: Props) {
  const [items, setItems] = useState<DiscoveredKey[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('all')
  const [recheckState, setRecheckState] = useState<Record<number, { loading: boolean; meta?: string }>>({})

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const r = await findingsApi.listAiKeys()
      setItems(r.findings)
    } catch (e) {
      onToast('Could not load AI keys', (e as Error).message, 'error')
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
      if (r.live) onToast(`${item.type} valid`, r.info || 'Key accepted', 'info')
      else onToast(`${item.type} invalid`, r.info || 'Key rejected', 'error')
    } catch (e) {
      setRecheckState(s => ({ ...s, [item.id]: { loading: false } }))
      onToast('Recheck failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const remove = useCallback(async (id: number) => {
    if (!window.confirm('Delete this key?')) return
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
          <h2 className="findings-panel__title">AI Keys</h2>
          <p className="muted findings-panel__sub">
            OpenAI, Anthropic, Gemini, HuggingFace, and Replicate API keys discovered during scanning.
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
          <p style={{ margin: 0, fontWeight: 600 }}>No {filter === 'all' ? 'AI' : filter} keys yet</p>
          <p className="muted" style={{ margin: '.35rem 0 0', fontSize: '.82rem' }}>Keys appear here automatically as the scanner finds them.</p>
        </div>
      )}

      {visible.length > 0 && (
        <div className="findings-table-wrap">
          <table className="findings-table">
            <thead>
              <tr>
                <th>Provider</th>
                <th>Key</th>
                <th>Source</th>
                <th>Status</th>
                <th>Info</th>
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
