// Pick something from another Pocket app: a document from the archive, a Pocket Sign template.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtDate } from '../lib/format'
import { Icon } from './icons'
import { Loading, Modal, errMsg, useAsync, useDebounce } from './ui'

export function DocumentPicker({ onPick, onClose }: { onPick: (id: string) => void; onClose: () => void }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [res, setRes] = useState<{ documents: { id: string; title: string; created: string }[]; off?: string } | null>(null)
  useEffect(() => {
    let alive = true
    api.get<{ documents: { id: string; title: string; created: string }[]; off?: string }>(`${V1}/apps/documents?q=${encodeURIComponent(dq)}`).then(r => { if (alive) setRes(r) }).catch(e => { if (alive) setRes({ documents: [], off: errMsg(e) }) })
    return () => { alive = false }
  }, [dq])
  return (
    <Modal title="Attach a paper from Pocket Documents" onClose={onClose}>
      <div className="stack">
        <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Lease, insurance, ID…" autoFocus aria-label="Find a document" /></div>
        {!res ? <Loading /> : res.off ? <p className="muted small">{res.off}</p> : res.documents.length === 0 ? <p className="muted small">Nothing found.</p> : (
          <div className="pick-list tall">{res.documents.map(d => (
            <button key={d.id} type="button" className="pick-row" onClick={() => onPick(d.id)}><Icon name="file" size={15} /><span className="grow ellipsis">{d.title}</span><span className="muted tiny">{d.created && fmtDate(d.created)}</span></button>
          ))}</div>
        )}
      </div>
    </Modal>
  )
}

export function TemplatePicker({ onPick, onClose }: { onPick: (id: number, send: boolean) => void; onClose: () => void }) {
  const { data, error } = useAsync(() => api.get<{ id: string; name: string; roles: { id: number; name: string; email: string }[] }[]>(`${V1}/apps/options/templates`), [])
  const [pick, setPick] = useState('')
  const [send, setSend] = useState(true)
  return (
    <Modal title="Send the lease to sign" onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" disabled={!pick} onClick={() => onPick(Number(pick), send)}><Icon name="pen" size={15} />{send ? 'Send it' : 'Make it ready'}</button>
    </>}>
      <div className="stack">
        <p className="muted small">Pick your lease template in Pocket Sign. The tenants fill its signer places (those without a fixed email), in order.</p>
        {error ? <p className="bad small">{error}</p> : !data ? <Loading /> : data.length === 0 ? <p className="muted">You have no templates in Pocket Sign yet: make one there from your lease PDF.</p> : (
          <div className="pick-list">{data.map(t => (
            <button key={t.id} type="button" className={`pick-row ${pick === t.id ? 'on' : ''}`} onClick={() => setPick(t.id)}>
              <Icon name={pick === t.id ? 'check-circle' : 'circle'} size={15} /><span className="grow ellipsis">{t.name}</span><span className="muted tiny">{(t.roles || []).length} to sign</span>
            </button>
          ))}</div>
        )}
        <label className="check-row small"><input type="checkbox" checked={send} onChange={e => setSend(e.target.checked)} />Send it to them now (else it waits in Pocket Sign)</label>
      </div>
    </Modal>
  )
}
