import { useEffect, useState, type SVGProps } from 'react'

const COLLAPSED_KEY = 'reconx.sidebar.collapsed'

function readCollapsed(): boolean {
  if (typeof window === 'undefined') return false
  return window.localStorage.getItem(COLLAPSED_KEY) === '1'
}

function IcoChevron(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <polyline points="15 18 9 12 15 6" />
    </svg>
  )
}

function IcoCross(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      width="26"
      height="26"
      viewBox="0 0 64 64"
      fill="none"
      stroke="currentColor"
      strokeWidth="3"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M10 20 L10 10 L20 10" />
      <path d="M44 10 L54 10 L54 20" />
      <path d="M10 44 L10 54 L20 54" />
      <path d="M44 54 L54 54 L54 44" />
      <path d="M22 22 L42 42" />
      <path d="M42 22 L22 42" />
    </svg>
  )
}

function IcoDash(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <path d="M3 10.5 12 4l9 6.5v8.5a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1v-8.5z" strokeLinejoin="round" />
    </svg>
  )
}

function IcoServers(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <rect x="4" y="3" width="16" height="6" rx="1.5" />
      <rect x="4" y="12" width="16" height="6" rx="1.5" />
      <path d="M8 21v-3M16 21v-3" strokeLinecap="round" />
      <circle cx="8" cy="6" r="1.2" fill="currentColor" stroke="none" />
      <circle cx="8" cy="15" r="1.2" fill="currentColor" stroke="none" />
    </svg>
  )
}

function IcoTrophy(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <path d="M8 21h8M12 17v4M8 17h8a4 4 0 0 0 4-4V5H4v8a4 4 0 0 0 4 4z" strokeLinejoin="round" />
      <path d="M4 7H2v1a3 3 0 0 0 3 3M20 7h2v1a3 3 0 0 1-3 3" strokeLinecap="round" />
    </svg>
  )
}

function IcoRadar(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="5" />
      <circle cx="12" cy="12" r="1.5" fill="currentColor" stroke="none" />
      <path d="M12 12L20 6" strokeLinecap="round" />
    </svg>
  )
}

function IcoArchive(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <rect x="3" y="4" width="18" height="4" rx="1" />
      <path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8M10 12h4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function IcoList(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <path d="M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function IcoGear(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <circle cx="12" cy="12" r="3" />
      <path
        d="M12 1v2.2M12 20.8V23M4.2 4.2l1.6 1.6M18.2 18.2l1.6 1.6M1 12h2.2M20.8 12H23M4.2 19.8l1.6-1.6M18.2 5.8l1.6-1.6"
        strokeLinecap="round"
      />
    </svg>
  )
}

function IcoSearch(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <circle cx="11" cy="11" r="7" />
      <path d="M21 21l-4.35-4.35" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M8 11h6M11 8v6" strokeLinecap="round" />
    </svg>
  )
}

function IcoTerminal(props: SVGProps<SVGSVGElement>) {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" {...props}>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M7 9l3 3-3 3M13 15h4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export type DashboardTab = 'overview' | 'ravenx' | 'warc' | 'lists' | 'fleet' | 'findings' | 'dorks' | 'logs' | 'settings'

type Item = { id: DashboardTab; label: string; Ico: typeof IcoDash; goldWhenActive?: boolean }

const ITEMS: Item[] = [
  { id: 'overview', label: 'Dashboard', Ico: IcoDash },
  { id: 'ravenx', label: 'Cracker', Ico: IcoRadar, goldWhenActive: true },
  { id: 'warc', label: 'WARC', Ico: IcoArchive },
  { id: 'lists', label: 'Lists', Ico: IcoList },
  { id: 'fleet', label: 'Fleet', Ico: IcoServers },
  { id: 'findings', label: 'Hits', Ico: IcoTrophy },
  { id: 'dorks', label: 'Dorks', Ico: IcoSearch },
  { id: 'logs', label: 'Logs', Ico: IcoTerminal },
  { id: 'settings', label: 'Settings', Ico: IcoGear },
]

type Props = {
  active: DashboardTab
  onChange: (t: DashboardTab) => void
}

const TAB_TITLE: Record<DashboardTab, string> = {
  overview: 'Command overview',
  ravenx: 'Cracker workspace',
  warc: 'WARC harvest',
  lists: 'Target lists',
  fleet: 'Fleet & shards',
  findings: 'Hits ledger',
  dorks: 'Dork hunter',
  logs: 'Logs',
  settings: 'Integrations',
}

export function AppSidebar({ active, onChange }: Props) {
  const [collapsed, setCollapsed] = useState<boolean>(readCollapsed)

  useEffect(() => {
    document.body.classList.toggle('sidebar-collapsed', collapsed)
    window.localStorage.setItem(COLLAPSED_KEY, collapsed ? '1' : '0')
  }, [collapsed])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && (e.key === 'b' || e.key === 'B')) {
        e.preventDefault()
        setCollapsed((c) => !c)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <nav className={`app-sidebar${collapsed ? ' app-sidebar--collapsed' : ''}`} aria-label="Primary">
      <div className="app-sidebar__brand">
        <span className="app-sidebar__mark" aria-hidden>
          <IcoCross />
        </span>
        {!collapsed && (
          <div>
            <div className="app-sidebar__product">Recon<span className="app-sidebar__product-x">X</span></div>
            <div className="app-sidebar__tag">Ops console</div>
          </div>
        )}
        <button
          type="button"
          className="app-sidebar__collapse"
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          aria-expanded={!collapsed}
          onClick={() => setCollapsed((c) => !c)}
          title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          <IcoChevron style={{ transform: collapsed ? 'rotate(180deg)' : undefined }} />
        </button>
      </div>

      {!collapsed && <div className="app-sidebar__label">MENU</div>}
      <ul className="app-sidebar__list" role="tablist">
        {ITEMS.map((item) => {
          const isActive = active === item.id
          const Ico = item.Ico
          return (
            <li key={item.id}>
              <button
                type="button"
                role="tab"
                aria-selected={isActive}
                id={`tab-${item.id}`}
                aria-controls={`panel-${item.id}`}
                className={`side-nav-btn${isActive ? ' side-nav-btn--active' : ''}${isActive && item.goldWhenActive ? ' side-nav-btn--gold' : ''}`}
                onClick={() => onChange(item.id)}
                title={collapsed ? item.label : undefined}
              >
                <span className="side-nav-btn__ico" aria-hidden>
                  <Ico />
                </span>
                {!collapsed && <span className="side-nav-btn__label">{item.label}</span>}
              </button>
            </li>
          )
        })}
      </ul>

      {!collapsed && (
        <div className="app-sidebar__footer">
          <p className="app-sidebar__ctx">{TAB_TITLE[active]}</p>
          <p className="app-sidebar__kbd">
            <kbd>⌃</kbd>+<kbd>B</kbd> toggle
          </p>
        </div>
      )}
    </nav>
  )
}
