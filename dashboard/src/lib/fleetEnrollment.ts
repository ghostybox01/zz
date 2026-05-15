import { pushCpuSample } from './vpsHistory'
import type { SshCredential } from './sshCredential'
import { storeFleetCredential } from './fleetCredStore'
import type { VpsNode } from '../types'

function guessRegion(host: string): string {
  if (host.startsWith('104.') || host.startsWith('159.')) return 'NYC3'
  if (host.startsWith('128.') || host.startsWith('139.')) return 'SGP1'
  if (host.startsWith('188.') || host.startsWith('167.')) return 'AMS3'
  return 'DISC'
}

export function makeDiscoveredVps(cred: SshCredential): VpsNode {
  const id = `vps-disc-${cred.fingerprint.replace(/[^a-z0-9]+/gi, '-').slice(0, 48)}`
  storeFleetCredential(id, cred)
  const short = cred.host.split('.').slice(-2).join('.') || cred.host
  return {
    id,
    label: `disc-${short}`,
    host: cred.port === 22 ? cred.host : `${cred.host}:${cred.port}`,
    region: guessRegion(cred.host),
    status: 'reconnecting',
    source: 'discovered',
    discoveredFromFindingId: cred.findingId,
    authType: cred.authType,
    cpuPercent: 0,
    cpuHistory: [],
    ramUsedGb: 0,
    ramTotalGb: 8,
    diskUsedGb: 0,
    diskTotalGb: 80,
    targetsAssigned: 0,
    targetsDone: 0,
    scansPerSecond: 0,
    reconnectFailCount: 0,
    findingsContributed: 0,
    uptimeMin: 0,
    lastEvent: `Discovered via scan hit · SSH ${cred.authType} as ${cred.user}`,
  }
}

export function markVpsEnrolled(node: VpsNode, message: string): VpsNode {
  return {
    ...node,
    status: 'healthy',
    reconnectFailCount: 0,
    cpuPercent: 22 + Math.floor(Math.random() * 18),
    cpuHistory: pushCpuSample(node.cpuHistory, 28),
    ramUsedGb: 1.2,
    uptimeMin: 1,
    lastEvent: message,
  }
}

export function markVpsEnrollFailed(node: VpsNode, message: string): VpsNode {
  return {
    ...node,
    status: 'offline',
    reconnectFailCount: 1,
    lastEvent: message,
  }
}
