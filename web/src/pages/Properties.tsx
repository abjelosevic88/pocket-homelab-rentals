// Your places, each with its units: who lives where and for what rent.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData } from '../lib/data'
import { fmtMoney } from '../lib/format'
import { kindIcon, names } from '../lib/rentals'
import type { Property } from '../lib/types'
import { openCreate } from '../components/Layout'
import { Icon } from '../components/icons'
import { Empty, Loading, Segmented, useAsync } from '../components/ui'

export default function Properties() {
  const { properties } = useData()
  const [show, setShow] = useState<'current' | 'archived'>('current')
  const { data: archived } = useAsync(() => show === 'archived' ? api.get<Property[]>(`${V1}/properties?archived=1`) : Promise.resolve(null), [show])
  const list = show === 'archived' ? (archived || []).filter(p => p.archived) : properties
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Places</h1><p className="muted">The homes and spaces you rent out.</p></div>
        <Segmented size="sm" value={show} onChange={setShow} options={[{ value: 'current', label: 'Current' }, { value: 'archived', label: 'Archived' }]} />
        <button type="button" className="btn primary" onClick={() => openCreate('property')}><Icon name="plus" size={15} />Add a place</button>
      </div>
      {!list ? <Loading /> : list.length === 0 ? <Empty icon="building" title={show === 'archived' ? 'Nothing archived' : 'No places yet'}>{show === 'current' && <button type="button" className="btn primary" onClick={() => openCreate('property')}>Add your first place</button>}</Empty> : (
        <div className="prop-grid">
          {list.map(p => <PropertyCard key={p.id} p={p} />)}
        </div>
      )}
    </div>
  )
}

function PropertyCard({ p }: { p: Property }) {
  return (
    <div className="card prop-card" style={{ ['--pc' as string]: p.color || 'var(--accent)' }}>
      <Link to={`/properties/${p.id}`} className="prop-head">
        <span className="prop-ico"><Icon name={kindIcon(p.kind)} size={18} /></span>
        <span className="grow min0"><span className="prop-name">{p.name}</span><span className="muted small ellipsis block-line">{[p.address, p.city].filter(Boolean).join(', ') || '—'}</span></span>
        {p.owed > 0 && <span className="pill st-overdue" title="Rent late"><Icon name="alert" size={11} />{fmtMoney(p.owed, p.currency)}</span>}
      </Link>
      <ul className="unit-lines">
        {(p.units || []).map(u => {
          const l = u.lease?.status === 'active' ? u.lease : undefined
          return (
            <li key={u.id}>
              {l ? <Link to={`/leases/${l.id}`} className="unit-line">
                <span className={`unit-dot ${l.overdue > 0 ? 'late' : 'let'}`} aria-hidden="true" />
                <span className="grow min0 ellipsis"><b>{u.name}</b> · {names(l.tenants) || 'Let'}</span>
                <span className="tnum small">{fmtMoney(l.rent, l.currency)}</span>
              </Link> : <button type="button" className="unit-line empty-unit" onClick={() => openCreate('lease', { unit_id: u.id })}>
                <span className="unit-dot" aria-hidden="true" />
                <span className="grow min0 ellipsis"><b>{u.name}</b> · <span className="muted">{u.lease?.status === 'draft' ? 'draft lease' : 'empty'}</span></span>
                <span className="small accent-text">Let it</span>
              </button>}
            </li>
          )
        })}
        {(p.units || []).length === 0 && <li className="muted small pad-line">No units yet: open the place to add flats or rooms.</li>}
      </ul>
      <div className="prop-foot muted small">
        <span>{p.let} of {p.unit_count} let</span>
        {p.rent_roll > 0 && <span>{fmtMoney(p.rent_roll, p.currency)} a month</span>}
        {p.open_repair > 0 && <span><Icon name="wrench" size={12} className="inline-ico" /> {p.open_repair}</span>}
      </div>
    </div>
  )
}
