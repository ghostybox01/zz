import { useEffect, useState } from 'react'
import { BUILD_SHA, REPO_SLUG, fetchLatestUpstreamSha } from '../lib/buildInfo'

const SEEN_KEY = 'reconx.update.seenSha.v1'
const CHECK_INTERVAL_MS = 5 * 60 * 1000 // 5 min

type Status =
  | { kind: 'idle' }
  | { kind: 'newer'; sha: string }
  | { kind: 'modal'; sha: string }

export function UpdateBanner() {
  const [status, setStatus] = useState<Status>({ kind: 'idle' })
  const [seen, setSeen] = useState<string>(() => window.localStorage.getItem(SEEN_KEY) ?? '')

  useEffect(() => {
    if (!REPO_SLUG) return
    let cancelled = false
    async function check() {
      try {
        const sha = await fetchLatestUpstreamSha()
        if (cancelled || !sha) return
        const short = sha.slice(0, 12)
        if (short !== BUILD_SHA && short !== seen) {
          setStatus({ kind: 'newer', sha: short })
        }
      } catch { /* network hiccup — try again next interval */ }
    }
    void check()
    const id = window.setInterval(check, CHECK_INTERVAL_MS)
    return () => { cancelled = true; window.clearInterval(id) }
  }, [seen])

  if (!REPO_SLUG) return null
  if (status.kind === 'idle') return null

  const sha = status.kind === 'newer' ? status.sha : status.kind === 'modal' ? status.sha : ''
  const dismiss = () => {
    window.localStorage.setItem(SEEN_KEY, sha)
    setSeen(sha)
    setStatus({ kind: 'idle' })
  }

  return (
    <>
      {status.kind === 'newer' && (
        <div className="update-banner" role="alert">
          <span className="update-banner__dot" aria-hidden />
          <span className="update-banner__text">
            New ReconX build available — <strong>{sha}</strong> on GitHub. Your build is at <code>{BUILD_SHA}</code>.
          </span>
          <div className="update-banner__actions">
            <button type="button" className="btn-primary update-banner__cta" onClick={() => setStatus({ kind: 'modal', sha })}>
              Update now
            </button>
            <button type="button" className="update-banner__close" onClick={dismiss} aria-label="Dismiss">
              ×
            </button>
          </div>
        </div>
      )}

      {status.kind === 'modal' && (
        <div className="cw-hub-modal__backdrop" role="dialog" aria-modal="true" onClick={dismiss}>
          <div className="cw-hub-modal update-modal" onClick={(e) => e.stopPropagation()}>
            <header className="cw-hub-modal__head">
              <div>
                <h3 style={{ margin: 0 }}>Install update {sha}?</h3>
                <p className="muted" style={{ margin: '.3rem 0 0', fontSize: '.82rem' }}>
                  GitHub <code>{REPO_SLUG}</code> has a newer commit. Your current build is <code>{BUILD_SHA}</code>.
                </p>
              </div>
            </header>
            <p style={{ margin: '.5rem 0', fontSize: '.85rem', color: 'var(--muted)', lineHeight: 1.5 }}>
              The dashboard can't update itself — the controller VPS owns the build. To install:
            </p>
            <ol style={{ margin: '.25rem 0 .75rem 1rem', fontSize: '.85rem', color: 'var(--text-strong)', lineHeight: 1.6 }}>
              <li>SSH to the controller</li>
              <li><code>cd /opt/reconx && git pull</code></li>
              <li><code>sudo python3 installer/deploy.py</code></li>
            </ol>
            <p style={{ margin: '.25rem 0', fontSize: '.78rem', color: 'var(--muted)' }}>
              The installer is idempotent — it rebuilds the dashboard + scanner binary and restarts services.
            </p>
            <div className="settings-btn-row" style={{ marginTop: '.85rem' }}>
              <button type="button" className="btn-primary" onClick={() => {
                window.open(`https://github.com/${REPO_SLUG}/compare/${BUILD_SHA}...${sha}`, '_blank')
              }}>
                View diff on GitHub
              </button>
              <button type="button" className="btn-secondary" onClick={() => {
                navigator.clipboard?.writeText(`cd /opt/reconx && git pull && sudo python3 installer/deploy.py`)
              }}>
                Copy install command
              </button>
              <button type="button" className="btn-glass" onClick={dismiss}>
                Skip this version
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
