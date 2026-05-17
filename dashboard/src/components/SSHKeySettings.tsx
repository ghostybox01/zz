import { useEffect, useState } from 'react'
import { sshKey, type SSHKey } from '../lib/reconApi'

/** Controller SSH keypair viewer + rotator.
 *
 *  Reads /api/ssh-key on mount; renders pubkey + fingerprint + created_at.
 *  Operators can copy the pubkey to clipboard (so they can paste into a
 *  worker's authorized_keys manually) or hit Regenerate to rotate the
 *  keypair on the controller. Regeneration is gated behind an explicit
 *  confirm dialog because it invalidates every previously-imported worker
 *  until they re-receive the new public key via the Fleet Bootstrap panel.
 */
export function SSHKeySettings() {
  const [data, setData] = useState<SSHKey | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'error'>('idle')
  const [regenBusy, setRegenBusy] = useState(false)
  const [regenError, setRegenError] = useState<string | null>(null)
  const [banner, setBanner] = useState<string | null>(null)

  const reload = async () => {
    try {
      const r = await sshKey.get()
      setData(r)
      setLoadError(null)
    } catch (e) {
      setLoadError((e as Error).message)
    }
  }

  useEffect(() => {
    void reload()
  }, [])

  const onCopy = async () => {
    if (!data?.pubkey) return
    try {
      await navigator.clipboard.writeText(data.pubkey)
      setCopyState('copied')
      window.setTimeout(() => setCopyState('idle'), 1800)
    } catch {
      setCopyState('error')
      window.setTimeout(() => setCopyState('idle'), 2500)
    }
  }

  const onRegenerate = async () => {
    const confirmed = window.confirm(
      "Regenerate controller SSH keypair? You'll need to re-run Import to fleet in the Fleet Bootstrap panel for every worker so the new public key is installed in their authorized_keys. The old keypair will be deleted.",
    )
    if (!confirmed) return
    setRegenBusy(true)
    setRegenError(null)
    try {
      const r = await sshKey.regenerate()
      if (!r.ok) {
        setRegenError(r.error ?? r.message ?? 'Regeneration failed')
      } else {
        await reload()
        setBanner('Keypair regenerated. Re-run Import to fleet on every worker.')
      }
    } catch (e) {
      setRegenError((e as Error).message)
    } finally {
      setRegenBusy(false)
    }
  }

  if (loadError) {
    return (
      <section className="card-block card-block--tight ssh-key-block">
        <div className="card-block__head card-block__head--row">
          <div>
            <h2>Controller SSH keypair</h2>
            <p className="card-block__lede card-block__lede--short">
              Failed to load: {loadError}
            </p>
          </div>
        </div>
      </section>
    )
  }

  if (!data) {
    return (
      <section className="card-block card-block--tight ssh-key-block">
        <p className="muted">Loading SSH key…</p>
      </section>
    )
  }

  if (!data.exists) {
    return (
      <section className="card-block card-block--tight ssh-key-block">
        <div className="card-block__head card-block__head--row">
          <div>
            <h2>Controller SSH keypair</h2>
            <p className="card-block__lede card-block__lede--short">
              No controller key found. Run <code>install-controller.sh</code> to provision.
            </p>
          </div>
        </div>
      </section>
    )
  }

  const createdLabel = data.created_at ? new Date(data.created_at).toLocaleString() : '—'

  return (
    <section className="card-block card-block--tight ssh-key-block">
      <div className="card-block__head card-block__head--row">
        <div>
          <h2>Controller SSH keypair</h2>
          <p className="card-block__lede card-block__lede--short">
            The public key the controller pushes to each worker's <code>authorized_keys</code>.
          </p>
        </div>
      </div>

      <div className="kv kv--form">
        <div className="kv__row">
          <label className="kv__label" htmlFor="ssh-pubkey">Public key</label>
          <textarea
            id="ssh-pubkey"
            className="ssh-key-pubkey mono"
            readOnly
            value={data.pubkey}
            spellCheck={false}
          />
        </div>
        <div className="kv__row">
          <span className="kv__label">Fingerprint</span>
          <span className="ssh-key-fp mono">{data.fingerprint || '—'}</span>
        </div>
        <div className="kv__row">
          <span className="kv__label">Created</span>
          <span className="mono">{createdLabel}</span>
        </div>
      </div>

      <div className="ssh-key-row">
        <button type="button" className="btn-secondary" onClick={() => void onCopy()}>
          {copyState === 'copied' ? 'Copied ✓' : copyState === 'error' ? 'Copy failed' : 'Copy to clipboard'}
        </button>
        <button
          type="button"
          className="btn-danger-outline"
          onClick={() => void onRegenerate()}
          disabled={regenBusy}
          title="Rotate controller keypair — workers will need re-import"
        >
          {regenBusy ? 'Regenerating…' : 'Regenerate'}
        </button>
        {regenError && <span className="muted danger-text">{regenError}</span>}
      </div>

      {banner && (
        <div className="ssh-key-banner" role="status">
          {banner}
        </div>
      )}
    </section>
  )
}
