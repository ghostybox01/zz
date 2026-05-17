import { useEffect, useRef, useState } from 'react'
import { warc, type WarcStatus } from '../lib/reconApi'

const POLL_MS = 3000

export function WarcPanel() {
  const [status, setStatus] = useState<WarcStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [maxDomains, setMaxDomains] = useState(10000)
  const [extractWorkers, setExtractWorkers] = useState(50)
  const [testWorkers, setTestWorkers] = useState(25)
  const [runOn, setRunOn] = useState<string>('controller')
  const [hosts, setHosts] = useState<string[]>(['controller'])
  const pollTimer = useRef<number | null>(null)

  async function refresh() {
    try {
      setStatus(await warc.status())
    } catch (e) {
      setError((e as Error).message)
    }
  }

  useEffect(() => {
    const refreshHosts = async () => {
      try {
        const r = await warc.hosts()
        // Backend now returns `['controller', ...warc-tagged workers]` already —
        // the earlier version of this component prepended a second 'controller',
        // which is why the dropdown showed 'controller (this VPS)' twice.
        const fromServer = Array.isArray(r?.hosts) && r.hosts.length > 0
          ? r.hosts
          : ['controller']
        setHosts((prev) =>
          prev.length === fromServer.length && prev.every((h, i) => h === fromServer[i])
            ? prev
            : fromServer,
        )
      } catch {
        // Keep whatever we already had — a transient failure shouldn't
        // strand the dropdown back to controller-only.
      }
    }
    void refresh()
    void refreshHosts()
    pollTimer.current = window.setInterval(() => {
      void refresh()
      void refreshHosts()
    }, POLL_MS)
    return () => {
      if (pollTimer.current) window.clearInterval(pollTimer.current)
    }
  }, [])

  async function onStart() {
    setBusy(true); setError(null)
    try {
      await warc.start({
        run_on: runOn,
        max_domains: maxDomains,
        extract_workers: extractWorkers,
        test_workers: testWorkers,
      })
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function onStop() {
    setBusy(true); setError(null)
    try {
      await warc.stop()
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function onExport() {
    setBusy(true); setError(null)
    try {
      const r = await warc.exportToR2()
      setError(`Exported to R2: ${r.r2_key}`)
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const running = status?.running === true
  const finishedAt = status?.finished_at
  const r2Key = status?.r2_key
  const r2Error = status?.r2_error
  const exitCode = status?.last_exit_code

  return (
    <section className="warc-panel">
      <header className="card-block__head card-block__head--row">
        <div>
          <h2>WARC harvest</h2>
          <p className="card-block__lede card-block__lede--short">
            <code className="inline-code">warc.go</code> runs on the controller, scans Common Crawl
            archives, tests liveness, writes results to a temp file, then ships them to R2 and
            deletes the local copy — controller disk stays clean.
          </p>
        </div>
        <div className="warc-head-actions">
          <div className="warc-mode">
            <span className={`pill ${running ? 'pill--ok' : 'pill--muted'}`}>
              {running ? 'Running' : 'Idle'}
            </span>
            {running && (
              <span className="pill pill--muted" style={{ fontSize: '0.72rem' }}>
                Running on: {status?.run_on ?? 'controller'}
              </span>
            )}
            {r2Key && (
              <span className="pill pill--ok" title={r2Key} style={{ fontSize: '0.72rem' }}>
                ↑ R2
              </span>
            )}
          </div>
          <div className="warc-controls">
            {running ? (
              <button type="button" className="btn-danger-outline" onClick={() => void onStop()} disabled={busy}>
                ■ Stop harvest
              </button>
            ) : (
              <button type="button" className="btn-glass" onClick={() => void onStart()} disabled={busy}>
                ▶ Start harvest
              </button>
            )}
            <button
              type="button"
              className="btn-glass"
              onClick={() => void onExport()}
              disabled={busy || !status?.domains_found}
              title={status?.domains_found ? `Push ${status.domains_found} domains to R2 now` : 'Nothing harvested yet'}
            >
              Export to R2 ({status?.domains_found ?? 0})
            </button>
          </div>
        </div>
      </header>

      {!running && (
        <div className="kv kv--form" style={{ marginTop: '1rem' }}>
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-runon">Run on</label>
            <select
              id="warc-runon"
              className="kv__input"
              value={runOn}
              onChange={(e) => setRunOn(e.target.value)}
            >
              {hosts.map((h) => <option key={h} value={h}>{h === 'controller' ? 'controller (this VPS)' : h}</option>)}
            </select>
          </div>
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-max">Max live domains</label>
            <input
              id="warc-max"
              type="number"
              min={100}
              className="kv__input"
              value={maxDomains}
              onChange={(e) => setMaxDomains(Math.max(100, Number(e.target.value) || 10000))}
            />
          </div>
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-ext">Extract workers</label>
            <input
              id="warc-ext"
              type="number"
              min={1}
              max={500}
              className="kv__input"
              value={extractWorkers}
              onChange={(e) => setExtractWorkers(Math.max(1, Number(e.target.value) || 50))}
            />
          </div>
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-test">Liveness workers</label>
            <input
              id="warc-test"
              type="number"
              min={1}
              max={500}
              className="kv__input"
              value={testWorkers}
              onChange={(e) => setTestWorkers(Math.max(1, Number(e.target.value) || 25))}
            />
          </div>
        </div>
      )}

      {status?.running === true
        && status?.domains_found === 0
        && status?.started_at
        && (Date.now() - new Date(status.started_at).getTime() < 60000) && (
        <div
          className="muted"
          style={{
            marginTop: '1rem',
            padding: '.55rem .75rem',
            background: 'rgba(255,255,255,.03)',
            borderRadius: '.4rem',
            fontSize: '.78rem',
            fontStyle: 'italic',
          }}
        >
          Initialising — fetching CC-MAIN snapshots (~30s)…
        </div>
      )}

      <div style={{ marginTop: '1rem', display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '.75rem' }}>
        <Stat label="Domains found" value={status?.domains_found ?? 0} />
        <Stat label="Target" value={status?.max_domains ?? '—'} />
        <Stat
          label="Started"
          value={status?.started_at ? new Date(status.started_at).toLocaleTimeString() : '—'}
        />
        <Stat
          label="Status"
          value={
            running
              ? 'Harvesting'
              : finishedAt
                ? exitCode === 0
                  ? 'Completed'
                  : `Exited (${exitCode})`
                : 'Idle'
          }
        />
      </div>

      {(error || r2Error) && (
        <p className={`settings-hint ${r2Error ? 'tg-hint--err' : ''}`} style={{ marginTop: '.75rem' }}>
          {error || r2Error}
        </p>
      )}

      {status?.log_tail && status.log_tail.length > 0 && (
        <details style={{ marginTop: '1rem' }}>
          <summary className="muted" style={{ cursor: 'pointer', fontSize: '.8rem' }}>
            Last {status.log_tail.length} log lines
          </summary>
          <pre
            className="mono"
            style={{
              fontSize: '.72rem',
              maxHeight: '14rem',
              overflowY: 'auto',
              background: 'rgba(0,0,0,.35)',
              padding: '.6rem .8rem',
              borderRadius: '.4rem',
              marginTop: '.4rem',
            }}
          >
            {status.log_tail.join('\n')}
          </pre>
        </details>
      )}
    </section>
  )
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div style={{ background: 'rgba(255,255,255,.03)', borderRadius: '.4rem', padding: '.55rem .7rem' }}>
      <div className="muted" style={{ fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.06em' }}>{label}</div>
      <div className="mono" style={{ fontSize: '1rem', marginTop: '.15rem' }}>{value}</div>
    </div>
  )
}
