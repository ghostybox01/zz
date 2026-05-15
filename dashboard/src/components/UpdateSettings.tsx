import { useEffect, useState } from 'react'
import { BUILD_SHA, BUILD_AT, REPO_SLUG, fetchLatestUpstreamSha } from '../lib/buildInfo'

const SEEN_KEY = 'reconx.update.seenSha.v1'

type State =
  | { kind: 'idle' }
  | { kind: 'checking' }
  | { kind: 'up-to-date'; sha: string }
  | { kind: 'update'; sha: string }
  | { kind: 'error'; message: string }

export function UpdateSettings() {
  const [state, setState] = useState<State>({ kind: 'idle' })
  const [seen, setSeen] = useState<string>(() => window.localStorage.getItem(SEEN_KEY) ?? '')

  async function check() {
    if (!REPO_SLUG) {
      setState({ kind: 'error', message: 'No repo configured. Set VITE/RECONX_REPO at build time (e.g. RECONX_REPO=myuser/reconx npm run build).' })
      return
    }
    setState({ kind: 'checking' })
    try {
      const upstream = await fetchLatestUpstreamSha()
      if (!upstream) {
        setState({ kind: 'error', message: 'GitHub returned no commit SHA.' })
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

  useEffect(() => {
    // Auto-check on mount when repo is configured. Modal shows only if a new SHA
    // is found AND the user hasn't dismissed THAT specific SHA yet.
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
          Checks <code>{REPO_SLUG || '(repo not set)'}</code> on GitHub for newer commits than this build.
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
            {state.kind === 'checking' && 'Checking GitHub…'}
            {state.kind === 'up-to-date' && (
              <span style={{ color: 'var(--ok)' }}>Up to date ({state.sha})</span>
            )}
            {state.kind === 'update' && (
              <span style={{ color: 'var(--gold)' }}>Update available → {state.sha}</span>
            )}
            {state.kind === 'error' && (
              <span style={{ color: 'var(--danger)' }}>{state.message}</span>
            )}
          </span>
        </div>
      </dl>

      <div className="settings-btn-row" style={{ marginTop: '.75rem' }}>
        <button type="button" className="btn-primary" onClick={() => void check()} disabled={state.kind === 'checking'}>
          {state.kind === 'checking' ? 'Checking…' : 'Check for updates'}
        </button>
      </div>

      {showModal && state.kind === 'update' && (
        <div className="cw-hub-modal__backdrop" role="dialog" aria-modal="true" onClick={dismiss}>
          <div className="cw-hub-modal update-modal" onClick={(e) => e.stopPropagation()}>
            <header className="cw-hub-modal__head">
              <div>
                <h3 style={{ margin: 0 }}>Update available</h3>
                <p className="muted" style={{ margin: '.25rem 0 0', fontSize: '.82rem' }}>
                  GitHub <code>{REPO_SLUG}</code> has commit <code>{state.sha}</code> — your build is at <code>{BUILD_SHA}</code>.
                </p>
              </div>
            </header>
            <p style={{ margin: '.5rem 0', fontSize: '.85rem', color: 'var(--muted)' }}>
              To install, rerun <code>installer/deploy.py</code> on the controller VPS (or click "Open install
              instructions" below). The installer re-pulls, rebuilds the dashboard and Go scanner, and restarts services.
            </p>
            <div className="settings-btn-row">
              <button type="button" className="btn-primary" onClick={() => {
                window.open(`https://github.com/${REPO_SLUG}/compare/${BUILD_SHA}...${state.sha}`, '_blank')
              }}>
                View diff on GitHub
              </button>
              <button type="button" className="btn-secondary" onClick={() => {
                window.open(`https://github.com/${REPO_SLUG}#install`, '_blank')
              }}>
                Open install instructions
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
