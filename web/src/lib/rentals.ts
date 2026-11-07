// Words, icons and colours for renting, shared by the pages.
import type { AppSlug, AppStateName, ChargeState, Condition, Lease, PropertyKind, Priority, RequestStatus } from './types'

export const APP_INFO: Record<AppSlug, { name: string; icon: string; what: string }> = {
  contacts: { name: 'Pocket Contacts', icon: 'users', what: 'tenants as contacts' },
  money: { name: 'Pocket Money', icon: 'wallet', what: 'rent and costs on your accounts' },
  sign: { name: 'Pocket Sign', icon: 'pen', what: 'leases to sign' },
  calendar: { name: 'Pocket Calendar', icon: 'calendar', what: 'inspections, visits and lease ends' },
  tasks: { name: 'Pocket Tasks', icon: 'check-square', what: 'repairs and renewals as tasks' },
  notes: { name: 'Pocket Notes', icon: 'note', what: 'reports as notes' },
  mail: { name: 'Pocket Mail', icon: 'mail', what: 'emails to tenants' },
  invoicing: { name: 'Pocket Invoicing', icon: 'receipt', what: 'rent invoices' },
  documents: { name: 'Pocket Documents', icon: 'file', what: 'papers from your archive' },
  home: { name: 'Pocket Home', icon: 'home', what: 'tradespeople and the repair log' },
  forms: { name: 'Pocket Forms', icon: 'list', what: 'repairs your tenants report' },
}

export const TARGET_APP: Record<string, AppSlug> = {
  contact: 'contacts', task: 'tasks', event: 'calendar', note: 'notes', email: 'mail', signing: 'sign', invoice: 'invoicing', document: 'documents', record: 'home', answer: 'forms',
}
export const TARGET_LABEL: Record<string, string> = {
  contact: 'Contact', task: 'Task', event: 'In the calendar', note: 'Note', email: 'Email', signing: 'To sign', invoice: 'Invoice', document: 'Document', record: 'Home log', answer: 'Form answer',
}

export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'Connected', partly: 'Partly connected', off: 'Switched off here', link_off: 'Switched off in Pocket Core', missing: 'Not on this box',
  old: 'Needs a newer version', no_core: 'No Pocket Core', not_set_up: 'Not set up', error: 'Not reachable',
}

export const KINDS: { value: PropertyKind; label: string; icon: string }[] = [
  { value: 'apartment', label: 'An apartment', icon: 'home' }, { value: 'house', label: 'A house', icon: 'home' }, { value: 'building', label: 'A building with flats', icon: 'building' },
  { value: 'room', label: 'A room', icon: 'bed' }, { value: 'commercial', label: 'Shop or office', icon: 'store' }, { value: 'garage', label: 'A garage or parking', icon: 'key' },
  { value: 'other', label: 'Something else', icon: 'layers' },
]
export const kindIcon = (k: string) => KINDS.find(x => x.value === k)?.icon || 'home'

export const FREQ: { value: Lease['frequency']; label: string; per: string }[] = [
  { value: 'monthly', label: 'Monthly', per: 'a month' }, { value: 'weekly', label: 'Weekly', per: 'a week' }, { value: 'quarterly', label: 'Every 3 months', per: 'a quarter' }, { value: 'yearly', label: 'Yearly', per: 'a year' },
]
export const perWord = (f: string) => FREQ.find(x => x.value === f)?.per || ''

export const WEEKDAYS = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday']
export function dueWords(l: Pick<Lease, 'frequency' | 'due_day'>) {
  if (l.frequency === 'weekly') return `every ${WEEKDAYS[(l.due_day - 1) % 7]}`
  const n = l.due_day
  const th = n % 10 === 1 && n !== 11 ? 'st' : n % 10 === 2 && n !== 12 ? 'nd' : n % 10 === 3 && n !== 13 ? 'rd' : 'th'
  return `on the ${n}${th}`
}

export const CHARGE_STATE: Record<ChargeState, { label: string; cls: string; icon: string }> = {
  paid: { label: 'Paid', cls: 'st-done', icon: 'check' }, part: { label: 'Part paid', cls: 'st-due', icon: 'clock' }, due: { label: 'Due', cls: 'st-due', icon: 'clock' },
  late: { label: 'Late', cls: 'st-overdue', icon: 'alert' }, upcoming: { label: 'Coming', cls: 'st-ok', icon: 'calendar' }, waived: { label: 'Waived', cls: 'st-paused', icon: 'x' },
}
export const CHARGE_KINDS: { value: string; label: string }[] = [
  { value: 'utilities', label: 'Bills (water, heating…)' }, { value: 'repair', label: 'A repair they pay for' }, { value: 'late_fee', label: 'Late fee' },
  { value: 'deposit', label: 'Deposit' }, { value: 'rent', label: 'Rent (by hand)' }, { value: 'other', label: 'Something else' },
]

