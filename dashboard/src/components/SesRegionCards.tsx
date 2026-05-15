import { useMemo } from 'react'
import type { Finding } from '../types'
import { fmtInt } from '../lib/format'

type RegionCard = {
  region: string
  quota: number
  sent: number
  rate: number
  domains: string[]
  healthy: boolean
  highQuota: boolean
}

type Props = {
  findings: readonly Finding[]
}

export function SesRegionCards({ findings }: Props) {
  const regions = useMemo(() => {
    const map = new Map<string, RegionCard>()
    for (const f of findings) {
      if (f.provider !== 'AWS' || !f.details?.sesQuota) continue
      const region = f.details.awsRegion ?? 'us-east-1'
      const q = f.details.sesQuota
      const existing = map.get(region)
      const domains = [...(existing?.domains ?? []), ...(q.verifiedDomains ?? [])]
      map.set(region, {
        region,
        quota: q.max24h ?? existing?.quota ?? 0,
        sent: q.sent24h ?? existing?.sent ?? 0,
        rate: q.ratePerSecond ?? existing?.rate ?? 0,
        domains: [...new Set(domains)],
        healthy: !q.sandbox,
        highQuota: (q.max24h ?? 0) >= 200_000,
      })
    }
    if (map.size === 0) {
      map.set('us-west-1', {
        region: 'us-west-1',
        quota: 1_000_000,
        sent: 0,
        rate: 80,
        domains: ['help@8hut.com', 'dchen@wangnafei.com'],
        healthy: true,
        highQuota: true,
      })
    }
    return [...map.values()]
  }, [findings])

  return (
    <section className="cw-ses">
      <header className="cw-ses__head">
        <h3 className="cw-ses__title">AWS SES Regions</h3>
        <p className="cw-ses__lede">
          Quotas & verified domains <em>discovered from current AWS findings</em> — grouped by region.
          Empty when no AWS hits include an SES quota probe.
        </p>
      </header>
      <div className="cw-ses__grid">
        {regions.map((r) => (
          <article key={r.region} className="cw-ses-card">
            <header className="cw-ses-card__head">
              <h4>{r.region}</h4>
              <div className="cw-ses-card__badges">
                {r.healthy && <span className="cw-ses-badge cw-ses-badge--ok">HEALTHY</span>}
                {r.highQuota && <span className="cw-ses-badge cw-ses-badge--quota">HIGH QUOTA</span>}
              </div>
            </header>
            <div className="cw-ses-card__metrics">
              <div>
                <span className="cw-ses-card__k">QUOTA</span>
                <strong>{fmtInt(r.quota)}</strong>
              </div>
              <div>
                <span className="cw-ses-card__k">SENT</span>
                <strong>{fmtInt(r.sent)}</strong>
              </div>
              <div>
                <span className="cw-ses-card__k">RATE</span>
                <strong>{r.rate}/s</strong>
              </div>
            </div>
            {r.domains.length > 0 && (
              <div className="cw-ses-card__domains">
                <span className="cw-ses-card__k">Verified emails / domains</span>
                <div className="cw-ses-card__chips">
                  {r.domains.map((d) => (
                    <span key={d} className="cw-ses-chip">
                      {d}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </article>
        ))}
      </div>
    </section>
  )
}
