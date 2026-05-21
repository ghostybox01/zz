// Created by https://t.me/boxxboyy
import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import type { Finding, Scan, ScanShard, TargetList, VpsNode } from '../types'
import {
  scannerConfig,
  crack,
  type CrackSession,
  type ReconScannerConfig,
  type ReconScannerConfigPatch,
} from '../lib/reconApi'
import {
  ADDON_CATALOG,
  getEnabledAddons,
  type CrackerAddonEnabledMap,
} from '../data/addonCatalog'
import { AddonsStrip, brandFor } from './AddonsStrip'
import { BrandLogo } from './BrandGlyph'
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

const CRACK_POLL_MS = 5000

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

  // Operator's catalog-level enabled map — read from /api/scanner-config
  // under the `cracker_addons` key. Missing → fall through to defaultOn
  // via `getEnabledAddons`.
  const [enabledMap, setEnabledMap] = useState<CrackerAddonEnabledMap | null>(null)

  // Crack session state — live list reconciled with backend on a 5s poll.
  const [sessions, setSessions] = useState<CrackSession[]>([])
  const [composerError, setComposerError] = useState<string | null>(null)

  // New-Crack composer state
  const [composerOpen, setComposerOpen] = useState(false)
  const [sessionName, setSessionName] = useState('')
  const [pickedListId, setPickedListId] = useState<string>('')
  const [pickedAddons, setPickedAddons] = useState<ReadonlySet<string>>(() => new Set())
  const [pickedVpsIds, setPickedVpsIds] = useState<ReadonlySet<string>>(() => new Set())
  const [submitting, setSubmitting] = useState(false)

  // ── Scanner config (existing live-flag patch path) ─────────────────
  useEffect(() => {
    let cancelled = false
    scannerConfig.get()
      .then((c) => {
        if (cancelled) return
        setConfig(c)
        const raw = (c as unknown as { cracker_addons?: CrackerAddonEnabledMap }).cracker_addons
        setEnabledMap(raw && typeof raw === 'object' ? raw : null)
      })
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

  // ── Crack session polling ──────────────────────────────────────────
  useEffect(() => {
    let cancelled = false
    const refresh = async () => {
      try {
        const r = await crack.list()
        if (!cancelled && Array.isArray(r?.sessions)) {
          setSessions(r.sessions)
        }
      } catch {
        // Backend endpoint may not be live yet — keep the optimistic-UI
        // entries we have and try again next tick.
      }
    }
    void refresh()
    const timer = window.setInterval(() => { void refresh() }, CRACK_POLL_MS)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [])

  const activeScan = useMemo(() => {
    const pick = activeScanId ? scans.find((s) => s.id === activeScanId) : null
    if (pick) return pick
    return scans.find((s) => s.status === 'running') ?? scans[0] ?? null
  }, [scans, activeScanId])

  const activeShards = activeScan ? shards.filter((sh) => sh.scanId === activeScan.id) : []

  // `queued` is in-flight too — the fire-and-forget dispatcher creates
  // a session in queued state and flips it to running once the SCP +
  // remote spawn complete. Excluding queued made the "Active cracks"
  // tile read 0 during the seconds-to-minutes window a session is
  // actually being dispatched.
  const activeCracks = scans.filter(
    (s) => s.status === 'running' || s.status === 'paused' || s.status === 'queued',
  ).length
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

  // ── Catalog-driven composer addon list ─────────────────────────────
  const composerAddons = useMemo(() => getEnabledAddons(enabledMap), [enabledMap])

  // ── Fleet filter for the worker-chip block ─────────────────────────
  const activeFleet = useMemo(() => fleet.filter((n) => n.status !== 'removed'), [fleet])
  const deployableFleet = useMemo(
    () => activeFleet.filter((n) => n.status === 'healthy' || n.status === 'degraded'),
    [activeFleet],
  )

  function openComposer() {
    setPickedListId((prev) => prev || lists[0]?.id || '')
    setComposerError(null)
    setComposerOpen(true)
  }

  function toggleAddon(id: string) {
    setPickedAddons((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleVps(id: string) {
    setPickedVpsIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  async function submitComposer(e: FormEvent) {
    e.preventDefault()
    setComposerError(null)
    const list = lists.find((l) => l.id === pickedListId)
    if (!list) {
      onToast({ kind: 'error', title: 'Pick a target list', message: 'Upload one on the Lists tab if none are available.' })
      return
    }
    if (pickedVpsIds.size === 0) {
      setComposerError('Pick at least one worker.')
      return
    }
    // Translate selected vpsIds → IPs/hosts that the backend can reach.
    const pickedNodes = deployableFleet.filter((n) => pickedVpsIds.has(n.id))
    const worker_ips = pickedNodes.map((n) => n.host).filter((h): h is string => !!h)
    if (worker_ips.length === 0) {
      setComposerError('Selected workers have no host/IP. Re-enrol them via Fleet.')
      return
    }
    const trimmedName = sessionName.trim() || `crack-${new Date().toISOString().slice(0, 16).replace(/[-:T]/g, '')}`

    setSubmitting(true)
    try {
      const r = await crack.start({
        session_name: trimmedName,
        list_id: list.id,
        addon_ids: Array.from(pickedAddons),
        worker_ips,
      })
      if (!r.ok || !r.session) {
        setComposerError(r.error || 'Backend rejected the request.')
        return
      }
      // Optimistic prepend; the next poll cycle will reconcile.
      setSessions((prev) => {
        const existing = prev.find((s) => s.id === r.session.id)
        if (existing) return prev
        return [r.session, ...prev]
      })
      // Trigger a fresh fetch so liveness/pids land within the same tick.
      crack.list()
        .then((rr) => { if (Array.isArray(rr?.sessions)) setSessions(rr.sessions) })
        .catch(() => { /* poller will catch up */ })
      onToast({ kind: 'info', title: 'Crack queued', message: `${trimmedName} → ${pickedNodes.length} worker(s)` })
      // Reset form, close panel.
      setSessionName('')
      setPickedAddons(new Set())
      setPickedVpsIds(new Set())
      setComposerOpen(false)
    } catch (err) {
      const e = err as Error
      setComposerError(e.message || 'Submit failed')
    } finally {
      setSubmitting(false)
    }
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

  const canSubmit = pickedVpsIds.size > 0 && lists.length > 0 && !submitting

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
          onNew={openComposer}
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

      {composerOpen && (
        <div
          className="cw-hub-modal__backdrop"
          role="dialog"
          aria-modal="true"
          onClick={() => { if (!submitting) setComposerOpen(false) }}
        >
          <div className="cw-hub-modal" style={{ width: 'min(680px, 92vw)' }} onClick={(e) => e.stopPropagation()}>
            <header className="cw-hub-modal__head">
              <div>
                <h3 style={{ margin: 0 }}>Compose new crack</h3>
                <p className="muted" style={{ margin: '.25rem 0 0', fontSize: '.82rem' }}>
                  Pick a target list, choose addons, name the session, and select the workers to split it across.
                </p>
              </div>
              <button
                type="button"
                className="btn-glass btn-glass--xs"
                onClick={() => setComposerOpen(false)}
                disabled={submitting}
              >
                Close
              </button>
            </header>

            <form onSubmit={submitComposer} className="cw-composer">
              <label className="cw-composer__field">
                <span className="cw-composer__label">Session name</span>
                <input
                  className="tg-input"
                  type="text"
                  value={sessionName}
                  onChange={(e) => setSessionName(e.target.value)}
                  placeholder="e.g. nightly-aws-sweep"
                  spellCheck={false}
                />
              </label>
              <label className="cw-composer__field">
                <span className="cw-composer__label">Target list</span>
                {lists.length === 0 ? (
                  <span className="muted">No lists uploaded — add one on the Lists tab.</span>
                ) : (
                  <select
                    className="tg-input"
                    value={pickedListId}
                    onChange={(e) => setPickedListId(e.target.value)}
                  >
                    {lists.map((l) => (
                      <option key={l.id} value={l.id}>
                        {l.name} ({l.lineCount.toLocaleString()} lines)
                      </option>
                    ))}
                  </select>
                )}
              </label>
              <fieldset className="cw-composer__field">
                <legend className="cw-composer__label">Addons ({pickedAddons.size} selected)</legend>
                <div className="cw-composer__addons">
                  {composerAddons.map((a) => {
                    const on = pickedAddons.has(a.id)
                    const { domain, Glyph } = brandFor(a)
                    return (
                      <button
                        key={a.id}
                        type="button"
                        className={`cw-addon${on ? ' cw-addon--on' : ''}`}
                        onClick={() => toggleAddon(a.id)}
                        aria-pressed={on}
                      >
                        <span className="cw-addon__logo" aria-hidden>
                          {domain ? (
                            <BrandLogo domain={domain} Fallback={Glyph} alt={a.label} size={42} />
                          ) : (
                            <Glyph width={42} height={42} />
                          )}
                        </span>
                        <span className="cw-addon__label">{a.label}</span>
                        <span className={`cw-addon__state cw-addon__state--${on ? 'on' : 'off'}`}>
                          {on ? 'ON' : 'OFF'}
                        </span>
                      </button>
                    )
                  })}
                  {composerAddons.length === 0 && (
                    <span className="muted">No addons enabled. Toggle some on in Settings → Cracker addons.</span>
                  )}
                </div>
              </fieldset>

              <fieldset className="cw-composer__field">
                <legend className="cw-composer__label">
                  Workers ({pickedVpsIds.size} of {deployableFleet.length} selected)
                </legend>
                <div className="tlist__chips">
                  {deployableFleet.length === 0 ? (
                    <span className="muted-callout">No healthy workers — add nodes via Fleet first.</span>
                  ) : (
                    deployableFleet.map((node) => {
                      const on = pickedVpsIds.has(node.id)
                      return (
                        <button
                          key={node.id}
                          type="button"
                          className={`tlist-chip${on ? ' tlist-chip--on' : ''}`}
                          onClick={() => toggleVps(node.id)}
                          title={`${node.region} · ${node.host}${node.source === 'discovered' ? ' · discovered' : ''}`}
                        >
                          <span className={`tlist-chip__dot tlist-chip__dot--${node.status}`} aria-hidden />
                          {node.label}
                          {node.source === 'discovered' && <span className="tlist-chip__disc">disc</span>}
                        </button>
                      )
                    })
                  )}
                </div>
              </fieldset>

              {composerError && (
                <p className="settings-hint" style={{ color: 'var(--danger)', marginTop: '.5rem' }}>{composerError}</p>
              )}

              <div className="settings-btn-row" style={{ marginTop: '0.75rem' }}>
                <button type="submit" className="btn-primary" disabled={!canSubmit}>
                  {submitting ? 'Queueing…' : 'Queue crack'}
                </button>
              </div>
            </form>

            {sessions.length > 0 && (
              <div className="muted-callout" style={{ marginTop: '0.75rem' }}>
                <strong>Active crack sessions</strong>
                <ul style={{ marginTop: '0.5rem', paddingLeft: '1.2rem' }}>
                  {sessions.map((s) => {
                    const addonLabels = s.addon_ids.map((id) =>
                      ADDON_CATALOG.find((a) => a.id === id)?.label ?? id,
                    )
                    return (
                      <li key={s.id} style={{ marginBottom: '0.25rem' }}>
                        <strong>{s.name}</strong>
                        {' · '}<code>{s.status}</code>
                        {' · list '}<code>{s.list_name}</code>
                        {' · '}<code>{s.worker_ips.length} worker(s)</code>
                        {addonLabels.length > 0
                          ? <> · addons: <code>{addonLabels.join(', ')}</code></>
                          : null}
                        {(s.status === 'running' || s.status === 'queued') && (
                          <button
                            type="button"
                            className="btn-glass btn-glass--xs"
                            onClick={() => {
                              crack.stop(s.id)
                                .then(() => crack.list())
                                .then((rr) => { if (Array.isArray(rr?.sessions)) setSessions(rr.sessions) })
                                .catch(() => { /* swallow */ })
                            }}
                            style={{ marginLeft: '0.4rem' }}
                          >
                            Stop
                          </button>
                        )}
                        {(s.status === 'completed' || s.status === 'stopped' || s.status === 'failed') && (
                          <button
                            type="button"
                            className="btn-glass btn-glass--xs"
                            onClick={() => {
                              crack.remove(s.id)
                                .then(() => setSessions((prev) => prev.filter((x) => x.id !== s.id)))
                                .catch(() => { /* swallow */ })
                            }}
                            style={{ marginLeft: '0.4rem' }}
                          >
                            Dismiss
                          </button>
                        )}
                      </li>
                    )
                  })}
                </ul>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
