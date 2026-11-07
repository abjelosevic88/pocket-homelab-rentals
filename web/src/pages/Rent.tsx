// One month of rent: every charge due in it, paid or not, with the totals.
import { useCallback, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { fmtDate, fmtMoney, fmtMoneyMap } from '../lib/format'
import { CHARGE_STATE, names } from '../lib/rentals'
import type { DueItem, Money } from '../lib/types'
import { openCreate } from '../components/Layout'
import { Pill } from '../components/Bits'
import { FoundInMoney } from './Today'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, useAsync } from '../components/ui'

const shift = (m: string, by: number) => { const [y, mm] = m.split('-').map(Number); const d = new Date(y, mm - 1 + by, 1); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}` }
const monthWords = (m: string) => new Date(Number(m.slice(0, 4)), Number(m.slice(5, 7)) - 1, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' })

export default function Rent() {
  const { settings } = useApp()
  const cur = settings?.currency || 'EUR'
  const [month, setMonth] = useState(() => new Date().toISOString().slice(0, 7))
  const { data, reload } = useAsync(() => api.get<{ month: string; items: DueItem[]; expected: Money; received: Money; outstanding: Money }>(`${V1}/rent?month=${month}`), [month])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const groups: [string, string, DueItem[]][] = data ? [
    ['Late', 'alert', data.items.filter(i => i.state === 'late')],
    ['Due', 'clock', data.items.filter(i => i.state === 'due' || i.state === 'part')],
    ['Coming', 'calendar', data.items.filter(i => i.state === 'upcoming')],
    ['Paid', 'check', data.items.filter(i => i.state === 'paid')],
    ['Waived', 'x', data.items.filter(i => i.state === 'waived')],
  ] : []
  const thisMonth = month === new Date().toISOString().slice(0, 7)
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Rent</h1><p className="muted">Everything due in {monthWords(month)}, and how far it is paid.</p></div>
        <div className="row tight month-pick">
          <button type="button" className="btn icon-only" aria-label="The month before" onClick={() => setMonth(m => shift(m, -1))}><Icon name="chevron-left" size={16} /></button>
          <span className="month-label">{monthWords(month)}</span>
          <button type="button" className="btn icon-only" aria-label="The month after" onClick={() => setMonth(m => shift(m, 1))}><Icon name="chevron-right" size={16} /></button>
          {!thisMonth && <button type="button" className="btn sm" onClick={() => setMonth(new Date().toISOString().slice(0, 7))}>This month</button>}
        </div>
      </div>
      {!data ? <Loading /> : <>
        <div className="stats-row">
          <div className="stat"><span className="stat-l">Due this month</span><span className="stat-v tnum">{fmtMoneyMap(data.expected, cur)}</span></div>
          <div className="stat"><span className="stat-l">Came in</span><span className="stat-v tnum ok">{fmtMoneyMap(data.received, cur)}</span></div>
          <div className="stat"><span className="stat-l">Still open</span><span className={`stat-v tnum ${Object.values(data.outstanding).some(v => v > 0) ? 'bad' : ''}`}>{fmtMoneyMap(data.outstanding, cur)}</span></div>
        </div>
        {thisMonth && <FoundInMoney onDone={reload} />}
        {data.items.length === 0 ? <Empty icon="wallet" title="Nothing due this month">Rent shows here once a lease runs in this month.</Empty> : groups.filter(g => g[2].length).map(([title, icon, items]) => (
          <Card key={title} title={<span className={`sec-title ${title === 'Late' ? 'bad' : ''}`}><Icon name={icon} size={15} />{title}<span className="count">{items.length}</span></span>} flush>
            <ul className="list">{items.map(d => {
              const st = CHARGE_STATE[d.state]
              const owed = d.amount - d.paid
              return (
                <li key={d.charge_id} className="list-row">
                  <Link to={`/leases/${d.lease_id}`} className="row-main grow">
                    <span className="row-title">{d.place}</span>
                    <span className="muted small ellipsis">{names(d.tenants) || 'No tenant'} · {d.title} · due {fmtDate(d.due_date)}{d.paid_on && d.state === 'paid' ? ` · paid ${fmtDate(d.paid_on)}` : ''}</span>
                  </Link>
                  <Pill cls={st.cls} icon={st.icon}>{st.label}</Pill>
                  <span className="row-amount tnum">{d.state === 'paid' || d.state === 'waived' ? fmtMoney(d.amount, d.currency) : <><b>{fmtMoney(owed, d.currency)}</b>{d.paid > 0 && <span className="muted tiny block-line">of {fmtMoney(d.amount, d.currency)}</span>}</>}</span>
                  {d.state !== 'paid' && d.state !== 'waived' ? <button type="button" className="btn sm" onClick={() => openCreate('payment', { lease_id: d.lease_id, amount: owed })}><Icon name="check" size={14} /><span className="hide-sm">Paid</span></button> : <span className="btn-space" />}
                </li>
              )
            })}</ul>
          </Card>
        ))}
      </>}
    </div>
  )
}
