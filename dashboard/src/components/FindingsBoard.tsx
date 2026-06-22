import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Finding } from '../types'
import { findingCredentialText } from '../lib/findingCredential'
import { FindingDetail } from './FindingDetail'
import { TableToolbar } from './TableToolbar'
import { credentials as credApi, findings as findingsApi } from '../lib/reconApi'
import { mapRecent } from '../hooks/useReconStats'

type SortKey = 'provider' | 'at' | 'severity'

type Props = {
  findings: readonly Finding[]
  onClearAll?: () => void | Promise<void>  // kept in props for backward compat but no longer rendered
  /**
   * Optional bulk-remove callback. When omitted, the panel only renders
   * filtered rows but cannot mutate parent state — trash + bulk delete
   * will be inert. App.tsx wires this to `setFindings` for client-side
   * pruning (the backend ledger is owned by /api/clear, not /remove).
   */
  onRemoveFindings?: (ids: readonly string[]) => void
  /** Toast callback — replaces browser alert() calls. */
  onToast?: (title: string, message?: string, kind?: 'error' | 'info') => void
}

function shortId(id: string): string {
  const tail = id.slice(-8).replace(/^:/, '')
  return `#${tail.padStart(6, '0').slice(-6)}`
}

function vulnTag(rule: string, method?: string): string {
  // Discovery method takes priority (this shows HOW the key leaked)
  if (method) {
    const m = method.toLowerCase()
    if (m === '.env') return '.env'
    if (m === 'phpinfo') return 'phpinfo'
    if (m === '.git') return '.git'
    if (m === 'wp-config') return 'wp-cfg'
    if (m === 'config' || m === 'xml-config') return 'config'
    if (m === 'settings') return 'settings'
    if (m === 'spring') return 'spring'
    if (m === 'db-config') return 'db.cfg'
    if (m === 'docker') return 'docker'
    if (m === 'backup') return 'backup'
    if (m === 'debug') return 'debug'
    if (m === 'api-endpoint') return 'api'
    if (m === 'package') return 'pkg'
    if (m === 'log') return 'log'
  }
  // Fall back to credential type from ruleLabel
  const u = rule.toUpperCase()
  if (u.includes('SMTP')) return 'SMTP'
  if (u.includes('AWS')) return 'AWS'
  if (u.includes('STRIPE') || u.includes('PAYPAL')) return 'PMT'
  if (u.includes('GITHUB') || u.includes('GITLAB')) return 'GIT'
  if (u.includes('SLACK') || u.includes('WEBHOOK')) return 'HOOK'
  if (u.includes('OPENAI') || u.includes('ANTHROPIC')) return 'AI'
  return 'page'
}

