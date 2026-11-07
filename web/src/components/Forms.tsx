// The forms: a place and its units, a lease with its tenants, rent that
// came in, a charge, a repair, a cost and an inspection. Each one works on
// its own; what another Pocket app adds (contacts, accounts, tradespeople,
// a task) is offered when that app is connected and greyed when it is not.
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { CHARGE_KINDS, COLORS, CURRENCIES, EXPENSE_KINDS, FREQ, INSPECTION_KIND, KINDS, WEEKDAYS, fromCents, names, placeName, toCents } from '../lib/rentals'
import { announceChange, useAppState, useData } from '../lib/data'
import { fmtDate, fmtMoney, todayISO } from '../lib/format'
import type { Charge, Expense, Inspection, Lease, PickedContact, Property, Request, Suggestion, Tenant, Unit } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { CoreHint, Field, Modal, Segmented, Spinner, Switch, errMsg, useAsync, useDebounce, useToast } from './ui'

type Init = Record<string, unknown>
const num = (v: unknown) => (typeof v === 'number' ? v : Number(v) || 0)

/** An amount with its currency. */
export function MoneyField({ label, value, onChange, currency, onCurrency, help, placeholder = '0' }: { label: string; value: string; onChange: (v: string) => void; currency: string; onCurrency?: (c: string) => void; help?: string; placeholder?: string }) {
  return (
    <Field label={label} help={help} group>
      <div className="input-group">
        <input inputMode="decimal" value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder} aria-label={label} />
        {onCurrency ? <Select ariaLabel="Currency" value={currency} onChange={onCurrency} options={Array.from(new Set([currency, ...CURRENCIES])).map(c => ({ value: c, label: c }))} />
          : <span className="input-suffix">{currency}</span>}
      </div>
    </Field>
  )
}

function useUnits() {
  return useAsync(() => api.get<Unit[]>(`${V1}/units`), [])
}

const unitLabel = (u: Unit) => placeName(u.name, u.property_name || '')

/** What a Money switch says when Pocket Money is not there. */
function MoneyOff({ ok, state }: { ok: boolean; state?: string }) {
  if (ok) return null
  return <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {state === 'missing' ? 'Pocket Money is not on this box.' : 'Connect Pocket Money (Settings → Connected apps) to book it on your accounts too.'}</p>
}

// ---- a place ----

