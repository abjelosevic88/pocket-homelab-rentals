// Left column: record rent (and the other ways to start), the sections, then your places.
import { useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { kindIcon } from '../lib/rentals'
import { useAppState, useData } from '../lib/data'
import { Icon } from './icons'
import { createItems, type CreateKind } from './Layout'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { properties, summary } = useData()
  const appState = useAppState()
  const forms = appState('forms', 'forms:read')
  const navigate = useNavigate()
  const loc = useLocation()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket Rentals'
  const empty = !!properties && properties.length === 0
  const path = loc.pathname
  const nav: { to: string; label: string; icon: string; active: boolean; count?: number; bad?: boolean; hint?: string }[] = [
    { to: '/', label: 'Today', icon: 'today', active: path === '/', count: (summary?.late || 0) + (summary?.urgent || 0), bad: !!summary?.late, hint: 'Late rent and urgent repairs' },
    { to: '/rent', label: 'Rent', icon: 'wallet', active: path === '/rent', count: summary?.due, hint: 'Rent due in the next days' },
    { to: '/properties', label: 'Places', icon: 'building', active: path.startsWith('/properties') },
    { to: '/leases', label: 'Leases', icon: 'key', active: path.startsWith('/leases'), count: summary?.ending, hint: 'Leases ending soon' },
    { to: '/tenants', label: 'Tenants', icon: 'users', active: path.startsWith('/tenants') },
    { to: '/repairs', label: 'Repairs', icon: 'wrench', active: path.startsWith('/repairs'), count: summary?.repairs, hint: forms.ok ? 'Repairs not done yet (and what tenants report in Pocket Forms)' : 'Repairs not done yet' },
    { to: '/inspections', label: 'Inspections', icon: 'check-square', active: path.startsWith('/inspections') },
    { to: '/costs', label: 'Costs', icon: 'receipt', active: path.startsWith('/costs') },
    { to: '/reports', label: 'Reports', icon: 'chart', active: path === '/reports' },
  ]
  const cur = path.startsWith('/properties/') ? Number(path.split('/')[2]) : 0
  return (
    <aside className="sidebar" aria-label="Pocket Rentals">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Rentals' ? title : <>Pocket <b>Rentals</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate(empty ? 'property' : 'payment')} title={empty ? 'Add a place (A)' : 'Record rent that came in (N)'}>
          <Icon name="plus" size={15} strokeWidth={2.2} />{empty ? 'Add a place' : 'Record rent'}
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createItems(onCreate)} onClose={() => setMenu(null)} width={300} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} title={n.hint} className={() => `sb-nav-item ${n.active ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className={`sb-count ${n.bad ? 'bad' : ''}`} title={n.hint}>{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      {properties && properties.length > 0 && <section className="sb-sec">
        <h4 className="sb-h">Your places</h4>
        {properties.map(p => (
          <button key={p.id} type="button" className={`sb-row sb-list ${cur === p.id ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/properties/${p.id}`) }}
            title={`${p.let} of ${p.unit_count} let`}>
            <span className="sb-place-ico" style={{ color: p.color || undefined }} aria-hidden="true"><Icon name={kindIcon(p.kind)} size={14} /></span>
            <span className="grow ellipsis">{p.name}</span>
            <span className="muted small tnum">{p.unit_count > 1 ? `${p.let}/${p.unit_count}` : ''}</span>
          </button>
        ))}
      </section>}
      {empty && <section className="sb-sec">
        <button type="button" className="sb-row sb-list" onClick={() => onCreate('property')}><span className="sb-list-ico" aria-hidden="true"><Icon name="plus" size={14} /></span><span className="grow">Add your first place</span></button>
      </section>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('prt:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
