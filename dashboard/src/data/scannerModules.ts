/** Scanner toggles that map 1:1 onto live keys in raven/config.json.
 *  Each ID listed here is consumed by main.go at runtime — see app.py
 *  SCANNER_CONFIG_SCHEMA and the audit notes alongside CrackerWorkspace. */

import type { ReconScannerConfig } from '../lib/reconApi'

// ── Scan method tiles (13 methods, VulnerabilityScannersPanel v2) ──────────

export type ScanMethod = {
  id: string
  label: string
  hint: string
  /** Which config section + key gates this method. Null = always runs. */
  configKey: { section: keyof ReconScannerConfig; key: string } | null
  /** Icon name for the tile. */
  icon: 'path' | 'js' | 'jsext' | 'smtp' | 'aws' | 'git' | 'php' | 'docker' | 'spring' | 'backup' | 'api' | 'r2s' | 'lfi' | 'ssrf' | 'ssh' | 'nvca' | 'gpl' | 'lib'
}

export const SCAN_METHODS: readonly ScanMethod[] = [
  { id: 'path',   label: 'Path Scanner',  icon: 'path',   configKey: null,                                                                  hint: 'Always-on. Probes 60+ .env variants. Upload a custom paths.txt to extend.' },
  { id: 'js',     label: 'JS Scanner',    icon: 'js',     configKey: { section: 'scanning_features', key: 'js_scan' },                     hint: 'Fetches all linked JS files and scans for hardcoded credentials in bundles' },
  { id: 'jsext',  label: 'JS Extended',   icon: 'jsext',  configKey: { section: 'scanning_features', key: 'js_extended_scan' },             hint: 'Scans raw page HTML body for inline credentials — keys in <script> blocks and data attributes. Requires JS Scanner.' },
  { id: 'smtp',   label: 'SMTP Scanner',  icon: 'smtp',   configKey: { section: 'scanning_features', key: 'smtp_credentials_scan' },       hint: 'Extracts SMTP config from env/PHP/YAML; validates via AUTH PLAIN + LOGIN handshake' },
  { id: 'aws',    label: 'AWS Scanner',   icon: 'aws',    configKey: { section: 'scanning_features', key: 'aws_main_scan' },               hint: 'AKIA/ASIA key detection + SES/SNS/Fargate + Secrets Manager + SSM across 19 regions' },
  { id: 'php',    label: 'PHP Scanner',   icon: 'php',    configKey: { section: 'scanning_features', key: 'phpinfo_scan' },                hint: 'phpinfo() + wp-config.php + config.php + database.php — most-exposed PHP credential files' },
  { id: 'git',    label: 'Git Scanner',   icon: 'git',    configKey: { section: 'scanning_features', key: 'git_config_scan' },             hint: '/.git/config + /.git/HEAD — git remotes often embed OAuth tokens in clone URLs' },
  { id: 'docker', label: 'Docker Scanner',icon: 'docker', configKey: { section: 'scanning_features', key: 'docker_scan' },                 hint: 'docker-compose.yml + docker-compose.override.yml + Dockerfile ENV statements' },
  { id: 'spring', label: 'Config Scanner',icon: 'spring', configKey: { section: 'scanning_features', key: 'config_file_scan' },            hint: 'application.properties + appsettings.json + settings.py + database.yml + .npmrc' },
  { id: 'backup', label: 'Backup Scanner',icon: 'backup', configKey: { section: 'scanning_features', key: 'backup_file_scan' },            hint: '.bak / .old / .orig / .env~ copies — forgotten backups often publicly accessible' },
  { id: 'api',    label: 'API Validator', icon: 'api',    configKey: null,                                                                  hint: 'Always-on. Validates 20+ API keys in any page: SendGrid, Stripe, Mailgun, Mailjet…' },
  { id: 'r2s',    label: 'R2S Scanner',   icon: 'r2s',    configKey: { section: 'exploit_methods', key: 'react2shell' },                   hint: 'React/Next source-map exploitation — reads full JS bundle source including env vars' },
  { id: 'lfi',    label: 'LFI Probe',     icon: 'lfi',    configKey: { section: 'exploit_methods', key: 'lfi' },                           hint: 'Local file inclusion — reads /etc/passwd, .env and config files via vulnerable params' },
  { id: 'ssrf',   label: 'SSRF Probe',    icon: 'ssrf',   configKey: { section: 'exploit_methods', key: 'ssrf' },                          hint: 'SSRF — hits EC2 metadata (169.254.169.254) to grab IAM role credentials' },
  { id: 'ssh',    label: 'SSH/Keys',      icon: 'ssh',    configKey: { section: 'scanning_features', key: 'ssh_scan' },                      hint: 'Probe SSH paths, extract private keys and credentials' },
  { id: 'nvca',   label: 'NVCA Scanner',  icon: 'nvca',   configKey: { section: 'scanning_features', key: 'nvca_scan' },                     hint: 'Node/Vue/Next.js/Nuxt config API endpoints — nuxt.config.js, build manifests, environment files' },
  { id: 'gpl',    label: 'GPL Scanner',   icon: 'gpl',    configKey: { section: 'scanning_features', key: 'gpl_scan' },                      hint: 'GraphQL introspection and endpoint probing — /graphql, /api/graphql, /graphiql' },
  { id: 'lib',    label: 'LIB Scanner',   icon: 'lib',    configKey: { section: 'scanning_features', key: 'lib_scan' },                      hint: 'Package manifests and .npmrc auth tokens — package.json, composer.lock, Gemfile, requirements.txt' },
]

