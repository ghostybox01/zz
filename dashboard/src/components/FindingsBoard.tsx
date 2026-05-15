import { useMemo, useState } from 'react'
import type { Finding } from '../types'
import { findingCredentialText } from '../lib/findingCredential'
import { FindingDetail } from './FindingDetail'

type SortKey = 'provider' | 'at' | 'severity'

type Props = {
  findings: readonly Finding[]
  onReplayDemo: () => void
  onClearAll?: () => void | Promise<void>
}

function shortId(id: string): string {
  const tail = id.slice(-8).replace(/^:/, '')
  return `#${tail.padStart(6, '0').slice(-6)}`
}

function ptsForSeverity(s: Finding['severity']): number {
  switch (s) {
    case 'critical':
      return 5
    case 'high':
      return 3
    case 'medium':
      return 2
    default:
      return 1
  }
}

function vulnTag(rule: string): string {
  const u = rule.toUpperCase()
  if (u.includes('KEY') || u.includes('TOKEN') || u.includes('SECRET')) return 'CRED'
  if (u.includes('SMTP') || u.includes('WEBHOOK')) return 'PATH'
  return 'LIB'
}

export function FindingsBoard({ findings, onReplayDemo, onClearAll }: Props) {
  const [sortKey, setSortKey] = useState<SortKey>('provider')
  const [dir, setDir] = useState<'asc' | 'desc'>('asc')
  const [addon, setAddon] = useState<string>('all')
  const [query, setQuery] = useState('')
  const [pageSize, setPageSize] = useState(50)
  const [activeId, setActiveId] = useState<string | null>(null)

  const addonChoices = useMemo(() => {
    const s = new Set<string>()
    for (const f of findings) s.add(f.provider)
    return ['all', ...[...s].sort((a, b) => a.localeCompare(b))]
  }, [findings])

  const sortedFiltered = useMemo(() => {
    const copy = [...findings]
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
    return copy.filter((f) => {
      if (addon !== 'all' && f.provider !== addon) return false
      if (!q) return true
      return (
        f.hostname.toLowerCase().includes(q) ||
        f.detail.toLowerCase().includes(q) ||
        findingCredentialText(f).toLowerCase().includes(q) ||
        f.ruleLabel.toLowerCase().includes(q) ||
        f.id.toLowerCase().includes(q)
      )
    })
  }, [findings, sortKey, dir, addon, query])

  const activeFilters = (addon !== 'all' ? 1 : 0) + (query.trim() ? 1 : 0)
  const rows = sortedFiltered.slice(0, Math.max(1, pageSize))

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

  const openFinding = (id: string) => setActiveId(id)

  const copyRow = async (f: Finding) => {
    const line = [f.provider, f.hostname, f.ruleLabel, findingCredentialText(f), f.severity, f.at].join('\t')
    try {
      await navigator.clipboard.writeText(line)
    } catch {
      /* ignore */
    }
  }

  const activeFinding = activeId ? sortedFiltered.find((f) => f.id === activeId) ?? findings.find((f) => f.id === activeId) ?? null : null

  if (activeFinding) {
    return (
      <section className="card-block card-block--hits card-block--detail">
        <FindingDetail
          finding={activeFinding}
          onBack={() => setActiveId(null)}
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
          <input
            className="hits-search"
            type="search"
            placeholder="Search path, hostname, rule, id…"
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
            Export
          </button>
          <button type="button" className="btn-glass btn-hit-tool" onClick={onReplayDemo}>
            Seed demo
          </button>
          <button
            type="button"
            className="btn-danger-outline btn-hit-tool"
            disabled={!onClearAll || findings.length === 0}
            onClick={() => {
              if (!onClearAll) return
              if (window.confirm(`Clear all ${findings.length} findings? This calls /api/clear on the backend.`)) {
                void onClearAll()
              }
            }}
            title={onClearAll ? 'POST /api/clear — clears credentials + result files' : 'Needs live backend'}
          >
            Delete all
          </button>
        </div>
      </div>

      <div className="findings-meta">
        <span>
          Showing <strong>{rows.length}</strong> of <strong>{sortedFiltered.length}</strong>
        </span>
        {findings.length !== sortedFiltered.length ? (
          <span className="findings-meta__pipe">
            {' '}
            · {findings.length} total in inbox
          </span>
        ) : null}
      </div>

      <div className="findings-scroll-wrap findings-scroll-wrap--tall">
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
                  <th className="th-narrow th-right">Pts</th>
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
                  const pts = ptsForSeverity(f.severity)
                  const pathish = f.path ?? (f.url ? f.url.replace(/^https?:\/\/[^/]+/, '') || '/' : '')
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
                      </td>
                      <td>
                        <span className="vuln-pill">{vulnTag(f.ruleLabel)}</span>
                      </td>
                      <td>
                        <span className={`status-pill-valid${f.details?.validated === false ? ' status-pill-valid--pending' : ''}`}>
                          <span className="status-pill-valid__dot" aria-hidden />
                          {f.details?.validated === false ? 'PENDING' : 'VALID'}
                        </span>
                      </td>
                      <td className="mono-cell th-right pts-cell">{pts > 1 ? `+${pts}` : '—'}</td>
                      <td>
                        <span className={`severity-pill severity-pill--${f.severity}`}>{f.severity}</span>
                      </td>
                      <td className="mono-cell small-cell date-cell">
                        {new Date(f.at).toLocaleString()}
                      </td>
                      <td className="row-actions" onClick={(e) => e.stopPropagation()}>
                        <span className="finding-chevron" aria-hidden>›</span>
                        <button type="button" className="icon-btn" title="Copy row" onClick={() => void copyRow(f)}>
                          ⧉
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
