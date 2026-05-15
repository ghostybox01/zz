/** Drip-feeds running scan stats so the dashboard feels alive. Pure browser-side simulation. */
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
          const validBump = Math.floor(rps * 0.6 + Math.random() * 8)
          const invalidBump = Math.floor(rps * 0.18 + Math.random() * 3)
          const hitsBump = Math.random() > 0.75 ? Math.floor(Math.random() * 3) : 0
          return {
            ...s,
            requestsPerSec: +rps.toFixed(1),
            parsingPerSec: +pps.toFixed(1),
            validHosts: Math.min(s.targetCount, s.validHosts + validBump),
            invalidHosts: s.invalidHosts + invalidBump,
            hitsFound: s.hitsFound + hitsBump,
            validHits: s.validHits + (hitsBump > 0 && Math.random() > 0.45 ? 1 : 0),
            rpsHistory: [...s.rpsHistory.slice(-23), Math.round(rps)],
            lastEvent: hitsBump > 0 ? `+${hitsBump} match${hitsBump > 1 ? 'es' : ''} routed` : s.lastEvent,
          }
        }),
      )
      setShards((prev) =>
        prev.map((sh) => {
          // Tick will be reconciled with the parent scan's status by the consumer;
          // here we just keep numbers moving for any shard that has done < assigned.
          if (sh.done >= sh.assigned) return sh
          const rps = jitter(sh.requestsPerSec, 1.2, 2)
          const pps = jitter(sh.parsingPerSec, 4, 6)
          const validBump = Math.floor(rps * 0.55 + Math.random() * 4)
          const invalidBump = Math.floor(rps * 0.15 + Math.random() * 2)
          const hitsBump = Math.random() > 0.85 ? Math.floor(Math.random() * 2) : 0
          return {
            ...sh,
            requestsPerSec: +rps.toFixed(1),
            parsingPerSec: +pps.toFixed(1),
            validHosts: Math.min(sh.assigned, sh.validHosts + validBump),
            invalidHosts: sh.invalidHosts + invalidBump,
            done: Math.min(sh.assigned, sh.done + validBump + invalidBump),
            hits: sh.hits + hitsBump,
          }
        }),
      )
    }, 1500)
    return () => window.clearInterval(id)
  }, [scanning, setScans, setShards])
}
