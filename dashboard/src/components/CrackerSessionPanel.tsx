import { CpuSparkline } from './CpuSparkline'
import type { Scan } from '../types'
import { fmtInt } from '../lib/format'

type Props = {
  scan: Scan
  onStop: () => void
  onViewStats: () => void
}

function fmtRps(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return n.toFixed(1)
}

export function CrackerSessionPanel({ scan, onStop, onViewStats }: Props) {
  const progress =
    scan.targetCount > 0
      ? Math.min(100, ((scan.validHosts + scan.invalidHosts) / scan.targetCount) * 100)
      : 0
  const ppsHistory = scan.rpsHistory.map((v) => Math.max(0, v * 1.4 + 40))

  return (
    <section className="cw-session">
      <header className="cw-session__head">
        <div>
          <h2 className="cw-session__title">
            {scan.label.split('·')[0]?.trim().toUpperCase() ?? scan.label.toUpperCase()}
            <span className="cw-session__id">
              #{scan.id.replace(/\D/g, '').slice(-4) || scan.id.slice(-4)}
            </span>
          </h2>
          <p className="cw-session__started muted">
            Started {scan.startedAt ? new Date(scan.startedAt).toLocaleString() : 'N/A'}
          </p>
        </div>
        <span className={`cw-session__run cw-session__run--${scan.status}`}>
          <span className="cw-session__run-dot" aria-hidden />
          {scan.status === 'running' ? 'Running'
            : scan.status === 'paused' ? 'Paused'
            : scan.status === 'done' ? 'Completed'
            : scan.status === 'failed' ? 'Failed'
            : 'Queued'}
        </span>
      </header>

      <div className="cw-session__hero">
        <article className="cw-metric cw-metric--gold">
          <div className="cw-metric__icon" aria-hidden>
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75">
              <path d="M4 14v4M8 10v8M12 6v12M16 10v8M20 14v4" strokeLinecap="round" />
            </svg>
          </div>
          <div className="cw-metric__body">
            <span className="cw-metric__label">Requests / Sec</span>
            <strong className="cw-metric__value">{fmtRps(scan.requestsPerSec)}</strong>
            <span className="cw-metric__unit">RPS</span>
          </div>
          <div className="cw-metric__bars" aria-hidden>
            {scan.rpsHistory.slice(-8).map((v, i) => (
              <span key={i} style={{ height: `${Math.min(100, (v / 120) * 100)}%` }} />
            ))}
          </div>
        </article>

        <article className="cw-metric cw-metric--purple">
          <div className="cw-metric__icon" aria-hidden>
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 7v5l3 2" strokeLinecap="round" />
            </svg>
          </div>
          <div className="cw-metric__body">
            <span className="cw-metric__label">Parsing / Sec</span>
            <strong className="cw-metric__value">{fmtInt(scan.parsingPerSec)}</strong>
            <span className="cw-metric__unit">PPS</span>
          </div>
          <div className="cw-metric__spark">
            <CpuSparkline values={ppsHistory} />
          </div>
        </article>
      </div>

      <div className="cw-session__completion">
        <div className="cw-session__completion-head">
          <span className="muted">Completion</span>
          <span className="mono">{progress.toFixed(0)}%</span>
        </div>
        <div className="cw-session__completion-track">
          <div className="cw-session__completion-fill" style={{ width: `${progress}%` }} />
        </div>
      </div>

      <div className="cw-session__stats">
        <article className="cw-stat cw-stat--green">
          <span className="cw-stat__ico" aria-hidden>
            <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M20 6 9 17l-5-5" />
            </svg>
          </span>
          <div>
            <strong>{fmtInt(scan.validHosts)}</strong>
            <span>Valid Hosts</span>
          </div>
        </article>
        <article className="cw-stat cw-stat--red">
          <span className="cw-stat__ico" aria-hidden>
            <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M18 6 6 18M6 6l12 12" />
            </svg>
          </span>
          <div>
            <strong>{fmtInt(scan.invalidHosts)}</strong>
            <span>Invalid Hosts</span>
          </div>
        </article>
        <article className="cw-stat cw-stat--blue">
          <span className="cw-stat__ico" aria-hidden>
            <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="11" cy="11" r="7" />
              <path d="m21 21-4.3-4.3" />
            </svg>
          </span>
          <div>
            <strong>{fmtInt(scan.hitsFound)}</strong>
            <span>Hits Found</span>
          </div>
        </article>
        <article className="cw-stat cw-stat--gold">
          <span className="cw-stat__ico" aria-hidden>
            <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d="M9 14h6l-1 7h-4l-1-7Z" />
              <path d="M7 4h10v6a5 5 0 0 1-10 0V4Z" />
              <path d="M5 5H3a2 2 0 0 0 2 4M19 5h2a2 2 0 0 1-2 4" />
            </svg>
          </span>
          <div>
            <strong>{fmtInt(scan.validHits)}</strong>
            <span>Valid Hits</span>
          </div>
        </article>
      </div>

      <footer className="cw-session__foot">
        <button type="button" className="btn-glass" onClick={onViewStats}>
          View Stats
        </button>
        <button type="button" className="btn-cw-stop" onClick={onStop} disabled={scan.status !== 'running'}>
          Stop Crack
        </button>
      </footer>
    </section>
  )
}
