import { useMemo, useRef, useState, type DragEvent } from 'react'
import type { TargetList, VpsNode } from '../types'
import { ListCard } from './ListCard'
import { makeListId, readListFile } from '../lib/listsStorage'
import { setListBody } from '../lib/listBodyCache'
import { vps as reconVps } from '../lib/reconApi'

type Props = {
  lists: readonly TargetList[]
  fleet: readonly VpsNode[]
  onUpload: (list: TargetList) => void
  onUpdate: (list: TargetList) => void
  onDelete: (listId: string) => void
  onDeploy: (listId: string) => void
}

type UploadError = { kind: 'duplicate' | 'empty' | 'too-large' | 'read'; message: string } | null

// Files above this threshold are streamed to the backend in chunks instead of loaded into the browser.
const CHUNK_THRESHOLD = 20 * 1024 * 1024   // 20 MiB
const CHUNK_SIZE      = 10 * 1024 * 1024   // 10 MiB per chunk — 20 updates for a 200 MB file

export function ListsPanel({ lists, fleet, onUpload, onUpdate, onDelete, onDeploy }: Props) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)
  const [error, setError] = useState<UploadError>(null)
  const [filter, setFilter] = useState<'all' | 'deployed' | 'idle' | 'completed'>('all')
  const [uploadProgress, setUploadProgress] = useState<number | null>(null)

  const filtered = useMemo(() => {
    switch (filter) {
      case 'deployed': return lists.filter((l) => l.status === 'deployed' || l.status === 'queued')
      case 'idle':     return lists.filter((l) => l.status === 'idle' || l.status === 'failed')
      case 'completed':return lists.filter((l) => l.status === 'completed')
      default:         return lists
    }
  }, [lists, filter])

  const counts = useMemo(() => {
    const tally = { all: lists.length, idle: 0, deployed: 0, completed: 0 }
    for (const l of lists) {
      if (l.status === 'idle' || l.status === 'failed') tally.idle++
      else if (l.status === 'deployed' || l.status === 'queued') tally.deployed++
      else if (l.status === 'completed') tally.completed++
    }
    return tally
  }, [lists])

  const totalLines = useMemo(() => lists.reduce((s, l) => s + l.lineCount, 0), [lists])
  const assignedFleet = useMemo(() => {
    const set = new Set<string>()
    for (const l of lists) for (const v of l.assignedVpsIds) set.add(v)
    return set.size
  }, [lists])

  async function ingest(file: File | null | undefined) {
    if (!file) return
    setError(null)
    if (file.size === 0) {
      setError({ kind: 'empty', message: 'File is empty.' })
      return
    }

    // Large file path — stream chunks to the backend, assemble there.
    if (file.size > CHUNK_THRESHOLD) {
      await ingestLarge(file)
      return
    }

    let parsed
    try {
      parsed = await readListFile(file)
    } catch (e) {
      setError({ kind: 'read', message: (e as Error).message })
      return
    }
    if (parsed.lineCount === 0) {
      setError({ kind: 'empty', message: 'No non-empty lines found.' })
      return
    }
    const dup = lists.find((l) => l.contentHash === parsed.hash)
    if (dup) {
      setError({ kind: 'duplicate', message: `Identical content already uploaded as "${dup.name}".` })
      return
    }
    const next: TargetList = {
      id: makeListId(),
      name: file.name,
      uploadedAt: new Date().toISOString(),
      lineCount: parsed.lineCount,
      contentHash: parsed.hash,
      preview: parsed.preview,
      assignedVpsIds: [],
      status: 'idle',
    }
    onUpload(next)
    setListBody(next.id, parsed.body)
  }

  async function ingestLarge(file: File) {
    const totalChunks = Math.ceil(file.size / CHUNK_SIZE)
    const uploadId = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 9)}`

    // Read preview from the first ~4 KB without loading the whole file.
    const previewText = await file.slice(0, 4096).text()
    const preview = previewText.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 6)

    setUploadProgress(0)
    try {
      for (let i = 0; i < totalChunks; i++) {
        const chunkStart = i * CHUNK_SIZE
        const content = await file.slice(chunkStart, chunkStart + CHUNK_SIZE).text()
        // XHR so we get byte-level progress within the chunk
        await uploadChunkXhr(uploadId, i, totalChunks, content, (bytesDone, bytesTotal) => {
          const chunkBase = (i / totalChunks) * 95
          const chunkSpan = (1 / totalChunks) * 95
          setUploadProgress(Math.round(chunkBase + (bytesDone / bytesTotal) * chunkSpan))
        })
      }

      setUploadProgress(98)
      const result = await reconVps.finalizeUpload(uploadId, totalChunks, file.name)
      setUploadProgress(100)

      const next: TargetList = {
        id: makeListId(),
        name: file.name,
        uploadedAt: new Date().toISOString(),
        lineCount: result.targets,
        contentHash: `server-${uploadId}`,
        preview,
        assignedVpsIds: [],
        status: 'idle',
        note: 'Uploaded via chunked transfer — list body is on the server as targets.txt',
      }
      onUpload(next)
    } catch (e) {
      setError({
        kind: 'too-large',
        message: `Chunked upload failed: ${(e as Error).message}. If no backend is connected, copy via SCP instead.`,
      })
    } finally {
      setUploadProgress(null)
    }
  }

  function uploadChunkXhr(
    uploadId: string,
    chunkIndex: number,
    totalChunks: number,
    content: string,
    onProgress: (done: number, total: number) => void,
  ): Promise<void> {
    return new Promise((resolve, reject) => {
      const body = JSON.stringify({ upload_id: uploadId, chunk_index: chunkIndex, total_chunks: totalChunks, content })
      const xhr = new XMLHttpRequest()
      xhr.open('POST', '/api/vps/upload-chunk')
      xhr.setRequestHeader('Content-Type', 'application/json')
      xhr.upload.onprogress = (e) => { if (e.lengthComputable) onProgress(e.loaded, e.total) }
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) resolve()
        else reject(new Error(`chunk ${chunkIndex} failed: HTTP ${xhr.status} — ${xhr.responseText.slice(0, 120)}`))
      }
      xhr.onerror = () => reject(new Error(`chunk ${chunkIndex} network error`))
      xhr.send(body)
    })
  }

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files?.[0]
    void ingest(file)
  }

  function toggleVps(listId: string, vpsId: string) {
    const list = lists.find((l) => l.id === listId)
    if (!list) return
    const has = list.assignedVpsIds.includes(vpsId)
    const next = has ? list.assignedVpsIds.filter((id) => id !== vpsId) : [...list.assignedVpsIds, vpsId]
    onUpdate({ ...list, assignedVpsIds: next })
  }

  function setStatus(listId: string, status: TargetList['status']) {
    const list = lists.find((l) => l.id === listId)
    if (!list) return
    onUpdate({ ...list, status })
  }

  function rename(listId: string, name: string) {
    const list = lists.find((l) => l.id === listId)
    if (!list) return
    onUpdate({ ...list, name })
  }

  return (
    <section className="lists-panel">
      <header className="lists-panel__head">
        <div>
          <h2 className="lists-panel__title">Target lists</h2>
          <p className="lists-panel__lede">
            Upload multiple lists, assign each to a subset of the fleet, then deploy. List 1 → ams-worker-01 + ams-worker-02, list 2 → sgp-lite-03, etc.
          </p>
        </div>
        <div className="lists-panel__summary">
          <span className="pill pill--muted">{lists.length} list{lists.length === 1 ? '' : 's'}</span>
          <span className="pill pill--muted">{totalLines.toLocaleString()} total lines</span>
          <span className="pill pill--ok">{assignedFleet} VPS targeted</span>
        </div>
      </header>

      <div
        className={`lists-upload${dragOver ? ' lists-upload--over' : ''}`}
        onDragOver={(e) => { e.preventDefault(); setDragOver(true) }}
        onDragLeave={() => setDragOver(false)}
        onDrop={onDrop}
      >
        <input
          ref={fileRef}
          type="file"
          accept=".txt,text/plain"
          className="lists-upload__file"
          onChange={(e) => {
            void ingest(e.target.files?.[0])
            e.target.value = ''
          }}
        />
        <div className="lists-upload__body">
          <div className="lists-upload__icon" aria-hidden>📥</div>
          <div>
            <strong>Drop a target list here</strong>
            <p className="lists-upload__hint">
              Plain <code className="inline-code">.txt</code>, one host per line.
              Files over 20 MiB are streamed to the backend in chunks — supports 1–2 GB lists.
            </p>
          </div>
          <button
            type="button"
            className="btn-primary btn-glass"
            disabled={uploadProgress !== null}
            onClick={() => fileRef.current?.click()}
          >
            Browse .txt
          </button>
        </div>
        {uploadProgress !== null && (
          <div className="lists-upload__progress">
            <div className="lists-upload__progress-bar" style={{ width: `${uploadProgress}%` }} />
            <span className="lists-upload__progress-label">
              {uploadProgress < 100 ? `Uploading… ${uploadProgress}%` : 'Finalizing…'}
            </span>
          </div>
        )}
        {error && (
          <p className={`lists-upload__error lists-upload__error--${error.kind}`}>
            {error.kind === 'duplicate' ? '⚠ ' : '✗ '}{error.message}
          </p>
        )}
      </div>

      <div className="lists-filter-row">
        {([
          { id: 'all',       label: 'All',       n: counts.all },
          { id: 'deployed',  label: 'Deployed',  n: counts.deployed },
          { id: 'idle',      label: 'Idle',      n: counts.idle },
          { id: 'completed', label: 'Completed', n: counts.completed },
        ] as const).map((f) => (
          <button
            key={f.id}
            type="button"
            className={`scan-filter${filter === f.id ? ' scan-filter--active' : ''}`}
            onClick={() => setFilter(f.id)}
          >
            {f.label}
            <span className="scan-filter__count">{f.n}</span>
          </button>
        ))}
      </div>

      {filtered.length === 0 ? (
        <p className="muted-callout">
          {lists.length === 0
            ? 'No lists uploaded yet. Drop a .txt file above to get started.'
            : 'No lists match this filter.'}
        </p>
      ) : (
        <div className="lists-grid">
          {filtered.map((list) => (
            <ListCard
              key={list.id}
              list={list}
              fleet={fleet}
              onToggleVps={toggleVps}
              onDeploy={(id) => onDeploy(id)}
              onPause={(id) => setStatus(id, 'idle')}
              onComplete={(id) => setStatus(id, 'completed')}
              onReset={(id) => setStatus(id, 'idle')}
              onDelete={onDelete}
              onRename={rename}
            />
          ))}
        </div>
      )}
    </section>
  )
}
