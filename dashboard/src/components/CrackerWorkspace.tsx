import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
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

// Addon catalogue for the New-Crack composer — mirrors AddonsStrip's tiles so
// the operator sees the same names. Each entry maps to the scanner-config flag
// that the workers actually consume. The composer toggles `selectedAddonIds`
// locally; on submit we surface the full picked-set in the queued callout.
const COMPOSER_ADDONS: ReadonlyArray<{ id: string; label: string }> = [
  { id: 'ai',        label: 'AI Keys' },
  { id: 'ses',       label: 'AWS SES' },
  { id: 'aws-deep',  label: 'AWS Deep' },
  { id: 'sendgrid',  label: 'SendGrid' },
  { id: 'mailgun',   label: 'Mailgun' },
  { id: 'brevo',     label: 'Brevo' },
  { id: 'mandrill',  label: 'Mandrill' },
  { id: 'stripe',    label: 'Stripe' },
  { id: 'twilio',    label: 'Twilio' },
  { id: 'github',    label: 'GitHub' },
  { id: 'smtp',      label: 'Random SMTP' },
]

type QueuedCrack = {
  id: string
  sessionName: string
  listId: string
  listName: string
  addonIds: ReadonlyArray<string>
  queuedAt: string
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

  // New-Crack composer state — local only; no backend POST exists for
  // "start crack session", so submissions stay client-side and surface as
  // a queued-list callout (Path B in the brief).
  const [composerOpen, setComposerOpen] = useState(false)
  const [sessionName, setSessionName] = useState('')
  const [pickedListId, setPickedListId] = useState<string>('')
  const [pickedAddons, setPickedAddons] = useState<ReadonlySet<string>>(() => new Set())
  const [queued, setQueued] = useState<ReadonlyArray<QueuedCrack>>([])

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

  function openComposer() {
    // Default the picked list to the first available so the operator can
    // submit immediately when there's only one option.
    setPickedListId((prev) => prev || lists[0]?.id || '')
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

  function submitComposer(e: FormEvent) {
    e.preventDefault()
    const list = lists.find((l) => l.id === pickedListId)
    if (!list) {
      onToast({ kind: 'error', title: 'Pick a target list', message: 'Upload one on the Lists tab if none are available.' })
      return
    }
    const trimmedName = sessionName.trim() || `crack-${new Date().toISOString().slice(0, 16).replace(/[-:T]/g, '')}`
    const entry: QueuedCrack = {
      id: `q-${Date.now().toString(36)}`,
      sessionName: trimmedName,
      listId: list.id,
      listName: list.name,
      addonIds: Array.from(pickedAddons),
      queuedAt: new Date().toISOString(),
    }
    setQueued((prev) => [entry, ...prev])
    // Reset the form for the next submission but leave the panel open so the
    // operator can see the queued entry appear immediately.
    setSessionName('')
    setPickedAddons(new Set())
  }

  function clearQueued(id: string) {
    setQueued((prev) => prev.filter((q) => q.id !== id))
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
          onNew={openComposer}
          onTogglePause={onTogglePause}
          onStop={onTogglePause}
        />

        <div className="cw__main">
          {composerOpen && (
            <section className="card-block card-block--tight" style={{ marginBottom: '0.75rem' }}>
              <div className="card-block__head">
                <h3 style={{ margin: 0 }}>Compose new crack</h3>
                <p className="card-block__lede card-block__lede--short">
                  Pick a target list, choose addons, name the session. There is no backend
                  endpoint to spawn a crack run yet — submissions queue locally so the
                  operator can see what would have been fired.
                </p>
              </div>
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
                    {COMPOSER_ADDONS.map((a) => {
                      const on = pickedAddons.has(a.id)
                      return (
                        <button
                          key={a.id}
                          type="button"
                          className={`cw-addon${on ? ' cw-addon--on' : ''}`}
                          onClick={() => toggleAddon(a.id)}
                          aria-pressed={on}
                          style={{ minWidth: '7rem' }}
                        >
                          <span className="cw-addon__label">{a.label}</span>
                          <span className={`cw-addon__state cw-addon__state--${on ? 'on' : 'off'}`}>
                            {on ? 'ON' : 'OFF'}
                          </span>
                        </button>
                      )
                    })}
                  </div>
                </fieldset>
                <div className="settings-btn-row" style={{ marginTop: '0.75rem' }}>
                  <button type="submit" className="btn-primary" disabled={lists.length === 0}>
                    Queue crack
                  </button>
                  <button
                    type="button"
                    className="btn-glass btn-glass--xs"
                    onClick={() => setComposerOpen(false)}
                  >
                    Close
                  </button>
                </div>
              </form>

              {queued.length > 0 && (
                <div className="muted-callout" style={{ marginTop: '0.75rem' }}>
                  <strong>Crack session queued locally — no backend handler is wired yet.</strong>
                  {' '}The selected addons + target list have been saved to the workspace. These entries clear on page refresh.
                  <ul style={{ marginTop: '0.5rem', paddingLeft: '1.2rem' }}>
                    {queued.map((q) => (
                      <li key={q.id} style={{ marginBottom: '0.25rem' }}>
                        <strong>{q.sessionName}</strong> — list <code>{q.listName}</code>
                        {q.addonIds.length > 0
                          ? <> · addons: <code>{q.addonIds.join(', ')}</code></>
                          : <> · no addons</>}
                        {' '}
                        <button
                          type="button"
                          className="btn-glass btn-glass--xs"
                          onClick={() => clearQueued(q.id)}
                          style={{ marginLeft: '0.4rem' }}
                        >
                          Dismiss
                        </button>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </section>
          )}

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
