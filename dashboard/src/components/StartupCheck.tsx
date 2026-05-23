import { useEffect, useState } from 'react'

type ProbeState = 'pending' | 'ok' | 'fail' | 'skip'

type Probe = {
  id: string
  label: string
  description: string
  /** Returns truthy on success. Rejecting => fail. Resolving with false => skip. */
  run: () => Promise<boolean>
}

const PROBES: Probe[] = [
  {
    id: 'flask',
    label: 'Dashboard backend',
    description: 'Flask + socket.io on the controller',
    run: async () => {
      const r = await fetch('/api/stats', { headers: { Accept: 'application/json' } })
      return r.ok
    },
  },
  {
    id: 'fleet-mgr',
    label: 'Fleet manager (SSH)',
    description: 'paramiko-backed control plane',
    run: async () => {
      const r = await fetch('/api/vps/available', { headers: { Accept: 'application/json' } })
      if (!r.ok) return false
      const j = (await r.json()) as { available?: boolean }
      return !!j.available
    },
  },
  {
    id: 'scanner-config',
    label: 'Scanner configuration',
    description: 'config.json flags reachable',
    run: async () => {
      const r = await fetch('/api/scanner-config')
      return r.ok
    },
  },
  {
    id: 'scanner-paths',
    label: 'Path-list service',
    description: 'paths.txt override endpoint',
    run: async () => {
      const r = await fetch('/api/scanner-paths')
      return r.ok
    },
  },
  {
    id: 'telegram',
    label: 'Telegram relay',
    description: 'bot_token / chat_id status',
    run: async () => {
      const r = await fetch('/api/telegram')
      return r.ok
    },
  },
  {
    id: 'fleet-roster',
    label: 'Fleet roster',
    description: 'live workers status',
    run: async () => {
      const r = await fetch('/api/vps/status')
      return r.ok
    },
  },
  {
    id: 'dorks',
    label: 'Dork hunter',
    description: 'AI generator + Shodan/FOFA/Google search endpoints',
    run: async () => {
      const r = await fetch('/api/dorks/keys', { headers: { Accept: 'application/json' } })
      return r.ok
    },
  },
]

type Props = {
  onDone: () => void
}

const SKIP_KEY = 'reconx.startupCheck.skip'

export function StartupCheck({ onDone }: Props) {
  const [states, setStates] = useState<Record<string, ProbeState>>(
    () => Object.fromEntries(PROBES.map((p) => [p.id, 'pending']))
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [done, setDone] = useState(false)

  useEffect(() => {
    let cancelled = false
    async function run() {
      for (const p of PROBES) {
        if (cancelled) return
        try {
          const ok = await Promise.race<Promise<boolean>>([
            p.run(),
            new Promise<boolean>((_, reject) => setTimeout(() => reject(new Error('timeout')), 4000)),
          ])
          if (cancelled) return
          setStates((prev) => ({ ...prev, [p.id]: ok ? 'ok' : 'fail' }))
          if (!ok) setErrors((prev) => ({ ...prev, [p.id]: 'unhealthy response' }))
        } catch (e) {
          if (cancelled) return
          setStates((prev) => ({ ...prev, [p.id]: 'fail' }))
          setErrors((prev) => ({ ...prev, [p.id]: (e as Error).message || 'error' }))
        }
        // Stagger so the UI animates
        await new Promise((r) => setTimeout(r, 180))
      }
      if (!cancelled) setDone(true)
    }
    void run()
    return () => { cancelled = true }
  }, [])

  const okCount = Object.values(states).filter((s) => s === 'ok').length
  const failCount = Object.values(states).filter((s) => s === 'fail').length

  return (
    <div className="startup-check" role="dialog" aria-modal="true" aria-label="Connecting to controller">
      <div className="startup-check__card">
        <header className="startup-check__head">
          <div>
            <h2 className="startup-check__title">Connecting to controller</h2>
            <p className="startup-check__sub">
              {done
                ? failCount === 0
                  ? 'All checks passed.'
                  : `${okCount} of ${PROBES.length} endpoints reachable. The others may need attention.`
                : 'Probing each endpoint…'}
            </p>
          </div>
          <div className="startup-check__count">
            <strong>{okCount}</strong>
            <span>/ {PROBES.length}</span>
          </div>
        </header>

        <ul className="startup-check__list">
          {PROBES.map((p) => {
            const s = states[p.id]
            return (
              <li key={p.id} className={`startup-check__row startup-check__row--${s}`}>
                <span className="startup-check__icon" aria-hidden>
                  {s === 'pending' && <Spinner />}
                  {s === 'ok'      && (
                    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M20 6 9 17l-5-5" />
                    </svg>
                  )}
                  {s === 'fail'    && (
                    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M18 6 6 18M6 6l12 12" />
                    </svg>
                  )}
                </span>
                <span className="startup-check__label">
                  <strong>{p.label}</strong>
                  <span className="startup-check__desc">
                    {s === 'fail' && errors[p.id] ? errors[p.id] : p.description}
                  </span>
                </span>
                <span className="startup-check__status">
                  {s === 'pending' ? 'probing…' : s === 'ok' ? 'OK' : 'FAIL'}
                </span>
              </li>
            )
          })}
        </ul>

        <footer className="startup-check__foot">
          <button
            type="button"
            className="btn-glass"
            onClick={() => { window.localStorage.setItem(SKIP_KEY, '1'); onDone() }}
            disabled={!done}
            title={done ? 'Skip this check on future loads' : 'Wait for probes to finish'}
          >
            Skip on next launch
          </button>
          <button
            type="button"
            className="btn-primary"
            onClick={onDone}
            disabled={!done}
          >
            {failCount === 0 ? 'Enter dashboard' : 'Enter anyway'}
          </button>
        </footer>
      </div>
    </div>
  )
}

function Spinner() {
  return (
    <svg viewBox="0 0 24 24" width="18" height="18" className="startup-check__spinner" aria-hidden>
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="2" strokeOpacity="0.18" />
      <path d="M21 12a9 9 0 0 1-9 9" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  )
}

export function shouldSkipStartupCheck(): boolean {
  return window.localStorage.getItem(SKIP_KEY) === '1'
}
