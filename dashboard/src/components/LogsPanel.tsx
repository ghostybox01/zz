import { useEffect, useRef, useState } from 'react'
import { logs } from '../lib/reconApi'
import { labelForHost } from '../lib/labelForHost'
import type { VpsNode } from '../types'

const POLL_MS = 5000
const DEFAULT_TAIL = 200

type Source = 'controller' | string // 'controller' or a worker IP

type Props = {
  /** Live fleet roster — resolves worker IPs in the Source dropdown to their
   * operator-facing labels. Optional so the panel still renders in isolation;
   * when absent, IPs are shown raw. */
  fleet?: readonly VpsNode[]
}

export function LogsPanel({ fleet = [] }: Props = {}) {
  const [workers, setWorkers] = useState<string[]>([])
  const [source, setSource] = useState<Source>('controller')
  const [lines, setLines] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)
  const [paused, setPaused] = useState(false)
  const [tail, setTail] = useState<number>(DEFAULT_TAIL)
  const [lastFetched, setLastFetched] = useState<Date | null>(null)
  const pollTimer = useRef<number | null>(null)
  const preRef = useRef<HTMLPreElement | null>(null)

  // One-time fetch of the worker list so the dropdown can include them.
  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const r = await logs.workersList()
        if (!cancelled) setWorkers(r.ips ?? [])
      } catch {
        if (!cancelled) setWorkers([])
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  async function refresh() {
    try {
      const r =
        source === 'controller'
          ? await logs.controller(tail)
          : await logs.worker(source, tail)
      setLines(r.lines ?? [])
      setError((r as { error?: string }).error ?? null)
      setLastFetched(new Date())
    } catch (e) {
      setError((e as Error).message)
    }
  }

  // Re-fetch immediately whenever source or tail changes, and (re)arm the
  // poll loop. The cleanup nukes the timer so we don't leak intervals when
  // the panel unmounts or the dependency tuple changes.
  useEffect(() => {
    void refresh()
    if (paused) {
      if (pollTimer.current) {
        window.clearInterval(pollTimer.current)
        pollTimer.current = null
      }
      return
    }
    pollTimer.current = window.setInterval(() => void refresh(), POLL_MS)
    return () => {
      if (pollTimer.current) {
        window.clearInterval(pollTimer.current)
        pollTimer.current = null
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source, tail, paused])

  // Auto-scroll the pre to the bottom whenever lines change, so the operator
  // sees the freshest tail without manual scrolling.
  useEffect(() => {
    const el = preRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [lines])

  const sourceLabel =
    source === 'controller'
      ? 'Controller (reconx-dashboard)'
      : `Worker ${labelForHost(source, fleet)}`

  return (
    <section className="card-block">
      <header className="card-block__head card-block__head--row">
        <div>
          <h2>Logs</h2>
          <p className="card-block__lede card-block__lede--short">
            Tail the controller journal or any worker&apos;s <code className="inline-code">output.log</code>{' '}
            without ssh-ing in. Polls every 5s.
          </p>
        </div>
        <div className="warc-head-actions">
          <div className="warc-mode">
            <span className={`pill ${paused ? 'pill--muted' : 'pill--ok'}`}>
              {paused ? 'Paused' : 'Live'}
            </span>
            {lastFetched && (
              <span className="pill pill--muted" style={{ fontSize: '0.72rem' }}>
                {lastFetched.toLocaleTimeString()}
              </span>
            )}
          </div>
          <div className="warc-controls">
            <button
              type="button"
              className="btn-glass"
              onClick={() => setPaused((p) => !p)}
              title={paused ? 'Resume 5s auto-refresh' : 'Pause auto-refresh'}
            >
              {paused ? '▶ Resume' : '❚❚ Pause'}
            </button>
            <button
              type="button"
              className="btn-glass"
              onClick={() => void refresh()}
              title="Fetch now"
            >
              ↻ Refresh
            </button>
          </div>
        </div>
      </header>

      <div className="kv kv--form" style={{ marginTop: '1rem' }}>
        <div className="kv__row">
          <label className="kv__label" htmlFor="logs-source">Source</label>
          <select
            id="logs-source"
            className="kv__input"
            value={source}
            onChange={(e) => setSource(e.target.value as Source)}
          >
            <option value="controller">Controller (reconx-dashboard)</option>
            {workers.map((ip) => (
              <option key={ip} value={ip}>
                Worker {labelForHost(ip, fleet)}
              </option>
            ))}
          </select>
        </div>
        <div className="kv__row">
          <label className="kv__label" htmlFor="logs-tail">Tail lines</label>
          <input
            id="logs-tail"
            type="number"
            min={10}
            max={500}
            step={10}
            className="kv__input"
            value={tail}
            onChange={(e) => setTail(Math.max(10, Math.min(500, Number(e.target.value) || DEFAULT_TAIL)))}
          />
        </div>
      </div>

      {error && (
        <p className="settings-hint tg-hint--err" style={{ marginTop: '.75rem' }}>
          {error}
        </p>
      )}

      <div style={{ marginTop: '1rem' }}>
        <div
          className="muted"
          style={{
            fontSize: '.7rem',
            textTransform: 'uppercase',
            letterSpacing: '.06em',
            marginBottom: '.35rem',
          }}
        >
          {sourceLabel} · {lines.length} lines
        </div>
        <pre
          ref={preRef}
          className="mono"
          style={{
            fontSize: '.72rem',
            maxHeight: '32rem',
            minHeight: '14rem',
            overflowY: 'auto',
            background: 'rgba(0,0,0,.45)',
            padding: '.7rem .9rem',
            borderRadius: '.4rem',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-all',
          }}
        >
          {lines.length > 0 ? lines.join('\n') : '(no log lines yet)'}
        </pre>
      </div>
    </section>
  )
}
