import { useCallback, useEffect, useState } from 'react'
import { findings as findingsApi, type DiscoveredKey, type CryptoBalanceResult } from '../lib/reconApi'

type Props = {
  onToast: (title: string, message?: string, kind?: 'error' | 'info') => void
}

type RefreshState = Record<number, { loading: boolean; result?: CryptoBalanceResult; addrInput?: string; chain?: 'eth' | 'btc' | 'bnb' }>

function parseStoredMeta(verify_meta: string | null): CryptoBalanceResult & { address?: string; chain?: string } {
  if (!verify_meta) return {} as never
  try { return JSON.parse(verify_meta) } catch { return {} as never }
}

export function CryptoPanel({ onToast }: Props) {
  const [items, setItems] = useState<DiscoveredKey[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshState, setRefreshState] = useState<RefreshState>({})

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const r = await findingsApi.listCrypto()
      setItems(r.findings)
    } catch (e) {
      onToast('Could not load crypto findings', (e as Error).message, 'error')
    } finally {
      setLoading(false)
    }
  }, [onToast])

  useEffect(() => { void reload() }, [reload])

  const refresh = useCallback(async (id: number) => {
    const st = refreshState[id]
    setRefreshState((s) => ({ ...s, [id]: { ...s[id], loading: true } }))
    try {
      const addr = st?.addrInput?.trim()
      const chain = st?.chain ?? 'eth'
      const r = await findingsApi.refreshCrypto(id, addr || undefined, chain)
      setRefreshState((s) => ({ ...s, [id]: { ...s[id], loading: false, result: r } }))
      if (!r.ok) {
        onToast('Refresh failed', r.error ?? 'unknown error', 'error')
      } else {
        onToast(`Balance: ${r.balance_native} ${r.symbol}`,
                r.balance_usd != null ? `≈ $${r.balance_usd.toLocaleString()}` : 'USD price unavailable')
      }
    } catch (e) {
      setRefreshState((s) => ({ ...s, [id]: { ...s[id], loading: false, result: { ok: false, error: (e as Error).message } } }))
      onToast('Refresh failed', (e as Error).message, 'error')
    }
  }, [refreshState, onToast])

  const remove = useCallback(async (id: number) => {
    if (!window.confirm('Delete this crypto finding from the ledger?')) return
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

  const copy = useCallback((text: string) => {
    function fallbackCopy(t: string) {
      try {
        const ta = document.createElement('textarea')
        ta.value = t
        ta.style.cssText = 'position:fixed;left:-9999px;top:-9999px;opacity:0'
        document.body.appendChild(ta)
        ta.select()
        document.execCommand('copy')
        document.body.removeChild(ta)
        onToast('Copied to clipboard')
      } catch {
        onToast('Copy failed', 'Clipboard access denied', 'error')
      }
    }
    if (navigator.clipboard) {
      navigator.clipboard.writeText(text).then(() => onToast('Copied to clipboard')).catch(() => fallbackCopy(text))
    } else {
      fallbackCopy(text)
    }
  }, [onToast])

  return (
    <div className="findings-panel">
      <header className="findings-panel__head">
        <div>
          <h2 className="findings-panel__title">Crypto — discovered keys & wallets</h2>
          <p className="muted findings-panel__sub">
            Private keys, mnemonics and wallet addresses detected by the scanner. Refresh balances
            via public RPCs (Ethereum, BSC, Bitcoin). Transfers belong in the leak owner's own wallet UI.
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
        <p className="muted-callout">Loading crypto findings…</p>
      )}
      {!loading && items.length === 0 && (
        <div className="muted-callout">
          <p style={{ margin: 0, fontWeight: 600 }}>No crypto findings yet</p>
          <p className="muted" style={{ margin: '.35rem 0 0', fontSize: '.82rem' }}>
            Enable the <strong>Crypto Wallets</strong> addon in Settings → Cracker addons and start a crack session.
            Detected private keys / mnemonics land here automatically.
          </p>
        </div>
      )}

      {items.length > 0 && (
        <div className="findings-table-wrap">
          <table className="findings-table">
            <thead>
              <tr>
                <th>Key / phrase</th>
                <th>Type</th>
                <th>Source</th>
                <th>Found</th>
                <th>Balance</th>
                <th style={{ width: '18rem' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => {
                const stored = parseStoredMeta(item.verify_meta)
                const state = refreshState[item.id]
                const known = state?.result ?? stored
                const knownAddr = state?.addrInput ?? stored.address ?? ''
                const knownChain = state?.chain ?? (stored.chain as 'eth' | 'btc' | 'bnb' | undefined) ?? 'eth'
                return (
                  <tr key={item.id} className={item.reported ? 'findings-row--reported' : ''}>
                    <td>
                      <code className="findings-key">{item.key_value.length > 80 ? `${item.key_value.slice(0, 80)}…` : item.key_value}</code>
                      {item.metadata && (
                        <div className="muted" style={{ fontSize: '.7rem' }}>{item.metadata}</div>
                      )}
                    </td>
                    <td>
                      <span className={`findings-mode findings-mode--${item.type.toLowerCase()}`}>{item.type}</span>
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
                      {!state?.loading && known.balance_native != null && (
                        <div>
                          <div className="findings-bal">{known.balance_native} {known.symbol}</div>
                          {known.balance_usd != null && (
                            <div className="muted" style={{ fontSize: '.7rem' }}>≈ ${known.balance_usd.toLocaleString()}</div>
                          )}
                          {known.explorer_url && (
                            <a href={known.explorer_url} target="_blank" rel="noopener noreferrer" className="findings-source" style={{ fontSize: '.7rem' }}>
                              explorer ↗
                            </a>
                          )}
                        </div>
                      )}
                      {!state?.loading && known.balance_native == null && (
                        <span className="muted" style={{ fontSize: '.75rem' }}>{known.error ?? 'No balance checked'}</span>
                      )}
                    </td>
                    <td className="findings-actions">
                      <input
                        className="tg-input findings-addr-input"
                        value={knownAddr}
                        onChange={(e) => setRefreshState((s) => ({ ...s, [item.id]: { ...s[item.id], addrInput: e.target.value } }))}
                        placeholder={stored.address ? stored.address : '0x... address'}
                        spellCheck={false}
                      />
                      <select
                        className="tg-input findings-chain-select"
                        value={knownChain}
                        onChange={(e) => setRefreshState((s) => ({ ...s, [item.id]: { ...s[item.id], chain: e.target.value as 'eth' | 'btc' | 'bnb' } }))}
                      >
                        <option value="eth">ETH</option>
                        <option value="btc">BTC</option>
                        <option value="bnb">BNB</option>
                      </select>
                      <button type="button" className="btn-glass btn-glass--xs" onClick={() => void refresh(item.id)} disabled={state?.loading} title="Check balance">↻</button>
                      <button type="button" className="btn-glass btn-glass--xs" onClick={() => void copy(item.key_value)} title="Copy">⧉</button>
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
