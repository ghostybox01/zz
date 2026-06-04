import { useCallback, useEffect, useRef, useState } from 'react'
import { prefilter as prefilterApi, vps as vpsApi } from '../lib/reconApi'
import type { TargetList } from '../types'

type Props = {
  lists: TargetList[]
  onToast: (title: string, message?: string, kind?: 'error' | 'info') => void
  /** Called when the operator clicks "Use as scan list" on a results set. */
  onUseResults?: (hosts: string[], label: string) => void
}

export function PrefilterPanel({ lists, onToast, onUseResults }: Props) {
  const [servers, setServers] = useState<string[]>([])
  const [selectedList, setSelectedList] = useState<string>('')
  const [pastedDomains, setPastedDomains] = useState<string>('')
  const [selectedVps, setSelectedVps] = useState<string>('')
  const [running, setRunning] = useState(false)
  const [results, setResults] = useState<string[]>([])
  const [totalChecked, setTotalChecked] = useState(0)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Load server roster on mount
  useEffect(() => {
    void (async () => {
      try {
        const r = await vpsApi.servers()
        const s = r.servers ?? []
        setServers(s)
        if (s.length > 0) setSelectedVps(s[0]!)
      } catch { /* ignore — backend may be offline */ }
    })()
  }, [])

  const stopPoll = useCallback(() => {
    if (pollRef.current !== null) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }, [])

  const pollResults = useCallback(async () => {
    try {
      const r = await prefilterApi.results()
      if (r.hits) setResults(r.hits)
      if (r.total_checked != null) setTotalChecked(r.total_checked)
      if (!r.running) {
        setRunning(false)
        stopPoll()
        onToast(
          `Pre-filter done — ${r.total_hits ?? r.hits?.length ?? 0} live`,
          `Checked ${r.total_checked ?? '?'} targets`,
          'info',
        )
      }
    } catch { /* keep polling */ }
  }, [onToast, stopPoll])

  const handleStart = useCallback(async () => {
    const listFile = selectedList
    const vpsIp = selectedVps
    const pasted = pastedDomains.trim()

    if (!vpsIp) {
      onToast('Select a VPS first', undefined, 'error')
      return
    }
    if (!listFile && !pasted) {
      onToast('Select a list or paste domains', undefined, 'error')
      return
    }

    setRunning(true)
    setResults([])
    setTotalChecked(0)

    try {
      await prefilterApi.start({ list_file: listFile || '__paste__', vps_ip: vpsIp })
      onToast('Pre-filter started', `Running on ${vpsIp}`, 'info')
      // Start polling for results
      pollRef.current = setInterval(() => { void pollResults() }, 3_000)
    } catch (e) {
      setRunning(false)
      onToast('Pre-filter failed to start', (e as Error).message, 'error')
    }
  }, [selectedList, selectedVps, pastedDomains, onToast, pollResults])

  // Clean up on unmount
  useEffect(() => () => { stopPoll() }, [stopPoll])

  const handleUseResults = useCallback(() => {
    if (results.length === 0) return
    onUseResults?.(results, `prefilter-${new Date().toISOString().slice(0, 10)}`)
  }, [results, onUseResults])

  return (
    <div className="findings-panel">
      <header className="findings-panel__head">
        <div>
          <h2 className="findings-panel__title">Pre-filter — target liveness probe</h2>
          <p className="muted findings-panel__sub">
            Quickly probe a target list to discard dead hosts before running a full crack session.
            Results are live URLs that responded with 2xx/3xx.
          </p>
        </div>
      </header>

      <div className="card-block card-block--tight" style={{ marginBottom: '1rem' }}>
        {/* List selector */}
        <label className="cw-scanners__upload-label" htmlFor="pf-list">
          Select an uploaded list
        </label>
        <select
          id="pf-list"
          className="tg-input"
          value={selectedList}
          onChange={(e) => setSelectedList(e.target.value)}
          disabled={running}
          style={{ width: '100%', marginBottom: '.5rem' }}
        >
          <option value="">— paste domains below instead —</option>
          {lists.map((l) => (
            <option key={l.id} value={l.name}>
              {l.name} ({l.lineCount.toLocaleString()} lines)
            </option>
          ))}
        </select>

        {/* Paste area */}
        <label className="cw-scanners__upload-label" htmlFor="pf-paste">
          Or paste domains directly (one per line)
        </label>
        <textarea
          id="pf-paste"
          className="tg-input"
          rows={5}
          value={pastedDomains}
          onChange={(e) => setPastedDomains(e.target.value)}
          disabled={running || !!selectedList}
          placeholder="https://example.com&#10;https://target2.io&#10;..."
          style={{ width: '100%', fontFamily: 'monospace', fontSize: '.82rem', resize: 'vertical', marginBottom: '.5rem' }}
        />

        {/* VPS selector */}
        <label className="cw-scanners__upload-label" htmlFor="pf-vps">
          Run pre-filter on VPS
        </label>
        <select
          id="pf-vps"
          className="tg-input"
          value={selectedVps}
          onChange={(e) => setSelectedVps(e.target.value)}
          disabled={running}
          style={{ width: '100%', marginBottom: '.75rem' }}
        >
          {servers.length === 0 && <option value="">No VPS rostered</option>}
          {servers.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>

        <div style={{ display: 'flex', gap: '.5rem', alignItems: 'center' }}>
          <button
            type="button"
            className="btn-glass"
            onClick={() => void handleStart()}
            disabled={running}
          >
            {running ? 'Running…' : 'Start Pre-filter'}
          </button>
          {running && (
            <span className="muted" style={{ fontSize: '.82rem' }}>
              Checked {totalChecked.toLocaleString()} · {results.length} live…
            </span>
          )}
        </div>
      </div>

      {/* Results */}
      {results.length > 0 && (
        <div className="card-block card-block--tight">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '.5rem' }}>
            <span className="muted" style={{ fontSize: '.82rem' }}>
              {results.length} live target{results.length !== 1 ? 's' : ''} found
            </span>
            {onUseResults && (
              <button type="button" className="btn-glass btn-glass--xs" onClick={handleUseResults}>
                Use as scan list
              </button>
            )}
          </div>
          <div className="findings-table-wrap">
            <table className="findings-table">
              <thead>
                <tr>
                  <th>#</th>
                  <th>URL</th>
                </tr>
              </thead>
              <tbody>
                {results.map((url, i) => (
                  <tr key={url}>
                    <td className="muted" style={{ fontSize: '.75rem', width: '3rem' }}>{i + 1}</td>
                    <td>
                      <a href={url} target="_blank" rel="noopener noreferrer" className="findings-source">
                        {url}
                      </a>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
