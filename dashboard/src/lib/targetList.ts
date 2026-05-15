export const MAX_PREVIEW_LINES = 12_000

/** Count non-empty trimmed lines from the first slice of upload (12MB client cap). */
export function readTargetTxtFile(file: File): Promise<number> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const text = String(reader.result ?? '')
      const lines = text
        .split(/\r?\n/)
        .map((l) => l.trim())
        .filter(Boolean)
      resolve(Math.min(lines.length, MAX_PREVIEW_LINES))
    }
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file.slice(0, 12 * 1024 * 1024), 'UTF-8')
  })
}