export function PropertyForm({ property, onClose, onSaved }: { property?: Property; onClose: () => void; onSaved?: () => void }) {
  const { settings } = useApp()
  const navigate = useNavigate()
  const { toast } = useToast()
  const appState = useAppState()
  const home = appState('home', 'home:read')
  const [f, setF] = useState(() => ({
    name: property?.name || '', kind: property?.kind || 'apartment', address: property?.address || '', city: property?.city || '', country: property?.country || '',
    currency: property?.currency || settings?.currency || 'EUR', value: fromCents(property?.value || 0), bought_on: property?.bought_on || '', bought_for: fromCents(property?.bought_for || 0),
    home_ref: property?.home_ref || '', money_tag: property?.money_tag || '', color: property?.color || '', notes: property?.notes || '', rent: '', deposit: '',
  }))
  const [units, setUnits] = useState<{ name: string; rent: string }[]>([{ name: 'Flat 1', rent: '' }, { name: 'Flat 2', rent: '' }])
  const [more, setMore] = useState(!!property)
  const [busy, setBusy] = useState(false)
  const { data: homes } = useAsync(() => home.ok ? api.get<{ id: string; name: string; address: string }[]>(`${V1}/apps/options/homes`).catch(() => []) : Promise.resolve([]), [home.ok])
  const set = (k: keyof typeof f, v: string) => setF(x => ({ ...x, [k]: v }))
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body: Record<string, unknown> = { name: f.name, kind: f.kind, address: f.address, city: f.city, country: f.country, currency: f.currency, value: toCents(f.value),
        bought_on: f.bought_on, bought_for: toCents(f.bought_for), home_ref: f.home_ref, money_tag: f.money_tag, color: f.color, notes: f.notes }
      if (!property) {
        if (f.kind === 'building') body.units = units.filter(u => u.name.trim()).map(u => ({ name: u.name.trim(), rent: toCents(u.rent) }))
        else if (f.rent || f.deposit) body.units = [{ name: { apartment: 'Apartment', room: 'Room', garage: 'Garage', commercial: 'Premises' }[f.kind as string] || 'Whole place', rent: toCents(f.rent), deposit: toCents(f.deposit) }]
      }
      const r = property ? await api.put<{ property: Property }>(`${V1}/properties/${property.id}`, body) : await api.post<{ property: Property }>(`${V1}/properties`, body)
      announceChange()
      toast(property ? 'Saved' : `Added ${r.property.name}`)
      onSaved?.()
      if (!property) navigate(`/properties/${r.property.id}`)
      onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={property ? 'Change the place' : 'Add a place'} onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !(f.name || f.address)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{property ? 'Save' : 'Add the place'}</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="What is it" group><Select ariaLabel="Kind" value={f.kind} onChange={v => set('kind', v)} options={KINDS.map(k => ({ value: k.value, label: k.label }))} /></Field>
        <div className="two-col tight">
          <Field label="Name" help="How you call it: “Linden Court”, “Mum’s flat”."><input value={f.name} onChange={e => set('name', e.target.value)} placeholder="Linden Court" autoFocus /></Field>
          <Field label="Address"><input value={f.address} onChange={e => set('address', e.target.value)} placeholder="Linden Street 14" /></Field>
          <Field label="Town or city"><input value={f.city} onChange={e => set('city', e.target.value)} /></Field>
          <Field label="Currency" group><Select ariaLabel="Currency" value={f.currency} onChange={v => set('currency', v)} options={Array.from(new Set([f.currency, ...CURRENCIES])).map(c => ({ value: c, label: c }))} /></Field>
        </div>
        {!property && f.kind === 'building' && <section className="form-sec">
          <h4 className="form-sec-h">The flats (or rooms) you let</h4>
          {units.map((u, i) => (
            <div key={i} className="row tight">
              <input value={u.name} onChange={e => setUnits(us => us.map((x, j) => j === i ? { ...x, name: e.target.value } : x))} aria-label="Unit name" placeholder={`Flat ${i + 1}`} />
              <input inputMode="decimal" className="num-md" value={u.rent} onChange={e => setUnits(us => us.map((x, j) => j === i ? { ...x, rent: e.target.value } : x))} aria-label="Rent" placeholder={`Rent, ${f.currency}`} />
              <button type="button" className="btn ghost icon-only" aria-label="Remove" onClick={() => setUnits(us => us.filter((_, j) => j !== i))}><Icon name="x" size={15} /></button>
            </div>
          ))}
          <button type="button" className="link-btn self-start" onClick={() => setUnits(us => [...us, { name: `Flat ${us.length + 1}`, rent: '' }])}><Icon name="plus" size={14} />Another one</button>
        </section>}
        {!property && f.kind !== 'building' && <div className="two-col tight">
          <MoneyField label="Rent you ask" value={f.rent} onChange={v => set('rent', v)} currency={f.currency} help="A month; you can change it on each lease." />
          <MoneyField label="Deposit you ask" value={f.deposit} onChange={v => set('deposit', v)} currency={f.currency} />
        </div>}
        {!more && <button type="button" className="link-btn self-start" onClick={() => setMore(true)}><Icon name="plus" size={14} />What it is worth, when you bought it, a colour, notes, Pocket Home</button>}
        {more && <section className="form-sec">
          <div className="two-col tight">
            <MoneyField label="Worth today" value={f.value} onChange={v => set('value', v)} currency={f.currency} help="For the yield in Reports (and Pocket Wealth)." />
            <MoneyField label="Bought for" value={f.bought_for} onChange={v => set('bought_for', v)} currency={f.currency} />
            <Field label="Bought on"><input type="date" value={f.bought_on} onChange={e => set('bought_on', e.target.value)} /></Field>
            <Field label="Tag in Pocket Money" help="Rent and costs booked there get it, so you can filter."><input value={f.money_tag} onChange={e => set('money_tag', e.target.value)} placeholder="linden" /></Field>
          </div>
          <Field label="The same place in Pocket Home" group help={home.ok ? 'Finished repairs go into its log there.' : undefined}>
            <Select ariaLabel="Home" value={f.home_ref} onChange={v => set('home_ref', v)} placeholder={home.ok ? 'Not linked' : 'Pocket Home is not connected'} clearLabel="Not linked" disabled={!home.ok}
              options={(homes || []).map(h => ({ value: String(h.id), label: h.name, hint: h.address }))} />
          </Field>
          {!home.ok && !home.loading && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {home.app?.state === 'missing' ? 'Pocket Home is not on this box.' : 'Connect Pocket Home (Settings → Connected apps) to link the place and log repairs there.'}{home.app?.state === 'no_core' && <CoreHint />}</p>}
          <Field label="Colour" group><div className="swatches">{COLORS.map(c => <button key={c} type="button" className={`swatch ${f.color === c ? 'on' : ''}`} style={{ background: c }} aria-label={c} onClick={() => set('color', f.color === c ? '' : c)} />)}</div></Field>
          <Field label="Notes" help="Keys, meters, the building manager, anything to remember."><textarea rows={3} value={f.notes} onChange={e => set('notes', e.target.value)} /></Field>
        </section>}
      </form>
    </Modal>
  )
}

// ---- a unit ----

export function UnitForm({ unit, propertyId, currency, onClose, onSaved }: { unit?: Unit; propertyId: number; currency: string; onClose: () => void; onSaved?: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: unit?.name || '', bedrooms: String(unit?.bedrooms ?? ''), bathrooms: String(unit?.bathrooms ?? ''), area_m2: unit?.area_m2 ? String(unit.area_m2) : '',
    rent: fromCents(unit?.rent || 0), deposit: fromCents(unit?.deposit || 0), notes: unit?.notes || '', archived: !!unit?.archived })
  const [busy, setBusy] = useState(false)
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body = { name: f.name, bedrooms: Number(f.bedrooms) || 0, bathrooms: Number(f.bathrooms) || 0, area_m2: Number(f.area_m2.replace(',', '.')) || 0, rent: toCents(f.rent), deposit: toCents(f.deposit), notes: f.notes, archived: f.archived }
      if (unit) await api.put(`${V1}/units/${unit.id}`, body)
      else await api.post(`${V1}/properties/${propertyId}/units`, body)
      announceChange(); toast('Saved'); onSaved?.(); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={unit ? `Change ${unit.name}` : 'Add a unit'} onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !f.name.trim()}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} placeholder="Flat 3" autoFocus /></Field>
        <div className="three-col tight">
          <Field label="Bedrooms"><input type="number" min={0} value={f.bedrooms} onChange={e => setF({ ...f, bedrooms: e.target.value })} /></Field>
          <Field label="Bathrooms"><input type="number" min={0} value={f.bathrooms} onChange={e => setF({ ...f, bathrooms: e.target.value })} /></Field>
          <Field label="Size, m²"><input inputMode="decimal" value={f.area_m2} onChange={e => setF({ ...f, area_m2: e.target.value })} /></Field>
        </div>
        <div className="two-col tight">
          <MoneyField label="Rent you ask" value={f.rent} onChange={v => setF({ ...f, rent: v })} currency={currency} />
          <MoneyField label="Deposit you ask" value={f.deposit} onChange={v => setF({ ...f, deposit: v })} currency={currency} />
        </div>
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
        {unit && <label className="check-row"><Switch checked={f.archived} onChange={v => setF({ ...f, archived: v })} label="Archived" /><span>Not let any more (kept with its history)</span></label>}
      </form>
    </Modal>
  )
}

