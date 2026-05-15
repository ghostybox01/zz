import { useMemo, useState } from 'react'
import type { Scan, ScanShard } from '../types'
import { ScanCard } from './ScanCard'

type Filter = 'all' | 'running' | 'paused' | 'done'

type Props = {
  scans: readonly Scan[]
  shards: readonly ScanShard[]
  onOpen: (scanId: string) => void
}

export function ScanList({ scans, shards, onOpen }: Props) {
  const [filter, setFilter] = useState<Filter>('all')

  const shardCounts = useMemo(() => {
    const m = new Map<string, number>()
    for (const sh of shards) m.set(sh.scanId, (m.get(sh.scanId) ?? 0) + 1)
    return m
  }, [shards])

  const filtered = useMemo(() => {
    if (filter === 'all') return scans
    return scans.filter((s) => s.status === filter)
  }, [scans, filter])

  const counts = useMemo(() => {
    const tally = { all: scans.length, running: 0, paused: 0, done: 0 }
    for (const s of scans) {
      if (s.status === 'running') tally.running++
      else if (s.status === 'paused') tally.paused++
      else if (s.status === 'done') tally.done++
    }
    return tally
  }, [scans])

  return (
    <section className="card-block card-block--tight">
      <div className="card-block__head card-block__head--row">
        <div>
          <h2>Active scans</h2>
          <p className="card-block__lede card-block__lede--short">
            Each card is a target list × scanner-config job fanned across the fleet. Click to drill into per-shard breakdown.
          </p>
        </div>
        <div className="scan-filters" role="tablist" aria-label="Scan status filter">
          {(['all', 'running', 'paused', 'done'] as const).map((f) => (
            <button
              key={f}
              type="button"
              role="tab"
              aria-selected={filter === f}
              className={`scan-filter${filter === f ? ' scan-filter--active' : ''}`}
              onClick={() => setFilter(f)}
            >
              {f}
              <span className="scan-filter__count">{counts[f]}</span>
            </button>
          ))}
        </div>
      </div>

      {filtered.length === 0 ? (
        <p className="muted-callout">No scans match this lens.</p>
      ) : (
        <div className="scan-grid">
          {filtered.map((s) => (
            <ScanCard
              key={s.id}
              scan={s}
              shardCount={shardCounts.get(s.id) ?? 0}
              onOpen={() => onOpen(s.id)}
            />
          ))}
        </div>
      )}
    </section>
  )
}
