import { useCallback, useEffect, useRef, useState } from 'react'
import type { Finding } from '../types'
import { HIT_TOAST_LABEL, hitToastCategory } from '../lib/toastCategory'

export type HitToast = {
  id: string
  kind: 'hit'
  finding: Finding
}

export type AlertToast = {
  id: string
  kind: 'error' | 'info'
  title: string
  message?: string
}

export type ToastItem = HitToast | AlertToast

type Props = {
  toasts: readonly ToastItem[]
  onDismiss: (id: string) => void
}

const MAX_VISIBLE = 3
const TOAST_MS = 3000

export function ToastStack({ toasts, onDismiss }: Props) {
  const [visible, setVisible] = useState<ToastItem[]>([])
  const timersRef = useRef<Map<string, number>>(new Map())
  const doneRef = useRef<Set<string>>(new Set())

  const dismissOne = useCallback(
    (id: string) => {
      const tid = timersRef.current.get(id)
      if (tid !== undefined) {
        window.clearTimeout(tid)
        timersRef.current.delete(id)
      }
      doneRef.current.add(id)
      setVisible((v) => v.filter((t) => t.id !== id))
      onDismiss(id)
    },
    [onDismiss],
  )

  // Pull oldest queued items into open slots (FIFO waterfall).
  useEffect(() => {
    setVisible((current) => {
      const visibleIds = new Set(current.map((t) => t.id))
      const pending = toasts.filter((t) => !visibleIds.has(t.id) && !doneRef.current.has(t.id))
      if (current.length >= MAX_VISIBLE || pending.length === 0) return current
      const take = Math.min(MAX_VISIBLE - current.length, pending.length)
      return [...current, ...pending.slice(0, take)]
    })
  }, [toasts])

  // Auto-dismiss each visible toast after 3s.
  useEffect(() => {
    for (const item of visible) {
      if (timersRef.current.has(item.id)) continue
      const tid = window.setTimeout(() => dismissOne(item.id), TOAST_MS)
      timersRef.current.set(item.id, tid)
    }
  }, [visible, dismissOne])

  useEffect(() => {
    const live = new Set(toasts.map((t) => t.id))
    for (const id of doneRef.current) {
      if (!live.has(id)) doneRef.current.delete(id)
    }
  }, [toasts])

  useEffect(
    () => () => {
      for (const tid of timersRef.current.values()) window.clearTimeout(tid)
      timersRef.current.clear()
    },
    [],
  )

  return (
    <div className="toasts" aria-live="polite">
      {visible.map((t) => (
        <ToastBubble key={t.id} item={t} onDismiss={() => dismissOne(t.id)} />
      ))}
    </div>
  )
}

function ToastBubble({ item, onDismiss }: { item: ToastItem; onDismiss: () => void }) {
  if (item.kind === 'hit') {
    const category = hitToastCategory(item.finding)
    const lane = HIT_TOAST_LABEL[category]
    return (
      <div className={`toast toast--${category}`} onClick={onDismiss} role="status">
        <span className="toast__dot" aria-hidden />
        <div className="toast__body">
          <div className="toast__title">
            {lane} · {item.finding.provider}
          </div>
          <div className="toast__sub">{item.finding.hostname}</div>
        </div>
      </div>
    )
  }

  return (
    <div className={`toast toast--${item.kind}`} onClick={onDismiss} role="status">
      <span className="toast__dot" aria-hidden />
      <div className="toast__body">
        <div className="toast__title">{item.title}</div>
        {item.message ? <div className="toast__sub">{item.message}</div> : null}
      </div>
    </div>
  )
}