export const PRIORITY: Record<Priority, { label: string; cls: string }> = { urgent: { label: 'Urgent', cls: 'st-overdue' }, normal: { label: 'Normal', cls: 'st-ok' }, low: { label: 'Low', cls: 'st-paused' } }
export const REQ_STATUS: Record<RequestStatus, { label: string; cls: string; icon: string }> = {
  new: { label: 'New', cls: 'st-new', icon: 'inbox' }, planned: { label: 'Planned', cls: 'st-ok', icon: 'calendar' }, in_progress: { label: 'Being fixed', cls: 'st-due', icon: 'wrench' },
  waiting: { label: 'Waiting', cls: 'st-paused', icon: 'hourglass' }, done: { label: 'Done', cls: 'st-done', icon: 'check' }, cancelled: { label: 'Cancelled', cls: 'st-paused', icon: 'x' },
}

export const EXPENSE_KINDS: { value: string; label: string }[] = [
  { value: 'repairs', label: 'Repairs' }, { value: 'insurance', label: 'Insurance' }, { value: 'tax', label: 'Property tax' }, { value: 'utilities', label: 'Bills' },
  { value: 'loan_interest', label: 'Loan interest' }, { value: 'building_fee', label: 'Building fee' }, { value: 'management', label: 'Management' }, { value: 'cleaning', label: 'Cleaning' },
  { value: 'furniture', label: 'Furniture' }, { value: 'supplies', label: 'Supplies' }, { value: 'legal', label: 'Legal and fees' }, { value: 'other', label: 'Something else' },
]
export const expenseLabel = (v: string) => EXPENSE_KINDS.find(x => x.value === v)?.label || v

export const CONDITIONS: { value: Condition; label: string; cls: string }[] = [
  { value: 'good', label: 'Good', cls: 'st-done' }, { value: 'fair', label: 'Fair', cls: 'st-ok' }, { value: 'poor', label: 'Poor', cls: 'st-due' },
  { value: 'damaged', label: 'Damaged', cls: 'st-overdue' }, { value: 'missing', label: 'Missing', cls: 'st-overdue' },
]
export const INSPECTION_KIND: Record<string, string> = { move_in: 'Moving in', move_out: 'Moving out', routine: 'Routine' }

export const ENTRY_ICON: Record<string, string> = { note: 'note', call: 'phone', email: 'mail', visit: 'users', payment: 'wallet', charge: 'receipt', lease: 'key', repair: 'wrench', inspection: 'check-square', made: 'link' }

export const CURRENCIES = ['EUR', 'BAM', 'USD', 'GBP', 'CHF', 'RSD', 'HRK', 'SEK', 'NOK', 'DKK', 'PLN', 'CZK', 'HUF', 'RON', 'BGN', 'TRY', 'CAD', 'AUD']
export const COLORS = ['#1d6fa5', '#0c8599', '#2f9e44', '#f08c00', '#e8590c', '#c2255c', '#7048e8', '#495057']

/** "Ana Marić and Luka Novak". */
export function names(ts: { name: string }[] | undefined) {
  const n = (ts || []).map(t => t.name)
  if (n.length < 2) return n[0] || ''
  return n.slice(0, -1).join(', ') + ' and ' + n[n.length - 1]
}

/** "Flat 2, Linden Court" (just the place when the unit is the whole of it). */
export function placeName(unit: string, property: string) {
  if (!unit || unit === property || unit === 'Whole place') return property
  return `${unit}, ${property}`
}

/** Cents from what a person types: "450", "450,50", "1.200,50". */
export function toCents(v: string): number {
  const s = v.replace(/[^\d.,-]/g, '')
  if (!s) return 0
  const last = Math.max(s.lastIndexOf('.'), s.lastIndexOf(','))
  let n: number
  if (last >= 0 && s.length - last - 1 === 3) n = Number(s.replace(/[.,]/g, ''))
  else if (last >= 0) n = Number(s.slice(0, last).replace(/[.,]/g, '') + '.' + s.slice(last + 1))
  else n = Number(s)
  return Number.isFinite(n) ? Math.round(n * 100) : 0
}
export const fromCents = (c: number) => (c ? (c / 100).toString() : '')
