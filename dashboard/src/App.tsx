import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import './App.css'
import { AppSidebar, type DashboardTab } from './components/AppSidebar'
import { ActivityFeed } from './components/ActivityFeed'
import { DataImport } from './components/DataImport'
import { FindingsBoard } from './components/FindingsBoard'
import { CrackerWorkspace } from './components/CrackerWorkspace'
import { FleetControlSettings } from './components/FleetControlSettings'
import { FleetPanel } from './components/FleetPanel'
import { HeroMetricTiles } from './components/HeroMetricTiles'
import { LiveSourceSettings } from './components/LiveSourceSettings'
import { ProgressRow } from './components/ProgressRow'
import { ProviderHeatstrip } from './components/ProviderHeatstrip'
import { ListsPanel } from './components/ListsPanel'
import { StatCard } from './components/StatCard'
import { TargetListUpload } from './components/TargetListUpload'
import { TelegramSettings } from './components/TelegramSettings'
import { FleetBootstrap } from './components/FleetBootstrap'
import { UpdateBanner } from './components/UpdateBanner'
import { UpdateSettings } from './components/UpdateSettings'
import { R2Settings } from './components/R2Settings'
import { StartupCheck, shouldSkipStartupCheck } from './components/StartupCheck'
import { ScannerConfigPanel } from './components/ScannerConfigPanel'
import { ScannerLimitsSettings } from './components/ScannerLimitsSettings'
import { NotificationsSettings } from './components/NotificationsSettings'
import { ScheduleSettings } from './components/ScheduleSettings'
import { ToastStack, type ToastItem } from './components/ToastStack'
import { categoryForFinding } from './lib/toastCategory'
import { WarcPanel } from './components/WarcPanel'
import { readTargetTxtFile } from './lib/targetList'
import { VulnerabilityPicker } from './components/VulnerabilityPicker'
import { demoFindingsSeed } from './data/demoFindings'
import { VULN_CATALOG, defaultVulnSelection, type VulnSelection } from './data/vulnCatalog'
import { useFleetEnrollment } from './hooks/useFleetEnrollment'
import { useLiveScan, type LiveTotals } from './hooks/useLiveScan'
import { useReconStats } from './hooks/useReconStats'
import { useReconFleet } from './hooks/useReconFleet'
import { useScanSimulation } from './hooks/useScanSimulation'
import { useScanTick } from './hooks/useScanTick'
import { loadLists, saveLists, hashContent, makeListId } from './lib/listsStorage'
import { loadFleetControl, deployListViaApi, type FleetControlConfig } from './lib/fleetControl'
import { clearFleetCredentials, getFleetCredential } from './lib/fleetCredStore'
import { deleteListBody, getListBody, clearListBodies, setListBody } from './lib/listBodyCache'
import { loadLiveSource, type LiveSourceConfig } from './lib/liveSource'
import { stats as reconStatsApi, vps as reconVps } from './lib/reconApi'
import { pushCpuSample } from './lib/vpsHistory'
import { allocateChunks } from './lib/splitWorkload'
import { fmtDuration, fmtInt, fmtPercent } from './lib/format'
import type { Finding, RunSnapshot, Scan, ScanShard, TargetList, VpsNode } from './types'

const TOAST_QUEUE_CAP = 48

function enqueueToast(prev: ToastItem[], next: ToastItem): ToastItem[] {
  return [...prev, next].slice(-TOAST_QUEUE_CAP)
}

function emptySnapshot(): RunSnapshot {
  return {
    id: 'session',
    label: 'New session',
    startedAt: new Date().toISOString(),
    snapshots: [],
    targetLiveDomains: 0,
    liveDomains: 0,
    totalExtracted: 0,
    totalTested: 0,
    filesProcessed: 0,
    filesTotal: 0,
    elapsedSeconds: 0,
  }
}

