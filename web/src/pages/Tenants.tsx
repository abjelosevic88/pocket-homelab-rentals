// Everyone who rents or rented from you, and one tenant's page.
import { useCallback, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useAppState, useOnChange } from '../lib/data'
import type { Lease, Tenant } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { LeaseRow } from './Leases'
import { Pill } from '../components/Bits'
import { offTitle } from '../components/Apps'
import { Icon } from '../components/icons'
import { Avatar, Card, Confirm, Empty, Field, Loading, Modal, Spinner, errMsg, useAsync, useToast } from '../components/ui'

export default function Tenants() {
  const { data, reload } = useAsync(() => api.get<Tenant[]>(`${V1}/tenants`), [])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const [q, setQ] = useState('')
  const [show, setShow] = useState<'now' | 'all'>('now')
  const list = useMemo(() => (data || []).filter(t => (show === 'all' || t.lease_id) && (!q || `${t.name} ${t.email} ${t.phone}`.toLowerCase().includes(q.toLowerCase()))), [data, q, show])
  return (
    <div className="page stack loose">
      <div className="page-head"><div className="grow"><h1 className="page-title">Tenants</h1><p className="muted">Who rents from you now, and who did.</p></div></div>
      <div className="row wrap filters">
        <div className="segmented sm" role="group">
          <button type="button" className={show === 'now' ? 'active' : ''} onClick={() => setShow('now')}>Renting now</button>
          <button type="button" className={show === 'all' ? 'active' : ''} onClick={() => setShow('all')}>Everyone</button>
        </div>
        <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find by name, email or phone" aria-label="Find" /></div>
      </div>
      {!data ? <Loading /> : list.length === 0 ? <Empty icon="users" title="No tenants here">Tenants are added with a lease.</Empty> : (
        <Card flush><ul className="list">{list.map(t => (
          <li key={t.id}><Link to={`/tenants/${t.id}`} className="list-row">
            <Avatar name={t.name} email={t.email} size={32} />
            <span className="row-main grow"><span className="row-title">{t.name}{t.contact_ref && <Icon name="link" size={11} />}</span><span className="muted small ellipsis">{t.current || (t.leases ? 'Rented before' : 'No lease')}{t.email ? ` · ${t.email}` : ''}</span></span>
            {t.balance > 0 && <Pill cls="st-due">Owes rent</Pill>}
          </Link></li>))}</ul></Card>
      )}
    </div>
  )
}

export function TenantPage() {
  const id = Number(useParams().id)
  const navigate = useNavigate()
  const { toast } = useToast()
  const appState = useAppState()
  const contacts = appState('contacts', 'contacts:read')
  const { data, reload, error } = useAsync(() => api.get<{ tenant: Tenant; leases: Lease[]; contact_url: string }>(`${V1}/tenants/${id}`), [id])
  const [edit, setEdit] = useState(false)
  const [del, setDel] = useState(false)
  const [busy, setBusy] = useState(false)
  useCrumb(data?.tenant.name)
  if (error) return <div className="page"><p className="bad">{error}</p></div>
  if (!data) return <Loading />
  const t = data.tenant
  const link = async () => {
    setBusy(true)
    try { await api.post(`${V1}/make`, { owner: 'tenant', owner_id: t.id, target: 'contact' }); toast('Linked to Pocket Contacts'); reload() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="page stack loose">
      <div className="det-head">
        <Avatar name={t.name} email={t.email} size={48} />
        <div className="grow"><h1 className="det-title">{t.name}</h1><p className="muted">{[t.email, t.phone].filter(Boolean).join(' · ') || 'No email or phone yet'}</p></div>
        <div className="row det-tools">
          {data.contact_url ? <a className="btn" href={data.contact_url} target="_blank" rel="noreferrer"><Icon name="users" size={15} />In Pocket Contacts</a>
            : <button type="button" className="btn" onClick={link} disabled={!contacts.ok || busy} title={contacts.ok ? 'Find them in Pocket Contacts, or add them there' : offTitle(contacts.app, 'contacts')}>{busy ? <Spinner /> : <Icon name="users" size={15} />}Link to Pocket Contacts</button>}
          <button type="button" className="btn" onClick={() => setEdit(true)}><Icon name="edit" size={15} />Change</button>
          <button type="button" className="btn ghost icon-only" aria-label="Delete" onClick={() => setDel(true)}><Icon name="trash" size={15} /></button>
        </div>
      </div>
      {t.notes && <p className="pre muted">{t.notes}</p>}
      <Card title="Leases" flush>{data.leases.length === 0 ? <p className="muted pad">No leases.</p> : <ul className="list">{data.leases.map(l => <LeaseRow key={l.id} l={l} />)}</ul>}</Card>
      {edit && <TenantModal tenant={t} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); reload(); announceChange() }} />}
      {del && <Confirm title={`Delete ${t.name}?`} message="Their leases stay, without them as the tenant." confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={async () => { try { await api.del(`${V1}/tenants/${t.id}`); announceChange(); navigate('/tenants') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

function TenantModal({ tenant, onClose, onSaved }: { tenant: Tenant; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: tenant.name, email: tenant.email, phone: tenant.phone, notes: tenant.notes })
  const [busy, setBusy] = useState(false)
  const save = async () => { setBusy(true); try { await api.put(`${V1}/tenants/${tenant.id}`, f); toast('Saved'); onSaved() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) } }
  return (
    <Modal title={`Change ${tenant.name}`} onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save} disabled={busy || !f.name.trim()}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button></>}>
      <div className="stack">
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus /></Field>
        <div className="two-col tight">
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
          <Field label="Phone"><input type="tel" value={f.phone} onChange={e => setF({ ...f, phone: e.target.value })} /></Field>
        </div>
        <Field label="Notes" help="A guarantor, how they like to be reached, anything to remember."><textarea rows={3} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
      </div>
    </Modal>
  )
}
