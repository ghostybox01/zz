import { useEffect, useMemo, useState } from 'react'
import {
  r2,
  type R2Account,
  type R2AccountInput,
  type R2Config,
  type R2HealthState,
  type R2Usage,
} from '../lib/reconApi'

// Mirrors the fleet-card status-pill family in App.css. `connected`
// uses the same green tone as a healthy worker; misconfigured = warn
// (orange); unreachable = bad (red); unknown = muted (gray). The dot
// glyph matches the existing live-pill convention.
const STATE_LABELS: Record<R2HealthState, string> = {
  connected: 'connected',
  misconfigured: 'misconfigured',
  unreachable: 'unreachable',
  unknown: 'unknown',
}
const STATE_PILL_CLASS: Record<R2HealthState, string> = {
  connected: 'status-pill status-pill--ok',
  misconfigured: 'status-pill status-pill--warn',
  unreachable: 'status-pill status-pill--bad',
  unknown: 'status-pill status-pill--muted',
}

/** Editable copy of an R2Account. We carry `_dirty` so the "Save all"
 *  button can flag which rows have unsaved changes, and `_isNew` so we
 *  know whether to mint an id on save. The masked secret placeholder
 *  is dropped before submit — the backend treats an empty string as
 *  "keep the previously-stored value", which is what we want for
 *  rows the operator didn't re-type. */
type EditableAccount = {
  id: string
  label: string
  account_id: string
  access_key_id: string
  secret_access_key: string
  bucket_name: string
  max_gb: number
  // Server-side health snapshot — read-only here.
  configured: boolean
  state: R2HealthState
  last_error: string | null
  usage: R2Usage | null
  // UI-only.
  _secretDirty: boolean
  _isNew: boolean
}

function toEditable(a: R2Account): EditableAccount {
  return {
    id: a.id,
    label: a.label || a.bucket_name || 'r2',
    account_id: a.account_id,
    access_key_id: a.access_key_id,
    secret_access_key: a.secret_access_key,
    bucket_name: a.bucket_name,
    max_gb: a.max_gb ?? 9.5,
    configured: a.configured,
    state: (a.state ?? 'unknown') as R2HealthState,
    last_error: a.last_error ?? null,
    usage: a.usage ?? null,
    _secretDirty: false,
    _isNew: false,
  }
}

function makeBlankAccount(): EditableAccount {
  return {
    id: `new-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`,
    label: '',
    account_id: '',
    access_key_id: '',
    secret_access_key: '',
    bucket_name: '',
    max_gb: 9.5,
    configured: false,
    state: 'misconfigured',
    last_error: null,
    usage: null,
    _secretDirty: true,
    _isNew: true,
  }
}

