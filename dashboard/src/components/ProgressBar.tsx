type Props = {
  current: number
  total: number
  /** Color tone. */
  tone?: 'accent' | 'ok' | 'warn' | 'danger' | 'gold' | 'orange'
  /** Compact height. */
  thin?: boolean
}

const TONE_MAP: Record<NonNullable<Props['tone']>, string> = {
  accent: 'var(--accent)',
  ok: 'var(--ok)',
  warn: 'var(--warn)',
  danger: 'var(--danger)',
  gold: 'var(--gold)',
  orange: '#ff8a3d',
}

export function ProgressBar({ current, total, tone = 'accent', thin }: Props) {
  const pct = total > 0 ? Math.min(100, (current / total) * 100) : 0
  return (
    <div className={`pb${thin ? ' pb--thin' : ''}`}>
      <div
        className="pb__fill"
        style={{ width: `${pct.toFixed(2)}%`, background: TONE_MAP[tone] }}
        aria-valuenow={Math.round(pct)}
        aria-valuemin={0}
        aria-valuemax={100}
        role="progressbar"
      />
    </div>
  )
}
