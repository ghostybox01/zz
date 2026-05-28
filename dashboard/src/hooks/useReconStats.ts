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
  TruffleHog: 'high',
  GitLeaks: 'high',
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
  GCP: 'critical',
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
    return undefined
  }
}

/** True when a string is an AWS AKIA access key ID. */
function isAkiaKey(s: string | null | undefined): boolean {
  return typeof s === 'string' && /^AKIA[A-Z0-9]{16}$/.test(s)
}

/** [type, key_value, source_url, timestamp, metadata, status, dbId?] from app.py's recent_findings tuple */
function mapRecent(row: ReconRecentFinding, i: number): Finding {
  const [type, keyValue, sourceUrl, ts, metadata, , dbId] = row
  const provider = String(type ?? 'Unknown')

  // aws_valid.txt stores "access_key:secret_key" with no source domain,
  // so after parsing: source_url = AKIA access key ID, key_value = secret key.
  // Detect this pattern and display as a proper ACCESS:SECRET pair.
  const akiaInSource = isAkiaKey(sourceUrl)
  const displayUrl = akiaInSource ? undefined : (sourceUrl || undefined)

  // Combined credential text for display/copy
  const credText = akiaInSource
    ? `${sourceUrl}:${keyValue ?? ''}`  // ACCESS_KEY:SECRET_KEY
    : (keyValue ?? '')

  const extra: Array<{ key: string; value: string }> = []
  if (akiaInSource) {
    extra.push({ key: 'ACCESS KEY ID', value: String(sourceUrl) })
    extra.push({ key: 'SECRET KEY', value: String(keyValue ?? '') })
  } else if (metadata) {
    extra.push({ key: 'Metadata', value: metadata })
  }

  // Use the DB row id as the Finding id so Recheck/Resend can target it
  const id = dbId != null ? String(dbId) : `rs-${provider}-${i}-${ts}`

  return {
    id,
    at: ts ?? new Date().toISOString(),
    provider,
    ruleLabel: `${provider} credential`,
    hostname: akiaInSource ? 'aws-account' : hostnameFromUrl(sourceUrl ?? ''),
    url: displayUrl,
    path: displayUrl ? pathFromUrl(displayUrl) : undefined,
    detail: credText,
    details: {
      validated: true,
      raw: credText,
      extra: extra.length > 0 ? extra : undefined,
    },
    severity: severityFor(provider),
    reportedByHost: 'recon-backend',
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
