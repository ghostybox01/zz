import type { Finding } from '../types'

/** Bottom-left hit toast lanes — one colour each. */
export type HitToastCategory = 'ssh-vps' | 'smtp' | 'ai' | 'brevo'

export const HIT_TOAST_LABEL: Record<HitToastCategory, string> = {
  'ssh-vps': 'SSH/VPS',
  smtp: 'SMTP creds',
  ai: 'AI creds',
  brevo: 'Brevo creds',
}

const SMTP_PROVIDERS = new Set(['smtp', 'sendgrid', 'mailgun', 'postmark', 'twilio'])
const AI_PROVIDERS = new Set(['openai', 'anthropic'])

export function hitToastCategory(finding: Finding): HitToastCategory {
  const provider = finding.provider.toLowerCase()
  const rule = finding.ruleLabel.toLowerCase()

  if (provider === 'brevo') return 'brevo'

  if (
    AI_PROVIDERS.has(provider) ||
    rule.includes('openai') ||
    rule.includes('anthropic') ||
    rule.includes('claude') ||
    rule.includes('gpt')
  ) {
    return 'ai'
  }

  if (
    provider === 'ssh' ||
    provider === 'fleet' ||
    rule.includes('ssh') ||
    rule.includes('vps') ||
    rule.includes('root / deploy')
  ) {
    return 'ssh-vps'
  }

  if (
    SMTP_PROVIDERS.has(provider) ||
    (provider === 'aws' && (rule.includes('ses') || rule.includes('smtp'))) ||
    rule.includes('smtp') ||
    rule.includes('sendgrid') ||
    rule.includes('mailgun') ||
    rule.includes('webhook')
  ) {
    return 'smtp'
  }

  // Stripe, GitHub, backup blobs, etc. — bucket by closest lane.
  if (provider === 'github' || provider === 'stripe' || provider === 'generic') return 'ai'

  return 'smtp'
}

/** Returns a popup lane for critical/high hits; other severities stay in the ledger only. */
export function categoryForFinding(finding: Finding): HitToastCategory | null {
  if (finding.severity !== 'critical' && finding.severity !== 'high') return null
  return hitToastCategory(finding)
}
