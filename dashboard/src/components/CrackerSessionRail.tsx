import { useState } from 'react'
import type { Scan } from '../types'

type Props = {
  scans: readonly Scan[]
  activeId: string | null
  onSelect: (id: string) => void
  onNew: () => void
  onTogglePause?: (id: string) => void
  onStop?: (id: string) => void
}

function PauseIcon() {
  return (
    <svg viewBox="0 0 24 24" width="13" height="13" fill="currentColor" aria-hidden>
      <rect x="6" y="5" width="4" height="14" rx="1" />
      <rect x="14" y="5" width="4" height="14" rx="1" />
    </svg>
  )
}
function PlayIcon() {
  return (
    <svg viewBox="0 0 24 24" width="13" height="13" fill="currentColor" aria-hidden>
      <path d="M7 5v14l12-7L7 5z" />
    </svg>
  )
}
function StopIcon() {
  return (
    <svg viewBox="0 0 24 24" width="13" height="13" fill="currentColor" aria-hidden>
      <rect x="6" y="6" width="12" height="12" rx="1.5" />
    </svg>
  )
}

export function CrackerSessionRail({ scans, activeId, onSelect, onNew, onTogglePause, onStop }: Props) {
  const running = scans.filter((s) => s.status === 'running' || s.status === 'paused' || s.status === 'queued')
  const [expandedId, setExpandedId] = useState<string | null>(null)

  return (
    <aside className="cw-rail">
      <button type="button" className="cw-rail__new" onClick={onNew}>
        + New Crack
      </button>
      <ul className="cw-rail__list">
        {running.map((scan) => {
          const active = scan.id === activeId
          const expanded = expandedId === scan.id
          const short = scan.label.split('·')[0]?.trim() ?? scan.label
          return (
            <li key={scan.id} className="cw-rail__li">
              <button
                type="button"
                className={`cw-rail__item${active ? ' cw-rail__item--active' : ''}${expanded ? ' cw-rail__item--expanded' : ''}`}
                onClick={() => {
                  onSelect(scan.id)
                  setExpandedId(expanded ? null : scan.id)
                }}
              >
                <span className={`cw-rail__pulse cw-rail__pulse--${scan.status}`} aria-hidden />
                <span className="cw-rail__name">{short.toUpperCase()}</span>
                <span className="cw-rail__sub mono">#{scan.id.replace(/\D/g, '').slice(-4)}</span>
                <span className="cw-rail__chev" aria-hidden style={{ transform: expanded ? 'rotate(180deg)' : undefined }}>▾</span>
              </button>
              {expanded && (
                <div className="cw-rail__controls">
                  {onTogglePause && (
                    <button
                      type="button"
                      className="btn-glass btn-glass--xs"
                      onClick={(e) => {
                        e.stopPropagation()
                        onTogglePause(scan.id)
                      }}
                    >
                      {scan.status === 'running' ? <><PauseIcon /> Pause</> : <><PlayIcon /> Resume</>}
                    </button>
                  )}
                  {onStop && (
                    <button
                      type="button"
                      className="btn-glass btn-glass--xs btn-glass--danger"
                      onClick={(e) => {
                        e.stopPropagation()
                        onStop(scan.id)
                      }}
                    >
                      <StopIcon /> Stop
                    </button>
                  )}
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </aside>
  )
}
