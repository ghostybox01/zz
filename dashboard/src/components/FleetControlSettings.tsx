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
        <h2>Fleet control plane</h2>
        <p className="card-block__lede card-block__lede--short">
          Optional backend for real SSH enroll + deploy. When disabled, discovered SSH/VPS hits still join the
          fleet in <strong>simulated</strong> mode. Run <code className="inline-code">python fleet_api.py</code> on
          your orchestrator host.
        </p>
      </div>

      <label className="tg-field tg-field--row">
        <span>Enable control plane</span>
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
    </section>
  )
}