export type ScanModule = {
  id: keyof ReconScannerConfig['scanning_features']
  label: string
  hint: string
}

export const SCAN_MODULES: readonly ScanModule[] = [
  { id: 'aws_main_scan',          label: 'AWS Main Scan',     hint: 'JS/page extraction + AWS SES/SNS/Federation checks' },
  { id: 'smtp_credentials_scan',  label: 'SMTP Path Probes',  hint: 'Probes /.env, /phpinfo.php and ~70 common paths for SMTP creds' },
  { id: 'github_token_deep_scan', label: 'GitHub Deep Scan',  hint: 'Token → user/repo enumeration via TruffleHog/Gitleaks' },
]

export type ExploitModule = {
  id: keyof ReconScannerConfig['exploit_methods']
  label: string
  hint: string
}

export const EXPLOIT_MODULES: readonly ExploitModule[] = [
  { id: 'react2shell',      label: 'React2Shell',      hint: 'Source-map / chunk dump on React/Next apps' },
  { id: 'bypass_waf',       label: 'WAF Bypass',       hint: 'Header & encoding tricks against CloudFlare/Akamai' },
  { id: 'bypass_middleware',label: 'Middleware Bypass',hint: 'Path traversal around Express/NestJS middleware' },
  { id: 'lfi',              label: 'LFI',              hint: 'Local file inclusion (limited; 1 finding cap)' },
  { id: 'xxe',              label: 'XXE',              hint: 'XML external entity (1 finding cap)' },
  { id: 'ssrf',             label: 'SSRF',             hint: 'Metadata endpoint discovery (2 finding cap)' },
]

export type ProviderModule = {
  id: keyof ReconScannerConfig['api_validation'] | keyof ReconScannerConfig['features']
  section: 'api_validation' | 'features'
  label: string
  group: 'mail' | 'pay' | 'sms' | 'ai' | 'cloud' | 'git'
}

