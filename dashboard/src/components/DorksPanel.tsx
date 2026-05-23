import { useCallback, useEffect, useRef, useState } from 'react'
import { dorks as dorksApi, type DorkResult, type GeneratedDork, type SavedDork } from '../lib/reconApi'

function detectKeyProvider(key: string): 'openai' | 'anthropic' | null {
  const k = key.trim()
  if (k.startsWith('sk-ant-')) return 'anthropic'
  if (k.startsWith('sk-proj-') || (k.startsWith('sk-') && !k.startsWith('sk-ant-'))) return 'openai'
  return null
}

const EVOLVE_THRESHOLD = 3

type Platform = 'shodan' | 'fofa' | 'google'

const CATEGORIES = [
  { id: 'file_exposure', label: 'File Exposure' },
  { id: 'aws',          label: 'AWS / Cloud' },
  { id: 'smtp',         label: 'SMTP / Mail' },
  { id: 'api',          label: 'API Keys' },
  { id: 'env',          label: '.env / Config' },
  { id: 'git',          label: 'Git / Source' },
  { id: 'custom',       label: 'Custom' },
]

const PLATFORM_HINT: Record<Platform, string> = {
  shodan: 'e.g. http.html:"aws_secret_access_key" http.status:200',
  fofa:   'e.g. body="aws_secret_access_key" && status_code=200',
  google: 'e.g. filetype:tfstate "AKIA" -site:stackoverflow.com -site:medium.com',
}

type Props = {
  onImportTargets: (hosts: string[], label: string) => void
  onToast: (title: string, message?: string, kind?: 'error' | 'info') => void
}

