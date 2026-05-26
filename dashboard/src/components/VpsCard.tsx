import { useState } from 'react'
import type { TargetList, VpsNode } from '../types'
import { VPS_MAX_RECONNECT_TRIES } from '../types'
import { hasScanBacklog, scanningListLabel, scanListCaption, vpsWorkloadState, workloadLabel } from '../lib/vpsWorkload'
import { CpuRing } from './CpuRing'
import { CpuSparkline } from './CpuSparkline'
import { ProgressBar } from './ProgressBar'

export type VpsNodeAction = 'start' | 'stop' | 'restart' | 'reconnect' | 'test-ssh'

type Props = {
  node: VpsNode
  lists?: readonly TargetList[]
  onForceOutage?: (id: string, mode: 'offline' | 'reconnect') => void
  /** Live-backend action handler. Receives the node's host IP. Should resolve with a short status message. */
  onAction?: (ip: string, action: VpsNodeAction) => Promise<{ ok: boolean; message?: string }>
}

function statusLabel(s: VpsNode['status']): string {
  switch (s) {
    case 'healthy': return 'HEALTHY'
    case 'degraded': return 'DEGRADED'
    case 'reconnecting': return 'RECONNECT'
    case 'offline': return 'OFFLINE'
    case 'removed': return 'PRUNED'
  }
}

function fmtUptime(min: number): string {
  if (!Number.isFinite(min) || min <= 0) return '—'
  if (min < 60) return `${Math.round(min)}m`
  const h = Math.floor(min / 60)
  const m = Math.round(min % 60)
  return `${h}h ${m}m`
}

