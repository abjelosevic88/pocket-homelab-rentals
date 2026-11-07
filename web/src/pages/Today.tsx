// What needs you today: rent late and due, rent found in your bank, leases
// ending, repairs, inspections coming, units standing empty.
import { useCallback, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useData, useOnChange } from '../lib/data'
import { fmtDate, fmtMoney, fmtMoneyMap } from '../lib/format'
import { CHARGE_STATE, INSPECTION_KIND, PRIORITY, REQ_STATUS, names, placeName } from '../lib/rentals'
import type { DueItem, Suggestion, TodayView } from '../lib/types'
import { openCreate } from '../components/Layout'
import { Pill } from '../components/Bits'
import { AppOff } from '../components/Apps'
import { Icon } from '../components/icons'
import { Card, Loading, Spinner, errMsg, useAsync, useToast } from '../components/ui'

export default function Today() {
  const { settings, user } = useApp()
  const { summary } = useData()
  const { data, reload } = useAsync(() => api.get<TodayView>(`${V1}/today`), [])
  useOnChange(useCallback(() => { reload() }, [reload]))
  if (!data) return <Loading />
  const cur = settings?.currency || 'EUR'
  if (data.empty) return <Welcome name={user?.name} />
  const hello = (() => { const h = new Date().getHours(); return h < 12 ? 'Good morning' : h < 18 ? 'Good afternoon' : 'Good evening' })()
  const nothing = !data.late.length && !data.due.length && !data.ending.length && !data.repairs.length && !data.inspections.length
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">{hello}{user?.name ? `, ${user.name.split(' ')[0]}` : ''}</h1><p className="muted">{nothing ? 'Nothing needs you today.' : 'Here is what needs you.'}</p></div>
      </div>
      <div className="tiles">
        <Link to="/rent" className="tile static-link"><div className="tile-head"><span className="tile-ico"><Icon name="wallet" size={14} /></span>Rent this month</div>
          <div className="tile-val small-val">{fmtMoneyMap(data.month.received, cur)}</div><div className="muted small">of {fmtMoneyMap(data.month.expected, cur)} due</div></Link>
        <Link to="/rent" className={`tile ${data.late.length ? 'tile-bad' : ''}`}><div className="tile-head"><span className="tile-ico"><Icon name="alert" size={14} /></span>Owed now</div>
          <div className="tile-val small-val">{fmtMoneyMap(summary?.owed, cur)}</div><div className="muted small">{data.late.length ? `${data.late.length} late` : 'nothing late'}</div></Link>
        <Link to="/properties" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="key" size={14} /></span>Let</div>
          <div className="tile-val">{summary?.let ?? 0}<span className="tile-unit">of {summary?.units ?? 0}</span></div><div className="muted small">{summary?.vacant ? `${summary.vacant} empty` : 'all let'}</div></Link>
        <Link to="/repairs" className={`tile ${summary?.urgent ? 'tile-warn' : ''}`}><div className="tile-head"><span className="tile-ico"><Icon name="wrench" size={14} /></span>Repairs</div>
          <div className="tile-val">{summary?.repairs ?? 0}</div><div className="muted small">{summary?.urgent ? `${summary.urgent} urgent` : 'none urgent'}</div></Link>
      </div>
      {data.late.length > 0 && <DueList title="Late" icon="alert" items={data.late} tone="bad" />}
      <FoundInMoney onDone={reload} />
      {data.due.length > 0 && <DueList title="Due soon" icon="clock" items={data.due} />}
      {data.ending.length > 0 && <Card title={<span className="sec-title"><Icon name="key" size={15} />Leases ending</span>} flush>
        <ul className="list">{data.ending.map(l => (
          <li key={l.id}><Link to={`/leases/${l.id}`} className="list-row">
            <span className={`list-ico ${(l.days_left ?? 99) < 0 ? 'st-overdue' : (l.days_left ?? 99) <= 30 ? 'st-due' : 'st-ok'}`}><Icon name="calendar" size={15} /></span>
            <span className="row-main grow"><span className="row-title">{placeName(l.unit_name, l.property_name)}</span><span className="muted small ellipsis">{names(l.tenants) || 'No tenant'} · ends {fmtDate(l.end_date)}</span></span>
            <span className="muted small">{(l.days_left ?? 0) < 0 ? `${-(l.days_left ?? 0)} days ago` : (l.days_left ?? 0) === 0 ? 'today' : `in ${l.days_left} days`}</span>
          </Link></li>))}</ul>
      </Card>}
      <div className="two-grid">
        {data.repairs.length > 0 && <Card title={<span className="sec-title"><Icon name="wrench" size={15} />Repairs</span>} actions={<Link to="/repairs" className="btn ghost sm">All</Link>} flush>
          <ul className="list">{data.repairs.map(r => (
            <li key={r.id}><Link to={`/repairs/${r.id}`} className="list-row">
              <span className={`list-ico ${r.priority === 'urgent' ? 'st-overdue' : 'st-ok'}`}><Icon name={REQ_STATUS[r.status].icon} size={15} /></span>
              <span className="row-main grow"><span className="row-title">{r.title}</span><span className="muted small ellipsis">{placeName(r.unit_name, r.property_name)}{r.planned_on ? ` · ${fmtDate(r.planned_on)}` : ''}</span></span>
              {r.priority === 'urgent' ? <Pill cls={PRIORITY.urgent.cls} icon="alert">Urgent</Pill> : <Pill cls={REQ_STATUS[r.status].cls}>{REQ_STATUS[r.status].label}</Pill>}
            </Link></li>))}</ul>
        </Card>}
        {(data.inspections.length > 0 || data.vacant.length > 0 || data.drafts.length > 0) && <div className="stack">
          {data.inspections.length > 0 && <Card title={<span className="sec-title"><Icon name="check-square" size={15} />Inspections coming</span>} flush>
            <ul className="list">{data.inspections.map(i => (
              <li key={i.id}><Link to={`/inspections/${i.id}`} className="list-row">
                <span className="list-ico"><Icon name="check-square" size={15} /></span>
                <span className="row-main grow"><span className="row-title">{INSPECTION_KIND[i.kind]}</span><span className="muted small ellipsis">{placeName(i.unit_name, i.property_name)}</span></span>
                <span className="muted small">{fmtDate(i.date)}{i.time ? `, ${i.time}` : ''}</span>
              </Link></li>))}</ul>
          </Card>}
          {data.vacant.length > 0 && <Card title={<span className="sec-title"><Icon name="door" size={15} />Empty</span>} flush>
            <ul className="list">{data.vacant.map(u => (
              <li key={u.id} className="list-row">
                <span className="list-ico"><Icon name="door" size={15} /></span>
                <Link to={`/properties/${u.property_id}`} className="row-main grow"><span className="row-title">{placeName(u.name, u.property_name || '')}</span><span className="muted small">{u.lease?.status === 'draft' ? 'A draft lease is waiting' : u.rent ? `Asking ${fmtMoney(u.rent, cur)}` : 'Not let'}</span></Link>
                {u.lease?.status === 'draft' ? <Link to={`/leases/${u.lease.id}`} className="btn sm">Open the draft</Link> : <button type="button" className="btn sm" onClick={() => openCreate('lease', { unit_id: u.id })}>Let it</button>}
              </li>))}</ul>
          </Card>}
        </div>}
      </div>
      {nothing && <div className="empty-day"><Icon name="check-circle" size={22} /><p>All paid, nothing to fix, no lease ending soon.</p></div>}
    </div>
  )
}

