/** Max automated SSH-style reconnect bursts before pruning a VPS from fleet. */
export const VPS_MAX_RECONNECT_TRIES = 3

export type RunSnapshot = {
  id: string
  label: string
  startedAt: string
  snapshots: string[]
  targetLiveDomains: number
  liveDomains: number
  totalExtracted: number
  totalTested: number
  filesProcessed: number
  filesTotal: number
  elapsedSeconds: number
  outputFile?: string
  extractWorkers?: number
  testWorkers?: number
}

export type VpsStatus =
  | 'healthy'
  | 'degraded'
  | 'reconnecting'
  | 'offline'
  | 'removed'

export type VpsAuthType = 'key' | 'password'

export type VpsNodeSource = 'seed' | 'discovered'

export type VpsNode = {
  id: string
  label: string
  host: string
  region: string
  status: VpsStatus
  /** Seed fleet vs auto-enrolled from a scan hit. */
  source?: VpsNodeSource
  /** Finding that produced this node (discovered only). */
  discoveredFromFindingId?: string
  authType?: VpsAuthType
  cpuPercent: number
  /** Last N CPU % samples (newest at end) for mini sparkline */
  cpuHistory?: readonly number[]
  ramUsedGb: number
  ramTotalGb: number
  diskUsedGb: number
  diskTotalGb?: number
  targetsAssigned: number
  targetsDone: number
  scansPerSecond: number
  /** Failed SSH/control reconnect attempts toward removal */
  reconnectFailCount: number
  /** Cumulative findings contributed by this node. */
  findingsContributed?: number
  /** Minutes since the node last booted into the fleet. */
  uptimeMin?: number
  lastEvent: string
  removedReason?: string
  /** Target list actively being scanned (cleared when shard finishes). */
  activeListId?: string
  activeListName?: string
}

export type FindingSeverity = 'low' | 'medium' | 'high' | 'critical'

export type StripeAccountInfo = {
  livemode?: boolean
  balance?: number
  pendingBalance?: number
  currency?: string
  country?: string
  chargesEnabled?: boolean
  payoutsEnabled?: boolean
  email?: string
}

export type SesQuotaInfo = {
  /** Daily send quota allotted by SES. */
  max24h?: number
  sent24h?: number
  ratePerSecond?: number
  verifiedDomains?: string[]
  sandbox?: boolean
}

export type GithubInfo = {
  user?: string
  scopes?: string[]
  rateLimit?: { remaining: number; limit: number }
  repos?: number
  privateRepos?: number
  twoFactor?: boolean
}

export type TwilioInfo = {
  sid: string
  status?: string
  balance?: number
  currency?: string
  numbers?: number
}

export type SmtpInfo = {
  host?: string
  port?: number
  user?: string
  authMethod?: string
}

export type FindingDetails = {
  /** Whether the credential passed live API validation. */
  validated?: boolean
  /** Unmasked credential — kept in-memory only, never persisted. */
  raw?: string
  /** Inline labelled key/value extras for anything else. */
  extra?: ReadonlyArray<{ key: string; value: string }>

  // AWS family
  awsRegion?: string
  awsServices?: ReadonlyArray<string>
  sesQuota?: SesQuotaInfo

  // Stripe
  stripe?: StripeAccountInfo

  // GitHub
  github?: GithubInfo

  // OpenAI / Anthropic
  modelsAvailable?: number
  modelExamples?: ReadonlyArray<string>

  // SendGrid / Mailgun / Brevo / Postmark
  senderDomains?: ReadonlyArray<string>
  monthlyCredits?: number
  sentLast30d?: number

  // SMTP creds
  smtp?: SmtpInfo

  // Twilio
  twilio?: TwilioInfo
}

export type Finding = {
  id: string
  at: string
  provider: string
  ruleLabel: string
  /** Host of the source URL (no scheme/path). */
  hostname: string
  /** Full source URL where the credential was found, when known. */
  url?: string
  /** Pathname of the source URL — used for the Paths analytics. */
  path?: string
  /** Masked credential or summary for the row's main cell. */
  detail: string
  /** Provider-specific rich metadata shown when the row is expanded. */
  details?: FindingDetails
  severity: FindingSeverity
  reportedByHost: string
  /** Which scan this finding belongs to (when known). */
  scanId?: string
}