export default function App() {
  const [tab, setTab] = useState<DashboardTab>('ravenx')
  const [activeScanId, setActiveScanId] = useState<string | null>(null)
  const [startupDone, setStartupDone] = useState<boolean>(() => shouldSkipStartupCheck())

  const [run, setRun] = useState<RunSnapshot>(emptySnapshot)
  const [fleet, setFleet] = useState<VpsNode[]>([])
  const [scans, setScans] = useState<Scan[]>([])
  const [shards, setShards] = useState<ScanShard[]>([])
  const [lists, setLists] = useState<TargetList[]>(() => loadLists())
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const lastToastIdRef = useRef<string | null>(null)
  const [liveCfg, setLiveCfg] = useState<LiveSourceConfig>(() => loadLiveSource())
  const [fleetCfg, setFleetCfg] = useState<FleetControlConfig>(() => loadFleetControl())
  const [liveTotals, setLiveTotals] = useState<LiveTotals>({
    liveDomains: 0,
    filesProcessed: 0,
    totalFindings: 0,
  })
  const [findings, setFindings] = useState<Finding[]>([])
  const [vulnSel, setVulnSel] = useState<VulnSelection>(() => defaultVulnSelection(true))

  const [targets, setTargets] = useState<{ count: number; name: string | null }>({
    count: 0,
    name: null,
  })

  const [scanning, setScanning] = useState(false)
  const [warcScanning, setWarcScanning] = useState(false)

  const vulnProviders = useMemo(() => {
    const s = new Set<string>()
    for (const r of VULN_CATALOG)
      if (vulnSel[r.id]) s.add(r.provider)
    return [...s].sort((a, b) => a.localeCompare(b))
  }, [vulnSel])

  const pushFinding = useCallback((draft: Omit<Finding, 'id'>) => {
    setFindings((prev) => {
      const next: Finding = {
        ...draft,
        id: `fd-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      }
      return [next, ...prev].slice(0, 260)
    })
  }, [])

  // ─── Real backend (Raven Flask) — overrides mocks when reachable ───
  const reconStats = useReconStats()
  const reconFleet = useReconFleet()
  const backendLive = reconStats.state === 'connected' || reconFleet.isLive

  // When real backend findings arrive, replace the mock findings entirely.
  useEffect(() => {
    if (reconStats.state !== 'connected') return
    setFindings(reconStats.findings)
  }, [reconStats.state, reconStats.findings])

  // When real backend fleet status arrives, replace the mock fleet (display-only — control
  // buttons hit the real backend; auto-enrollment of discovered nodes is intentionally NOT pushed).
  useEffect(() => {
    if (!reconFleet.isLive) return
    setFleet(reconFleet.fleet)
  }, [reconFleet.isLive, reconFleet.fleet])

  // When backend is live, hydrate the run snapshot from /api/stats. Otherwise keep demo behavior.
  useEffect(() => {
    if (reconStats.state !== 'connected' || !reconStats.run) return
    setRun(reconStats.run)
  }, [reconStats.state, reconStats.run])

  useScanSimulation({
    scanning: scanning && !liveCfg.enabled && !backendLive,
    setFleetActive: setFleet,
    vulnProviders,
    pushFinding,
  })

  useScanTick({
    scanning: scanning && !liveCfg.enabled && !backendLive,
    setScans,
    setShards,
  })

  // WARC harvest simulation — advances run snapshot while harvesting.
  useEffect(() => {
    if (!warcScanning || liveCfg.enabled || backendLive) return
    const id = window.setInterval(() => {
      setRun((prev) => {
        const liveBump = Math.floor(40 + Math.random() * 180)
        const fileBump = Math.random() > 0.35 ? 1 : 0
        const extractBump = Math.floor(800 + Math.random() * 4200)
        const testBump = Math.floor(600 + Math.random() * 3200)
        return {
          ...prev,
          liveDomains: Math.min(prev.targetLiveDomains, prev.liveDomains + liveBump),
          filesProcessed: Math.min(prev.filesTotal, prev.filesProcessed + fileBump),
          totalExtracted: prev.totalExtracted + extractBump,
          totalTested: prev.totalTested + testBump,
          elapsedSeconds: prev.elapsedSeconds + 2,
        }
      })
    }, 2000)
    return () => window.clearInterval(id)
  }, [warcScanning, liveCfg.enabled, backendLive])

  // Surface a toast only for the 4 popup categories: ssh-vps, smtp, ai, brevo.
  // Everything else stays in the Hits table without flashing the corner.
  // ToastStack handles the 3-visible/3s waterfall display logic — App just enqueues.
  useEffect(() => {
    const newest = findings[0]
    if (!newest) return
    if (lastToastIdRef.current === newest.id) return
    const category = categoryForFinding(newest)
    if (!category) return
    lastToastIdRef.current = newest.id
    setToasts((prev) => enqueueToast(prev, { id: newest.id, kind: 'hit' as const, finding: newest }))
  }, [findings])

  const pushAlertToast = useCallback((title: string, message?: string, kind: 'error' | 'info' = 'info') => {
    setToasts((prev) =>
      enqueueToast(prev, { id: `alert-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`, kind, title, message }),
    )
  }, [])

  const toggleScanPause = useCallback((scanId: string) => {
    setScans((prev) => {
      const target = prev.find((s) => s.id === scanId)
      // When backend is live and we're transitioning OUT of running, also stop the
      // fleet so SSH-launched workers actually halt — not just the local UI state.
      if (target && target.status === 'running' && backendLive) {
        void reconVps.stopAll().catch((e) => {
          pushAlertToast('Fleet stop failed', (e as Error).message, 'error')
        })
      } else if (target && target.status === 'paused' && backendLive) {
        void reconVps.startAll().catch((e) => {
          pushAlertToast('Fleet resume failed', (e as Error).message, 'error')
        })
      }
      return prev.map((s) =>
        s.id !== scanId
          ? s
          : s.status === 'running'
            ? { ...s, status: 'paused', endedAt: new Date().toISOString(), requestsPerSec: 0, parsingPerSec: 0 }
            : s.status === 'paused'
              ? { ...s, status: 'running', endedAt: undefined }
              : s,
      )
    })
  }, [backendLive, pushAlertToast])

  const liveStatus = useLiveScan({
    config: liveCfg,
    pushFinding,
    setLiveTotals,
  })

  useFleetEnrollment({
    findings,
    fleetControl: fleetCfg,
    setFleet,
    onEnrolled: (node) => {
      if (node.source !== 'discovered' || node.status !== 'healthy') return
      setToasts((prev) =>
        enqueueToast(prev, {
          id: `fleet-${node.id}`,
          kind: 'hit' as const,
          finding: {
            id: `fleet-${node.id}`,
            at: new Date().toISOString(),
            provider: 'Fleet',
            ruleLabel: 'SSH node enrolled',
            hostname: node.host,
            detail: `${node.label} added to fleet — assign lists on Targets tab`,
            severity: 'high' as const,
            reportedByHost: node.id,
          },
        }),
      )
    },
  })

  const effectiveRun = useMemo<RunSnapshot>(() => {
    if (!liveCfg.enabled) return run
    return {
      ...run,
      id: 'live',
      label: liveCfg.baseUrl || 'Live source',
      liveDomains: liveTotals.liveDomains,
      totalExtracted: liveTotals.totalFindings,
      totalTested: Math.max(run.totalTested, liveTotals.liveDomains),
    }
  }, [liveCfg.enabled, liveCfg.baseUrl, run, liveTotals])

  const liveRate = useMemo(() => {
    if (effectiveRun.totalTested <= 0) return 0
    return (effectiveRun.liveDomains / effectiveRun.totalTested) * 100
  }, [effectiveRun.liveDomains, effectiveRun.totalTested])

  const extractionRate = useMemo(() => {
    if (effectiveRun.elapsedSeconds <= 0) return 0
    return effectiveRun.totalExtracted / effectiveRun.elapsedSeconds
  }, [effectiveRun.elapsedSeconds, effectiveRun.totalExtracted])

  const testRate = useMemo(() => {
    if (effectiveRun.elapsedSeconds <= 0) return 0
    return effectiveRun.totalTested / effectiveRun.elapsedSeconds
  }, [effectiveRun.elapsedSeconds, effectiveRun.totalTested])

  const exportSnapshot = useCallback(() => {
    const blob = new Blob([JSON.stringify(effectiveRun, null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${effectiveRun.id}-run-metrics.json`
    a.click()
    URL.revokeObjectURL(url)
  }, [effectiveRun])

  const assignShards = useCallback(() => {
    setFleet((prev) => {
      const activeOrdered = prev.filter(
        (n) =>
          n.status !== 'removed' &&
          (n.status === 'healthy' || n.status === 'degraded'),
      )
      if (!targets.count || activeOrdered.length === 0) return prev
      const slices = allocateChunks(targets.count, activeOrdered.length)
      let ai = 0
      return prev.map((node) => {
        if (node.status === 'removed') return node
        const slice = slices[ai] ?? 0
        ai++
        return {
          ...node,
          targetsAssigned: slice,
          targetsDone: 0,
          scansPerSecond: 0,
          activeListId: undefined,
          activeListName: targets.name ?? undefined,
          cpuHistory: pushCpuSample(undefined, node.cpuPercent),
        }
      })
    })
  }, [targets.count])

  const handleFiles = async (file: File | null) => {
    if (!file) {
      setTargets({ count: 0, name: null })
      return
    }
    try {
      const count = await readTargetTxtFile(file)
      setTargets({ count, name: file.name })
    } catch {
      setTargets({ count: 0, name: null })
    }
  }

  const vulnPreset = (preset: 'all' | 'none' | 'email' | 'wired') => {
    if (preset === 'all') {
      setVulnSel(Object.fromEntries(VULN_CATALOG.map((r) => [r.id, true])))
    } else if (preset === 'none') {
      setVulnSel(defaultVulnSelection(false))
    } else if (preset === 'wired') {
      setVulnSel(defaultVulnSelection(true))
    } else {
      // 'email' preset: email + payments categories (only wired so toggles map to real data)
      setVulnSel(
        Object.fromEntries(
          VULN_CATALOG.map((r) => [
            r.id,
            r.wired && (r.category === 'email' || r.category === 'payments'),
          ]),
        ),
      )
    }
  }

  const forceOutage = (id: string, mode: 'offline' | 'reconnect') => {
    setFleet((prev) =>
      prev.map((n) =>
        n.id !== id || n.status === 'removed'
          ? n
          : {
              ...n,
              status: mode === 'offline' ? 'offline' : 'reconnecting',
              reconnectFailCount: 0,
              lastEvent:
                mode === 'offline'
                  ? 'Operator simulated network black-hole'
                  : 'Operator stalled SSH/control channel',
              scansPerSecond: 0,
            },
      ),
    )
  }

  const resetEverything = () => {
    setRun(emptySnapshot())
    setFleet([])
    clearFleetCredentials()
    clearListBodies()
    setLists([])
    saveLists([])
    setTargets({ count: 0, name: null })
    setVulnSel(defaultVulnSelection(true))
    setFindings([])
    setScans([])
    setShards([])
    setActiveScanId(null)
    setScanning(false)
    setWarcScanning(false)
  }

  const aggregates = useMemo(() => {
    const liveHosts = fleet.filter((n) => n.status !== 'removed').length
    const done = fleet.reduce((s, n) => s + n.targetsDone, 0)
    const assigned = fleet.reduce((s, n) => s + n.targetsAssigned, 0)
    return { liveHosts, done, assigned }
  }, [fleet])

  const hitInsights = useMemo(() => {
    const cracks = findings.filter((f) => f.severity === 'critical' || f.severity === 'high').length
    return { total: findings.length, cracks }
  }, [findings])

  const warcExportHosts = useMemo(() => {
    const hosts = new Set<string>()
    for (const f of findings) {
      const h = f.hostname?.trim()
      if (h) hosts.add(h)
    }
    return [...hosts]
  }, [findings])

  const [clock, setClock] = useState(() => new Date())
  useEffect(() => {
    const id = window.setInterval(() => setClock(new Date()), 1000)
    return () => window.clearInterval(id)
  }, [])

  const overviewHidden = tab !== 'overview'
  const ravenxHidden = tab !== 'ravenx'
  const warcHidden = tab !== 'warc'
  const listsHidden = tab !== 'lists'
  const fleetHidden = tab !== 'fleet'
  const findingsHidden = tab !== 'findings'
  const settingsHidden = tab !== 'settings'

  const upsertList = useCallback(
    (next: TargetList) => {
      setLists((prev) => {
        const i = prev.findIndex((l) => l.id === next.id)
        const out = i === -1 ? [next, ...prev] : prev.map((l) => (l.id === next.id ? next : l))
        saveLists(out)
        return out
      })
    },
    [],
  )

  const deleteList = useCallback((listId: string) => {
    deleteListBody(listId)
    setLists((prev) => {
      const out = prev.filter((l) => l.id !== listId)
      saveLists(out)
      return out
    })
  }, [])

  const exportWarcFindingsToList = useCallback(() => {
    if (warcExportHosts.length === 0) {
      pushAlertToast('Nothing to export', 'Harvest findings first — no hostnames in the inbox.', 'info')
      return
    }
    const body = warcExportHosts.join('\n')
    const hash = hashContent(body)
    const dup = lists.find((l) => l.contentHash === hash)
    if (dup) {
      pushAlertToast('Already exported', `Identical list exists as "${dup.name}".`, 'info')
      setTab('lists')
      return
    }
    const stamp = new Date().toISOString().slice(0, 10)
    const next: TargetList = {
      id: makeListId(),
      name: `warc-hits-${stamp}.txt`,
      uploadedAt: new Date().toISOString(),
      lineCount: warcExportHosts.length,
      contentHash: hash,
      preview: warcExportHosts.slice(0, 6),
      assignedVpsIds: [],
      status: 'idle',
      note: 'Exported from WARC harvest findings',
    }
    setListBody(next.id, body)
    upsertList(next)
    pushAlertToast(`Exported ${warcExportHosts.length} hosts`, `Created "${next.name}" on Lists tab.`, 'info')
    setTab('lists')
  }, [warcExportHosts, lists, upsertList, pushAlertToast])

  /** When the real Raven backend is up, deploy uses /api/vps/upload-targets + /api/vps/deploy
   *  against the operator's rostered VPSes (`server_ips.txt`). The chip selection in ListsPanel
   *  is treated as UI intent — backend deploys to its full roster as that's the only mode app.py supports. */
  const deployListViaRecon = useCallback(
    async (listId: string): Promise<boolean> => {
      const list = lists.find((l) => l.id === listId)
      if (!list) return false
      const body = getListBody(listId)
      if (!body) {
        pushAlertToast('Deploy needs the list body', 'Re-upload the list — content was not retained from a previous session.', 'error')
        return false
      }
      upsertList({ ...list, status: 'queued' })
      try {
        const file = new File([body], list.name, { type: 'text/plain' })
        const upRes = await reconVps.uploadTargets(file)
        if (!upRes.success) throw new Error(upRes.error ?? 'upload-targets returned non-success')
        const depRes = await reconVps.deploy({ targetFile: upRes.filename, autoStart: true })
        upsertList({ ...list, status: 'deployed' })
        const msg = typeof depRes.message === 'string' ? depRes.message : `${upRes.targets.toLocaleString()} targets across rostered fleet`
        pushAlertToast(`Deployed ${list.name}`, msg, 'info')
        return true
      } catch (e) {
        upsertList({ ...list, status: 'failed' })
        pushAlertToast(`Deploy failed: ${list.name}`, (e as Error).message ?? 'unknown error', 'error')
        return false
      }
    },
    [lists, upsertList, pushAlertToast],
  )

  /** Per-node action handler used by VpsCard when backend is live. */
  const onVpsAction = useCallback(
    async (ip: string, action: 'start' | 'stop' | 'restart'): Promise<{ ok: boolean; message?: string }> => {
      try {
        const result =
          action === 'start' ? await reconVps.start(ip)
          : action === 'stop' ? await reconVps.stop(ip)
          : await reconVps.restart(ip)
        const ok = result.success !== false
        return { ok, message: typeof result.message === 'string' ? result.message : action }
      } catch (e) {
        return { ok: false, message: (e as Error).message ?? 'request failed' }
      }
    },
    [],
  )

  /** Bulk fleet-action handler — used by FleetPanel header when backend is live. */
  const onFleetBulkAction = useCallback(
    async (action: 'start-all' | 'stop-all' | 'restart-all' | 'test-connections'): Promise<{ ok: boolean; message?: string }> => {
      try {
        const result =
          action === 'start-all' ? await reconVps.startAll()
          : action === 'stop-all' ? await reconVps.stopAll()
          : action === 'restart-all' ? await reconVps.restartAll()
          : await reconVps.testConnections()
        const ok = result.success !== false
        const message = typeof result.message === 'string' ? result.message : `${action} ok`
        pushAlertToast(ok ? `Bulk ${action} dispatched` : `Bulk ${action} failed`, message, ok ? 'info' : 'error')
        return { ok, message }
      } catch (e) {
        const message = (e as Error).message ?? 'request failed'
        pushAlertToast(`Bulk ${action} failed`, message, 'error')
        return { ok: false, message }
      }
    },
    [pushAlertToast],
  )

  const deployList = useCallback(
    async (listId: string) => {
      if (backendLive) {
        await deployListViaRecon(listId)
        return
      }
      const list = lists.find((l) => l.id === listId)
      if (!list || list.assignedVpsIds.length === 0) return

      upsertList({ ...list, status: 'deployed' })

      const body = getListBody(listId)
      const lines = body ? body.split('\n').filter(Boolean) : []
      const sliceSize = lines.length > 0 ? Math.ceil(lines.length / list.assignedVpsIds.length) : 0

      for (const vpsId of list.assignedVpsIds) {
        const node = fleet.find((n) => n.id === vpsId)
        const cred = getFleetCredential(vpsId)
        if (!node || node.status === 'removed') continue

        const idx = list.assignedVpsIds.indexOf(vpsId)
        const chunk = sliceSize > 0 ? lines.slice(idx * sliceSize, (idx + 1) * sliceSize).join('\n') : ''
        const simLines = sliceSize > 0 ? chunk.split('\n').filter(Boolean).length : Math.floor(list.lineCount / list.assignedVpsIds.length)

        if (!cred || !chunk) {
          setFleet((prev) =>
            prev.map((n) =>
              n.id === vpsId
                ? {
                    ...n,
                    targetsAssigned: simLines,
                    targetsDone: 0,
                    activeListId: list.id,
                    activeListName: list.name,
                    lastEvent: `Scanning ${list.name}`,
                  }
                : n,
            ),
          )
          continue
        }

        const [hostOnly, portStr] = node.host.includes(':')
          ? node.host.split(':')
          : [node.host, '22']
        const result = await deployListViaApi(fleetCfg, {
          vpsId,
          host: hostOnly!,
          port: Number(portStr) || 22,
          user: cred.user,
          secret: cred.secret,
          authType: cred.authType,
          listName: list.name,
          targets: chunk,
        })
        setFleet((prev) =>
          prev.map((n) =>
            n.id === vpsId
              ? {
                  ...n,
                  targetsAssigned: chunk.split('\n').filter(Boolean).length,
                  targetsDone: 0,
                  activeListId: list.id,
                  activeListName: list.name,
                  lastEvent: result.ok ? `Scanning ${list.name}` : `Deploy failed: ${result.message}`,
                  status: result.ok ? n.status : 'degraded',
                }
              : n,
          ),
        )
      }
    },
    [lists, fleet, fleetCfg, upsertList, backendLive, deployListViaRecon],
  )

  if (!startupDone) {
    return <StartupCheck onDone={() => setStartupDone(true)} />
  }

  return (
    <div className="app-layout">
      <AppSidebar active={tab} onChange={setTab} />

      <div className="app-main">
        <UpdateBanner />
        <div className="shell shell--main">
          <header className="header header--lux">
            <div className="header__lead">
              <h1>Scan cockpit</h1>
              <p className="header__subtitle">
                Unified surface for crawl metrics, distributed workers, and credential hits — tuned for long sessions.
              </p>
            </div>
            <div className="header__ribbon" aria-live="polite">
              <span className="ribbon-clock">{clock.toLocaleTimeString()}</span>
              <span className={`ribbon-env${liveCfg.enabled ? ' ribbon-env--live' : ''}`}>
                {liveCfg.enabled ? 'LIVE' : 'SANDBOX'}
              </span>
              <span className="ribbon-stat">{hitInsights.total} hits</span>
              <span className="ribbon-stat ribbon-stat--gold">{hitInsights.cracks} cracks</span>
            </div>
          </header>

          <div className="tab-panels">
            <section
              id="panel-overview"
              role="tabpanel"
              aria-labelledby="tab-overview"
              hidden={overviewHidden}
              className="tab-panel"
            >
              <HeroMetricTiles
                totalHits={hitInsights.total}
                cracks={hitInsights.cracks}
                liveDomains={effectiveRun.liveDomains}
              />

              <section className="meta-bar meta-bar--stack meta-bar--tight meta-bar--glass">
                <div className="meta-bar__top">
                  <div>
                    <span className="meta-bar__muted">Label</span>
                    <strong>{effectiveRun.label}</strong>
                    <span className="meta-bar__id">{effectiveRun.id}</span>
                  </div>
                  <div className="meta-metrics">
                    {liveCfg.enabled ? (
                      <span className={`live-pill live-pill--${liveStatus.state}`}>
                        {liveStatus.state === 'ok'
                          ? `Live · ${liveStatus.filesSeen} file(s)`
                          : liveStatus.state === 'error'
                            ? 'Live · error'
                            : liveStatus.state === 'connecting'
                              ? 'Live · connecting'
                              : 'Live · idle'}
                      </span>
                    ) : (
                      <span className="pill pill--muted">Demo mode</span>
                    )}
                    <span className="pill pill--ok">{aggregates.liveHosts} VPS</span>
                    <span className="pill pill--muted">
                      {targets.count ? `${targets.count.toLocaleString()} queued` : 'No upload'}
                    </span>
                    <span className="pill pill--muted">{findings.length} hits</span>
                  </div>
                </div>
                <div className="meta-bar__chips" aria-label="WARC/CC snapshots baked into metrics">
                  {effectiveRun.snapshots.map((s) => (
                    <span key={s} className="chip">
                      {s}
                    </span>
                  ))}
                </div>
              </section>

              <div className="overview-enrich">
                <div className="overview-enrich__main">
              <main className="overview-grid">
                <div className="stats stats--lux">
                  <StatCard
                    title="Live domains"
                    value={fmtInt(effectiveRun.liveDomains)}
                    hint={`${fmtPercent(effectiveRun.liveDomains, effectiveRun.targetLiveDomains)} of ${fmtInt(effectiveRun.targetLiveDomains)}`}
                    accent="green"
                  />
                  <StatCard title="Tested" value={fmtInt(effectiveRun.totalTested)} hint={liveCfg.enabled ? 'from live source' : 'from metrics blob'} />
                  <StatCard title="Extracted" value={fmtInt(effectiveRun.totalExtracted)} hint={liveCfg.enabled ? 'live findings tally' : 'grabber tally'} />
                  <StatCard
                    title="Elapsed"
                    value={fmtDuration(effectiveRun.elapsedSeconds)}
                    hint={`${fmtInt(extractionRate)} · ${fmtInt(testRate)} /s`}
                    accent="amber"
                  />
                </div>

                <aside className="panel panel--glass">
                  <h2 className="panel__title">Run metrics</h2>
                  <p className="panel-explainer">
                    Numbers on this strip come from the in-memory{' '}
                    <code className="inline-code">RunSnapshot</code> object — same payload you{' '}
                    <strong>import below</strong> or <strong>export</strong> from Settings after a crawl. It mirrors
                    WARC/live-check totals, worker counts, and CC snapshot IDs, not your hits inbox.
                  </p>
                  <ProgressRow label="Live vs target" current={effectiveRun.liveDomains} total={effectiveRun.targetLiveDomains} />
                  <ProgressRow label="WARC files" current={effectiveRun.filesProcessed} total={effectiveRun.filesTotal} />
                  <dl className="kv kv--compact">
                    <div>
                      <dt>Live rate</dt>
                      <dd>{liveRate.toFixed(2)}%</dd>
                    </div>
                    <div>
                      <dt>Workers</dt>
                      <dd>
                        {effectiveRun.extractWorkers ?? '—'} / {effectiveRun.testWorkers ?? '—'}
                      </dd>
                    </div>
                    <div>
                      <dt>Output</dt>
                      <dd className="kv__mono">{effectiveRun.outputFile ?? '—'}</dd>
                    </div>
                    <div>
                      <dt>Started</dt>
                      <dd className="kv__mono">{new Date(effectiveRun.startedAt).toLocaleString()}</dd>
                    </div>
                  </dl>
                  <p className="panel-note">
                    Import / export JSON in{' '}
                    <button type="button" className="link-ish" onClick={() => setTab('settings')}>
                      Settings
                    </button>
                    .
                  </p>
                </aside>
              </main>
                </div>
                <div className="overview-enrich__rail">
                  <ActivityFeed
                    findings={findings}
                    liveLabel={liveCfg.enabled ? (liveCfg.baseUrl || 'Live HTTP') : 'Demo feed'}
                  />
                  <ProviderHeatstrip findings={findings} />
                </div>
              </div>

            </section>

            <section
              id="panel-ravenx"
              role="tabpanel"
              aria-labelledby="tab-ravenx"
              hidden={ravenxHidden}
              className="tab-panel"
            >
              <CrackerWorkspace
                scans={scans}
                shards={shards}
                lists={lists}
                findings={findings}
                fleet={fleet}
                activeScanId={activeScanId}
                onSelectScan={setActiveScanId}
                onTogglePause={toggleScanPause}
                onReplayDemo={() => setFindings(demoFindingsSeed.map((f) => ({ ...f })))}
                onDeleteList={deleteList}
                onToast={(t) => pushAlertToast(t.title, t.message, t.kind === 'error' ? 'error' : 'info')}
              />
            </section>

            <section
              id="panel-warc"
              role="tabpanel"
              aria-labelledby="tab-warc"
              hidden={warcHidden}
              className="tab-panel"
            >
              <WarcPanel
                run={effectiveRun}
                liveEnabled={liveCfg.enabled}
                scanning={warcScanning}
                onToggleScan={() => setWarcScanning((s) => !s)}
                onExportToList={exportWarcFindingsToList}
                exportCount={warcExportHosts.length}
              />
            </section>

            <section
              id="panel-lists"
              role="tabpanel"
              aria-labelledby="tab-lists"
              hidden={listsHidden}
              className="tab-panel"
            >
              <ListsPanel
                lists={lists}
                fleet={fleet}
                onUpload={upsertList}
                onUpdate={upsertList}
                onDelete={deleteList}
                onDeploy={(id) => void deployList(id)}
              />
            </section>

            <section
              id="panel-fleet"
              role="tabpanel"
              aria-labelledby="tab-fleet"
              hidden={fleetHidden}
              className="tab-panel"
            >
              <FleetPanel
                fleet={fleet}
                lists={lists}
                totalTargets={targets.count}
                scanning={scanning}
                onRedeploySplit={assignShards}
                onForceOutage={forceOutage}
                onAction={backendLive ? onVpsAction : undefined}
                onBulkAction={backendLive ? onFleetBulkAction : undefined}
              />
            </section>

            <section
              id="panel-findings"
              role="tabpanel"
              aria-labelledby="tab-findings"
              hidden={findingsHidden}
              className="tab-panel"
            >
              <FindingsBoard
                findings={findings}
                onReplayDemo={() => setFindings(demoFindingsSeed.map((f) => ({ ...f })))}
                onClearAll={
                  backendLive
                    ? async () => {
                        try {
                          await reconStatsApi.clear()
                          setFindings([])
                          pushAlertToast('Cleared findings', 'Backend cleared credentials + result files.', 'info')
                        } catch (e) {
                          pushAlertToast('Clear failed', (e as Error).message, 'error')
                        }
                      }
                    : () => {
                        setFindings([])
                        pushAlertToast('Cleared findings (local)', 'Live backend not reachable — local ledger reset.', 'info')
                      }
                }
              />
            </section>

            <section
              id="panel-settings"
              role="tabpanel"
              aria-labelledby="tab-settings"
              hidden={settingsHidden}
              className="tab-panel"
            >
              <div className="settings-stack">
                <details className="settings-acc" open>
                  <summary>Ingest &amp; live HTTP</summary>
                  <div className="settings-acc__body">
                    <LiveSourceSettings
                      config={liveCfg}
                      onChange={(c) => {
                        setLiveCfg(c)
                        if (c.enabled) {
                          setFindings((prev) => prev.filter((f) => !f.id.startsWith('fd-')))
                        }
                      }}
                      status={liveStatus}
                    />

                    <TargetListUpload
                      lines={targets.count}
                      fileLabel={targets.name}
                      onFile={(f) => void handleFiles(f)}
                    />
                  </div>
                </details>

                <details className="settings-acc" open>
                  <summary>Playback &amp; run metrics</summary>
                  <div className="settings-acc__body">
                    <section className="card-block card-block--tight">
                      <div className="card-block__head card-block__head--row">
                        <div>
                          <h2>Playback</h2>
                          <p className="card-block__lede card-block__lede--short">
                            Pause UI ticks, ship run metrics JSON, reset fabric.
                          </p>
                        </div>
                      </div>
                      <div className="settings-btn-row">
                        {scanning ? (
                          <button
                            type="button"
                            className="btn-secondary"
                            onClick={() => setScanning(false)}
                          >
                            Pause simulation
                          </button>
                        ) : (
                          <button
                            type="button"
                            className="btn-primary"
                            onClick={() => setScanning(true)}
                          >
                            Resume simulation
                          </button>
                        )}
                        <button type="button" className="btn-secondary" onClick={exportSnapshot}>
                          Export run metrics JSON
                        </button>
                        <button type="button" className="btn-secondary" onClick={resetEverything}>
                          Reset
                        </button>
                      </div>
                      <p className="settings-hint">
                        Routed {aggregates.done.toLocaleString()}
                        {aggregates.assigned
                          ? ` / ${aggregates.assigned.toLocaleString()} partitioned`
                          : ''}{' '}
                        · Shard on Cracker after uploading targets here.
                      </p>
                    </section>
                  </div>
                </details>

                <details className="settings-acc" open>
                  <summary>Storage · Cloudflare R2</summary>
                  <div className="settings-acc__body">
                    <R2Settings />
                  </div>
                </details>

                <details className="settings-acc" open>
                  <summary>Updates (GitHub)</summary>
                  <div className="settings-acc__body">
                    <UpdateSettings />
                  </div>
                </details>

                <details className="settings-acc" open>
                  <summary>Fleet bootstrap (paste / upload SSH creds)</summary>
                  <div className="settings-acc__body">
                    <FleetBootstrap />
                  </div>
                </details>

                <details className="settings-acc">
                  <summary>Fleet SSH control plane (advanced)</summary>
                  <div className="settings-acc__body">
                    <FleetControlSettings config={fleetCfg} onChange={setFleetCfg} />
                  </div>
                </details>

                <details className="settings-acc" open>
                  <summary>Scanner configuration (live flags)</summary>
                  <div className="settings-acc__body">
                    <ScannerConfigPanel
                      onToast={(t) => pushAlertToast(t.title, t.message, t.kind)}
                    />
                  </div>
                </details>

                <details className="settings-acc" open>
                  <summary>Scanner limits &amp; behaviour</summary>
                  <div className="settings-acc__body">
                    <ScannerLimitsSettings />
                  </div>
                </details>

                <details className="settings-acc">
                  <summary>Notifications · Telegram</summary>
                  <div className="settings-acc__body">
                    <TelegramSettings />
                  </div>
                </details>

                <details className="settings-acc">
                  <summary>Notifications · webhooks &amp; Slack</summary>
                  <div className="settings-acc__body">
                    <NotificationsSettings />
                  </div>
                </details>

                <details className="settings-acc">
                  <summary>Scheduled re-scans</summary>
                  <div className="settings-acc__body">
                    <ScheduleSettings />
                  </div>
                </details>

                <details className="settings-acc">
                  <summary>Detector modules</summary>
                  <div className="settings-acc__body">
                    <div className="vuln-scroll-wrap">
                      <VulnerabilityPicker
                        selection={vulnSel}
                        onToggle={(id, on) => setVulnSel((s) => ({ ...s, [id]: on }))}
                        onPreset={(p) => vulnPreset(p)}
                      />
                    </div>
                  </div>
                </details>

                <details className="settings-acc">
                  <summary>Run metrics JSON import</summary>
                  <div className="settings-acc__body">
                    <section className="card-block panel">
                      <h2 className="panel__title">Hydrate run metrics (JSON)</h2>
                      <p className="panel-explainer">
                        Drop the file your orchestrator emits after a crawl (same shape as{' '}
                        <strong>Export run metrics JSON</strong>). Only the KPI / progress widgets update — VPS +
                        hits tabs keep their local state unless you reload.
                      </p>
                      <DataImport onImport={(r) => setRun(r)} />
                    </section>
                  </div>
                </details>
              </div>
            </section>
          </div>

          <footer className="footer footer--minimal">
            Run metrics = crawl summary blob · Hits inbox = separate ledger · Telegram prefs stay local until you add a relay.
          </footer>
        </div>
      </div>

      <ToastStack
        toasts={toasts}
        onDismiss={(id) => setToasts((prev) => prev.filter((t) => t.id !== id))}
      />
    </div>
  )
}
