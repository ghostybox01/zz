/** Fleet control plane — real SSH enroll/deploy when a backend URL is configured. */

const LS_BASE = 'scan-cockpit-fleet-base'
const LS_TOKEN = 'scan-cockpit-fleet-token'
const LS_ENABLED = 'scan-cockpit-fleet-enabled'
const LS_AUTO = 'scan-cockpit-fleet-auto-enroll'

export type FleetControlConfig = {
  enabled: boolean
  /** e.g. http://127.0.0.1:8787 */
  baseUrl: string
  bearerToken: string
  /** Auto-enroll SSH/VPS hits into fleet while scanning. */
  autoEnroll: boolean
}

const DEFAULT: FleetControlConfig = {
  enabled: false,
  baseUrl: 'http://127.0.0.1:8787',
  bearerToken: '',
  autoEnroll: true,
}

export function loadFleetControl(): FleetControlConfig {
  if (typeof localStorage === 'undefined') return { ...DEFAULT }
  return {
    enabled: localStorage.getItem(LS_ENABLED) === '1',
    baseUrl: localStorage.getItem(LS_BASE) ?? DEFAULT.baseUrl,
    bearerToken: localStorage.getItem(LS_TOKEN) ?? '',
    autoEnroll: localStorage.getItem(LS_AUTO) !== '0',
  }
}

export function saveFleetControl(cfg: FleetControlConfig): void {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(LS_ENABLED, cfg.enabled ? '1' : '0')
  localStorage.setItem(LS_BASE, cfg.baseUrl)
  localStorage.setItem(LS_TOKEN, cfg.bearerToken)
  localStorage.setItem(LS_AUTO, cfg.autoEnroll ? '1' : '0')
}

function headers(token: string): HeadersInit {
  const h: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) h.Authorization = `Bearer ${token}`
  return h
}

export type EnrollResult =
  | { ok: true; message: string; hostname?: string; region?: string }
  | { ok: false; message: string }

export async function enrollSshViaApi(
  cfg: FleetControlConfig,
  body: {
    host: string
    port: number
    user: string
    secret: string
    authType: 'key' | 'password'
    vpsId: string
  },
): Promise<EnrollResult> {
  if (!cfg.enabled || !cfg.baseUrl) {
    return { ok: true, message: 'Simulated SSH handshake (no control plane)' }
  }
  const url = cfg.baseUrl.replace(/\/$/, '') + '/api/fleet/enroll'
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: headers(cfg.bearerToken),
      body: JSON.stringify(body),
    })
    const data = (await res.json()) as { ok?: boolean; message?: string; hostname?: string; region?: string }
    if (!res.ok || !data.ok) {
      return { ok: false, message: data.message ?? `HTTP ${res.status}` }
    }
    return {
      ok: true,
      message: data.message ?? 'SSH session established',
      hostname: data.hostname,
      region: data.region,
    }
  } catch (err) {
    return { ok: false, message: (err as Error).message }
  }
}

export async function deployListViaApi(
  cfg: FleetControlConfig,
  body: {
    vpsId: string
    host: string
    port: number
    user: string
    secret: string
    authType: 'key' | 'password'
    listName: string
    targets: string
  },
): Promise<{ ok: boolean; message: string }> {
  if (!cfg.enabled || !cfg.baseUrl) {
    return { ok: true, message: 'Simulated deploy (no control plane)' }
  }
  const url = cfg.baseUrl.replace(/\/$/, '') + '/api/fleet/deploy'
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: headers(cfg.bearerToken),
      body: JSON.stringify(body),
    })
    const data = (await res.json()) as { ok?: boolean; message?: string }
    if (!res.ok || !data.ok) {
      return { ok: false, message: data.message ?? `HTTP ${res.status}` }
    }
    return { ok: true, message: data.message ?? 'Deploy queued' }
  } catch (err) {
    return { ok: false, message: (err as Error).message }
  }
}
