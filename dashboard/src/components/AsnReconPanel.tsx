import { useEffect, useRef, useState } from 'react'
import { asnRecon } from '../lib/reconApi'

type JobState = {
  jobId: string
  status: 'running' | 'done' | 'error'
  log: string[]
  summary: string | null
  combinedLines: number
  asns: string[]
}

export function AsnReconPanel() {
  const [asnInput, setAsnInput]     = useState('')
  const [workers, setWorkers]       = useState(100)
  const [maxIps, setMaxIps]         = useState(100000)
  const [skipRdns, setSkipRdns]     = useState(false)
  const [crtsh, setCrtsh]           = useState(false)
  const [shodanKey, setShodanKey]   = useState('')
  const [job, setJob]               = useState<JobState | null>(null)
  const [starting, setStarting]     = useState(false)
  const [error, setError]           = useState<string | null>(null)
  const pollRef                     = useRef<ReturnType<typeof setInterval> | null>(null)
  const logRef                      = useRef<HTMLDivElement>(null)

  // Auto-scroll log to bottom
  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight
    }
  }, [job?.log])

  // Poll status while running
  useEffect(() => {
    if (!job || job.status !== 'running') {
      if (pollRef.current) clearInterval(pollRef.current)
      return
    }
    pollRef.current = setInterval(async () => {
      try {
        const res = await asnRecon.status(job.jobId)
        setJob(prev => prev ? {
          ...prev,
          status: res.status,
          log: res.log,
          summary: res.summary,
          combinedLines: res.combined_lines,
        } : prev)
      } catch { /* ignore transient errors */ }
    }, 2000)
    return () => { if (pollRef.current) clearInterval(pollRef.current) }
  }, [job?.jobId, job?.status])

  function parseAsns(raw: string): string[] {
    return raw
      .split(/[\n,\s]+/)
      .map(s => s.trim().toUpperCase())
      .filter(s => /^AS?\d+$/i.test(s))
      .map(s => s.startsWith('AS') ? s : `AS${s}`)
  }

  async function handleStart() {
    setError(null)
    const asns = parseAsns(asnInput)
    if (asns.length === 0) {
      setError('Enter at least one valid ASN (e.g. AS14061 or 14061)')
      return
    }
    setStarting(true)
    try {
      const res = await asnRecon.start({
        asns,
        workers,
        max_ips: maxIps,
        skip_rdns: skipRdns,
        crtsh,
        shodan_key: shodanKey || undefined,
      })
      if (!res.ok || !res.job_id) {
        setError(res.error ?? 'Failed to start job')
        return
      }
      setJob({
        jobId: res.job_id,
        status: 'running',
        log: [],
        summary: null,
        combinedLines: 0,
        asns,
      })
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setStarting(false)
    }
  }

  function handleDownload() {
    if (!job) return
    const url = asnRecon.downloadUrl(job.jobId)
    const a = document.createElement('a')
    a.href = url
    a.download = `asn_recon_${job.asns.join('_')}.txt`
    a.click()
  }

  function handleReset() {
    setJob(null)
    setError(null)
  }

  const parsedAsns = parseAsns(asnInput)

  return (
    <section className="asn-recon-panel">
      <header className="lists-panel__head">
        <div>
          <h2 className="lists-panel__title">ASN Recon</h2>
          <p className="lists-panel__lede">
            Enter ASN numbers to fetch all IP ranges, run reverse DNS, and extract domains ready to feed into the scanner.
          </p>
        </div>
      </header>

      {!job && (
        <div className="asn-recon-panel__form">
          {/* ASN input */}
          <div className="asn-recon-panel__field">
            <label className="asn-recon-panel__label">
              ASN numbers
              <span className="asn-recon-panel__hint">one per line, or comma-separated. e.g. AS14061, AS24940</span>
            </label>
            <textarea
              className="asn-recon-panel__textarea"
              rows={5}
              placeholder={'AS14061\nAS24940\nAS51167'}
              value={asnInput}
              onChange={e => setAsnInput(e.target.value)}
              spellCheck={false}
            />
            {parsedAsns.length > 0 && (
              <p className="asn-recon-panel__parsed">
                Parsed: {parsedAsns.map(a => <code key={a} className="inline-code" style={{ marginRight: 4 }}>{a}</code>)}
              </p>
            )}
          </div>

          {/* Options row */}
          <div className="asn-recon-panel__options">
            <div className="asn-recon-panel__opt-group">
              <label className="asn-recon-panel__label">Workers (rdns concurrency)</label>
              <input
                type="number"
                className="asn-recon-panel__num"
                min={10} max={500} step={10}
                value={workers}
                onChange={e => setWorkers(Number(e.target.value))}
              />
            </div>
            <div className="asn-recon-panel__opt-group">
              <label className="asn-recon-panel__label">Max IPs per ASN</label>
              <input
                type="number"
                className="asn-recon-panel__num"
                min={1000} max={2000000} step={10000}
                value={maxIps}
                onChange={e => setMaxIps(Number(e.target.value))}
              />
            </div>
            <label className="asn-recon-panel__toggle">
              <input type="checkbox" checked={skipRdns} onChange={e => setSkipRdns(e.target.checked)} />
              Skip reverse DNS
            </label>
            <label className="asn-recon-panel__toggle">
              <input type="checkbox" checked={crtsh} onChange={e => setCrtsh(e.target.checked)} />
              crt.sh (slow, more domains)
            </label>
          </div>

          {/* Shodan key */}
          <div className="asn-recon-panel__field">
            <label className="asn-recon-panel__label">
              Shodan API key <span className="asn-recon-panel__hint">optional — adds Shodan as a third source</span>
            </label>
            <input
              type="password"
              className="asn-recon-panel__input"
              placeholder="Leave blank to skip Shodan"
              value={shodanKey}
              onChange={e => setShodanKey(e.target.value)}
              autoComplete="off"
            />
          </div>

          {error && <p className="lists-upload__error lists-upload__error--read">✗ {error}</p>}

          <button
            type="button"
            className="btn-primary btn-glass"
            disabled={starting || parsedAsns.length === 0}
            onClick={() => void handleStart()}
          >
            {starting ? 'Starting…' : `Run recon on ${parsedAsns.length} ASN${parsedAsns.length === 1 ? '' : 's'}`}
          </button>
        </div>
      )}

      {job && (
        <div className="asn-recon-panel__job">
          {/* Header bar */}
          <div className="asn-recon-panel__job-head">
            <div className="asn-recon-panel__job-asns">
              {job.asns.map(a => (
                <span key={a} className="pill pill--muted">{a}</span>
              ))}
            </div>
            <div className="asn-recon-panel__job-status">
              {job.status === 'running' && (
                <span className="pill" style={{ background: 'color-mix(in srgb, #f59e0b, transparent 70%)', color: '#f59e0b' }}>
                  ● Running…
                </span>
              )}
              {job.status === 'done' && (
                <span className="pill" style={{ background: 'color-mix(in srgb, #10b981, transparent 70%)', color: '#10b981' }}>
                  ✓ Done
                </span>
              )}
              {job.status === 'error' && (
                <span className="pill" style={{ background: 'color-mix(in srgb, #ef4444, transparent 70%)', color: '#ef4444' }}>
                  ✗ Error
                </span>
              )}
            </div>
          </div>

          {/* Stats */}
          {(job.combinedLines > 0 || job.status === 'done') && (
            <div className="asn-recon-panel__stats">
              <div className="stat-chip">
                <span className="stat-chip__k">TARGETS FOUND</span>
                <strong className="stat-chip__v">{job.combinedLines.toLocaleString()}</strong>
              </div>
            </div>
          )}

          {/* Summary */}
          {job.summary && (
            <pre className="asn-recon-panel__summary">{job.summary}</pre>
          )}

          {/* Log */}
          <div className="asn-recon-panel__log" ref={logRef}>
            {job.log.length === 0 && job.status === 'running' && (
              <span className="muted">Starting process…</span>
            )}
            {job.log.map((line, i) => (
              <div key={i} className="asn-recon-panel__log-line">{stripAnsi(line)}</div>
            ))}
          </div>

          {/* Actions */}
          <div className="asn-recon-panel__actions">
            {job.status === 'done' && job.combinedLines > 0 && (
              <button type="button" className="btn-primary btn-glass" onClick={handleDownload}>
                ↓ Download combined.txt ({job.combinedLines.toLocaleString()} targets)
              </button>
            )}
            <button type="button" className="btn-glass" onClick={handleReset}>
              {job.status === 'running' ? 'Cancel & Reset' : 'New Job'}
            </button>
          </div>
        </div>
      )}
    </section>
  )
}

function stripAnsi(s: string): string {
  // eslint-disable-next-line no-control-regex
  return s.replace(/\x1b\[[0-9;]*m/g, '')
}
