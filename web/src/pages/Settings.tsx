import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, AppState, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'
import { ImportModal } from '../components/ImportModal'
import { APP_INFO, CURRENCIES, STATE_WORDS } from '../lib/rentals'
import { store as storeDefaults } from '../lib/defaults'

type TabId = 'general' | 'rent' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'home', sub: 'Dates, times, your currency and the app title.' },
  { id: 'rent', label: 'Rent & reminders', icon: 'wallet', sub: 'When rent is due and late for new leases, Pocket Money, the rent reminder, and what goes to your other apps by itself.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'A morning note with rent due and late, leases ending and urgent repairs.' },
  { id: 'apps', label: 'Connected apps', icon: 'link', sub: 'Your contacts, money, signing, calendar, tasks, notes, mail, invoices, documents, home and forms — what your rentals can reach.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who helps you: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box. Each person has their own rentals.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Siri Shortcuts, scripts and other tools.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Bring leases and payments from a spreadsheet, export everything, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'rent' && <RentTab />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on"><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <Section title="Money" icon="wallet">
        <SettingRow title="Your currency" help="New places and leases start in it. A place can be in another currency; totals never mix currencies."><Select ariaLabel="Currency" value={f.currency} onChange={v => setF({ ...f, currency: v })} options={Array.from(new Set([f.currency, ...CURRENCIES])).map(c => ({ value: c, label: c }))} /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function RentTab() {
  const { f, setF, save, busy } = useSettingsForm()
  const moneyOn = !!f && f.money_mode !== 'off' && f.money_via !== ''
  const { data: accounts } = useAsync(() => moneyOn ? api.get<{ id: number; name: string; currency: string }[]>(`${V1}/apps/options/accounts`).catch(() => []) : Promise.resolve([]), [moneyOn])
  const { data: incCats } = useAsync(() => moneyOn ? api.get<{ id: number; name: string }[]>(`${V1}/apps/options/income-categories`).catch(() => []) : Promise.resolve([]), [moneyOn])
  const { data: expCats } = useAsync(() => moneyOn ? api.get<{ id: number; name: string }[]>(`${V1}/apps/options/expense-categories`).catch(() => []) : Promise.resolve([]), [moneyOn])
  const formsOn = !!f && f.forms_mode !== 'off' && f.forms_via !== ''
  const { data: forms } = useAsync(() => formsOn ? api.get<{ id: string; name: string }[]>(`${V1}/apps/options/forms`).catch(() => []) : Promise.resolve([]), [formsOn])
  if (!f) return <Loading />
  const on = (slug: AppSlug) => f[`${slug}_mode`] !== 'off' && f[`${slug}_via`] !== ''
  const n = (k: keyof SettingsT, lo: number, hi: number) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: Math.min(hi, Math.max(lo, Number(e.target.value) || 0)) } as SettingsT)
  return (
    <div className="stack loose">
      <Section title="New leases start with" icon="key" sub="Each lease can have its own; these are what a new one is filled with.">
        <SettingRow title="Rent is due on" help="The day of the month."><Select ariaLabel="Due day" value={String(f.due_day)} onChange={v => setF({ ...f, due_day: Number(v) })} options={Array.from({ length: 28 }, (_, i) => ({ value: String(i + 1), label: `The ${i + 1}.` }))} /></SettingRow>
        <SettingRow title="Late after" help="Days after the due day before rent counts as late."><input type="number" className="num-sm" min={0} max={60} value={f.grace_days} onChange={n('grace_days', 0, 60)} /></SettingRow>
        <SettingRow title="Notice" help="Days either side gives to end a lease."><input type="number" className="num-sm" min={0} max={365} value={f.notice_days} onChange={n('notice_days', 0, 365)} /></SettingRow>
      </Section>
      <Section title="What counts as soon" icon="clock">
        <SettingRow title="Rent coming" help="Today and the morning note mention rent due this many days ahead."><input type="number" className="num-sm" min={0} max={30} value={f.remind_days} onChange={n('remind_days', 0, 30)} /></SettingRow>
        <SettingRow title="A lease ends soon" help="Days before its end."><input type="number" className="num-sm" min={7} max={365} value={f.ending_days} onChange={n('ending_days', 7, 365)} /></SettingRow>
      </Section>
      <Section title="Pocket Money" icon="wallet" sub={moneyOn ? 'Rent and costs can go on your accounts there, and rent already in your bank is found and offered.' : 'Connect Pocket Money (Connected apps) to book rent and costs on your accounts and to find rent already in your bank.'}>
        <SettingRow title="Book as you go" help="Every payment and cost you add here is booked there too (you can still say no each time)."><Switch checked={f.money_book} onChange={v => setF({ ...f, money_book: v })} label="Book in Pocket Money" disabled={!moneyOn} /></SettingRow>
        <SettingRow title="Account" help="Empty: your everyday account."><Select ariaLabel="Account" value={String(f.money_account || '')} onChange={v => setF({ ...f, money_account: Number(v) })} placeholder="Your everyday account" clearLabel="Your everyday account" disabled={!moneyOn} options={(accounts || []).map(a => ({ value: String(a.id), label: a.name, hint: a.currency }))} /></SettingRow>
        <SettingRow title="Rent goes to"><Select ariaLabel="Income category" value={String(f.money_income_cat || '')} onChange={v => setF({ ...f, money_income_cat: Number(v) })} placeholder="No category" clearLabel="No category" disabled={!moneyOn} options={(incCats || []).map(c => ({ value: String(c.id), label: c.name }))} /></SettingRow>
        <SettingRow title="Costs go to"><Select ariaLabel="Expense category" value={String(f.money_expense_cat || '')} onChange={v => setF({ ...f, money_expense_cat: Number(v) })} placeholder="No category" clearLabel="No category" disabled={!moneyOn} options={(expCats || []).map(c => ({ value: String(c.id), label: c.name }))} /></SettingRow>
        <SettingRow title="Find rent in my bank" help="Money that came in and looks like a lease’s rent is offered on Today, on Rent and on the lease: one tap and it is recorded, never booked twice."><Switch checked={f.money_match} onChange={v => setF({ ...f, money_match: v })} label="Find rent" disabled={!moneyOn} /></SettingRow>
        <SettingRow title="How close" help="Days from the due day, and how far off the amount may be (percent)."><span className="row tight"><input type="number" className="num-sm" min={1} max={31} value={f.money_match_days} onChange={n('money_match_days', 1, 31)} aria-label="Days" /><input type="number" className="num-sm" min={0} max={50} value={f.money_match_pct} onChange={n('money_match_pct', 0, 50)} aria-label="Percent" /></span></SettingRow>
      </Section>
      <Section title="By itself, in your other apps" icon="link" sub="Each only when its app is connected.">
        <SettingRow title="Repairs in Pocket Tasks" help={on('tasks') ? 'A new repair is a task too; ticking it off there finishes the repair here.' : 'Connect Pocket Tasks first.'}><Switch checked={f.repair_tasks} onChange={v => setF({ ...f, repair_tasks: v })} label="Repairs as tasks" disabled={!on('tasks')} /></SettingRow>
        <SettingRow title="Put days in Pocket Calendar" help={on('calendar') ? 'Inspections, repair visits and the day a lease ends, kept in step when they move.' : 'Connect Pocket Calendar first; until then each has a calendar file.'}><Switch checked={f.calendar_sync} onChange={v => setF({ ...f, calendar_sync: v })} label="Days in the calendar" disabled={!on('calendar')} /></SettingRow>
        <SettingRow title="Finished repairs in Pocket Home" help={on('home') ? 'Into the log of the place linked there (change a place to link it).' : 'Connect Pocket Home first.'}><Switch checked={f.home_log} onChange={v => setF({ ...f, home_log: v })} label="Repairs in Pocket Home" disabled={!on('home')} /></SettingRow>
        <SettingRow title="Your repair form" help={formsOn ? 'Answers to it show on Repairs, one click from being a repair. Empty: answers to all your forms.' : 'Connect Pocket Forms first.'}><Select ariaLabel="Repair form" value={f.repair_form} onChange={v => setF({ ...f, repair_form: v })} placeholder="All your forms" clearLabel="All your forms" disabled={!formsOn} options={(forms || []).map(x => ({ value: String(x.id), label: x.name }))} /></SettingRow>
        <SettingRow title="Send emails from" help={on('mail') ? 'One of your Pocket Mail addresses; empty: its default.' : 'Connect Pocket Mail first; without it, emails open in your own email app.'}><input value={f.mail_from} onChange={e => setF({ ...f, mail_from: e.target.value })} placeholder="you@example.com" disabled={!on('mail')} /></SettingRow>
      </Section>
      <Section title="The rent reminder" icon="mail" sub={<>What “Remind” on a lease starts with. <code>{'{name}'}</code>, <code>{'{amount}'}</code>, <code>{'{due}'}</code> and <code>{'{place}'}</code> are filled in.</>}>
        <textarea rows={8} value={f.reminder_text} onChange={e => setF({ ...f, reminder_text: e.target.value })} />
        <button type="button" className="link-btn self-start" onClick={() => setF({ ...f, reminder_text: storeDefaults.reminder })}>Back to the usual text</button>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const now = async () => { try { const r = await api.post<{ sent: boolean; text: string }>(`${V1}/notify/check?send=1`); toast(r.sent ? 'Sent' : `Nothing sent${f.notify_url ? '' : ' (no push URL)'}: ${r.text.split('\n')[0]}`) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack loose">
      <Section title="Push" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/rentals" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="What to tell you" icon="clock">
        <SettingRow title="Morning note" help="Rent due today and in the next days, rent that is late, leases ending, urgent repairs and today’s inspections. Nothing is sent on a day with nothing to say."><Switch checked={f.notify_digest} onChange={v => setF({ ...f, notify_digest: v })} label="Morning note" /></SettingRow>
        <SettingRow title="Morning message at" help="Server time."><Select ariaLabel="Hour" value={String(f.notify_hour)} onChange={v => setF({ ...f, notify_hour: Number(v) })} options={HOURS} /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        <button type="button" className="btn" onClick={now}><Icon name="refresh" size={15} />Check now</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'contacts', title: 'Pocket Contacts — your tenants', sub: 'Pick tenants from your contacts or add them there, and open their card from the lease. Without it, a tenant’s name, email and phone live here.', token: 'Pocket Contacts → Settings → API tokens', port: '8191' },
  { slug: 'money', title: 'Pocket Money — rent and costs on your accounts', sub: 'Book rent that came in and the costs of a place, and find rent already in your bank transactions. Without it, the money is counted here only.', token: 'Pocket Money → Settings → API tokens', port: '8178' },
  { slug: 'sign', title: 'Pocket Sign — leases to sign', sub: 'Send a lease from your templates to the tenants; the lease shows when everyone signed (and a draft starts).', token: 'Pocket Sign → Settings → API tokens', port: '8234' },
  { slug: 'calendar', title: 'Pocket Calendar — the days', sub: 'Inspections, repair visits and the day a lease ends, kept in step. Without it, each one downloads as a file any calendar opens.', token: 'Pocket Calendar → Settings → API tokens', port: '8190' },
  { slug: 'tasks', title: 'Pocket Tasks — repairs and renewals', sub: 'A repair or a lease to renew as a task; ticking it off there finishes the repair here.', token: 'Pocket Tasks → Settings → API tokens', port: '8184' },
  { slug: 'notes', title: 'Pocket Notes — reports', sub: 'An inspection report, a lease summary or a repair as a note.', token: 'Pocket Notes → Settings → API tokens', port: '8182' },
  { slug: 'mail', title: 'Pocket Mail — emails to tenants', sub: 'Reminders and notes from your own mailbox. Without it, emails open in your own email app.', token: 'Pocket Mail → Settings → API tokens', port: '8208' },
  { slug: 'invoicing', title: 'Pocket Invoicing — rent invoices', sub: 'A draft invoice for a rent or a charge, for tenants who need one, and when it is paid.', token: 'Pocket Invoicing → Settings → API tokens', port: '8168' },
  { slug: 'documents', title: 'Pocket Documents — papers', sub: 'Attach the signed lease, IDs and insurance from your archive to a lease or a place.', token: 'Pocket Documents → Settings → API tokens', port: '8172' },
  { slug: 'home', title: 'Pocket Home — tradespeople and the repair log', sub: 'Pick who fixes a repair from your tradespeople, and write finished repairs into the place’s log there.', token: 'Pocket Home → Settings → API tokens', port: '8229' },
  { slug: 'forms', title: 'Pocket Forms — repairs your tenants report', sub: 'Answers to your repair form become repairs in one click, at the right lease (found by the tenant’s email).', token: 'Pocket Forms → Settings → API tokens', port: '8248' },
]

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: states } = useAsync(() => api.get<{ apps: AppState[] }>(`${V1}/apps`).then(r => r.apps).catch(() => [] as AppState[]), [f?.contacts_mode, f?.money_mode, f?.sign_mode, f?.calendar_mode, f?.tasks_mode, f?.notes_mode, f?.mail_mode, f?.invoicing_mode, f?.documents_mode, f?.home_mode, f?.forms_mode])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Rentals only adds things in them when you ask (or when you switched that on under Rent &amp; reminders), and everything here works without them. Through <b>Pocket Core</b> Rentals acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        const name = APP_INFO[a.slug].name
        const st = states?.find(x => x.slug === a.slug)
        return (
          <Section key={a.slug} title={a.title} icon={APP_INFO[a.slug].icon} actions={st && st.state !== 'on' && st.state !== 'off' ? <span className={`badge ${st.state === 'partly' ? 'ok' : 'warn'}`}>{STATE_WORDS[st.state]}</span> : connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
            {st?.fix && st.state !== 'on' && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {st.fix}.</p>}
          </Section>
        )
      })}
      <Section title="Other Pocket apps" icon="layers" sub="Rentals offers your rentals to the rest of the box through Pocket Core: Pocket Contacts shows on a tenant’s card where they rent and what they owe, Pocket Day writes rent in and repairs into your journal, Pocket Search finds places, leases and tenants, Pocket Renewals reminds you of lease ends, Pocket AI tells you what is due and reports repairs when you ask, and Pocket Flows can start a flow when rent is late. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>rentals:read</dt><dd className="muted">Your places, units, tenants and leases, rent due, paid and late, repairs, inspections and the year’s totals.</dd></div>
          <div><dt>rentals:write</dt><dd className="muted">Report a repair for you and add a line to a lease’s timeline; that app may change only the repairs it added.</dd></div>
          <div><dt>events:read</dt><dd className="muted"><code>rentals.rent_due</code>, <code>rentals.rent_late</code>, <code>rentals.payment_recorded</code>, <code>rentals.lease_started</code>, <code>rentals.lease_ending</code>, <code>rentals.lease_ended</code>, <code>rentals.request_created</code>, <code>rentals.request_done</code> — for Pocket Flows.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="Only when you press its button: a rent reminder, a note about late rent or a lease ending, what to do first about a repair, and a short sum-up of an inspection (for moving out: what is fair to keep from the deposit). Only that lease, repair or inspection is sent, and nothing is saved or sent until you say so. Questions about all your rentals go to Pocket AI through the Ask button at the top.">
        <SettingRow title="Pocket AI buttons" help="Off: the buttons are gone and nothing is sent to an AI."><Switch checked={f.ai_help} onChange={v => save({ ai_help: v })} label="AI buttons" /></SettingRow>
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Rentals does not reach ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own places, leases, settings and connections. Admins manage accounts; they do not see anyone else’s rentals.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their places, leases, payments, repairs and photos." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or send a recording from a Siri Shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone Shortcut)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>. Money is in cents.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /today</code></dt><dd className="muted">Rent late and due, leases ending, repairs, inspections and empty units.</dd></div>
          <div><dt><code>GET /summary</code></dt><dd className="muted">Units let, rent owed, open repairs — for Homepage.</dd></div>
          <div><dt><code>POST /leases/&lt;id&gt;/payments</code></dt><dd className="muted"><code>{'{"amount": 65000, "date": "2026-11-03"}'}</code> records rent that came in (from a Shortcut).</dd></div>
          <div><dt><code>POST /requests</code></dt><dd className="muted"><code>{'{"property_id": 1, "title": "Tap drips"}'}</code> reports a repair.</dd></div>
          <div><dt><code>GET /rent?month=YYYY-MM</code></dt><dd className="muted">Every charge due in a month and how far it is paid.</dd></div>
          <div><dt><code>GET /reports/year.csv?year=YYYY</code></dt><dd className="muted">The year’s money in and out, line by line, for taxes.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const [importing, setImporting] = useState(false)
  return (
    <div className="stack loose">
      <Section title="Bring your rentals" icon="upload" sub="A spreadsheet of leases (place, unit, tenant, rent, start, end, deposit) or of rent payments (day, amount, who), saved as CSV — or a Pocket Rentals export. Bringing the same file twice adds nothing twice.">
        <button type="button" className="btn primary" onClick={() => setImporting(true)}><Icon name="upload" size={15} />Bring a file…</button>
      </Section>
      <Section title="Export" icon="archive" sub="Everything of yours as one JSON file (it comes back in with “Bring a file”, also on another box) — or every lease as one table for a spreadsheet. For taxes, Reports has each year’s money in and out as CSV.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Leases as CSV</a></div>
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only. Photos live next to it in the files folder.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-rentals.db</code> in the data folder.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
      {importing && <ImportModal onClose={() => setImporting(false)} />}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Places', counts.properties, 'building'], ['Leases', counts.leases, 'key'], ['Payments', counts.payments, 'wallet'], ['Repairs', counts.requests, 'wrench']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Rentals</h3>
          <p className="muted small">The homes you rent out: units, tenants and leases, rent due and paid, deposits, repairs and inspections — linked to your contacts, money, signing, calendar, tasks, notes, mail, invoices, documents, home and forms. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-rentals" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Contacts, Money, Sign, Calendar, Tasks, Notes, Mail, Invoicing, Documents, Home, Forms, Flows, Day, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}

