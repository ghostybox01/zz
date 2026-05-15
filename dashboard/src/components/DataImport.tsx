import { useCallback, useRef, useState, type ChangeEvent } from 'react'
import { parseRunSnapshot, type RunSnapshot } from '../types'

type Props = {
  onImport: (run: RunSnapshot) => void
}

export function DataImport({ onImport }: Props) {
  const [error, setError] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const applyJson = useCallback(
    (text: string) => {
      setError(null)
      try {
        const data = JSON.parse(text) as unknown
        const run = parseRunSnapshot(data)
        if (!run) {
          setError('Expected exported run-metrics schema (domains, workers, WARC totals, snapshots[]).')
          return
        }
        onImport(run)
      } catch {
        setError('Invalid JSON.')
      }
    },
    [onImport],
  )

  const onFile = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      applyJson(String(reader.result ?? ''))
    }
    reader.readAsText(file, 'UTF-8')
    e.target.value = ''
  }

  return (
    <div className="data-import">
      <label className="data-import__label" htmlFor="snapshot-json-file">
        Choose file
      </label>
      <div className="data-import__row">
        <input
          ref={fileRef}
          id="snapshot-json-file"
          type="file"
          accept="application/json,.json"
          onChange={onFile}
          className="data-import__file"
        />
        <button type="button" className="btn-secondary" onClick={() => fileRef.current?.click()}>
          Browse JSON
        </button>
      </div>
      <textarea
        className="data-import__paste"
        spellCheck={false}
        placeholder="Paste run-metrics JSON, blur to apply."
        rows={4}
        onBlur={(ev) => {
          const v = ev.target.value.trim()
          if (!v) return
          applyJson(v)
          ev.target.value = ''
        }}
      />
      {error ? <p className="data-import__err">{error}</p> : null}
    </div>
  )
}