export function VpsCard({ node, lists = [], onForceOutage, onAction }: Props) {
  const removed = node.status === 'removed'
  const workload = vpsWorkloadState(node, lists)
  const scanList = scanningListLabel(node, lists)
  const scanCaption = scanList ? scanListCaption(node, lists) : null
  // If backend overlays a crack session name onto activeListName but there's no WARC scan
  // backlog, the worker is cracking (not scanning). Show a distinct label.
  const isCracking = workload === 'busy' && !!node.activeListName && !hasScanBacklog(node)
  const ramRatio = node.ramTotalGb > 0 ? node.ramUsedGb / node.ramTotalGb : 0
  const ramTone = ramRatio >= 0.85 ? 'danger' : ramRatio >= 0.7 ? 'orange' : 'ok'
  const diskTotal = node.diskTotalGb ?? Math.max(node.diskUsedGb * 4, 80)
  const [pending, setPending] = useState<VpsNodeAction | null>(null)
  const [actionMessage, setActionMessage] = useState<string | null>(null)

  const runAction = async (action: VpsNodeAction) => {
    if (!onAction || pending) return
    setPending(action)
    setActionMessage(null)
    try {
      const r = await onAction(node.host, action)
      setActionMessage(r.message ?? (r.ok ? 'ok' : 'failed'))
      if (!r.ok) window.setTimeout(() => setActionMessage(null), 4500)
      else window.setTimeout(() => setActionMessage(null), 2500)
    } catch (e) {
      setActionMessage((e as Error).message ?? 'error')
      window.setTimeout(() => setActionMessage(null), 4500)
    } finally {
      setPending(null)
    }
  }

  return (
    <article className={`fnode fnode--${node.status}`}>
      <header className="fnode__head">
        <div className="fnode__title-row">
          <h3 className="fnode__name" title={node.label}>
            {node.label}
          </h3>
          {node.source === 'discovered' && (
            <span className="fnode__discovered" title="Auto-enrolled from scan hit">
              DISC
            </span>
          )}
        </div>
        <span className={`fnode__status fnode__status--${node.status}`}>
          <span className="fnode__status-dot" aria-hidden />
          {statusLabel(node.status)}
        </span>
      </header>

      <div className="fnode__meta">
        <span className="fnode__tag">{node.region}</span>
        <span className="fnode__host mono">{node.host}</span>
      </div>

      <div className="fnode__workload" aria-label="Worker load">
        <span className={`fnode__load fnode__load--${workload}`}>
          <span className="fnode__load-dot" aria-hidden />
          {isCracking ? 'CRACKING' : workloadLabel(workload)}
        </span>
        {workload === 'busy' && scanList ? (
          <span className="fnode__scan-list" title={scanList}>
            <span className="fnode__scan-list-k">{isCracking ? 'Cracking' : scanCaption}</span>
            <span className="fnode__scan-list-name mono">{scanList}</span>
          </span>
        ) : workload === 'free' ? (
          <span className="fnode__scan-list fnode__scan-list--idle muted">
            {node.activeListName ?? 'No active list'}
          </span>
        ) : null}
      </div>

      {!removed && (
        <>
          <div className="fnode__cpu">
            <CpuRing percent={node.cpuPercent} />
            <div className="fnode__cpu-trend">
              <div className="fnode__cpu-label">
                <span className="muted">CPU trend</span>
                <span className="mono">{node.scansPerSecond.toFixed(1)} probes/s</span>
              </div>
              <div className="fnode__cpu-spark" style={{ color: node.cpuPercent >= 90 ? 'var(--danger)' : node.cpuPercent >= 70 ? '#ff8a3d' : 'var(--accent)' }}>
                <CpuSparkline values={node.cpuHistory ?? [node.cpuPercent]} />
              </div>
            </div>
          </div>

          {node.targetsAssigned > 0 && (
            <div className="fnode__block">
              <div className="fnode__row">
                <span className="muted">Targets</span>
                <span className="mono">
                  {node.targetsDone.toLocaleString()} / {node.targetsAssigned.toLocaleString()}
                </span>
              </div>
              <ProgressBar
                current={node.targetsDone}
                total={node.targetsAssigned}
                tone={node.status === 'degraded' ? 'warn' : 'accent'}
              />
            </div>
          )}

          {node.ramTotalGb > 0 && (
            <div className="fnode__block">
              <div className="fnode__row">
                <span className="muted">RAM</span>
                <span className="mono">{node.ramUsedGb.toFixed(1)} / {node.ramTotalGb} GB</span>
              </div>
              <ProgressBar current={node.ramUsedGb} total={node.ramTotalGb} tone={ramTone} thin />
            </div>
          )}

          {diskTotal > 0 && (
            <div className="fnode__block">
              <div className="fnode__row">
                <span className="muted">Disk</span>
                <span className="mono">{node.diskUsedGb} / {diskTotal} GB</span>
              </div>
              <ProgressBar current={node.diskUsedGb} total={diskTotal} tone="ok" thin />
            </div>
          )}

          <footer className="fnode__foot">
            <div className="fnode__stat">
              <span className="fnode__stat-label">FINDINGS</span>
              <strong>{(node.findingsContributed ?? 0).toLocaleString()}</strong>
            </div>
            <div className="fnode__stat">
              <span className="fnode__stat-label">UPTIME</span>
              <strong>{fmtUptime(node.uptimeMin ?? 0)}</strong>
            </div>
            <div className="fnode__stat">
              <span className="fnode__stat-label">SSH RETRIES</span>
              <strong className={node.reconnectFailCount > 0 ? 'danger-text' : ''}>
                {node.reconnectFailCount}/{VPS_MAX_RECONNECT_TRIES}
              </strong>
            </div>
          </footer>

          {node.lastEvent && <p className="fnode__event">{node.lastEvent}</p>}

          {onAction && (
            <div className="fnode__controls">
              <button
                type="button"
                className="btn-glass btn-glass--xs"
                disabled={pending !== null}
                onClick={() => void runAction('start')}
                title="POST /api/vps/server/<ip>/start"
              >
                {pending === 'start' ? '…' : '▶ Start'}
              </button>
              <button
                type="button"
                className="btn-glass btn-glass--xs"
                disabled={pending !== null}
                onClick={() => void runAction('stop')}
                title="POST /api/vps/server/<ip>/stop"
              >
                {pending === 'stop' ? '…' : '■ Stop'}
              </button>
              <button
                type="button"
                className="btn-glass btn-glass--xs"
                disabled={pending !== null}
                onClick={() => void runAction('restart')}
                title="POST /api/vps/server/<ip>/restart"
              >
                {pending === 'restart' ? '…' : '↻ Restart'}
              </button>
              <button
                type="button"
                className="btn-glass btn-glass--xs"
                disabled={pending !== null}
                onClick={() => void runAction('reconnect')}
                title="POST /api/vps/server/<ip>/fix — self-heal SSH + scanner"
              >
                {pending === 'reconnect' ? '…' : '⟲ Reconnect'}
              </button>
              <button
                type="button"
                className="btn-glass btn-glass--xs"
                disabled={pending !== null}
                onClick={() => void runAction('test-ssh')}
                title="GET /api/vps/server/<ip>/test — probe SSH only"
              >
                {pending === 'test-ssh' ? '…' : '⚡ SSH Test'}
              </button>
              {actionMessage && <span className="fnode__action-msg muted">{actionMessage}</span>}
            </div>
          )}

          {!onAction && onForceOutage && (
            <div className="fnode__controls">
              <button type="button" className="btn-glass btn-glass--xs" onClick={() => onForceOutage(node.id, 'offline')}>
                Simulate drop
              </button>
              <button type="button" className="btn-glass btn-glass--xs" onClick={() => onForceOutage(node.id, 'reconnect')}>
                Flap SSH
              </button>
            </div>
          )}
        </>
      )}

      {removed && (
        <>
          <footer className="fnode__foot fnode__foot--compact">
            <div className="fnode__stat">
              <span className="fnode__stat-label">FINDINGS</span>
              <strong>{(node.findingsContributed ?? 0).toLocaleString()}</strong>
            </div>
            <div className="fnode__stat">
              <span className="fnode__stat-label">UPTIME</span>
              <strong>{fmtUptime(node.uptimeMin ?? 0)}</strong>
            </div>
          </footer>
          <p className="fnode__event fnode__event--removed">⚠ {node.removedReason ?? node.lastEvent}</p>
        </>
      )}
    </article>
  )
}
