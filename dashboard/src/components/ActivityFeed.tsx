import { useMemo } from 'react'
import { findingCredentialText } from '../lib/findingCredential'
import type { Finding } from '../types'

type Props = {
  findings: readonly Finding[]
  liveLabel: string
}

export function ActivityFeed({ findings, liveLabel }: Props) {
  const items = useMemo(() => {
    const recent = [...findings]
      .sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime())
      .slice(0, 6)
    return recent.map((f) => {
      const cred = findingCredentialText(f)
      return {
        id: f.id,
        t: new Date(f.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
        title: `${f.provider} · ${f.ruleLabel}`,
        sub: f.hostname,
        cred: cred.length > 0 ? cred : null,
        tone: f.severity === 'critical' || f.severity === 'high' ? 'hot' : 'calm',
      }
    })
  }, [findings])

  return (
    <aside className="activity-feed">
      <div className="activity-feed__head">
        <h3>Signal log</h3>
        <span className="activity-feed__src">{liveLabel}</span>
      </div>
      {items.length === 0 ? (
        <p className="activity-feed__empty">Quiet channel — enable live source or seed demo hits.</p>
      ) : (
        <ul className="activity-feed__list">
          {items.map((it) => (
            <li key={it.id} className={`activity-feed__item activity-feed__item--${it.tone}`}>
              <span className="activity-feed__time">{it.t}</span>
              <span className="activity-feed__title">{it.title}</span>
              <span className="activity-feed__sub">{it.sub}</span>
              {it.cred ? (
                <span className="activity-feed__cred mono" title={it.cred}>
                  {it.cred}
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </aside>
  )
}
