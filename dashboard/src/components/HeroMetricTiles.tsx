type Props = {
  totalHits: number
  cracks: number
  liveDomains: number
}

function IcoCrosshair() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 2v4M12 18v4M2 12h4M18 12h4" strokeLinecap="round" />
    </svg>
  )
}

function IcoPlanet() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
      <circle cx="12" cy="12" r="5" />
      <path d="M4 12c3-2 13-2 16 0" strokeLinecap="round" />
      <path d="M4 12c3 2 13 2 16 0" strokeLinecap="round" />
    </svg>
  )
}

export function HeroMetricTiles({ totalHits, cracks, liveDomains }: Props) {
  return (
    <div className="hero-metrics">
      <article className="hero-tile">
        <div className="hero-tile__ico" aria-hidden>
          <IcoCrosshair />
        </div>
        <div className="hero-tile__body">
          <span className="hero-tile__label">Total hits</span>
          <span className="hero-tile__value">{totalHits.toLocaleString()}</span>
          <span className="hero-tile__sub">All scanned credentials</span>
        </div>
      </article>
      <article className="hero-tile hero-tile--accent">
        <div className="hero-tile__ico" aria-hidden>
          <IcoPlanet />
        </div>
        <div className="hero-tile__body">
          <span className="hero-tile__label">Cracks</span>
          <span className="hero-tile__value">{cracks.toLocaleString()}</span>
          <span className="hero-tile__sub">API-validated hits</span>
        </div>
      </article>
      <article className="hero-tile hero-tile--teal">
        <div className="hero-tile__ico" aria-hidden>
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6">
            <path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1" strokeLinecap="round" />
            <circle cx="12" cy="12" r="4" />
          </svg>
        </div>
        <div className="hero-tile__body">
          <span className="hero-tile__label">Live hosts</span>
          <span className="hero-tile__value">{liveDomains.toLocaleString()}</span>
          <span className="hero-tile__sub">Active fleet nodes</span>
        </div>
      </article>
    </div>
  )
}
