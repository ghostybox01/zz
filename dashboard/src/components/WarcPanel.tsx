type Props = {
  liveEnabled: boolean
  scanning: boolean
  onToggleScan: () => void
  onExportToList: () => void
  exportCount?: number
}

export function WarcPanel({ liveEnabled, scanning, onToggleScan, onExportToList, exportCount = 0 }: Props) {
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
              {liveEnabled ? 'Live ingest' : 'Offline'}
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

      <p className="muted-callout" style={{ marginTop: '1.5rem' }}>
        Connect the <code className="inline-code">warc.go</code> process to your backend and enable Live ingest above.
        Discovered hostnames will appear in the Hits tab and can be exported to a target list.
      </p>
    </section>
  )
}
