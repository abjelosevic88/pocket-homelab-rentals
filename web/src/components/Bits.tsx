// Pieces the pages share: a timeline with a note box, what something has in
// the other Pocket apps (greyed with what to do when an app is off), photos.
import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, upload, V1 } from '../lib/api'
import { APP_INFO, ENTRY_ICON, TARGET_APP, TARGET_LABEL } from '../lib/rentals'
import { useAppState } from '../lib/data'
import { ago, fmtDateTime } from '../lib/format'
import type { AppSlug, Entry, FileRec, Made } from '../lib/types'
import { offTitle } from './Apps'
import { Icon } from './icons'
import { Confirm, Spinner, errMsg, useToast } from './ui'

export function Pill({ cls, icon, children, title }: { cls: string; icon?: string; children: React.ReactNode; title?: string }) {
  return <span className={`pill ${cls}`} title={title}>{icon && <Icon name={icon} size={11} />}{children}</span>
}

/** What happened, newest first, with a box to write a line. */
export function Timeline({ entries, onAdd, onDelete, kinds = true }: { entries: Entry[]; onAdd?: (e: { kind: string; title: string; body: string }) => Promise<void>; onDelete?: (id: number) => void; kinds?: boolean }) {
  const [kind, setKind] = useState('note')
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState<Entry | null>(null)
  const add = async () => {
    const t = text.trim()
    if (!t || !onAdd) return
    setBusy(true)
    const [first, ...rest] = t.split('\n')
    try { await onAdd({ kind, title: first.slice(0, 300), body: rest.join('\n').trim() }); setText('') } finally { setBusy(false) }
  }
  return (
    <div className="stack">
      {onAdd && <div className="tl-add">
        {kinds && <div className="segmented sm tl-kinds" role="group" aria-label="What it was">
          {[['note', 'Note', 'note'], ['call', 'Call', 'phone'], ['email', 'Email', 'mail'], ['visit', 'Visit', 'users']].map(([v, l, i]) => (
            <button key={v} type="button" className={kind === v ? 'active' : ''} aria-pressed={kind === v} onClick={() => setKind(v)}><Icon name={i} size={13} />{l}</button>
          ))}
        </div>}
        <div className="row tight">
          <textarea rows={1} value={text} onChange={e => setText(e.target.value)} placeholder="Write what happened (“Called Ana about the rent”)" aria-label="Write what happened"
            onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); add() } }} />
          <button type="button" className="btn" onClick={add} disabled={busy || !text.trim()}>{busy ? <Spinner /> : 'Add'}</button>
        </div>
      </div>}
      {entries.length === 0 ? <p className="muted small">Nothing yet.</p> : (
        <ol className="timeline">
          {entries.map(e => (
            <li key={e.id} className={`tl tl-${e.kind}`}>
              <span className="tl-dot"><Icon name={ENTRY_ICON[e.kind] || 'dot'} size={14} /></span>
              <div className="tl-body">
                <div className="tl-h">
                  <span className="tl-t">{e.url ? (e.url.startsWith('/') ? <Link to={e.url}>{e.title}</Link> : <a href={e.url} target="_blank" rel="noreferrer">{e.title}</a>) : e.title}</span>
                  <span className="muted tiny tl-when" title={fmtDateTime(e.at)}>{ago(e.at)}</span>
                  {onDelete && ['note', 'call', 'email', 'visit'].includes(e.kind) && <button type="button" className="btn ghost icon-only sm tl-x" aria-label="Remove" onClick={() => setDel(e)}><Icon name="x" size={13} /></button>}
                </div>
                {e.body && <p className="tl-text pre">{e.body}</p>}
              </div>
            </li>
          ))}
        </ol>
      )}
      {del && onDelete && <Confirm title="Remove this line?" message={del.title} confirmLabel="Remove" onCancel={() => setDel(null)} onConfirm={() => { onDelete(del.id); setDel(null) }} />}
    </div>
  )
}

export interface MakeAction { target: string; label: string; icon: string; app: AppSlug; scope?: string; run: () => void; hide?: boolean }