// ---- tenants on a lease ----

type TenantDraft = { id?: number; name: string; email: string; phone: string; contact_ref: string }

function ContactSearch({ onPick }: { onPick: (c: PickedContact) => void }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<PickedContact[] | null>(null)
  const [off, setOff] = useState('')
  useEffect(() => {
    if (dq.trim().length < 2) { setList(null); return }
    let alive = true
    api.get<{ contacts: PickedContact[]; off?: string }>(`${V1}/apps/contacts?q=${encodeURIComponent(dq.trim())}`)
      .then(r => { if (alive) { setList(r.contacts); setOff(r.off || '') } }).catch(e => { if (alive) setOff(errMsg(e)) })
    return () => { alive = false }
  }, [dq])
  return (
    <div className="picker">
      <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find in Pocket Contacts…" aria-label="Find in Pocket Contacts" /></div>
      {off && <p className="muted small">{off}</p>}
      {list && (list.length === 0 ? <p className="muted small">Nobody called “{dq}” in your contacts.</p> : (
        <div className="pick-list">
          {list.map(c => (
            <button key={c.id} type="button" className="pick-row" onClick={() => { onPick(c); setQ(''); setList(null) }}>
              <span className="pick-ava" style={{ background: c.color || 'var(--accent)' }}>{c.initials || c.name.slice(0, 1)}</span>
              <span className="grow ellipsis"><b>{c.name}</b>{c.email && <span className="muted"> · {c.email}</span>}</span>
            </button>
          ))}
        </div>
      ))}
    </div>
  )
}

function TenantsEditor({ value, onChange }: { value: TenantDraft[]; onChange: (v: TenantDraft[]) => void }) {
  const appState = useAppState()
  const contacts = appState('contacts', 'contacts:read')
  const { data: known } = useAsync(() => api.get<Tenant[]>(`${V1}/tenants`), [])
  const [typing, setTyping] = useState(value.length === 0)
  const [d, setD] = useState<TenantDraft>({ name: '', email: '', phone: '', contact_ref: '' })
  const add = (t: TenantDraft) => { if (!value.some(v => (v.id && v.id === t.id) || (t.email && v.email === t.email))) onChange([...value, t]) }
  const addTyped = () => { if (d.name.trim() || d.email.trim()) { add({ ...d, name: d.name.trim(), email: d.email.trim() }); setD({ name: '', email: '', phone: '', contact_ref: '' }); setTyping(false) } }
  const others = (known || []).filter(k => !value.some(v => v.id === k.id))
  return (
    <section className="form-sec">
      <h4 className="form-sec-h">Who rents it</h4>
      {value.length > 0 && <div className="tenant-chips">
        {value.map((t, i) => <span key={i} className="chip">{t.name || t.email}{t.contact_ref && <Icon name="link" size={11} className="inline-ico" />}<button type="button" className="chip-x" aria-label={`Remove ${t.name}`} onClick={() => onChange(value.filter((_, j) => j !== i))}><Icon name="x" size={11} /></button></span>)}
      </div>}
      {others.length > 0 && <Select ariaLabel="A tenant you have" value="" onChange={v => { const t = others.find(o => String(o.id) === v); if (t) add({ id: t.id, name: t.name, email: t.email, phone: t.phone, contact_ref: t.contact_ref }) }}
        placeholder="Pick someone who rented from you before…" options={others.map(o => ({ value: String(o.id), label: o.name, hint: o.email }))} />}
      {contacts.ok ? <ContactSearch onPick={c => add({ name: c.name, email: c.email, phone: c.phone, contact_ref: c.id })} />
        : contacts.app && <p className="muted small"><Icon name="info" size={13} className="inline-ico" /> {contacts.app.state === 'missing' ? 'Type their details.' : 'Type their details, or connect Pocket Contacts (Settings → Connected apps) to pick them from your contacts.'}</p>}
      {typing ? <div className="tenant-new">
        <div className="three-col tight">
          <Field label="Name"><input value={d.name} onChange={e => setD({ ...d, name: e.target.value })} placeholder="Ana Marić" autoComplete="off" /></Field>
          <Field label="Email"><input type="email" value={d.email} onChange={e => setD({ ...d, email: e.target.value })} autoComplete="off" /></Field>
          <Field label="Phone"><input type="tel" value={d.phone} onChange={e => setD({ ...d, phone: e.target.value })} autoComplete="off" /></Field>
        </div>
        <div className="row"><button type="button" className="btn sm" onClick={addTyped} disabled={!d.name.trim() && !d.email.trim()}><Icon name="plus" size={14} />Add {d.name.trim() || 'them'}</button>{value.length > 0 && <button type="button" className="btn ghost sm" onClick={() => setTyping(false)}>Cancel</button>}</div>
      </div> : <button type="button" className="link-btn self-start" onClick={() => setTyping(true)}><Icon name="plus" size={14} />Type a new tenant</button>}
    </section>
  )
}

// ---- a lease ----

