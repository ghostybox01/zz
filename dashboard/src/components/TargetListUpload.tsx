import { MAX_PREVIEW_LINES } from '../lib/targetList'

type Props = {
  lines: number
  fileLabel: string | null
  onFile: (file: File | null) => void
}

export function TargetListUpload({ lines, fileLabel, onFile }: Props) {
  return (
    <section className="card-block">
      <div className="card-block__head">
        <h2>Targets</h2>
        <p className="card-block__lede card-block__lede--short">
          Plain <code className="inline-code">.txt</code> — counted locally (cap {MAX_PREVIEW_LINES.toLocaleString()} lines /
          12MB slice).
        </p>
      </div>
      <label className="upload-field">
        <input
          type="file"
          accept=".txt,text/plain"
          className="upload-field__native"
          onChange={(ev) => onFile(ev.target.files?.[0] ?? null)}
        />
        <span className="upload-field__ui">
          {fileLabel ? fileLabel : 'Drop or browse .txt hosts'}
        </span>
      </label>
      <div className="upload-meta">
        <span className="pill pill--muted">
          {lines > 0 ? `${lines.toLocaleString()} queued lines` : 'No targets loaded'}
        </span>
      </div>
    </section>
  )
}
