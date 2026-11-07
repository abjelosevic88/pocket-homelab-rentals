// One lease: the rent owed and paid, the deposit, the tenants, signing,
// what happened, and the other apps it reaches.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useOnChange } from '../lib/data'
import { fmtDate, fmtMoney, todayISO } from '../lib/format'
import { CHARGE_STATE, INSPECTION_KIND, REQ_STATUS, dueWords, names, perWord, placeName, toCents, fromCents } from '../lib/rentals'
import type { Charge, Entry, FileRec, Inspection, Lease, Made, Payment, Request } from '../lib/types'
import { openCreate, useCrumb } from '../components/Layout'
import { MadeBox, Photos, Pill, Timeline } from '../components/Bits'
import { DocumentPicker, TemplatePicker } from '../components/Pickers'
import { ChargeForm, LeaseForm, MoneyField } from '../components/Forms'
import { FoundInMoney } from './Today'
import { Icon } from '../components/icons'
import { Card, Confirm, Field, Loading, MenuButton, Modal, Segmented, Spinner, Switch, errMsg, useAsync, useToast, type MenuItem } from '../components/ui'

interface View { lease: Lease; charges: Charge[]; payments: Payment[]; timeline: Entry[]; made: Made[]; requests: Request[]; inspections: Inspection[]; files: FileRec[]; signing_url: string }

