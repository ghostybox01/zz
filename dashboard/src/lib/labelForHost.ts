import type { VpsNode } from '../types'

/**
 * Resolve a raw worker host/IP into the operator-facing label.
 *
 * Lookup order:
 *   1. Match `node.host` exactly against the supplied fleet roster.
 *   2. Match `node.host` with a trailing `:port` (e.g. fleet has `1.2.3.4:22`
 *      but the caller only has the bare `1.2.3.4`).
 *
 * When a match has a non-empty `node.label`, the returned string follows the
 * design rule "label primary, host visible": `"label (host)"`. Empty or
 * whitespace-only labels — plus hosts that are not in the fleet at all
 * (e.g., the WARC dropdown's special `'controller'` entry) — fall through to
 * the raw host string so the operator can still tie what they see to a real
 * machine.
 */
export function labelForHost(host: string, fleet: readonly VpsNode[]): string {
  if (!host) return host
  const node = fleet.find(
    (n) => n.host === host || n.host.startsWith(`${host}:`),
  )
  const label = node?.label?.trim()
  if (label) return `${label} (${host})`
  return host
}
