import { useEffect, useState } from 'react'
import { loadScannerLimits, saveScannerLimits, type ScannerLimitsPrefs } from '../lib/scannerLimitsPrefs'

export function ScannerLimitsSettings() {
  const [prefs, setPrefs] = useState<ScannerLimitsPrefs>(() => loadScannerLimits())

  useEffect(() => {
    saveScannerLimits(prefs)
  }, [prefs])

  function patch<K extends keyof ScannerLimitsPrefs>(k: K, v: ScannerLimitsPrefs[K]) {
    setPrefs((p) => ({ ...p, [k]: v }))
  }

  return (
    <section className="card-block card-block--tight settings-section">
      <div className="card-block__head">
        <h2>Scanner limits &amp; behaviour</h2>
        <p className="card-block__lede card-block__lede--short">
          Tuning knobs for the Go scanner. Stored in your browser; the next deploy
          can push them to <code>config.json</code> on each VPS.
        </p>
      </div>

      <div className="settings-grid">
        <label className="tg-field">
          <span>HTTP timeout (ms)</span>
          <input
            className="tg-input"
            type="number"
            min={1000}
            step={500}
            value={prefs.httpTimeoutMs}
            onChange={(e) => patch('httpTimeoutMs', Number(e.target.value) || DEFAULT_TIMEOUT)}
          />
        </label>

        <label className="tg-field">
          <span>Max concurrent fetches per VPS</span>
          <input
            className="tg-input"
            type="number"
            min={1}
            max={500}
            value={prefs.maxConcurrency}
            onChange={(e) => patch('maxConcurrency', Number(e.target.value) || 1)}
          />
        </label>

        <label className="tg-field">
          <span>Per-host rate cap (requests/minute)</span>
          <input
            className="tg-input"
            type="number"
            min={1}
            max={6000}
            value={prefs.perHostRpm}
            onChange={(e) => patch('perHostRpm', Number(e.target.value) || 1)}
          />
        </label>

        <label className="tg-field">
          <span>Result retention (days)</span>
          <input
            className="tg-input"
            type="number"
            min={1}
            max={365}
            value={prefs.saveRetentionDays}
            onChange={(e) => patch('saveRetentionDays', Number(e.target.value) || 1)}
          />
        </label>

        <label className="tg-field tg-field--wide">
          <span>HTTP User-Agent</span>
          <input
            className="tg-input"
            type="text"
            value={prefs.userAgent}
            onChange={(e) => patch('userAgent', e.target.value)}
            placeholder="Mozilla/5.0 (compatible; ReconX/1.0)"
          />
        </label>

        <label className="tg-field">
          <span>Default export format</span>
          <select
            className="tg-input"
            value={prefs.exportFormat}
            onChange={(e) => patch('exportFormat', e.target.value as ScannerLimitsPrefs['exportFormat'])}
          >
            <option value="json">JSON (default)</option>
            <option value="csv">CSV</option>
            <option value="txt">Plain text</option>
          </select>
        </label>

        <label className="tg-toggle">
          <input
            type="checkbox"
            checked={prefs.followRedirects}
            onChange={(e) => patch('followRedirects', e.target.checked)}
          />
          <span>
            Follow HTTP redirects <span className="tg-muted">(disable to detect 30x leaks intentionally)</span>
          </span>
        </label>
      </div>
    </section>
  )
}

const DEFAULT_TIMEOUT = 15_000
