// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { ImportModal } from './ImportModal'
import { ExpenseForm, LeaseForm, PaymentForm, PropertyForm, RequestForm, InspectionForm } from './Forms'
import { Avatar, Loading, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'prt-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'property' | 'lease' | 'payment' | 'repair' | 'expense' | 'inspection' | 'import'
export type CreateInitial = Record<string, unknown>

/** Start something from anywhere (a page's button), with what is known already. */
export const openCreate = (k: CreateKind, initial?: CreateInitial) => window.dispatchEvent(new CustomEvent('prt:create', { detail: { kind: k, initial } }))

/** The "new" menu: the sidebar's caret. */
export function createItems(onCreate: (k: CreateKind) => void): MenuItem[] {
  return [
    { label: 'Record rent that came in', icon: 'wallet', hint: 'N', onSelect: () => onCreate('payment') },
    { label: 'Report a repair', icon: 'wrench', hint: 'X', onSelect: () => onCreate('repair') },
    { label: 'Add a cost', icon: 'receipt', hint: 'C', onSelect: () => onCreate('expense') },
    { label: 'Plan an inspection', icon: 'check-square', onSelect: () => onCreate('inspection') },
    'sep',
    { label: 'New lease', icon: 'key', onSelect: () => onCreate('lease') },
    { label: 'Add a place', icon: 'home', hint: 'A', onSelect: () => onCreate('property') },
    'sep',
    { label: 'Bring leases or payments from a spreadsheet…', icon: 'upload', onSelect: () => onCreate('import') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { properties } = useData()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [making, setMaking] = useState<{ kind: CreateKind; initial: CreateInitial } | null>(null)

  const empty = properties !== null && properties.length === 0
  const create = (k: CreateKind, initial?: CreateInitial) => {
    closeDrawer()
    // nothing to record rent for yet: a place first
    if (empty && k !== 'property' && k !== 'import') k = 'property'
    setMaking({ kind: k, initial: initial || {} })
  }
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind; initial?: CreateInitial }>).detail; createRef.current(d.kind, d.initial) }
    window.addEventListener('prt:shortcuts', open)
    window.addEventListener('prt:create', make)
    return () => { window.removeEventListener('prt:shortcuts', open); window.removeEventListener('prt:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': mk('payment'); break
        case 'a': mk('property'); break
        case 'x': mk('repair'); break
        case 'c': mk('expense'); break
        case 'h': go('/'); break
        case 'r': go('/rent'); break
        case 'p': go('/properties'); break
        case 'l': go('/leases'); break
        case 't': go('/tenants'); break
        case 'f': go('/repairs'); break
        case 'i': go('/inspections'); break
        case 'g': go('/reports'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'pay', label: 'Record rent that came in', icon: 'wallet', hint: 'N', keywords: 'payment paid money in', run: () => create('payment') },
    { id: 'repair', label: 'Report a repair', icon: 'wrench', hint: 'X', keywords: 'fix broken maintenance', run: () => create('repair') },
    { id: 'cost', label: 'Add a cost', icon: 'receipt', hint: 'C', keywords: 'expense bill tax insurance', run: () => create('expense') },
    { id: 'insp', label: 'Plan an inspection', icon: 'check-square', keywords: 'move in move out check', run: () => create('inspection') },
    { id: 'lease', label: 'New lease', icon: 'key', keywords: 'tenant contract let rent out', run: () => create('lease') },
    { id: 'place', label: 'Add a place', icon: 'home', hint: 'A', keywords: 'property flat house building unit', run: () => create('property') },
    { id: 'import', label: 'Bring leases or payments from a spreadsheet', icon: 'upload', keywords: 'import csv excel', run: () => create('import') },
    { id: 'today', label: 'Today', icon: 'today', hint: 'H', run: () => navigate('/') },
    { id: 'rent', label: 'Rent this month', icon: 'wallet', hint: 'R', keywords: 'due late paid', run: () => navigate('/rent') },
    { id: 'places', label: 'Places', icon: 'building', hint: 'P', run: () => navigate('/properties') },
    { id: 'leases', label: 'Leases', icon: 'key', hint: 'L', run: () => navigate('/leases') },
    { id: 'tenants', label: 'Tenants', icon: 'users', hint: 'T', run: () => navigate('/tenants') },
    { id: 'repairs', label: 'Repairs', icon: 'wrench', hint: 'F', run: () => navigate('/repairs') },
    { id: 'inspections', label: 'Inspections', icon: 'check-square', hint: 'I', run: () => navigate('/inspections') },
    { id: 'reports', label: 'Reports: what each place earns and costs', icon: 'chart', hint: 'G', keywords: 'tax income expenses year', run: () => navigate('/reports') },
    ...(properties || []).map(p => ({ id: `p-${p.id}`, label: p.name, icon: 'home', keywords: `${p.address} ${p.city}`, run: () => navigate(`/properties/${p.id}`) })),
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'rent-set', label: 'Settings: rent and reminders', icon: 'sliders', keywords: 'due day grace notice currency', run: () => navigate('/settings/rent') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'link', keywords: 'contacts money sign calendar tasks notes mail invoicing documents home forms core', run: () => navigate('/settings/apps') },
    { id: 'ai-set', label: 'Settings: AI', icon: 'sparkles', keywords: 'pocket ai openai', run: () => navigate('/settings/ai') },
    { id: 'export', label: 'Settings: export', icon: 'download', keywords: 'backup json csv', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  const close = () => setMaking(null)
  const m = making
  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className="content"><div className="content-inner">{properties ? <Outlet /> : <Loading />}</div></main>
        </div>
        {mobile && !/^\/(settings|inspections\/\d)/.test(loc.pathname) && <button type="button" className="fab" aria-label={empty ? 'Add a place' : 'Record rent'} title={empty ? 'Add a place' : 'Record rent'} onClick={() => create(empty ? 'property' : 'payment')}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {m?.kind === 'import' && <ImportModal onClose={close} />}
        {m?.kind === 'property' && <PropertyForm onClose={close} />}
        {m?.kind === 'lease' && <LeaseForm initial={m.initial} onClose={close} />}
        {m?.kind === 'payment' && <PaymentForm initial={m.initial} onClose={close} />}
        {m?.kind === 'repair' && <RequestForm initial={m.initial} onClose={close} />}
        {m?.kind === 'expense' && <ExpenseForm initial={m.initial} onClose={close} />}
        {m?.kind === 'inspection' && <InspectionForm initial={m.initial} onClose={close} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = {
  '': ['Today', '/'], rent: ['Rent', '/rent'], properties: ['Places', '/properties'], leases: ['Leases', '/leases'], tenants: ['Tenants', '/tenants'],
  repairs: ['Repairs', '/repairs'], inspections: ['Inspections', '/inspections'], reports: ['Reports', '/reports'], costs: ['Costs', '/costs'],
}

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('prt:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
