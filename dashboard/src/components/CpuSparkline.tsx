type SparkProps = {
  values: readonly number[] | undefined
  className?: string
}

/** Normalized polyline sparkline for rolling CPU samples. */
export function CpuSparkline({ values, className }: SparkProps) {
  const pts = values?.length ? [...values] : [0]
  const min = Math.min(...pts)
  const max = Math.max(...pts)
  const span = max === min ? 1 : max - min
  const w = 100
  const h = 26
  const padY = 3
  const coords = pts.map((v, i) => {
    const x = pts.length === 1 ? w / 2 : (i / (pts.length - 1)) * w
    const y = h - padY - ((v - min) / span) * (h - padY * 2)
    return `${x.toFixed(2)},${y.toFixed(2)}`
  })
  const d = `M ${coords.join(' L ')}`
  return (
    <svg
      className={className}
      viewBox={`0 0 ${w} ${h}`}
      preserveAspectRatio="none"
      aria-hidden
    >
      <path
        d={d}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
        opacity={0.85}
      />
    </svg>
  )
}

type RingProps = {
  percent: number
  className?: string
}

const R = 13
const C = 2 * Math.PI * R

/** Thin SVG ring showing instantaneous CPU load. */
export function CpuRingMini({ percent, className }: RingProps) {
  const p = Math.min(100, Math.max(0, percent))
  const dash = (p / 100) * C
  return (
    <svg
      className={className}
      width={34}
      height={34}
      viewBox="0 0 34 34"
      aria-hidden
    >
      <circle
        cx={17}
        cy={17}
        r={R}
        fill="none"
        stroke="var(--hairline)"
        strokeWidth={3}
      />
      <circle
        cx={17}
        cy={17}
        r={R}
        fill="none"
        stroke="currentColor"
        strokeWidth={3}
        strokeLinecap="round"
        strokeDasharray={`${dash} ${C}`}
        transform="rotate(-90 17 17)"
      />
    </svg>
  )
}
