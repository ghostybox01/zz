import { useEffect, useState } from 'react'
import { loadScannerLimits, saveScannerLimits, type ScannerLimitsPrefs } from '../lib/scannerLimitsPrefs'

const PRESETS: Array<{ label: string; cron: string }> = [
  { label: 'Every 6 hours',  cron: '0 */6 * * *' },
  { label: 'Every 12 hours', cron: '0 */12 * * *' },
  { label: 'Daily at 04:00', cron: '0 4 * * *' },
  { label: 'Weekly (Sun 03:00)', cron: '0 3 * * 0' },
]

export function ScheduleSettings() {
  const [prefs, setPrefs] = useState<ScannerLimitsPrefs>(() => loadScannerLimits())

  useEffect(() => {
    saveScannerLimits(prefs)
  }, [prefs])

  return (
    <section className="card-block card-block--tight settings-section">
      <div className="card-block__head">
        <h2>Scheduled re-scans</h2>
        <p className="card-block__lede card-block__lede--short">
          Cron expression evaluated on the controller. Workers re-run the most recent target list each tick.
        </p>
      </div>

      <label className="tg-toggle">
        <input
          type="checkbox"
          checked={prefs.scheduleEnabled}
          onChange={(e) => setPrefs({ ...prefs, scheduleEnabled: e.target.checked })}
        />
        <span>
          Enable scheduled re-scans <span className="tg-muted">(operator must install the cron entry)</span>
        </span>
      </label>

      <div className="settings-grid" style={{ marginTop: '0.5rem' }}>
        <label className="tg-field tg-field--wide">
          <span>Cron expression</span>
          <input
            className="tg-input"
            type="text"
            value={prefs.scheduleCron}
            onChange={(e) => setPrefs({ ...prefs, scheduleCron: e.target.value })}
            disabled={!prefs.scheduleEnabled}
            placeholder="0 */6 * * *"
          />
        </label>
      </div>

      <div className="schedule-presets" style={{ display: 'flex', gap: '.4rem', flexWrap: 'wrap', marginTop: '.6rem' }}>
        {PRESETS.map((p) => (
          <button
            key={p.cron}
            type="button"
            className="btn-glass btn-glass--xs"
            disabled={!prefs.scheduleEnabled}
            onClick={() => setPrefs({ ...prefs, scheduleCron: p.cron })}
          >
            {p.label}
          </button>
        ))}
      </div>
    </section>
  )
}
