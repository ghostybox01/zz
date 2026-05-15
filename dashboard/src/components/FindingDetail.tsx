import { useState, type ReactNode } from 'react'
import type { Finding } from '../types'
import { findingCredentialText } from '../lib/findingCredential'

type Props = {
  finding: Finding
  onBack: () => void
  onRecheck?: (f: Finding) => void
  onResend?: (f: Finding) => void
}

/* ─── Icons ──────────────────────────────────────────────────── */

function Icon({ d, size = 14 }: { d: string; size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
      <path d={d} />
    </svg>
  )
}
const IcoServer = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="6" rx="1.5"/><rect x="3" y="14" width="18" height="6" rx="1.5"/><circle cx="7" cy="7" r="1" fill="currentColor" stroke="none"/><circle cx="7" cy="17" r="1" fill="currentColor" stroke="none"/></svg>
const IcoLink = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
const IcoBug = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M12 2v4"/><path d="M5 8a7 7 0 0 1 14 0v6a7 7 0 0 1-14 0V8z"/><path d="M5 12H1m22 0h-4M5 16l-3 2m20-2l-3 2M5 8L2 6m20 0l-3 2"/></svg>
const IcoKey = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><circle cx="8" cy="15" r="4"/><path d="M10.85 12.15 19 4l3 3-2 2-2-2-3 3-2-2-2.15 2.15"/></svg>
const IcoBack = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M19 12H5M12 19l-7-7 7-7"/></svg>
const IcoShield = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
const IcoSend = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M22 2 11 13M22 2l-7 20-4-9-9-4 20-7z"/></svg>
const IcoCalendar = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 11h18"/></svg>
const IcoCheck = () => <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M20 6 9 17l-5-5"/></svg>
const IcoGrid = () => <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>
const IcoBell = () => <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0"/></svg>
const IcoInfo = () => <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>
const IcoList = () => <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/></svg>
const IcoWallet = () => <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M20 7H5a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h15a1 1 0 0 0 1-1V8a1 1 0 0 0-1-1Z"/><path d="M16 14h2"/><path d="M3 9V6a2 2 0 0 1 2-2h13"/></svg>

/* ─── Helpers ────────────────────────────────────────────────── */

function shortId(id: string): string {
  const numeric = id.replace(/\D/g, '')
  if (numeric.length >= 6) return `#${numeric.slice(-8)}`
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0
  return `#${(h % 99_999_999).toString().padStart(8, '0')}`
}

function vulnTag(rule: string): 'CRED' | 'PATH' | 'LIB' | 'GPL' {
  const u = rule.toUpperCase()
  if (u.includes('STRIPE') || u.includes('GPL')) return 'GPL'
  if (u.includes('SMTP') || u.includes('WEBHOOK') || u.includes('BACKUP') || u.includes('ACTUATOR')) return 'PATH'
  if (u.includes('KEY') || u.includes('TOKEN') || u.includes('SECRET') || u.includes('CRED')) return 'CRED'
  return 'LIB'
}

function fmtMoney(cents: number | undefined, currency = 'USD'): string {
  if (cents === undefined || !Number.isFinite(cents)) return '—'
  const amount = cents / 100
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency: currency.toUpperCase() }).format(amount)
  } catch {
    return `${amount.toFixed(2)} ${currency.toUpperCase()}`
  }
}

function copy(text: string, onDone?: () => void) {
  navigator.clipboard?.writeText(text).then(onDone).catch(() => undefined)
}

function CopyCell({
  label,
  value,
  mono,
  big,
  green,
}: {
  label: string
  value: string
  mono?: boolean
  big?: boolean
  green?: boolean
}) {
  const [done, setDone] = useState(false)
  const cls = [
    'fdv-cell__value',
    mono ? 'fdv-cell__value--mono' : '',
    big ? 'fdv-cell__value--big' : '',
    green ? 'fdv-cell__value--green' : '',
  ].filter(Boolean).join(' ')
  return (
    <div
      className="fdv-cell"
      onClick={() => copy(value, () => { setDone(true); window.setTimeout(() => setDone(false), 1100) })}
      title="Click to copy"
    >
      <span className="fdv-cell__label">{label}</span>
      <span className={cls}>{value}</span>
      {done ? <span className="fdv-cell__copied">copied</span> : <span className="fdv-cell__copy-hint">click to copy</span>}
    </div>
  )
}

/* ─── Building blocks ────────────────────────────────────────── */

