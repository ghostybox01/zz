import { useMemo } from 'react'
import type { Finding } from '../types'

type Props = {
  findings: readonly Finding[]
  maxBars?: number
}

export function ProviderHeatstrip({ findings, maxBars = 8 }: Props) {
  const rows = useMemo(() => {
    const m = new Map<string, number>()
    for (const f of findings) m.set(f.provider, (m.get(f.provider) ?? 0) + 1)
    const sorted = [...m.entries()].sort((a, b) => b[1] - a[1])
    const top = sorted.slice(0, maxBars)
    const max = top[0]?.[1] ?? 1
    return top.map(([name, n]) => ({ name, n, pct: Math.round((n / max) * 100) }))
  }, [findings, maxBars])

  if (rows.length === 0) {
    return (
      <div className="heatstrip heatstrip--empty">
        <span className="heatstrip__title">Addon mix</span>
        <p className="heatstrip__empty">No hits yet — heat map fills as findings arrive.</p>
      </div>
    )
  }

  return (
    <div className="heatstrip">
      <div className="heatstrip__head">
        <span className="heatstrip__title">Addon mix</span>
        <span className="heatstrip__hint">Top providers by volume</span>
      </div>
      <ul className="heatstrip__list">
        {rows.map((r) => (
          <li key={r.name} className="heatstrip__row">
            <span className="heatstrip__name">{r.name}</span>
            <div className="heatstrip__track" title={`${r.n} hits`}>
              <div className="heatstrip__fill" style={{ width: `${r.pct}%` }} />
            </div>
            <span className="heatstrip__num">{r.n}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