export default function LeasePage() {
  const id = Number(useParams().id)
  const navigate = useNavigate()
  const { toast } = useToast()
  const { settings } = useApp()
  const appState = useAppState()
  const { data, reload, error } = useAsync(() => api.get<View>(`${V1}/leases/${id}`), [id])
  useOnChange(useCallback(() => { reload() }, [reload]))
  useEffect(() => { api.post(`${V1}/sync`).then(() => reload()).catch(() => {}) }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  const [modal, setModal] = useState<'edit' | 'charge' | 'end' | 'renew' | 'deposit' | 'email' | 'sign' | 'doc' | 'delete' | 'invoice' | null>(null)
  const [charge, setCharge] = useState<Charge | null>(null)
  const [showAll, setShowAll] = useState(false)
  useCrumb(data ? placeName(data.lease.unit_name, data.lease.property_name) : undefined)
  if (error) return <div className="page"><p className="bad">{error}</p></div>
  if (!data) return <Loading />
  const l = data.lease
  const make = async (body: Record<string, unknown>, ok: string) => {
    try { await api.post(`${V1}/make`, { owner: 'lease', owner_id: l.id, ...body }); reload(); toast(ok) } catch (e) { toast(errMsg(e), 'err') }
  }
  const t = todayISO()
  const charges = [...data.charges].reverse()
  const shown = showAll ? charges : charges.filter(c => c.state !== 'paid' && c.state !== 'waived' || c.due_date >= t.slice(0, 8) + '01').slice(0, 14)
  const hidden = charges.length - shown.length
  const active = l.status === 'active'
  const waive = async (c: Charge, v: boolean) => { try { await api.put(`${V1}/charges/${c.id}`, { waived: v }); reload(); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  const delPayment = async (p: Payment) => { try { await api.del(`${V1}/payments/${p.id}${p.money_tx_ref && !p.money_found ? '?money=1' : ''}`); reload(); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  const bookPayment = async (p: Payment) => { try { const r = await api.post<{ ok: boolean; error?: string; account?: string }>(`${V1}/payments/${p.id}/money`, {}); if (r.ok) toast(`Booked on ${r.account || 'your account'}`); else toast(r.error || 'Not booked', 'err'); reload() } catch (e) { toast(errMsg(e), 'err') } }
  const money = appState('money', 'transactions:write')
  const more: MenuItem[] = [
    { label: 'Change the lease', icon: 'edit', onSelect: () => setModal('edit') },
    { label: 'Add a charge (bills, a fee…)', icon: 'receipt', onSelect: () => setModal('charge') },
    ...(l.status === 'draft' ? [{ label: 'Start it now', icon: 'play', onSelect: async () => { try { await api.put(`${V1}/leases/${l.id}`, { status: 'active' }); reload(); announceChange() } catch (e) { toast(errMsg(e), 'err') } } }] : []),
    ...(l.status !== 'draft' ? [{ label: 'Renew it', icon: 'repeat', onSelect: () => setModal('renew') }] : []),
    ...(active ? [{ label: 'End the lease', icon: 'log-out', onSelect: () => setModal('end') }] : []),
    ...(l.deposit > 0 ? [{ label: 'Settle the deposit', icon: 'wallet', onSelect: () => setModal('deposit') }] : []),
    'sep',
    { label: 'Plan an inspection', icon: 'check-square', onSelect: () => openCreate('inspection', { property_id: l.property_id, unit_id: l.unit_id, lease_id: l.id, kind: l.status === 'ended' ? 'move_out' : 'routine' }) },
    { label: 'Report a repair', icon: 'wrench', onSelect: () => openCreate('repair', { property_id: l.property_id, unit_id: l.unit_id, lease_id: l.id }) },
    'sep',
    { label: 'Delete the lease', icon: 'trash', danger: true, onSelect: () => setModal('delete') },
  ]
  return (
    <div className="det-page">
      <div className="det-head">
        <span className="det-ico"><Icon name="key" size={22} /></span>
        <div className="grow">
          <div className="det-kicker">
            <Link to={`/properties/${l.property_id}`} className="small">{l.property_name}</Link>
            {l.status === 'draft' && <Pill cls="st-paused">Draft</Pill>}{l.status === 'ended' && <Pill cls="st-paused">Ended {fmtDate(l.ended_on || l.end_date)}</Pill>}
            {l.signing_state && <span className={`state st-${l.signing_state}`}>{l.signing_state === 'completed' ? 'signed' : l.signing_state}</span>}
          </div>
          <h1 className="det-title">{names(l.tenants) || 'No tenant yet'}</h1>
          <p className="muted">{placeName(l.unit_name, l.property_name)} · {fmtMoney(l.rent, l.currency)} {perWord(l.frequency)}, due {dueWords(l)}</p>
        </div>
        <div className="row det-tools">
          {l.status !== 'draft' && <button type="button" className="btn primary" onClick={() => openCreate('payment', { lease_id: l.id, amount: l.balance > 0 ? l.balance : l.rent })}><Icon name="check" size={15} />Rent came in</button>}
          {active && <button type="button" className="btn" onClick={() => setModal('email')}><Icon name="mail" size={15} /><span className="hide-sm">Remind</span></button>}
          <MenuButton label="More" className="btn icon-only" align="right" width={260} items={more}><Icon name="more" size={16} /></MenuButton>
        </div>
      </div>
      <div className={`balance-card ${l.overdue > 0 ? 'bad' : l.balance > 0 ? 'due' : 'ok'}`}>
        <span className="balance-ico"><Icon name={l.overdue > 0 ? 'alert' : l.balance > 0 ? 'clock' : 'check-circle'} size={20} /></span>
        <div className="grow">
          <div className="balance-l">{l.balance > 0 ? (l.overdue > 0 ? 'Owed, partly late' : 'Due now') : l.balance < 0 ? 'Paid ahead' : 'Paid up'}</div>
          <div className="balance-v tnum">{fmtMoney(Math.abs(l.balance), l.currency)}</div>
          {l.overdue > 0 && <div className="small">{fmtMoney(l.overdue, l.currency)} past the {l.grace_days} days</div>}
        </div>
        {l.next_due && <div className="balance-next"><span className="muted small">Next rent</span><b>{fmtDate(l.next_due)}</b></div>}
      </div>
      <FoundInMoney lease={l.id} onDone={reload} />
      <div className="det-grid">
        <div className="stack loose min0">
          <Card title="Rent and charges" actions={<button type="button" className="btn ghost sm" onClick={() => setModal('charge')}><Icon name="plus" size={14} />Charge</button>} flush>
            {charges.length === 0 ? <p className="muted pad">{l.status === 'draft' ? 'Rent starts when the lease does.' : 'Nothing charged yet.'}</p> : <ul className="list">{shown.map(c => {
              const st = CHARGE_STATE[c.state]
              return (
                <li key={c.id} className="list-row">
                  <span className={`list-ico ${st.cls}`}><Icon name={st.icon} size={15} /></span>
                  <span className="row-main grow"><span className="row-title">{c.title}</span><span className="muted small">due {fmtDate(c.due_date)}{c.state === 'paid' && c.paid_on ? ` · paid ${fmtDate(c.paid_on)}` : c.paid > 0 && c.state !== 'paid' ? ` · ${fmtMoney(c.paid, c.currency)} paid` : ''}{c.invoice_ref ? ' · invoiced' : ''}</span></span>
                  <Pill cls={st.cls}>{st.label}</Pill>
                  <span className="row-amount tnum">{fmtMoney(c.amount, c.currency)}</span>
                  <MenuButton label="Charge" className="btn ghost icon-only sm" align="right" width={230} items={[
                    ...(c.state !== 'paid' && c.state !== 'waived' ? [{ label: 'It was paid', icon: 'check', onSelect: () => openCreate('payment', { lease_id: l.id, amount: c.amount - c.paid }) }] : []),
                    { label: 'Change', icon: 'edit', onSelect: () => setCharge(c) },
                    c.waived ? { label: 'Charge it again', icon: 'undo', onSelect: () => waive(c, false) } : { label: 'Waive it (not owed)', icon: 'x', onSelect: () => waive(c, true) },
                    ...(!c.invoice_ref ? [{ label: 'A draft invoice for it', icon: 'receipt', disabled: !appState('invoicing', 'invoices:write').ok, onSelect: () => make({ target: 'invoice', charge_id: c.id }, 'Draft invoice made in Pocket Invoicing') }] : []),
                    ...(c.kind !== 'rent' || !c.period ? ['sep' as const, { label: 'Delete', icon: 'trash', danger: true, onSelect: async () => { try { await api.del(`${V1}/charges/${c.id}`); reload() } catch (e) { toast(errMsg(e), 'err') } } }] : []),
                  ]}><Icon name="more" size={14} /></MenuButton>
                </li>
              )
            })}</ul>}
            {hidden > 0 && <div className="pad"><button type="button" className="btn ghost sm" onClick={() => setShowAll(true)}>Show {hidden} earlier, all paid</button></div>}
          </Card>
          <Card title="Payments" flush>
            {data.payments.length === 0 ? <p className="muted pad">No payments yet.</p> : <ul className="list">{[...data.payments].reverse().slice(0, showAll ? 500 : 12).map(p => (
              <li key={p.id} className="list-row">
                <span className="list-ico st-done"><Icon name="wallet" size={15} /></span>
                <span className="row-main grow"><span className="row-title">{fmtDate(p.date)}</span><span className="muted small ellipsis">{p.method}{p.note ? ` · ${p.note}` : ''}{p.source_app ? ` · from Pocket ${p.source_app}` : ''}</span></span>
                {p.money_url ? <a href={p.money_url} target="_blank" rel="noreferrer" className="pill st-ok" title={p.money_found ? 'Found in Pocket Money' : 'Booked in Pocket Money'}><Icon name="wallet" size={11} />Money</a> : null}
                <span className="row-amount tnum"><b>{fmtMoney(p.amount, p.currency)}</b></span>
                <MenuButton label="Payment" className="btn ghost icon-only sm" align="right" items={[
                  ...(!p.money_tx_ref ? [{ label: 'Book it in Pocket Money', icon: 'wallet', disabled: !money.ok, onSelect: () => bookPayment(p) }] : []),
                  { label: p.money_tx_ref && !p.money_found ? 'Delete (here and in Pocket Money)' : 'Delete', icon: 'trash', danger: true, onSelect: () => delPayment(p) },
                ]}><Icon name="more" size={14} /></MenuButton>
              </li>))}</ul>}
          </Card>
          <Card title="What happened"><Timeline entries={data.timeline} onAdd={async e => { await api.post(`${V1}/entries`, { ...e, lease_id: l.id }); reload() }} onDelete={async eid => { await api.del(`${V1}/entries/${eid}`); reload() }} /></Card>
        </div>
        <aside className="stack">
          <div className="side-card card">
            <h4 className="side-h">Tenants</h4>
            {l.tenants.length === 0 ? <p className="muted small m0">Nobody yet. <button type="button" className="link-btn" onClick={() => setModal('edit')}>Add them</button></p> : l.tenants.map(tn => (
              <div key={tn.id} className="tenant-card">
                <Link to={`/tenants/${tn.id}`} className="who-row"><Icon name="user" size={14} /><b className="ellipsis">{tn.name}</b>{tn.contact_ref && <Icon name="link" size={11} />}</Link>
                {tn.email && <a className="who-row small" href={`mailto:${tn.email}`}><Icon name="mail" size={13} /><span className="ellipsis">{tn.email}</span></a>}
                {tn.phone && <a className="who-row small" href={`tel:${tn.phone.replace(/\s/g, '')}`}><Icon name="phone" size={13} />{tn.phone}</a>}
              </div>
            ))}
          </div>
          <div className="side-card card">
            <h4 className="side-h">Terms</h4>
            <dl className="facts">
              <dt>Rent</dt><dd>{fmtMoney(l.rent, l.currency)} {perWord(l.frequency)}</dd>
              <dt>Due</dt><dd>{dueWords(l)}, late after {l.grace_days} days</dd>
              <dt>From</dt><dd>{fmtDate(l.start_date)}</dd>
              <dt>To</dt><dd>{l.end_date ? <>{fmtDate(l.end_date)}{l.days_left !== undefined && active && <span className="muted"> · {l.days_left >= 0 ? `in ${l.days_left} days` : `${-l.days_left} days ago`}</span>}</> : 'until notice'}</dd>
              <dt>Notice</dt><dd>{l.notice_days} days</dd>
              {l.late_fee > 0 && <><dt>Late fee</dt><dd>{fmtMoney(l.late_fee, l.currency)}</dd></>}
              {l.charges_from && l.charges_from > l.start_date && <><dt>Counted from</dt><dd>{fmtDate(l.charges_from)}</dd></>}
            </dl>
            {l.notes && <p className="pre small m0 muted">{l.notes}</p>}
          </div>
          {l.deposit > 0 && <div className="side-card card">
            <h4 className="side-h">Deposit</h4>
            <div className="row"><b className="grow tnum">{fmtMoney(l.deposit, l.currency)}</b><Pill cls={l.deposit_state === 'held' ? 'st-done' : l.deposit_state === 'due' ? 'st-due' : 'st-paused'}>{{ none: '—', due: 'Not paid yet', held: 'You hold it', returned: 'Given back', part_returned: 'Partly given back' }[l.deposit_state]}</Pill></div>
            {(l.deposit_state === 'returned' || l.deposit_state === 'part_returned') && <p className="small m0">Given back {fmtMoney(l.deposit_back, l.currency)}{l.deposit - l.deposit_back > 0 ? `, kept ${fmtMoney(l.deposit - l.deposit_back, l.currency)}` : ''}.{l.deposit_note && <span className="muted"> {l.deposit_note}</span>}</p>}
            {l.deposit_state === 'held' && l.status === 'ended' && <button type="button" className="btn sm self-start" onClick={() => setModal('deposit')}>Settle it</button>}
          </div>}
          <MadeBox made={data.made} onForget={async mid => { await api.del(`${V1}/made/${mid}`); reload() }} actions={[
            { target: 'signing', label: l.signing_ref ? 'Sign again' : 'Send to sign', icon: 'pen', app: 'sign', scope: 'signing:write', run: () => setModal('sign') },
            { target: 'email', label: 'Email them', icon: 'mail', app: 'mail', scope: 'mail:send', run: () => setModal('email') },
            { target: 'task', label: 'Renewal task', icon: 'check-square', app: 'tasks', scope: 'tasks:write', run: () => make({ target: 'task' }, 'Task made in Pocket Tasks'), hide: !l.end_date || !active },
            { target: 'note', label: 'As a note', icon: 'note', app: 'notes', scope: 'notes:write', run: () => make({ target: 'note' }, 'Saved in Pocket Notes') },
            { target: 'document', label: 'Attach a paper', icon: 'file', app: 'documents', scope: 'documents:read', run: () => setModal('doc') },
          ]} />
          {!appState('calendar', 'calendar:write').ok && l.end_date && active && <a className="btn sm" href={`${V1}/ics/lease/${l.id}`}><Icon name="calendar" size={14} />The end day as a calendar file</a>}
          {data.signing_url && <a className="btn sm" href={data.signing_url} target="_blank" rel="noreferrer"><Icon name="pen" size={14} />Open it in Pocket Sign</a>}
          {(data.requests.length > 0 || data.inspections.length > 0) && <div className="side-card card">
            <h4 className="side-h">Repairs and inspections</h4>
            <ul className="mini-list">
              {data.requests.slice(0, 5).map(r => <li key={`r${r.id}`}><Link to={`/repairs/${r.id}`}><Icon name="wrench" size={13} />{r.title}</Link><span className="muted tiny">{REQ_STATUS[r.status].label}</span></li>)}
              {data.inspections.slice(0, 4).map(i => <li key={`i${i.id}`}><Link to={`/inspections/${i.id}`}><Icon name="check-square" size={13} />{INSPECTION_KIND[i.kind]}</Link><span className="muted tiny">{fmtDate(i.date)}</span></li>)}
            </ul>
          </div>}
          <div className="side-card card"><h4 className="side-h">The lease and papers</h4><Photos owner="lease" ownerId={l.id} files={data.files} onChange={reload} /></div>
        </aside>
      </div>
      {modal === 'edit' && <LeaseForm lease={l} onClose={() => setModal(null)} onSaved={reload} />}
      {(modal === 'charge' || charge) && <ChargeForm lease={l} charge={charge || undefined} onClose={() => { setModal(null); setCharge(null) }} onSaved={reload} />}
      {modal === 'end' && <EndLease lease={l} onClose={() => setModal(null)} onDone={() => { setModal(null); reload(); announceChange() }} />}
      {modal === 'renew' && <RenewLease lease={l} onClose={() => setModal(null)} onDone={() => { setModal(null); reload(); announceChange() }} />}
      {modal === 'deposit' && <DepositModal lease={l} onClose={() => setModal(null)} onDone={() => { setModal(null); reload() }} />}
      {modal === 'email' && <EmailModal lease={l} aiOn={!!settings?.ai_help} onClose={() => setModal(null)} onSent={() => { setModal(null); reload() }} />}
      {modal === 'sign' && <TemplatePicker onClose={() => setModal(null)} onPick={(tid, send) => { setModal(null); make({ target: 'signing', template_id: tid, send }, send ? 'Sent to sign' : 'Ready in Pocket Sign') }} />}
      {modal === 'doc' && <DocumentPicker onClose={() => setModal(null)} onPick={ref => { setModal(null); make({ target: 'document', ref }, 'Attached') }} />}
      {modal === 'delete' && <Confirm title="Delete this lease?" message="Its rent, payments and timeline are deleted here. To keep the history, end it instead." confirmLabel="Delete" onCancel={() => setModal(null)} onConfirm={async () => { try { await api.del(`${V1}/leases/${l.id}`); announceChange(); navigate('/leases') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

function EndLease({ lease, onClose, onDone }: { lease: Lease; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [on, setOn] = useState(lease.end_date && lease.end_date < todayISO() ? lease.end_date : todayISO())
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const go = async () => { setBusy(true); try { await api.post(`${V1}/leases/${lease.id}/end`, { on, reason }); toast('Lease ended'); onDone() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) } }
  return (
    <Modal title="End the lease" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={go} disabled={busy}>{busy ? <Spinner /> : <Icon name="check" size={15} />}End it</button></>}>
      <div className="stack">
        <p className="muted small">No rent is counted after this day; the last part is charged by the day. The unit shows as empty again. Then plan the moving-out inspection and settle the deposit.</p>
        <Field label="Last day"><input type="date" value={on} onChange={e => setOn(e.target.value)} /></Field>
        <Field label="Why (for you)"><input value={reason} onChange={e => setReason(e.target.value)} placeholder="Moved for work" /></Field>
      </div>
    </Modal>
  )
}

function RenewLease({ lease, onClose, onDone }: { lease: Lease; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const base = lease.end_date && lease.end_date > todayISO() ? lease.end_date : todayISO()
  const next = (() => { const d = new Date(base); d.setFullYear(d.getFullYear() + 1); return d.toISOString().slice(0, 10) })()
  const [end, setEnd] = useState(next)
  const [open, setOpen] = useState(false)
  const [rent, setRent] = useState(fromCents(lease.rent))
  const [busy, setBusy] = useState(false)
  const go = async () => { setBusy(true); try { await api.post(`${V1}/leases/${lease.id}/renew`, { end_date: open ? '' : end, rent: toCents(rent) }); toast('Renewed'); onDone() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) } }
  return (
    <Modal title="Renew the lease" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={go} disabled={busy}>{busy ? <Spinner /> : <Icon name="repeat" size={15} />}Renew</button></>}>
      <div className="stack">
        <Field label="Until" group><div className="stack tight"><input type="date" value={open ? '' : end} onChange={e => setEnd(e.target.value)} disabled={open} aria-label="Until" />
          <label className="check-row small"><input type="checkbox" checked={open} onChange={e => setOpen(e.target.checked)} />Until someone gives notice</label></div></Field>
        <MoneyField label="Rent from now on" value={rent} onChange={setRent} currency={lease.currency} help="A new rent counts from the next rent that is not made yet." />
      </div>
    </Modal>
  )
}

function DepositModal({ lease, onClose, onDone }: { lease: Lease; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const appState = useAppState()
  const money = appState('money', 'transactions:write')
  const [state, setState] = useState<'returned' | 'part_returned' | 'held'>(lease.deposit_state === 'part_returned' ? 'part_returned' : 'returned')
  const [back, setBack] = useState(fromCents(lease.deposit_back || lease.deposit))
  const [note, setNote] = useState(lease.deposit_note)
  const [book, setBook] = useState(false)
  const [busy, setBusy] = useState(false)
  const go = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ money?: { ok: boolean; error?: string } }>(`${V1}/leases/${lease.id}/deposit`, { state, back: state === 'returned' ? lease.deposit : toCents(back), note, money: book && money.ok })
      if (r.money && !r.money.ok) toast(`Saved; Pocket Money: ${r.money.error}`, 'err'); else toast('Saved')
      onDone()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Settle the deposit" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={go} disabled={busy}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button></>}>
      <div className="stack">
        <p className="muted small">The deposit is {fmtMoney(lease.deposit, lease.currency)}. Compare with the moving-in inspection before you decide what to keep.</p>
        <Segmented size="sm" value={state} onChange={setState} options={[{ value: 'returned', label: 'All back' }, { value: 'part_returned', label: 'Part back' }, { value: 'held', label: 'Still holding it' }]} />
        {state === 'part_returned' && <MoneyField label="Given back" value={back} onChange={setBack} currency={lease.currency} />}
        {state !== 'held' && <Field label="Why something was kept"><textarea rows={2} value={note} onChange={e => setNote(e.target.value)} placeholder="Repainting the bedroom walls, one key missing" /></Field>}
        {state !== 'held' && <label className={`check-row ${money.ok ? '' : 'disabled'}`}><Switch checked={book && money.ok} onChange={setBook} label="In Pocket Money" disabled={!money.ok} /><span>Book the money given back in Pocket Money</span></label>}
      </div>
    </Modal>
  )
}

function EmailModal({ lease, aiOn, onClose, onSent }: { lease: Lease; aiOn: boolean; onClose: () => void; onSent: () => void }) {
  const { toast } = useToast()
  const appState = useAppState()
  const mail = appState('mail', 'mail:send')
  const [f, setF] = useState({ subject: '', body: '', to: '' })
  const [busy, setBusy] = useState('')
  const emails = lease.tenants.map(t => t.email).filter(Boolean)
  useEffect(() => { api.get<{ subject: string; body: string; to: string[] }>(`${V1}/leases/${lease.id}/reminder`).then(r => setF(x => ({ ...x, subject: r.subject, body: r.body }))).catch(() => {}) }, [lease.id])
  const ai = async (want: string) => {
    setBusy(want)
    try { const r = await api.post<{ subject: string; body: string }>(`${V1}/ai`, { want, lease_id: lease.id }); setF(x => ({ ...x, subject: r.subject, body: r.body })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const send = async () => {
    setBusy('send')
    try { await api.post(`${V1}/make`, { owner: 'lease', owner_id: lease.id, target: 'email', subject: f.subject, body: f.body, to: f.to }); toast('Sent'); onSent() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const mailto = `mailto:${encodeURIComponent(f.to || emails.join(','))}?subject=${encodeURIComponent(f.subject)}&body=${encodeURIComponent(f.body)}`
  return (
    <Modal title="Write to the tenants" onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      {mail.ok ? <button type="button" className="btn primary" onClick={send} disabled={!!busy || !f.body.trim() || (!emails.length && !f.to)}>{busy === 'send' ? <Spinner /> : <Icon name="send" size={15} />}Send from Pocket Mail</button>
        : <a className="btn primary" href={mailto} onClick={onClose}><Icon name="mail" size={15} />Open in your email app</a>}
    </>}>
      <div className="stack">
        {emails.length === 0 && <p className="bad small">The tenants have no email address here. Add one, or write it below.</p>}
        <Field label="To" help={emails.length ? `Empty: ${emails.join(', ')}` : undefined}><input value={f.to} onChange={e => setF({ ...f, to: e.target.value })} placeholder={emails.join(', ') || 'name@example.com'} /></Field>
        <Field label="Subject"><input value={f.subject} onChange={e => setF({ ...f, subject: e.target.value })} /></Field>
        <Field label="Message"><textarea rows={9} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} /></Field>
        {aiOn && <div className="row wrap">
          <span className="muted small">Pocket AI writes:</span>
          <button type="button" className="btn sm" onClick={() => ai('reminder')} disabled={!!busy}>{busy === 'reminder' ? <Spinner /> : <Icon name="sparkles" size={13} />}A friendly reminder</button>
          {lease.overdue > 0 && <button type="button" className="btn sm" onClick={() => ai('late')} disabled={!!busy}>{busy === 'late' ? <Spinner /> : <Icon name="sparkles" size={13} />}The rent is late</button>}
          {lease.end_date && <button type="button" className="btn sm" onClick={() => ai('ending')} disabled={!!busy}>{busy === 'ending' ? <Spinner /> : <Icon name="sparkles" size={13} />}The lease ends</button>}
        </div>}
        {!mail.ok && !mail.loading && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {mail.app?.state === 'missing' ? 'Pocket Mail is not on this box: the message opens in your own email app.' : 'Connect Pocket Mail to send from your own mailbox here; until then it opens in your email app.'}</p>}
      </div>
    </Modal>
  )
}
