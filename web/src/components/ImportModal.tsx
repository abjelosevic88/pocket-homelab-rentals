// Bring leases in: a Pocket Rentals export, or a spreadsheet of leases or
// of rent payments. Bringing the same file again adds nothing twice.
import { useState } from 'react'
import { V1 } from '../lib/api'
import { announceChange } from '../lib/data'
import { Icon } from './icons'
import { Modal, Spinner, errMsg, useToast } from './ui'

const WORDS: Record<string, [string, string]> = {
  properties: ['place', 'places'], units: ['unit', 'units'], tenants: ['tenant', 'tenants'], leases: ['lease', 'leases'], payments: ['payment', 'payments'],
  requests: ['repair', 'repairs'], expenses: ['cost', 'costs'], inspections: ['inspection', 'inspections'],
}

export function ImportModal({ onClose }: { onClose: () => void }) {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<Record<string, number> | null>(null)
  const send = async (file: File) => {
    setBusy(true)
    try {
      const fd = new FormData()
      fd.append('file', file)
      const res = await fetch(`${V1}/import`, { method: 'POST', body: fd, credentials: 'same-origin' })
      const j = await res.json()
      if (!res.ok) throw new Error(j.error || res.statusText)
      setDone(j.added || {})
      announceChange()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const lines = done ? Object.entries(WORDS).filter(([k]) => done[k]).map(([k, [one, many]]) => `${done[k]} ${done[k] === 1 ? one : many}`) : []
  return (
    <Modal title="Bring your rentals" onClose={onClose} size="sm" footer={<button type="button" className="btn" onClick={onClose}>{done ? 'Done' : 'Cancel'}</button>}>
      {done ? <div className="stack">
        <p><Icon name="check-circle" size={16} className="inline-ico ok" /> {lines.length ? lines.join(', ') + ' added' : 'Nothing new: it was all here already'}.</p>
        {!!done.skipped && <p className="muted small">{done.skipped} {done.skipped === 1 ? 'line was' : 'lines were'} here already or had no place and rent.</p>}
        {!!done.unmatched && <p className="muted small">{done.unmatched} {done.unmatched === 1 ? 'payment' : 'payments'} did not match a tenant or a place here.</p>}
      </div> : (
        <div className="stack">
          <p className="muted">A spreadsheet saved as CSV (Excel, Numbers or Google Sheets: File → Download → CSV), or a Pocket Rentals export.</p>
          <p className="muted small"><b>Leases:</b> one per row with the place (or its address), the unit, the tenant, their email and phone, the rent, the start and end, the deposit. Rent from before this month is not counted as owed.</p>
          <p className="muted small"><b>Payments:</b> one per row with the day, the amount and who paid (their name or email) or the place.</p>
          <label className="btn primary file-btn">{busy ? <Spinner /> : <Icon name="upload" size={15} />}Choose a file<input type="file" accept=".csv,.tsv,.txt,.json,text/csv,application/json" hidden onChange={e => { const f = e.target.files?.[0]; if (f) send(f) }} disabled={busy} /></label>
          <p className="muted small">Bringing the same file again adds nothing twice.</p>
        </div>
      )}
    </Modal>
  )
}
