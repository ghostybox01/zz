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
  status: string,
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
  // Live machine metrics from the probe — populated when the box is
  // reachable. Older backends without these fields will yield undefined
  // and the UI falls back to zero.
  cpu_percent?: number
  ram_used_gb?: number
  ram_total_gb?: number
  disk_used_gb?: number
  disk_total_gb?: number
  sys_uptime_sec?: number
  last_good_update?: string
  /** Overlaid by backend when a crack session is actively running on this worker. */
  active_list_name?: string
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
  /** Backend-assigned id under the per-list store (`lists/<id>.txt`).
   * Sent back to /api/crack/start so the resolver knows which file to
   * ship. null only on legacy backends that haven't been upgraded. */
  list_id?: string | null
  error?: string
}

/** A list as the controller actually has it on disk — the source of
 *  truth that supersedes the dashboard's localStorage cache. */
export type ReconServerList = {
  id: string
  name: string
  lines: number
  size: number
  uploaded_at: string
  /** False when the sidecar metadata survives but the data file is gone
   *  (rsync wipe, manual rm, etc.). UI should treat as unusable. */
  present: boolean
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
    // Surface the backend's own error message when present (e.g. the
    // /api/crack/start "target list not found" 404). The old format
    // showed only "Recon API 404 /crack/start", which hid the actual
    // reason and looked like a missing route.
    const detail = ReconApiError.extractDetail(payload)
    super(detail ? `${detail} (${status} ${path})` : `Recon API ${status} ${path}`)
    this.status = status
    this.path = path
    this.payload = payload
  }

  private static extractDetail(payload: unknown): string {
    if (!payload) return ''
    if (typeof payload === 'string') return payload.slice(0, 200)
    if (typeof payload === 'object') {
      const p = payload as { error?: unknown; message?: unknown; detail?: unknown }
      const msg = p.error ?? p.message ?? p.detail
      if (typeof msg === 'string' && msg.length > 0) return msg.slice(0, 200)
    }
    return ''
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

/* ── Per-list server inventory (sidesteps the localStorage lie) ──── */
/*  Returns what the controller can actually resolve via list_id when
 *  /api/crack/start runs. Use this on dashboard mount to reconcile the
 *  composer dropdown with disk truth instead of trusting localStorage. */

export const lists = {
  list:   () => getJson<{ lists: ReconServerList[] }>('/lists'),
  remove: async (id: string): Promise<{ ok: boolean; error?: string }> => {
    const res = await fetch(`${BASE}/lists/${encodeURIComponent(id)}`, { method: 'DELETE' })
    let body: unknown
    try { body = await res.json() } catch { body = {} }
    return body as { ok: boolean; error?: string }
  },
  create: (name: string, lines: string[]) =>
    postJson<{ ok: boolean; id: string; name: string; lines: number }>('/lists/create', { name, lines }),
  clearAll: () => postJson<{ ok: boolean; deleted: number }>('/lists/clear-all', {}),
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
    crypto_wallet: boolean
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
export type R2Usage = {
  error: string | null
  total_bytes: number
  /** Bytes counted toward the cap. Hits are excluded per operator policy. */
  counted_bytes: number
  bytes_by: { warc: number; uploads: number; hits: number; other: number }
  count_by: { warc: number; uploads: number; hits: number; other: number }
  limit_bytes: number
  percent: number
  threshold_75_hit: boolean
  threshold_95_hit: boolean
}

export type R2HealthState = 'connected' | 'misconfigured' | 'unreachable' | 'unknown'

/** One row in the canonical multi-account R2 config. The dashboard
 *  surfaces these as separate cards in R2Settings; the backend's
 *  priority picker walks them in array order looking for the first
 *  account whose `counted_bytes < 0.95 * limit_bytes`. */
export type R2Account = {
  id: string
  label: string
  account_id: string
  access_key_id: string
  /** Masked (`●●●●●●●●`) on read so we never echo real secrets back to
   *  the browser. Submit empty to keep the existing value; non-empty to
   *  rotate. */
  secret_access_key: string
  bucket_name: string
  priority: number
  /** Per-account soft cap in GB. Default 9.5 (Cloudflare R2 free-tier). */
  max_gb: number
  configured: boolean
  state?: R2HealthState
  last_error?: string | null
  usage?: R2Usage | null
}

export type R2Config = {
  /** Canonical multi-account list. Always present in new backends. */
  accounts: R2Account[]
  /** Id of the account the picker will route the next upload to. May be
   *  null when no account is configured. */
  primary_id: string | null
  /** True when every configured account is at or past 95% usage — the
   *  caller can still proceed but should warn the operator that the
   *  next upload is going into a near-full bucket. */
  all_full: boolean
  /* ── Legacy single-account mirrors (account #0). Kept so existing
   *    code paths that read these fields keep compiling during the
   *    cutover. New code should read `accounts[]`. ─────────────────── */
  account_id: string
  access_key_id: string
  secret_access_key: string
  bucket_name: string
  configured: boolean
  state?: R2HealthState
  last_error?: string | null
  usage?: R2Usage | null
}

export type R2Object = {
  key: string
  size: number
  modified: string | null
  storage_class?: string | null
  /** Which account this row came from. Stamped by the backend on the
   *  multi-account list endpoint so the UI can badge the source bucket. */
  account_id?: string
  account_label?: string
}

/** Per-account write payload. Mirrors R2Account minus the server-only
 *  computed fields (configured/state/last_error/usage). The `id` field
 *  is optional on create — the backend will mint one. */
export type R2AccountInput = {
  id?: string
  label: string
  account_id: string
  access_key_id: string
  secret_access_key: string
  bucket_name: string
  max_gb: number
}

export const r2 = {
  getConfig: () => getJson<R2Config>('/upload/r2-config'),
  /** Replace the full account list. Per-account blank secret means
   *  "keep existing for this id". */
  saveAccounts: (accounts: R2AccountInput[]) =>
    postJson<R2Config>('/upload/r2-config', { accounts }),
  /** Legacy single-account save — kept for the original form path. */
  saveConfig: (cfg: Omit<R2Account, 'id' | 'configured' | 'priority' | 'max_gb' | 'label'> & Partial<Pick<R2Account, 'label' | 'max_gb'>>) =>
    postJson<R2Config>('/upload/r2-config', cfg),
  deleteAccount: (id: string) =>
    getJson<{ ok: boolean; deleted_id?: string; remaining?: number; error?: string }>(
      `/upload/r2-config/${encodeURIComponent(id)}`,
      { method: 'DELETE' },
    ),
  reorderAccount: (id: string, priority: number) =>
    postJson<{ ok: boolean; id: string; priority: number; error?: string }>(
      `/upload/r2-config/${encodeURIComponent(id)}/reorder`,
      { priority },
    ),
  presign: (filename: string) =>
    getJson<{ url: string; key: string; upload_id: string; account_id?: string; account_label?: string }>(
      `/upload/presign?filename=${encodeURIComponent(filename)}`,
    ),
  complete: (key: string, filename: string, accountId?: string) =>
    postJson<{ success: boolean; targets: number; preview: string[]; filename: string; list_id?: string | null }>(
      '/upload/complete', { key, filename, account_id: accountId },
    ),
  /** List objects across one or all accounts. Optional prefix filter.
   *  Without `accountId` the backend unions across every connected
   *  account and tags each row with its origin. */
  listObjects: (prefix?: string, limit = 100, accountId?: string) => {
    const params = new URLSearchParams()
    if (prefix) params.set('prefix', prefix)
    params.set('limit', String(limit))
    if (accountId) params.set('account', accountId)
    return getJson<{
      ok: boolean
      bucket?: string
      prefix?: string
      objects: R2Object[]
      accounts?: Array<{ id: string; label: string; state: string; error?: string | null }>
      warnings?: string[]
      error?: string
    }>(`/r2/objects?${params.toString()}`)
  },
  /** Install the dashboard's CORS rule. Without `accountId` installs on
   *  every connected account in one batch. */
  setupCors: (accountId?: string) => {
    const qs = accountId ? `?account=${encodeURIComponent(accountId)}` : ''
    return postJson<{
      ok: boolean
      results?: Array<{ id: string; label: string; ok: boolean; error?: string }>
      rules?: unknown
      error?: string
    }>(`/r2/cors-setup${qs}`, {})
  },
  /** Delete one object by exact key. Optional `accountId` targets one
   *  bucket; without it the backend HEADs across accounts and deletes
   *  in whichever one holds the key. */
  deleteObject: async (key: string, accountId?: string): Promise<{ ok: boolean; deleted?: string; error?: string }> => {
    const params = new URLSearchParams({ key })
    if (accountId) params.set('account', accountId)
    const res = await fetch(`${BASE}/r2/object?${params.toString()}`, { method: 'DELETE' })
    if (!res.ok) {
      let body: { error?: string } = {}
      try { body = (await res.json()) as { error?: string } } catch { /* ignore */ }
      return { ok: false, error: body.error ?? `HTTP ${res.status}` }
    }
    return (await res.json()) as { ok: boolean; deleted?: string }
  },
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
  /** Which R2 account absorbed the last upload. Stamped by the backend
   *  on every export path so the cockpit can label the destination
   *  bucket. */
  r2_account_id?: string | null
  r2_account_label?: string | null
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
  exportToR2: () => postJson<{
    success: boolean
    r2_key: string
    /** When `noop` is true the upload was short-circuited — same content
     *  already exists at the named key. */
    noop?: boolean
    message?: string
    sha256?: string
    /** Which R2 account absorbed the upload. Populated on both success
     *  and noop paths. */
    account_id?: string | null
    account_label?: string | null
  }>('/warc/export-to-r2', {}),
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

/* ── Crack sessions (Contract B — HMS Iris owns the backend) ─────────── */

export type CrackSession = {
  id: string
  name: string
  list_id: string
  list_name: string
  addon_ids: string[]
  worker_ips: string[]
  created_at: string
  status: 'queued' | 'running' | 'completed' | 'stopped' | 'failed'
  remote_pids?: Record<string, number>
  finished_at?: string
  last_error?: string
  /** Lines scanned so far (from last_progress on the backend). */
  scanned?: number
  /** Current scan speed in lines/sec. */
  speed?: number
  /** Total targets in the list (may be 0 until first /crack/sessions fetch after session start). */
  targets?: number
  /** Total credential hits (valid + unvalidated) for this session. */
  hits?: number
  /** Confirmed-valid credential hits for this session. */
  valid_hits?: number
}

export const crack = {
  start: (body: { session_name: string; list_id: string; addon_ids: string[]; worker_ips: string[] }) =>
    postJson<{ ok: boolean; session: CrackSession; error?: string }>('/crack/start', body),
  list: () => getJson<{ sessions: CrackSession[] }>('/crack/sessions'),
  stop: (id: string) => postJson<{ ok: boolean }>(`/crack/${encodeURIComponent(id)}/stop`),
  reattach: (id: string) => postJson<{ ok: boolean; found_pids?: Record<string, number>; error?: string }>(`/crack/${encodeURIComponent(id)}/reattach`),
  remove: async (id: string): Promise<{ ok: boolean; error?: string }> => {
    const res = await fetch(`${BASE}/crack/${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) {
      let body: { error?: string } = {}
      try { body = (await res.json()) as { error?: string } } catch { /* ignore */ }
      return { ok: false, error: body.error ?? `HTTP ${res.status}` }
    }
    return (await res.json()) as { ok: boolean }
  },
}

export { ReconApiError }

/* ── Dorks — AI generator + Shodan/FOFA search ───────────────────── */

export type DorkResult = {
  id: string
  host: string
  ip: string
  port: number
  protocol: string
  hostname: string
  title: string
  data: string
  platform: 'shodan' | 'fofa'
}

export type SavedDork = {
  id: string
  query: string
  category: string
  platform: 'shodan' | 'fofa' | 'both' | 'google'
  notes: string
  createdAt: string
  runs?: number
  hits?: number
  last_run_hits?: number
  score?: number
}

export type GeneratedDork = {
  query: string
  notes: string
}

export const dorks = {
  getKeys: () => getJson<{ shodan_key: string; fofa_email: string; fofa_key: string; anthropic_key: string; openai_key: string }>('/dorks/keys'),
  saveKeys: (body: { shodan_key?: string; fofa_email?: string; fofa_key?: string; anthropic_key?: string; openai_key?: string }) =>
    postJson<{ ok: boolean }>('/dorks/keys', body),
  generate: (body: { objective: string; platform: string; count: number; category: string }) =>
    postJson<{ ok: boolean; dorks: GeneratedDork[]; source: 'ai' | 'template' }>('/dorks/generate', body),
  run: (body: { query: string; platform: string; limit: number }) =>
    postJson<{ ok: boolean; results: DorkResult[]; total: number }>('/dorks/run', body),
  listSaved: () => getJson<{ dorks: SavedDork[] }>('/dorks/saved'),
  seedLibrary: () => postJson<{ ok: boolean; added: number; total: number; dorks: SavedDork[] }>('/dorks/seed-library'),
  save: (body: { query: string; category: string; platform: string; notes: string }) =>
    postJson<{ ok: boolean; dork: SavedDork }>('/dorks/saved', body),
  deleteSaved: async (id: string): Promise<{ ok: boolean }> => {
    const res = await fetch(`${BASE}/dorks/saved/${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) {
      let body: unknown
      try { body = await res.json() } catch { body = await res.text().catch(() => '') }
      throw new ReconApiError(res.status, `/dorks/saved/${id}`, body)
    }
    return (await res.json()) as { ok: boolean }
  },
  scoreRun: (id: string, resultCount: number) =>
    postJson<{ ok: boolean; updated: boolean }>(`/dorks/saved/${encodeURIComponent(id)}/score`, { result_count: resultCount }),
  evolve: (body: { query: string; category: string; platform: string; count?: number }) =>
    postJson<{ ok: boolean; dorks: GeneratedDork[]; source: 'ai' | 'none' }>('/dorks/evolve', body),
}

export type CryptoBalanceResult = {
  ok: boolean
  address?: string
  chain?: string
  balance_native?: number
  symbol?: string
  balance_usd?: number | null
  explorer_url?: string
  source?: string
  error?: string
}

export const crypto = {
  verifyBalance: (address: string, chain: 'eth' | 'btc' | 'bnb') =>
    postJson<CryptoBalanceResult>('/crypto/verify-balance', { address, chain }),
}

/* ── Findings panels (Stripe + Crypto admin) ─────────────────────── */

export type DiscoveredKey = {
  id: number
  type: string
  key_value: string
  source_url: string
  status: string
  metadata: string
  timestamp: string
  reported: boolean
  last_verified: string | null
  verify_meta: string | null
}

export type StripeRefreshResult = {
  ok: boolean
  live: boolean
  livemode?: boolean
  available?: Array<{ amount: number; currency: string }>
  pending?: Array<{ amount: number; currency: string }>
  status?: number
  error?: string
}

export const findings = {
  listStripe: () =>
    getJson<{ ok: boolean; findings: DiscoveredKey[] }>('/findings/stripe'),
  listCrypto: () =>
    getJson<{ ok: boolean; findings: DiscoveredKey[] }>('/findings/crypto'),
  refreshStripe: (id: number) =>
    postJson<StripeRefreshResult>(`/findings/stripe/${id}/refresh`, {}),
  refreshCrypto: (id: number, address?: string, chain?: 'eth' | 'btc' | 'bnb') =>
    postJson<CryptoBalanceResult>(`/findings/crypto/${id}/refresh`, address ? { address, chain: chain ?? 'eth' } : {}),
  markReported: (id: number, reported: boolean) =>
    postJson<{ ok: boolean; reported: boolean }>(`/findings/${id}/report`, { reported }),
  remove: async (id: number): Promise<{ ok: boolean; deleted: number }> => {
    const res = await fetch(`${BASE}/findings/${id}`, { method: 'DELETE' })
    if (!res.ok) {
      let body: unknown
      try { body = await res.json() } catch { body = await res.text().catch(() => '') }
      throw new ReconApiError(res.status, `/findings/${id}`, body)
    }
    return (await res.json()) as { ok: boolean; deleted: number }
  },
}
