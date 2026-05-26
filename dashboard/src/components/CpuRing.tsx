type Props = {
  percent: number
  size?: number
  thickness?: number
  /** Color override; defaults to load-based gradient (green → yellow → orange → red). */
  color?: string
}

/** Radial CPU load gauge. Color shifts by load: green < 70 < yellow/orange < 90 < red. */
export function CpuRing({ percent, size = 56, thickness = 5, color }: Props) {
  const p = Math.max(0, Math.min(100, percent))
  const r = (size - thickness) / 2
  const c = 2 * Math.PI * r
  const offset = c * (1 - p / 100)
  const tone =
    color ??
    (p >= 90
      ? 'var(--danger)'
      : p >= 70
        ? '#ff8a3d'
        : 'var(--ok)')

  return (
    <div className="cpuring" style={{ width: size, height: size }} aria-label={`CPU ${Math.round(p)}%`}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="rgba(255, 255, 255, 0.06)" strokeWidth={thickness} />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={tone}
          strokeWidth={thickness}
          strokeDasharray={c}
          strokeDashoffset={offset}
          strokeLinecap="round"
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
          style={{ transition: 'stroke-dashoffset 600ms ease, stroke 200ms ease' }}
        />
      </svg>
      <span className="cpuring__pct" style={{ color: tone }}>
        {Math.round(p)}
        <small>%</small>
      </span>
    </div>
  )
}
