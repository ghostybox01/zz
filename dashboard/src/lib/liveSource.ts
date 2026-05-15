/** Live-source config — points the dashboard at a VPS that exposes the scanner's output dir over HTTP. */

const LS_BASE = 'scan-cockpit-live-base'
const LS_TOKEN = 'scan-cockpit-live-token'
const LS_ENABLED = 'scan-cockpit-live-enabled'
const LS_INTERVAL = 'scan-cockpit-live-interval'

export type LiveSourceConfig = {
  enabled: boolean
  /** Base URL where scanner output files live, e.g. `https://vps.example.com/results/`. Trailing slash recommended. */
  baseUrl: string
  /** Optional bearer token sent as `Authorization: Bearer <token>` when fetching. */
  bearerToken: string
  pollIntervalMs: number
}

const DEFAULT_INTERVAL = 5000

export function loadLiveSource(): LiveSourceConfig {
  if (typeof localStorage === 'undefined') {
    return { enabled: false, baseUrl: '', bearerToken: '', pollIntervalMs: DEFAULT_INTERVAL }
  }
  const intervalRaw = localStorage.getItem(LS_INTERVAL)
  const interval = intervalRaw ? Number(intervalRaw) : DEFAULT_INTERVAL
  return {
    enabled: localStorage.getItem(LS_ENABLED) === '1',
    baseUrl: localStorage.getItem(LS_BASE) ?? '',
    bearerToken: localStorage.getItem(LS_TOKEN) ?? '',
    pollIntervalMs: Number.isFinite(interval) && interval >= 1000 ? interval : DEFAULT_INTERVAL,
  }
}

export function saveLiveSource(cfg: LiveSourceConfig) {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(LS_BASE, cfg.baseUrl)
  localStorage.setItem(LS_TOKEN, cfg.bearerToken)
  localStorage.setItem(LS_ENABLED, cfg.enabled ? '1' : '0')
  localStorage.setItem(LS_INTERVAL, String(cfg.pollIntervalMs))
}

export function joinUrl(base: string, name: string): string {
  if (!base) return name
  return base.endsWith('/') ? base + name : `${base}/${name}`
}

export function authHeaders(token: string): HeadersInit {
  return token ? { Authorization: `Bearer ${token}` } : {}
}