function DueList({ title, icon, items, tone }: { title: string; icon: string; items: DueItem[]; tone?: "bad" }) {
  return (
    <Card title={<span className={`sec-title ${tone || ''}`}><Icon name={icon} size={15} />{title}</span>} flush>
      <ul className="list">{items.map(d => {
        const st = CHARGE_STATE[d.state]
        const owed = d.amount - d.paid
        return (
          <li key={d.charge_id} className="list-row">
            <span className={`list-ico ${st.cls}`}><Icon name={st.icon} size={15} /></span>
            <Link to={`/leases/${d.lease_id}`} className="row-main grow">
              <span className="row-title">{names(d.tenants) || d.place}</span>
              <span className="muted small ellipsis">{d.place} · {d.title} · {d.state === 'late' ? `due ${fmtDate(d.due_date)}, ${d.days} days ago` : d.days === 0 ? 'due today' : d.days > 0 ? `due ${d.days} days ago` : `due ${fmtDate(d.due_date)}`}</span>
            </Link>
            <span className="row-amount tnum"><b>{fmtMoney(owed, d.currency)}</b>{d.paid > 0 && <span className="muted tiny block-line">of {fmtMoney(d.amount, d.currency)}</span>}</span>
            <button type="button" className="btn sm" onClick={() => openCreate('payment', { lease_id: d.lease_id, amount: owed })}><Icon name="check" size={14} /><span className="hide-sm">Paid</span></button>
          </li>
        )
      })}</ul>
    </Card>
  )
}

