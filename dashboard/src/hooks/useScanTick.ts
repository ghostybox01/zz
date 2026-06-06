/** Animates RPS/PPS so the dashboard feels alive while a scan runs.
 *  Counts (validHosts, invalidHosts, done, hits) are intentionally NOT
 *  bumped here — they come from the real API and must only go up. */
import { useEffect } from 'react'
import type { Scan, ScanShard } from '../types'

type Args = {
  scanning: boolean
  setScans: React.Dispatch<React.SetStateAction<Scan[]>>
  setShards: React.Dispatch<React.SetStateAction<ScanShard[]>>
}

function jitter(n: number, span: number, min = 0): number {
  return Math.max(min, n + (Math.random() - 0.5) * span * 2)
}

export function useScanTick({ scanning, setScans, setShards }: Args) {
  useEffect(() => {
    if (!scanning) return
    const id = window.setInterval(() => {
      setScans((prev) =>
        prev.map((s) => {
          if (s.status !== 'running') return s
          const rps = jitter(s.requestsPerSec, 2.5, 4)
          const pps = jitter(s.parsingPerSec, 8, 12)
          return {
            ...s,
            requestsPerSec: +rps.toFixed(1),
            parsingPerSec: +pps.toFixed(1),
            rpsHistory: [...s.rpsHistory.slice(-23), Math.round(rps)],
          }
        }),
      )
      setShards((prev) =>
        prev.map((sh) => {
          if (sh.done >= sh.assigned) return sh
          const rps = jitter(sh.requestsPerSec, 1.2, 2)
          const pps = jitter(sh.parsingPerSec, 4, 6)
          return {
            ...sh,
            requestsPerSec: +rps.toFixed(1),
            parsingPerSec: +pps.toFixed(1),
          }
        }),
      )
    }, 1500)
    return () => window.clearInterval(id)
  }, [scanning, setScans, setShards])
}
