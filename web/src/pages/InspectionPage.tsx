// One inspection: room by room, how each thing looks, a note and photos —
// for moving out, how it looked when they moved in, side by side.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useOnChange } from '../lib/data'
import { fmtDate, fmtMoney } from '../lib/format'
import { CONDITIONS, INSPECTION_KIND, names, placeName } from '../lib/rentals'
import type { Condition, FileRec, Inspection, InspectionItem, Made, Tenant } from '../lib/types'
import { openCreate, useCrumb } from '../components/Layout'
import { MadeBox, Photos } from '../components/Bits'
import { Icon } from '../components/icons'
import { Confirm, Loading, MenuButton, Spinner, errMsg, useAsync, useToast } from '../components/ui'

interface Before { id: number; kind: string; date: string; items: Record<string, { condition: Condition; note: string; photos: FileRec[] }> }
interface View { inspection: Inspection; before?: Before; lease?: { id: number; title: string; tenants: Tenant[]; deposit: number; deposit_state: string; currency: string }; made: Made[] }

export default function InspectionPage() {
  const id = Number(useParams().id)
  const navigate = useNavigate()
  const { toast } = useToast()
  const { settings } = useApp()
  const appState = useAppState()
  const { data, reload, error } = useAsync(() => api.get<View>(`${V1}/inspections/${id}`), [id])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const [items, setItems] = useState<InspectionItem[] | null>(null)
  const [saving, setSaving] = useState<'' | 'saving' | 'saved' | 'error'>('')
  const [summary, setSummary] = useState('')
  const [ai, setAI] = useState('')
  const [busy, setBusy] = useState('')
  const [del, setDel] = useState(false)
  const [newRoom, setNewRoom] = useState('')
  const timer = useRef<number>(0)
  useEffect(() => { if (data) { setItems(data.inspection.items || []); setSummary(data.inspection.summary) } }, [data?.inspection.id]) // eslint-disable-line react-hooks/exhaustive-deps
  useCrumb(data ? INSPECTION_KIND[data.inspection.kind] : undefined)
  const rooms = useMemo(() => { const out: string[] = []; for (const it of items || []) if (!out.includes(it.room)) out.push(it.room); return out }, [items])
  if (error) return <div className="page"><p className="bad">{error}</p></div>
  if (!data || !items) return <Loading />
  const ins = data.inspection
  const save = (next: InspectionItem[]) => {
    setItems(next)
    setSaving('saving')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(async () => {
      try {
        const r = await api.put<View>(`${V1}/inspections/${ins.id}/items`, next.map(({ id, room, item, condition, note }) => ({ id, room, item, condition, note })))
        setItems(cur => (r.inspection.items || []).map((it, i) => ({ ...it, photos: it.photos.length ? it.photos : cur?.[i]?.photos || [] })))
        setSaving('saved')
      } catch (e) { setSaving('error'); toast(errMsg(e), 'err') }
    }, 600)
  }
  const setItem = (i: number, patch: Partial<InspectionItem>) => save(items.map((it, j) => j === i ? { ...it, ...patch } : it))
  const put = async (patch: Record<string, unknown>) => { try { await api.put(`${V1}/inspections/${ins.id}`, patch); reload(); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  const askAI = async () => { setBusy('ai'); try { const r = await api.post<{ text: string }>(`${V1}/ai`, { want: 'inspection', inspection_id: ins.id }); setAI(r.text) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') } }
  const note = async () => { setBusy('note'); try { await api.post(`${V1}/make`, { owner: 'inspection', owner_id: ins.id, target: 'note' }); toast('Saved in Pocket Notes'); reload() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') } }
  const issues = items.filter(i => ['poor', 'damaged', 'missing'].includes(i.condition))
  const before = data.before
  const was = (it: InspectionItem) => before?.items[`${it.room}|${it.item}`.toLowerCase()]
  const worse = (it: InspectionItem) => { const b = was(it); const rank = (c: string) => ['good', 'fair', 'poor', 'damaged', 'missing'].indexOf(c); return !!b && !!it.condition && !!b.condition && rank(it.condition) > rank(b.condition) }
  return (
    <div className="det-page insp-page">
      <div className="det-head">
        <span className="det-ico"><Icon name="check-square" size={22} /></span>
        <div className="grow">
          <div className="det-kicker"><Link to={`/properties/${ins.property_id}`} className="small">{placeName(ins.unit_name, ins.property_name)}</Link>{data.lease && <Link to={`/leases/${data.lease.id}`} className="small muted">· {names(data.lease.tenants)}</Link>}</div>
          <h1 className="det-title">{INSPECTION_KIND[ins.kind]} inspection</h1>
          <p className="muted">{fmtDate(ins.date)}{ins.time ? `, ${ins.time}` : ''} · {ins.status === 'done' ? 'done' : 'planned'}{before ? ` · compared with ${INSPECTION_KIND[before.kind].toLowerCase()} on ${fmtDate(before.date)}` : ''}</p>
        </div>
        <div className="row det-tools">
          <span className={`save-state ${saving === 'error' ? 'error' : ''}`}>{saving === 'saving' ? 'Saving…' : saving === 'saved' ? 'Saved' : saving === 'error' ? 'Not saved' : ''}</span>
          {ins.status === 'planned' ? <button type="button" className="btn primary" onClick={() => put({ status: 'done', summary })}><Icon name="check" size={15} />Done</button>
            : <button type="button" className="btn" onClick={() => put({ status: 'planned' })}><Icon name="undo" size={15} />Not done yet</button>}
          <MenuButton label="More" className="btn icon-only" align="right" items={[
            { label: 'Delete', icon: 'trash', danger: true, onSelect: () => setDel(true) },
          ]}><Icon name="more" size={16} /></MenuButton>
        </div>
      </div>
      <div className="det-grid">
        <div className="stack loose min0">
          {rooms.map(room => (
            <section key={room} className="card room-card">
              <h3 className="room-h">{room || 'Other'}</h3>
              {items.map((it, i) => it.room !== room ? null : (
                <div key={it.id || `n${i}`} className={`insp-line ${worse(it) ? 'worse' : ''}`}>
                  <div className="insp-top">
                    <input className="insp-item" value={it.item} onChange={e => setItem(i, { item: e.target.value })} aria-label="Thing" />
                    <div className="cond-btns" role="group" aria-label={`How ${it.item} looks`}>
                      {CONDITIONS.map(c => <button key={c.value} type="button" className={`cond ${c.cls} ${it.condition === c.value ? 'on' : ''}`} aria-pressed={it.condition === c.value} onClick={() => setItem(i, { condition: it.condition === c.value ? '' : c.value })}>{c.label}</button>)}
                    </div>
                    <button type="button" className="btn ghost icon-only sm" aria-label="Remove the line" onClick={() => save(items.filter((_, j) => j !== i))}><Icon name="x" size={13} /></button>
                  </div>
                  {was(it) && <div className="insp-was small">Before: <b>{CONDITIONS.find(c => c.value === was(it)!.condition)?.label || '—'}</b>{was(it)!.note ? ` — ${was(it)!.note}` : ''}{was(it)!.photos.length > 0 && <span className="muted"> · {was(it)!.photos.length} photo{was(it)!.photos.length > 1 ? 's' : ''}</span>}</div>}
                  <div className="insp-bottom">
                    <input className="insp-note" value={it.note} onChange={e => setItem(i, { note: e.target.value })} placeholder="A note (a reading, a mark, what is missing)" aria-label="Note" />
                    {it.id ? <Photos owner="item" ownerId={it.id} files={it.photos} onChange={reload} compact /> : null}
                    {['poor', 'damaged', 'missing'].includes(it.condition) && <button type="button" className="btn ghost sm" title="Make a repair from this line"
                      onClick={() => openCreate('repair', { property_id: ins.property_id, unit_id: ins.unit_id || 0, lease_id: ins.lease_id || 0, title: `${it.item} (${room})`, details: it.note })}><Icon name="wrench" size={13} /><span className="hide-sm">Repair</span></button>}
                  </div>
                </div>
              ))}
              <button type="button" className="link-btn" onClick={() => save([...items, { id: 0, room, item: 'Something else', condition: '', note: '', photos: [] }])}><Icon name="plus" size={13} />A line in {room || 'this room'}</button>
            </section>
          ))}
          <div className="row tight">
            <input value={newRoom} onChange={e => setNewRoom(e.target.value)} placeholder="Another room (Balcony, Cellar…)" aria-label="Another room" />
            <button type="button" className="btn" disabled={!newRoom.trim()} onClick={() => { save([...items, { id: 0, room: newRoom.trim(), item: 'Walls and floor', condition: '', note: '', photos: [] }]); setNewRoom('') }}><Icon name="plus" size={14} />Add</button>
          </div>
        </div>
        <aside className="stack">
          <div className="side-card card">
            <h4 className="side-h">In short</h4>
            <textarea rows={4} value={summary} onChange={e => setSummary(e.target.value)} onBlur={() => summary !== ins.summary && put({ summary })} placeholder="How it was overall, keys handed over, meter readings" />
            {issues.length > 0 && <p className="small m0"><Icon name="alert" size={12} className="inline-ico" /> {issues.length} {issues.length === 1 ? 'thing' : 'things'} to look at: {issues.slice(0, 4).map(i => i.item).join(', ')}{issues.length > 4 ? '…' : ''}</p>}
            {settings?.ai_help && <button type="button" className="btn sm self-start" onClick={askAI} disabled={!!busy}>{busy === 'ai' ? <Spinner /> : <Icon name="sparkles" size={14} />}{ins.kind === 'move_out' && before ? 'Pocket AI: what to keep from the deposit' : 'Pocket AI: sum it up'}</button>}
            {ai && <div className="ai-sum small"><p className="pre m0">{ai}</p><button type="button" className="link-btn small" onClick={() => { setSummary(s => (s ? s + '\n\n' : '') + ai); put({ summary: (summary ? summary + '\n\n' : '') + ai }) }}>Put it in the summary</button></div>}
          </div>
          {data.lease && data.lease.deposit > 0 && ins.kind === 'move_out' && <div className="side-card card">
            <h4 className="side-h">The deposit</h4>
            <p className="small m0">{fmtMoney(data.lease.deposit, data.lease.currency)} · {data.lease.deposit_state === 'held' ? 'you hold it' : data.lease.deposit_state.replace('_', ' ')}</p>
            <Link to={`/leases/${data.lease.id}`} className="btn sm self-start">Settle it on the lease</Link>
          </div>}
          <MadeBox made={data.made} actions={[{ target: 'note', label: 'As a note', icon: 'note', app: 'notes', scope: 'notes:write', run: note }]} />
          {!appState('calendar', 'calendar:write').ok && ins.status === 'planned' && <a className="btn sm" href={`${V1}/ics/inspection/${ins.id}`}><Icon name="calendar" size={14} />As a calendar file</a>}
        </aside>
      </div>
      {del && <Confirm title="Delete this inspection?" message="Its lines and photos are deleted." confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={async () => { try { await api.del(`${V1}/inspections/${ins.id}`); announceChange(); navigate('/inspections') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}