/** Rent already in the bank: incoming transactions in Pocket Money that look like a lease's rent. */
export function FoundInMoney({ onDone, lease }: { onDone?: () => void; lease?: number }) {
  const { settings } = useApp()
  const appState = useAppState()
  const money = appState('money', 'transactions:read')
  const { toast } = useToast()
  const [busy, setBusy] = useState('')
  const [gone, setGone] = useState<string[]>([])
  const { data } = useAsync(() => money.ok && settings?.money_match ? api.get<{ items: Suggestion[]; off?: string }>(`${V1}/money/suggestions${lease ? `?lease=${lease}` : ''}`).catch(() => null) : Promise.resolve(null), [money.ok, settings?.money_match, lease])
  if (!settings?.money_match) return null
  if (!money.ok) {
    if (money.loading || lease) return null
    return <AppOff slug="money" app={money.app} compact>Rent that came into your bank would show here, ready to tick off.</AppOff>
  }
  const items = (data?.items || []).filter(s => !gone.includes(s.tx_id))
  if (!items.length) return null
  const yes = async (s: Suggestion) => {
    setBusy(s.tx_id)
    try { await api.post(`${V1}/money/match`, { tx_id: s.tx_id, lease_id: s.lease_id }); setGone(g => [...g, s.tx_id]); announceChange(); onDone?.(); toast('Recorded') } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  return (
    <Card title={<span className="sec-title"><Icon name="wallet" size={15} />Found in Pocket Money</span>} sub="Money that came in and looks like rent. Is it?" flush>
      <ul className="list">{items.slice(0, 6).map(s => (
        <li key={s.tx_id} className="list-row">
          <span className="list-ico st-done"><Icon name="wallet" size={15} /></span>
          <span className="row-main grow"><span className="row-title">{fmtMoney(s.amount, s.currency)} on {fmtDate(s.date)}</span>
            <span className="muted small ellipsis">{s.payee || s.description} → {s.lease_title}</span><span className="muted tiny">Looks like {s.why}</span></span>
          {s.url && <a className="btn ghost sm hide-sm" href={s.url} target="_blank" rel="noreferrer" title="Open in Pocket Money"><Icon name="external" size={14} /></a>}
          <button type="button" className="btn sm" onClick={() => yes(s)} disabled={!!busy}>{busy === s.tx_id ? <Spinner /> : <Icon name="check" size={14} />}That’s it</button>
          <button type="button" className="btn ghost sm" onClick={() => setGone(g => [...g, s.tx_id])} title="Not this one">Not it</button>
        </li>))}</ul>
    </Card>
  )
}

function Welcome({ name }: { name?: string }) {
  return (
    <div className="page welcome">
      <img src="/favicon.svg" alt="" width={64} height={64} />
      <h1 className="page-title">Welcome{name ? `, ${name.split(' ')[0]}` : ''}</h1>
      <p className="muted">Pocket Rentals keeps the homes you rent out in one place: who lives where, the rent due and paid, deposits, repairs and inspections, and what each place earns and costs.</p>
      <div className="row wrap center">
        <button type="button" className="btn primary lg" onClick={() => openCreate('property')}><Icon name="plus" size={16} />Add your first place</button>
        <button type="button" className="btn lg" onClick={() => openCreate('import')}><Icon name="upload" size={16} />Bring it from a spreadsheet</button>
      </div>
      <ol className="welcome-steps">
        <li><b>Add a place</b> — a flat, a house, a building with flats or a garage.</li>
        <li><b>Make a lease</b> — who rents it, the rent and the deposit. Rent is then counted every month by itself.</li>
        <li><b>Tick off rent</b> as it comes in — or let Pocket Money find it in your bank.</li>
      </ol>
    </div>
  )
}