function FieldBox({
  icon,
  label,
  children,
  copyValue,
  variant,
  tone,
}: {
  icon: ReactNode
  label: string
  children: ReactNode
  copyValue?: string
  variant?: 'plain' | 'mono' | 'amber' | 'blue' | 'purple' | 'green'
  tone?: 'plain' | 'amber' | 'blue' | 'purple' | 'green'
}) {
  const [done, setDone] = useState(false)
  const v = variant ?? 'plain'
  const t = tone ?? 'plain'

  return (
    <div className={`fdv-field fdv-field--tone-${t}`} onClick={copyValue ? () => copy(copyValue, () => { setDone(true); window.setTimeout(() => setDone(false), 1100) }) : undefined} style={copyValue ? { cursor: 'pointer' } : undefined}>
      <span className="fdv-field__label">
        <span className="fdv-field__ico">{icon}</span>
        {label}
      </span>
      <div className={`fdv-field__value fdv-field__value--${v}`}>
        {children}
        {done && <span className="fdv-field__copied">copied</span>}
      </div>
    </div>
  )
}

function StatusPill({
  tone,
  icon,
  children,
}: { tone: 'green' | 'purple' | 'gold' | 'neutral'; icon: ReactNode; children: ReactNode }) {
  return (
    <span className={`fdv-pill fdv-pill--${tone}`}>
      <span className="fdv-pill__ico">{icon}</span>
      {children}
    </span>
  )
}

/* ─── Stripe Account panel ──────────────────────────────────── */

function StripePanel({ stripe, accountId }: { stripe: NonNullable<Finding['details']>['stripe']; accountId?: string }) {
  if (!stripe) return null
  return (
    <div className="fdv-section">
      <h3 className="fdv-section__title">Stripe Account</h3>

      <div className="fdv-card">
        <div className="fdv-card__head">
          <span className="fdv-card__brand">
            <span className="fdv-card__brand-mark">S</span>
            <strong>Main Account</strong>
          </span>
          {stripe.chargesEnabled && (
            <span className="fdv-pill fdv-pill--green fdv-pill--solid">CHARGES ENABLED</span>
          )}
        </div>
        <div className="fdv-card__grid">
          <CopyCell label="EMAIL" value={stripe.email ?? '—'} />
          <CopyCell label="COUNTRY" value={stripe.country ?? '—'} />
          <CopyCell label="CURRENCY" value={stripe.currency?.toUpperCase() ?? '—'} />
          <CopyCell label="PAYOUTS" value={stripe.payoutsEnabled ? 'Enabled' : 'Disabled'} green={stripe.payoutsEnabled} />
        </div>
      </div>

      <div className="fdv-card">
        <div className="fdv-card__head">
          <span className="fdv-card__brand">
            <span className="fdv-card__brand-mark fdv-card__brand-mark--green"><IcoWallet /></span>
            <strong>Balance</strong>
          </span>
          {stripe.balance !== undefined && (
            <span className="fdv-pill fdv-pill--green fdv-pill--solid">{fmtMoney(stripe.balance, stripe.currency)}</span>
          )}
        </div>
        <div className="fdv-card__grid">
          <CopyCell
            label={(stripe.currency ?? 'USD').toUpperCase()}
            value={((stripe.balance ?? 0) / 100).toFixed(2)}
            big
            green
          />
          <CopyCell label="PENDING" value={((stripe.pendingBalance ?? 0) / 100).toFixed(2)} big />
        </div>
      </div>

      <div className="fdv-card__grid fdv-card__grid--solo">
        <CopyCell label="ACCOUNT ID" value={accountId ?? '—'} mono />
        <CopyCell label="CUSTOM ACCOUNTS" value="Cannot Create" />
      </div>
    </div>
  )
}

/* ─── Generic Metadata panel (used when no provider-specific) ── */