/** "In your other apps": what was made, and buttons to make more — greyed, never hidden. */
export function MadeBox({ made, actions, onForget }: { made: Made[]; actions: MakeAction[]; onForget?: (id: number) => void }) {
  const appState = useAppState()
  const shown = made.filter(m => m.target !== 'answer' || m.url)
  return (
    <div className="side-card card">
      <h4 className="side-h">In your other apps</h4>
      {shown.length > 0 && <ul className="made-list">
        {shown.map(m => (
          <li key={m.id} className={m.error ? 'bad' : ''}>
            <Icon name={APP_INFO[TARGET_APP[m.target] || 'notes']?.icon || 'link'} size={14} />
            <span className="grow min0 ellipsis">{m.url ? <a href={m.url} target="_blank" rel="noreferrer">{m.title || TARGET_LABEL[m.target]}</a> : (m.title || TARGET_LABEL[m.target])}</span>
            {m.state && <span className={`state st-${m.state}`}>{m.state.replace('_', ' ')}</span>}
            {onForget && <button type="button" className="btn ghost icon-only sm" aria-label="Forget the link" title="Forget the link (it stays in the other app)" onClick={() => onForget(m.id)}><Icon name="x" size={12} /></button>}
          </li>
        ))}
      </ul>}
      <div className="make-grid">
        {actions.filter(a => !a.hide).map(a => {
          const st = appState(a.app, a.scope)
          return (
            <button key={a.target + a.label} type="button" className={`make-btn ${st.ok ? '' : 'off'}`} onClick={a.run} disabled={!st.ok} title={st.ok ? `${a.label} — ${APP_INFO[a.app].name}` : offTitle(st.app, a.app)}>
              <Icon name={a.icon} size={16} /><span>{a.label}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

/** Photos and PDFs on something: add, look, remove. */
export function Photos({ owner, ownerId, files, onChange, compact = false }: { owner: string; ownerId: number; files: FileRec[]; onChange: () => void; compact?: boolean }) {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const [view, setView] = useState<FileRec | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const add = async (list: FileList | null) => {
    if (!list?.length) return
    setBusy(true)
    try {
      for (const f of Array.from(list)) await upload(`${V1}/files?owner=${owner}&owner_id=${ownerId}`, 'file', f)
      onChange()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false); if (input.current) input.current.value = '' }
  }
  const remove = async (f: FileRec) => { try { await api.del(`${V1}/files/${f.id}`); setView(null); onChange() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <div className={`photos ${compact ? 'compact' : ''}`}>
      {files.map(f => (
        <button key={f.id} type="button" className="photo" onClick={() => f.mime.startsWith('image/') ? setView(f) : window.open(`${V1}/files/${f.id}`, '_blank')} title={f.name}>
          {f.mime.startsWith('image/') ? <img src={`${V1}/files/${f.id}`} alt={f.name} loading="lazy" /> : <span className="photo-file"><Icon name="file" size={18} /><span className="tiny ellipsis">{f.name}</span></span>}
        </button>
      ))}
      <label className={`photo add ${busy ? 'busy' : ''}`} title="Add a photo">
        {busy ? <Spinner /> : <Icon name="camera" size={compact ? 15 : 18} />}
        {!compact && <span className="tiny">Add</span>}
        <input ref={input} type="file" accept="image/*,application/pdf" capture={compact ? 'environment' : undefined} multiple hidden onChange={e => add(e.target.files)} disabled={busy} />
      </label>
      {view && <div className="photo-view" role="dialog" aria-label={view.name} onClick={() => setView(null)}>
        <img src={`${V1}/files/${view.id}`} alt={view.name} />
        <div className="photo-view-bar" onClick={e => e.stopPropagation()}>
          <span className="ellipsis">{view.name}</span>
          <a className="btn sm" href={`${V1}/files/${view.id}?download=1`}><Icon name="download" size={14} />Download</a>
          <button type="button" className="btn sm danger-text" onClick={() => remove(view)}><Icon name="trash" size={14} />Remove</button>
          <button type="button" className="btn sm" onClick={() => setView(null)}><Icon name="x" size={14} />Close</button>
        </div>
      </div>}
    </div>
  )
}

/** A line saying an app is not there, for a list it would fill. */
export function offLine(st: { ok: boolean; app?: { state: string; fix: string; name: string } }, what: string) {
  if (st.ok || !st.app) return null
  return <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {st.app.state === 'missing' ? `${st.app.name} is not on this box: ${what} would show here.` : `Connect ${st.app.name} to see ${what} here. ${st.app.fix || ''}`}</p>
}