export function LeaseForm({ lease, initial = {}, onClose, onSaved }: { lease?: Lease; initial?: Init; onClose: () => void; onSaved?: () => void }) {
  const { settings } = useApp()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data: units } = useUnits()
  const firstOfMonth = todayISO().slice(0, 8) + '01'
  const [f, setF] = useState(() => ({
    unit_id: lease?.unit_id || num(initial.unit_id), status: lease?.status || 'active', start_date: lease?.start_date || todayISO(), end_date: lease?.end_date || '',
    until_notice: lease ? !lease.end_date : false, rent: fromCents(lease?.rent || 0), currency: lease?.currency || settings?.currency || 'EUR', frequency: lease?.frequency || 'monthly',
    due_day: String(lease?.due_day || settings?.due_day || 1), deposit: fromCents(lease?.deposit || 0), deposit_paid: lease ? lease.deposit_state !== 'due' : false,
    grace_days: String(lease?.grace_days ?? settings?.grace_days ?? 5), late_fee: fromCents(lease?.late_fee || 0), notice_days: String(lease?.notice_days ?? settings?.notice_days ?? 30),
    charges_from: lease?.charges_from || '', notes: lease?.notes || '',
  }))
  const [tenants, setTenants] = useState<TenantDraft[]>(() => (lease?.tenants || []).map(t => ({ id: t.id, name: t.name, email: t.email, phone: t.phone, contact_ref: t.contact_ref })))
  const [more, setMore] = useState(false)
  const [busy, setBusy] = useState(false)
  const picked = units?.find(u => u.id === f.unit_id)
  // a new lease takes the unit's asking rent and deposit
  useEffect(() => {
    if (lease || !picked) return
    setF(x => ({ ...x, rent: x.rent || fromCents(picked.rent), deposit: x.deposit || fromCents(picked.deposit) }))
  }, [picked?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const started = !lease && f.start_date < firstOfMonth
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body: Record<string, unknown> = {
        unit_id: f.unit_id, status: f.status, start_date: f.start_date, end_date: f.until_notice ? '' : f.end_date, rent: toCents(f.rent), currency: f.currency,
        frequency: f.frequency, due_day: Number(f.due_day) || 1, deposit: toCents(f.deposit), deposit_state: toCents(f.deposit) ? (f.deposit_paid ? 'held' : 'due') : 'none',
        grace_days: Number(f.grace_days) || 0, late_fee: toCents(f.late_fee), notice_days: Number(f.notice_days) || 0, charges_from: f.charges_from, notes: f.notes,
        tenants: tenants.map(t => t.id ? { id: t.id } : { name: t.name, email: t.email, phone: t.phone, contact_ref: t.contact_ref }),
      }
      if (started && !f.charges_from) body.charges_from = firstOfMonth
      const r = lease ? await api.put<{ lease: Lease }>(`${V1}/leases/${lease.id}`, body) : await api.post<{ lease: Lease }>(`${V1}/leases`, body)
      announceChange()
      toast(lease ? 'Saved' : 'Lease added')
      onSaved?.()
      if (!lease) navigate(`/leases/${r.lease.id}`)
      onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const dayOptions = f.frequency === 'weekly' ? WEEKDAYS.map((w, i) => ({ value: String(i + 1), label: w })) : Array.from({ length: 28 }, (_, i) => ({ value: String(i + 1), label: `The ${i + 1}${[, 'st', 'nd', 'rd'][(i + 1) % 10 > 3 || [11, 12, 13].includes(i + 1) ? 0 : (i + 1) % 10] || 'th'}` }))
  return (
    <Modal title={lease ? 'Change the lease' : 'New lease'} onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !f.unit_id || !toCents(f.rent)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{lease ? 'Save' : f.status === 'draft' ? 'Save as a draft' : 'Add the lease'}</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="Which unit" group>
          <Select ariaLabel="Unit" value={f.unit_id ? String(f.unit_id) : ''} onChange={v => setF({ ...f, unit_id: Number(v) })} placeholder="Pick the flat, room or house…"
            options={(units || []).map(u => ({ value: String(u.id), label: unitLabel(u), hint: u.lease?.status === 'active' && u.lease.id !== lease?.id ? `let to ${names(u.lease.tenants) || 'someone'}` : u.rent ? fmtMoney(u.rent, u.lease?.currency || f.currency) : undefined }))} />
        </Field>
        <TenantsEditor value={tenants} onChange={setTenants} />
        <section className="form-sec">
          <h4 className="form-sec-h">When</h4>
          <div className="two-col tight">
            <Field label="Starts on"><input type="date" value={f.start_date} onChange={e => setF({ ...f, start_date: e.target.value })} /></Field>
            <Field label="Ends on" group>
              <div className="stack tight">
                <input type="date" value={f.until_notice ? '' : f.end_date} onChange={e => setF({ ...f, end_date: e.target.value })} disabled={f.until_notice} aria-label="Ends on" />
                <label className="check-row small"><input type="checkbox" checked={f.until_notice} onChange={e => setF({ ...f, until_notice: e.target.checked })} />It runs until someone gives notice</label>
              </div>
            </Field>
          </div>
          {started && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> It started before this month: rent is counted from {fmtDate(f.charges_from || firstOfMonth)} on, so older rent does not show as owed. Change it under “More”.</p>}
        </section>
        <section className="form-sec">
          <h4 className="form-sec-h">Rent</h4>
          <div className="two-col tight">
            <MoneyField label="Rent" value={f.rent} onChange={v => setF({ ...f, rent: v })} currency={f.currency} onCurrency={c => setF({ ...f, currency: c })} />
            <Field label="Paid" group><Select ariaLabel="How often" value={f.frequency} onChange={v => setF({ ...f, frequency: v as Lease['frequency'], due_day: v === 'weekly' ? '1' : f.due_day })} options={FREQ.map(x => ({ value: x.value, label: x.label }))} /></Field>
            <Field label="Due" group help="Rent pays ahead, from this day to the next. A start between two due days pays the part by the day."><Select ariaLabel="Due day" value={f.due_day} onChange={v => setF({ ...f, due_day: v })} options={dayOptions} /></Field>
            <Field label="Deposit" group>
              <div className="stack tight">
                <div className="input-group"><input inputMode="decimal" value={f.deposit} onChange={e => setF({ ...f, deposit: e.target.value })} placeholder="0" aria-label="Deposit" /><span className="input-suffix">{f.currency}</span></div>
                {!!toCents(f.deposit) && <label className="check-row small"><input type="checkbox" checked={f.deposit_paid} onChange={e => setF({ ...f, deposit_paid: e.target.checked })} />Already paid (you hold it)</label>}
              </div>
            </Field>
          </div>
        </section>
        {!more && <button type="button" className="link-btn self-start" onClick={() => setMore(true)}><Icon name="plus" size={14} />More: late days, a late fee, notice, a draft, notes</button>}
        {more && <section className="form-sec">
          <div className="three-col tight">
            <Field label="Late after" help="Days after the due day."><input type="number" min={0} max={60} value={f.grace_days} onChange={e => setF({ ...f, grace_days: e.target.value })} /></Field>
            <MoneyField label="Late fee" value={f.late_fee} onChange={v => setF({ ...f, late_fee: v })} currency={f.currency} help="Added once when a rent is late." />
            <Field label="Notice" help="Days."><input type="number" min={0} max={365} value={f.notice_days} onChange={e => setF({ ...f, notice_days: e.target.value })} /></Field>
          </div>
          <div className="two-col tight">
            <Field label="Count rent from" help="For a lease that started long ago: rent before this day is settled."><input type="date" value={f.charges_from} onChange={e => setF({ ...f, charges_from: e.target.value })} /></Field>
            <Field label="State" group><Segmented size="sm" value={f.status} onChange={v => setF({ ...f, status: v as Lease['status'] })} options={[{ value: 'draft', label: 'Draft' }, { value: 'active', label: 'Active' }, ...(lease ? [{ value: 'ended', label: 'Ended' }] : [])]} /></Field>
          </div>
          <Field label="Notes"><textarea rows={3} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
        </section>}
      </form>
    </Modal>
  )
}

// ---- rent that came in ----

export function PaymentForm({ initial = {}, onClose, onSaved }: { initial?: Init; onClose: () => void; onSaved?: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const appState = useAppState()
  const money = appState('money', 'transactions:write')
  const moneyRead = appState('money', 'transactions:read')
  const { data: leases } = useAsync(() => api.get<Lease[]>(`${V1}/leases?status=active`), [])
  const [leaseID, setLeaseID] = useState(num(initial.lease_id))
  const lease = leases?.find(l => l.id === leaseID)
  const [f, setF] = useState({ date: todayISO(), amount: initial.amount ? fromCents(num(initial.amount)) : '', method: 'bank', note: '', money: !!settings?.money_book, account: '' })
  const [busy, setBusy] = useState(false)
  const [found, setFound] = useState<Suggestion[] | null>(null)
  useEffect(() => {
    if (!lease || initial.amount) return
    setF(x => ({ ...x, amount: x.amount || fromCents(lease.balance > 0 ? lease.balance : lease.rent) }))
  }, [lease?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!leaseID || !moneyRead.ok || !settings?.money_match) { setFound(null); return }
    let alive = true
    api.get<{ items: Suggestion[] }>(`${V1}/money/suggestions?lease=${leaseID}`).then(r => { if (alive) setFound(r.items) }).catch(() => { if (alive) setFound([]) })
    return () => { alive = false }
  }, [leaseID, moneyRead.ok, settings?.money_match])
  const { data: accounts } = useAsync(() => money.ok && f.money ? api.get<{ id: number; name: string; currency: string }[]>(`${V1}/apps/options/accounts`).catch(() => []) : Promise.resolve([]), [money.ok, f.money])
  const done = (msg: string) => { announceChange(); toast(msg); onSaved?.(); onClose() }
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    if (!lease) return
    setBusy(true)
    try {
      const r = await api.post<{ money?: { ok: boolean; error?: string; account?: string } }>(`${V1}/leases/${lease.id}/payments`, { date: f.date, amount: toCents(f.amount), method: f.method, note: f.note, money: money.ok && f.money, account_id: Number(f.account) || 0 })
      if (r.money && !r.money.ok) toast(`Recorded here; Pocket Money: ${r.money.error}`, 'err')
      done(r.money?.ok ? `Recorded, and booked on ${r.money.account}` : 'Recorded')
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const match = async (s: Suggestion) => {
    setBusy(true)
    try { await api.post(`${V1}/money/match`, { tx_id: s.tx_id, lease_id: s.lease_id }); done('Linked to Pocket Money') } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Rent that came in" onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !lease || !toCents(f.amount)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Record it</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="From which lease" group>
          <Select ariaLabel="Lease" value={leaseID ? String(leaseID) : ''} onChange={v => setLeaseID(Number(v))} placeholder="Pick the lease…"
            options={(leases || []).map(l => ({ value: String(l.id), label: l.title, hint: l.balance > 0 ? `owes ${fmtMoney(l.balance, l.currency)}` : 'paid up' }))} />
        </Field>
        {found && found.length > 0 && <div className="found-box">
          <div className="found-h"><Icon name="wallet" size={14} />Already in Pocket Money?</div>
          {found.slice(0, 3).map(s => (
            <div key={s.tx_id} className="found-row">
              <div className="grow min0"><b>{fmtMoney(s.amount, s.currency)}</b> on {fmtDate(s.date)} <span className="muted">· {s.payee || s.description}</span><div className="muted tiny">Looks like it: {s.why}</div></div>
              <button type="button" className="btn sm" onClick={() => match(s)} disabled={busy}>That’s it</button>
            </div>
          ))}
        </div>}
        <div className="two-col tight">
          <Field label="Came in on"><input type="date" value={f.date} onChange={e => setF({ ...f, date: e.target.value })} /></Field>
          <MoneyField label="How much" value={f.amount} onChange={v => setF({ ...f, amount: v })} currency={lease?.currency || settings?.currency || 'EUR'}
            help={lease ? (lease.balance > 0 ? `Owed now: ${fmtMoney(lease.balance, lease.currency)}` : `Rent: ${fmtMoney(lease.rent, lease.currency)}`) : undefined} />
        </div>
        <Field label="How" group><Segmented size="sm" value={f.method} onChange={v => setF({ ...f, method: v })} options={[{ value: 'bank', label: 'Bank' }, { value: 'cash', label: 'Cash' }, { value: 'card', label: 'Card' }, { value: 'other', label: 'Other' }]} /></Field>
        <Field label="Note"><input value={f.note} onChange={e => setF({ ...f, note: e.target.value })} placeholder="Optional" /></Field>
        <div className="money-opt">
          <label className={`check-row ${money.ok ? '' : 'disabled'}`}><Switch checked={money.ok && f.money} onChange={v => setF({ ...f, money: v })} label="Also in Pocket Money" disabled={!money.ok} /><span>Also book it in Pocket Money</span></label>
          {money.ok && f.money && (accounts || []).length > 0 && <Select ariaLabel="Account" value={f.account || String(settings?.money_account || '')} onChange={v => setF({ ...f, account: v })} placeholder="Your everyday account" options={(accounts || []).map(a => ({ value: String(a.id), label: a.name, hint: a.currency }))} />}
          <MoneyOff ok={money.ok || money.loading} state={money.app?.state} />
        </div>
      </form>
    </Modal>
  )
}

// ---- a charge ----

export function ChargeForm({ lease, charge, onClose, onSaved }: { lease: Lease; charge?: Charge; onClose: () => void; onSaved?: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ kind: charge?.kind || 'utilities', title: charge?.title || '', due_date: charge?.due_date || todayISO(), amount: fromCents(charge?.amount || 0), note: charge?.note || '' })
  const [busy, setBusy] = useState(false)
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body = { kind: f.kind, title: f.title, due_date: f.due_date, amount: toCents(f.amount), note: f.note }
      if (charge) await api.put(`${V1}/charges/${charge.id}`, body)
      else await api.post(`${V1}/leases/${lease.id}/charges`, body)
      announceChange(); toast('Saved'); onSaved?.(); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={charge ? 'Change the charge' : 'Add a charge'} onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !toCents(f.amount)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
    </>}>
      <form className="stack" onSubmit={save}>
        {!charge && <Field label="What for" group><Select ariaLabel="Kind" value={f.kind} onChange={v => setF({ ...f, kind: v as Charge['kind'] })} options={CHARGE_KINDS} /></Field>}
        <Field label="In words" help="Shown to you and on an invoice."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} placeholder="Water, October" autoFocus /></Field>
        <div className="two-col tight">
          <MoneyField label="How much" value={f.amount} onChange={v => setF({ ...f, amount: v })} currency={lease.currency} />
          <Field label="Due on"><input type="date" value={f.due_date} onChange={e => setF({ ...f, due_date: e.target.value })} /></Field>
        </div>
        <Field label="Note"><input value={f.note} onChange={e => setF({ ...f, note: e.target.value })} /></Field>
      </form>
    </Modal>
  )
}

