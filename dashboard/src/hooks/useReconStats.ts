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

/** Extract an AKIA access key ID from a string, or return null.
 *  Handles bare "AKIA..." and prefixed "AWS AKIA..." (from aws_deep_scan.txt log format). */
function extractAkiaKey(s: string | null | undefined): string | null {
  if (typeof s !== 'string') return null
  const m = s.match(/AKIA[A-Z0-9]{16}/)
  return m ? m[0] : null
}

/** [type, key_value, source_url, timestamp, metadata, status, dbId?] from app.py's recent_findings tuple */
function mapRecent(row: ReconRecentFinding, i: number): Finding {
  const [type, keyValue, sourceUrl, ts, metadata, , dbId] = row
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

  const extra: Array<{ key: string; value: string }> = []
  if (akiaFromSource) {
    extra.push({ key: 'ACCESS KEY ID', value: akiaFromSource })
    extra.push({ key: 'SECRET KEY', value: cleanSecret })
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
    hostname: akiaFromSource ? 'aws-account' : hostnameFromUrl(sourceUrl ?? ''),
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
