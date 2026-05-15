import { useMemo } from 'react'
import type { Finding, Scan, ScanShard, VpsNode } from '../types'
import { CpuSparkline } from './CpuSparkline'
import { FindingsBoard } from './FindingsBoard'
import { fmtInt } from '../lib/format'

type Props = {
  scan: Scan
  shards: readonly ScanShard[]
  fleet: readonly VpsNode[]
  findings: readonly Finding[]
  onBack: () => void
  onTogglePause: (scanId: string) => void
  onReplayDemo: () => void
}

function IcoBack() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" aria-hidden>
      <path d="M19 12H5M12 19l-7-7 7-7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function statusTone(s: Scan['status']) {
  return `scan-detail__status scan-detail__status--${s}`
}

export function ScanDetail({ scan, shards, fleet, findings, onBack, onTogglePause, onReplayDemo }: Props) {
  const scopedFindings = useMemo(() => {
    const fromScan = findings.filter((f) => f.scanId === scan.id)
    if (fromScan.length > 0) return fromScan
    // Fallback: show findings from VPSes that participate in this scan (so the
    // demo seed without scanId still gets a relevant slice).
    const ids = new Set(scan.shardVpsIds)
    return findings.filter((f) => ids.has(f.reportedByHost))
  }, [findings, scan.id, scan.shardVpsIds])

  const totalAssigned = shards.reduce((s, sh) => s + sh.assigned, 0)
  const totalDone = shards.reduce((s, sh) => s + sh.done, 0)
  const progress = totalAssigned > 0 ? (totalDone / totalAssigned) * 100 : 0
  const hitRate = scan.validHosts > 0 ? (scan.validHits / scan.validHosts) * 100 : 0

  const fleetById = useMemo(() => {
    const m = new Map<string, VpsNode>()
    for (const n of fleet) m.set(n.id, n)
    return m
  }, [fleet])

  return (
    <section className="scan-detail">
      <header className="scan-detail__head">
        <button type="button" className="scan-detail__back" onClick={onBack} aria-label="Back to scans">
          <IcoBack />
          <span>Scans</span>
        </button>
        <div className="scan-detail__title-block">
          <h2 className="scan-detail__title">{scan.label}</h2>
          <p className="scan-detail__sub">
            <span className="scan-detail__id">{scan.id}</span>
            {scan.snapshots.length > 0 && (
              <>
                <span className="muted"> · </span>
                {scan.snapshots.map((s) => (
                  <span key={s} className="chip chip--small">{s}</span>
                ))}
              </>
            )}
          </p>
        </div>
        <span className={statusTone(scan.status)}>
          <span className="scan-detail__status-dot" aria-hidden />
          {scan.status}
        </span>
        <div className="scan-detail__actions">
          {(scan.status === 'running' || scan.status === 'paused') && (
            <button type="button" className="btn-secondary" onClick={() => onTogglePause(scan.id)}>
              {scan.status === 'running' ? 'Pause scan' : 'Resume scan'}
            </button>
          )}
        </div>
      </header>

      <div className="scan-detail__kpis">
        <div className="scan-detail__kpi scan-detail__kpi--green">
          <span className="scan-detail__kpi-label">Valid hosts</span>
          <span className="scan-detail__kpi-value">{fmtInt(scan.validHosts)}</span>
          <span className="scan-detail__kpi-sub">responsive / 2xx-3xx</span>
        </div>
        <div className="scan-detail__kpi scan-detail__kpi--muted">
          <span className="scan-detail__kpi-label">Invalid hosts</span>
          <span className="scan-detail__kpi-value">{fmtInt(scan.invalidHosts)}</span>
          <span className="scan-detail__kpi-sub">errors / 4xx-5xx</span>
        </div>
        <div className="scan-detail__kpi">
          <span className="scan-detail__kpi-label">Hits found</span>
          <span className="scan-detail__kpi-value">{fmtInt(scan.hitsFound)}</span>
          <span className="scan-detail__kpi-sub">raw matches</span>
        </div>
        <div className="scan-detail__kpi scan-detail__kpi--gold">
          <span className="scan-detail__kpi-label">Verified hits</span>
          <span className="scan-detail__kpi-value">{fmtInt(scan.validHits)}</span>
          <span className="scan-detail__kpi-sub">{hitRate.toFixed(2)}% hit rate</span>
        </div>
        <div className="scan-detail__kpi">
          <span className="scan-detail__kpi-label">Parsing/sec</span>
          <span className="scan-detail__kpi-value">{scan.parsingPerSec.toFixed(0)}</span>
          <span className="scan-detail__kpi-sub">payloads/s</span>
        </div>
        <div className="scan-detail__kpi scan-detail__kpi--rps">
          <span className="scan-detail__kpi-label">Requests/sec</span>
          <span className="scan-detail__kpi-value">{scan.requestsPerSec.toFixed(1)}</span>
          <div className="scan-detail__kpi-spark">
            <CpuSparkline values={scan.rpsHistory} />
          </div>
        </div>
      </div>

      <div className="scan-detail__progress">
        <div className="scan-detail__progress-head">
          <span className="muted">Workload progress</span>
          <span className="scan-detail__mono">
            {fmtInt(totalDone)} <span className="muted">/ {fmtInt(totalAssigned || scan.targetCount)}</span>
            <span className="muted"> · </span>
            {progress.toFixed(1)}%
          </span>
        </div>
        <div className="scan-detail__progress-track">
          <div className="scan-detail__progress-fill" style={{ width: `${progress.toFixed(2)}%` }} />
        </div>
      </div>

      <section className="card-block card-block--tight scan-detail__shards">
        <div className="card-block__head">
          <h3 style={{ margin: 0 }}>Per-VPS shards <span className="muted">({shards.length})</span></h3>
          <p className="card-block__lede card-block__lede--short">
            Each row is one VPS worker processing its slice of the target list.
          </p>
        </div>
        <div className="shard-table-wrap">
          <table className="shard-table">
            <thead>
              <tr>
                <th>Worker</th>
                <th>Region</th>
                <th>Progress</th>
                <th>Valid</th>
                <th>Invalid</th>
                <th>Hits</th>
                <th>Parsing/s</th>
                <th>Req/s</th>
              </tr>
            </thead>
            <tbody>
              {shards.map((sh) => {
                const node = fleetById.get(sh.vpsId)
                const pct = sh.assigned > 0 ? Math.min(100, (sh.done / sh.assigned) * 100) : 0
                return (
                  <tr key={`${sh.scanId}-${sh.vpsId}`}>
                    <td className="shard-table__mono">{node?.label ?? sh.vpsId}</td>
                    <td>{node?.region ?? '—'}</td>
                    <td>
                      <div className="shard-table__progress" title={`${pct.toFixed(1)}%`}>
                        <div className="shard-table__progress-track">
                          <div className="shard-table__progress-fill" style={{ width: `${pct.toFixed(2)}%` }} />
                        </div>
                        <span className="shard-table__mono shard-table__progress-pct">{pct.toFixed(0)}%</span>
                      </div>
                    </td>
                    <td className="shard-table__num">{fmtInt(sh.validHosts)}</td>
                    <td className="shard-table__num">{fmtInt(sh.invalidHosts)}</td>
                    <td className="shard-table__num shard-table__num--accent">{fmtInt(sh.hits)}</td>
                    <td className="shard-table__num">{sh.parsingPerSec.toFixed(0)}</td>
                    <td className="shard-table__num">{sh.requestsPerSec.toFixed(1)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </section>

      <FindingsBoard findings={scopedFindings} onReplayDemo={onReplayDemo} />
    </section>
  )
}
