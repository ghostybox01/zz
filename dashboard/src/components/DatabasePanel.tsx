import { useCallback, useEffect, useState } from 'react'
import { findings as findingsApi, type DiscoveredKey } from '../lib/reconApi'

type Props = {
  onToast: (title: string, message?: string, kind?: 'error' | 'info') => void
}

type DbTab = 'MySQL' | 'PostgreSQL' | 'Redis'

const DB_TABS: DbTab[] = ['MySQL', 'PostgreSQL', 'Redis']

function maskPass(s: string | undefined): string {
  if (!s) return ''
  if (s.length <= 4) return '***'
  return s.slice(0, 2) + '…' + '***'
}

/** Parse the detail string for display columns.
 *  Format stored by the backend varies; we keep it simple and split on ' · '.
 */
function parseDbDetail(item: DiscoveredKey): {
  host: string
  port: string
  user: string
  pass: string
  dbname: string
} {
  // Try JSON metadata first
  try {
    const m = item.metadata ? (JSON.parse(item.metadata) as Record<string, string>) : {}
    if (m.host) {
      return {
        host: m.host ?? '',
        port: m.port ?? '',
        user: m.user ?? '',
        pass: m.pass ?? '',
        dbname: m.dbname ?? '',
      }
    }
  } catch { /* fall through */ }
  // Fall back: split key_value on ':'
  // format: host:port:user:pass:dbname
  const parts = item.key_value.split(':')
  return {
    host: parts[0] ?? '',
    port: parts[1] ?? '',
    user: parts[2] ?? '',
    pass: parts[3] ?? '',
    dbname: parts[4] ?? '',
  }
}

function copyToClipboard(text: string, onToast: Props['onToast']) {
  function fallback(t: string) {
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
    navigator.clipboard.writeText(text).then(() => onToast('Copied to clipboard')).catch(() => fallback(text))
  } else {
    fallback(text)
  }
}

export function DatabasePanel({ onToast }: Props) {
  const [items, setItems] = useState<DiscoveredKey[]>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<DbTab>('MySQL')

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const r = await findingsApi.listDatabase()
      setItems(r.findings)
    } catch (e) {
      onToast('Could not load database findings', (e as Error).message, 'error')
    } finally {
      setLoading(false)
    }
  }, [onToast])

  useEffect(() => { void reload() }, [reload])

  const tabItems = items.filter((item) => item.type === activeTab)

  const remove = useCallback(async (id: number) => {
    if (!window.confirm('Delete this database finding?')) return
    try {
      await findingsApi.remove(id)
      setItems((prev) => prev.filter((k) => k.id !== id))
    } catch (e) {
      onToast('Delete failed', (e as Error).message, 'error')
    }
  }, [onToast])

  return (
    <div className="findings-panel">
      <header className="findings-panel__head">
        <div>
          <h2 className="findings-panel__title">Database — discovered credentials</h2>
          <p className="muted findings-panel__sub">
            MySQL, PostgreSQL, and Redis credentials extracted and validated by the scanner.
          </p>
        </div>
        <div style={{ display: 'flex', gap: '.5rem', alignItems: 'center' }}>
          <span className="muted" style={{ fontSize: '.8rem' }}>{items.length} total</span>
          <button type="button" className="btn-glass btn-glass--xs" onClick={() => void reload()} disabled={loading}>
            {loading ? '…' : '↻ Reload'}
          </button>
        </div>
      </header>

      {/* Tab bar */}
      <div style={{ display: 'flex', gap: '.25rem', marginBottom: '.75rem' }}>
        {DB_TABS.map((t) => {
          const count = items.filter((i) => i.type === t).length
          return (
            <button
              key={t}
              type="button"
              className={`btn-glass btn-glass--xs${activeTab === t ? ' btn-glass--on' : ''}`}
              onClick={() => setActiveTab(t)}
            >
              {t} {count > 0 && <span className="muted">({count})</span>}
            </button>
          )
        })}
      </div>

      {loading && items.length === 0 && (
        <p className="muted-callout">Loading database findings…</p>
      )}
      {!loading && tabItems.length === 0 && (
        <div className="muted-callout">
          <p style={{ margin: 0, fontWeight: 600 }}>No {activeTab} credentials discovered yet</p>
          <p className="muted" style={{ margin: '.35rem 0 0', fontSize: '.82rem' }}>
            Enable the {activeTab} addon and start a crack session. Validated credentials land here automatically.
          </p>
        </div>
      )}

      {tabItems.length > 0 && (
        <div className="findings-table-wrap">
          <table className="findings-table">
            <thead>
              <tr>
                <th>Host</th>
                {activeTab !== 'Redis' && <th>Port</th>}
                {activeTab !== 'Redis' && <th>User</th>}
                {activeTab !== 'Redis' && <th>Password</th>}
                {activeTab !== 'Redis' && <th>DB Name</th>}
                <th>Source</th>
                <th>Status</th>
                <th style={{ width: '6rem' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {tabItems.map((item) => {
                const d = parseDbDetail(item)
                const isValidated = item.status === 'valid' || item.status === 'validated'
                return (
                  <tr
                    key={item.id}
                    style={isValidated ? { background: 'rgba(0,255,100,0.04)' } : undefined}
                  >
                    <td><code className="findings-key">{activeTab === 'Redis' ? item.key_value : d.host}</code></td>
                    {activeTab !== 'Redis' && <td className="muted" style={{ fontSize: '.75rem' }}>{d.port || '—'}</td>}
                    {activeTab !== 'Redis' && <td><code>{d.user || '—'}</code></td>}
                    {activeTab !== 'Redis' && <td><code className="muted">{maskPass(d.pass) || '—'}</code></td>}
                    {activeTab !== 'Redis' && <td className="muted" style={{ fontSize: '.75rem' }}>{d.dbname || '—'}</td>}
                    <td>
                      <a href={item.source_url} target="_blank" rel="noopener noreferrer" className="findings-source" title={item.source_url}>
                        {item.source_url}
                      </a>
                    </td>
                    <td>
                      <span className={`findings-mode findings-mode--${isValidated ? 'live' : 'unknown'}`}>
                        {isValidated ? 'Validated' : 'Found'}
                      </span>
                    </td>
                    <td className="findings-actions">
                      <button
                        type="button"
                        className="btn-glass btn-glass--xs"
                        title="Copy credential line"
                        onClick={() => copyToClipboard(item.key_value, onToast)}
                      >
                        ⧉
                      </button>
                      <button
                        type="button"
                        className="btn-glass btn-glass--xs btn-glass--danger"
                        title="Delete"
                        onClick={() => void remove(item.id)}
                      >
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
