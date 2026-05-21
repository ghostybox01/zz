import { useEffect, useState } from 'react'
import { BUILD_SHA, BUILD_AT, REPO_SLUG, fetchLatestUpstreamSha } from '../lib/buildInfo'

// REPO_SLUG is intentionally NOT rendered anywhere user-visible — it
// still drives fetchLatestUpstreamSha under the hood (the build needs
// to know which GitHub repo to poll), but the operator UI stays
// vendor-neutral. No links out to github.com, no SHA-vs-repo prose,
// no "open install instructions" button that opens a browser tab.

const SEEN_KEY = 'reconx.update.seenSha.v1'

type State =
  | { kind: 'idle' }
  | { kind: 'checking' }
  | { kind: 'up-to-date'; sha: string }
  | { kind: 'update'; sha: string }
  | { kind: 'installing' }
  | { kind: 'installed' }
  | { kind: 'error'; message: string }

export function UpdateSettings() {
  const [state, setState] = useState<State>({ kind: 'idle' })
  const [seen, setSeen] = useState<string>(() => window.localStorage.getItem(SEEN_KEY) ?? '')

  async function check() {
    if (!REPO_SLUG) {
      // Build-time misconfig — surfaced generically so we don't leak
      // the configured slug into the operator UI.
      setState({ kind: 'error', message: 'Update source not configured at build time.' })
      return
    }
    setState({ kind: 'checking' })
    try {
      const upstream = await fetchLatestUpstreamSha()
      if (!upstream) {
        setState({ kind: 'error', message: 'No commit SHA returned from upstream.' })
        return
      }
      const short = upstream.slice(0, 12)
      if (short === BUILD_SHA) {
        setState({ kind: 'up-to-date', sha: short })
      } else {
        setState({ kind: 'update', sha: short })
      }
    } catch (e) {
      setState({ kind: 'error', message: (e as Error).message })
    }
  }

  // One-click install: POSTs /api/update on the controller, which
  // shells out to /usr/local/bin/reconx-update (sudoers-allowlisted)
  // detached. The helper does git fetch + reset + deploy.py and
  // restarts services — the dashboard typically comes back in ~30–90s.
  async function install() {
    setState({ kind: 'installing' })
    try {
      const res = await fetch('/api/update', { method: 'POST', headers: { Accept: 'application/json' } })
      if (!res.ok) {
        const body = await res.json().catch(() => ({} as { error?: string }))
        setState({ kind: 'error', message: body.error || `Install failed (HTTP ${res.status}).` })
        return
      }
      setState({ kind: 'installed' })
    } catch (e) {
      setState({ kind: 'error', message: (e as Error).message })
    }
  }

  useEffect(() => {
    // Auto-check on mount when the build knows where to look. Modal
    // only opens if a newer SHA is found AND the user hasn't already
    // dismissed THAT specific SHA.
    if (REPO_SLUG) void check()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const dismiss = () => {
    if (state.kind === 'update') {
      window.localStorage.setItem(SEEN_KEY, state.sha)
      setSeen(state.sha)
    }
  }

  const showModal = state.kind === 'update' && state.sha !== seen

  return (
    <section className="card-block card-block--tight settings-section">
      <div className="card-block__head">
        <h2>Updates</h2>
        <p className="card-block__lede card-block__lede--short">
          Checks for a newer build than what's running here.
        </p>
      </div>

      <dl className="settings-grid update-grid">
        <div className="tg-field">
          <span>Current build</span>
          <span className="mono" style={{ fontSize: '.92rem' }}>{BUILD_SHA}</span>
        </div>
        <div className="tg-field">
          <span>Built at</span>
          <span className="mono" style={{ fontSize: '.82rem' }}>{BUILD_AT}</span>
        </div>
        <div className="tg-field">
          <span>Status</span>
          <span style={{ fontSize: '.92rem' }}>
            {state.kind === 'idle' && 'Not checked'}
            {state.kind === 'checking' && 'Checking for updates…'}
            {state.kind === 'up-to-date' && (
              <span style={{ color: 'var(--ok)' }}>Up to date ({state.sha})</span>
            )}
            {state.kind === 'update' && (
              <span style={{ color: 'var(--gold)' }}>Newer build available → {state.sha}</span>
            )}
            {state.kind === 'installing' && (
              <span style={{ color: 'var(--gold)' }}>Installing — services will restart in 30–90s…</span>
            )}
            {state.kind === 'installed' && (
              <span style={{ color: 'var(--ok)' }}>Install started — refresh in ~60s to pick up the new build</span>
            )}
            {state.kind === 'error' && (
              <span style={{ color: 'var(--danger)' }}>{state.message}</span>
            )}
          </span>
        </div>
      </dl>

      <div className="settings-btn-row" style={{ marginTop: '.75rem' }}>
        <button type="button" className="btn-primary" onClick={() => void check()}
                disabled={state.kind === 'checking' || state.kind === 'installing'}>
          {state.kind === 'checking' ? 'Checking…' : 'Check for updates'}
        </button>
        {/* Inline install affordance for the common case where the
            operator has just clicked "Check" and got a newer SHA but
            dismissed (or never opened) the modal. Keeps a one-click
            path visible at all times when there's something to install. */}
        {state.kind === 'update' && (
          <button type="button" className="btn-primary" onClick={() => void install()}>
            Install now
          </button>
        )}
        {state.kind === 'installing' && (
          <button type="button" className="btn-primary" disabled>
            Installing…
          </button>
        )}
      </div>

      {showModal && state.kind === 'update' && (
        <div className="cw-hub-modal__backdrop" role="dialog" aria-modal="true" onClick={dismiss}>
          <div className="cw-hub-modal update-modal" onClick={(e) => e.stopPropagation()}>
            <header className="cw-hub-modal__head">
              <div>
                <h3 style={{ margin: 0 }}>Update available</h3>
                <p className="muted" style={{ margin: '.25rem 0 0', fontSize: '.82rem' }}>
                  A newer build (<code>{state.sha}</code>) is available — your current build is <code>{BUILD_SHA}</code>.
                </p>
              </div>
            </header>
            <p style={{ margin: '.5rem 0', fontSize: '.85rem', color: 'var(--muted)' }}>
              Click <strong>Install now</strong> to apply it. The controller will pull the new build,
              rebuild the dashboard and Go scanner, and restart services. Expect ~30–90s of downtime.
            </p>
            <div className="settings-btn-row">
              <button type="button" className="btn-primary" onClick={() => {
                dismiss()
                void install()
              }}>
                Install now
              </button>
              <button type="button" className="btn-glass" onClick={dismiss}>
                Skip for now
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}