// ---- a repair ----

type Pro = { id: string; name: string; trade: string; phone: string; company: string }

export function RequestForm({ request, initial = {}, onClose, onSaved }: { request?: Request; initial?: Init; onClose: () => void; onSaved?: (r: Request) => void }) {
  const { settings } = useApp()
  const { properties } = useData()
  const navigate = useNavigate()
  const { toast } = useToast()
  const appState = useAppState()
  const home = appState('home', 'home:read')
  const tasks = appState('tasks', 'tasks:write')
  const { data: units } = useUnits()
  const [f, setF] = useState(() => ({
    property_id: request?.property_id || num(initial.property_id) || (properties?.length === 1 ? properties[0].id : 0), unit_id: request?.unit_id || num(initial.unit_id),
    title: request?.title || String(initial.title || ''), details: request?.details || String(initial.details || ''), priority: request?.priority || 'normal',
    reported_by: request?.reported_by || '', planned_on: request?.planned_on || '', planned_time: request?.planned_time || '', cost: fromCents(request?.cost || 0),
    pro_name: request?.pro_name || '', pro_phone: request?.pro_phone || '', pro_ref: request?.pro_ref || '', tenant_pays: !!request?.tenant_pays, task: !!settings?.repair_tasks,
  }))
  const [busy, setBusy] = useState(false)
  const prop = properties?.find(p => p.id === f.property_id)
  const theirUnits = useMemo(() => (units || []).filter(u => u.property_id === f.property_id), [units, f.property_id])
  const { data: pros } = useAsync(() => home.ok ? api.get<Pro[]>(`${V1}/apps/options/pros`).catch(() => [] as Pro[]) : Promise.resolve([] as Pro[]), [home.ok])
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body: Record<string, unknown> = { property_id: f.property_id, unit_id: f.unit_id || null, title: f.title, details: f.details, priority: f.priority, reported_by: f.reported_by,
        planned_on: f.planned_on, planned_time: f.planned_on ? f.planned_time : '', cost: toCents(f.cost), currency: prop?.currency, pro_name: f.pro_name, pro_phone: f.pro_phone, pro_ref: f.pro_ref, tenant_pays: f.tenant_pays }
      if (initial.lease_id && !request) body.lease_id = num(initial.lease_id)
      if (!request) body.task = tasks.ok && f.task
      const r = request ? await api.put<{ request: Request }>(`${V1}/requests/${request.id}`, body) : await api.post<{ request: Request; task_error?: string }>(`${V1}/requests`, body)
      announceChange()
      toast(request ? 'Saved' : 'Repair added')
      if ((r as { task_error?: string }).task_error) toast(`Pocket Tasks: ${(r as { task_error?: string }).task_error}`, 'err')
      onSaved?.(r.request)
      if (!request) navigate(`/repairs/${r.request.id}`)
      onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={request ? 'Change the repair' : 'Report a repair'} onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !f.property_id || !f.title.trim()}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{request ? 'Save' : 'Add the repair'}</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <div className="two-col tight">
          <Field label="Where" group><Select ariaLabel="Place" value={f.property_id ? String(f.property_id) : ''} onChange={v => setF({ ...f, property_id: Number(v), unit_id: 0 })} placeholder="Pick the place…" options={(properties || []).map(p => ({ value: String(p.id), label: p.name }))} /></Field>
          <Field label="Which unit" group><Select ariaLabel="Unit" value={f.unit_id ? String(f.unit_id) : ''} onChange={v => setF({ ...f, unit_id: Number(v) })} placeholder="The whole place or common parts" clearLabel="The whole place or common parts" options={theirUnits.map(u => ({ value: String(u.id), label: u.name, hint: u.lease?.status === 'active' ? names(u.lease.tenants) : undefined }))} disabled={!f.property_id} /></Field>
        </div>
        <Field label="What needs fixing"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} placeholder="Dripping kitchen tap" autoFocus /></Field>
        <Field label="Details"><textarea rows={3} value={f.details} onChange={e => setF({ ...f, details: e.target.value })} placeholder="What happens, since when, what was tried" /></Field>
        <div className="two-col tight">
          <Field label="How urgent" group><Segmented size="sm" value={f.priority} onChange={v => setF({ ...f, priority: v })} options={[{ value: 'low', label: 'It can wait' }, { value: 'normal', label: 'Normal' }, { value: 'urgent', label: 'Urgent' }]} /></Field>
          <Field label="Reported by" help="Empty: the tenant of that unit."><input value={f.reported_by} onChange={e => setF({ ...f, reported_by: e.target.value })} /></Field>
        </div>
        <section className="form-sec">
          <h4 className="form-sec-h">Who fixes it, and when</h4>
          {home.ok && (pros || []).length > 0 && <Select ariaLabel="Tradesperson" value={f.pro_ref} onChange={v => { const p = (pros || []).find(x => x.id === v); setF({ ...f, pro_ref: v, pro_name: p ? p.name || p.company : f.pro_name, pro_phone: p?.phone || f.pro_phone }) }}
            placeholder="Pick from your tradespeople in Pocket Home…" clearLabel="Nobody picked" options={(pros || []).map(p => ({ value: p.id, label: p.name || p.company, hint: p.trade }))} />}
          {!home.ok && !home.loading && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {home.app?.state === 'missing' ? 'Type who fixes it.' : 'Type who fixes it, or connect Pocket Home to pick from your tradespeople.'}</p>}
          <div className="two-col tight">
            <Field label="Name"><input value={f.pro_name} onChange={e => setF({ ...f, pro_name: e.target.value, pro_ref: '' })} placeholder="Ivo the plumber" /></Field>
            <Field label="Phone"><input type="tel" value={f.pro_phone} onChange={e => setF({ ...f, pro_phone: e.target.value })} /></Field>
            <Field label="Day"><input type="date" value={f.planned_on} onChange={e => setF({ ...f, planned_on: e.target.value })} /></Field>
            <Field label="Time"><input type="time" value={f.planned_time} onChange={e => setF({ ...f, planned_time: e.target.value })} disabled={!f.planned_on} /></Field>
            <MoneyField label="Cost" value={f.cost} onChange={v => setF({ ...f, cost: v })} currency={prop?.currency || settings?.currency || 'EUR'} help="When it is done, this becomes a cost of the place." />
            <Field label="The tenant pays" group><label className="check-row"><Switch checked={f.tenant_pays} onChange={v => setF({ ...f, tenant_pays: v })} label="The tenant pays" /><span className="small">They are charged when it is done</span></label></Field>
          </div>
        </section>
        {!request && <div>
          <label className={`check-row ${tasks.ok ? '' : 'disabled'}`}><Switch checked={tasks.ok && f.task} onChange={v => setF({ ...f, task: v })} label="Also a task" disabled={!tasks.ok} /><span>Also a task in Pocket Tasks{f.planned_on ? `, due ${fmtDate(f.planned_on)}` : ''}</span></label>
          {!tasks.ok && !tasks.loading && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {tasks.app?.state === 'missing' ? 'Pocket Tasks is not on this box.' : 'Connect Pocket Tasks (Settings → Connected apps) to have repairs in your task list.'}</p>}
        </div>}
      </form>
    </Modal>
  )
}

