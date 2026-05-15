import type { RunSnapshot } from '../types'

export const demoSnapshot: RunSnapshot = {
  id: 'demo-local',
  label: 'Demo — WARC live check',
  startedAt: new Date(Date.now() - 42 * 60 * 1000).toISOString(),
  snapshots: ['CC-MAIN-2026-08', 'CC-MAIN-2026-04'],
  targetLiveDomains: 10000,
  liveDomains: 6842,
  totalExtracted: 184320,
  totalTested: 41290,
  filesProcessed: 842,
  filesTotal: 2400,
  elapsedSeconds: 2514,
  outputFile: 'live_domains.txt',
  extractWorkers: 200,
  testWorkers: 100,
}
