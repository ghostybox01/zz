import { useEffect, useState } from 'react'
import { loadScannerLimits, saveScannerLimits, type ScannerLimitsPrefs } from '../lib/scannerLimitsPrefs'

const SEVERITIES: ScannerLimitsPrefs['notifyMinSeverity'][] = ['low', 'medium', 'high', 'critical']

export function NotificationsSettings() {
  const [prefs, setPrefs] = useState<ScannerLimitsPrefs>(() => loadScannerLimits())

  useEffect(() => {
    saveScannerLimits(prefs)
  }, [prefs])

  return (
    <section className="card-block card-block--tight settings-section">
      <div className="card-block__head">
        <h2>External notifications</h2>
        <p className="card-block__lede card-block__lede--short">
          Where the workers send each finding besides the Hits ledger. The dashboard records intent; outbound delivery still happens from your scanner box.
        </p>
      </div>

      <div className="settings-grid">
        <label className="tg-field tg-field--wide">
          <span>Generic webhook URL <span className="tg-muted">(POST JSON for every finding)</span></span>
          <input
            className="tg-input"
            type="url"
            value={prefs.notifyWebhookUrl}
            placeholder="https://your-collector.example.com/hooks/reconx"
            onChange={(e) => setPrefs({ ...prefs, notifyWebhookUrl: e.target.value })}
          />
        </label>

        <label className="tg-field tg-field--wide">
          <span>Slack incoming-webhook URL</span>
          <input
            className="tg-input"
            type="url"
            value={prefs.notifySlackUrl}
            placeholder="https://hooks.slack.com/services/T0/B0/abc"
            onChange={(e) => setPrefs({ ...prefs, notifySlackUrl: e.target.value })}
          />
        </label>

        <label className="tg-field">
          <span>Minimum severity to notify</span>
          <select
            className="tg-input"
            value={prefs.notifyMinSeverity}
            onChange={(e) => setPrefs({ ...prefs, notifyMinSeverity: e.target.value as ScannerLimitsPrefs['notifyMinSeverity'] })}
          >
            {SEVERITIES.map((s) => (
              <option key={s} value={s}>
                {s} and above
              </option>
            ))}
          </select>
        </label>
      </div>
    </section>
  )
}
