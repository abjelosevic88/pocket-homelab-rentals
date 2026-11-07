// One place: its units and who rents them, this year's money, repairs,
// inspections, costs and what happened.
import { useCallback, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useOnChange } from '../lib/data'
import { fmtDate, fmtMoney } from '../lib/format'
import { INSPECTION_KIND, KINDS, PRIORITY, REQ_STATUS, expenseLabel, kindIcon, names } from '../lib/rentals'
import type { Entry, Expense, FileRec, Inspection, Lease, Made, Property, Request, Unit } from '../lib/types'
import { openCreate, useCrumb } from '../components/Layout'
import { MadeBox, Photos, Pill, Timeline } from '../components/Bits'
import { DocumentPicker } from '../components/Pickers'
import { ExpenseForm, PropertyForm, UnitForm } from '../components/Forms'
import { Icon } from '../components/icons'
import { Card, Confirm, Loading, MenuButton, errMsg, useAsync, useToast } from '../components/ui'

interface View { property: Property; leases: Lease[]; requests: Request[]; inspections: Inspection[]; made: Made[]; files: FileRec[]; year: { year: string; income: number; expenses: number; net: number; currency: string }; home_url: string }

export default function PropertyPage() {
  const id = Number(useParams().id)
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data, reload, error } = useAsync(() => api.get<View>(`${V1}/properties/${id}`), [id])
  const { data: entries, reload: reloadTl } = useAsync(() => api.get<Entry[]>(`${V1}/entries?property=${id}&limit=60`), [id])
  const { data: costs, reload: reloadCosts } = useAsync(() => api.get<Expense[]>(`${V1}/expenses?property=${id}`), [id])
  useOnChange(useCallback(() => { reload(); reloadTl(); reloadCosts() }, [reload, reloadTl, reloadCosts]))
  const [edit, setEdit] = useState(false)
  const [unit, setUnit] = useState<Unit | 'new' | null>(null)
  const [cost, setCost] = useState<Expense | null>(null)
  const [del, setDel] = useState(false)
  const [docs, setDocs] = useState(false)
  useCrumb(data?.property.name)
  if (error) return <div className="page"><p className="bad">{error}</p></div>
  if (!data) return <Loading />
  const p = data.property
  const y = data.year
  const open = data.requests.filter(r => r.status !== 'done' && r.status !== 'cancelled')
  const done = data.requests.filter(r => r.status === 'done' || r.status === 'cancelled').slice(0, 5)
  const past = data.leases.filter(l => l.status === 'ended')
  const archive = async (v: boolean) => { try { await api.put(`${V1}/properties/${p.id}`, { archived: v }); announceChange(); toast(v ? 'Archived' : 'Back in your places') } catch (e) { toast(errMsg(e), 'err') } }
  const remove = async () => { try { await api.del(`${V1}/properties/${p.id}`); announceChange(); navigate('/properties') } catch (e) { toast(errMsg(e), 'err') } }
  const attach = async (ref: string) => { try { await api.post(`${V1}/make`, { owner: 'property', owner_id: p.id, target: 'document', ref }); reload() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <div className="det-page">
      <div className="det-head">
        <span className="det-ico" style={{ color: p.color || undefined }}><Icon name={kindIcon(p.kind)} size={22} /></span>
        <div className="grow">
          <div className="det-kicker"><span className="muted small">{KINDS.find(k => k.value === p.kind)?.label}</span>{p.archived && <span className="badge">archived</span>}</div>
          <h1 className="det-title">{p.name}</h1>
          <p className="muted">{[p.address, p.city, p.country].filter(Boolean).join(', ')}</p>
        </div>
        <div className="row det-tools">
          <button type="button" className="btn" onClick={() => openCreate('repair', { property_id: p.id })}><Icon name="wrench" size={15} />Repair</button>
          <button type="button" className="btn" onClick={() => setCost({} as Expense)}><Icon name="receipt" size={15} />Cost</button>
          <MenuButton label="More" className="btn icon-only" align="right" items={[
            { label: 'Change the place', icon: 'edit', onSelect: () => setEdit(true) },
            { label: 'Add a unit', icon: 'plus', onSelect: () => setUnit('new') },
            { label: 'Plan an inspection', icon: 'check-square', onSelect: () => openCreate('inspection', { property_id: p.id }) },
            'sep',
            p.archived ? { label: 'Back in your places', icon: 'undo', onSelect: () => archive(false) } : { label: 'Archive (not let any more)', icon: 'archive', onSelect: () => archive(true) },
            { label: 'Delete with everything in it', icon: 'trash', danger: true, onSelect: () => setDel(true) },
          ]}><Icon name="more" size={16} /></MenuButton>
        </div>
      </div>
      <div className="stats-row">
        <div className="stat"><span className="stat-l">Rent in, {y.year}</span><span className="stat-v tnum">{fmtMoney(y.income, y.currency)}</span></div>
        <div className="stat"><span className="stat-l">Costs, {y.year}</span><span className="stat-v tnum">{fmtMoney(y.expenses, y.currency)}</span></div>
        <div className="stat"><span className="stat-l">Left over</span><span className={`stat-v tnum ${y.net < 0 ? 'bad' : 'ok'}`}>{fmtMoney(y.net, y.currency)}</span></div>
        <div className="stat"><span className="stat-l">Let</span><span className="stat-v tnum">{p.let} of {p.unit_count}</span></div>
      </div>
      <div className="det-grid">
        <div className="stack loose min0">
          <Card title="Units" actions={<button type="button" className="btn ghost sm" onClick={() => setUnit('new')}><Icon name="plus" size={14} />Add</button>} flush>
            <ul className="list">{(p.units || []).map(u => {
              const l = u.lease
              return (
                <li key={u.id} className="list-row">
                  <span className={`list-ico ${l?.status === 'active' ? (l.overdue > 0 ? 'st-overdue' : 'st-done') : ''}`}><Icon name={l?.status === 'active' ? 'key' : 'door'} size={15} /></span>
                  <span className="row-main grow">
                    <span className="row-title">{u.name}{u.bedrooms > 0 && <span className="muted small"> · {u.bedrooms} bed</span>}{u.area_m2 > 0 && <span className="muted small"> · {u.area_m2} m²</span>}</span>
                    {l?.status === 'active' ? <Link to={`/leases/${l.id}`} className="small ellipsis">{names(l.tenants) || 'Let'} · {fmtMoney(l.rent, l.currency)}{l.balance > 0 ? ` · owes ${fmtMoney(l.balance, l.currency)}` : ''}{l.end_date ? ` · to ${fmtDate(l.end_date)}` : ''}</Link>
                      : <span className="muted small">{l?.status === 'draft' ? <Link to={`/leases/${l.id}`}>A draft lease</Link> : u.rent ? `Empty · asking ${fmtMoney(u.rent, p.currency)}` : 'Empty'}</span>}
                  </span>
                  {(!l || l.status !== 'active') && <button type="button" className="btn sm" onClick={() => openCreate('lease', { unit_id: u.id })}>Let it</button>}
                  <button type="button" className="btn ghost icon-only sm" aria-label={`Change ${u.name}`} onClick={() => setUnit(u)}><Icon name="edit" size={14} /></button>
                </li>
              )
            })}
            {(p.units || []).length === 0 && <li className="list-row muted">No units yet. <button type="button" className="link-btn" onClick={() => setUnit('new')}>Add one</button></li>}
            </ul>
          </Card>
          <Card title="Repairs" actions={<button type="button" className="btn ghost sm" onClick={() => openCreate('repair', { property_id: p.id })}><Icon name="plus" size={14} />Report</button>} flush>
            {open.length + done.length === 0 ? <p className="muted pad">No repairs.</p> : <ul className="list">{[...open, ...done].map(r => (
              <li key={r.id}><Link to={`/repairs/${r.id}`} className="list-row">
                <span className={`list-ico ${r.priority === 'urgent' && r.status !== 'done' ? 'st-overdue' : r.status === 'done' ? 'st-done' : ''}`}><Icon name={REQ_STATUS[r.status].icon} size={15} /></span>
                <span className="row-main grow"><span className="row-title">{r.title}</span><span className="muted small ellipsis">{r.unit_name || 'Whole place'} · {r.status === 'done' ? `done ${fmtDate(r.done_on)}` : `reported ${fmtDate(r.reported_on)}`}</span></span>
                {r.priority === 'urgent' && r.status !== 'done' ? <Pill cls={PRIORITY.urgent.cls}>Urgent</Pill> : <Pill cls={REQ_STATUS[r.status].cls}>{REQ_STATUS[r.status].label}</Pill>}
              </Link></li>))}</ul>}
          </Card>
          <Card title="Costs" actions={<button type="button" className="btn ghost sm" onClick={() => setCost({} as Expense)}><Icon name="plus" size={14} />Add</button>} flush>
            {!costs ? <Loading /> : costs.length === 0 ? <p className="muted pad">No costs yet.</p> : <ul className="list">{costs.slice(0, 12).map(e => (
              <li key={e.id} className="list-row clickable" onClick={() => setCost(e)}>
                <span className="list-ico"><Icon name="receipt" size={15} /></span>
                <span className="row-main grow"><span className="row-title">{expenseLabel(e.category)}{e.note && <span className="muted small"> · {e.note}</span>}</span><span className="muted small">{fmtDate(e.date)}{e.payee ? ` · ${e.payee}` : ''}</span></span>
                {e.money_url && <a href={e.money_url} target="_blank" rel="noreferrer" className="muted" title="In Pocket Money" onClick={ev => ev.stopPropagation()}><Icon name="wallet" size={14} /></a>}
                <span className="row-amount tnum">{fmtMoney(e.amount, e.currency)}</span>
              </li>))}</ul>}
            {costs && costs.length > 12 && <div className="pad"><Link to={`/costs?property=${p.id}`} className="btn ghost sm">All {costs.length} costs</Link></div>}
          </Card>
          <Card title="What happened"><Timeline entries={entries || []} onAdd={async e => { await api.post(`${V1}/entries`, { ...e, property_id: p.id }); reloadTl() }} onDelete={async id => { await api.del(`${V1}/entries/${id}`); reloadTl() }} /></Card>
        </div>
        <aside className="stack">
          {p.notes && <div className="side-card card"><h4 className="side-h">Notes</h4><p className="pre small m0">{p.notes}</p></div>}
          <div className="side-card card">
            <h4 className="side-h">Inspections</h4>
            {data.inspections.length === 0 ? <p className="muted small m0">None yet.</p> : <ul className="mini-list">{data.inspections.slice(0, 6).map(i => (
              <li key={i.id}><Link to={`/inspections/${i.id}`}><Icon name="check-square" size={13} />{INSPECTION_KIND[i.kind]}{i.unit_name ? ` · ${i.unit_name}` : ''}</Link><span className="muted tiny">{fmtDate(i.date)}{i.status === 'planned' ? ' · planned' : i.issues ? ` · ${i.issues} to look at` : ''}</span></li>))}</ul>}
            <button type="button" className="btn sm self-start" onClick={() => openCreate('inspection', { property_id: p.id })}><Icon name="plus" size={14} />Plan one</button>
          </div>
          {past.length > 0 && <div className="side-card card"><h4 className="side-h">Earlier tenants</h4><ul className="mini-list">{past.slice(0, 6).map(l => (
            <li key={l.id}><Link to={`/leases/${l.id}`}><Icon name="user" size={13} />{names(l.tenants) || 'Someone'}</Link><span className="muted tiny">{l.unit_name} · {fmtDate(l.start_date)} – {fmtDate(l.ended_on || l.end_date)}</span></li>))}</ul></div>}
          <div className="side-card card">
            <h4 className="side-h">Facts</h4>
            <dl className="facts">
              {p.value > 0 && <><dt>Worth</dt><dd>{fmtMoney(p.value, p.currency)}</dd></>}
              {p.bought_for > 0 && <><dt>Bought for</dt><dd>{fmtMoney(p.bought_for, p.currency)}{p.bought_on ? `, ${fmtDate(p.bought_on)}` : ''}</dd></>}
              {p.money_tag && <><dt>Money tag</dt><dd>#{p.money_tag}</dd></>}
              <dt>Pocket Home</dt><dd>{data.home_url ? <a href={data.home_url} target="_blank" rel="noreferrer">Open its page <Icon name="external" size={11} /></a> : p.home_ref ? 'Linked' : <button type="button" className="link-btn" onClick={() => setEdit(true)}>Not linked</button>}</dd>
            </dl>
          </div>
          <MadeBox made={data.made} onForget={async id => { await api.del(`${V1}/made/${id}`); reload() }} actions={[
            { target: 'document', label: 'Attach a paper', icon: 'file', app: 'documents', scope: 'documents:read', run: () => setDocs(true) },
            { target: 'note', label: 'As a note', icon: 'note', app: 'notes', scope: 'notes:write', run: async () => { try { await api.post(`${V1}/make`, { owner: 'property', owner_id: p.id, target: 'note' }); reload(); toast('Saved in Pocket Notes') } catch (e) { toast(errMsg(e), 'err') } } },
          ]} />
          <div className="side-card card"><h4 className="side-h">Photos and papers</h4><Photos owner="property" ownerId={p.id} files={data.files} onChange={reload} /></div>
        </aside>
      </div>
      {edit && <PropertyForm property={p} onClose={() => setEdit(false)} onSaved={reload} />}
      {unit && <UnitForm unit={unit === 'new' ? undefined : unit} propertyId={p.id} currency={p.currency} onClose={() => setUnit(null)} onSaved={reload} />}
      {cost && <ExpenseForm expense={cost.id ? cost : undefined} initial={{ property_id: p.id }} onClose={() => setCost(null)} onSaved={() => { reloadCosts(); reload() }} />}
      {docs && <DocumentPicker onPick={ref => { setDocs(false); attach(ref) }} onClose={() => setDocs(false)} />}
      {del && <Confirm title={`Delete ${p.name}?`} message="Its units, leases, rent, payments, repairs, costs, inspections and photos are deleted here (nothing is removed in your other apps). Archive it instead to keep the history." confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={remove} />}
    </div>
  )
}
