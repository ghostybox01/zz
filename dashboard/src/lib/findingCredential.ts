import type { Finding } from '../types'

/** Full discovered material for display / copy (prefer unmasked `details.raw`). */
export function findingCredentialText(f: Finding): string {
  const s = f.details?.raw ?? f.detail
  return typeof s === 'string' ? s.trim() : ''
}
