type Props = {
  title: string
  value: string
  hint?: string
  accent?: 'default' | 'green' | 'amber'
}

export function StatCard({ title, value, hint, accent = 'default' }: Props) {
  return (
    <article className={`stat-card stat-card--${accent}`}>
      <h3>{title}</h3>
      <p className="stat-card__value">{value}</p>
      {hint ? <p className="stat-card__hint">{hint}</p> : null}
    </article>
  )
}
