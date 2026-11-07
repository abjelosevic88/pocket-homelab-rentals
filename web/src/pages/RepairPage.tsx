// One repair: where, what, how urgent, who fixes it and when, the cost,
// photos, and what happened — with a task, a calendar visit and the place's
// log in Pocket Home when those are connected.
import { useCallback, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useOnChange } from '../lib/data'
import { fmtDate, fmtMoney } from '../lib/format'
import { PRIORITY, REQ_STATUS, names, placeName } from '../lib/rentals'
import type { Entry, Expense, FileRec, Made, Request, RequestStatus, Tenant } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { MadeBox, Photos, Pill, Timeline } from '../components/Bits'
import { RequestForm } from '../components/Forms'
import { Icon } from '../components/icons'
import { Card, Confirm, Loading, MenuButton, Segmented, Spinner, errMsg, useAsync, useToast } from '../components/ui'

interface View { request: Request; timeline: Entry[]; made: Made[]; files: FileRec[]; expense?: Expense; lease?: { id: number; title: string; tenants: Tenant[] } }

export default function RepairPage() {
  const id = Number(useParams().id)
  const navigate = useNavigate()
  const { toast } = useToast()
  const { settings } = useApp()
  const appState = useAppState()
  const { data, reload, error } = useAsync(() => api.get<View>(`${V1}/requests/${id}`), [id])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const [edit, setEdit] = useState(false)
  const [del, setDel] = useState(false)
  const [busy, setBusy] = useState(false)
  const [tip, setTip] = useState<{ priority: string; trade: string; steps: string } | null>(null)
  useCrumb(data?.request.title)
  if (error) return <div className="page"><p className="bad">{error}</p></div>
  if (!data) return <Loading />
  const r = data.request
  const setStatus = async (status: RequestStatus) => {
    try {
      const out = await api.put<{ home?: string; money?: { ok: boolean; error?: string } }>(`${V1}/requests/${r.id}`, { status })
      if (out.home) toast(out.home, 'err')
      if (out.money && !out.money.ok && out.money.error) toast(`Pocket Money: ${out.money.error}`, 'err')
      reload(); announceChange()
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const make = async (target: string, ok: string) => { try { await api.post(`${V1}/make`, { owner: 'request', owner_id: r.id, target }); reload(); toast(ok) } catch (e) { toast(errMsg(e), 'err') } }
  const askAI = async () => { setBusy(true); try { setTip(await api.post(`${V1}/ai`, { want: 'repair', request_id: r.id })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) } }
  const calOn = appState('calendar', 'calendar:write').ok
  const inCal = data.made.some(m => m.target === 'event')
  const open = r.status !== 'done' && r.status !== 'cancelled'
  return (
    <div className="det-page">
      <div className="det-head">
        <span className={`det-ico ${r.priority === 'urgent' && open ? 'bad' : ''}`}><Icon name="wrench" size={22} /></span>
        <div className="grow">
          <div className="det-kicker"><Link to={`/properties/${r.property_id}`} className="small">{placeName(r.unit_name, r.property_name)}</Link>{r.priority !== 'normal' && <Pill cls={PRIORITY[r.priority].cls}>{PRIORITY[r.priority].label}</Pill>}</div>
          <h1 className="det-title">{r.title}</h1>
          <p className="muted">Reported {fmtDate(r.reported_on)}{r.reported_by ? ` by ${r.reported_by}` : ''}{r.source_app ? ` · from Pocket ${r.source_app}` : ''}</p>
        </div>
        <div className="row det-tools">
          {open && <button type="button" className="btn primary" onClick={() => setStatus('done')}><Icon name="check" size={15} />It is fixed</button>}
          <button type="button" className="btn" onClick={() => setEdit(true)}><Icon name="edit" size={15} />Change</button>
          <MenuButton label="More" className="btn icon-only" align="right" items={[
            ...(open ? [{ label: 'Cancel it', icon: 'x', onSelect: () => setStatus('cancelled') }] : [{ label: 'Open it again', icon: 'undo', onSelect: () => setStatus('new') }]),
            { label: 'Delete', icon: 'trash', danger: true, onSelect: () => setDel(true) },
          ]}><Icon name="more" size={16} /></MenuButton>
        </div>
      </div>
      <div className="scroll-x"><Segmented value={r.status} onChange={setStatus} options={(Object.keys(REQ_STATUS) as RequestStatus[]).map(s => ({ value: s, label: REQ_STATUS[s].label, disabled: s === 'planned' && !r.planned_on, title: s === 'planned' && !r.planned_on ? 'Set a day first (Change)' : undefined }))} /></div>
      <div className="det-grid">
        <div className="stack loose min0">
          {r.details && <Card title="What is wrong"><p className="pre m0">{r.details}</p></Card>}
          {tip && <div className="callout info ai-tip"><Icon name="sparkles" size={15} /><div><b>{tip.trade || 'Who fixes it'}</b> · {tip.priority}<p className="pre m0">{tip.steps}</p></div></div>}
          <Card title="Photos"><Photos owner="request" ownerId={r.id} files={data.files} onChange={reload} /></Card>
          <Card title="What happened"><Timeline entries={data.timeline} onAdd={async e => { await api.post(`${V1}/entries`, { ...e, request_id: r.id }); reload() }} onDelete={async eid => { await api.del(`${V1}/entries/${eid}`); reload() }} /></Card>
        </div>
        <aside className="stack">
          <div className="side-card card">
            <h4 className="side-h">Who and when</h4>
            <dl className="facts">
              <dt>Fixed by</dt><dd>{r.pro_name || <span className="muted">not said</span>}{r.pro_phone && <> · <a href={`tel:${r.pro_phone.replace(/\s/g, '')}`}>{r.pro_phone}</a></>}</dd>
              <dt>Day</dt><dd>{r.planned_on ? `${fmtDate(r.planned_on)}${r.planned_time ? ', ' + r.planned_time : ''}` : <span className="muted">not planned</span>}</dd>
              {r.status === 'done' && <><dt>Done</dt><dd>{fmtDate(r.done_on)}</dd></>}
              <dt>Cost</dt><dd>{r.cost ? fmtMoney(r.cost, r.currency) : <span className="muted">—</span>}{r.tenant_pays && ' · the tenant pays'}</dd>
              {data.lease && <><dt>Tenant</dt><dd><Link to={`/leases/${data.lease.id}`}>{names(data.lease.tenants) || data.lease.title}</Link></dd></>}
            </dl>
            {data.expense && <p className="small m0 muted"><Icon name="receipt" size={12} className="inline-ico" /> In the costs of the place{data.expense.money_tx_ref ? ' and in Pocket Money' : ''}.</p>}
            {r.planned_on && !calOn && open && <a className="btn sm self-start" href={`${V1}/ics/request/${r.id}`}><Icon name="calendar" size={14} />As a calendar file</a>}
            {r.planned_on && calOn && open && !inCal && !settings?.calendar_sync && <p className="muted tiny m0">Switch on “Put days in Pocket Calendar” (Settings → Rent) to have visits there.</p>}
          </div>
          <MadeBox made={data.made} onForget={async mid => { await api.del(`${V1}/made/${mid}`); reload() }} actions={[
            { target: 'task', label: 'As a task', icon: 'check-square', app: 'tasks', scope: 'tasks:write', run: () => make('task', 'Task made in Pocket Tasks'), hide: !!r.task_ref || !open },
            { target: 'note', label: 'As a note', icon: 'note', app: 'notes', scope: 'notes:write', run: () => make('note', 'Saved in Pocket Notes') },
          ]} />
          {settings?.ai_help && open && <button type="button" className="btn sm" onClick={askAI} disabled={busy}>{busy ? <Spinner /> : <Icon name="sparkles" size={14} />}Ask Pocket AI what to do first</button>}
        </aside>
      </div>
      {edit && <RequestForm request={r} onClose={() => setEdit(false)} onSaved={() => reload()} />}
      {del && <Confirm title="Delete this repair?" message="Its photos and timeline go too. A task or a calendar visit made for it is removed from the calendar; the task stays in Pocket Tasks." confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={async () => { try { await api.del(`${V1}/requests/${r.id}`); announceChange(); navigate('/repairs') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}