export type PathCategory = 'env' | 'config' | 'backup' | 'debug' | 'wp' | 'api' | 'cloud' | 'misc'

export type PathEntry = {
  path: string
  category: PathCategory
  /** Aggregate hits attributed to this path. */
  hits: number
  /** Whether the scanner will probe this path. */
  enabled: boolean
}

export type ListStatus = 'idle' | 'queued' | 'deployed' | 'completed' | 'failed'

/** A user-managed target list. The full body is held in memory while the tab session is alive;
 *  metadata + content hash + first few preview lines persist to localStorage. */
export type TargetList = {
  id: string
  name: string
  uploadedAt: string
  /** Total non-empty line count. */
  lineCount: number
  /** Raw file size in bytes (from File.size at upload time). */
  fileSize?: number
  /** SHA-1-ish content hash for dedup. */
  contentHash: string
  /** First few lines for the card preview. */
  preview: ReadonlyArray<string>
  /** VPS node IDs this list deploys to (subset of fleet). Empty = unassigned. */
  assignedVpsIds: ReadonlyArray<string>
  /** Workflow state. */
  status: ListStatus
  /** Free-form note. */
  note?: string
}

export type ScanStatus = 'queued' | 'running' | 'paused' | 'done' | 'failed'

/** A logical scan job (target list × scanner config), fanned out across one or more VPS shards. */
export type Scan = {
  id: string
  label: string
  status: ScanStatus
  startedAt: string
  endedAt?: string
  /** Total target lines (across all shards). */
  targetCount: number
  validHosts: number
  invalidHosts: number
  hitsFound: number
  validHits: number
  parsingPerSec: number
  requestsPerSec: number
  /** Recent rps samples for the spark trend (newest at end). */
  rpsHistory: readonly number[]
  /** Source WARC snapshots feeding this scan, when known. */
  snapshots: readonly string[]
  /** Fleet node IDs this scan is fanned across. */
  shardVpsIds: readonly string[]
  /** Recent narrative event for the card. */
  lastEvent: string
}

/** One VPS's slice of a Scan. */
export type ScanShard = {
  scanId: string
  vpsId: string
  /** Lines assigned to this shard. */
  assigned: number
  done: number
  /** Per-shard stats. */
  validHosts: number
  invalidHosts: number
  hits: number
  parsingPerSec: number
  requestsPerSec: number
}

export function parseRunSnapshot(raw: unknown): RunSnapshot | null {
  if (!raw || typeof raw !== 'object') return null
  const o = raw as Record<string, unknown>

  const str = (k: string, fallback = '') =>
    typeof o[k] === 'string' ? o[k] : fallback
  const num = (k: string) =>
    typeof o[k] === 'number' && Number.isFinite(o[k] as number)
      ? (o[k] as number)
      : NaN
  const strArr = (k: string) =>
    Array.isArray(o[k]) && (o[k] as unknown[]).every((x) => typeof x === 'string')
      ? (o[k] as string[])
      : null

  const snapshots = strArr('snapshots')
  const targetLiveDomains = num('targetLiveDomains')
  const liveDomains = num('liveDomains')
  const totalExtracted = num('totalExtracted')
  const totalTested = num('totalTested')
  const filesProcessed = num('filesProcessed')
  const filesTotal = num('filesTotal')
  const elapsedSeconds = num('elapsedSeconds')

  if (
    !snapshots ||
    [targetLiveDomains, liveDomains, totalExtracted, totalTested, filesProcessed, filesTotal, elapsedSeconds].some(
      (n) => Number.isNaN(n) || n < 0,
    )
  ) {
    return null
  }

  return {
    id: str('id', `run-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`),
    label: str('label', 'Imported run'),
    startedAt:
      str('startedAt').length > 0 ? str('startedAt') : new Date().toISOString(),
    snapshots,
    targetLiveDomains,
    liveDomains,
    totalExtracted,
    totalTested,
    filesProcessed,
    filesTotal,
    elapsedSeconds,
    outputFile: typeof o.outputFile === 'string' ? o.outputFile : undefined,
    extractWorkers:
      typeof o.extractWorkers === 'number' ? o.extractWorkers : undefined,
    testWorkers: typeof o.testWorkers === 'number' ? o.testWorkers : undefined,
  }
}

export function jitter(n: number, span: number): number {
  return Math.round(Math.min(99, Math.max(0, n + (Math.random() - 0.5) * span * 2)))
}
