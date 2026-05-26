/** Subscribes to the real backend for fleet status.
 *  - GET /api/vps/status every 5s
 *  - Socket event `vps_update` whenever the backend broadcasts (manager.start_monitoring callback)
 *  - Maps Raven's ServerStatus dataclass into the dashboard's VpsNode shape.
 *
 *  IMPORTANT: This hook reads the roster from the BACKEND (which sources `server_ips.txt`).
 *  Auto-enrolled "discovered" nodes from `useFleetEnrollment` are NOT pushed to the backend by this hook —
 *  by design. The backend roster is the single source of truth so any client-side enrollment
 *  is overwritten on the next poll. To add a VPS, the operator edits server_ips.txt explicitly
 *  (or uses vps.addServerByIp with a manually typed IP).
 */
import { useEffect, useRef, useState } from 'react'
import { vps as vpsApi, type ReconFleetStatus, type ReconServerStatus, type ReconGlobalStats } from '../lib/reconApi'
import { getReconSocket } from '../lib/reconSocket'
import type { VpsNode, VpsStatus } from '../types'

export type UseReconFleetResult = {
  fleet: VpsNode[]
  globalStats: ReconGlobalStats | null
  lastError: string | null
  /** Last successful poll timestamp. */
  lastPollAt: number | null
  refresh: () => Promise<void>
  isLive: boolean
}

const POLL_MS = 5_000

/** Map backend's free-form status string into our typed VpsStatus enum. */
function mapStatus(raw: string): VpsStatus {
  const s = raw.toUpperCase()
  if (s === 'RUNNING' || s === 'HEALTHY' || s === 'ONLINE' || s === 'OK' || s === 'IDLE') return 'healthy'
  if (s === 'DEGRADED' || s === 'SLOW' || s === 'WARNING') return 'degraded'
  if (s === 'CONNECTING' || s === 'RECONNECTING' || s === 'STARTING') return 'reconnecting'
  if (s === 'OFFLINE' || s === 'DOWN' || s === 'ERROR' || s === 'UNREACHABLE') return 'offline'
  if (s === 'REMOVED' || s === 'PRUNED') return 'removed'
  return 'reconnecting'
}

function uptimeStringToMin(uptime: string): number {
  // ssh_manager stores uptime as a free-form string like "2h 14m" or "—". Parse best-effort.
  if (!uptime || uptime === '-' || uptime === '—') return 0
  let total = 0
  const dMatch = uptime.match(/(\d+)\s*d/i)
  const hMatch = uptime.match(/(\d+)\s*h/i)
  const mMatch = uptime.match(/(\d+)\s*m(?!s)/i)
  if (dMatch && dMatch[1]) total += Number(dMatch[1]) * 24 * 60
  if (hMatch && hMatch[1]) total += Number(hMatch[1]) * 60
  if (mMatch && mMatch[1]) total += Number(mMatch[1])
  return total
}

function deriveRegion(ip: string): string {
  // No real region info from backend yet. Use the leading octet as a stable label.
  const parts = ip.split('.')
  return parts[0] ? `IP/${parts[0]}` : 'IP/?'
}

function mapServer(s: ReconServerStatus): VpsNode {
  const status = mapStatus(s.status)
  return {
    id: `srv-${s.ip}`,
    label: s.ip,
    host: s.ip,
    region: deriveRegion(s.ip),
    status,
    source: 'seed',          // backend roster only — never 'discovered'
    // Live machine metrics now come from the probe (CPU% via /proc/stat
    // delta, RAM via /proc/meminfo, disk via df, system uptime via
    // /proc/uptime). Falls back to 0 if the backend predates the field
    // (e.g. mid-deploy where a cached snapshot is still around).
    cpuPercent: s.cpu_percent ?? 0,
    cpuHistory: [],
    ramUsedGb: s.ram_used_gb ?? 0,
    ramTotalGb: s.ram_total_gb ?? 0,
    diskUsedGb: s.disk_used_gb ?? 0,
    diskTotalGb: s.disk_total_gb,
    targetsAssigned: s.targets ?? 0,
    targetsDone: s.scanned ?? 0,
    scansPerSecond: s.speed ?? 0,
    reconnectFailCount: 0,
    findingsContributed: s.hits ?? 0,
    // Prefer the system uptime (whole-host, "is the box on?") over the
    // per-process etime. ssh_manager sets sys_uptime_sec on every healthy
    // probe; falls back to parsing the etime string otherwise.
    uptimeMin: (s.sys_uptime_sec ?? 0) > 0
      ? Math.floor((s.sys_uptime_sec ?? 0) / 60)
      : uptimeStringToMin(s.uptime),
    activeListName: s.active_list_name ?? undefined,
    lastEvent: s.error
      ? `Error: ${s.error}`
      : s.last_good_update
        ? `Last probe ${s.last_good_update}${s.batch_info && s.batch_info !== '-' ? ` · ${s.batch_info}` : ''}`
        : s.batch_info ?? '',
    removedReason: status === 'removed' ? s.error ?? 'Removed' : undefined,
  }
}

export function useReconFleet(): UseReconFleetResult {
  const [fleet, setFleet] = useState<VpsNode[]>([])
  const [globalStats, setGlobalStats] = useState<ReconGlobalStats | null>(null)
  const [lastError, setLastError] = useState<string | null>(null)
  const [lastPollAt, setLastPollAt] = useState<number | null>(null)
  const [isLive, setIsLive] = useState(false)
  const mountedRef = useRef(true)
  const pollIdRef = useRef<number | null>(null)

  const apply = (payload: ReconFleetStatus) => {
    if (!mountedRef.current) return
    setFleet((payload.servers ?? []).map(mapServer))
    setGlobalStats(payload.stats ?? null)
    setLastError(null)
    setLastPollAt(Date.now())
    setIsLive(true)
  }

  const refresh = async (): Promise<void> => {
    try {
      const payload = await vpsApi.status()
      apply(payload)
    } catch (err) {
      if (!mountedRef.current) return
      const status = (err as { status?: number }).status
      // 503 means SSH manager not available — backend running but no fleet wired. Don't spam errors.
      if (status !== 503) setLastError((err as Error).message ?? 'Failed to fetch /api/vps/status')
      setIsLive(false)
    }
  }

  useEffect(() => {
    mountedRef.current = true
    void refresh()

    const socket = getReconSocket()
    const onVpsUpdate = (payload: { servers?: ReconServerStatus[]; stats?: ReconGlobalStats }) => {
      if (!payload?.servers) return
      apply({ servers: payload.servers, stats: payload.stats ?? {} })
    }
    socket.on('vps_update', onVpsUpdate)
    // Ask backend to start its 5s monitor for socket pushes.
    socket.emit('vps_start_monitoring')
    // Re-subscribe after any reconnect — the backend drops monitoring state on disconnect.
    const onReconnect = () => { socket.emit('vps_start_monitoring') }
    socket.on('connect', onReconnect)

    pollIdRef.current = window.setInterval(() => { void refresh() }, POLL_MS)

    return () => {
      mountedRef.current = false
      socket.off('vps_update', onVpsUpdate)
      socket.off('connect', onReconnect)
      socket.emit('vps_stop_monitoring')
      if (pollIdRef.current !== null) {
        window.clearInterval(pollIdRef.current)
        pollIdRef.current = null
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return { fleet, globalStats, lastError, lastPollAt, refresh, isLive }
}
