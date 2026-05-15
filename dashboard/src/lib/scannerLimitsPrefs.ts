/** Local scanner-limits preferences. Persisted to localStorage on every change.
 *  These are advisory: the Go scanner reads its own config.json, but the dashboard
 *  surfaces them so the operator records intent and (later) we can push them via
 *  /api/scanner-config. */

const KEY = 'reconx.scannerLimits.v1'

export type ScannerLimitsPrefs = {
  httpTimeoutMs: number
  maxConcurrency: number
  perHostRpm: number
  userAgent: string
  followRedirects: boolean
  saveRetentionDays: number
  exportFormat: 'json' | 'csv' | 'txt'
  notifyWebhookUrl: string
  notifySlackUrl: string
  notifyMinSeverity: 'low' | 'medium' | 'high' | 'critical'
  scheduleEnabled: boolean
  scheduleCron: string
}

const DEFAULTS: ScannerLimitsPrefs = {
  httpTimeoutMs: 15_000,
  maxConcurrency: 50,
  perHostRpm: 60,
  userAgent: 'Mozilla/5.0 (compatible; ReconX/1.0)',
  followRedirects: true,
  saveRetentionDays: 30,
  exportFormat: 'json',
  notifyWebhookUrl: '',
  notifySlackUrl: '',
  notifyMinSeverity: 'high',
  scheduleEnabled: false,
  scheduleCron: '0 */6 * * *',
}

export function loadScannerLimits(): ScannerLimitsPrefs {
  try {
    const raw = window.localStorage.getItem(KEY)
    if (!raw) return { ...DEFAULTS }
    const parsed = JSON.parse(raw)
    return { ...DEFAULTS, ...parsed }
  } catch {
    return { ...DEFAULTS }
  }
}

export function saveScannerLimits(prefs: ScannerLimitsPrefs): void {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(prefs))
  } catch {
    /* swallow */
  }
}