function MetadataPanel({ finding }: { finding: Finding }) {
  const d = finding.details
  const rows: Array<{ icon: ReactNode; label: string; value: ReactNode; mono?: boolean; tone?: 'amber' | 'blue' | 'plain' }> = []

  // Raw credential is shown in the main "Discovered credential" field — skip duplicate KEY row here.
  if (d?.stripe?.email) rows.push({ icon: <IcoInfo />, label: 'EMAIL', value: d.stripe.email, tone: 'blue' })
  if (finding.url) rows.push({ icon: <IcoLink />, label: 'URL', value: finding.url, mono: true, tone: 'blue' })

  // Mail-provider style metadata
  if (d?.senderDomains && d.senderDomains.length > 0) {
    rows.push({
      icon: <IcoList />,
      label: 'SENDERS',
      value: <DomainList values={d.senderDomains} />,
      tone: 'plain',
    })
  }
  if (d?.monthlyCredits != null) {
    rows.push({ icon: <IcoInfo />, label: 'PLAN', value: `${d.monthlyCredits.toLocaleString()} credits / mo`, tone: 'blue' })
  }
  if (d?.sentLast30d != null) {
    rows.push({ icon: <IcoInfo />, label: 'SENT (30D)', value: d.sentLast30d.toLocaleString(), tone: 'blue' })
  }

  // GitHub
  if (d?.github?.user) rows.push({ icon: <IcoInfo />, label: 'USER', value: d.github.user, tone: 'blue' })
  if (d?.github?.scopes && d.github.scopes.length > 0) {
    rows.push({ icon: <IcoList />, label: 'SCOPES', value: <DomainList values={d.github.scopes} />, tone: 'plain' })
  }
  if (d?.github?.rateLimit) rows.push({ icon: <IcoInfo />, label: 'RATE LIMIT', value: `${d.github.rateLimit.remaining}/${d.github.rateLimit.limit}`, tone: 'blue' })
  if (d?.github?.repos !== undefined) rows.push({ icon: <IcoInfo />, label: 'REPOS', value: `${d.github.repos} (${d.github.privateRepos ?? 0} private)`, tone: 'blue' })

  // SES
  if (d?.sesQuota?.max24h !== undefined) rows.push({ icon: <IcoInfo />, label: 'DAILY QUOTA', value: d.sesQuota.max24h.toLocaleString(), tone: 'blue' })
  if (d?.sesQuota?.sent24h !== undefined) rows.push({ icon: <IcoInfo />, label: 'SENT (24H)', value: d.sesQuota.sent24h.toLocaleString(), tone: 'blue' })
  if (d?.sesQuota?.ratePerSecond !== undefined) rows.push({ icon: <IcoInfo />, label: 'RATE/SEC', value: d.sesQuota.ratePerSecond, tone: 'blue' })
  if (d?.sesQuota?.verifiedDomains) rows.push({ icon: <IcoList />, label: 'VERIFIED DOMAINS', value: <DomainList values={d.sesQuota.verifiedDomains} />, tone: 'plain' })

  // AWS
  if (d?.awsRegion) rows.push({ icon: <IcoInfo />, label: 'REGION', value: d.awsRegion, mono: true, tone: 'blue' })
  if (d?.awsServices) rows.push({ icon: <IcoList />, label: 'SERVICES', value: <DomainList values={d.awsServices} />, tone: 'plain' })

  // Twilio
  if (d?.twilio?.sid) rows.push({ icon: <IcoInfo />, label: 'ACCOUNT SID', value: d.twilio.sid, mono: true, tone: 'blue' })
  if (d?.twilio?.balance !== undefined) rows.push({ icon: <IcoInfo />, label: 'BALANCE', value: `${d.twilio.balance.toFixed(2)} ${d.twilio.currency ?? ''}`, tone: 'blue' })
  if (d?.twilio?.numbers !== undefined) rows.push({ icon: <IcoInfo />, label: 'NUMBERS', value: d.twilio.numbers, tone: 'blue' })

  // SMTP
  if (d?.smtp?.host) rows.push({ icon: <IcoInfo />, label: 'HOST', value: `${d.smtp.host}${d.smtp.port ? `:${d.smtp.port}` : ''}`, mono: true, tone: 'blue' })
  if (d?.smtp?.user) rows.push({ icon: <IcoInfo />, label: 'USER', value: d.smtp.user, mono: true, tone: 'blue' })

  // Models
  if (d?.modelsAvailable !== undefined) rows.push({ icon: <IcoInfo />, label: 'MODELS', value: `${d.modelsAvailable} available`, tone: 'blue' })
  if (d?.modelExamples) rows.push({ icon: <IcoList />, label: 'EXAMPLES', value: <DomainList values={d.modelExamples} />, tone: 'plain' })

  // Extra freeform
  if (d?.extra) {
    for (const ex of d.extra) {
      rows.push({ icon: <IcoInfo />, label: ex.key.toUpperCase(), value: ex.value, tone: 'blue' })
    }
  }

  if (rows.length === 0) {
    return (
      <div className="fdv-section">
        <h3 className="fdv-section__title">Metadata</h3>
        <p className="fdv-section__empty">No additional metadata captured.</p>
      </div>
    )
  }

  return (
    <div className="fdv-section">
      <h3 className="fdv-section__title">Metadata</h3>
      {rows.map((r) => (
        <FieldBox
          key={r.label}
          icon={r.icon}
          label={r.label}
          variant={r.mono ? 'mono' : 'plain'}
          tone={r.tone}
        >
          {r.value}
        </FieldBox>
      ))}
    </div>
  )
}