export const PROVIDER_MODULES: readonly ProviderModule[] = [
  // Mail / SMTP
  { id: 'sendgrid',      section: 'api_validation', label: 'SendGrid',    group: 'mail' },
  { id: 'mailgun',       section: 'api_validation', label: 'Mailgun',     group: 'mail' },
  { id: 'brevo',         section: 'features',       label: 'Brevo',       group: 'mail' },
  { id: 'xsmtp',         section: 'features',       label: 'XSMTP',       group: 'mail' },
  { id: 'mandrill',      section: 'features',       label: 'Mandrill',    group: 'mail' },
  { id: 'mailersend',    section: 'features',       label: 'MailerSend',  group: 'mail' },
  { id: 'new_mailgun',   section: 'features',       label: 'NewMailgun',  group: 'mail' },
  // Pay
  { id: 'stripe',        section: 'api_validation', label: 'Stripe',      group: 'pay' },
  // SMS
  { id: 'twilio',        section: 'api_validation', label: 'Twilio',      group: 'sms' },
  { id: 'nexmo',         section: 'api_validation', label: 'Nexmo',       group: 'sms' },
  { id: 'telnyx',        section: 'api_validation', label: 'Telnyx',      group: 'sms' },
  { id: 'messagebird',   section: 'api_validation', label: 'MessageBird', group: 'sms' },
  // AI
  { id: 'openai',        section: 'api_validation', label: 'OpenAI',      group: 'ai' },
  { id: 'anthropic',     section: 'api_validation', label: 'Anthropic',   group: 'ai' },
  // Cloud / Git
  { id: 'gcp_api_key',   section: 'api_validation', label: 'GCP Key',     group: 'cloud' },
  { id: 'github',        section: 'api_validation', label: 'GitHub',      group: 'git' },
]

export type AwsCheck = {
  id: keyof ReconScannerConfig['aws_checks']
  label: string
}

export const AWS_CHECKS: readonly AwsCheck[] = [
  { id: 'ses_quota_check',       label: 'SES Quota' },
  { id: 'sns_limit_check',       label: 'SNS Limits' },
  { id: 'fargate_limit_check',   label: 'Fargate Limits' },
  { id: 'federation_console_url',label: 'Federation Console' },
]

export const GROUP_LABEL: Record<ProviderModule['group'], string> = {
  mail: 'Mail / SMTP',
  pay: 'Payments',
  sms: 'SMS',
  ai: 'AI',
  cloud: 'Cloud',
  git: 'Git Hosts',
}

/** Helper: list of all provider IDs grouped for rendering. */
export function groupProviders(
  cfg: ReconScannerConfig,
): Record<ProviderModule['group'], Array<ProviderModule & { on: boolean }>> {
  const out = { mail: [], pay: [], sms: [], ai: [], cloud: [], git: [] } as Record<
    ProviderModule['group'],
    Array<ProviderModule & { on: boolean }>
  >
  for (const p of PROVIDER_MODULES) {
    const block = cfg[p.section] as Record<string, boolean>
    out[p.group].push({ ...p, on: !!block[p.id] })
  }
  return out
}

/* ── Backwards-compat shims for older imports ─────────────────────── */

export type ScannerSelection = Record<string, boolean>
export type PlatformSelection = { github: boolean; gitlab: boolean; bitbucket: boolean }
export type AddonSelection = Record<string, boolean>

export const WORKSPACE_ADDONS: readonly { id: string; label: string; icon: 'mail' | 'cloud' | 'sms' | 'pay' | 'key' }[] = [
  { id: 'ses', label: 'AWS SES', icon: 'mail' },
  { id: 'sendgrid', label: 'SendGrid', icon: 'mail' },
  { id: 'stripe', label: 'Stripe', icon: 'pay' },
  { id: 'twilio', label: 'Twilio', icon: 'sms' },
  { id: 's3', label: 'S3 Deep', icon: 'cloud' },
]

export function defaultScannerSelection(): ScannerSelection {
  return {}
}

export function defaultPlatformSelection(): PlatformSelection {
  return { github: true, gitlab: true, bitbucket: false }
}

export function defaultAddonSelection(): AddonSelection {
  return Object.fromEntries(WORKSPACE_ADDONS.map((a) => [a.id, true]))
}

export type PlatformId = 'github' | 'gitlab' | 'bitbucket'

export const GIT_PLATFORMS: readonly { id: PlatformId; label: string }[] = [
  { id: 'github', label: 'GitHub' },
  { id: 'gitlab', label: 'GitLab' },
  { id: 'bitbucket', label: 'Bitbucket' },
]
