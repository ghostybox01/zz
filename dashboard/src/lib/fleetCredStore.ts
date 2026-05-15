import type { SshCredential } from './sshCredential'

/** In-memory only — never written to localStorage. */
const byVpsId = new Map<string, SshCredential>()
const byFingerprint = new Map<string, string>()

export function storeFleetCredential(vpsId: string, cred: SshCredential): void {
  byVpsId.set(vpsId, cred)
  byFingerprint.set(cred.fingerprint, vpsId)
}

export function getFleetCredential(vpsId: string): SshCredential | undefined {
  return byVpsId.get(vpsId)
}

export function hasFleetFingerprint(fingerprint: string): boolean {
  return byFingerprint.has(fingerprint)
}

export function getVpsIdByFingerprint(fingerprint: string): string | undefined {
  return byFingerprint.get(fingerprint)
}

export function clearFleetCredentials(): void {
  byVpsId.clear()
  byFingerprint.clear()
}
