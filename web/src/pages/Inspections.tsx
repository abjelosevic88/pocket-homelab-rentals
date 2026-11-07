// Inspections: planned ones first, then what was found.
import { useCallback } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import { INSPECTION_KIND, placeName } from '../lib/rentals'
import type { Inspection } from '../lib/types'
import { openCreate } from '../components/Layout'
import { Pill } from '../components/Bits'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, useAsync } from '../components/ui'

export default function Inspections() {
  const { data, reload } = useAsync(() => api.get<Inspection[]>(`${V1}/inspections`), [])
  useOnChange(useCallback(() => { reload() }, [reload]))
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Inspections</h1><p className="muted">Walk through a place room by room, with photos — and compare moving out with moving in.</p></div>
        <button type="button" className="btn primary" onClick={() => openCreate('inspection')}><Icon name="plus" size={15} />Plan an inspection</button>
      </div>
      {!data ? <Loading /> : data.length === 0 ? <Empty icon="check-square" title="No inspections yet">Do one when a tenant moves in: it is what the deposit is measured against when they move out.</Empty> : (
        <Card flush><ul className="list">{data.map(i => (
          <li key={i.id}><Link to={`/inspections/${i.id}`} className="list-row">
            <span className={`list-ico ${i.status === 'planned' ? 'st-ok' : i.issues ? 'st-due' : 'st-done'}`}><Icon name="check-square" size={15} /></span>
            <span className="row-main grow"><span className="row-title">{INSPECTION_KIND[i.kind]}</span><span className="muted small ellipsis">{placeName(i.unit_name, i.property_name)} · {fmtDate(i.date)}{i.time ? `, ${i.time}` : ''}</span></span>
            {i.status === 'planned' ? <Pill cls="st-ok" icon="calendar">Planned</Pill> : i.issues ? <Pill cls="st-due">{i.issues} to look at</Pill> : <Pill cls="st-done" icon="check">All fine</Pill>}
          </Link></li>))}</ul></Card>
      )}
    </div>
  )
}
