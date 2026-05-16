import { useEffect, useMemo, useRef, useState } from 'react'
import type { Finding, Scan, ScanShard, TargetList, VpsNode } from '../types'
import {
  scannerConfig,
  type ReconScannerConfig,
  type ReconScannerConfigPatch,
} from '../lib/reconApi'
import { AddonsStrip } from './AddonsStrip'
import { CrackerListTile } from './CrackerListTile'
import { CrackerSessionPanel } from './CrackerSessionPanel'
import { CrackerSessionRail } from './CrackerSessionRail'
import { ScanDetail } from './ScanDetail'
import { DiscoveryHubs } from './DiscoveryHubs'

export type CrackerToast = {
  id: string
  kind: 'error' | 'hit' | 'info'
  title: string
  message?: string
}

type Props = {
  scans: readonly Scan[]
  shards: readonly ScanShard[]
  lists: readonly TargetList[]
  findings: readonly Finding[]
  fleet: readonly VpsNode[]
  activeScanId: string | null
  onSelectScan: (id: string | null) => void
  onTogglePause: (scanId: string) => void
  onDeleteList: (id: string) => void
  onToast: (t: Omit<CrackerToast, 'id'>) => void
}

export function CrackerWorkspace({
  scans,
  shards,
  lists,
  findings,
  fleet,
  activeScanId,
  onSelectScan,
  onTogglePause,
  onDeleteList,
  onToast,
}: Props) {
  const [viewStats, setViewStats] = useState(false)
  const [config, setConfig] = useState<ReconScannerConfig | null>(null)
  const saveSeq = useRef(0)

  useEffect(() => {
    let cancelled = false
    scannerConfig.get()
      .then((c) => { if (!cancelled) setConfig(c) })
      .catch(() => { /* silent — AddonsStrip will simply disable */ })
    return () => { cancelled = true }
  }, [])

  function patchConfig(patch: ReconScannerConfigPatch) {
    setConfig((prev) => {
      if (!prev) return prev
      const next: Record<string, Record<string, boolean>> = {
        scanning_features: { ...prev.scanning_features },
        aws_checks: { ...prev.aws_checks },
        api_validation: { ...prev.api_validation },
        features: { ...prev.features },
        exploit_methods: { ...prev.exploit_methods },
      }
      for (const section of Object.keys(patch)) {
        const sectionPatch = (patch as Record<string, Record<string, boolean> | undefined>)[section]
        if (!sectionPatch) continue
        next[section] = { ...next[section], ...sectionPatch }
      }
      return next as unknown as ReconScannerConfig
    })
    const seq = ++saveSeq.current
    scannerConfig.update(patch)
      .then((c) => { if (seq === saveSeq.current) setConfig(c) })
      .catch((e: Error) => {
        if (seq !== saveSeq.current) return
        onToast({ kind: 'error', title: 'Save failed', message: e.message })
      })
  }

  const activeScan = useMemo(() => {
    const pick = activeScanId ? scans.find((s) => s.id === activeScanId) : null
    if (pick) return pick
    return scans.find((s) => s.status === 'running') ?? scans[0] ?? null
  }, [scans, activeScanId])

  const activeShards = activeScan ? shards.filter((sh) => sh.scanId === activeScan.id) : []

  const activeCracks = scans.filter((s) => s.status === 'running' || s.status === 'paused').length
  const addonCount = useMemo(() => {
    if (!config) return 0
    return (
      Number(config.aws_checks.ses_quota_check) +
      Number(config.api_validation.sendgrid) +
      Number(config.api_validation.stripe) +
      Number(config.api_validation.twilio) +
      Number(config.scanning_features.aws_main_scan)
    )
  }, [config])

  function validateAndStart() {
    onToast({ kind: 'info', title: 'Crack queued', message: 'Session config saved.' })
  }

  if (viewStats && activeScan) {
    return (
      <ScanDetail
        scan={activeScan}
        shards={activeShards}
        fleet={fleet}
        findings={findings}
        onBack={() => setViewStats(false)}
        onTogglePause={onTogglePause}
      />
    )
  }

  return (
    <div className="cw">
      <header className="cw__head">
        <div>
          <h2 className="cw__title">Cracker Workspace</h2>
          <p className="cw__lede">
            Compose new cracking sessions, orchestrate scanners and pilot cloud/API addons from a single refined panel.
          </p>
        </div>
        <div className="cw__summary">
          <div className="cw__summary-card">
            <span className="cw__summary-k">Active Cracks</span>
            <strong>{activeCracks}</strong>
          </div>
          <div className="cw__summary-card">
            <span className="cw__summary-k">Available Lists</span>
            <strong>{lists.length}</strong>
          </div>
          <div className="cw__summary-card">
            <span className="cw__summary-k">Owned Addons</span>
            <strong>{addonCount}</strong>
          </div>
        </div>
      </header>

      <div className="cw__body">
        <CrackerSessionRail
          scans={scans}
          activeId={activeScan?.id ?? null}
          onSelect={(id) => onSelectScan(id)}
          onNew={validateAndStart}
          onTogglePause={onTogglePause}
          onStop={onTogglePause}
        />

        <div className="cw__main">
          {activeScan ? (
            <CrackerSessionPanel
              scan={activeScan}
              onStop={() => onTogglePause(activeScan.id)}
              onViewStats={() => setViewStats(true)}
            />
          ) : (
            <p className="muted-callout">No active crack — start a session from the rail or configure scanners in Settings.</p>
          )}
        </div>
      </div>

      <AddonsStrip config={config} onPatch={patchConfig} />

      <div className="cw__below">
        <DiscoveryHubs findings={findings} />
        {lists.length > 0 && (
          <section className="cw__lists">
            <header className="cw__lists-head">
              <h3 className="cw__lists-title">Uploaded lists</h3>
              <p className="muted cw__lists-hint">Quick view — manage deploy on the Lists tab</p>
            </header>
            <div className="cw__lists-grid">
              {lists.slice(0, 3).map((l) => (
                <CrackerListTile key={l.id} list={l} onDelete={onDeleteList} />
              ))}
            </div>
          </section>
        )}
      </div>
    </div>
  )
}
