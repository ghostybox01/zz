import type { TargetList } from '../types'
import { getListBody } from '../lib/listBodyCache'

type Props = {
  list: TargetList
  onDelete: (id: string) => void
}

function fmtDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString()
  } catch {
    return '—'
  }
}

const STATUS_BADGE: Record<TargetList['status'], { label: string; tone: string }> = {
  idle:      { label: 'IDLE',      tone: 'muted' },
  queued:    { label: 'QUEUED',    tone: 'accent' },
  deployed:  { label: 'DEPLOYED',  tone: 'accent' },
  completed: { label: 'COMPLETED', tone: 'gold' },
  failed:    { label: 'FAILED',    tone: 'danger' },
}

function downloadList(list: TargetList) {
  const body = getListBody(list.id)
  const blob = new Blob([body ?? list.preview.join('\n')], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = list.name
  a.click()
  URL.revokeObjectURL(url)
}

export function CrackerListTile({ list, onDelete }: Props) {
  const badge = STATUS_BADGE[list.status]

  return (
    <article className="cw-list-tile">
      <header className="cw-list-tile__head">
        <h4 className="cw-list-tile__name" title={list.name}>
          {list.name.replace(/\.txt$/i, '')}
        </h4>
        <span className={`cw-list-tile__badge cw-list-tile__badge--${badge.tone}`}>
          {badge.label}
        </span>
      </header>

      <dl className="cw-list-tile__meta">
        <div>
          <dt>URLs</dt>
          <dd>{list.lineCount.toLocaleString()}</dd>
        </div>
        <div>
          <dt>Type</dt>
          <dd>uploaded</dd>
        </div>
        <div>
          <dt>Created</dt>
          <dd>{fmtDate(list.uploadedAt)}</dd>
        </div>
      </dl>

      <footer className="cw-list-tile__foot">
        <button
          type="button"
          className="btn-glass btn-glass--xs"
          onClick={() => downloadList(list)}
        >
          Download
        </button>
        <button
          type="button"
          className="btn-glass btn-glass--xs btn-glass--danger"
          onClick={() => onDelete(list.id)}
        >
          Delete
        </button>
      </footer>
    </article>
  )
}