export function R2Settings() {
  const [accounts, setAccounts] = useState<EditableAccount[]>([])
  const [primaryId, setPrimaryId] = useState<string | null>(null)
  const [allFull, setAllFull] = useState(false)
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [saveError, setSaveError] = useState<string | null>(null)

  // CORS / delete per-account status flags.
  const [corsBusy, setCorsBusy] = useState<string | null>(null)
  const [corsMsg, setCorsMsg] = useState<{ id: string; ok: boolean; text: string } | null>(null)

  // Track which row the operator is currently editing so the polling
  // refresh doesn't clobber their typing. Without this, the 30-second
  // poll would re-fetch and reset every field every half minute.
  const [editingRowIds, setEditingRowIds] = useState<Set<string>>(new Set())

  useEffect(() => {
    let alive = true
    async function load() {
      try {
        const c = await r2.getConfig()
        if (!alive) return
        applyConfig(c, /* preserveDrafts */ true)
      } catch {
        // ignore — health probe will retry
      }
    }
    void load()
    const t = window.setInterval(load, 30_000)
    return () => { alive = false; window.clearInterval(t) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function applyConfig(c: R2Config, preserveDrafts: boolean) {
    const incoming = (c.accounts || []).map(toEditable)
    setPrimaryId(c.primary_id ?? (incoming[0]?.id ?? null))
    setAllFull(!!c.all_full)
    setAccounts((prev) => {
      if (!preserveDrafts) return incoming
      // Merge: if a previous row is in the editing set OR is _isNew,
      // keep its edited form; otherwise take the server value. New rows
      // that weren't saved get carried forward.
      const byId = new Map(incoming.map((a) => [a.id, a]))
      const merged: EditableAccount[] = []
      for (const row of prev) {
        const fresh = byId.get(row.id)
        if (row._isNew && !fresh) {
          merged.push(row); continue
        }
        if (editingRowIds.has(row.id) || row._secretDirty) {
          // Refresh server-derived fields only.
          merged.push(fresh ? {
            ...row,
            configured: fresh.configured,
            state: fresh.state,
            last_error: fresh.last_error,
            usage: fresh.usage,
          } : row)
          continue
        }
        if (fresh) {
          merged.push(fresh)
          byId.delete(row.id)
        }
      }
      // Any rows in `incoming` not yet shown — append them.
      for (const fresh of incoming) {
        if (!merged.find((m) => m.id === fresh.id)) merged.push(fresh)
      }
      return merged
    })
  }

  function markEditing(id: string) {
    setEditingRowIds((prev) => {
      if (prev.has(id)) return prev
      const next = new Set(prev)
      next.add(id)
      return next
    })
  }

  function updateRow(id: string, patch: Partial<EditableAccount>) {
    markEditing(id)
    setAccounts((prev) => prev.map((a) => (a.id === id ? { ...a, ...patch } : a)))
  }

  function addAccount() {
    setAccounts((prev) => [...prev, makeBlankAccount()])
  }

  function moveAccount(id: string, dir: -1 | 1) {
    setAccounts((prev) => {
      const idx = prev.findIndex((a) => a.id === id)
      if (idx < 0) return prev
      const j = idx + dir
      if (j < 0 || j >= prev.length) return prev
      const next = prev.slice()
      const [row] = next.splice(idx, 1)
      next.splice(j, 0, row)
      return next
    })
  }

  async function deleteRow(id: string) {
    const target = accounts.find((a) => a.id === id)
    if (!target) return
    const isPrimary = id === primaryId
    const confirmText = isPrimary
      ? `${target.label || target.bucket_name || id} is currently the active account. Delete anyway? The next-priority account becomes primary.`
      : `Delete account "${target.label || target.bucket_name || id}"? Stored credentials are removed; objects in the bucket are not touched.`
    if (!window.confirm(confirmText)) return
    // New rows live only client-side — no API call needed.
    if (target._isNew) {
      setAccounts((prev) => prev.filter((a) => a.id !== id))
      return
    }
    try {
      const res = await r2.deleteAccount(id)
      if (!res.ok) throw new Error(res.error || 'delete failed')
      setAccounts((prev) => prev.filter((a) => a.id !== id))
    } catch (e) {
      setSaveError((e as Error).message)
    }
  }

  async function saveAll() {
    setStatus('saving'); setSaveError(null)
    // Build the input list in current array order — backend renumbers
    // priorities to match. Strip masked placeholders (treat the all-bullet
    // string as "no change"); rows the operator typed in pass through.
    const payload: R2AccountInput[] = accounts.map((a) => {
      const masked = a.secret_access_key && /^●+$/.test(a.secret_access_key)
      return {
        id: a._isNew ? undefined : a.id,
        label: a.label || a.bucket_name || 'r2',
        account_id: a.account_id.trim(),
        access_key_id: a.access_key_id.trim(),
        // Empty secret means "keep what's on disk" per backend contract.
        secret_access_key: a._secretDirty && !masked ? a.secret_access_key : '',
        bucket_name: a.bucket_name.trim(),
        max_gb: Number.isFinite(a.max_gb) && a.max_gb > 0 ? a.max_gb : 9.5,
      }
    })
    try {
      const c = await r2.saveAccounts(payload)
      // After save, drop all editing flags + secret-dirty bits and pull the
      // canonical server state.
      setEditingRowIds(new Set())
      applyConfig(c, /* preserveDrafts */ false)
      setStatus('saved')
      setTimeout(() => setStatus('idle'), 2000)
    } catch (e) {
      setStatus('error')
      setSaveError((e as Error).message)
    }
  }

  async function setupCors(id: string) {
    setCorsBusy(id); setCorsMsg(null)
    try {
      const res = await r2.setupCors(id)
      const ok = !!res.ok
      const msg = ok
        ? 'CORS installed'
        : (res.error || res.results?.find((r0) => !r0.ok)?.error || 'CORS install failed')
      setCorsMsg({ id, ok, text: msg })
      window.setTimeout(() => setCorsMsg(null), 3000)
    } catch (e) {
      setCorsMsg({ id, ok: false, text: (e as Error).message })
    } finally {
      setCorsBusy(null)
    }
  }

  const dirty = useMemo(
    () => editingRowIds.size > 0 || accounts.some((a) => a._isNew || a._secretDirty),
    [accounts, editingRowIds],
  )

  return (
    <section className="card-block card-block--tight">
      <div className="card-block__head card-block__head--row">
        <div>
          <h2>Cloudflare R2 storage</h2>
          <p className="card-block__lede card-block__lede--short">
            Multi-account spillover: uploads land in the lowest-priority account with headroom.
            When account #1 crosses 95% of its cap, new uploads automatically flow to #2, then #3.
            {allFull && (
              <span
                className="status-pill status-pill--warn"
                style={{ fontSize: '0.75rem', marginLeft: '0.5rem' }}
                title="Every configured account is at or past 95%. Uploads still go through but may push a bucket past its soft cap — prune somewhere or add a fresh account."
              >
                ● all near cap
              </span>
            )}
          </p>
        </div>
      </div>

      {accounts.length === 0 ? (
        <p className="settings-hint">
          No R2 accounts configured yet. Add one below — uploads stay disabled until at least one
          row is filled in.
        </p>
      ) : null}

      <div style={{ display: 'flex', flexDirection: 'column', gap: '.75rem', marginTop: '.5rem' }}>
        {accounts.map((a, idx) => (
          <AccountCard
            key={a.id}
            account={a}
            index={idx}
            total={accounts.length}
            isPrimary={a.id === primaryId}
            onChange={(patch) => updateRow(a.id, patch)}
            onMoveUp={() => moveAccount(a.id, -1)}
            onMoveDown={() => moveAccount(a.id, 1)}
            onDelete={() => void deleteRow(a.id)}
            onSetupCors={() => void setupCors(a.id)}
            corsBusy={corsBusy === a.id}
            corsMsg={corsMsg?.id === a.id ? corsMsg : null}
            onMarkSecretDirty={() => updateRow(a.id, { _secretDirty: true })}
          />
        ))}
      </div>

      <div className="settings-btn-row" style={{ marginTop: '.85rem' }}>
        <button type="button" className="btn-glass" onClick={addAccount}>
          + Add account
        </button>
        <button
          type="button"
          className="btn-primary"
          onClick={() => void saveAll()}
          disabled={status === 'saving' || !dirty}
          title={dirty ? 'Persist every account to config.json' : 'No unsaved changes'}
        >
          {status === 'saving' ? 'Saving…'
            : status === 'saved' ? 'Saved ✓'
            : status === 'error' ? 'Error — retry'
            : 'Save all'}
        </button>
      </div>

      {saveError && (
        <p className="settings-hint" style={{ color: 'var(--danger)' }}>
          {saveError}
        </p>
      )}

      <p className="settings-hint">
        Create an R2 API token per account at Cloudflare Dashboard → R2 → Manage API tokens. Grant
        Object Read &amp; Write on each bucket. Account ID is on the R2 overview page (right sidebar).
        Lower priority numbers absorb uploads first.
      </p>
    </section>
  )
}

function AccountCard({
  account, index, total, isPrimary,
  onChange, onMoveUp, onMoveDown, onDelete, onSetupCors,
  corsBusy, corsMsg, onMarkSecretDirty,
}: {
  account: EditableAccount
  index: number
  total: number
  isPrimary: boolean
  onChange: (patch: Partial<EditableAccount>) => void
  onMoveUp: () => void
  onMoveDown: () => void
  onDelete: () => void
  onSetupCors: () => void
  corsBusy: boolean
  corsMsg: { ok: boolean; text: string } | null
  onMarkSecretDirty: () => void
}) {
  const a = account
  const pillClass = STATE_PILL_CLASS[a.state] || STATE_PILL_CLASS.unknown
  const pillLabel = STATE_LABELS[a.state] || STATE_LABELS.unknown
  const pillTitle = a.last_error
    ? `R2 health: ${pillLabel} — ${a.last_error}`
    : `R2 health: ${pillLabel}`
  return (
    <div
      className="card-block"
      style={{
        // Inline override so we can give the primary card a subtle accent
        // outline without inventing a new CSS class for the rewrite.
        outline: isPrimary ? '1px solid var(--accent, #6cc6ff)' : 'none',
        padding: '.75rem .9rem',
      }}
    >
      <div className="card-block__head card-block__head--row" style={{ marginBottom: '.45rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '.5rem', flexWrap: 'wrap' }}>
          <strong>
            #{index + 1}{' '}
            {a.label || a.bucket_name || (a._isNew ? '(new account)' : 'unlabeled')}
          </strong>
          {isPrimary && (
            <span className="pill pill--ok" style={{ fontSize: '0.7rem' }}>
              Primary
            </span>
          )}
          <span
            className={pillClass}
            style={{ fontSize: '0.72rem' }}
            title={pillTitle}
          >
            ● {pillLabel}
          </span>
          {a.configured && (
            <span className="pill pill--muted" style={{ fontSize: '0.7rem' }}>
              Configured
            </span>
          )}
        </div>
        <div style={{ display: 'flex', gap: '.25rem' }}>
          <button
            type="button"
            className="btn-glass btn-glass--xs"
            onClick={onMoveUp}
            disabled={index === 0}
            title="Higher priority (uploads land here sooner)"
          >
            ↑
          </button>
          <button
            type="button"
            className="btn-glass btn-glass--xs"
            onClick={onMoveDown}
            disabled={index === total - 1}
            title="Lower priority (uploads spill here after earlier accounts fill)"
          >
            ↓
          </button>
          <button
            type="button"
            className="btn-danger-outline"
            style={{ fontSize: '.7rem', padding: '.15rem .55rem' }}
            onClick={onDelete}
            title="Remove this account from the rotation"
          >
            Delete
          </button>
        </div>
      </div>

      {a.usage && a.usage.error == null && (
        <UsageBar usage={a.usage} />
      )}

      <div className="kv kv--form" style={{ marginTop: '.5rem' }}>
        {[
          { key: 'label', label: 'Label', placeholder: 'primary / overflow / backup', type: 'text' },
          { key: 'account_id', label: 'Account ID', placeholder: 'a1b2c3d4e5f6...', type: 'text' },
          { key: 'access_key_id', label: 'Access Key ID', placeholder: 'R2 API token key ID', type: 'text' },
          {
            key: 'secret_access_key',
            label: 'Secret Access Key',
            placeholder: a.configured && !a._secretDirty
              ? '(unchanged — leave blank to keep)'
              : 'R2 API token secret',
            type: 'password',
          },
          { key: 'bucket_name', label: 'Bucket Name', placeholder: 'my-targets-bucket', type: 'text' },
        ].map(({ key, label, placeholder, type }) => (
          <div key={key} className="kv__row">
            <label className="kv__label" htmlFor={`r2-${a.id}-${key}`}>{label}</label>
            <input
              id={`r2-${a.id}-${key}`}
              type={type}
              className="kv__input"
              placeholder={placeholder}
              value={(a as unknown as Record<string, string>)[key] ?? ''}
              onChange={(e) => {
                const v = e.target.value
                if (key === 'secret_access_key') onMarkSecretDirty()
                onChange({ [key]: v } as Partial<EditableAccount>)
              }}
            />
          </div>
        ))}
        <div className="kv__row">
          <label className="kv__label" htmlFor={`r2-${a.id}-maxgb`}>Soft cap (GB)</label>
          <input
            id={`r2-${a.id}-maxgb`}
            type="number"
            min={0.001}
            step={0.1}
            className="kv__input"
            value={a.max_gb}
            onChange={(e) => onChange({ max_gb: Number(e.target.value) || 9.5 })}
            title="Spillover triggers at 95% of this. Cloudflare R2 free tier is 10 GB; default 9.5 leaves a small safety margin."
          />
        </div>
      </div>

      <div className="settings-btn-row" style={{ marginTop: '.55rem' }}>
        <button
          type="button"
          className="btn-glass"
          onClick={onSetupCors}
          disabled={corsBusy || a.state !== 'connected'}
          title={
            a.state !== 'connected'
              ? 'Save and connect this account first (the bucket must be reachable)'
              : 'Install a CORS rule on this bucket so the Lists panel can upload directly'
          }
        >
          {corsBusy
            ? 'Installing CORS…'
            : corsMsg?.ok
              ? 'CORS installed ✓'
              : corsMsg && !corsMsg.ok
                ? 'CORS install failed — retry'
                : 'Allow browser uploads (CORS)'}
        </button>
      </div>

      {corsMsg && !corsMsg.ok && (
        <p className="settings-hint" style={{ color: 'var(--danger)' }}>
          CORS install error: {corsMsg.text}
        </p>
      )}

      {a.last_error && (a.state === 'unreachable' || a.state === 'misconfigured') && (
        <p className="settings-hint" style={{ color: 'var(--danger)' }}>
          R2 probe error: {a.last_error}
        </p>
      )}
    </div>
  )
}

function formatBytes(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 ** 2) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(1)} MB`
  return `${(b / 1024 ** 3).toFixed(2)} GB`
}

function UsageBar({ usage }: { usage: R2Usage }) {
  // Colour stops mirror fleet card severity: < 75% safe (green); 75-95 %
  // warn (orange); >= 95 % danger (red). The bar fills against
  // `counted_bytes / limit_bytes`; hits are excluded by policy.
  const pct = Math.min(100, usage.percent)
  const tone = usage.threshold_95_hit ? 'bad'
    : usage.threshold_75_hit ? 'warn'
    : 'ok'
  const barColour =
    tone === 'bad' ? 'var(--danger, #ff5a5a)' :
    tone === 'warn' ? '#ff8a3d' :
    'var(--accent, #6cc6ff)'
  const limitGb = (usage.limit_bytes / 1024 ** 3).toFixed(2)
  return (
    <div style={{ marginTop: '.25rem' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', fontSize: '.78rem' }}>
        <span className="muted" style={{ textTransform: 'uppercase', letterSpacing: '.06em' }}>
          R2 usage
        </span>
        <span className="mono" title="Hits are not counted toward the cap">
          {formatBytes(usage.counted_bytes)} / {formatBytes(usage.limit_bytes)}
          {' '}({pct.toFixed(1)}%)
        </span>
      </div>
      <div style={{
        marginTop: '.25rem',
        height: '8px',
        background: 'rgba(255,255,255,.08)',
        borderRadius: '4px',
        overflow: 'hidden',
      }}>
        <div style={{
          width: `${pct}%`,
          height: '100%',
          background: barColour,
          transition: 'width .4s ease',
        }} />
      </div>
      <div style={{ marginTop: '.35rem', display: 'flex', gap: '.65rem', flexWrap: 'wrap', fontSize: '.72rem' }} className="muted">
        <span>WARC {formatBytes(usage.bytes_by.warc)} · {usage.count_by.warc}</span>
        <span>Lists {formatBytes(usage.bytes_by.uploads)} · {usage.count_by.uploads}</span>
        <span>Hits {formatBytes(usage.bytes_by.hits)} · {usage.count_by.hits} (uncapped)</span>
        {usage.bytes_by.other > 0 && <span>Other {formatBytes(usage.bytes_by.other)} · {usage.count_by.other}</span>}
      </div>
      {usage.threshold_95_hit && (
        <p style={{ marginTop: '.4rem', color: 'var(--danger, #ff5a5a)', fontSize: '.78rem' }}>
          ⚠ R2 storage is at {pct.toFixed(1)}% of the {limitGb} GB cap. Uploads will spill to the
          next-priority account if one is configured.
        </p>
      )}
      {!usage.threshold_95_hit && usage.threshold_75_hit && (
        <p style={{ marginTop: '.4rem', color: '#ff8a3d', fontSize: '.78rem' }}>
          R2 storage is at {pct.toFixed(1)}% of the {limitGb} GB cap — consider pruning before it fills up.
        </p>
      )}
    </div>
  )
}
