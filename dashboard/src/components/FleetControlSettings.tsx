import { useEffect, useState } from 'react'
import { saveFleetControl, type FleetControlConfig } from '../lib/fleetControl'

type Props = {
  config: FleetControlConfig
  onChange: (cfg: FleetControlConfig) => void
}

export function FleetControlSettings({ config, onChange }: Props) {
  const [draft, setDraft] = useState<FleetControlConfig>(config)

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- sync controlled prop into local draft
    setDraft(config)
  }, [config])

  const apply = (next: Partial<FleetControlConfig>) => {
    const merged = { ...draft, ...next }
    setDraft(merged)
    saveFleetControl(merged)
    onChange(merged)
  }

  return (
    <section className="card-block card-block--tight">
      <div className="card-block__head">
        <h2>Auto-enroll &amp; backend connection</h2>
        <p className="card-block__lede card-block__lede--short">
          When ON, discovered SSH/VPS hits POST to the controller's real backend instead of running in
          client-side simulation. Auto-enroll fires the moment a scan finding includes credentials.
        </p>
      </div>

      <label className="tg-field tg-field--row">
        <span>Use real backend</span>
        <input
          type="checkbox"
          checked={draft.enabled}
          onChange={(e) => apply({ enabled: e.target.checked })}
        />
      </label>

      <label className="tg-field tg-field--row">
        <span>Auto-enroll SSH/VPS hits</span>
        <input
          type="checkbox"
          checked={draft.autoEnroll}
          onChange={(e) => apply({ autoEnroll: e.target.checked })}
        />
      </label>

      <details className="settings-acc">
        <summary>Advanced</summary>
        <div className="settings-acc__body">
          <label className="tg-field">
            <span>Control plane URL</span>
            <input
              className="tg-input"
              type="url"
              autoComplete="off"
              spellCheck={false}
              value={draft.baseUrl}
              onChange={(e) => setDraft({ ...draft, baseUrl: e.target.value })}
              onBlur={() => apply({ baseUrl: draft.baseUrl })}
              placeholder="http://127.0.0.1:8787"
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
            />
          </label>

          <p className="settings-hint">
            Scanner should emit <code className="inline-code">ssh_valid.txt</code> or{' '}
            <code className="inline-code">vps_ssh_found.txt</code> lines as{' '}
            <code className="inline-code">source_url:host:user:secret</code>. Private keys in{' '}
            <code className="inline-code">private_keys_found.txt</code> enroll when paired with a host.
          </p>
        </div>
      </details>
    </section>
  )
}
