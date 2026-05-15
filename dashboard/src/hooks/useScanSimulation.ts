import { useEffect, useRef } from 'react'
import { pushCpuSample } from '../lib/vpsHistory'
import type { Finding, FindingSeverity, VpsNode } from '../types'
import { VPS_MAX_RECONNECT_TRIES, jitter } from '../types'

type Args = {
  scanning: boolean
  setFleetActive: React.Dispatch<React.SetStateAction<VpsNode[]>>
  vulnProviders: readonly string[]
  pushFinding: (f: Omit<Finding, 'id'>) => void
}

/** Browser-only clock — adjusts load, bounded SSH retries, merges synthetic hits back to UI. */
export function useScanSimulation({
  scanning,
  setFleetActive,
  vulnProviders,
  pushFinding,
}: Args) {
  const reconnectPhase = useRef(0)

  useEffect(() => {
    const id = window.setInterval(() => {
      reconnectPhase.current = (reconnectPhase.current + 1) % 3
      const spacedReconnectPulse = reconnectPhase.current === 0

      setFleetActive((prev) => {
        const next = prev.map((node): VpsNode => {
          if (node.status === 'removed') return node

          if (
            (node.status === 'reconnecting' || node.status === 'offline') &&
            spacedReconnectPulse
          ) {
            const healed = Math.random() > (node.status === 'offline' ? 0.55 : 0.45)
            if (healed) {
              const cpu = jitter(38, 18)
              return {
                ...node,
                status: 'healthy',
                reconnectFailCount: 0,
                cpuPercent: cpu,
                cpuHistory: pushCpuSample(node.cpuHistory, cpu),
                lastEvent:
                  node.status === 'offline'
                    ? 'Worker back online'
                    : 'Connection stabilized',
              }
            }

            const nextFails = node.reconnectFailCount + 1
            if (nextFails >= VPS_MAX_RECONNECT_TRIES) {
              return {
                ...node,
                status: 'removed',
                reconnectFailCount: nextFails,
                scansPerSecond: 0,
                removedReason:
                  node.status === 'offline'
                    ? 'Host unreachable after scripted restart budget'
                    : 'Control channel never stabilized',
                lastEvent: `Removed after ${nextFails} failed reconnects`,
              }
            }

            return {
              ...node,
              reconnectFailCount: nextFails,
              lastEvent:
                node.status === 'offline'
                  ? `Still unreachable (${nextFails}/${VPS_MAX_RECONNECT_TRIES})`
                  : `Reconnect attempt ${nextFails}/${VPS_MAX_RECONNECT_TRIES}`,
            }
          }

          let n = node
          if (
            scanning &&
            n.status !== 'reconnecting' &&
            n.status !== 'offline' &&
            n.status !== 'removed'
          ) {
            const cpuPercent = jitter(
              n.targetsAssigned > 0 && n.targetsDone < n.targetsAssigned
                ? 58
                : 33,
              18,
            )
            n = {
              ...n,
              cpuPercent,
              cpuHistory: pushCpuSample(n.cpuHistory, cpuPercent),
              ramUsedGb: +(Math.min(n.ramTotalGb * 0.94,
                n.ramUsedGb + (Math.random() * 0.12 - 0.03))).toFixed(2),
              scansPerSecond:
                n.targetsAssigned > 0 && n.targetsDone < n.targetsAssigned
                  ? n.status === 'degraded'
                    ? +(7 + Math.random() * 12).toFixed(1)
                    : +(14 + Math.random() * 40).toFixed(1)
                  : 0,
            }

            const backlog = n.targetsAssigned - n.targetsDone
            if (backlog > 0) {
              const leap =
                backlog > 2000 ? 210 + Math.floor(Math.random() * 420)
                : backlog > 500 ? 60 + Math.floor(Math.random() * 160)
                  : 6 + Math.floor(Math.random() * 48)
              const newDone = Math.min(n.targetsAssigned, n.targetsDone + leap)
              const finished = newDone >= n.targetsAssigned
              n = {
                ...n,
                targetsDone: newDone,
                ...(finished
                  ? {
                      activeListId: undefined,
                      activeListName: undefined,
                      scansPerSecond: 0,
                      lastEvent: n.activeListName
                        ? `Finished ${n.activeListName}`
                        : 'Shard complete — idle',
                    }
                  : {}),
              }
            }
          }
          return n
        })

        if (scanning && Math.random() > 0.965) {
          queueMicrotask(() => {
            const ip = `167.${40 + Math.floor(Math.random() * 80)}.${Math.floor(Math.random() * 255)}.${1 + Math.floor(Math.random() * 250)}`
            pushFinding({
              at: new Date().toISOString(),
              provider: 'SSH',
              ruleLabel: 'VPS root / deploy SSH material',
              hostname: ip,
              url: `https://leak-${Math.floor(Math.random() * 900 + 100)}.example/.env`,
              path: '/.env',
              detail: `${ip} · root · ***`,
              severity: 'critical',
              reportedByHost: 'sim-scanner',
              details: {
                validated: true,
                raw: `${ip}:root:sim-${Math.random().toString(36).slice(2, 10)}`,
              },
            })
          })
        } else if (scanning && vulnProviders.length > 0 && Math.random() > 0.88) {
          const liveWorkers = next.filter(
            (v) =>
              (v.status === 'healthy' || v.status === 'degraded') &&
              v.targetsAssigned > 0 &&
              v.targetsDone < v.targetsAssigned,
          )
          if (liveWorkers.length > 0) {
            queueMicrotask(() => {
              const worker =
                liveWorkers[Math.floor(Math.random() * liveWorkers.length)]!
              const provider =
                vulnProviders[
                  Math.floor(Math.random() * vulnProviders.length)
                ]!
              const severityPool: FindingSeverity[] = [
                'low',
                'medium',
                'high',
                'critical',
              ]
              const severity =
                severityPool[Math.floor(Math.random() * severityPool.length)]!
              const sample = MOCK[provider] ?? MOCK.Generic
              pushFinding({
                at: new Date().toISOString(),
                provider,
                ruleLabel:
                  provider === 'Generic'
                    ? 'Configuration blob detector'
                    : `${provider}-scoped rule`,
                hostname: `${worker.label.replace(/\W+/g, '')}-tgt-${Math.floor(
                  Math.random() * 800 + 100,
                )}.io`,
                detail: sample,
                severity,
                reportedByHost: worker.id,
                details: { raw: sample },
              })
            })
          }
        }

        return next
      })
    }, 1000)

    return () => window.clearInterval(id)
  }, [scanning, pushFinding, setFleetActive, vulnProviders])
}

const MOCK: Record<string, string> = {
  AWS: 'Ses / IAM identifier material surfaced in scraped path',
  GCP: '"type":"service_account" plus key block remnants',
  Azure: 'ADO / storage connection leaked in debug endpoint',
  SendGrid: 'Bearer SG.+ match in plaintext response bundle',
  Brevo: 'Transactional SMTP artefact echoed in stale asset',
  Mailgun: '"key-xxxx" token pair in webhook dump',
  Postmark: 'Server token surfaced on mis-deployed SPA',
  Twilio: 'Account SID mirrored next to probable auth secret',
  Stripe: 'sk_live_ bearer material outside signing flow',
  Cloudflare: 'CF API bearer + scoped permission markers',
  Generic: '.env leakage with SMTP + provider markers',
}
