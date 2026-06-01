/** Subscribes to the real backend for stats + recent findings.
 *  - Initial GET /api/stats on mount
 *  - Listens for `stats_update` socket events (pushed every 2s by background_file_monitor in app.py)
 *  - Falls back to 10s polling if the socket disconnects
 */
import { useEffect, useRef, useState } from 'react'
import { stats as statsApi, type ReconStats, type ReconRecentFinding } from '../lib/reconApi'
import { getReconSocket } from '../lib/reconSocket'
import type { Finding, FindingSeverity } from '../types'

export type ReconConnectionState = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'error'

export type UseReconStatsResult = {
  state: ReconConnectionState
  lastError: string | null
  /** Raw payload from the most recent /api/stats response or socket push. */
  raw: ReconStats | null
  /** Mapped to the dashboard's existing types. */
  findings: Finding[]
  /** Manual refresh, returns the new payload. */
  refresh: () => Promise<ReconStats | null>
}

const PROVIDER_SEVERITY: Record<string, FindingSeverity> = {
  AWS: 'critical',
  Stripe: 'critical',
  OpenAI: 'high',
  Anthropic: 'high',
  SendGrid: 'high',
  Mailgun: 'high',
  Twilio: 'high',
  Brevo: 'medium',
  Mandrill: 'medium',
  MailerSend: 'medium',
  Nexmo: 'medium',
  Telnyx: 'medium',
  MessageBird: 'medium',
  SMTP: 'high',
}

function severityFor(provider: string): FindingSeverity {
  return PROVIDER_SEVERITY[provider] ?? 'medium'
}

