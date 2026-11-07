// What the places cost: every expense, by place and kind, booked in Pocket Money or not.
import { useCallback, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData, useOnChange } from '../lib/data'
import { fmtDate, fmtMoney, fmtMoneyMap } from '../lib/format'
import { EXPENSE_KINDS, expenseLabel } from '../lib/rentals'
import type { Expense, Money } from '../lib/types'
import { ExpenseForm } from '../components/Forms'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { Card, Confirm, Empty, Loading, errMsg, useAsync, useToast } from '../components/ui'

export default function Costs() {
  const { settings } = useApp()
  const { properties } = useData()
  const { toast } = useToast()
  const [sp, setSp] = useSearchParams()
  const prop = sp.get('property') || ''
  const [year, setYear] = useState(String(new Date().getFullYear()))
  const [kind, setKind] = useState('')
  const { data, reload } = useAsync(() => api.get<Expense[]>(`${V1}/expenses?from=${year}-01-01&to=${year}-12-31${prop ? `&property=${prop}` : ''}`), [year, prop])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const [edit, setEdit] = useState<Expense | 'new' | null>(null)
  const [del, setDel] = useState<Expense | null>(null)
  const list = useMemo(() => (data || []).filter(e => !kind || e.category === kind), [data, kind])
  const total: Money = {}
  for (const e of list) total[e.currency] = (total[e.currency] || 0) + e.amount
  const years = Array.from({ length: 6 }, (_, i) => String(new Date().getFullYear() - i))
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Costs</h1><p className="muted">{fmtMoneyMap(total, settings?.currency || 'EUR')} in {year}{kind ? ` on ${expenseLabel(kind).toLowerCase()}` : ''}.</p></div>
        <button type="button" className="btn primary" onClick={() => setEdit('new')}><Icon name="plus" size={15} />Add a cost</button>
      </div>
      <div className="row wrap filters">
        <Select ariaLabel="Year" value={year} onChange={setYear} options={years.map(y => ({ value: y, label: y }))} />
        <Select ariaLabel="Place" value={prop} onChange={v => setSp(v ? { property: v } : {})} placeholder="Every place" clearLabel="Every place" options={(properties || []).map(p => ({ value: String(p.id), label: p.name }))} />
        <Select ariaLabel="Kind" value={kind} onChange={setKind} placeholder="Every kind" clearLabel="Every kind" options={EXPENSE_KINDS} />
      </div>
      {!data ? <Loading /> : list.length === 0 ? <Empty icon="receipt" title="No costs here" /> : (
        <Card flush><ul className="list">{list.map(e => (
          <li key={e.id} className="list-row clickable" onClick={() => setEdit(e)}>
            <span className="list-ico"><Icon name="receipt" size={15} /></span>
            <span className="row-main grow"><span className="row-title">{expenseLabel(e.category)}{e.note && <span className="muted small"> · {e.note}</span>}</span><span className="muted small ellipsis">{e.property_name} · {fmtDate(e.date)}{e.payee ? ` · ${e.payee}` : ''}</span></span>
            {e.money_url && <a href={e.money_url} target="_blank" rel="noreferrer" className="pill st-ok" onClick={ev => ev.stopPropagation()} title={e.money_found ? 'Found in Pocket Money' : 'Booked in Pocket Money'}><Icon name="wallet" size={11} />Money</a>}
            <span className="row-amount tnum"><b>{fmtMoney(e.amount, e.currency)}</b></span>
            <button type="button" className="btn ghost icon-only sm" aria-label="Delete" onClick={ev => { ev.stopPropagation(); setDel(e) }}><Icon name="trash" size={14} /></button>
          </li>))}</ul></Card>
      )}
      {edit && <ExpenseForm expense={edit === 'new' ? undefined : edit} initial={prop ? { property_id: Number(prop) } : {}} onClose={() => setEdit(null)} onSaved={reload} />}
      {del && <Confirm title="Delete this cost?" message={del.money_tx_ref && !del.money_found ? 'It is removed from Pocket Money too.' : 'It is removed here.'} confirmLabel="Delete" onCancel={() => setDel(null)}
        onConfirm={async () => { const d = del; setDel(null); try { await api.del(`${V1}/expenses/${d.id}${d.money_tx_ref && !d.money_found ? '?money=1' : ''}`); reload() } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}
