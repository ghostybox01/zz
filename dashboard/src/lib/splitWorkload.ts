export function allocateChunks(total: number, buckets: number): number[] {
  if (buckets <= 0) return []
  const base = Math.floor(total / buckets)
  const remainder = total % buckets
  return Array.from({ length: buckets }, (_, i) => base + (i < remainder ? 1 : 0))
}
