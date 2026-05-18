/** Typed client for the Raven Flask backend (app.py).
 *  All endpoints documented at /tmp/raven-unzip/raven/app.py.
 *
 *  Auth model: Flask runs locally / on the operator's box; no token required out-of-the-box.
 *  If you later put nginx + bearer in front, override `setReconBase()` and add header injection.
 */

let BASE = '/api'

export function setReconBase(prefix: string): void {
  BASE = prefix.replace(/\/$/, '')
}

export function getReconBase(): string {
  return BASE
}

/* ── Shared types matching app.py response shapes ─────────────────── */

export type ReconRecentFinding = readonly [
  type: string,
  keyValue: string,
  sourceUrl: string,
  timestamp: string,
  metadata: string | null,
]

export type ReconStats = {
  total_urls: number
  total_hits: number
  total_valid: number
  smtp_servers: number
  type_counts: Record<string, number>
  recent_findings: ReconRecentFinding[]
  last_update: string
  progress_current: number
  progress_total: number
  progress_percent: number
  scan_rate: number
}

export type ReconServerStatus = {
  ip: string
  status: string
  scanned: number
  targets: number
  hits: number
  speed: number
  uptime: string
  batch_info: string
  batches_done: number
  batches_total: number
  current_batch_progress: number
  last_update: string
  error: string | null
}

export type ReconGlobalStats = {
  total_servers?: number
  online?: number
  offline?: number
  total_scanned?: number
  total_hits?: number
  total_speed?: number
  [k: string]: unknown
}

export type ReconFleetStatus = {
  servers: ReconServerStatus[]
  stats: ReconGlobalStats
}

export type ReconActionResult = {
  success: boolean
  message?: string
  [k: string]: unknown
}

export type ReconUploadResult = {
  success: boolean
  filename: string
  targets: number
  error?: string
}

export type ReconDeployResult = {
  success?: boolean
  ip?: string
  targets_assigned?: number
  message?: string
  steps?: string[]
  [k: string]: unknown
}

export type ReconConfig = {
  ssh_key_path?: string
  remote_user?: string
  work_dir?: string
  batch_size?: number
  target_file?: string
  ssh_timeout?: number
  [k: string]: unknown
}

/* ── Fetch wrappers ───────────────────────────────────────────────── */

class ReconApiError extends Error {
  status: number
  path: string
  payload: unknown
  constructor(status: number, path: string, payload: unknown) {
    super(`Recon API ${status} ${path}`)
    this.status = status
    this.path = path
    this.payload = payload
  }
}

async function getJson<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: { Accept: 'application/json', ...(init?.headers ?? {}) },
  })
  if (!res.ok) {
    let body: unknown
    try { body = await res.json() } catch { body = await res.text().catch(() => '') }
    throw new ReconApiError(res.status, path, body)
  }
  return (await res.json()) as T
}

async function postJson<T>(path: string, body?: unknown): Promise<T> {
  return getJson<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
}

async function putJson<T>(path: string, body?: unknown): Promise<T> {
  return getJson<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
}

/* ── Stats / clear ────────────────────────────────────────────────── */

export const stats = {
  get: () => getJson<ReconStats>('/stats'),
  clear: () => postJson<ReconActionResult>('/clear'),
}

/* ── VPS roster + status (read-only display) ──────────────────────── */

