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
  'AWS SNS': 'medium',
  Stripe: 'critical',
  Mnemonic: 'critical',
  Crypto: 'critical',
  TruffleHog: 'high',
  GitLeaks: 'high',
  GitHub: 'high',
  GitLab: 'high',
  Heroku: 'high',
  Datadog: 'high',
  OpenAI: 'high',
  Anthropic: 'high',
  SendGrid: 'high',
  Mailgun: 'high',
  Twilio: 'high',
  Brevo: 'medium',
  Mandrill: 'medium',
  MailerSend: 'medium',
  Postmark: 'medium',
  SparkPost: 'medium',
  Mailtrap: 'medium',
  Mailjet: 'medium',
  Nexmo: 'medium',
  Telnyx: 'medium',
  MessageBird: 'medium',
  Plivo: 'medium',
  SMTP: 'high',
  GCP: 'critical',
  Cloudflare: 'high',
  DigitalOcean: 'medium',
  Sentry: 'medium',
  NPM: 'medium',
  PyPI: 'medium',
  Discord: 'medium',
  Slack: 'medium',
  JWT: 'high',
  Azure: 'high',
  Tencent: 'medium',
  XSMTP: 'medium',
}

function severityFor(provider: string): FindingSeverity {
  return PROVIDER_SEVERITY[provider] ?? 'medium'
}


function pathFromUrl(url: string): string | undefined {
  if (!url) return undefined
  try {
    return new URL(url).pathname
  } catch {
    return undefined
  }
}

/** [type, key_value, source_url, timestamp, metadata] from app.py's recent_findings tuple */
function mapRecent(row: ReconRecentFinding, i: number): Finding {
  const [type, keyValue, sourceUrl, ts, metadata] = row
  const provider = String(type ?? 'Unknown')

  // key_value holds "//hostname (module-name)" — strip leading // and (module) suffix
  const hostname = String(keyValue ?? '').replace(/^\/\//, '').replace(/\s*\(.*\)$/, '').trim()

  // metadata format: "/path):credential" — split on first "):" to get path + credential
  let extractedPath: string | undefined = pathFromUrl(sourceUrl ?? '')
  let ruleLabel = `${provider} credential`
  let credential = metadata ?? ''
  if (metadata && metadata.includes('):')) {
    const idx = metadata.indexOf('):')
    const pathPart = metadata.slice(0, idx)
    credential = metadata.slice(idx + 2)
    if (pathPart.startsWith('/') || pathPart.startsWith('.')) {
      extractedPath = pathPart
      ruleLabel = `${provider} via ${pathPart}`
    }
  }

  return {
    id: `rs-${provider}-${i}-${ts}`,
    at: ts ?? new Date().toISOString(),
    provider,
    ruleLabel,
    hostname,
    url: sourceUrl ?? undefined,
    path: extractedPath,
    detail: credential,
    details: {
      validated: true,
      raw: credential,
      extra: [{ key: 'Source', value: `${hostname}${extractedPath ?? ''}` }],
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
    setState('connected')
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
    // Always fire — the `state` captured here is stale due to the empty dep array.
    pollIdRef.current = window.setInterval(() => {
      void refresh()
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
