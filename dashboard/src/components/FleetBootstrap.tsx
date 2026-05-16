import { useRef, useState } from 'react'
import { fleetBulkCreds, type BulkCredsResponse, type InstallKeysResponse } from '../lib/reconApi'

const PLACEHOLDER = `# One per line. Lines starting with # are ignored.
# Simplest — IP only (uses controller key + root):
198.51.100.10
198.51.100.11

# user@host
root@worker-01.example.com

# user@host:port:auth_kind:secret  (auth_kind = key | password)
root@198.51.100.20:22:key:/root/.ssh/worker.pem
deploy@198.51.100.21:2222:password:s3cretPass!`

export function FleetBootstrap() {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [installing, setInstalling] = useState(false)
  const [result, setResult] = useState<BulkCredsResponse | null>(null)
  /** Snapshot of the textarea contents at the moment Parse + test was clicked.
   * The install-keys call reuses these exact lines so it tries the same
   * passwords paramiko just verified. */
  const [testedText, setTestedText] = useState<string>('')
  const [install, setInstall] = useState<InstallKeysResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  async function runTest(payload: 'text' | File) {
    setBusy(true)
    setError(null)
    setInstall(null)
    try {
      if (payload === 'text') {
        setTestedText(text)
        setResult(await fleetBulkCreds.testText(text))
      } else {
        setTestedText(await payload.text())
        setResult(await fleetBulkCreds.testFile(payload))
      }
    } catch (e) {
      setError((e as Error).message)
      setResult(null)
    } finally {
      setBusy(false)
    }
  }

  async function runInstallKeys() {
    if (!testedText.trim()) return
    setInstalling(true)
    setError(null)
    try {
      const res = await fleetBulkCreds.installKeysText(testedText)
      setInstall(res)
      if (res.installed > 0) {
        // Wipe plaintext passwords from the DOM once they've served their purpose.
        setText('')
        setTestedText('')
      }
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setInstalling(false)
    }
  }

  return (
    <section className="card-block card-block--tight settings-section">
      <div className="card-block__head">
        <h2>Fleet bootstrap — SSH credentials</h2>
        <p className="card-block__lede card-block__lede--short">
          Paste or upload worker SSH creds. The controller parses each line, tests it with paramiko, and adds
          OK rows to <code>server_ips.txt</code>. Format reference in <code>installer.txt</code>.
        </p>
      </div>

      <div className="fleet-boot__row">
        <textarea
          className="tg-input fleet-boot__textarea"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={PLACEHOLDER}
          rows={10}
          spellCheck={false}
        />
      </div>

      <div className="settings-btn-row" style={{ marginTop: '.75rem' }}>
        <button
          type="button"
          className="btn-primary"
          onClick={() => void runTest('text')}
          disabled={busy || !text.trim()}
        >
          {busy ? 'Testing…' : 'Parse + test'}
        </button>
        <button
          type="button"
          className="btn-secondary"
          onClick={() => fileRef.current?.click()}
          disabled={busy}
        >
          Upload .txt
        </button>
        <input
          ref={fileRef}
          type="file"
          accept=".txt,text/plain"
          style={{ display: 'none' }}
          onChange={(e) => {
            const f = e.target.files?.[0]
            e.target.value = ''
            if (f) void runTest(f)
          }}
        />
        {result && (
          <span className="muted" style={{ alignSelf: 'center', fontSize: '.78rem' }}>
            {result.ok}/{result.total} reachable · {result.added_to_roster} added to roster
          </span>
        )}
      </div>

      {result && result.ok > 0 && testedText.trim() && (
        <div className="settings-btn-row" style={{ marginTop: '.5rem' }}>
          <button
            type="button"
            className="btn-primary"
            onClick={() => void runInstallKeys()}
            disabled={installing || busy}
            title="paramiko-connects with the tested password and appends the controller's id_ed25519.pub to ~user/.ssh/authorized_keys on each OK row"
          >
            {installing ? 'Installing…' : `Import to fleet — install controller key on ${result.ok} OK worker${result.ok === 1 ? '' : 's'}`}
          </button>
          {install && (
            <span className="muted" style={{ alignSelf: 'center', fontSize: '.78rem' }}>
              {install.installed} installed · {install.failed} failed · {install.skipped} skipped
            </span>
          )}
        </div>
      )}

      {install && install.results.length > 0 && (
        <div className="fleet-boot__results">
          <table className="fleet-boot__table">
            <thead>
              <tr><th>Host</th><th>User</th><th>Status</th><th>Message</th></tr>
            </thead>
            <tbody>
              {install.results.map((r, i) => {
                const skipped = r.message.startsWith('skipped')
                const pillClass = r.installed ? 'pill--ok' : skipped ? 'pill--muted' : 'pill--err'
                return (
                <tr key={`k-${i}`} className={r.installed ? 'fleet-boot__row--ok' : 'fleet-boot__row--fail'}>
                  <td className="mono">{r.host}</td>
                  <td className="mono">{r.user}</td>
                  <td>
                    <span className={`pill ${pillClass}`}>
                      {r.installed ? 'KEY INSTALLED' : skipped ? 'SKIPPED' : 'FAIL'}
                    </span>
                  </td>
                  <td className="muted" style={{ fontSize: '.75rem' }}>{r.message}</td>
                </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {error && <p className="settings-hint tg-hint tg-hint--err">{error}</p>}

      {result && result.results.length > 0 && !install && (
        <div className="fleet-boot__results">
          <table className="fleet-boot__table">
            <thead>
              <tr>
                <th>Host</th>
                <th>User</th>
                <th>Port</th>
                <th>Status</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {result.results.map((r, i) => (
                <tr key={i} className={r.ok ? 'fleet-boot__row--ok' : 'fleet-boot__row--fail'}>
                  <td className="mono">{r.host}</td>
                  <td className="mono">{r.user}</td>
                  <td className="mono">{r.port}</td>
                  <td>
                    <span className={`pill ${r.ok ? 'pill--ok' : 'pill--err'}`}>
                      {r.ok ? 'OK' : 'FAIL'}
                    </span>
                  </td>
                  <td className="muted" style={{ fontSize: '.75rem' }}>{r.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
