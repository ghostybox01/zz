import { useEffect, useState } from 'react'
import { loadLiveSource, saveLiveSource, type LiveSourceConfig } from '../lib/liveSource'
import type { LiveScanStatus } from '../hooks/useLiveScan'

type Props = {
  config: LiveSourceConfig
  onChange: (cfg: LiveSourceConfig) => void
  status: LiveScanStatus
}

export function LiveSourceSettings({ config, onChange, status }: Props) {
  const [draft, setDraft] = useState<LiveSourceConfig>(config)

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- sync controlled prop into local draft
    setDraft(config)
  }, [config])

  const apply = (next: Partial<LiveSourceConfig>) => {
    const merged = { ...draft, ...next }
    setDraft(merged)
    saveLiveSource(merged)
    onChange(merged)
  }

  const initial = loadLiveSource()
  const dirty =
    draft.baseUrl !== initial.baseUrl ||
    draft.bearerToken !== initial.bearerToken ||
    draft.pollIntervalMs !== initial.pollIntervalMs

  return (
    <section className="card-block card-block--tight">
      <div className="card-block__head">
        <h2>Live source</h2>
        <p className="card-block__lede card-block__lede--short">
          Point the dashboard at a VPS that serves the scanner's output directory over HTTP. When enabled, demo data is
          replaced by live findings polled from <code className="inline-code">{`<base>/valid_*.txt`}</code> and{' '}
          <code className="inline-code">{`<base>/*_found.txt`}</code>.
        </p>
      </div>

      <label className="tg-field">
        <span>Base URL</span>
        <input
          className="tg-input"
          type="url"
          autoComplete="off"
          spellCheck={false}
          value={draft.baseUrl}
          onChange={(e) => setDraft({ ...draft, baseUrl: e.target.value })}
          onBlur={() => apply({ baseUrl: draft.baseUrl })}
          placeholder="https://vps.example.com/results/"
        />
      </label>

      <label className="tg-field">
        <span>Bearer token (optional)</span>
        <input
          className="tg-input"
          type="password"
          autoComplete="off"
          value={draft.bearerToken}
          onChange={(e) => setDraft({ ...draft, bearerToken: e.target.value })}
          onBlur={() => apply({ bearerToken: draft.bearerToken })}
          placeholder="If your nginx requires Authorization: Bearer …"
        />
      </label>

      <label className="tg-field">
        <span>Poll interval (ms)</span>
        <input
          className="tg-input"
          type="number"
          min={1000}
          step={500}
          value={draft.pollIntervalMs}
          onChange={(e) => setDraft({ ...draft, pollIntervalMs: Number(e.target.value) || 5000 })}
          onBlur={() =>
            apply({ pollIntervalMs: Math.max(1000, Math.round(draft.pollIntervalMs)) })
          }
        />
      </label>

      <label className="tg-toggle">
        <input
          type="checkbox"
          checked={draft.enabled}
          onChange={(e) => apply({ enabled: e.target.checked })}
        />
        <span>
          Enable live polling <span className="tg-muted">(disables demo simulation)</span>
        </span>
      </label>

      <div className="settings-btn-row">
        <span className={`live-pill live-pill--${status.state}`}>
          {labelForStatus(status)}
        </span>
        {dirty ? <span className="tg-muted">Edits auto-save on blur.</span> : null}
      </div>
    </section>
  )
}

function labelForStatus(s: LiveScanStatus): string {
  switch (s.state) {
    case 'idle':
      return 'Idle — toggle off'
    case 'connecting':
      return 'Connecting…'
    case 'ok':
      return `Live · ${new Date(s.lastPollAt).toLocaleTimeString()} · ${s.filesSeen} file(s)`
    case 'error':
      return `Error · ${s.message}`
  }
}
