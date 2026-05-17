import { useCallback, useMemo, useState } from 'react'
import type { TargetList, VpsNode } from '../types'
import { vpsWorkloadState } from '../lib/vpsWorkload'
import { VpsCard, type VpsNodeAction } from './VpsCard'
import { TableToolbar } from './TableToolbar'
import { vps as reconVps } from '../lib/reconApi'

type WorkerRole = 'scanner' | 'warc'

export type FleetBulkAction = 'start-all' | 'stop-all' | 'restart-all' | 'test-connections'

type Props = {
  fleet: readonly VpsNode[]
  lists: readonly TargetList[]
  totalTargets: number
  scanning: boolean
  onRedeploySplit: () => void
  onForceOutage: (id: string, mode: 'offline' | 'reconnect') => void
  /** When set, replaces the demo `onForceOutage` controls with real backend actions. */
  onAction?: (ip: string, action: VpsNodeAction) => Promise<{ ok: boolean; message?: string }>
  /** When set, exposes bulk-action buttons in the header (live backend only). */
  onBulkAction?: (action: FleetBulkAction) => Promise<{ ok: boolean; message?: string }>
  /**
   * Local state mutator — used by per-row trash and bulk delete to remove
   * nodes from the visible fleet when the backend lacks a /remove endpoint.
   * If omitted, removal is a no-op (read-only roster view).
   */
  onRemoveNodes?: (ids: readonly string[]) => void
  /**
   * Surface rename / role-change failures from the persistence calls so the
   * operator sees them instead of a silently-reverted optimistic UI. Threaded
   * down from App.tsx so we share the existing alert-toast queue.
   */
  pushAlertToast?: (title: string, message?: string, kind?: 'error' | 'info') => void
}

