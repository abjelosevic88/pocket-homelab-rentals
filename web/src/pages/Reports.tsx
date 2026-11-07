// What each place earns and costs in a year, how full it was, and the yield.
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtMoney } from '../lib/format'
import { expenseLabel } from '../lib/rentals'
import type { PropertyYear } from '../lib/types'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, useAsync } from '../components/ui'

const MONTHS = ['J', 'F', 'M', 'A', 'M', 'J', 'J', 'A', 'S', 'O', 'N', 'D']

export default function Reports() {
  const [year, setYear] = useState(new Date().getFullYear())
  const { data } = useAsync(() => api.get<{ year: number; properties: PropertyYear[] }>(`${V1}/reports?year=${year}`), [year])
  const years = Array.from({ length: 6 }, (_, i) => new Date().getFullYear() - i)
  const list = data?.properties || []
  // totals per currency
  const byCur: Record<string, { income: number; expenses: number; months: [number, number][]; cats: Record<string, number> }> = {}
  for (const p of list) {
    const t = byCur[p.currency] ||= { income: 0, expenses: 0, months: Array.from({ length: 12 }, () => [0, 0] as [number, number]), cats: {} }
    t.income += p.income; t.expenses += p.expenses
    p.months.forEach((m, i) => { t.months[i][0] += m[0]; t.months[i][1] += m[1] })
    for (const [k, v] of Object.entries(p.by_category)) t.cats[k] = (t.cats[k] || 0) + v
  }
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Reports</h1><p className="muted">Rent in, costs out and what is left, place by place.</p></div>
        <Select ariaLabel="Year" value={String(year)} onChange={v => setYear(Number(v))} options={years.map(y => ({ value: String(y), label: String(y) }))} />
        <a className="btn" href={`${V1}/reports/year.csv?year=${year}`}><Icon name="download" size={15} />The year as CSV</a>
      </div>
      {!data ? <Loading /> : list.length === 0 ? <Empty icon="chart" title="Nothing to report yet" /> : <>
        {Object.entries(byCur).map(([cur, t]) => {
          const peak = Math.max(1, ...t.months.flat())
          const cats = Object.entries(t.cats).sort((a, b) => b[1] - a[1])
          const catMax = Math.max(1, ...cats.map(c => c[1]))
          return (
            <div key={cur} className="stack loose">
              <div className="stats-row">
                <div className="stat"><span className="stat-l">Rent in</span><span className="stat-v tnum ok">{fmtMoney(t.income, cur)}</span></div>
                <div className="stat"><span className="stat-l">Costs</span><span className="stat-v tnum">{fmtMoney(t.expenses, cur)}</span></div>
                <div className="stat"><span className="stat-l">Left over</span><span className={`stat-v tnum ${t.income - t.expenses < 0 ? 'bad' : ''}`}>{fmtMoney(t.income - t.expenses, cur)}</span></div>
              </div>
              <div className="two-grid">
                <Card title={`Month by month, ${cur}`}>
                  <div className="month-pairs" role="img" aria-label="Rent in and costs by month">
                    {t.months.map((m, i) => (
                      <div key={i} className="mp-col" title={`${new Date(year, i, 1).toLocaleDateString(undefined, { month: 'long' })}: in ${fmtMoney(m[0], cur)}, out ${fmtMoney(m[1], cur)}`}>
                        <div className="mp-bars"><span className="mp-in" style={{ height: `${(m[0] / peak) * 100}%` }} /><span className="mp-out" style={{ height: `${(m[1] / peak) * 100}%` }} /></div>
                        <span className="mb-l">{MONTHS[i]}</span>
                      </div>
                    ))}
                  </div>
                  <div className="legend small muted mt"><span><i className="lg-in" />Rent in</span><span><i className="lg-out" />Costs</span></div>
                </Card>
                <Card title="Where the money went">
                  {cats.length === 0 ? <p className="muted">No costs.</p> : <div className="bars">{cats.map(([k, v]) => (
                    <div key={k} className="bar-row"><span className="bar-l">{expenseLabel(k)}</span><span className="bar-track"><span className="bar-fill" style={{ width: `${(v / catMax) * 100}%` }} /></span><span className="bar-n tnum">{fmtMoney(v, cur)}</span></div>
                  ))}</div>}
                </Card>
              </div>
            </div>
          )
        })}
        <Card title="Place by place" flush>
          <div className="scroll-x"><table className="table">
            <thead><tr><th>Place</th><th className="right">Rent in</th><th className="right">Costs</th><th className="right">Left over</th><th className="right">Let</th><th className="right">Yield</th><th className="right">Owed now</th></tr></thead>
            <tbody>{list.map(p => (
              <tr key={p.id}>
                <td>{p.name}</td>
                <td className="right tnum">{fmtMoney(p.income, p.currency)}</td>
                <td className="right tnum">{fmtMoney(p.expenses, p.currency)}</td>
                <td className={`right tnum ${p.net < 0 ? 'bad' : ''}`}>{fmtMoney(p.net, p.currency)}</td>
                <td className="right tnum">{p.occupancy}%</td>
                <td className="right tnum" title={p.value ? 'What is left in a year, against what the place is worth' : 'Add what the place is worth to see it'}>{p.value ? `${p.yield.toFixed(1)}%` : '—'}</td>
                <td className={`right tnum ${p.owed > 0 ? 'bad' : 'muted'}`}>{p.owed > 0 ? fmtMoney(p.owed, p.currency) : '—'}</td>
              </tr>
            ))}</tbody>
          </table></div>
        </Card>
      </>}
    </div>
  )
}
