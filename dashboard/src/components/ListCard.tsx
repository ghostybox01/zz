import { useState } from 'react'
import type { TargetList, VpsNode } from '../types'

const PlayIcon  = () => <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" aria-hidden><path d="M7 5v14l12-7L7 5z"/></svg>
const PauseIcon = () => <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" aria-hidden><rect x="6" y="5" width="4" height="14" rx="1"/><rect x="14" y="5" width="4" height="14" rx="1"/></svg>
const CheckIcon = () => <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden><path d="M20 6 9 17l-5-5"/></svg>
const ResetIcon = () => <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/></svg>

type Props = {
  list: TargetList
  fleet: readonly VpsNode[]
  onToggleVps: (listId: string, vpsId: string) => void
  onDeploy: (listId: string) => void
  onPause: (listId: string) => void
  onComplete: (listId: string) => void
  onReset: (listId: string) => void
  onDelete: (listId: string) => void
  onRename: (listId: string, name: string) => void
}

function fmtAge(iso: string): string {
  const ms = Date.now() - Date.parse(iso)
  if (!Number.isFinite(ms) || ms < 0) return '—'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  return `${d}d ago`
}

export function ListCard({
  list,
  fleet,
  onToggleVps,
  onDeploy,
  onPause,
  onComplete,
  onReset,
  onDelete,
  onRename,
}: Props) {
  const [editing, setEditing] = useState(false)
  const [draftName, setDraftName] = useState(list.name)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const activeFleet = fleet.filter((n) => n.status !== 'removed')
  const deployableFleet = activeFleet.filter((n) => n.status === 'healthy' || n.status === 'degraded')
  const assignedSet = new Set(list.assignedVpsIds)
  const assignedCount = assignedSet.size

  return (
    <article className={`tlist tlist--${list.status}`}>
      <header className="tlist__head">
        <div className="tlist__head-left">
          {editing ? (
            <input
              type="text"
              className="tlist__name-edit"
              value={draftName}
              autoFocus
              onChange={(e) => setDraftName(e.target.value)}
              onBlur={() => {
                setEditing(false)
                if (draftName.trim() && draftName !== list.name) onRename(list.id, draftName.trim())
                else setDraftName(list.name)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
                else if (e.key === 'Escape') {
                  setDraftName(list.name)
                  setEditing(false)
                }
              }}
            />
          ) : (
            <h3 className="tlist__name" onClick={() => setEditing(true)} title="Click to rename">
              {list.name}
            </h3>
          )}
          <p className="tlist__meta">
            <span className="mono">{list.lineCount.toLocaleString()} lines</span>
            <span className="muted"> · </span>
            <span className="muted">uploaded {fmtAge(list.uploadedAt)}</span>
            <span className="muted"> · </span>
            <span className="mono muted">{list.contentHash.slice(0, 10)}</span>
          </p>
        </div>
        <span className={`tlist__status tlist__status--${list.status}`}>
          <span className="tlist__status-dot" aria-hidden />
          {list.status}
        </span>
      </header>

      {list.note && <p className="tlist__note">{list.note}</p>}

      {list.preview.length > 0 && (
        <details className="tlist__preview">
          <summary>Preview · first {list.preview.length} entries</summary>
          <pre className="tlist__preview-body mono">{list.preview.join('\n')}</pre>
        </details>
      )}

      <div className="tlist__deploy">
        <div className="tlist__deploy-head">
          <span className="muted">Deploy to</span>
          <span className="mono">
            {assignedCount > 0 ? `${assignedCount} / ${deployableFleet.length} VPS` : 'unassigned'}
          </span>
        </div>
        <div className="tlist__chips">
          {deployableFleet.length === 0 && activeFleet.length > 0 ? (
            <span className="muted-callout">Waiting for SSH enroll — {activeFleet.length} node(s) connecting…</span>
          ) : deployableFleet.length === 0 ? (
            <span className="muted-callout">No active VPS in fleet — add nodes or wait for scan hits.</span>
          ) : (
            activeFleet.map((node) => {
              const on = assignedSet.has(node.id)
              const canAssign = node.status === 'healthy' || node.status === 'degraded'
              return (
                <button
                  key={node.id}
                  type="button"
                  className={`tlist-chip${on ? ' tlist-chip--on' : ''}`}
                  disabled={!canAssign}
                  onClick={() => canAssign && onToggleVps(list.id, node.id)}
                  title={`${node.region} · ${node.host}${node.source === 'discovered' ? ' · discovered' : ''}`}
                >
                  <span className={`tlist-chip__dot tlist-chip__dot--${node.status}`} aria-hidden />
                  {node.label}
                  {node.source === 'discovered' && <span className="tlist-chip__disc">disc</span>}
                  {on && (
                    <span className="tlist-chip__lines mono">
                      {Math.floor(list.lineCount / assignedCount).toLocaleString()}
                    </span>
                  )}
                </button>
              )
            })
          )}
        </div>
      </div>

      <footer className="tlist__actions">
        {list.status === 'idle' || list.status === 'failed' ? (
          <button
            type="button"
            className="btn-primary btn-glass btn-with-ico"
            disabled={assignedCount === 0}
            onClick={() => onDeploy(list.id)}
            title={assignedCount === 0 ? 'Assign at least one VPS first' : 'Push this list to the assigned VPSes'}
          >
            <PlayIcon /> Deploy
          </button>
        ) : null}
        {list.status === 'deployed' && (
          <>
            <button type="button" className="btn-glass btn-with-ico" onClick={() => onPause(list.id)}>
              <PauseIcon /> Pause
            </button>
            <button type="button" className="btn-glass btn-with-ico" onClick={() => onComplete(list.id)}>
              <CheckIcon /> Mark complete
            </button>
          </>
        )}
        {list.status === 'queued' && (
          <button type="button" className="btn-glass btn-with-ico" onClick={() => onDeploy(list.id)}>
            <PlayIcon /> Start now
          </button>
        )}
        {(list.status === 'completed' || list.status === 'deployed') && (
          <button type="button" className="btn-glass btn-with-ico" onClick={() => onReset(list.id)}>
            <ResetIcon /> Reset
          </button>
        )}
        <span className="tlist__actions-spacer" />
        {confirmDelete ? (
          <>
            <span className="muted">delete this list?</span>
            <button type="button" className="btn-danger-outline" onClick={() => onDelete(list.id)}>
              Yes, delete
            </button>
            <button type="button" className="btn-glass" onClick={() => setConfirmDelete(false)}>
              Cancel
            </button>
          </>
        ) : (
          <button type="button" className="btn-glass btn-glass--danger" onClick={() => setConfirmDelete(true)}>
            🗑 Delete
          </button>
        )}
      </footer>
    </article>
  )
}