function DomainList({ values }: { values: ReadonlyArray<string> }) {
  return (
    <div className="fdv-domains">
      {values.map((v) => (
        <span key={v} className="fdv-domain">{v}</span>
      ))}
    </div>
  )
}

/* ─── Main view ─────────────────────────────────────────────── */

export function FindingDetail({ finding, onBack, onRecheck, onResend }: Props) {
  const d = finding.details
  const vuln = vulnTag(finding.ruleLabel)
  const validated = d?.validated !== false
  // SUBSCRIBED = confirmed paid / recurring plan attached to the credential.
  // Triggers: SendGrid/Brevo/Mailgun monthly credit plan, Stripe live mode with charges, SES out of sandbox,
  // Twilio active account with positive balance.
  const subscribed =
    (d?.monthlyCredits !== undefined && d.monthlyCredits > 0) ||
    (d?.stripe?.livemode === true && d.stripe.chargesEnabled === true) ||
    (d?.sesQuota?.sandbox === false) ||
    (d?.twilio?.status === 'active' && (d.twilio.balance ?? 0) > 0)
  const hasRichDetails = !!(d?.stripe || d?.sesQuota || d?.github || d?.twilio || d?.senderDomains || d?.smtp || d?.modelsAvailable !== undefined)

  return (
    <section className="fdv">
      <header className="fdv__head">
        <div className="fdv__title-block">
          <h1 className="fdv__title">
            <span className="fdv__provider">{finding.provider.toUpperCase()}</span>
            <span className="fdv__id" title={finding.id}>{shortId(finding.id)}</span>
          </h1>

          <div className="fdv__pills">
            {validated ? (
              <StatusPill tone="green" icon={<IcoCheck />}>VALID</StatusPill>
            ) : (
              <StatusPill tone="gold" icon={<IcoInfo />}>PENDING</StatusPill>
            )}
            {hasRichDetails && (
              <StatusPill tone="purple" icon={<IcoKey />}>VALID CREDENTIALS</StatusPill>
            )}
            <StatusPill tone="neutral" icon={<IcoGrid />}>ADDON</StatusPill>
            {subscribed && (
              <StatusPill tone="gold" icon={<IcoBell />}>SUBSCRIBED</StatusPill>
            )}
            <StatusPill tone="purple" icon={<IcoKey />}>{vuln}</StatusPill>
          </div>

          <p className="fdv__date">
            <IcoCalendar />
            <span>{new Date(finding.at).toLocaleString()}</span>
          </p>
        </div>

        <div className="fdv__actions">
          <button type="button" className="fdv-btn" onClick={onBack}>
            <IcoBack /> Back
          </button>
          <button
            type="button"
            className="fdv-btn"
            onClick={() => onRecheck?.(finding)}
            disabled={!onRecheck}
            title={onRecheck ? 'Re-validate this credential' : 'Wire scanner API to enable'}
          >
            <IcoShield /> Recheck
          </button>
          <button
            type="button"
            className="fdv-btn fdv-btn--gold"
            onClick={() => onResend?.(finding)}
            disabled={!onResend}
            title={onResend ? 'Resend Telegram notification' : 'Wire scanner API to enable'}
          >
            <IcoSend /> Resend
          </button>
        </div>
      </header>

      <div className="fdv__grid">
        <div className="fdv-section">
          <h3 className="fdv-section__title">Information</h3>

          <FieldBox icon={<IcoServer />} label="SERVICE">
            <span>{finding.provider.toLowerCase()}</span>
          </FieldBox>

          <FieldBox icon={<IcoLink />} label="URL" variant="mono" copyValue={finding.url ?? finding.hostname}>
            <span className="fdv-url">{finding.url ?? finding.hostname}</span>
          </FieldBox>

          <FieldBox icon={<IcoBug />} label="VULNERABILITY">
            <span className="fdv-vuln-tag">
              <span className="fdv-vuln-tag__ico"><Icon d="M12 2 4 6v6c0 5 3.5 9 8 10 4.5-1 8-5 8-10V6l-8-4z" /></span>
              {vuln}
            </span>
          </FieldBox>

          <FieldBox
            icon={<IcoKey />}
            label="Discovered credential"
            variant="amber"
            copyValue={findingCredentialText(finding)}
          >
            <code className="fdv-secret">{findingCredentialText(finding)}</code>
          </FieldBox>
        </div>

        {d?.stripe ? (
          <StripePanel
            stripe={d.stripe}
            accountId={d.extra?.find((e) => e.key.toLowerCase().includes('account'))?.value}
          />
        ) : (
          <MetadataPanel finding={finding} />
        )}
      </div>
    </section>
  )
}
