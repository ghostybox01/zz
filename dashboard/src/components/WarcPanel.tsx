import { useState } from 'react'
import type { WarcStatus } from '../lib/reconApi'
import type { WarcReachability } from '../hooks/useWarcStatus'

type Props = {
  status: WarcStatus | null
  reachability: WarcReachability
  lastError: string | null
  busy: boolean
  onStart: (maxDomains: number) => Promise<void> | void
  onStop: () => Promise<void> | void
  onExportToList: () => Promise<void> | void
}

type ConnectivityPill = {
  className: string
  label: string
  title: string
}

function connectivityPill(reachability: WarcReachability, binaryPresent: boolean): ConnectivityPill {
  if (reachability === 'unreachable') {
    return {
      className: 'pill pill--danger',
      label: 'Backend offline',
      title: '/api/warc/status is unreachable — is reconx-dashboard running on the controller?',
    }
  }
  if (reachability === 'unknown') {
    return {
      className: 'pill pill--muted',
      label: 'Connecting…',
      title: 'Polling /api/warc/status for the first time.',
    }
  }
  if (!binaryPresent) {
    return {
      className: 'pill pill--warn',
      label: 'Binary missing',
      title: 'Backend is up but warc_live_checker is not built. Run the deploy script or build it manually.',
    }
  }
  return {
    className: 'pill pill--ok',
    label: 'Connected',
    title: 'Backend reachable and warc_live_checker is installed.',
  }
}

function fmt(n: number | undefined): string {
  if (!n || !Number.isFinite(n)) return '0'
  return n.toLocaleString()
}

function fmtBytes(b: number | undefined): string {
  if (!b) return '0 B'
  if (b >= 1024 ** 3) return `${(b / 1024 ** 3).toFixed(2)} GB`
  if (b >= 1024 ** 2) return `${(b / 1024 ** 2).toFixed(1)} MB`
  if (b >= 1024) return `${(b / 1024).toFixed(1)} KB`
  return `${b} B`
}

export function WarcPanel({ status, reachability, lastError, busy, onStart, onStop, onExportToList }: Props) {
  const [maxDomains, setMaxDomains] = useState(10_000)

  const running = !!status?.running
  const binaryPresent = !!status?.binary_present
  const pill = connectivityPill(reachability, binaryPresent)
  const live = status?.live ?? 0
  const tested = status?.tested ?? 0
  const extracted = status?.extracted ?? 0
  const filesProcessed = status?.files_processed ?? 0
  const filesTotal = status?.files_total ?? 0
  const target = status?.max_domains ?? 0
  const outputBytes = status?.output_bytes ?? 0
  const exitCode = status?.exit_code
  const lastLine = status?.last_line ?? ''

  const handleToggle = () => {
    if (running) void onStop()
    else void onStart(maxDomains)
  }

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
            <span className={pill.className} title={pill.title}>
              {pill.label}
            </span>
            <span className={`warc-run-pill${running ? ' warc-run-pill--on' : ''}`}>
              <span className="warc-run-pill__dot" aria-hidden />
              {running ? 'Harvesting' : exitCode !== null && exitCode !== undefined ? `Stopped (exit ${exitCode})` : 'Paused'}
            </span>
          </div>
          <div className="warc-controls">
            <label className="warc-controls__field" title="Target live-domain count before the harvester self-terminates">
              <span className="warc-controls__label">Target</span>
              <input
                type="number"
                min={100}
                max={10_000_000}
                step={1000}
                value={maxDomains}
                disabled={running || busy}
                onChange={(e) => setMaxDomains(Math.max(100, Math.min(10_000_000, Number(e.target.value) || 0)))}
                className="warc-controls__input"
              />
            </label>
            <button
              type="button"
              className={running ? 'btn-danger-outline' : 'btn-glass'}
              onClick={handleToggle}
              disabled={busy || (!running && !binaryPresent)}
              title={!binaryPresent && !running ? 'warc binary not built — run `go build -o warc_live_checker warc.go` on the backend host' : undefined}
            >
              {running ? '■ Stop harvest' : '▶ Start harvest'}
            </button>
            <button
              type="button"
              className="btn-glass"
              onClick={() => void onExportToList()}
              disabled={busy || outputBytes === 0}
              title={outputBytes === 0 ? 'No live_domains.txt yet — start a harvest first' : `Export ${fmtBytes(outputBytes)} of hostnames to Lists`}
            >
              Export to list
            </button>
          </div>
        </div>
      </header>

      <div className="warc-metrics">
        <div className="warc-metric">
          <span className="warc-metric__label">Live</span>
          <span className="warc-metric__value">{fmt(live)}{target > 0 ? <span className="warc-metric__sub"> / {fmt(target)}</span> : null}</span>
        </div>
        <div className="warc-metric">
          <span className="warc-metric__label">Tested</span>
          <span className="warc-metric__value">{fmt(tested)}</span>
        </div>
        <div className="warc-metric">
          <span className="warc-metric__label">Extracted</span>
          <span className="warc-metric__value">{fmt(extracted)}</span>
        </div>
        <div className="warc-metric">
          <span className="warc-metric__label">Files</span>
          <span className="warc-metric__value">{fmt(filesProcessed)}{filesTotal > 0 ? <span className="warc-metric__sub"> / {fmt(filesTotal)}</span> : null}</span>
        </div>
        <div className="warc-metric">
          <span className="warc-metric__label">live_domains.txt</span>
          <span className="warc-metric__value">{fmtBytes(outputBytes)}</span>
        </div>
      </div>

      {reachability === 'unreachable' && (
        <p className="muted-callout" style={{ marginTop: '1rem' }}>
          The dashboard can't reach <code className="inline-code">/api/warc/status</code>. On the controller
          box run <code className="inline-code">systemctl status reconx-dashboard</code> to make sure the Flask
          service is up, then reload nginx if you've changed its config.
        </p>
      )}

      {reachability === 'ok' && !binaryPresent && !running && (
        <p className="muted-callout" style={{ marginTop: '1rem' }}>
          The <code className="inline-code">warc_live_checker</code> binary isn't on the controller yet. From
          the install dir run <code className="inline-code">./install-controller.sh</code> again, or build it
          manually with <code className="inline-code">go build -o warc_live_checker warc.go</code>.
        </p>
      )}

      {lastLine && (
        <p className="warc-last-line" style={{ marginTop: '1rem', fontFamily: 'ui-monospace, monospace', fontSize: '0.85em', opacity: 0.75 }}>
          {lastLine}
        </p>
      )}

      {lastError && (
        <p className="lists-upload__error lists-upload__error--read" style={{ marginTop: '1rem' }}>
          ✗ {lastError}
        </p>
      )}
    </section>
  )
}