export const vps = {
  available:    () => getJson<{ available: boolean }>('/vps/available'),
  config:       () => getJson<ReconConfig>('/vps/config'),
  updateConfig: (c: Partial<ReconConfig>) => postJson<{ success: boolean; config: ReconConfig }>('/vps/config', c),

  /** Read roster from `server_ips.txt` on the backend. */
  servers:        () => getJson<{ servers: string[] }>('/vps/servers'),
  /** Replace the full roster list. Use this for explicit, operator-driven roster edits. */
  saveServers:    (servers: string[]) => postJson<{ success: boolean; count: number }>('/vps/servers', { servers }),
  /** Append one IP that the operator has explicitly typed in. */
  addServerByIp:  (ip: string) => putJson<{ success: boolean; servers: string[] }>('/vps/servers', { ip }),

  status:    () => getJson<ReconFleetStatus>('/vps/status'),
  serverStatus: (ip: string) => getJson<ReconServerStatus>(`/vps/server/${encodeURIComponent(ip)}/status`),

  /* ── Per-server control (operator action against a rostered IP) ── */
  test:    (ip: string) => getJson<ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/test`),
  start:   (ip: string) => postJson<ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/start`),
  stop:    (ip: string) => postJson<ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/stop`),
  restart: (ip: string) => postJson<ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/restart`),
  logs:    (ip: string, lines = 50) => getJson<{ logs: string[] } & ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/logs?lines=${lines}`),
  diagnose:(ip: string) => getJson<ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/diagnose`),
  fix:     (ip: string) => postJson<ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/fix`),
  deployToServer: (ip: string, pkg?: string) => postJson<ReconDeployResult>(`/vps/server/${encodeURIComponent(ip)}/deploy`, { package: pkg }),
  collect: (ip: string) => postJson<ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/collect`),
  /**
   * Remove an IP from the rostered fleet (server_ips.txt). Backend may or
   * may not implement /api/vps/server/<ip>/remove — callers should fall
   * back to client-side state removal on 404 / network failure.
   */
  removeFromRoster: (ip: string) => postJson<ReconActionResult>(`/vps/server/${encodeURIComponent(ip)}/remove`),

  /** Rename a worker — persisted in fleet_creds.json. Routed via /api/fleet/worker/<ip>/label. */
  setLabel: (ip: string, label: string) =>
    postJson<{ ok: boolean; label: string }>(`/fleet/worker/${encodeURIComponent(ip)}/label`, { label }),
  /** Tag a worker role (scanner | warc). Routed via /api/fleet/worker/<ip>/role. */
  setRole: (ip: string, role: 'scanner' | 'warc') =>
    postJson<{ ok: boolean; role: 'scanner' | 'warc' }>(`/fleet/worker/${encodeURIComponent(ip)}/role`, { role }),

  /* ── Bulk ops ──────────────────────────────────────────────────── */
  startAll:   () => postJson<ReconActionResult>('/vps/start-all'),
  stopAll:    () => postJson<ReconActionResult>('/vps/stop-all'),
  restartAll: () => postJson<ReconActionResult>('/vps/restart-all'),
  deployAll:  (pkg?: string) => postJson<ReconActionResult>('/vps/deploy-all', { package: pkg }),
  collectAll: () => postJson<ReconActionResult>('/vps/collect-all'),
  testConnections: () => postJson<ReconActionResult>('/vps/test-connections'),
  testSsh:    () => postJson<ReconActionResult>('/vps/test-ssh'),

  /* ── Deploy / targets ─────────────────────────────────────────── */
  prepareDeploy: (targetFile?: string) => postJson<ReconActionResult>('/vps/prepare-deploy', { target_file: targetFile }),
  deploy: (opts: { targetFile?: string; scannerFile?: string; runnerFile?: string; autoStart?: boolean }) =>
    postJson<ReconActionResult>('/vps/deploy', {
      target_file:  opts.targetFile,
      scanner_file: opts.scannerFile,
      runner_file:  opts.runnerFile,
      auto_start:   opts.autoStart ?? false,
    }),

  /** Multipart upload of a target list .txt. Returns server-side filename + line count. */
  async uploadTargets(file: File): Promise<ReconUploadResult> {
    const form = new FormData()
    form.append('file', file)
    const res = await fetch(`${BASE}/vps/upload-targets`, { method: 'POST', body: form })
    if (!res.ok) {
      let body: unknown
      try { body = await res.json() } catch { body = await res.text().catch(() => '') }
      throw new ReconApiError(res.status, '/vps/upload-targets', body)
    }
    return (await res.json()) as ReconUploadResult
  },

  listFiles:   (dir = '.') => getJson<{ files: Array<{ name: string; path: string; size: number; lines?: number }> }>(`/vps/list-files?dir=${encodeURIComponent(dir)}`),
  selectFile:  (path: string) => postJson<ReconUploadResult>('/vps/select-file', { path }),

  // The chunked uploader lives in ListsPanel and posts the raw Blob
  // body directly with XHR so progress events are byte-accurate; there
  // is no JSON helper for /vps/upload-chunk on purpose.

  finalizeUpload: (uploadId: string, totalChunks: number, filename: string) =>
    postJson<ReconUploadResult>('/vps/finalize-upload', { upload_id: uploadId, total_chunks: totalChunks, filename }),
}

