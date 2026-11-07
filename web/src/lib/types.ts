export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'contacts' | 'money' | 'sign' | 'calendar' | 'tasks' | 'notes' | 'mail' | 'invoicing' | 'documents' | 'home' | 'forms'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  currency: string; due_day: number; grace_days: number; notice_days: number; remind_days: number; ending_days: number
  money_book: boolean; money_match: boolean; money_account: number; money_income_cat: number; money_expense_cat: number; money_match_days: number; money_match_pct: number
  repair_tasks: boolean; calendar_sync: boolean; home_log: boolean; mail_from: string; repair_form: string; reminder_text: string; invoice_rent: boolean; ai_help: boolean
  notify_digest: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type Money = Record<string, number>
export type PropertyKind = 'house' | 'apartment' | 'building' | 'room' | 'commercial' | 'garage' | 'other'

export interface Unit {
  id: number; property_id: number; name: string; bedrooms: number; bathrooms: number; area_m2: number; rent: number; deposit: number; notes: string
  position: number; archived: boolean; created_at: string; property_name?: string; lease?: Lease
}

export interface Property {
  id: number; name: string; kind: PropertyKind; address: string; city: string; country: string; currency: string; value: number; bought_on: string; bought_for: number
  home_ref: string; money_tag: string; color: string; notes: string; archived: boolean; position: number; created_at: string; updated_at: string
  units?: Unit[]; unit_count: number; let: number; rent_roll: number; open_repair: number; owed: number
}

export interface Tenant {
  id: number; name: string; email: string; phone: string; contact_ref: string; notes: string; created_at: string; updated_at: string
  leases: number; current?: string; lease_id?: number; balance: number
}

export type LeaseStatus = 'draft' | 'active' | 'ended'
export type Frequency = 'monthly' | 'weekly' | 'quarterly' | 'yearly'

export interface Lease {
  id: number; unit_id: number; status: LeaseStatus; start_date: string; end_date: string; charges_from: string; rent: number; currency: string; frequency: Frequency
  due_day: number; grace_days: number; late_fee: number; notice_days: number; deposit: number; deposit_state: 'none' | 'due' | 'held' | 'returned' | 'part_returned'
  deposit_back: number; deposit_note: string; ended_on: string; end_reason: string; signing_ref: string; signing_state: string; notes: string; source_app: string
  created_at: string; updated_at: string
  unit_name: string; property_id: number; property_name: string; tenants: Tenant[]; balance: number; overdue: number; next_due: string; days_left?: number; title: string
}

export type ChargeState = 'paid' | 'part' | 'due' | 'late' | 'upcoming' | 'waived'
export interface Charge {
  id: number; lease_id: number; kind: 'rent' | 'deposit' | 'utilities' | 'late_fee' | 'repair' | 'other'; title: string; period: string; due_date: string
  amount: number; currency: string; waived: boolean; invoice_ref: string; note: string; created_at: string; paid: number; state: ChargeState; paid_on?: string
}

export interface Payment { id: number; lease_id: number; date: string; amount: number; currency: string; method: 'bank' | 'cash' | 'card' | 'other'; note: string; money_tx_ref: string; money_found: boolean; source_app: string; created_at: string; money_url?: string }

export type Priority = 'low' | 'normal' | 'urgent'
export type RequestStatus = 'new' | 'planned' | 'in_progress' | 'waiting' | 'done' | 'cancelled'
export interface Request {
  id: number; property_id: number; unit_id: number | null; lease_id: number | null; title: string; details: string; priority: Priority; status: RequestStatus
  reported_by: string; reported_on: string; planned_on: string; planned_time: string; done_on: string; cost: number; currency: string
  pro_name: string; pro_phone: string; pro_ref: string; tenant_pays: boolean; task_ref: string; event_ref: string; home_record_ref: string; source_app: string; source_ref: string
  created_at: string; updated_at: string; property_name: string; unit_name: string; photos: number
}

export interface Expense {
  id: number; property_id: number; unit_id: number | null; request_id: number | null; date: string; amount: number; currency: string; category: string; payee: string; note: string
  money_tx_ref: string; money_found: boolean; created_at: string; property_name: string; money_url?: string
}

export type Condition = '' | 'good' | 'fair' | 'poor' | 'damaged' | 'missing'
export interface FileRec { id: number; owner: string; owner_id: number; name: string; mime: string; size: number; created_at: string }
export interface InspectionItem { id: number; inspection_id?: number; room: string; item: string; condition: Condition; note: string; position?: number; photos: FileRec[] }
export interface Inspection {
  id: number; property_id: number; unit_id: number | null; lease_id: number | null; kind: 'move_in' | 'move_out' | 'routine'; date: string; time: string; status: 'planned' | 'done'
  summary: string; event_ref: string; created_at: string; updated_at: string; property_name: string; unit_name: string; items?: InspectionItem[]; item_count: number; issues: number
}

export interface Entry { id: number; lease_id: number | null; property_id: number | null; request_id: number | null; kind: string; title: string; body: string; at: string; app: string; url: string; source_app: string; created_at: string }

export interface Made { id: number; owner: string; owner_id: number; target: string; app: string; ref: string; title: string; url: string; state: string; error?: string; checked_at: string; created_at: string }

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'
export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

export interface Summary {
  properties: number; units: number; let: number; vacant: number; late: number; due: number; repairs: number; urgent: number; ending: number; drafts: number
  rent_roll: Money; owed: Money; currency: string
}

export interface DueItem { charge_id: number; lease_id: number; lease: string; place: string; tenants: Tenant[]; title: string; kind: string; due_date: string; amount: number; paid: number; currency: string; state: ChargeState; paid_on?: string; days: number }

export interface TodayView {
  date: string; late: DueItem[]; due: DueItem[]; ending: Lease[]; repairs: Request[]; inspections: Inspection[]; vacant: Unit[]; drafts: Lease[]
  month: { month: string; expected: Money; received: Money }; empty: boolean
}

export interface Suggestion { tx_id: string; date: string; amount: number; currency: string; payee: string; description: string; account: string; url: string; lease_id: number; lease_title: string; owed: number; why: string; score: number }

export interface PropertyYear { id: number; name: string; currency: string; income: number; expenses: number; net: number; by_category: Record<string, number>; months: [number, number][]; occupancy: number; owed: number; value: number; yield: number }

export interface PickedContact { id: string; name: string; company: string; email: string; phone: string; initials: string; color: string }
