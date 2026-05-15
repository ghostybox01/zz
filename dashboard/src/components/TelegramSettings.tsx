import { useEffect, useState } from 'react'
import { loadTelegramPrefs, saveTelegramPrefs } from '../lib/telegramPrefs'
import { telegramApi, type ReconTelegramView } from '../lib/reconApi'

type Status = { kind: 'ok' | 'err' | 'info'; text: string } | null

export function TelegramSettings() {
  const [botToken, setBotToken] = useState(() => loadTelegramPrefs().botToken)
  const [chatId, setChatId] = useState(() => loadTelegramPrefs().chatId)
  const [notifyNewHits, setNotifyNewHits] = useState(() => loadTelegramPrefs().notifyNewHits)
  const [remote, setRemote] = useState<ReconTelegramView | null>(null)
  const [backendReachable, setBackendReachable] = useState(false)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [status, setStatus] = useState<Status>(null)

  // Always mirror local prefs for offline use.
  useEffect(() => {
    saveTelegramPrefs({ botToken, chatId, notifyNewHits })
  }, [botToken, chatId, notifyNewHits])

  // Pull current backend state on mount; if reachable, prefer the backend's chat_id.
  useEffect(() => {
    let cancelled = false
    telegramApi
      .get()
      .then((view) => {
        if (cancelled) return
        setRemote(view)
        setBackendReachable(true)
        if (view.chat_id && !chatId) setChatId(view.chat_id)
      })
      .catch(() => {
        if (!cancelled) setBackendReachable(false)
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function pushToBackend() {
    if (!backendReachable) {
      setStatus({ kind: 'info', text: 'Backend unreachable — saved to browser only.' })
      window.setTimeout(() => setStatus(null), 4500)
      return
    }
    setSaving(true)
    try {
      const next = await telegramApi.update({
        bot_token: botToken,
        chat_id: chatId,
      })
      setRemote(next)
      setStatus({ kind: 'ok', text: `Saved to backend (chat ${next.chat_id || '—'}).` })
    } catch (e) {
      setStatus({ kind: 'err', text: (e as Error).message })
    } finally {
      setSaving(false)
      window.setTimeout(() => setStatus(null), 5500)
    }
  }

  async function sendTest() {
    if (!backendReachable) {
      setStatus({ kind: 'info', text: 'Backend unreachable — cannot send via API.' })
      window.setTimeout(() => setStatus(null), 4500)
      return
    }
    setTesting(true)
    try {
      const res = await telegramApi.test('ReconX dashboard — telegram test ping.')
      if (res.success) setStatus({ kind: 'ok', text: 'Test message delivered.' })
      else setStatus({ kind: 'err', text: res.error ?? 'Telegram refused.' })
    } catch (e) {
      setStatus({ kind: 'err', text: (e as Error).message })
    } finally {
      setTesting(false)
      window.setTimeout(() => setStatus(null), 6500)
    }
  }

  return (
    <section className="card-block card-block--tight">
      <div className="card-block__head">
        <h2>Telegram</h2>
        <p className="card-block__lede card-block__lede--short">
          Stored in <code>raven/config.json</code> when the backend is reachable, in your browser otherwise.
          {remote?.has_token && (
            <>
              {' '}
              <span className="tg-muted">Backend currently has token {remote.token_tail}.</span>
            </>
          )}
        </p>
      </div>

      <label className="tg-field">
        <span>Bot token</span>
        <input
          className="tg-input"
          type="password"
          autoComplete="off"
          value={botToken}
          onChange={(e) => setBotToken(e.target.value)}
          placeholder="123456789:ABC..."
        />
      </label>

      <label className="tg-field">
        <span>Chat ID</span>
        <input
          className="tg-input"
          type="text"
          autoComplete="off"
          value={chatId}
          onChange={(e) => setChatId(e.target.value)}
          placeholder="-100xxxxxxxx or user id"
        />
      </label>

      <label className="tg-toggle">
        <input
          type="checkbox"
          checked={notifyNewHits}
          onChange={(e) => setNotifyNewHits(e.target.checked)}
        />
        <span>
          Notify on each new finding <span className="tg-muted">(local pref — workers relay)</span>
        </span>
      </label>

      <div className="settings-btn-row">
        <button
          type="button"
          className="btn-primary"
          onClick={pushToBackend}
          disabled={saving || !backendReachable}
          title={backendReachable ? 'Persist to raven/config.json' : 'Backend unreachable'}
        >
          {saving ? 'Saving…' : 'Save to backend'}
        </button>
        <button
          type="button"
          className="btn-secondary"
          onClick={sendTest}
          disabled={testing || !backendReachable || !chatId}
          title={
            !backendReachable
              ? 'Backend unreachable'
              : !chatId
                ? 'Set chat_id first'
                : 'Send a test message via /api/telegram/test'
          }
        >
          {testing ? 'Sending…' : 'Send test ping'}
        </button>
      </div>
      {status && (
        <p className={`settings-hint tg-hint tg-hint--${status.kind}`}>{status.text}</p>
      )}
    </section>
  )
}
