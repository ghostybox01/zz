import { useEffect, useRef } from 'react'
import type { Finding, VpsNode } from '../types'
import type { FleetControlConfig } from '../lib/fleetControl'
import { enrollSshViaApi } from '../lib/fleetControl'
import {
  hasFleetFingerprint,
  getVpsIdByFingerprint,
} from '../lib/fleetCredStore'
import {
  makeDiscoveredVps,
  markVpsEnrolled,
  markVpsEnrollFailed,
} from '../lib/fleetEnrollment'
import { extractSshCredential } from '../lib/sshCredential'

type Args = {
  findings: readonly Finding[]
  fleetControl: FleetControlConfig
  setFleet: React.Dispatch<React.SetStateAction<VpsNode[]>>
  onEnrolled?: (node: VpsNode, finding: Finding) => void
}

/** When scanning surfaces SSH/VPS credentials, enroll them into the fleet (sim or control plane). */
export function useFleetEnrollment({
  findings,
  fleetControl,
  setFleet,
  onEnrolled,
}: Args) {
  const processed = useRef<Set<string>>(new Set())
  const enrolling = useRef<Set<string>>(new Set())

  useEffect(() => {
    if (!fleetControl.autoEnroll) return

    const pending = findings.filter((f) => !processed.current.has(f.id))
    if (pending.length === 0) return

    for (const finding of pending) {
      processed.current.add(finding.id)
      const cred = extractSshCredential(finding)
      if (!cred) continue
      if (hasFleetFingerprint(cred.fingerprint)) {
        const existingId = getVpsIdByFingerprint(cred.fingerprint)
        if (existingId) {
          setFleet((prev) =>
            prev.map((n) =>
              n.id === existingId
                ? { ...n, lastEvent: `Re-seen in scan (${finding.provider})` }
                : n,
            ),
          )
        }
        continue
      }
      if (enrolling.current.has(cred.fingerprint)) continue
      enrolling.current.add(cred.fingerprint)

      const draft = makeDiscoveredVps(cred)
      setFleet((prev) => {
        if (prev.some((n) => n.id === draft.id || n.host === draft.host)) return prev
        return [...prev, draft]
      })

      void (async () => {
        const simDelay = 1200 + Math.random() * 1800
        const useApi = fleetControl.enabled && fleetControl.baseUrl

        const finish = (next: VpsNode) => {
          setFleet((prev) => prev.map((n) => (n.id === draft.id ? next : n)))
          enrolling.current.delete(cred.fingerprint)
          onEnrolled?.(next, finding)
        }

        if (!useApi) {
          await new Promise((r) => window.setTimeout(r, simDelay))
          const ok = Math.random() > 0.12
          finish(
            ok
              ? markVpsEnrolled(draft, 'SSH verified (sim) — ready for shard deploy')
              : markVpsEnrollFailed(draft, 'SSH handshake failed (sim)'),
          )
          return
        }

        const result = await enrollSshViaApi(fleetControl, {
          host: cred.host,
          port: cred.port,
          user: cred.user,
          secret: cred.secret,
          authType: cred.authType,
          vpsId: draft.id,
        })

        if (result.ok) {
          finish(
            markVpsEnrolled(
              {
                ...draft,
                label: result.hostname ? `disc-${result.hostname.split('.')[0]}` : draft.label,
                region: result.region ?? draft.region,
              },
              result.message,
            ),
          )
        } else {
          finish(markVpsEnrollFailed(draft, result.message))
        }
      })()
    }
  }, [findings, fleetControl, setFleet, onEnrolled])
}
