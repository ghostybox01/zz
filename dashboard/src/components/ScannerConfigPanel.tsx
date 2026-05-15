import { useEffect, useRef, useState } from 'react'
import {
  scannerConfig,
  scannerPaths,
  type ReconScannerConfig,
  type ReconScannerConfigPatch,
  type ReconScannerPaths,
} from '../lib/reconApi'
import {
  defaultPlatformSelection,
  type PlatformSelection,
} from '../data/scannerModules'
import { VulnerabilityScannersPanel } from './VulnerabilityScannersPanel'

type Props = {
  onToast?: (msg: { kind: 'info' | 'error'; title: string; message?: string }) => void
}

/** Self-contained wrapper around VulnerabilityScannersPanel — loads + saves
 *  scanner-config and paths-file on its own. Drop into Settings. */
export function ScannerConfigPanel({ onToast }: Props) {
  const [platforms, setPlatforms] = useState<PlatformSelection>(() => defaultPlatformSelection())
  const [pathFileName, setPathFileName] = useState<string | null>(null)
  const [pathState, setPathState] = useState<ReconScannerPaths | null>(null)
  const [pathBusy, setPathBusy] = useState(false)
  const [config, setConfig] = useState<ReconScannerConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const saveSeq = useRef(0)

  useEffect(() => {
    let cancelled = false
    scannerConfig.get()
      .then((c) => { if (!cancelled) { setConfig(c); setError(null) } })
      .catch((e: Error) => { if (!cancelled) setError(e.message) })
      .finally(() => { if (!cancelled) setLoading(false) })
    scannerPaths.get().then((p) => { if (!cancelled) setPathState(p) }).catch(() => {})
    return () => { cancelled = true }
  }, [])

  function patch(p: ReconScannerConfigPatch) {
    setConfig((prev) => {
      if (!prev) return prev
      const next: Record<string, Record<string, boolean>> = {
        scanning_features: { ...prev.scanning_features },
        aws_checks: { ...prev.aws_checks },
        api_validation: { ...prev.api_validation },
        features: { ...prev.features },
        exploit_methods: { ...prev.exploit_methods },
      }
      for (const sec of Object.keys(p)) {
        const sp = (p as Record<string, Record<string, boolean> | undefined>)[sec]
        if (sp) next[sec] = { ...next[sec], ...sp }
      }
      return next as unknown as ReconScannerConfig
    })
    const seq = ++saveSeq.current
    setSaving(true)
    scannerConfig.update(p)
      .then((c) => { if (seq === saveSeq.current) { setConfig(c); setError(null) } })
      .catch((e: Error) => {
        if (seq !== saveSeq.current) return
        setError(e.message)
        onToast?.({ kind: 'error', title: 'Save failed', message: e.message })
      })
      .finally(() => { if (seq === saveSeq.current) setSaving(false) })
  }

  async function uploadPaths(file: File | null) {
    if (!file) {
      setPathFileName(null); setPathBusy(true)
      try {
        const cleared = await scannerPaths.clear()
        setPathState(cleared)
        onToast?.({ kind: 'info', title: 'Path list reverted', message: 'Backend now uses built-in paths.' })
      } catch (e) {
        onToast?.({ kind: 'error', title: 'Clear failed', message: (e as Error).message })
      } finally { setPathBusy(false) }
      return
    }
    setPathFileName(file.name); setPathBusy(true)
    try {
      const next = await scannerPaths.upload(file)
      setPathState(next)
      onToast?.({ kind: 'info', title: 'Paths uploaded', message: `${next.lines} path${next.lines === 1 ? '' : 's'} active.` })
    } catch (e) {
      onToast?.({ kind: 'error', title: 'Upload failed', message: (e as Error).message })
    } finally { setPathBusy(false) }
  }

  return (
    <VulnerabilityScannersPanel
      config={config}
      loading={loading}
      saving={saving}
      error={error}
      platforms={platforms}
      pathFileName={pathFileName ?? (pathState?.present ? `paths.txt (${pathState.lines})` : null)}
      pathBusy={pathBusy}
      onPatch={patch}
      onTogglePlatform={(id, on) => setPlatforms((s) => ({ ...s, [id]: on }))}
      onPathFile={(f) => void uploadPaths(f)}
      onDeselectPlatforms={() => setPlatforms({ github: false, gitlab: false, bitbucket: false })}
    />
  )
}
