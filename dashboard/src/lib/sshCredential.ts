import type { Finding } from '../types'
import type { VpsAuthType } from '../types'

export type SshCredential = {
  host: string
  port: number
  user: string
  secret: string
  authType: VpsAuthType
  /** Dedup key — normalized host (+ port when non-22). */
  fingerprint: string
  findingId: string
  sourceUrl?: string
}

const IPV4 =
  /\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b/

const PRIVATE_KEY_MARK = /-----BEGIN (?:OPENSSH |RSA |EC |DSA )?PRIVATE KEY-----/

function normHost(host: string): string {
  return host.trim().toLowerCase().replace(/^\[|\]$/g, '')
}

export function sshFingerprint(host: string, port = 22): string {
  const h = normHost(host)
  return port === 22 ? h : `${h}:${port}`
}

function isPrivateKeyMaterial(s: string): boolean {
  return PRIVATE_KEY_MARK.test(s)
}

function parseHostPort(raw: string): { host: string; port: number } | null {
  const t = raw.trim()
  if (!t) return null
  const bracket = t.match(/^\[([^\]]+)\]:(\d+)$/)
  if (bracket) return { host: bracket[1]!, port: Number(bracket[2]) }
  const colon = t.match(/^(.*?):(\d+)$/)
  if (colon && IPV4.test(colon[1]!)) return { host: colon[1]!, port: Number(colon[2]) }
  if (IPV4.test(t)) return { host: t, port: 22 }
  if (/^[a-z0-9.-]+\.[a-z]{2,}$/i.test(t)) return { host: t, port: 22 }
  return null
}

/** Scanner line: `host:user:secret` or `host:port:user:secret` (no source URL). */
export function parseSshScannerLine(line: string, findingId: string): SshCredential | null {
  const parts = line.split(':')
  if (parts.length < 3) return null

  let port = 22
  let host: string
  let user: string
  let secret: string

  if (parts.length >= 4 && /^\d+$/.test(parts[1]!)) {
    host = parts[0]!
    port = Number(parts[1])
    user = parts[2]!
    secret = parts.slice(3).join(':')
  } else {
    host = parts[0]!
    user = parts[1]!
    secret = parts.slice(2).join(':')
  }

  const hp = parseHostPort(host)
  if (!hp) return null
  if (!user || !secret) return null

  const authType: VpsAuthType = isPrivateKeyMaterial(secret) ? 'key' : 'password'
  return {
    host: hp.host,
    port: hp.port || port,
    user: user.trim(),
    secret: secret.trim(),
    authType,
    fingerprint: sshFingerprint(hp.host, hp.port || port),
    findingId,
    sourceUrl: undefined,
  }
}

/** `url:host:user:secret` from live scanner files. */
export function parseSshWithSourceUrl(
  url: string,
  parts: string[],
  findingId: string,
): SshCredential | null {
  const [hostRaw, user, ...rest] = parts
  if (!hostRaw || !user || rest.length === 0) return null
  const secret = rest.join(':')
  const hp = parseHostPort(hostRaw)
  if (!hp || !secret.trim()) return null
  const authType: VpsAuthType = isPrivateKeyMaterial(secret) ? 'key' : 'password'
  return {
    host: hp.host,
    port: hp.port,
    user: user.trim(),
    secret: secret.trim(),
    authType,
    fingerprint: sshFingerprint(hp.host, hp.port),
    findingId,
    sourceUrl: url || undefined,
  }
}

/** Pull SSH/VPS material from a finding row. */
export function extractSshCredential(finding: Finding): SshCredential | null {
  const raw = finding.details?.raw ?? finding.detail
  const provider = finding.provider.toLowerCase()

  if (provider === 'ssh' || provider === 'vps') {
    if (finding.url && raw.includes(':')) {
      const parts = raw.split(':')
      if (parts.length >= 3) {
        return parseSshWithSourceUrl(finding.url, parts, finding.id)
      }
    }
    const fromLine = parseSshScannerLine(raw, finding.id)
    if (fromLine) return fromLine
  }

  if (provider === 'private key') {
    if (!isPrivateKeyMaterial(raw)) return null
    const host =
      finding.details?.extra?.find((e) => e.key.toLowerCase() === 'ssh_host')?.value ??
      finding.details?.extra?.find((e) => e.key.toLowerCase() === 'host')?.value ??
      extractHostFromEnv(raw) ??
      extractIpFromText(raw) ??
      finding.hostname
    const user =
      finding.details?.extra?.find((e) => e.key.toLowerCase() === 'ssh_user')?.value ??
      finding.details?.extra?.find((e) => e.key.toLowerCase() === 'user')?.value ??
      'root'
    const hp = parseHostPort(host)
    if (!hp) return null
    return {
      host: hp.host,
      port: hp.port,
      user,
      secret: raw.trim(),
      authType: 'key',
      fingerprint: sshFingerprint(hp.host, hp.port),
      findingId: finding.id,
      sourceUrl: finding.url,
    }
  }

  // .env-style blobs surfaced as SMTP/Generic with SSH_* keys
  const envHost = /SSH_HOST[=:]\s*([^\s'";]+)/i.exec(raw)?.[1]
  const envUser = /SSH_USER(?:NAME)?[=:]\s*([^\s'";]+)/i.exec(raw)?.[1] ?? 'root'
  const envPass =
    /SSH_PASS(?:WORD)?[=:]\s*([^\s'";]+)/i.exec(raw)?.[1] ??
    /SSH_PRIVATE_KEY[=:]\s*["']?([^"']+)/i.exec(raw)?.[1]
  if (envHost && envPass) {
    const hp = parseHostPort(envHost)
    if (!hp) return null
    const authType: VpsAuthType = isPrivateKeyMaterial(envPass) ? 'key' : 'password'
    return {
      host: hp.host,
      port: hp.port,
      user: envUser,
      secret: envPass,
      authType,
      fingerprint: sshFingerprint(hp.host, hp.port),
      findingId: finding.id,
      sourceUrl: finding.url,
    }
  }

  // root@ip:password in detail
  const atPass = /(?:^|\s)([a-z_][\w-]*)@([0-9.]+):(\S+)/i.exec(raw)
  if (atPass) {
    const hp = parseHostPort(atPass[2]!)
    if (!hp) return null
    return {
      host: hp.host,
      port: hp.port,
      user: atPass[1]!,
      secret: atPass[3]!,
      authType: 'password',
      fingerprint: sshFingerprint(hp.host, hp.port),
      findingId: finding.id,
      sourceUrl: finding.url,
    }
  }

  return null
}

function extractHostFromEnv(text: string): string | null {
  return /SSH_HOST[=:]\s*([^\s'";]+)/i.exec(text)?.[1] ?? null
}

function extractIpFromText(text: string): string | null {
  return IPV4.exec(text)?.[0] ?? null
}
