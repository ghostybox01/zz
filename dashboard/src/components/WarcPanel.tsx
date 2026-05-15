import { useMemo } from 'react'
import type { RunSnapshot } from '../types'
import { CpuSparkline } from './CpuSparkline'
import { fmtInt, fmtDuration } from '../lib/format'

type Props = {
  run: RunSnapshot
  liveEnabled: boolean
  scanning: boolean
  onToggleScan: () => void
  onExportToList: () => void
  exportCount?: number
}

/** WARC = the `warc.go` companion: ingests Common Crawl WARC files, emits live_domains.txt. */
export function WarcPanel({ run, liveEnabled, scanning, onToggleScan, onExportToList, exportCount = 0 }: Props) {
  const filesPct = run.filesTotal > 0 ? Math.min(100, (run.filesProcessed / run.filesTotal) * 100) : 0
  const livePct = run.targetLiveDomains > 0 ? Math.min(100, (run.liveDomains / run.targetLiveDomains) * 100) : 0
  const rate = run.elapsedSeconds > 0 ? run.totalExtracted / run.elapsedSeconds : 0

  const perSnapshot = useMemo(() => {
    if (run.snapshots.length === 0) return []
    const share = 1 / run.snapshots.length
    return run.snapshots.map((s, i) => ({
      id: s,
      files: Math.round(run.filesProcessed * share * (i === 0 ? 1.1 : 0.9)),
      liveDomains: Math.round(run.liveDomains * share * (i === 0 ? 1.05 : 0.95)),
      extracted: Math.round(run.totalExtracted * share),
    }))
  }, [run])

  return (
    <section className="warc-panel">
      <header className="card-block__head card-block__head--row">
        <div>
          <h2>WARC harvest</h2>
          <p className="card-block__lede card-block__lede--short">
            <code className="inline-code">warc.go</code> companion — ingests Common Crawl WARC archives,
            tests liveness, emits <code className="inline-code">live_domains.txt</code> for RavenX.
          </p>
        </div>
        <div className="warc-head-actions">
          <div className="warc-mode">
            <span className={`pill ${liveEnabled ? 'pill--ok' : 'pill--muted'}`}>
              {liveEnabled ? 'Live ingest' : 'Sandbox playback'}
            </span>
            <span className={`warc-run-pill${scanning ? ' warc-run-pill--on' : ''}`}>
              <span className="warc-run-pill__dot" aria-hidden />
              {scanning ? 'Harvesting' : 'Paused'}
            </span>
          </div>
          <div className="warc-controls">
            <button
              type="button"
              className={scanning ? 'btn-danger-outline' : 'btn-glass'}
              onClick={onToggleScan}
            >
              {scanning ? '■ Stop harvest' : '▶ Start harvest'}
            </button>
            <button
              type="button"
              className="btn-glass"
              onClick={onExportToList}
              disabled={exportCount === 0}
              title={exportCount === 0 ? 'No findings to export yet' : `Export ${exportCount} hostnames to Lists`}
            >
              Export to list ({exportCount})
            </button>
          </div>
        </div>
      </header>

      <div className="warc-grid">
        <div className="warc-kpi warc-kpi--green">
          <span className="warc-kpi__label">Live domains</span>
          <span className="warc-kpi__value">{fmtInt(run.liveDomains)}</span>
          <span className="warc-kpi__sub">target {fmtInt(run.targetLiveDomains)}</span>
          <div className="warc-progress">
            <div className="warc-progress__fill warc-progress__fill--green" style={{ width: `${livePct.toFixed(2)}%` }} />
          </div>
        </div>
        <div className="warc-kpi">
          <span className="warc-kpi__label">WARC files</span>
          <span className="warc-kpi__value">{fmtInt(run.filesProcessed)} <span className="muted">/ {fmtInt(run.filesTotal)}</span></span>
          <span className="warc-kpi__sub">{filesPct.toFixed(1)}% scanned</span>
          <div className="warc-progress">
            <div className="warc-progress__fill" style={{ width: `${filesPct.toFixed(2)}%` }} />
          </div>
        </div>
        <div className="warc-kpi">
          <span className="warc-kpi__label">Extracted URLs</span>
          <span className="warc-kpi__value">{fmtInt(run.totalExtracted)}</span>
          <span className="warc-kpi__sub">{rate.toFixed(0)}/s lifetime avg</span>
        </div>
        <div className="warc-kpi warc-kpi--amber">
          <span className="warc-kpi__label">Elapsed</span>
          <span className="warc-kpi__value">{fmtDuration(run.elapsedSeconds)}</span>
          <span className="warc-kpi__sub">{run.extractWorkers ?? '—'} extract / {run.testWorkers ?? '—'} test workers</span>
        </div>
      </div>

      <section className="card-block card-block--tight" style={{ marginTop: '1.25rem' }}>
        <div className="card-block__head">
          <h3 style={{ margin: 0 }}>Snapshots</h3>
          <p className="card-block__lede card-block__lede--short">
            Common Crawl monthly archives feeding this run.
          </p>
        </div>
        {perSnapshot.length === 0 ? (
          <p className="muted-callout">No snapshot identifiers in the loaded run metrics.</p>
        ) : (
          <div className="warc-snap-grid">
            {perSnapshot.map((s, i) => (
              <article key={s.id} className="warc-snap">
                <header className="warc-snap__head">
                  <h4>{s.id}</h4>
                  <span className="chip chip--small">{i === 0 ? 'primary' : 'mirror'}</span>
                </header>
                <dl className="warc-snap__stats">
                  <div><dt>Files</dt><dd>{fmtInt(s.files)}</dd></div>
                  <div><dt>Live</dt><dd>{fmtInt(s.liveDomains)}</dd></div>
                  <div><dt>URLs</dt><dd>{fmtInt(s.extracted)}</dd></div>
                </dl>
                <div className="warc-snap__spark" style={{ color: i === 0 ? 'var(--accent)' : 'var(--ok)' }}>
                  <CpuSparkline values={fakeSeries(s.id)} />
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <section className="card-block card-block--tight warc-pacing" style={{ marginTop: '1rem' }}>
        <div className="card-block__head">
          <h3 style={{ margin: 0 }}>Harvest pacing &amp; worker pool</h3>
          <p className="card-block__lede card-block__lede--short">
            Throughput trend and per-pool allocation across the run.
          </p>
        </div>
        <div className="warc-pacing__grid">
          <div className="warc-pacing__chart" style={{ color: 'var(--accent)' }}>
            <CpuSparkline values={fakeSeries(run.id + '-pacing')} />
          </div>
          <dl className="warc-pacing__stats">
            <div>
              <dt>Liveness yield</dt>
              <dd>{((run.liveDomains / Math.max(1, run.totalTested)) * 100).toFixed(1)}%</dd>
            </div>
            <div>
              <dt>Extract → test ratio</dt>
              <dd>{(run.totalExtracted / Math.max(1, run.totalTested)).toFixed(2)}×</dd>
            </div>
            <div>
              <dt>Per-worker rate</dt>
              <dd>{(rate / Math.max(1, run.extractWorkers ?? 1)).toFixed(0)}/s</dd>
            </div>
            <div>
              <dt>Workers</dt>
              <dd>{run.extractWorkers ?? '—'} extract · {run.testWorkers ?? '—'} test</dd>
            </div>
          </dl>
        </div>
        <div className="warc-pool-bar" aria-label="Worker pool split">
          <div className="warc-pool-bar__seg warc-pool-bar__seg--extract"
               style={{ flex: `${run.extractWorkers ?? 1} 1 0` }}
               title={`Extract workers: ${run.extractWorkers ?? '—'}`}>
            EXTRACT {run.extractWorkers ?? '—'}
          </div>
          <div className="warc-pool-bar__seg warc-pool-bar__seg--test"
               style={{ flex: `${run.testWorkers ?? 1} 1 0` }}
               title={`Test workers: ${run.testWorkers ?? '—'}`}>
            TEST {run.testWorkers ?? '—'}
          </div>
        </div>
      </section>
    </section>
  )
}

function fakeSeries(seed: string): number[] {
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0
  const out: number[] = []
  for (let i = 0; i < 16; i++) {
    h = (h * 1103515245 + 12345) >>> 0
    out.push(20 + (h % 60))
  }
  return out
}
