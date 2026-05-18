import { useEffect, useRef, useState } from 'react'
import { warc, r2, type WarcStatus, type R2Config, type R2Object } from '../lib/reconApi'

const POLL_MS = 3000

/** Optional toast plumbing. WarcPanel renders standalone in any context, but
 * when the host page passes a notifier we surface Stop/Export progress and
 * completion through it instead of repurposing the inline `error` slot. */
export type WarcPanelToast = (title: string, message: string, kind: 'info' | 'error') => void

type Props = {
  notify?: WarcPanelToast
}

export function WarcPanel({ notify }: Props = {}) {
  const [status, setStatus] = useState<WarcStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [maxDomains, setMaxDomains] = useState(10000)
  const [extractWorkers, setExtractWorkers] = useState(50)
  const [testWorkers, setTestWorkers] = useState(25)
  const [snapshots, setSnapshots] = useState(0)
  const [runOn, setRunOn] = useState<string>('controller')
  const [hosts, setHosts] = useState<string[]>(['controller'])
  // Producer source toggles — default mirrors the legacy CC-only flow.
  // Either may be true; both true runs them concurrently into the same
  // dedupe map on the worker.
  const [sourceCC, setSourceCC] = useState(true)
  const [sourceCrtSh, setSourceCrtSh] = useState(false)
  // crt.sh pivot inputs — comma-separated, free-text. Only meaningful
  // when sourceCrtSh is true; the form gates them visually.
  const [crtTld, setCrtTld] = useState('')
  const [crtDomain, setCrtDomain] = useState('')
  // Opt-in apex filter (drops FQDNs equal to their own eTLD+1).
  const [subdomainOnly, setSubdomainOnly] = useState(false)
  // R2 health, refreshed alongside warc/status. Surfaces the same pill
  // R2Settings shows so the operator can see at the export site whether
  // a click on "Export to R2" will actually land.
  const [r2State, setR2State] = useState<R2Config['state']>('unknown')
  const [r2LastError, setR2LastError] = useState<string | null>(null)
  // R2 exports list — populated on demand (mount + after each export +
  // after each delete). Lets the operator see what landed and prune
  // duplicates without leaving the cockpit.
  const [r2Objects, setR2Objects] = useState<R2Object[]>([])
  const [r2Listing, setR2Listing] = useState(false)
  const pollTimer = useRef<number | null>(null)

  async function refresh() {
    try {
      setStatus(await warc.status())
    } catch (e) {
      setError((e as Error).message)
    }
  }

  async function refreshR2Health() {
    try {
      const c = await r2.getConfig()
      setR2State(c.state ?? (c.configured ? 'unknown' : 'misconfigured'))
      setR2LastError(c.last_error ?? null)
    } catch {
      // best-effort — leave the pill at its prior state
    }
  }

  async function refreshR2Objects() {
    setR2Listing(true)
    try {
      const r = await r2.listObjects('warc/', 200)
      if (r.ok) setR2Objects(r.objects ?? [])
    } catch {
      // ignore — listing is non-critical
    } finally {
      setR2Listing(false)
    }
  }

  async function onDeleteR2(key: string) {
    if (!window.confirm(`Delete ${key} from R2? This cannot be undone.`)) return
    notify?.('Deleting R2 object', key, 'info')
    const res = await r2.deleteObject(key)
    if (res.ok) {
      notify?.('R2 delete complete', key, 'info')
      setR2Objects((prev) => prev.filter((o) => o.key !== key))
      // If the dedup pointer was this object, status will re-sync on next poll.
      await refresh()
    } else {
      notify?.('R2 delete failed', res.error ?? 'unknown error', 'error')
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
    void refreshR2Health()
    void refreshR2Objects()
    pollTimer.current = window.setInterval(() => {
      void refresh()
      void refreshHosts()
      // R2 health changes slowly — poll at 1/4 the WARC cadence so we stay
      // responsive to operator config changes without burning round-trips.
      if ((Date.now() / POLL_MS) % 4 < 1) void refreshR2Health()
    }, POLL_MS)
    return () => {
      if (pollTimer.current) window.clearInterval(pollTimer.current)
    }
  }, [])

  async function onStart() {
    setBusy(true); setError(null)
    // Build the source list from the checkboxes. If the operator
    // unchecked both, default to ['cc'] so we never POST a no-producer
    // request — the backend would reject it and the UX would be
    // confusing.
    const sources: string[] = []
    if (sourceCC) sources.push('cc')
    if (sourceCrtSh) sources.push('crtsh')
    if (sources.length === 0) sources.push('cc')

    // Local guard: require at least one pivot if crt.sh is checked, so
    // the operator sees a clear inline message instead of a 400 from
    // the backend.
    if (sourceCrtSh && !crtTld.trim() && !crtDomain.trim()) {
      setError('crt.sh source selected — provide at least one TLD or domain to pivot on.')
      setBusy(false)
      return
    }

    try {
      await warc.start({
        run_on: runOn,
        max_domains: maxDomains,
        extract_workers: extractWorkers,
        test_workers: testWorkers,
        snapshots,
        source: sources,
        crt_tld: sourceCrtSh ? crtTld.trim() : undefined,
        crt_domain: sourceCrtSh ? crtDomain.trim() : undefined,
        subdomain_only: subdomainOnly,
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
    notify?.('Stopping WARC harvest', 'SIGTERM dispatched — status will update on next monitor cycle.', 'info')
    try {
      await warc.stop()
      await refresh()
    } catch (e) {
      const msg = (e as Error).message
      setError(msg)
      notify?.('Stop request failed', msg, 'error')
    } finally {
      setBusy(false)
    }
  }

  async function onExport() {
    setBusy(true); setError(null)
    const count = status?.domains_found ?? 0
    notify?.('Uploading to R2', `Shipping ${count.toLocaleString()} domains to Cloudflare R2…`, 'info')
    try {
      const r = await warc.exportToR2() as { r2_key?: string; noop?: boolean; message?: string }
      if (r.noop) {
        notify?.('No upload needed', `${r.message ?? 'content unchanged'} — kept ${r.r2_key}`, 'info')
      } else {
        notify?.('R2 upload complete', `Saved as ${r.r2_key}`, 'info')
      }
      await refresh()
      void refreshR2Objects()
    } catch (e) {
      const msg = (e as Error).message
      setError(msg)
      notify?.('R2 upload failed', msg, 'error')
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
            {(() => {
              const cls =
                r2State === 'connected' ? 'pill--ok'
                : r2State === 'misconfigured' ? 'pill--warn'
                : r2State === 'unreachable' ? 'pill--danger'
                : 'pill--muted'
              const label =
                r2State === 'connected' ? '● R2 connected'
                : r2State === 'misconfigured' ? '● R2 not configured'
                : r2State === 'unreachable' ? '● R2 unreachable'
                : '● R2 …'
              return (
                <span
                  className={`pill ${cls}`}
                  style={{ fontSize: '0.72rem' }}
                  title={r2LastError ?? (r2State === 'connected' ? 'head_bucket ok on last probe' : 'See R2 Settings')}
                >
                  {label}
                </span>
              )
            })()}
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
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-snapshots">CC-MAIN snapshots</label>
            <input
              id="warc-snapshots"
              type="number"
              min={0}
              max={20}
              className="kv__input"
              value={snapshots}
              onChange={(e) => setSnapshots(Math.max(0, Math.min(20, Number(e.target.value) || 0)))}
              title="0 = auto (1 snapshot for <500k domains, 2 for <1M, 3 for >1M). Set 1–20 to override."
            />
          </div>
          {/* ── Producer source selection ────────────────────────────
              Two independent checkboxes (not a radio): the operator
              may enable CC, crt.sh, or both in the same harvest. */}
          <div className="kv__row">
            <span className="kv__label">Sources</span>
            <div style={{ display: 'flex', gap: '1rem', alignItems: 'center', flexWrap: 'wrap' }}>
              <label style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={sourceCC}
                  onChange={(e) => setSourceCC(e.target.checked)}
                />
                <span>Common Crawl</span>
              </label>
              <label style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={sourceCrtSh}
                  onChange={(e) => setSourceCrtSh(e.target.checked)}
                />
                <span>crt.sh</span>
              </label>
            </div>
          </div>
          {sourceCrtSh && (
            <>
              <div className="kv__row">
                <label className="kv__label" htmlFor="warc-crt-tld">crt.sh TLDs</label>
                <input
                  id="warc-crt-tld"
                  type="text"
                  className="kv__input"
                  placeholder="com,net,io"
                  value={crtTld}
                  onChange={(e) => setCrtTld(e.target.value)}
                  title="Comma-separated TLD list, e.g. com,net,io"
                />
              </div>
              <div className="kv__row">
                <label className="kv__label" htmlFor="warc-crt-domain">crt.sh domains</label>
                <input
                  id="warc-crt-domain"
                  type="text"
                  className="kv__input"
                  placeholder="example.com,foo.io"
                  value={crtDomain}
                  onChange={(e) => setCrtDomain(e.target.value)}
                  title="Comma-separated registered-domain list (e.g. example.com,foo.io)"
                />
              </div>
            </>
          )}
          <div className="kv__row">
            <span className="kv__label">Filter</span>
            <label style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={subdomainOnly}
                onChange={(e) => setSubdomainOnly(e.target.checked)}
              />
              <span>Subdomain only (drop apex / eTLD+1 entries)</span>
            </label>
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
        {/* TARGET tile — click-to-edit when idle. While a harvest is in
            flight, max-domains is locked into the running process; editing
            here only matters for the *next* Start. We surface this via the
            disabled state + title attr instead of silently no-op'ing. */}
        <EditableStat
          label="Target"
          value={maxDomains}
          locked={running}
          onCommit={(v) => setMaxDomains(Math.max(1, v))}
          lockedHint="Locked while harvest is running — Stop to change for next run"
          editHint="Click to set the next harvest's target"
        />
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

      {/* Detailed harvest stats — parsed from the latest [PROGRESS] line
          in log_tail. Hidden when no progress lines exist yet (clean Idle
          state before the first run) so the cockpit doesn't show a row of
          zeros to a confused operator. */}
      {(() => {
        const prog = parseLatestProgress(status?.log_tail ?? [])
        if (!prog && !running && !finishedAt) return null
        const live = prog?.live ?? status?.domains_found ?? 0
        const target = prog?.target ?? status?.max_domains ?? maxDomains
        const tested = prog?.tested ?? 0
        const extracted = prog?.extracted ?? 0
        const filesDone = prog?.filesDone ?? 0
        const filesTotal = prog?.filesTotal ?? 0
        const startedMs = status?.started_at ? new Date(status.started_at).getTime() : 0
        const finishedMs = finishedAt ? new Date(finishedAt).getTime() : Date.now()
        const elapsedSec = startedMs > 0 ? Math.max(1, (finishedMs - startedMs) / 1000) : 0
        const rate = elapsedSec > 0 ? live / elapsedSec : 0
        const hitRate = tested > 0 ? (live / tested) * 100 : 0
        const remaining = Math.max(0, target - live)
        const etaSec = rate > 0 ? remaining / rate : 0
        return (
          <div style={{ marginTop: '.6rem', display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '.75rem' }}>
            <Stat label="Tested" value={tested.toLocaleString()} />
            <Stat label="Extracted" value={extracted.toLocaleString()} />
            <Stat
              label="WARC files"
              value={filesTotal > 0 ? `${filesDone}/${filesTotal}` : '—'}
            />
            <Stat
              label="Hit rate"
              value={tested > 0 ? `${hitRate.toFixed(1)}%` : '—'}
            />
            <Stat
              label="Rate"
              value={rate > 0 ? `${rate.toFixed(1)} live/s` : '—'}
            />
            <Stat
              label="Elapsed"
              value={elapsedSec > 0 ? formatDuration(elapsedSec) : '—'}
            />
            <Stat
              label="ETA"
              value={
                running && rate > 0 && remaining > 0
                  ? formatDuration(etaSec)
                  : !running && finishedAt
                    ? 'done'
                    : '—'
              }
            />
            <Stat
              label="Progress"
              value={target > 0 ? `${Math.min(100, (live / target) * 100).toFixed(1)}%` : '—'}
            />
          </div>
        )
      })()}

      {(error || r2Error) && (
        <p className={`settings-hint ${r2Error ? 'tg-hint--err' : ''}`} style={{ marginTop: '.75rem' }}>
          {error || r2Error}
        </p>
      )}

      <div style={{ marginTop: '1rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '.6rem' }}>
          <div className="muted" style={{ fontSize: '.78rem' }}>
            R2 exports {r2Objects.length > 0 ? `(${r2Objects.length})` : ''}
          </div>
          <button
            type="button"
            className="btn-glass btn-glass--xs"
            onClick={() => void refreshR2Objects()}
            disabled={r2Listing}
            title="Re-list objects in the warc/ prefix"
          >
            {r2Listing ? '…' : '↻'}
          </button>
        </div>
        {r2Objects.length === 0 ? (
          <p className="muted" style={{ fontSize: '.72rem', marginTop: '.3rem' }}>
            {r2State === 'connected' ? 'No exports yet.' : 'Connect R2 in Settings to see exports.'}
          </p>
        ) : (
          <table className="fleet-boot__table" style={{ marginTop: '.4rem', fontSize: '.74rem' }}>
            <thead>
              <tr>
                <th>Key</th>
                <th style={{ textAlign: 'right' }}>Size</th>
                <th>Modified</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {r2Objects.map((o) => (
                <tr key={o.key}>
                  <td className="mono" style={{ wordBreak: 'break-all' }}>{o.key}</td>
                  <td className="mono" style={{ textAlign: 'right' }}>{(o.size / 1024).toFixed(1)} KB</td>
                  <td className="muted">{o.modified ? new Date(o.modified).toLocaleString() : '—'}</td>
                  <td>
                    <button
                      type="button"
                      className="btn-danger-outline"
                      style={{ fontSize: '.7rem', padding: '.15rem .5rem' }}
                      onClick={() => void onDeleteR2(o.key)}
                      title={`Delete ${o.key} from R2`}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

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

/** Parse the most recent `[PROGRESS] Live: 18/10000 | Tested: 76 | Extracted:
 * 3636 | Files: 0/200` line emitted by warc.go. ANSI colour escapes are
 * stripped so the regex stays simple. Returns null when no progress line is
 * present (clean Idle state pre-first-run). */
function parseLatestProgress(lines: readonly string[]): {
  live: number; target: number; tested: number; extracted: number
  filesDone: number; filesTotal: number
} | null {
  if (!lines || lines.length === 0) return null
  const stripAnsi = (s: string) => s.replace(/\[[0-9;]*m/g, '')
  const re = /\[PROGRESS\]\s+Live:\s+(\d+)\/(\d+)\s+\|\s+Tested:\s+(\d+)\s+\|\s+Extracted:\s+(\d+)\s+\|\s+Files:\s+(\d+)\/(\d+)/
  // Walk lines in reverse — the most recent progress line wins. A single
  // log entry can hold many concatenated progress chunks (\r-based progress
  // bar in warc.go), so we also re-scan inside each entry with a global
  // regex and take the last match.
  for (let i = lines.length - 1; i >= 0; i--) {
    const cleaned = stripAnsi(lines[i] ?? '')
    let last: RegExpExecArray | null = null
    const g = new RegExp(re, 'g')
    let m: RegExpExecArray | null
    while ((m = g.exec(cleaned)) !== null) last = m
    if (last) {
      return {
        live: Number(last[1]) || 0,
        target: Number(last[2]) || 0,
        tested: Number(last[3]) || 0,
        extracted: Number(last[4]) || 0,
        filesDone: Number(last[5]) || 0,
        filesTotal: Number(last[6]) || 0,
      }
    }
  }
  return null
}

/** "1h 23m 04s" / "23m 04s" / "04s" — used by Elapsed and ETA tiles. */
function formatDuration(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m ${String(sec).padStart(2, '0')}s`
  if (m > 0) return `${m}m ${String(sec).padStart(2, '0')}s`
  return `${sec}s`
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div style={{ background: 'rgba(255,255,255,.03)', borderRadius: '.4rem', padding: '.55rem .7rem' }}>
      <div className="muted" style={{ fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.06em' }}>{label}</div>
      <div className="mono" style={{ fontSize: '1rem', marginTop: '.15rem' }}>{value}</div>
    </div>
  )
}

/** Same shape as Stat but the value cell turns into an input on click.
 * Used for TARGET so the operator can change the next run's max-domains
 * without scrolling back to the form. Locked while a harvest is running. */
function EditableStat({
  label, value, locked, lockedHint, editHint, onCommit,
}: {
  label: string
  value: number
  locked: boolean
  lockedHint: string
  editHint: string
  onCommit: (next: number) => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<string>(String(value))
  // Keep the draft in sync if the parent value updates (e.g., after a Start
  // resets it). Without this the field would silently drift.
  useEffect(() => { if (!editing) setDraft(String(value)) }, [value, editing])

  const commit = () => {
    const n = Number(draft)
    if (Number.isFinite(n) && n > 0) onCommit(Math.floor(n))
    setEditing(false)
  }

  return (
    <div
      style={{
        background: 'rgba(255,255,255,.03)',
        borderRadius: '.4rem',
        padding: '.55rem .7rem',
        cursor: locked ? 'not-allowed' : 'pointer',
        outline: editing ? '1px solid var(--accent, #6cc6ff)' : 'none',
      }}
      title={locked ? lockedHint : editHint}
      onClick={() => { if (!locked && !editing) setEditing(true) }}
    >
      <div className="muted" style={{ fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.06em' }}>
        {label} {locked ? '🔒' : editing ? '✎' : ''}
      </div>
      {editing ? (
        <input
          autoFocus
          type="number"
          min={1}
          className="mono"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') commit()
            else if (e.key === 'Escape') { setDraft(String(value)); setEditing(false) }
          }}
          style={{
            fontSize: '1rem', marginTop: '.15rem', width: '100%',
            background: 'transparent', color: 'inherit',
            border: 'none', outline: 'none', padding: 0,
          }}
        />
      ) : (
        <div className="mono" style={{ fontSize: '1rem', marginTop: '.15rem' }}>
          {value.toLocaleString()}
        </div>
      )}
    </div>
  )
}
