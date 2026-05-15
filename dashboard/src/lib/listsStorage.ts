import type { TargetList } from '../types'

const KEY = 'ravenx.dashboard.lists.v1'
const MAX_PREVIEW = 6

export function loadLists(): TargetList[] {
  if (typeof localStorage === 'undefined') return []
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as TargetList[]
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

export function saveLists(lists: ReadonlyArray<TargetList>): void {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(KEY, JSON.stringify(lists))
  } catch {
    /* quota — ignore; user will lose persistence but session works */
  }
}

/** Fast non-crypto hash. Sufficient for dedup of UTF-8 list bodies. */
export function hashContent(text: string): string {
  let h1 = 0x811c9dc5
  let h2 = 0xdeadbeef
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i)
    h1 = Math.imul(h1 ^ c, 2654435761)
    h2 = Math.imul(h2 ^ c, 1597334677)
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507)
  h1 ^= Math.imul(h2 ^ (h2 >>> 13), 3266489909)
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507)
  h2 ^= Math.imul(h1 ^ (h1 >>> 13), 3266489909)
  const hex = (n: number) => (n >>> 0).toString(16).padStart(8, '0')
  return `${hex(h1)}${hex(h2)}-${text.length.toString(16)}`
}

export async function readListFile(file: File): Promise<{
  body: string
  lineCount: number
  preview: string[]
  hash: string
}> {
  const text = await file.text()
  const lines: string[] = []
  const preview: string[] = []
  let count = 0
  for (const raw of text.split(/\r?\n/)) {
    const t = raw.trim()
    if (!t) continue
    count++
    if (lines.length < 4096) lines.push(t)
    if (preview.length < MAX_PREVIEW) preview.push(t)
  }
  return {
    body: lines.join('\n'),
    lineCount: count,
    preview,
    hash: hashContent(text),
  }
}

export function makeListId(): string {
  return `list-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`
}
