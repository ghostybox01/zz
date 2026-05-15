type Props = {
  label: string
  current: number
  total: number
}

export function ProgressRow({ label, current, total }: Props) {
  const pct =
    total > 0 ? Math.min(100, Math.max(0, (current / total) * 100)) : 0
  return (
    <div className="progress-row">
      <div className="progress-row__head">
        <span>{label}</span>
        <span className="progress-row__nums">
          {current.toLocaleString()} / {total.toLocaleString()}
        </span>
      </div>
      <div className="progress-row__track" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
        <div className="progress-row__fill" style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}