// ---- a cost ----

export function ExpenseForm({ expense, initial = {}, onClose, onSaved }: { expense?: Expense; initial?: Init; onClose: () => void; onSaved?: () => void }) {
  const { settings } = useApp()
  const { properties } = useData()
  const { toast } = useToast()
  const appState = useAppState()
  const money = appState('money', 'transactions:write')
  const { data: units } = useUnits()
  const [f, setF] = useState(() => ({ property_id: expense?.property_id || num(initial.property_id) || (properties?.length === 1 ? properties[0].id : 0), unit_id: expense?.unit_id || 0,
    date: expense?.date || todayISO(), amount: fromCents(expense?.amount || 0), category: expense?.category || 'repairs', payee: expense?.payee || '', note: expense?.note || '', money: !!settings?.money_book }))
  const [busy, setBusy] = useState(false)
  const prop = properties?.find(p => p.id === f.property_id)
  const theirUnits = (units || []).filter(u => u.property_id === f.property_id)
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body: Record<string, unknown> = { property_id: f.property_id, unit_id: f.unit_id || null, date: f.date, amount: toCents(f.amount), currency: expense?.currency || prop?.currency, category: f.category, payee: f.payee, note: f.note }
      if (!expense) body.money = money.ok && f.money
      const r = expense ? await api.put<{ money?: { ok: boolean; error?: string } }>(`${V1}/expenses/${expense.id}`, body) : await api.post<{ money?: { ok: boolean; error?: string } }>(`${V1}/expenses`, body)
      if (r.money && !r.money.ok && r.money.error) toast(`Saved here; Pocket Money: ${r.money.error}`, 'err')
      announceChange(); toast(r.money?.ok ? 'Saved, and booked in Pocket Money' : 'Saved'); onSaved?.(); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={expense ? 'Change the cost' : 'Add a cost'} onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !f.property_id || !toCents(f.amount)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <div className="two-col tight">
          <Field label="For which place" group><Select ariaLabel="Place" value={f.property_id ? String(f.property_id) : ''} onChange={v => setF({ ...f, property_id: Number(v), unit_id: 0 })} placeholder="Pick the place…" options={(properties || []).map(p => ({ value: String(p.id), label: p.name }))} /></Field>
          <Field label="What kind" group><Select ariaLabel="Kind" value={f.category} onChange={v => setF({ ...f, category: v })} options={EXPENSE_KINDS} /></Field>
          <MoneyField label="How much" value={f.amount} onChange={v => setF({ ...f, amount: v })} currency={expense?.currency || prop?.currency || settings?.currency || 'EUR'} />
          <Field label="On"><input type="date" value={f.date} onChange={e => setF({ ...f, date: e.target.value })} /></Field>
          <Field label="Paid to"><input value={f.payee} onChange={e => setF({ ...f, payee: e.target.value })} placeholder="Riverside Mutual" /></Field>
          {theirUnits.length > 1 && <Field label="For one unit" group><Select ariaLabel="Unit" value={f.unit_id ? String(f.unit_id) : ''} onChange={v => setF({ ...f, unit_id: Number(v) })} placeholder="The whole place" clearLabel="The whole place" options={theirUnits.map(u => ({ value: String(u.id), label: u.name }))} /></Field>}
        </div>
        <Field label="Note"><input value={f.note} onChange={e => setF({ ...f, note: e.target.value })} placeholder="Building insurance for the year" /></Field>
        {!expense && <div className="money-opt">
          <label className={`check-row ${money.ok ? '' : 'disabled'}`}><Switch checked={money.ok && f.money} onChange={v => setF({ ...f, money: v })} label="Also in Pocket Money" disabled={!money.ok} /><span>Also book it in Pocket Money</span></label>
          <MoneyOff ok={money.ok || money.loading} state={money.app?.state} />
        </div>}
      </form>
    </Modal>
  )
}

