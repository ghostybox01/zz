/** Polls the warc.go subprocess status endpoint while a harvest is running.
 *  Backs off to a slow heartbeat when idle so we don't hammer nginx for nothing.
 */
import { useEffect, useRef, useState } from 'react'
import { warc as warcApi, type WarcStatus } from '../lib/reconApi'

const POLL_FAST_MS = 3_000   // while the subprocess is running
const POLL_SLOW_MS = 30_000  // when idle — just to notice the binary appearing

export type WarcReachability = 'unknown' | 'ok' | 'unreachable'

export type UseWarcStatusResult = {
  status: WarcStatus | null
  lastError: string | null
  reachability: WarcReachability
  refresh: () => Promise<WarcStatus | null>
}

export function useWarcStatus(): UseWarcStatusResult {
  const [status, setStatus] = useState<WarcStatus | null>(null)
  const [lastError, setLastError] = useState<string | null>(null)
  const [reachability, setReachability] = useState<WarcReachability>('unknown')
  const timerRef = useRef<number | null>(null)
  const mountedRef = useRef(true)

  const refresh = async (): Promise<WarcStatus | null> => {
    try {
      const next = await warcApi.status()
      if (!mountedRef.current) return next
      setStatus(next)
      setLastError(null)
      setReachability('ok')
      return next
    } catch (err) {
      const msg = (err as Error).message || 'failed to fetch /api/warc/status'
      if (mountedRef.current) {
        setLastError(msg)
        setReachability('unreachable')
      }
      return null
    }
  }

  useEffect(() => {
    mountedRef.current = true
    let cancelled = false

    const tick = async () => {
      if (cancelled) return
      const next = await refresh()
      if (cancelled) return
      const interval = next?.running ? POLL_FAST_MS : POLL_SLOW_MS
      timerRef.current = window.setTimeout(tick, interval)
    }
    void tick()

    return () => {
      mountedRef.current = false
      cancelled = true
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current)
        timerRef.current = null
      }
    }
  }, [])

  return { status, lastError, reachability, refresh }
}