export function FindingsBoard({ findings, onClearAll: _onClearAll, onRemoveFindings, onToast }: Props) {
  const [sortKey, setSortKey] = useState<SortKey>('provider')
  const [dir, setDir] = useState<'asc' | 'desc'>('asc')
  const [addon, setAddon] = useState<string>('all')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [vulnFilter, setVulnFilter] = useState<string>('all')
  const [query, setQuery] = useState('')
  const [pageSize, setPageSize] = useState(50)
  const [activeId, setActiveId] = useState<string | null>(null)
  type RecheckOverride = { extra: ReadonlyArray<{ key: string; value: string }>; live: boolean }
  const [recheckResults, setRecheckResults] = useState<Map<string, RecheckOverride>>(new Map())
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [filter, setFilter] = useState('')
  const [confirmDialog, setConfirmDialog] = useState<{ message: string; onConfirm: () => void } | null>(null)

  // Self-load full hits from dedicated endpoint — avoids the polling stats
  // endpoint's LIMIT 200 cap that feeds the activity feed / overview widgets.
  const [ownFindings, setOwnFindings] = useState<Finding[] | null>(null)
  const [ownLoading, setOwnLoading] = useState(true)
  const [ownError, setOwnError] = useState<string | null>(null)
  const [dbTotal, setDbTotal] = useState<number | null>(null)

  const loadOwnFindings = useCallback(async () => {
    setOwnLoading(true)
    setOwnError(null)
    try {
      const r = await findingsApi.listHits(2000, 0)
      setOwnFindings(r.findings.map((row, i) => mapRecent(row, i)))
      if (r.total != null) setDbTotal(r.total)
    } catch (e) {
      setOwnError((e as Error).message)
    } finally {
      setOwnLoading(false)
    }
  }, [])

  useEffect(() => { void loadOwnFindings() }, [loadOwnFindings])

  // Prefer self-loaded data; fall back to prop data while loading
  const allFindings = ownFindings ?? findings

  const addonChoices = useMemo(() => {
    const s = new Set<string>()
    for (const f of allFindings) s.add(f.provider)
    return ['all', ...[...s].sort((a, b) => a.localeCompare(b))]
  }, [allFindings])

  const vulnChoices = useMemo(() => {
    const s = new Set<string>()
    for (const f of allFindings) s.add(vulnTag(f.ruleLabel, f.discoveryMethod))
    return ['all', ...[...s].sort((a, b) => a.localeCompare(b))]
  }, [allFindings])

  const sortedFiltered = useMemo(() => {
    const copy = [...allFindings]
    const mult = dir === 'asc' ? 1 : -1
    copy.sort((a, b) => {
      if (sortKey === 'at')
        return (new Date(a.at).getTime() - new Date(b.at).getTime()) * mult
      if (sortKey === 'severity') {
        const rank: Record<Finding['severity'], number> = {
          critical: 0,
          high: 1,
          medium: 2,
          low: 3,
        }
        const d = rank[a.severity] - rank[b.severity]
        if (d !== 0) return d * mult
      }
      return a.provider.localeCompare(b.provider) * mult
    })

    const q = query.trim().toLowerCase()
    const tf = filter.trim().toLowerCase()
    return copy.filter((f) => {
      if (addon !== 'all' && f.provider !== addon) return false
      if (statusFilter !== 'all' && f.status !== statusFilter) return false
      if (vulnFilter !== 'all' && vulnTag(f.ruleLabel, f.discoveryMethod) !== vulnFilter) return false
      const matchesText = (needle: string) =>
        f.hostname.toLowerCase().includes(needle) ||
        f.detail.toLowerCase().includes(needle) ||
        findingCredentialText(f).toLowerCase().includes(needle) ||
        f.ruleLabel.toLowerCase().includes(needle) ||
        f.provider.toLowerCase().includes(needle) ||
        f.id.toLowerCase().includes(needle)
      if (q && !matchesText(q)) return false
      if (tf && !matchesText(tf)) return false
      return true
    })
  }, [allFindings, sortKey, dir, addon, statusFilter, vulnFilter, query, filter])

  const activeFilters = (addon !== 'all' ? 1 : 0) + (statusFilter !== 'all' ? 1 : 0) + (vulnFilter !== 'all' ? 1 : 0) + (query.trim() ? 1 : 0)
  const [page, setPage] = useState(0)
  // Reset to page 0 whenever the filtered set changes
  useEffect(() => setPage(0), [sortedFiltered.length, pageSize, addon, statusFilter, vulnFilter, query, filter])
  const pageCount = Math.max(1, Math.ceil(sortedFiltered.length / pageSize))
  const safePage = Math.min(page, pageCount - 1)
  const rows = sortedFiltered.slice(safePage * pageSize, (safePage + 1) * pageSize)

  function toggle(head: SortKey) {
    if (sortKey !== head) {
      setSortKey(head)
      setDir(head === 'at' ? 'desc' : 'asc')
    } else setDir((d) => (d === 'asc' ? 'desc' : 'asc'))
  }

  const exportFiltered = () => {
    const blob = new Blob([JSON.stringify(sortedFiltered, null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `hits-export-${sortedFiltered.length}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  const exportCsv = () => {
    const header = ['provider', 'credential', 'source_url', 'status', 'severity', 'discovered_at']
    const rows = sortedFiltered.map((f) => [
      f.provider,
      findingCredentialText(f),
      f.hostname,
      f.status ?? '',
      f.severity,
      f.at,
    ].map((v) => `"${String(v).replace(/"/g, '""')}"`).join(','))
    const csv = [header.join(','), ...rows].join('\n')
    const blob = new Blob([csv], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `hits-export-${sortedFiltered.length}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  const [loadingMore, setLoadingMore] = useState(false)
  const loadMore = useCallback(async () => {
    if (!ownFindings || loadingMore) return
    setLoadingMore(true)
    try {
      const r = await findingsApi.listHits(2000, ownFindings.length)
      if (r.findings.length > 0) {
        const newRows = r.findings.map((row, i) => mapRecent(row, ownFindings.length + i))
        setOwnFindings((prev) => (prev ? [...prev, ...newRows] : newRows))
      }
      if (r.total != null) setDbTotal(r.total)
    } catch (e) {
      onToast?.('Load more failed', (e as Error).message, 'error')
    } finally {
      setLoadingMore(false)
    }
  }, [ownFindings, loadingMore, onToast])

  const openFinding = (id: string) => setActiveId(id)

  const copyRow = (f: Finding) => {
    const line = [f.provider, f.hostname, f.ruleLabel, findingCredentialText(f), f.severity, f.at].join('\t')
    function fallbackCopy() {
      try {
        const ta = document.createElement('textarea')
        ta.value = line
        ta.style.cssText = 'position:fixed;left:-9999px;top:-9999px;opacity:0'
        document.body.appendChild(ta)
        ta.select()
        document.execCommand('copy')
        document.body.removeChild(ta)
      } catch { /* ignore */ }
    }
    if (navigator.clipboard) {
      navigator.clipboard.writeText(line).catch(() => fallbackCopy())
    } else {
      fallbackCopy()
    }
  }

  const activeFinding = activeId ? sortedFiltered.find((f) => f.id === activeId) ?? allFindings.find((f) => f.id === activeId) ?? null : null

  // Prune stale selections when the underlying findings list shrinks.
  useEffect(() => {
    setSelected((prev) => {
      if (prev.size === 0) return prev
      const valid = new Set(allFindings.map((f) => f.id))
      let changed = false
      const next = new Set<string>()
      for (const id of prev) {
        if (valid.has(id)) next.add(id)
        else changed = true
      }
      return changed ? next : prev
    })
  }, [allFindings])

  const toggleOne = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const handleSelectAll = () => setSelected(new Set(sortedFiltered.map((r) => r.id)))
  const handleClearSelection = () => setSelected(new Set())
  const handleDeleteSelected = () => {
    const ids = [...selected]
    if (ids.length === 0) return
    onRemoveFindings?.(ids)
    setSelected(new Set())
  }
  const handleRowTrash = (f: Finding) => {
    setConfirmDialog({
      message: `Delete finding ${shortId(f.id)} (${f.provider} · ${f.hostname})?`,
      onConfirm: () => {
        onRemoveFindings?.([f.id])
        setSelected((prev) => {
          if (!prev.has(f.id)) return prev
          const next = new Set(prev)
          next.delete(f.id)
          return next
        })
      },
    })
  }

  async function handleRecheck(f: Finding) {
    const numId = parseInt(f.id, 10)
    if (isNaN(numId)) return
    try {
      const res = await credApi.recheck(numId)
      setRecheckResults((prev) => new Map(prev).set(f.id, { extra: res.extra ?? [], live: res.live }))
      if (res.live) {
        onToast?.('Credential LIVE', res.info || 'Credential is valid.', 'info')
      } else {
        onToast?.('Credential DEAD', res.info || 'Credential no longer valid.', 'error')
      }
    } catch (e) {
      onToast?.('Recheck failed', e instanceof Error ? e.message : String(e), 'error')
    }
  }

  async function handleResend(f: Finding) {
    const numId = parseInt(f.id, 10)
    if (isNaN(numId)) return
    try {
      const res = await credApi.resend(numId)
      if (res.ok) {
        onToast?.('Sent to Telegram', undefined, 'info')
      } else {
        onToast?.('Resend failed', res.error ?? 'Unknown error', 'error')
      }
    } catch (e) {
      onToast?.('Resend failed', e instanceof Error ? e.message : String(e), 'error')
    }
  }

  if (activeFinding) {
    const numId = parseInt(activeFinding.id, 10)
    const override = recheckResults.get(activeFinding.id)
    const displayFinding = override
      ? {
          ...activeFinding,
          details: {
            ...activeFinding.details,
            extra: override.extra,
            validated: override.live,
          },
        }
      : activeFinding
    return (
      <section className="card-block card-block--hits card-block--detail">
        <FindingDetail
          finding={displayFinding}
          onBack={() => setActiveId(null)}
          onRecheck={!isNaN(numId) ? handleRecheck : undefined}
          onResend={!isNaN(numId) ? handleResend : undefined}
        />
      </section>
    )
  }

  return (
    <section className="card-block card-block--hits">
      <div className="card-block__head card-block__head--row hits-card-head">
        <div>
          <h2>Hits</h2>
          <p className="card-block__lede card-block__lede--short">
            Production-style ledger — filters, status chips, severity points, row detail.
          </p>
        </div>
      </div>

      <TableToolbar
        totalRows={sortedFiltered.length}
        selectedCount={selected.size}
        filter={filter}
        onFilterChange={setFilter}
        onSelectAll={handleSelectAll}
        onClearSelection={handleClearSelection}
        onDeleteSelected={handleDeleteSelected}
        filterPlaceholder="Filter by host, rule, credential, id…"
      />

      <div className="hits-toolbar">
        <div className="hits-toolbar__filters">
          <span className="filters-badge">
            Filters
            {activeFilters ? <span className="filters-badge__n">{activeFilters}</span> : null}
          </span>
          <select
            className="hits-select"
            value={addon}
            onChange={(e) => setAddon(e.target.value)}
            aria-label="Filter hits by addon vendor"
          >
            {addonChoices.map((p) => (
              <option key={p} value={p}>
                {p === 'all' ? 'All addons' : p}
              </option>
            ))}
          </select>
          <select
            className="hits-select"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            aria-label="Filter by status"
          >
            <option value="all">All status</option>
            <option value="hit">Hit</option>
            <option value="valid">Valid</option>
            <option value="dead">Dead</option>
          </select>
          <select
            className="hits-select"
            value={vulnFilter}
            onChange={(e) => setVulnFilter(e.target.value)}
            aria-label="Filter by vuln type"
          >
            {vulnChoices.map((v) => (
              <option key={v} value={v}>
                {v === 'all' ? 'All types' : v}
              </option>
            ))}
          </select>
          <input
            className="hits-search"
            type="search"
            placeholder="Search host, credential, rule…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label="Search hits"
          />
        </div>
        <div className="hits-toolbar__actions">
          <label className="hits-toolbar-lbl hits-toolbar-lbl--row">
            <span>Rows</span>
            <select
              className="hits-select hits-select--sm"
              value={pageSize}
              onChange={(e) => setPageSize(Number(e.target.value))}
            >
              {[25, 50, 100, 250].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
          <button type="button" className="btn-glass btn-hit-tool" onClick={exportFiltered}>
            JSON
          </button>
          <button type="button" className="btn-glass btn-hit-tool" onClick={exportCsv}>
            CSV
          </button>
          <a href="/api/export/hits.csv" download className="btn-glass btn-hit-tool"
            style={{ textDecoration: 'none', lineHeight: '1' }}
            title="Download all hits from DB (no row cap)">
            All CSV
          </a>
        </div>
      </div>

      <div className="findings-meta">
        <span>
          {ownLoading
            ? <span className="muted">Loading all hits…</span>
            : <><strong>{rows.length}</strong> of <strong>{sortedFiltered.length}</strong> visible</>
          }
        </span>
        {!ownLoading && allFindings.length !== sortedFiltered.length ? (
          <span className="findings-meta__pipe"> · {allFindings.length} loaded</span>
        ) : null}
        {!ownLoading && dbTotal != null && dbTotal > allFindings.length ? (
          <span className="findings-meta__pipe" style={{ color: '#f59e0b' }}> · {dbTotal.toLocaleString()} in DB</span>
        ) : null}
        {ownError && (
          <span className="muted" style={{ color: '#f87171', marginLeft: '.5rem', fontSize: '.75rem' }}>
            load error — showing cached data
          </span>
        )}
        <button
          type="button"
          className="btn-glass btn-glass--xs"
          style={{ marginLeft: '.5rem' }}
          onClick={() => void loadOwnFindings()}
          disabled={ownLoading}
          title="Reload from database"
        >
          {ownLoading ? '…' : '↻'}
        </button>
        {!ownLoading && dbTotal != null && dbTotal > allFindings.length && (
          <button
            type="button"
            className="btn-glass btn-glass--xs"
            style={{ marginLeft: '.25rem', color: '#f59e0b', borderColor: 'rgba(245,158,11,.3)' }}
            onClick={() => void loadMore()}
            disabled={loadingMore}
            title={`Load next 2000 (${dbTotal - allFindings.length} remaining)`}
          >
            {loadingMore ? '…' : `+ Load more (${(dbTotal - allFindings.length).toLocaleString()})`}
          </button>
        )}
      </div>

      <div className="findings-scroll-wrap findings-scroll-wrap--tall" key={safePage}>
        <div className="findings-shell">
          {rows.length === 0 ? (
            <div className="hits-empty">
              <div className="hits-empty__ring" aria-hidden />
              <p className="hits-empty__title">No rows match this lens</p>
              <p className="hits-empty__sub">Clear search or set addon to “All” to widen the net.</p>
            </div>
          ) : (
            <table className="finding-table finding-table--rich">
              <thead>
                <tr>
                  <th className="th-narrow th-check" aria-label="Select row" />
                  <th className="th-narrow">ID</th>
                  <th aria-sort={sortKey === 'provider' ? ariaDir(dir) : undefined}>
                    <button type="button" className="tbl-sort" onClick={() => toggle('provider')}>
                      Addon {hint(sortKey === 'provider', dir)}
                    </button>
                  </th>
                  <th>Path</th>
                  <th>Credential</th>
                  <th className="th-narrow">Vuln</th>
                  <th className="th-narrow">Status</th>
                  <th aria-sort={sortKey === 'severity' ? ariaDir(dir) : undefined}>
                    <button type="button" className="tbl-sort" onClick={() => toggle('severity')}>
                      Sev {hint(sortKey === 'severity', dir)}
                    </button>
                  </th>
                  <th aria-sort={sortKey === 'at' ? ariaDir(dir) : undefined}>
                    <button type="button" className="tbl-sort" onClick={() => toggle('at')}>
                      Date {hint(sortKey === 'at', dir)}
                    </button>
                  </th>
                  <th className="th-narrow"> </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((f) => {
                  // Actual path portion of source URL (strip scheme+host)
                  const rawPath = f.path ?? (f.url ? f.url.replace(/^https?:\/\/[^/]+/, '') || '' : '')
                  // For pure-domain source_url (no path), check if discoveryMethod gives more context
                  const pathish = rawPath && rawPath !== '/' ? rawPath
                    : (f.discoveryMethod && f.discoveryMethod !== 'page' ? `[${f.discoveryMethod}]` : '')
                  return (
                    <tr
                      key={f.id}
                      className="finding-row finding-row--clickable"
                      onClick={() => openFinding(f.id)}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(ev) => {
                        if (ev.key === 'Enter' || ev.key === ' ') {
                          ev.preventDefault()
                          openFinding(f.id)
                        }
                      }}
                    >
                      <td className="th-check" onClick={(e) => e.stopPropagation()}>
                        <input
                          type="checkbox"
                          checked={selected.has(f.id)}
                          onChange={() => toggleOne(f.id)}
                          aria-label={`Select ${shortId(f.id)}`}
                        />
                      </td>
                      <td className="mono-cell finding-id">{shortId(f.id)}</td>
                      <td>
                        <span className="provider-badge provider-badge--lg">{f.provider}</span>
                      </td>
                      <td className="mono-cell path-cell" title={f.url ?? f.hostname}>
                        <span className="path-cell__host">{f.hostname}</span>
                        {pathish && pathish !== '/' && <span className="path-cell__path muted">{pathish}</span>}
                      </td>
                      <td className="mono-cell cred-cell" title={findingCredentialText(f)}>
                        <code className="cred-cell__code">{findingCredentialText(f)}</code>
                        {f.details?._verifySnippet && (
                          <span className="cred-meta-pill">{f.details._verifySnippet}</span>
                        )}
                      </td>
                      <td>
                        <span className="vuln-pill">{vulnTag(f.ruleLabel, f.discoveryMethod)}</span>
                      </td>
                      <td>
                        {f.status === 'dead' ? (
                          <span className="status-pill-dead">
                            <span className="status-pill-dead__dot" aria-hidden />
                            DEAD
                          </span>
                        ) : f.status === 'hit' ? (
                          <span className="status-pill-hit">
                            <span className="status-pill-hit__dot" aria-hidden />
                            HIT
                          </span>
                        ) : (
                          <span className="status-pill-valid">
                            <span className="status-pill-valid__dot" aria-hidden />
                            VALID
                          </span>
                        )}
                        {f.details?._hasSub && (
                          <span className="sub-bell" title="Active subscription / balance">🔔</span>
                        )}
                      </td>
                      <td>
                        <span className={`severity-pill severity-pill--${f.severity}`}>{f.severity}</span>
                      </td>
                      <td className="mono-cell small-cell date-cell">
                        {new Date(f.at).toLocaleString()}
                      </td>
                      <td className="row-actions" onClick={(e) => e.stopPropagation()}>
                        <span className="finding-chevron" style={{cursor:'pointer'}} onClick={() => openFinding(f.id)} title="View detail">›</span>
                        <button type="button" className="icon-btn" title="Copy row" onClick={() => void copyRow(f)}>
                          ⧉
                        </button>
                        <button
                          type="button"
                          className="icon-btn"
                          title="Delete row"
                          onClick={() => handleRowTrash(f)}
                          disabled={!onRemoveFindings}
                        >
                          🗑
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {pageCount > 1 && (
        <div className="hits-pagination">
          <button type="button" className="btn-glass btn-glass--xs" disabled={safePage === 0} onClick={() => setPage(0)} title="First page">«</button>
          <button type="button" className="btn-glass btn-glass--xs" disabled={safePage === 0} onClick={() => setPage(p => Math.max(0, p - 1))} title="Previous page">‹</button>
          <span className="hits-pagination__info">
            Page {safePage + 1} of {pageCount}
            <span className="muted"> · {sortedFiltered.length.toLocaleString()} rows</span>
          </span>
          <button type="button" className="btn-glass btn-glass--xs" disabled={safePage >= pageCount - 1} onClick={() => setPage(p => Math.min(pageCount - 1, p + 1))} title="Next page">›</button>
          <button type="button" className="btn-glass btn-glass--xs" disabled={safePage >= pageCount - 1} onClick={() => setPage(pageCount - 1)} title="Last page">»</button>
        </div>
      )}

      {confirmDialog && (
        <div className="confirm-overlay" onClick={() => setConfirmDialog(null)} role="dialog" aria-modal="true">
          <div className="confirm-dialog" onClick={(e) => e.stopPropagation()}>
            <p className="confirm-dialog__msg">{confirmDialog.message}</p>
            <div className="confirm-dialog__actions">
              <button type="button" className="btn-glass btn-glass--sm" onClick={() => setConfirmDialog(null)}>
                Cancel
              </button>
              <button
                type="button"
                className="btn-danger-outline confirm-dialog__confirm"
                onClick={() => { confirmDialog.onConfirm(); setConfirmDialog(null) }}
              >
                Confirm
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}

function hint(active: boolean, direction: 'asc' | 'desc') {
  if (!active) return ''
  return direction === 'asc' ? '^' : 'v'
}

function ariaDir(direction: 'asc' | 'desc'): 'ascending' | 'descending' {
  return direction === 'asc' ? 'ascending' : 'descending'
}
