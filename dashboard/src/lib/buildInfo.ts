/** Build-time constants stamped at `npm run build`. Vite replaces these
 *  via the `define` field in vite.config.ts. */

declare const __RECONX_BUILD_SHA__: string
declare const __RECONX_BUILD_AT__: string
declare const __RECONX_REPO__: string

export const BUILD_SHA = typeof __RECONX_BUILD_SHA__ !== 'undefined' ? __RECONX_BUILD_SHA__ : 'dev'
export const BUILD_AT  = typeof __RECONX_BUILD_AT__  !== 'undefined' ? __RECONX_BUILD_AT__  : 'dev'
export const REPO_SLUG = typeof __RECONX_REPO__       !== 'undefined' ? __RECONX_REPO__       : ''

/** Returns the latest commit SHA on the repo's default branch, or null if not configured. */
export async function fetchLatestUpstreamSha(): Promise<string | null> {
  if (!REPO_SLUG) return null
  const url = `https://api.github.com/repos/${REPO_SLUG}/commits/HEAD`
  const res = await fetch(url, { headers: { Accept: 'application/vnd.github+json' } })
  if (!res.ok) throw new Error(`GitHub API ${res.status}`)
  const j = (await res.json()) as { sha?: string }
  return j.sha ?? null
}