export function FleetPanel({
  fleet,
  lists,
  totalTargets,
  scanning,
  onRedeploySplit,
  onForceOutage,
  onAction,
  onBulkAction,
  onRemoveNodes,
  pushAlertToast,
}: Props) {
  const active = useMemo(() => fleet.filter((n) => n.status !== 'removed'), [fleet])
  const removed = useMemo(() => fleet.filter((n) => n.status === 'removed'), [fleet])

  const healthy = active.filter((n) => n.status === 'healthy').length
  const degraded = active.filter((n) => n.status === 'degraded').length
  const reconnecting = active.filter((n) => n.status === 'reconnecting' || n.status === 'offline').length

  const totalAssigned = active.reduce((s, n) => s + n.targetsAssigned, 0)
  const totalDone = active.reduce((s, n) => s + n.targetsDone, 0)
  const totalSps = active.reduce((s, n) => s + n.scansPerSecond, 0)
  const totalFindings = fleet.reduce((s, n) => s + (n.findingsContributed ?? 0), 0)
  const busyCount = active.filter((n) => vpsWorkloadState(n, lists) === 'busy').length
  const freeCount = active.filter((n) => vpsWorkloadState(n, lists) === 'free').length

  const [bulkBusy, setBulkBusy] = useState<FleetBulkAction | null>(null)
  const [bulkMessage, setBulkMessage] = useState<string | null>(null)

  // Effect D — table affordances: filter + bulk select + per-row trash.
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [filter, setFilter] = useState('')

  // Per-worker label + role overrides, keyed by IP. Hard refresh will rehydrate
  // from the server's next status payload once the backend includes them; until
  // then this gives the operator immediate visual feedback after a rename/tag.
  const [labelOverride, setLabelOverride] = useState<Record<string, string>>({})
  const [roleOverride, setRoleOverride] = useState<Record<string, WorkerRole>>({})
  const [renamingIp, setRenamingIp] = useState<string | null>(null)
  const [renameDraft, setRenameDraft] = useState('')

  const labelFor = useCallback(
    (n: VpsNode) => labelOverride[n.host] ?? n.label ?? n.host,
    [labelOverride],
  )
  const roleFor = useCallback(
    (n: VpsNode): WorkerRole => roleOverride[n.host] ?? 'scanner',
    [roleOverride],
  )

  const commitRename = async (ip: string, nextLabel: string) => {
    const trimmed = nextLabel.trim().slice(0, 80)
    setRenamingIp(null)
    if (!trimmed) return
    // Capture the previous override so we can roll back if the backend rejects.
    const previous = labelOverride[ip]
    setLabelOverride((m) => ({ ...m, [ip]: trimmed }))
    try {
      await reconVps.setLabel(ip, trimmed)
    } catch (e) {
      setLabelOverride((m) => {
        const next = { ...m }
        if (previous === undefined) delete next[ip]
        else next[ip] = previous
        return next
      })
      pushAlertToast?.('Rename failed', (e as Error).message, 'error')
    }
  }

  const toggleRole = async (ip: string, current: WorkerRole) => {
    const nextRole: WorkerRole = current === 'warc' ? 'scanner' : 'warc'
    const previous = roleOverride[ip]
    setRoleOverride((m) => ({ ...m, [ip]: nextRole }))
    try {
      await reconVps.setRole(ip, nextRole)
    } catch (e) {
      setRoleOverride((m) => {
        const next = { ...m }
        if (previous === undefined) delete next[ip]
        else next[ip] = previous
        return next
      })
      pushAlertToast?.('Role change failed', (e as Error).message, 'error')
    }
  }

  const filterLower = filter.trim().toLowerCase()

  const visibleActive = useMemo(() => {
    if (!filterLower) return active
    return active.filter((n) =>
      labelFor(n).toLowerCase().includes(filterLower) ||
      n.host.toLowerCase().includes(filterLower) ||
      n.region.toLowerCase().includes(filterLower),
    )
  }, [active, filterLower, labelFor])

  const visibleRemoved = useMemo(() => {
    if (!filterLower) return removed
    return removed.filter((n) =>
      labelFor(n).toLowerCase().includes(filterLower) ||
      n.host.toLowerCase().includes(filterLower) ||
      n.region.toLowerCase().includes(filterLower),
    )
  }, [removed, filterLower, labelFor])

  const allVisibleIds = useMemo(
    () => [...visibleActive, ...visibleRemoved].map((n) => n.id),
    [visibleActive, visibleRemoved],
  )

  const toggleOne = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const handleSelectAll = () => setSelected(new Set(allVisibleIds))
  const handleClearSelection = () => setSelected(new Set())
  const handleSelectDead = () =>
    setSelected(
      new Set(
        [...visibleActive, ...visibleRemoved]
          .filter((n) => n.status === 'offline' || n.status === 'removed')
          .map((n) => n.id),
      ),
    )

  const removeNodes = (ids: readonly string[]) => {
    if (ids.length === 0) return
    // Best-effort: try the new backend endpoint per node, but always remove
    // locally so the operator sees the row disappear. If the endpoint is
    // missing the POST will reject — we swallow the error and rely on the
    // client-side prune.
    // TODO(backend): wire /api/vps/server/<ip>/remove on the Flask side so
    // the roster file (server_ips.txt) is updated authoritatively.
    const idSet = new Set(ids)
    const ipsToTry = fleet.filter((n) => idSet.has(n.id)).map((n) => n.host)
    for (const ip of ipsToTry) {
      void reconVps.removeFromRoster(ip).catch(() => {
        /* endpoint may not exist — client prune is the source of truth */
      })
    }
    onRemoveNodes?.(ids)
    setSelected((prev) => {
      const next = new Set(prev)
      for (const id of ids) next.delete(id)
      return next
    })
  }

  const handleDeleteSelected = () => removeNodes([...selected])

  const handleRowTrash = (node: VpsNode) => {
    if (window.confirm(`Remove ${node.label} (${node.host}) from the roster?`)) {
      removeNodes([node.id])
    }
  }

  const runBulk = async (action: FleetBulkAction) => {
    if (!onBulkAction || bulkBusy) return
    setBulkBusy(action)
    setBulkMessage(null)
    try {
      const r = await onBulkAction(action)
      setBulkMessage(r.message ?? (r.ok ? `${action} ok` : `${action} failed`))
    } catch (e) {
      setBulkMessage((e as Error).message ?? `${action} error`)
    } finally {
      setBulkBusy(null)
      window.setTimeout(() => setBulkMessage(null), 4500)
    }
  }

  return (
    <section className="flt">
      <header className="flt__head">
        <div className="flt__title-block">
          <h2 className="flt__title">Fleet</h2>
          <span className="flt__pill flt__pill--muted">{active.length} active{removed.length > 0 ? ` · ${removed.length} pruned` : ''}</span>
        </div>
        <div className="flt__head-right">
          {onBulkAction && (
            <div className="flt__bulk" role="group" aria-label="Bulk fleet actions">
              <button type="button" className="btn-glass btn-glass--xs" disabled={bulkBusy !== null} onClick={() => void runBulk('start-all')} title="POST /api/vps/start-all">
                {bulkBusy === 'start-all' ? '…' : '▶ Start all'}
              </button>
              <button type="button" className="btn-glass btn-glass--xs" disabled={bulkBusy !== null} onClick={() => void runBulk('stop-all')} title="POST /api/vps/stop-all">
                {bulkBusy === 'stop-all' ? '…' : '■ Stop all'}
              </button>
              <button type="button" className="btn-glass btn-glass--xs" disabled={bulkBusy !== null} onClick={() => void runBulk('restart-all')} title="POST /api/vps/restart-all">
                {bulkBusy === 'restart-all' ? '…' : '↻ Restart all'}
              </button>
              <button type="button" className="btn-glass btn-glass--xs" disabled={bulkBusy !== null} onClick={() => void runBulk('test-connections')} title="POST /api/vps/test-connections">
                {bulkBusy === 'test-connections' ? '…' : '✔ Test SSH'}
              </button>
            </div>
          )}
          <button type="button" className="btn-glass" onClick={onRedeploySplit}>
            Recalculate shards
          </button>
          <span className={`flt__sim${scanning ? ' flt__sim--on' : ''}`}>
            {scanning ? 'Sim active' : 'Sim paused'}
          </span>
          <span className="flt__pill flt__pill--ok">
            <span className="flt__pill-dot" />
            {healthy} healthy
          </span>
          <span className="flt__pill flt__pill--busy">{busyCount} busy</span>
          <span className="flt__pill flt__pill--free">{freeCount} free</span>
          {degraded > 0 && (
            <span className="flt__pill flt__pill--warn">
              <span className="flt__pill-dot" />
              {degraded} degraded
            </span>
          )}
          {reconnecting > 0 && (
            <span className="flt__pill flt__pill--orange">
              <span className="flt__pill-dot" />
              {reconnecting} reconnecting
            </span>
          )}
        </div>
      </header>

      {bulkMessage && <p className="flt__bulk-msg muted">{bulkMessage}</p>}

      <TableToolbar
        totalRows={visibleActive.length + visibleRemoved.length}
        selectedCount={selected.size}
        filter={filter}
        onFilterChange={setFilter}
        onSelectAll={handleSelectAll}
        onClearSelection={handleClearSelection}
        onSelectDead={handleSelectDead}
        onDeleteSelected={handleDeleteSelected}
        filterPlaceholder="Filter fleet by label, host, region…"
      />

      <div className="flt__kpis">
        <div className="flt__kpi">
          <div className="flt__kpi-label">COMBINED THROUGHPUT</div>
          <div className="flt__kpi-value">
            {totalSps.toFixed(1)}
            <span>probes/s</span>
          </div>
        </div>
        <div className="flt__kpi">
          <div className="flt__kpi-label">TARGETS PROCESSED</div>
          <div className="flt__kpi-value">
            {totalDone.toLocaleString()}
            <span>/ {(totalAssigned || totalTargets || 0).toLocaleString()}</span>
          </div>
        </div>
        <div className="flt__kpi">
          <div className="flt__kpi-label">FINDINGS CONTRIBUTED</div>
          <div className="flt__kpi-value">
            {totalFindings.toLocaleString()}
            <span>across fleet</span>
          </div>
        </div>
        <div className="flt__kpi">
          <div className="flt__kpi-label">MODE</div>
          <div className="flt__kpi-value">
            {scanning ? 'Simulated' : 'Paused'}
            <span>data source</span>
          </div>
        </div>
      </div>

      {totalAssigned > 0 && (
        <div className="flt__shard-bar" aria-label="Shard distribution">
          {active.map((n) => (
            <div
              key={n.id}
              className={`flt__shard flt__shard--${n.status}`}
              style={{ flex: `${Math.max(1, n.targetsAssigned)} 1 0` }}
              title={`${n.label}: ${n.targetsAssigned.toLocaleString()} lines · ${n.targetsDone.toLocaleString()} done`}
            >
              <span className="mono">{n.label}</span>
            </div>
          ))}
        </div>
      )}

      <div className="flt__grid">
        {visibleActive.map((n) => (
          <div key={n.id} className="flt__row-wrap">
            <FleetCardControls
              node={n}
              selected={selected.has(n.id)}
              onToggleSelect={() => toggleOne(n.id)}
              onTrash={() => handleRowTrash(n)}
              label={labelFor(n)}
              role={roleFor(n)}
              renaming={renamingIp === n.host}
              renameDraft={renameDraft}
              onStartRename={() => {
                setRenameDraft(labelFor(n))
                setRenamingIp(n.host)
              }}
              onChangeRenameDraft={setRenameDraft}
              onCommitRename={() => commitRename(n.host, renameDraft)}
              onCancelRename={() => setRenamingIp(null)}
              onToggleRole={() => toggleRole(n.host, roleFor(n))}
            />
            <VpsCard
              node={{ ...n, label: labelFor(n) }}
              lists={lists}
              onForceOutage={onAction ? undefined : onForceOutage}
              onAction={onAction}
            />
          </div>
        ))}
      </div>

      {visibleRemoved.length > 0 && (
        <div className="flt__grid flt__grid--removed">
          {visibleRemoved.map((n) => (
            <div key={n.id} className="flt__row-wrap">
              <FleetCardControls
                node={n}
                selected={selected.has(n.id)}
                onToggleSelect={() => toggleOne(n.id)}
                onTrash={() => handleRowTrash(n)}
                label={labelFor(n)}
                role={roleFor(n)}
                renaming={renamingIp === n.host}
                renameDraft={renameDraft}
                onStartRename={() => {
                  setRenameDraft(labelFor(n))
                  setRenamingIp(n.host)
                }}
                onChangeRenameDraft={setRenameDraft}
                onCommitRename={() => commitRename(n.host, renameDraft)}
                onCancelRename={() => setRenamingIp(null)}
                onToggleRole={() => toggleRole(n.host, roleFor(n))}
              />
              <VpsCard node={{ ...n, label: labelFor(n) }} />
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

/** Thin control strip rendered ABOVE each fleet card.
 *  Hosts: bulk-select checkbox, rename pencil (inline input on click),
 *  role chip (toggles scanner↔warc), and trash. Lives in its own row
 *  so it can never overlap the card title or status pills underneath. */
function FleetCardControls(props: {
  node: VpsNode
  selected: boolean
  onToggleSelect: () => void
  onTrash: () => void
  label: string
  role: WorkerRole
  renaming: boolean
  renameDraft: string
  onStartRename: () => void
  onChangeRenameDraft: (v: string) => void
  onCommitRename: () => void
  onCancelRename: () => void
  onToggleRole: () => void
}) {
  const {
    node, selected, onToggleSelect, onTrash, label, role,
    renaming, renameDraft, onStartRename, onChangeRenameDraft,
    onCommitRename, onCancelRename, onToggleRole,
  } = props
  return (
    <div className="flt__row-controls">
      <label className="flt__row-check" title="Select for bulk action">
        <input
          type="checkbox"
          checked={selected}
          onChange={onToggleSelect}
          aria-label={`Select ${label}`}
        />
      </label>

      {renaming ? (
        <input
          type="text"
          className="flt__row-name-edit"
          value={renameDraft}
          autoFocus
          onChange={(e) => onChangeRenameDraft(e.target.value)}
          onBlur={onCommitRename}
          onKeyDown={(e) => {
            if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
            else if (e.key === 'Escape') onCancelRename()
          }}
          aria-label={`Rename ${node.host}`}
        />
      ) : (
        <button
          type="button"
          className="flt__row-rename"
          title={`Rename ${label}`}
          onClick={onStartRename}
        >
          ✎
        </button>
      )}

      <button
        type="button"
        className={`flt__role-chip flt__role-chip--${role}`}
        title={`Role: ${role} — click to toggle`}
        onClick={onToggleRole}
      >
        {role}
      </button>

      <span className="flt__row-controls-spacer" />

      <button
        type="button"
        className="icon-btn flt__row-trash"
        title={`Remove ${label} from roster`}
        onClick={onTrash}
      >
        🗑
      </button>
    </div>
  )
}
