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
        <h2>Notification filters</h2>
        <p className="card-block__lede card-block__lede--short">
          Telegram is the only outbound channel — configure the bot in the Telegram panel above. This card just
          decides <em>which</em> findings are loud enough to fire a ping.
        </p>
      </div>

      <div className="settings-grid">
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
