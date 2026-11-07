// Repairs: what is open (urgent first), what is done, and what tenants
// reported through your form in Pocket Forms.
import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useAppState, useData, useOnChange } from '../lib/data'
import { ago, fmtDate, fmtMoney } from '../lib/format'
import { PRIORITY, REQ_STATUS, placeName } from '../lib/rentals'
import type { Request } from '../lib/types'
import { openCreate } from '../components/Layout'
import { Pill } from '../components/Bits'
import { AppOff } from '../components/Apps'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, Modal, Segmented, Spinner, errMsg, useAsync, useToast } from '../components/ui'

export default function Repairs() {
  const { properties } = useData()
  const [show, setShow] = useState<'open' | 'done' | 'all'>('open')
  const [prop, setProp] = useState('')
  const { data, reload } = useAsync(() => api.get<Request[]>(`${V1}/requests${prop ? `?property=${prop}` : ''}`), [prop])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const list = useMemo(() => (data || []).filter(r => show === 'all' || (show === 'open' ? r.status !== 'done' && r.status !== 'cancelled' : r.status === 'done' || r.status === 'cancelled')), [data, show])
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Repairs</h1><p className="muted">What needs fixing, who fixes it, and what it cost.</p></div>
        <button type="button" className="btn primary" onClick={() => openCreate('repair', prop ? { property_id: Number(prop) } : {})}><Icon name="plus" size={15} />Report a repair</button>
      </div>
      <FromForms onTaken={reload} />
      <div className="row wrap filters">
        <Segmented size="sm" value={show} onChange={setShow} options={[{ value: 'open', label: 'To do' }, { value: 'done', label: 'Done' }, { value: 'all', label: 'All' }]} />
        {(properties || []).length > 1 && <Select ariaLabel="Place" value={prop} onChange={setProp} placeholder="Every place" clearLabel="Every place" options={(properties || []).map(p => ({ value: String(p.id), label: p.name }))} />}
      </div>
      {!data ? <Loading /> : list.length === 0 ? <Empty icon="wrench" title={show === 'open' ? 'Nothing to fix' : 'No repairs here'} /> : (
        <Card flush><ul className="list">{list.map(r => (
          <li key={r.id}><Link to={`/repairs/${r.id}`} className="list-row">
            <span className={`list-ico ${r.priority === 'urgent' && r.status !== 'done' ? 'st-overdue' : r.status === 'done' ? 'st-done' : ''}`}><Icon name={REQ_STATUS[r.status].icon} size={15} /></span>
            <span className="row-main grow"><span className="row-title">{r.title}{r.photos > 0 && <Icon name="camera" size={12} />}</span>
              <span className="muted small ellipsis">{placeName(r.unit_name, r.property_name)}{r.reported_by ? ` · ${r.reported_by}` : ''} · {r.status === 'done' ? `done ${fmtDate(r.done_on)}` : r.planned_on ? `${fmtDate(r.planned_on)}${r.planned_time ? ' ' + r.planned_time : ''}${r.pro_name ? ', ' + r.pro_name : ''}` : `reported ${fmtDate(r.reported_on)}`}</span></span>
            {r.cost > 0 && <span className="row-amount tnum small">{fmtMoney(r.cost, r.currency)}</span>}
            {r.priority === 'urgent' && r.status !== 'done' && r.status !== 'cancelled' ? <Pill cls={PRIORITY.urgent.cls} icon="alert">Urgent</Pill> : <Pill cls={REQ_STATUS[r.status].cls}>{REQ_STATUS[r.status].label}</Pill>}
          </Link></li>))}</ul></Card>
      )}
    </div>
  )
}

interface Answer { id: string; form: string; name: string; email: string; preview: string; at: string; url: string; tenant?: string; request_id?: number; request_title?: string }

/** Answers to your repair form in Pocket Forms, one click from being a repair. */
function FromForms({ onTaken }: { onTaken: () => void }) {
  const appState = useAppState()
  const forms = appState('forms', 'forms:read')
  const { properties } = useData()
  const { toast } = useToast()
  const { data, reload } = useAsync(() => forms.ok ? api.get<{ items: Answer[]; state: string }>(`${V1}/form-answers`).catch(() => null) : Promise.resolve(null), [forms.ok])
  const [busy, setBusy] = useState('')
  const [pick, setPick] = useState<Answer | null>(null)
  if (forms.loading) return null
  if (!forms.ok) return forms.app?.state === 'missing' ? null : <AppOff slug="forms" app={forms.app} compact>Repairs your tenants report through a form would show here.</AppOff>
  const items = (data?.items || []).filter(a => !a.request_id)
  if (!items.length) return null
  const take = async (a: Answer, property_id?: number) => {
    setBusy(a.id)
    try { await api.post(`${V1}/form-answers/${a.id}/take`, property_id ? { property_id } : {}); toast('Repair added'); setPick(null); reload(); onTaken(); announceChange() }
    catch (e) { if (!property_id && (properties || []).length > 0) setPick(a); else toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  return (
    <Card title={<span className="sec-title"><Icon name="list" size={15} />Reported in Pocket Forms<span className="count">{items.length}</span></span>} flush>
      <ul className="list">{items.map(a => (
        <li key={a.id} className="list-row">
          <span className="list-ico st-new"><Icon name="inbox" size={15} /></span>
          <span className="row-main grow"><span className="row-title">{a.tenant || a.name || a.email || 'Someone'}<span className="muted small"> · {a.form}</span></span><span className="muted small ellipsis">{a.preview}</span><span className="muted tiny">{ago(a.at)}</span></span>
          {a.url && <a className="btn ghost sm hide-sm" href={a.url} target="_blank" rel="noreferrer" title="Open in Pocket Forms"><Icon name="external" size={14} /></a>}
          <button type="button" className="btn sm" onClick={() => take(a)} disabled={!!busy}>{busy === a.id ? <Spinner /> : <Icon name="plus" size={14} />}Make it a repair</button>
        </li>))}</ul>
      {pick && <Modal title="Which place is it?" onClose={() => setPick(null)}>
        <div className="stack">
          <p className="muted small">The answer does not say, and {pick.email || 'its email'} is not one of your tenants.</p>
          <div className="pick-list">{(properties || []).map(p => <button key={p.id} type="button" className="pick-row" onClick={() => take(pick, p.id)}><Icon name="home" size={15} /><span className="grow">{p.name}</span></button>)}</div>
        </div>
      </Modal>}
    </Card>
  )
}