/* ── Scanner config (live flags consumed by main.go) ─────────────── */

export type ReconScannerConfig = {
  scanning_features: {
    aws_main_scan: boolean
    github_token_deep_scan: boolean
    smtp_credentials_scan: boolean
  }
  aws_checks: {
    ses_quota_check: boolean
    sns_limit_check: boolean
    fargate_limit_check: boolean
    federation_console_url: boolean
  }
  api_validation: {
    openai: boolean
    anthropic: boolean
    stripe: boolean
    gcp_api_key: boolean
    sendgrid: boolean
    mailgun: boolean
    twilio: boolean
    nexmo: boolean
    telnyx: boolean
    messagebird: boolean
    github: boolean
  }
  features: {
    brevo: boolean
    xsmtp: boolean
    mandrill: boolean
    mailersend: boolean
    new_mailgun: boolean
  }
  exploit_methods: {
    react2shell: boolean
    bypass_waf: boolean
    bypass_middleware: boolean
    lfi: boolean
    xxe: boolean
    ssrf: boolean
  }
}

export type ReconScannerConfigPatch = {
  [K in keyof ReconScannerConfig]?: Partial<ReconScannerConfig[K]>
}

export const scannerConfig = {
  get: () => getJson<ReconScannerConfig>('/scanner-config'),
  update: (patch: ReconScannerConfigPatch) => postJson<ReconScannerConfig>('/scanner-config', patch),
}

/* ── Path-list override (paths.txt consumed by main.go's loadEnvPaths) ── */

export type ReconScannerPaths = {
  present: boolean
  lines: number
  source: 'builtin' | 'paths.txt'
  error?: string
}

/* ── Telegram (bot_token / chat_id stored in raven/config.json) ───── */

export type ReconTelegramView = {
  has_token: boolean
  token_tail: string
  chat_id: string
}

export const telegramApi = {
  get: () => getJson<ReconTelegramView>('/telegram'),
  update: (patch: { bot_token?: string; chat_id?: string }) =>
    postJson<ReconTelegramView>('/telegram', patch),
  test: (text?: string) =>
    postJson<{ success: boolean; error?: string }>('/telegram/test', { text }),
}

export const scannerPaths = {
  get: () => getJson<ReconScannerPaths>('/scanner-paths'),
  clear: () => getJson<ReconScannerPaths>('/scanner-paths', { method: 'DELETE' }),
  async upload(file: File): Promise<ReconScannerPaths> {
    const form = new FormData()
    form.append('file', file)
    const res = await fetch(`${BASE}/scanner-paths`, { method: 'POST', body: form })
    if (!res.ok) {
      let body: unknown
      try { body = await res.json() } catch { body = await res.text().catch(() => '') }
      throw new ReconApiError(res.status, '/scanner-paths', body)
    }
    return (await res.json()) as ReconScannerPaths
  },
}

/* ── Fleet bulk creds (parse + paramiko-test) ─────────────────── */

export type BulkCredRow = {
  host: string
  port: number
  user: string
  auth_kind: 'key' | 'password' | string
  secret: string
}

export type BulkCredResult = {
  host: string
  port: number
  user: string
  ok: boolean
  message: string
}

export type BulkCredsResponse = {
  total: number
  ok: number
  failed: number
  results: BulkCredResult[]
  added_to_roster: number
}

export type InstallKeyResult = {
  host: string
  port: number
  user: string
  ok: boolean
  installed: boolean
  message: string
}

export type InstallKeysResponse = {
  total: number
  installed: number
  skipped: number
  failed: number
  results: InstallKeyResult[]
}

export const fleetBulkCreds = {
  testText: (text: string) =>
    postJson<BulkCredsResponse>('/fleet/bulk-creds', { text }),
  testRows: (creds: BulkCredRow[]) =>
    postJson<BulkCredsResponse>('/fleet/bulk-creds', { creds }),
  async testFile(file: File): Promise<BulkCredsResponse> {
    const form = new FormData()
    form.append('file', file)
    const res = await fetch(`${BASE}/fleet/bulk-creds`, { method: 'POST', body: form })
    if (!res.ok) {
      let body: unknown
      try { body = await res.json() } catch { body = await res.text().catch(() => '') }
      throw new ReconApiError(res.status, '/fleet/bulk-creds', body)
    }
    return (await res.json()) as BulkCredsResponse
  },
  installKeysText: (text: string) =>
    postJson<InstallKeysResponse>('/fleet/install-keys', { text }),
}

