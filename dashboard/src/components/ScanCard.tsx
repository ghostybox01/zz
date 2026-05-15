import type { Scan } from '../types'
import { CpuSparkline } from './CpuSparkline'
import { fmtInt } from '../lib/format'

type Props = {
  scan: Scan
  shardCount: number
  onOpen: () => void
}

function statusTone(s: Scan['status']) {
  switch (s) {
    case 'running': return 'scan-card__status--running'
    case 'paused':  return 'scan-card__status--paused'
    case 'queued':  return 'scan-card__status--queued'
    case 'done':    return 'scan-card__status--done'
    case 'failed':  return 'scan-card__status--failed'
  }
}

function fmtElapsed(startedAtIso: string, endedAtIso?: string): string {
  const start = Date.parse(startedAtIso)
  const end = endedAtIso ? Date.parse(endedAtIso) : Date.now()
  const s = Math.max(0, Math.floor((end - start) / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m`
  return `${s}s`
}

export function ScanCard({ scan, shardCount, onOpen }: Props) {
  const progress = scan.targetCount > 0 ? Math.min(100, ((scan.validHosts + scan.invalidHosts) / scan.targetCount) * 100) : 0
  const hitRate = scan.validHosts > 0 ? (scan.validHits / scan.validHosts) * 100 : 0

  return (
    <article
      className={`scan-card scan-card--${scan.status}`}
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onOpen()
        }
      }}
    >
      <header className="scan-card__head">
        <div>
          <h3 className="scan-card__label">{scan.label}</h3>
          <p className="scan-card__id">{scan.id}</p>
        </div>
        <span className={`scan-card__status ${statusTone(scan.status)}`}>
          <span className="scan-card__status-dot" aria-hidden />
          {scan.status}
        </span>
      </header>

      <div className="scan-card__stats">
        <div className="scan-card__stat">
          <span className="scan-card__stat-label">Valid</span>
          <strong>{fmtInt(scan.validHosts)}</strong>
        </div>
        <div className="scan-card__stat">
          <span className="scan-card__stat-label">Invalid</span>
          <strong>{fmtInt(scan.invalidHosts)}</strong>
        </div>
        <div className="scan-card__stat">
          <span className="scan-card__stat-label">Hits</span>
          <strong>{fmtInt(scan.hitsFound)}</strong>
          <small className="scan-card__sub">{fmtInt(scan.validHits)} verified</small>
        </div>
        <div className="scan-card__stat scan-card__stat--span">
          <div className="scan-card__rps">
            <span className="scan-card__stat-label">Requests/sec</span>
            <strong className="scan-card__rps-val">{scan.requestsPerSec.toFixed(1)}</strong>
            <span className="scan-card__sub">parsing {scan.parsingPerSec.toFixed(0)}/s</span>
          </div>
          <div className={`scan-card__spark scan-card__spark--${scan.status}`}>
            <CpuSparkline values={scan.rpsHistory} />
          </div>
        </div>
      </div>

      <div className="scan-card__progress">
        <div className="scan-card__progress-row">
          <span>{fmtInt(scan.validHosts + scan.invalidHosts)} <span className="muted">/ {fmtInt(scan.targetCount)}</span></span>
          <span className="scan-card__mono">{progress.toFixed(1)}%</span>
        </div>
        <div className="scan-card__progress-track">
          <div className="scan-card__progress-fill" style={{ width: `${progress.toFixed(2)}%` }} />
        </div>
      </div>

      <footer className="scan-card__foot">
        <span>{shardCount} shard{shardCount === 1 ? '' : 's'}</span>
        <span className="muted">·</span>
        <span>{fmtElapsed(scan.startedAt, scan.endedAt)}</span>
        <span className="muted">·</span>
        <span>{hitRate.toFixed(2)}% hit rate</span>
      </footer>

      {scan.lastEvent ? <p className="scan-card__event">{scan.lastEvent}</p> : null}
    </article>
  )
}
