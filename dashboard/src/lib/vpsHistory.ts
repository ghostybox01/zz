/** Rolling CPU samples for per-node sparklines (UI sim + live hooks later). */
export const CPU_HISTORY_MAX = 28

export function pushCpuSample(history: readonly number[] | undefined, cpu: number): number[] {
  const next = [...(history ?? []), Math.min(100, Math.max(0, Math.round(cpu)))]
  return next.slice(-CPU_HISTORY_MAX)
}