function hostnameFromUrl(url: string): string {
  if (!url) return ''
  try {
    return new URL(url).host
  } catch {
    const m = url.match(/^https?:\/\/([^/?#]+)/)
    return m && m[1] ? m[1] : url
  }
}

function pathFromUrl(url: string): string | undefined {
  if (!url) return undefined
  try {
    return new URL(url).pathname
  } catch {
    // For bare "domain/path" strings without scheme
    const slash = url.indexOf('/')
    return slash > 0 ? url.slice(slash) : undefined
  }
}

/** Classify how the credential was discovered based on the source URL path. */
function discoveryMethod(sourceUrl: string): string {
  if (!sourceUrl) return 'page'
  const u = sourceUrl.toLowerCase()
  if (u.includes('.env')) return '.env'
  if (u.includes('phpinfo')) return 'phpinfo'
  if (u.includes('.git')) return '.git'
  if (u.includes('wp-config')) return 'wp-config'
  if (u.includes('config.php') || u.includes('config.yml') || u.includes('config.yaml') || u.includes('config.json')) return 'config'
  if (u.includes('settings.py') || u.includes('settings.php') || u.includes('appsettings')) return 'settings'
  if (u.includes('.xml') && (u.includes('web') || u.includes('app'))) return 'xml-config'
  if (u.includes('docker-compose') || u.includes('dockerfile')) return 'docker'
  if (u.includes('application.properties') || u.includes('application.yml')) return 'spring'
  if (u.includes('database.php') || u.includes('db.php')) return 'db-config'
  if (u.includes('backup') || u.includes('.bak') || u.includes('.sql')) return 'backup'
  if (u.includes('debug') || u.includes('actuator') || u.includes('swagger')) return 'debug'
  if (u.includes('/api/') || u.includes('/v1/') || u.includes('/v2/')) return 'api-endpoint'
  if (u.includes('package.json') || u.includes('composer.json') || u.includes('requirements.txt')) return 'package'
  if (u.includes('.log')) return 'log'
  // If URL looks like just a domain (no path) or is an AKIA key → page scrape
  if (!u.includes('/')) return 'page'
  return 'page'
}

/** Extract an AKIA access key ID from a string, or return null.
 *  Handles bare "AKIA..." and prefixed "AWS AKIA..." (from aws_deep_scan.txt log format). */
function extractAkiaKey(s: string | null | undefined): string | null {
  if (typeof s !== 'string') return null
  const m = s.match(/AKIA[A-Z0-9]{16}/)
  return m ? m[0] : null
}

/** [type, key_value, source_url, timestamp, metadata, status, dbId?] from app.py's recent_findings tuple */
function mapRecent(row: ReconRecentFinding, i: number): Finding {
  const [type, keyValue, sourceUrl, ts, metadata, dbStatus, dbId] = row
  const provider = String(type ?? 'Unknown')

  // aws_valid.txt (old format) stores "access_key:secret_key" with no domain,
  // so source_url ends up as the AKIA key and key_value as the secret.
  // aws_deep_scan.txt stores "AWS AKIA...:secret SES: ..." log lines.
  // In both cases extract the AKIA key from source_url and strip log junk.
  const akiaFromSource = extractAkiaKey(sourceUrl)
  const displayUrl = akiaFromSource ? undefined : (sourceUrl || undefined)

  // Strip " SES" / " SNS" log suffixes from key_value for deep-scan entries
  const cleanSecret = (keyValue ?? '').replace(/\s+(SES|SNS|IAM|Fargate).*$/i, '').trim()

  // Combined credential text for display/copy
  const credText = akiaFromSource
    ? `${akiaFromSource}:${cleanSecret}`  // ACCESS_KEY:SECRET_KEY
    : (keyValue ?? '')

  // Discovery method — how/where the credential was exposed
  const method = akiaFromSource ? 'page' : discoveryMethod(sourceUrl ?? '')

  // Build a descriptive ruleLabel: "SendGrid API key via .env" etc.
  const methodLabel = method !== 'page' ? ` via ${method}` : ''
  const ruleLabel = `${provider} credential${methodLabel}`

  const extra: Array<{ key: string; value: string }> = []
  if (akiaFromSource) {
    extra.push({ key: 'ACCESS KEY ID', value: akiaFromSource })
    extra.push({ key: 'SECRET KEY', value: cleanSecret })
  } else if (metadata) {
    extra.push({ key: 'Metadata', value: metadata })
  }
  if (displayUrl) {
    extra.push({ key: 'Source URL', value: displayUrl })
  }

  // Use the DB row id as the Finding id so Recheck/Resend can target it
  const id = dbId != null ? String(dbId) : `rs-${provider}-${i}-${ts}`

  // Map DB status to Finding status
  const rawStatus = String(dbStatus ?? 'hit').toLowerCase()
  const status: Finding['status'] =
    rawStatus === 'valid' ? 'valid' :
    rawStatus === 'dead' || rawStatus === 'invalid' ? 'dead' :
    'hit'

  return {
    id,
    at: ts ?? new Date().toISOString(),
    provider,
    ruleLabel,
    hostname: akiaFromSource ? 'aws-account' : hostnameFromUrl(sourceUrl ?? ''),
    url: displayUrl,
    path: displayUrl ? pathFromUrl(displayUrl) : undefined,
    detail: credText,
    details: {
      validated: status === 'valid',
      raw: credText,
      extra: extra.length > 0 ? extra : undefined,
    },
    severity: severityFor(provider),
    reportedByHost: 'recon-backend',
    status,
    discoveryMethod: method,
  }
}

export function useReconStats(): UseReconStatsResult {
  const [state, setState] = useState<ReconConnectionState>('connecting')
  const [lastError, setLastError] = useState<string | null>(null)
  const [raw, setRaw] = useState<ReconStats | null>(null)
  const [findings, setFindings] = useState<Finding[]>([])
  const pollIdRef = useRef<number | null>(null)
  const mountedRef = useRef(true)

  const apply = (payload: ReconStats) => {
    if (!mountedRef.current) return
    setRaw(payload)
    setFindings((payload.recent_findings ?? []).map((row, i) => mapRecent(row, i)))
    setLastError(null)
  }

  const refresh = async (): Promise<ReconStats | null> => {
    try {
      const payload = await statsApi.get()
      apply(payload)
      return payload
    } catch (err) {
      const msg = (err as Error).message || 'Failed to fetch /api/stats'
      if (mountedRef.current) setLastError(msg)
      return null
    }
  }

  useEffect(() => {
    mountedRef.current = true
    setState('connecting')

    // Initial REST hit so the UI populates immediately even if socket is slow.
    void (async () => {
      const ok = await refresh()
      if (!mountedRef.current) return
      setState(ok ? 'connected' : 'error')
    })()

    // Socket subscription for live pushes.
    const socket = getReconSocket()

    const onConnect = () => {
      if (!mountedRef.current) return
      setState('connected')
      setLastError(null)
    }
    const onDisconnect = (reason: string) => {
      if (!mountedRef.current) return
      setState('disconnected')
      setLastError(`socket disconnected: ${reason}`)
    }
    const onError = (err: Error) => {
      if (!mountedRef.current) return
      setLastError(err.message || 'socket error')
    }
    const onStatsUpdate = (payload: ReconStats) => {
      apply(payload)
    }

    socket.on('connect', onConnect)
    socket.on('disconnect', onDisconnect)
    socket.on('connect_error', onError)
    socket.on('stats_update', onStatsUpdate)

    // Fallback poll every 10s in case the socket is wedged.
    pollIdRef.current = window.setInterval(() => {
      if (state !== 'connected') void refresh()
    }, 10_000)

    return () => {
      mountedRef.current = false
      socket.off('connect', onConnect)
      socket.off('disconnect', onDisconnect)
      socket.off('connect_error', onError)
      socket.off('stats_update', onStatsUpdate)
      if (pollIdRef.current !== null) {
        window.clearInterval(pollIdRef.current)
        pollIdRef.current = null
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return { state, lastError, raw, findings, refresh }
}
