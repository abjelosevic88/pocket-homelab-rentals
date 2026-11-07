// Every lease: active first (late ones on top), drafts, and the ones that ended.
import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useOnChange } from '../lib/data'
import { fmtDate, fmtMoney } from '../lib/format'
import { dueWords, names, perWord, placeName } from '../lib/rentals'
import type { Lease } from '../lib/types'
import { openCreate } from '../components/Layout'
import { Pill } from '../components/Bits'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, Segmented, useAsync } from '../components/ui'

export default function Leases() {
  const { data, reload } = useAsync(() => api.get<Lease[]>(`${V1}/leases`), [])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const [show, setShow] = useState<'active' | 'draft' | 'ended'>('active')
  const [q, setQ] = useState('')
  const list = useMemo(() => (data || []).filter(l => l.status === show && (!q || l.title.toLowerCase().includes(q.toLowerCase())))
    .sort((a, b) => (b.overdue > 0 ? 1 : 0) - (a.overdue > 0 ? 1 : 0)), [data, show, q])
  const n = (s: string) => (data || []).filter(l => l.status === s).length
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Leases</h1><p className="muted">Who rents what, for how much and until when.</p></div>
        <button type="button" className="btn primary" onClick={() => openCreate('lease')}><Icon name="plus" size={15} />New lease</button>
      </div>
      <div className="row wrap filters">
        <Segmented size="sm" value={show} onChange={setShow} options={[{ value: 'active', label: `Active ${n('active') || ''}` }, { value: 'draft', label: `Drafts ${n('draft') || ''}` }, { value: 'ended', label: `Ended ${n('ended') || ''}` }]} />
        <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find a place or a tenant" aria-label="Find" /></div>
      </div>
      {!data ? <Loading /> : list.length === 0 ? <Empty icon="key" title={show === 'active' ? 'No active leases' : show === 'draft' ? 'No drafts' : 'No ended leases'} /> : (
        <Card flush><ul className="list">{list.map(l => <LeaseRow key={l.id} l={l} />)}</ul></Card>
      )}
    </div>
  )
}

export function LeaseRow({ l }: { l: Lease }) {
  const ending = l.status === 'active' && l.days_left !== undefined && l.days_left <= 60
  return (
    <li><Link to={`/leases/${l.id}`} className="list-row">
      <span className={`list-ico ${l.overdue > 0 ? 'st-overdue' : l.status === 'ended' ? '' : 'st-ok'}`}><Icon name={l.status === 'ended' ? 'archive' : 'key'} size={15} /></span>
      <span className="row-main grow">
        <span className="row-title">{placeName(l.unit_name, l.property_name)}</span>
        <span className="muted small ellipsis">{names(l.tenants) || 'No tenant yet'} · {fmtMoney(l.rent, l.currency)} {perWord(l.frequency)}, {dueWords(l)}{l.status === 'ended' ? ` · ended ${fmtDate(l.ended_on || l.end_date)}` : l.end_date ? ` · to ${fmtDate(l.end_date)}` : ' · until notice'}</span>
      </span>
      {l.overdue > 0 ? <Pill cls="st-overdue" icon="alert">Owes {fmtMoney(l.balance, l.currency)}</Pill>
        : l.balance > 0 ? <Pill cls="st-due">Due {fmtMoney(l.balance, l.currency)}</Pill>
          : ending ? <Pill cls="st-due" icon="calendar">Ends in {l.days_left} days</Pill>
            : l.status === 'draft' ? <Pill cls="st-paused">Draft</Pill>
              : l.status === 'active' ? <Pill cls="st-done" icon="check">Paid up</Pill> : null}
    </Link></li>
  )
}
