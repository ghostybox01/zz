/** Polls the configured live source for scanner output files and emits new findings.
 *  Tracks per-file ETag/last-modified + line counts so duplicates are dropped between polls.
 *  Files are fetched as plain text (the scanner appends one record per line, never rewrites).
 */
import { useEffect, useRef, useState } from 'react'
import type { Finding } from '../types'
import { authHeaders, joinUrl, type LiveSourceConfig } from '../lib/liveSource'
import { COUNTER_FILES, SCAN_FILES, countLines, parseScanFile, type ScanFileSchema } from '../lib/parseScanFiles'

export type LiveScanStatus =
  | { state: 'idle' }
  | { state: 'connecting' }
  | { state: 'ok'; lastPollAt: string; filesSeen: number }
  | { state: 'error'; message: string; lastTryAt: string }

export type LiveTotals = {
  liveDomains: number
  filesProcessed: number
  totalFindings: number
}

type Args = {
  config: LiveSourceConfig
  pushFinding: (f: Omit<Finding, 'id'>) => void
  setLiveTotals: (t: LiveTotals) => void
}

type FileState = {
  /** Number of non-empty lines we've already emitted findings for. */
  emittedLines: number
  /** Last `ETag` or `Last-Modified` value we saw; used to skip unchanged polls. */
  lastSig: string
}

export function useLiveScan({ config, pushFinding, setLiveTotals }: Args): LiveScanStatus {
  const [status, setStatus] = useState<LiveScanStatus>({ state: 'idle' })
  const fileStateRef = useRef<Map<string, FileState>>(new Map())
  const totalsRef = useRef<LiveTotals>({ liveDomains: 0, filesProcessed: 0, totalFindings: 0 })

  useEffect(() => {
    if (!config.enabled || !config.baseUrl) {
      queueMicrotask(() => {
        setStatus({ state: 'idle' })
        fileStateRef.current.clear()
        totalsRef.current = { liveDomains: 0, filesProcessed: 0, totalFindings: 0 }
      })
      return
    }

    let cancelled = false
    queueMicrotask(() => setStatus({ state: 'connecting' }))

    const reportedByHost = (() => {
      try {
        return new URL(config.baseUrl).host || 'remote'
      } catch {
        return 'remote'
      }
    })()

    const fetchFile = async (schema: ScanFileSchema): Promise<{ skipped: boolean; emitted: number; lines: number }> => {
      const url = joinUrl(config.baseUrl, schema.file)
      const headers = authHeaders(config.bearerToken)
      const state = fileStateRef.current.get(schema.file) ?? { emittedLines: 0, lastSig: '' }

      let res: Response
      try {
        res = await fetch(url, { headers })
      } catch (err) {
        throw new Error(`fetch failed for ${schema.file}`, { cause: err })
      }

      if (res.status === 404) {
        // File simply hasn't been created yet — normal until scanner finds something of this type.
        return { skipped: true, emitted: 0, lines: state.emittedLines }
      }
      if (!res.ok) throw new Error(`${schema.file}: HTTP ${res.status}`)

      const sig =
        res.headers.get('etag') ??
        res.headers.get('last-modified') ??
        res.headers.get('content-length') ??
        ''
      if (sig && sig === state.lastSig) {
        return { skipped: true, emitted: 0, lines: state.emittedLines }
      }

      const body = await res.text()
      const totalLines = countLines(body)

      let emitted = 0
      if (totalLines > state.emittedLines) {
        const all = parseScanFile(schema, body, reportedByHost, new Date().toISOString())
        const fresh = all.slice(state.emittedLines)
        for (const f of fresh) {
          if (cancelled) break
          pushFinding({
            at: f.at,
            provider: f.provider,
            ruleLabel: f.ruleLabel,
            hostname: f.hostname,
            url: f.url,
            path: f.path,
            detail: f.detail,
            severity: f.severity,
            reportedByHost: f.reportedByHost,
            details: f.details,
          })
          emitted++
        }
      }

      fileStateRef.current.set(schema.file, {
        emittedLines: totalLines,
        lastSig: sig,
      })
      return { skipped: false, emitted, lines: totalLines }
    }

    const fetchCounter = async (file: string): Promise<number> => {
      try {
        const res = await fetch(joinUrl(config.baseUrl, file), { headers: authHeaders(config.bearerToken) })
        if (!res.ok) return totalsRef.current.liveDomains
        const body = await res.text()
        return countLines(body)
      } catch {
        return totalsRef.current.liveDomains
      }
    }

    const pollOnce = async () => {
      let filesSeen = 0
      let errored: string | null = null
      let totalLinesFindings = 0

      for (const schema of SCAN_FILES) {
        if (cancelled) return
        try {
          const r = await fetchFile(schema)
          if (!r.skipped) filesSeen++
          totalLinesFindings += r.lines
        } catch (err) {
          errored = (err as Error).message
        }
      }

      const liveDomains = await fetchCounter(COUNTER_FILES.liveDomains)
      totalsRef.current = {
        liveDomains,
        filesProcessed: fileStateRef.current.size,
        totalFindings: totalLinesFindings,
      }
      setLiveTotals(totalsRef.current)

      if (cancelled) return
      if (errored && filesSeen === 0) {
        setStatus({ state: 'error', message: errored, lastTryAt: new Date().toISOString() })
      } else {
        setStatus({ state: 'ok', lastPollAt: new Date().toISOString(), filesSeen })
      }
    }

    // Kick immediately, then on interval.
    void pollOnce()
    const id = window.setInterval(() => void pollOnce(), config.pollIntervalMs)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [config.enabled, config.baseUrl, config.bearerToken, config.pollIntervalMs, pushFinding, setLiveTotals])

  return status
}