// ---- an inspection ----

export function InspectionForm({ initial = {}, onClose }: { initial?: Init; onClose: () => void }) {
  const { properties } = useData()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data: units } = useUnits()
  const [f, setF] = useState(() => ({ property_id: num(initial.property_id) || (properties?.length === 1 ? properties[0].id : 0), unit_id: num(initial.unit_id), lease_id: num(initial.lease_id),
    kind: String(initial.kind || 'routine'), date: String(initial.date || todayISO()), time: '', start: 'list' }))
  const [busy, setBusy] = useState(false)
  const theirUnits = (units || []).filter(u => u.property_id === f.property_id)
  const unit = theirUnits.find(u => u.id === f.unit_id)
  const { data: before } = useAsync(() => f.property_id ? api.get<Inspection[]>(`${V1}/inspections?property=${f.property_id}`) : Promise.resolve([] as Inspection[]), [f.property_id])
  const earlier = (before || []).filter(i => (!f.unit_id || i.unit_id === f.unit_id) && i.item_count > 0)
  const ref = useRef(false)
  useEffect(() => { if (!ref.current && theirUnits.length === 1 && !f.unit_id) { ref.current = true; setF(x => ({ ...x, unit_id: theirUnits[0].id })) } }, [theirUnits.length]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const lease = f.lease_id || (unit?.lease?.status === 'active' ? unit.lease.id : 0)
      const body: Record<string, unknown> = { property_id: f.property_id, unit_id: f.unit_id || null, lease_id: lease || null, kind: f.kind, date: f.date, time: f.time, status: f.date < todayISO() ? 'done' : 'planned' }
      if (f.start === 'blank') body.blank = true
      else if (f.start !== 'list') body.from_id = Number(f.start)
      const r = await api.post<{ inspection: Inspection }>(`${V1}/inspections`, body)
      announceChange(); navigate(`/inspections/${r.inspection.id}`); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="An inspection" onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !f.property_id}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{f.date > todayISO() ? 'Plan it' : 'Start it'}</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="What for" group><Segmented size="sm" value={f.kind} onChange={v => setF({ ...f, kind: v })} options={Object.entries(INSPECTION_KIND).map(([value, label]) => ({ value, label }))} /></Field>
        <div className="two-col tight">
          <Field label="Where" group><Select ariaLabel="Place" value={f.property_id ? String(f.property_id) : ''} onChange={v => setF({ ...f, property_id: Number(v), unit_id: 0, start: 'list' })} placeholder="Pick the place…" options={(properties || []).map(p => ({ value: String(p.id), label: p.name }))} /></Field>
          <Field label="Which unit" group><Select ariaLabel="Unit" value={f.unit_id ? String(f.unit_id) : ''} onChange={v => setF({ ...f, unit_id: Number(v), start: 'list' })} placeholder="The whole place" clearLabel="The whole place" options={theirUnits.map(u => ({ value: String(u.id), label: u.name, hint: u.lease?.status === 'active' ? names(u.lease.tenants) : 'empty' }))} disabled={!f.property_id} /></Field>
          <Field label="Day"><input type="date" value={f.date} onChange={e => setF({ ...f, date: e.target.value })} /></Field>
          <Field label="Time"><input type="time" value={f.time} onChange={e => setF({ ...f, time: e.target.value })} /></Field>
        </div>
        <Field label="The list to go through" group>
          <Select ariaLabel="Start from" value={f.start} onChange={v => setF({ ...f, start: v })} options={[
            { value: 'list', label: 'The usual rooms (entrance, living room, kitchen, bathroom, bedroom, meters)' },
            ...earlier.map(i => ({ value: String(i.id), label: `The same lines as ${INSPECTION_KIND[i.kind].toLowerCase()} on ${fmtDate(i.date)}`, hint: `${i.item_count} lines` })),
            { value: 'blank', label: 'An empty list' },
          ]} />
        </Field>
      </form>
    </Modal>
  )
}
