import type { TargetList, VpsNode } from '../types'

export type VpsWorkloadState = 'free' | 'busy' | 'unavailable'

const ACTIVE_LIST_STATUSES = new Set<TargetList['status']>(['deployed', 'queued'])

/** List this node is tied to — node fields, then list roster (matches Lists tab chips). */
export function listAssignmentForNode(
  node: VpsNode,
  lists: readonly TargetList[],
): TargetList | null {
  if (node.activeListId) {
    const hit = lists.find((l) => l.id === node.activeListId)
    if (hit) return hit
  }
  if (node.activeListName) {
    const hit = lists.find((l) => l.name === node.activeListName)
    if (hit) return hit
  }
  return (
    lists.find(
      (l) =>
        l.assignedVpsIds.includes(node.id) &&
        ACTIVE_LIST_STATUSES.has(l.status),
    ) ?? null
  )
}

export function vpsWorkloadState(
  node: VpsNode,
  lists: readonly TargetList[] = [],
): VpsWorkloadState {
  if (node.status === 'removed' || node.status === 'offline' || node.status === 'reconnecting') {
    return 'unavailable'
  }

  const backlog = node.targetsAssigned - node.targetsDone
  if (backlog > 0) return 'busy'

  if (node.activeListId || node.activeListName) return 'busy'

  if (listAssignmentForNode(node, lists)) return 'busy'

  return 'free'
}

export function workloadLabel(state: VpsWorkloadState): string {
  switch (state) {
    case 'busy':
      return 'BUSY'
    case 'free':
      return 'FREE'
    case 'unavailable':
      return 'UNAVAILABLE'
  }
}

export function scanningListLabel(node: VpsNode, lists: readonly TargetList[]): string | null {
  const list = listAssignmentForNode(node, lists)
  if (list) return list.name
  if (node.activeListName) return node.activeListName
  return null
}

/** True when the worker still has lines left on its shard. */
export function hasScanBacklog(node: VpsNode): boolean {
  return node.targetsAssigned > node.targetsDone
}

export function scanListCaption(node: VpsNode, lists: readonly TargetList[]): string {
  const name = scanningListLabel(node, lists)
  if (!name) return ''
  return hasScanBacklog(node) ? 'Scanning' : 'Assigned to'
}
