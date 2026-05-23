import { useCallback, useEffect, useState } from 'react'
import { findings as findingsApi, type DiscoveredKey, type StripeRefreshResult } from '../lib/reconApi'

type Props = {
  onToast: (title: string, message?: string, kind?: 'error' | 'info') => void
}

type RefreshState = Record<number, { loading: boolean; result?: StripeRefreshResult }>

function fmtMoney(amount: number, currency: string): string {
  const v = amount / 100
  return `${v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ${currency.toUpperCase()}`
}

function detectMode(metadata: string, key: string): { mode: 'Live' | 'Test' | 'Unknown'; keyType: string } {
  let mode: 'Live' | 'Test' | 'Unknown' = 'Unknown'
  if (key.includes('_live_') || metadata.toLowerCase().includes('live')) mode = 'Live'
  else if (key.includes('_test_') || metadata.toLowerCase().includes('test')) mode = 'Test'
  let keyType = 'Unknown'
  if (key.startsWith('sk_')) keyType = 'Secret'
  else if (key.startsWith('pk_')) keyType = 'Publishable'
  else if (key.startsWith('rk_')) keyType = 'Restricted'
  return { mode, keyType }
}

export function StripePanel({ onToast }: Props) {
  const [items, setItems] = useState<DiscoveredKey[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshState, setRefreshState] = useState<RefreshState>({})

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const r = await findingsApi.listStripe()
      setItems(r.findings)
    } catch (e) {
      onToast('Could not load Stripe findings', (e as Error).message, 'error')
    } finally {
      setLoading(false)
    }
  }, [onToast])

  useEffect(() => { void reload() }, [reload])

  const refresh = useCallback(async (id: number) => {
    setRefreshState((s) => ({ ...s, [id]: { loading: true } }))
    try {
      const r = await findingsApi.refreshStripe(id)
      setRefreshState((s) => ({ ...s, [id]: { loading: false, result: r } }))
      onToast(r.live ? 'Stripe key live' : 'Stripe key no longer valid',
              r.live ? 'Balance refreshed' : `HTTP ${r.status ?? '—'}`,
              r.live ? 'info' : 'error')
    } catch (e) {
      setRefreshState((s) => ({ ...s, [id]: { loading: false, result: { ok: false, live: false, error: (e as Error).message } } }))
      onToast('Refresh failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const remove = useCallback(async (id: number) => {
    if (!window.confirm('Delete this Stripe finding from the ledger?')) return
    try {
      await findingsApi.remove(id)
      setItems((prev) => prev.filter((k) => k.id !== id))
    } catch (e) {
      onToast('Delete failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const toggleReported = useCallback(async (id: number, current: boolean) => {
    try {
      await findingsApi.markReported(id, !current)
      setItems((prev) => prev.map((k) => k.id === id ? { ...k, reported: !current } : k))
    } catch (e) {
      onToast('Update failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const copy = useCallback(async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      onToast('Copied to clipboard')
    } catch {
      onToast('Copy failed', 'Browser blocked clipboard access', 'error')
    }
  }, [onToast])

  return (
    <div className="findings-panel">
      <header className="findings-panel__head">
        <div>
          <h2 className="findings-panel__title">Stripe — discovered keys</h2>
          <p className="muted findings-panel__sub">
            Keys the scanner validated against <code>GET /v1/balance</code>. Re-test, copy or mark
            as reported. Withdraws happen in the leak owner's own Stripe dashboard.
          </p>
        </div>
        <div style={{ display: 'flex', gap: '.5rem', alignItems: 'center' }}>
          <span className="muted" style={{ fontSize: '.8rem' }}>{items.length} total</span>
          <button type="button" className="btn-glass btn-glass--xs" onClick={() => void reload()} disabled={loading}>
            {loading ? '…' : '↻ Reload'}
          </button>
        </div>
      </header>

      {loading && items.length === 0 && (
        <p className="muted-callout">Loading Stripe findings…</p>
      )}
      {!loading && items.length === 0 && (
        <div className="muted-callout">
          <p style={{ margin: 0, fontWeight: 600 }}>No Stripe keys discovered yet</p>
          <p className="muted" style={{ margin: '.35rem 0 0', fontSize: '.82rem' }}>
            Enable the Stripe addon in Settings → Cracker addons and start a crack session.
            Validated keys land here automatically.
          </p>
        </div>
      )}

      {items.length > 0 && (
        <div className="findings-table-wrap">
          <table className="findings-table">
            <thead>
              <tr>
                <th>Key</th>
                <th>Mode</th>
                <th>Source</th>
                <th>Found</th>
                <th>Live balance</th>
                <th style={{ width: '14rem' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => {
                const { mode, keyType } = detectMode(item.metadata, item.key_value)
                const state = refreshState[item.id]
                let storedMeta: { livemode?: boolean; available?: Array<{ amount: number; currency: string }> } = {}
                try { storedMeta = item.verify_meta ? JSON.parse(item.verify_meta) : {} } catch { /* ignore */ }
                const liveBalance = state?.result?.available ?? storedMeta.available
                return (
                  <tr key={item.id} className={item.reported ? 'findings-row--reported' : ''}>
                    <td>
                      <code className="findings-key">{item.key_value}</code>
                      <div className="muted" style={{ fontSize: '.7rem' }}>{keyType} key</div>
                    </td>
                    <td>
                      <span className={`findings-mode findings-mode--${mode.toLowerCase()}`}>{mode}</span>
                    </td>
                    <td>
                      <a href={item.source_url} target="_blank" rel="noopener noreferrer" className="findings-source" title={item.source_url}>
                        {item.source_url}
                      </a>
                    </td>
                    <td className="muted" style={{ fontSize: '.75rem', whiteSpace: 'nowrap' }}>
                      {item.timestamp}
                    </td>
                    <td>
                      {state?.loading && <span className="muted">…</span>}
                      {!state?.loading && liveBalance && liveBalance.length > 0 && (
                        <div>
                          {liveBalance.map((b, i) => (
                            <div key={i} className="findings-bal">{fmtMoney(b.amount, b.currency)}</div>
                          ))}
                        </div>
                      )}
                      {!state?.loading && (!liveBalance || liveBalance.length === 0) && (
                        <span className="muted" style={{ fontSize: '.75rem' }}>
                          {state?.result && !state.result.live ? 'Key dead' : 'Click ↻ to check'}
                        </span>
                      )}
                    </td>
                    <td className="findings-actions">
                      <button type="button" className="btn-glass btn-glass--xs" onClick={() => void refresh(item.id)} disabled={state?.loading} title="Re-test against Stripe">↻</button>
                      <button type="button" className="btn-glass btn-glass--xs" onClick={() => void copy(item.key_value)} title="Copy key">⧉</button>
                      <button type="button" className={`btn-glass btn-glass--xs${item.reported ? ' btn-glass--on' : ''}`} onClick={() => void toggleReported(item.id, item.reported)} title="Mark reported">
                        {item.reported ? '✓' : '!'}
                      </button>
                      <button type="button" className="btn-glass btn-glass--xs btn-glass--danger" onClick={() => void remove(item.id)} title="Delete">×</button>
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