export function DorksPanel({ onImportTargets, onToast }: Props) {
  const [savedDorks, setSavedDorks] = useState<SavedDork[]>([])
  const [catFilter, setCatFilter] = useState('all')

  const [query, setQuery]       = useState('')
  const [platform, setPlatform] = useState<Platform>('shodan')
  const [limit, setLimit]       = useState(100)
  const [results, setResults]   = useState<DorkResult[]>([])
  const [searchTotal, setSearchTotal] = useState<number | null>(null)
  const [searching, setSearching]     = useState(false)
  const [searchError, setSearchError] = useState<string | null>(null)
  const [selected, setSelected]       = useState<Set<string>>(new Set())

  const [genOpen, setGenOpen]           = useState(false)
  const [genObjective, setGenObjective] = useState('')
  const [genCategory, setGenCategory]   = useState('file_exposure')
  const [genPlatform, setGenPlatform]   = useState<Platform>('google')
  const [genCount, setGenCount]         = useState(10)
  const [generating, setGenerating]     = useState(false)
  const [generated, setGenerated]       = useState<GeneratedDork[]>([])
  const [genSource, setGenSource]       = useState<'ai' | 'template' | null>(null)

  const [keysOpen, setKeysOpen]           = useState(false)
  const [shodanKey, setShodanKey]         = useState('')
  const [fofaEmail, setFofaEmail]         = useState('')
  const [fofaKey, setFofaKey]             = useState('')
  const [anthropicKey, setAnthropicKey]   = useState('')
  const [openaiKey, setOpenaiKey]         = useState('')
  const [savingKeys, setSavingKeys]       = useState(false)

  const [savingDork, setSavingDork] = useState(false)

  const [autoHunting, setAutoHunting]       = useState(false)
  const [huntStatus, setHuntStatus]         = useState<string | null>(null)
  const [huntCycles, setHuntCycles]         = useState(0)
  const [huntNewDorks, setHuntNewDorks]     = useState(0)
  const autoStopRef = useRef(false)

  useEffect(() => {
    dorksApi.listSaved()
      .then((r) => setSavedDorks(r.dorks))
      .catch((e) => onToast('Could not load saved dorks', (e as Error).message, 'error'))
  }, [onToast])

  useEffect(() => {
    if (!keysOpen) return
    dorksApi.getKeys().then((k) => {
      setShodanKey(k.shodan_key ?? '')
      setFofaEmail(k.fofa_email ?? '')
      setFofaKey(k.fofa_key ?? '')
      setAnthropicKey(k.anthropic_key ?? '')
      setOpenaiKey(k.openai_key ?? '')
    }).catch((e) => onToast('Could not load API keys', (e as Error).message, 'error'))
  }, [keysOpen, onToast])

  const openInGoogle = useCallback(() => {
    if (!query.trim()) return
    window.open(`https://www.google.com/search?q=${encodeURIComponent(query.trim())}`, '_blank', 'noopener')
  }, [query])

  const runSearch = useCallback(async () => {
    if (!query.trim()) return
    if (platform === 'google') { openInGoogle(); return }
    setSearching(true)
    setSearchError(null)
    setResults([])
    setSelected(new Set())
    setSearchTotal(null)
    try {
      const r = await dorksApi.run({ query: query.trim(), platform, limit })
      setResults(r.results)
      setSearchTotal(r.total)
    } catch (e) {
      setSearchError((e as Error).message)
    } finally {
      setSearching(false)
    }
  }, [query, platform, limit, openInGoogle])

  const saveDork = useCallback(async () => {
    if (!query.trim()) return
    setSavingDork(true)
    try {
      const r = await dorksApi.save({ query: query.trim(), category: 'custom', platform, notes: '' })
      setSavedDorks((prev) => [r.dork, ...prev])
      onToast('Dork saved', query.trim().slice(0, 60))
    } catch (e) {
      onToast('Save failed', (e as Error).message, 'error')
    } finally {
      setSavingDork(false)
    }
  }, [query, platform, onToast])

  const deleteSaved = useCallback(async (id: string) => {
    try {
      await dorksApi.deleteSaved(id)
      setSavedDorks((prev) => prev.filter((d) => d.id !== id))
    } catch (e) {
      onToast('Delete failed', (e as Error).message, 'error')
    }
  }, [onToast])

  const runGenerate = useCallback(async () => {
    setGenerating(true)
    setGenerated([])
    try {
      const r = await dorksApi.generate({ objective: genObjective, platform: genPlatform, count: genCount, category: genCategory })
      setGenerated(r.dorks)
      setGenSource(r.source)
    } catch (e) {
      onToast('Generation failed', (e as Error).message, 'error')
    } finally {
      setGenerating(false)
    }
  }, [genObjective, genPlatform, genCount, genCategory, onToast])

  const adoptGenerated = useCallback(async (d: GeneratedDork) => {
    setQuery(d.query)
    setPlatform(genPlatform)
    setGenOpen(false)
    try {
      const r = await dorksApi.save({ query: d.query, category: genCategory, platform: genPlatform, notes: d.notes })
      setSavedDorks((prev) => [r.dork, ...prev])
    } catch (e) {
      onToast('Could not save dork', (e as Error).message, 'error')
    }
  }, [genPlatform, genCategory, onToast])

  const saveKeys = useCallback(async () => {
    setSavingKeys(true)
    try {
      await dorksApi.saveKeys({ shodan_key: shodanKey, fofa_email: fofaEmail, fofa_key: fofaKey, anthropic_key: anthropicKey, openai_key: openaiKey })
      onToast('API keys saved')
      setKeysOpen(false)
    } catch (e) {
      onToast('Save failed', (e as Error).message, 'error')
    } finally {
      setSavingKeys(false)
    }
  }, [shodanKey, fofaEmail, fofaKey, anthropicKey, openaiKey, onToast])

  const handleKeyPaste = useCallback((e: React.ClipboardEvent<HTMLInputElement>) => {
    const pasted = e.clipboardData.getData('text').trim()
    const provider = detectKeyProvider(pasted)
    if (!provider) return
    e.preventDefault()
    if (provider === 'openai') {
      setOpenaiKey(pasted)
      onToast('OpenAI key detected', 'Pasted into the OpenAI field automatically')
    } else {
      setAnthropicKey(pasted)
      onToast('Anthropic key detected', 'Pasted into the Anthropic field automatically')
    }
  }, [onToast])

  const startAutoHunt = useCallback(async () => {
    if (autoHunting) return
    const initial = await dorksApi.listSaved().catch(() => ({ dorks: [] }))
    const runnable = initial.dorks.filter((d) => d.platform !== 'both' && d.platform !== 'google')
    if (runnable.length === 0) {
      onToast('Auto Hunt needs saved dorks', 'Add Shodan or FOFA dorks to the library first.', 'error')
      return
    }
    setAutoHunting(true)
    setHuntCycles(0)
    setHuntNewDorks(0)
    autoStopRef.current = false

    let queue: SavedDork[] = [...runnable].sort((a, b) => (b.score ?? 0) - (a.score ?? 0))
    let qi = 0
    let newDorkCount = 0
    let cycleCount = 0

    while (!autoStopRef.current) {
      if (qi >= queue.length) {
        // End of queue — reload to pick up newly added dorks, re-sort by score
        try {
          const r = await dorksApi.listSaved()
          queue = r.dorks
            .filter((d) => d.platform !== 'both' && d.platform !== 'google')
            .sort((a, b) => (b.score ?? 0) - (a.score ?? 0))
          setSavedDorks(r.dorks)
        } catch { break }
        qi = 0
        cycleCount++
        setHuntCycles(cycleCount)
        if (autoStopRef.current) break
      }

      const dork = queue[qi++]
      if (!dork) continue

      setHuntStatus(`[${cycleCount + 1}] ${dork.query.slice(0, 60)}`)
      setQuery(dork.query)
      setPlatform(dork.platform as Platform)

      try {
        const r = await dorksApi.run({ query: dork.query, platform: dork.platform, limit: 100 })
        setResults(r.results)
        setSearchTotal(r.total)
        await dorksApi.scoreRun(dork.id, r.results.length)

        // Evolve high-performers — spawn variations with AI
        if (r.results.length >= EVOLVE_THRESHOLD && !autoStopRef.current) {
          const ev = await dorksApi.evolve({ query: dork.query, category: dork.category, platform: dork.platform, count: 5 }).catch(() => ({ ok: true, dorks: [], source: 'none' as const }))
          for (const nd of ev.dorks) {
            if (autoStopRef.current) break
            try {
              const saved = await dorksApi.save({ query: nd.query, category: dork.category, platform: dork.platform, notes: nd.notes })
              queue.push(saved.dork)
              setSavedDorks((prev) => [saved.dork, ...prev])
              newDorkCount++
              setHuntNewDorks(newDorkCount)
            } catch { /* skip */ }
          }
        }
      } catch { /* skip failing dork */ }

      // Rate limit between requests
      await new Promise((res) => setTimeout(res, 1800))
    }

    setAutoHunting(false)
    setHuntStatus(null)
    onToast('Auto Hunt stopped', `${cycleCount} full cycles · ${newDorkCount} new dorks spawned`)
  }, [autoHunting, onToast])

  const stopAutoHunt = useCallback(() => {
    autoStopRef.current = true
  }, [])

  const loadSaved = (d: SavedDork) => {
    setQuery(d.query)
    if (d.platform !== 'both') setPlatform(d.platform as Platform)
    else onToast('Platform not set', `This dork is tagged "both" — select Shodan, FOFA, or Google manually.`)
  }

  const toggleResult = (id: string) => {
    setSelected((prev) => {
      const n = new Set(prev)
      n.has(id) ? n.delete(id) : n.add(id)
      return n
    })
  }

  const importSelected = () => {
    const hosts = results
      .filter((r) => selected.has(r.id))
      .map((r) => {
        const h = r.hostname || r.ip
        return r.port && r.port !== 80 && r.port !== 443 ? `${h}:${r.port}` : h
      })
      .filter(Boolean)
    if (hosts.length === 0) return
    onImportTargets(hosts, `dork-${platform}-${Date.now()}`)
    onToast('Imported to cracker', `${hosts.length} host(s) added as new target list`)
    setSelected(new Set())
  }

  const visibleSaved = (catFilter === 'all'
    ? savedDorks
    : savedDorks.filter((d) => d.category === catFilter)
  ).sort((a, b) => (b.score ?? 0) - (a.score ?? 0))

  const isGoogle = platform === 'google'

  return (
    <div className="dorks-panel">
      <header className="dorks-panel__head">
        <div>
          <h2 className="dorks-panel__title">Dork Hunter</h2>
          <p className="dorks-panel__lede muted">
            AI-generated dorks · Shodan + FOFA search · Google file exposure · pipe results into the cracker
          </p>
        </div>
        <div className="dorks-panel__actions">
          {autoHunting ? (
            <button type="button" className="btn-danger" onClick={stopAutoHunt}>
              ■ Stop Hunt
            </button>
          ) : (
            <button type="button" className="btn-primary btn-with-ico" onClick={() => void startAutoHunt()} title="Auto-iterate saved dorks by score, spawn AI variants of high performers">
              ◎ Auto Hunt
            </button>
          )}
          <button type="button" className="btn-primary btn-with-ico" onClick={() => setGenOpen(true)}>
            ✦ AI Generate
          </button>
          <button type="button" className="btn-glass" onClick={() => setKeysOpen(true)}>
            API Keys
          </button>
        </div>
      </header>

      {autoHunting && (
        <div className="dorks-hunt-bar">
          <span className="dorks-hunt-bar__dot" aria-hidden />
          <span className="mono" style={{ fontSize: '.75rem' }}>{huntStatus || 'starting…'}</span>
          <span className="muted" style={{ fontSize: '.72rem', marginLeft: 'auto' }}>
            cycle {huntCycles + 1} · +{huntNewDorks} new dorks
          </span>
        </div>
      )}

      <div className="dorks-panel__body">
        {/* ── Saved dorks rail ── */}
        <aside className="dorks-rail">
          <div className="dorks-rail__head">
            <span className="dorks-rail__title">Saved dorks</span>
            <span className="mono muted" style={{ fontSize: '.72rem' }}>{savedDorks.length}</span>
          </div>
          <div className="dorks-rail__cats">
            <button
              type="button"
              className={`dorks-cat${catFilter === 'all' ? ' dorks-cat--on' : ''}`}
              onClick={() => setCatFilter('all')}
            >All</button>
            {CATEGORIES.map((c) => (
              <button
                key={c.id}
                type="button"
                className={`dorks-cat${catFilter === c.id ? ' dorks-cat--on' : ''}`}
                onClick={() => setCatFilter(c.id)}
              >{c.label}</button>
            ))}
          </div>
          <ul className="dorks-rail__list">
            {visibleSaved.length === 0 && (
              <li className="dorks-rail__empty muted">
                No saved dorks yet — generate some or run a search and save it.
              </li>
            )}
            {visibleSaved.map((d) => (
              <li key={d.id} className="dorks-rail__item">
                <button
                  type="button"
                  className="dorks-rail__load"
                  onClick={() => loadSaved(d)}
                  title={d.notes || d.query}
                >
                  <div className="dorks-rail__badges">
                    <span className="dorks-rail__badge dorks-rail__badge--cat">{d.category.replace('_', ' ')}</span>
                    <span className="dorks-rail__badge dorks-rail__badge--plat">{d.platform}</span>
                    {d.runs != null && d.runs > 0 && (
                      <span className={`dorks-rail__badge dorks-rail__badge--score${(d.score ?? 0) >= EVOLVE_THRESHOLD ? ' dorks-rail__badge--hot' : ''}`} title={`${d.hits ?? 0} hits over ${d.runs} runs`}>
                        ↑{d.score?.toFixed(1) ?? '0'}
                      </span>
                    )}
                  </div>
                  <span className="dorks-rail__query mono">{d.query}</span>
                  {d.notes && <span className="dorks-rail__notes muted">{d.notes}</span>}
                </button>
                <button
                  type="button"
                  className="dorks-rail__del"
                  onClick={() => void deleteSaved(d.id)}
                  aria-label="Delete dork"
                >×</button>
              </li>
            ))}
          </ul>
        </aside>

        {/* ── Search + results ── */}
        <div className="dorks-main">
          <div className="dorks-search">
            <div className="dorks-search__row">
              <div className="dorks-plat-toggle">
                {(['shodan', 'fofa', 'google'] as Platform[]).map((p) => (
                  <button
                    key={p}
                    type="button"
                    className={`dorks-plat-btn${platform === p ? ' dorks-plat-btn--on' : ''}`}
                    onClick={() => { setPlatform(p); setResults([]); setSearchError(null) }}
                  >{p.charAt(0).toUpperCase() + p.slice(1)}</button>
                ))}
              </div>
              <input
                className="tg-input dorks-search__input"
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') void runSearch() }}
                placeholder={PLATFORM_HINT[platform]}
                spellCheck={false}
              />
              {!isGoogle && (
                <select
                  className="tg-input dorks-search__limit"
                  value={limit}
                  onChange={(e) => setLimit(Number(e.target.value))}
                >
                  {[25, 50, 100, 200, 500].map((n) => <option key={n} value={n}>{n}</option>)}
                </select>
              )}
              <button
                type="button"
                className="btn-primary"
                onClick={() => void runSearch()}
                disabled={searching || !query.trim()}
              >
                {isGoogle ? '↗ Open in Google' : searching ? 'Searching…' : 'Search'}
              </button>
              <button
                type="button"
                className="btn-glass"
                onClick={() => void saveDork()}
                disabled={!query.trim() || savingDork}
                title="Save this query to the library"
              >{savingDork ? '…' : '＋ Save'}</button>
            </div>

            {isGoogle && query.trim() && (
              <div className="dorks-google-hint">
                <span className="muted" style={{ fontSize: '.78rem' }}>Google dork — opens in browser. Copy to search manually or use a SERP API for automation.</span>
                <code className="mono" style={{ fontSize: '.72rem', wordBreak: 'break-all', color: 'var(--accent)' }}>
                  https://www.google.com/search?q={encodeURIComponent(query.trim())}
                </code>
              </div>
            )}

            {searchError && (
              <p className="settings-hint" style={{ color: 'var(--danger)', marginTop: '.4rem' }}>{searchError}</p>
            )}
          </div>

          {/* Shodan/FOFA Results */}
          {results.length > 0 && (
            <div className="dorks-results">
              <div className="dorks-results__bar">
                <span className="muted" style={{ fontSize: '.8rem' }}>
                  Showing <strong>{results.length}</strong>
                  {searchTotal != null && searchTotal > results.length ? ` of ${searchTotal.toLocaleString()} total` : ''} results
                </span>
                <div style={{ display: 'flex', gap: '.5rem', alignItems: 'center', flexWrap: 'wrap' }}>
                  <button type="button" className="btn-glass btn-glass--xs"
                    onClick={() => setSelected(new Set(results.map((r) => r.id)))}>Select all</button>
                  <button type="button" className="btn-glass btn-glass--xs"
                    onClick={() => setSelected(new Set())}>Clear</button>
                  {selected.size > 0 && (
                    <button type="button" className="btn-primary"
                      style={{ fontSize: '.75rem', padding: '.3rem .8rem' }}
                      onClick={importSelected}
                    >⬆ Import {selected.size} to cracker</button>
                  )}
                </div>
              </div>
              <div className="dorks-results__table-wrap">
                <table className="dorks-table">
                  <thead>
                    <tr>
                      <th style={{ width: '2rem' }}>
                        <input type="checkbox"
                          checked={selected.size === results.length && results.length > 0}
                          onChange={(e) => setSelected(e.target.checked ? new Set(results.map((r) => r.id)) : new Set())}
                        />
                      </th>
                      <th>Host</th>
                      <th>IP</th>
                      <th>Port</th>
                      <th>Proto</th>
                      <th>Title</th>
                      <th>Banner</th>
                    </tr>
                  </thead>
                  <tbody>
                    {results.map((r) => (
                      <tr
                        key={r.id}
                        className={selected.has(r.id) ? 'dorks-row--selected' : undefined}
                        onClick={() => toggleResult(r.id)}
                      >
                        <td onClick={(e) => e.stopPropagation()}>
                          <input type="checkbox" checked={selected.has(r.id)} onChange={() => toggleResult(r.id)} />
                        </td>
                        <td className="mono" style={{ maxWidth: '180px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {r.hostname || r.host}
                        </td>
                        <td className="mono" style={{ whiteSpace: 'nowrap' }}>{r.ip}</td>
                        <td className="mono">{r.port}</td>
                        <td className="muted" style={{ fontSize: '.75rem' }}>{r.protocol}</td>
                        <td style={{ maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: '.78rem' }}>
                          {r.title || <span className="muted">—</span>}
                        </td>
                        <td style={{ maxWidth: '220px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: '.72rem' }} className="muted">
                          {r.data || '—'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {!searching && results.length === 0 && !searchError && !isGoogle && (
            <div className="muted-callout" style={{ marginTop: '1.5rem' }}>
              <p style={{ margin: 0, fontWeight: 600 }}>No results yet</p>
              <p className="muted" style={{ margin: '.35rem 0 0', fontSize: '.82rem' }}>
                Pick a saved dork from the library, generate new ones with AI, or write a query and hit Search.
              </p>
            </div>
          )}
        </div>
      </div>

      {/* ── AI Generator modal ── */}
      {genOpen && (
        <div className="cw-hub-modal__backdrop" role="dialog" aria-modal="true" onClick={() => !generating && setGenOpen(false)}>
          <div className="cw-hub-modal" style={{ width: 'min(720px, 94vw)' }} onClick={(e) => e.stopPropagation()}>
            <header className="cw-hub-modal__head">
              <div>
                <h3 style={{ margin: 0 }}>AI Dork Generator</h3>
                <p className="muted" style={{ margin: '.25rem 0 0', fontSize: '.82rem' }}>
                  Describe what you're hunting — ChatGPT or Claude writes syntax-perfect dorks for the selected platform.
                  Falls back to curated templates if no AI key is configured.
                </p>
              </div>
              <button type="button" className="btn-glass btn-glass--xs" onClick={() => setGenOpen(false)} disabled={generating}>Close</button>
            </header>

            <div className="dorks-gen-form">
              <label className="cw-composer__field">
                <span className="cw-composer__label">Category</span>
                <div style={{ display: 'flex', gap: '.4rem', flexWrap: 'wrap' }}>
                  {CATEGORIES.map((c) => (
                    <button key={c.id} type="button"
                      className={`dorks-cat${genCategory === c.id ? ' dorks-cat--on' : ''}`}
                      onClick={() => setGenCategory(c.id)}
                    >{c.label}</button>
                  ))}
                </div>
              </label>

              <label className="cw-composer__field">
                <span className="cw-composer__label">Platform</span>
                <div className="dorks-plat-toggle">
                  {(['shodan', 'fofa', 'google'] as Platform[]).map((p) => (
                    <button key={p} type="button"
                      className={`dorks-plat-btn${genPlatform === p ? ' dorks-plat-btn--on' : ''}`}
                      onClick={() => setGenPlatform(p)}
                    >{p.charAt(0).toUpperCase() + p.slice(1)}</button>
                  ))}
                </div>
              </label>

              <label className="cw-composer__field">
                <span className="cw-composer__label">Objective (optional — used by AI)</span>
                <input className="tg-input" type="text" value={genObjective}
                  onChange={(e) => setGenObjective(e.target.value)}
                  placeholder={`e.g. find exposed ${CATEGORIES.find((c) => c.id === genCategory)?.label ?? 'credentials'} on small hosting providers`}
                  spellCheck={false}
                />
              </label>

              <label className="cw-composer__field">
                <span className="cw-composer__label">Count</span>
                <select className="tg-input" value={genCount} onChange={(e) => setGenCount(Number(e.target.value))}>
                  {[5, 10, 15, 20, 30].map((n) => <option key={n} value={n}>{n} dorks</option>)}
                </select>
              </label>

              <div className="settings-btn-row">
                <button type="button" className="btn-primary" onClick={() => void runGenerate()} disabled={generating}>
                  {generating ? 'Generating…' : '✦ Generate'}
                </button>
              </div>
            </div>

            {generated.length > 0 && (
              <div className="dorks-gen-results">
                <p className="muted" style={{ fontSize: '.78rem', margin: '0 0 .6rem' }}>
                  {generated.length} dorks — {genSource === 'ai' ? 'Claude AI' : 'curated templates'}
                  {' · '}click to load into the search bar and save to library.
                </p>
                <ul className="dorks-gen-list">
                  {generated.map((d, i) => (
                    <li key={i}>
                      <button type="button" className="dorks-gen-item" onClick={() => void adoptGenerated(d)}>
                        <span className="mono dorks-gen-item__query">{d.query}</span>
                        {d.notes && <span className="muted dorks-gen-item__notes">{d.notes}</span>}
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── API Keys modal ── */}
      {keysOpen && (
        <div className="cw-hub-modal__backdrop" role="dialog" aria-modal="true" onClick={() => setKeysOpen(false)}>
          <div className="cw-hub-modal" style={{ width: 'min(560px, 94vw)' }} onClick={(e) => e.stopPropagation()}>
            <header className="cw-hub-modal__head">
              <div>
                <h3 style={{ margin: 0 }}>API Keys</h3>
                <p className="muted" style={{ margin: '.25rem 0 0', fontSize: '.82rem' }}>Stored on the controller — Anthropic key is write-only after first save.</p>
              </div>
              <button type="button" className="btn-glass btn-glass--xs" onClick={() => setKeysOpen(false)}>Close</button>
            </header>
            <div className="cw-composer">
              <label className="cw-composer__field">
                <span className="cw-composer__label">Shodan API key</span>
                <input className="tg-input" type="password" value={shodanKey} onChange={(e) => setShodanKey(e.target.value)} onPaste={handleKeyPaste} placeholder="your-shodan-api-key" spellCheck={false} autoComplete="off" />
              </label>
              <label className="cw-composer__field">
                <span className="cw-composer__label">FOFA email</span>
                <input className="tg-input" type="email" value={fofaEmail} onChange={(e) => setFofaEmail(e.target.value)} placeholder="you@example.com" autoComplete="off" />
              </label>
              <label className="cw-composer__field">
                <span className="cw-composer__label">FOFA API key</span>
                <input className="tg-input" type="password" value={fofaKey} onChange={(e) => setFofaKey(e.target.value)} onPaste={handleKeyPaste} placeholder="your-fofa-api-key" spellCheck={false} autoComplete="off" />
              </label>
              <label className="cw-composer__field">
                <span className="cw-composer__label">OpenAI API key (ChatGPT — tried first)</span>
                <input className="tg-input" type="password" value={openaiKey} onChange={(e) => setOpenaiKey(e.target.value)} onPaste={handleKeyPaste}
                  placeholder={openaiKey === '***' ? '(saved — enter new to replace)' : 'sk-...'} spellCheck={false} autoComplete="off" />
              </label>
              <label className="cw-composer__field">
                <span className="cw-composer__label">Anthropic API key (Claude — fallback)</span>
                <input className="tg-input" type="password" value={anthropicKey} onChange={(e) => setAnthropicKey(e.target.value)} onPaste={handleKeyPaste}
                  placeholder={anthropicKey === '***' ? '(saved — enter new to replace)' : 'sk-ant-...'} spellCheck={false} autoComplete="off" />
              </label>
              <div className="settings-btn-row">
                <button type="button" className="btn-primary" onClick={() => void saveKeys()} disabled={savingKeys}>
                  {savingKeys ? 'Saving…' : 'Save keys'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