export const updater = {
  check: () => getJson<{ available: boolean; helper: string }>('/update'),
  trigger: () => postJson<{ started: boolean; message?: string; error?: string }>('/update'),
}

/* ── R2 upload ───────────────────────────────────────────────────────── */
export type R2Config = {
  account_id: string
  access_key_id: string
  secret_access_key: string
  bucket_name: string
  configured: boolean
  /** Cached health state from the backend's periodic head_bucket probe.
   *  Optional so a stale frontend cache survives a backend without the
   *  field set yet. */
  state?: 'connected' | 'misconfigured' | 'unreachable' | 'unknown'
  /** Real error string from the most recent probe — `null` when healthy
   *  or before the first probe completes. */
  last_error?: string | null
}

export const r2 = {
  getConfig: () => getJson<R2Config>('/upload/r2-config'),
  saveConfig: (cfg: Omit<R2Config, 'configured'>) => postJson<R2Config>('/upload/r2-config', cfg),
  presign: (filename: string) => getJson<{ url: string; key: string; upload_id: string }>(`/upload/presign?filename=${encodeURIComponent(filename)}`),
  complete: (key: string, filename: string) => postJson<{ success: boolean; targets: number; preview: string[]; filename: string }>('/upload/complete', { key, filename }),
}

/* ── WARC harvest control plane ──────────────────────────────────────── */
export type WarcStatus = {
  running: boolean
  pid: number | null
  run_id: string | null
  started_at: string | null
  finished_at: string | null
  max_domains: number | null
  domains_found: number
  last_exit_code: number | null
  r2_key: string | null
  r2_uploaded_at: string | null
  r2_error: string | null
  log_tail: string[]
  run_on: string | null
  remote_pid: number | null
}

export type WarcStartOptions = {
  max_domains?: number
  extract_workers?: number
  test_workers?: number
  verbose?: boolean
  run_on?: string
  /** Number of CC-MAIN snapshots to span. 0 = auto (1/<500k, 2/<1M, 3/>1M). */
  snapshots?: number
  /** Producer list — any of 'cc', 'crtsh'. Omit → backend defaults to ['cc']. */
  source?: string[]
  /** Comma-separated TLDs for crt.sh TLD pivot (e.g. 'com,net,io'). */
  crt_tld?: string
  /** Comma-separated registered domains for crt.sh domain pivot. */
  crt_domain?: string
  /** Drop FQDNs whose eTLD+1 equals themselves (apex/registered domains). */
  subdomain_only?: boolean
}

export const warc = {
  status: () => getJson<WarcStatus>('/warc/status'),
  start:  (opts: WarcStartOptions = {}) =>
    postJson<{ success: boolean; pid: number; run_id: string; max_domains: number }>('/warc/start', opts),
  stop:   () => postJson<{ success: boolean; message?: string }>('/warc/stop', {}),
  exportToR2: () => postJson<{ success: boolean; r2_key: string }>('/warc/export-to-r2', {}),
  hosts:  () => getJson<{ hosts: string[] }>('/warc/hosts'),
}

/* ── Controller SSH keypair ───────────────────────────────────────────── */
export type SSHKey = {
  exists: boolean
  pubkey: string
  fingerprint: string
  created_at: string | null
  error?: string
}

export const sshKey = {
  get: () => getJson<SSHKey>('/ssh-key'),
  regenerate: () =>
    postJson<{ ok: boolean; pubkey: string; fingerprint: string; message: string; error?: string }>(
      '/ssh-key/regenerate',
      {},
    ),
}

/* ── Logs ──────────────────────────────────────────────────────────── */
export const logs = {
  controller: (n = 200) => getJson<{ lines: string[]; error?: string }>(`/logs/controller?n=${n}`),
  worker: (ip: string, n = 200) => getJson<{ ip: string; lines: string[] }>(`/logs/worker/${encodeURIComponent(ip)}?n=${n}`),
  workersList: () => getJson<{ ips: string[] }>('/logs/workers'),
}

export { ReconApiError }
