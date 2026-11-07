import { AskButton } from '../components/AskButton'
import React, { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, upload, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, AppState, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'
import { APP_INFO, CURRENCIES, STATE_WORDS } from '../lib/league'

type TabId = 'general' | 'notifications' | 'apps' | 'email' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string; players: boolean }[] = [
  { id: 'general', label: 'General', icon: 'home', sub: 'Dates, times, your currency and the app title.', players: true },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'New games, reminders before you play, results to check, the chat — and where they reach you.', players: true },
  { id: 'apps', label: 'Connected apps', icon: 'link', sub: 'Your calendar, health, habits, money, Split groups, weather, places, contacts, mail and tasks — what League does in them for you.', players: false },
  { id: 'email', label: 'Email', icon: 'mail', sub: 'How sign-in links, plans and reminders reach your players: from your mailbox, your mail server, or not at all.', players: false },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who helps you: Pocket AI through Core, or your own OpenAI key.', players: false },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email, phone and password.', players: true },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone with an account on this box, including the players organisers added.', players: false },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Siri Shortcuts, scripts and other tools.', players: false },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Export your groups, results and chat, bring them back, download the database.', players: true },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.', players: true },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const { user } = useApp()
  const tabs = TABS.filter(t => user?.role !== 'player' || t.players)
  const tab: TabId = tabs.find(t => t.id === raw)?.id || 'general'
  const current = tabs.find(t => t.id === tab)!
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
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={tabs.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {tabs.map(t => (
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
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'email' && <EmailTab />}
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
        <SettingRow title="Your currency" help="New groups start in it; a group can be in another currency."><Select ariaLabel="Currency" value={f.currency} onChange={v => setF({ ...f, currency: v })} options={Array.from(new Set([f.currency, ...CURRENCIES])).map(c => ({ value: c, label: c === 'BAM' ? 'KM (BAM)' : c }))} /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { user } = useApp()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const player = user?.role === 'player'
  return (
    <div className="stack loose">
      <Section title="What to tell you" icon="bell" sub="Each group can also be set to everything, only games, or nothing (the group's ⋯ menu).">
        <SettingRow title="New games and changes" help="A game planned, moved or cancelled; a vote on a time; you moved up from the waiting list."><Switch checked={f.notify_plans} onChange={v => setF({ ...f, notify_plans: v })} label="New games" /></SettingRow>
        <SettingRow title="Reminders" help="The day before the games you are in (with the weather for outdoor ones)."><Switch checked={f.notify_reminders} onChange={v => setF({ ...f, notify_reminders: v })} label="Reminders" /></SettingRow>
        <SettingRow title="And shortly before" help="Another reminder this many hours before."><Select ariaLabel="Hours before" value={String(f.remind_hours)} onChange={v => setF({ ...f, remind_hours: Number(v) })} options={[{ value: '0', label: 'No' }, ...[1, 2, 3, 4, 6].map(h => ({ value: String(h), label: `${h} ${h === 1 ? 'hour' : 'hours'} before` }))]} /></SettingRow>
        <SettingRow title="Results to check" help="Someone entered a result you played in: is it right?"><Switch checked={f.notify_results} onChange={v => setF({ ...f, notify_results: v })} label="Results" /></SettingRow>
        <SettingRow title="The chat" help="New messages while League is closed. Never by email."><Switch checked={f.notify_chat} onChange={v => setF({ ...f, notify_chat: v })} label="Chat" /></SettingRow>
        {!player && <SettingRow title="Short of players" help="For the groups you organise: the day before, when fewer are in than needed."><Switch checked={f.notify_short} onChange={v => setF({ ...f, notify_short: v })} label="Short of players" /></SettingRow>}
      </Section>
      <Section title="By email" icon="mail" sub="Without a push address below, these come by email (not the chat) — when the group sends emails.">
        <SettingRow title="Emails" help={user?.email ? `To ${user.email}.` : 'Your account has no email address (Profile).'}><Switch checked={f.email_me} onChange={v => setF({ ...f, email_me: v })} label="Emails" disabled={!user?.email} /></SettingRow>
      </Section>
      <Section title="Push" icon="smartphone" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go instead of email: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="https://ntfy.sh/my-league-topic" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="bell" size={15} />Send a test</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'calendar', title: 'Pocket Calendar — your games', sub: 'The games you are in go to your calendar, and move or go when the plan changes. When voting on a time, see what you already have on. Without it, download a game or subscribe to a group in any calendar.', token: 'Pocket Calendar → Settings → API tokens', port: '8190' },
  { slug: 'health', title: 'Pocket Health — matches as workouts', sub: 'Each match you play is recorded as a workout (tennis, football…), so it counts in your health history and your Fitness week.', token: 'Pocket Health → Settings → API tokens', port: '8200' },
  { slug: 'habits', title: 'Pocket Habits — tick your habit', sub: 'Tick the habit you pick (“Sport”, “Move”) on the days you play.', token: 'Pocket Habits → Settings → API tokens', port: '8177' },
  { slug: 'split', title: 'Pocket Split — share the court', sub: 'Split what a game cost between the players in one of your Split groups.', token: 'Pocket Split → Settings → API tokens', port: '8185' },
  { slug: 'money', title: 'Pocket Money — your share', sub: 'Book your share of a game as an expense.', token: 'Pocket Money → Settings → API tokens', port: '8178' },
  { slug: 'weather', title: 'Pocket Weather — the forecast', sub: 'The forecast at the court for outdoor games, and a warning in the reminder when rain or wind is coming.', token: 'Pocket Weather → Settings → API tokens', port: '8239' },
  { slug: 'places', title: 'Pocket Places — the venue', sub: 'Pick where a group plays from your places, and save a new court there.', token: 'Pocket Places → Settings → API tokens', port: '8236' },
  { slug: 'contacts', title: 'Pocket Contacts — the players', sub: 'Pick the people you add to a group from your contacts, with their email and phone.', token: 'Pocket Contacts → Settings → API tokens', port: '8191' },
  { slug: 'mail', title: 'Pocket Mail — emails to players', sub: 'Sign-in links, plans and reminders leave from your own mailbox (see Email).', token: 'Pocket Mail → Settings → API tokens', port: '8208' },
  { slug: 'tasks', title: 'Pocket Tasks — book the court', sub: 'A “book the court” task a couple of days before a game you plan.', token: 'Pocket Tasks → Settings → API tokens', port: '8184' },
]

type Opt = { id: number; name: string; note?: string }
function useOptions(what: string, on: boolean, extra = '') {
  const [list, setList] = useState<Opt[] | null>(null)
  useEffect(() => {
    if (!on) { setList(null); return }
    api.get<Opt[]>(`${V1}/apps/options/${what}${extra}`).then(setList).catch(() => setList([]))
  }, [what, on, extra])
  return list
}

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: states } = useAsync(() => api.get<{ apps: AppState[] }>(`${V1}/apps`).then(r => r.apps).catch(() => [] as AppState[]), [f?.calendar_mode, f?.health_mode, f?.habits_mode, f?.split_mode, f?.money_mode, f?.weather_mode, f?.places_mode, f?.contacts_mode, f?.mail_mode, f?.tasks_mode])
  const ok = (slug: AppSlug) => !!f && f[`${slug}_mode`] !== 'off' && f[`${slug}_via`] !== ''
  const calendars = useOptions('calendars', ok('calendar'))
  const habits = useOptions('habits', ok('habits'))
  const splits = useOptions('split_groups', ok('split'))
  const accounts = useOptions('accounts', ok('money'))
  const cats = useOptions('categories', ok('money') && !!f?.money_account, f?.money_account ? `?account=${f.money_account}` : '')
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  const extra = (slug: AppSlug): React.ReactNode => {
    const on = ok(slug)
    const off = on ? undefined : 'Connect it first.'
    switch (slug) {
      case 'calendar': return <>
        <SettingRow title="Games I'm in go to my calendar" help={off || 'Removed again when you say you are out, or the game is cancelled.'}><Switch checked={f.calendar_auto} onChange={v => setF({ ...f, calendar_auto: v })} label="Games in my calendar" disabled={!on} /></SettingRow>
        <SettingRow title="Into" help={off}><Select ariaLabel="Calendar" value={f.calendar_id ? String(f.calendar_id) : ''} placeholder="Your default calendar" clearLabel="Your default calendar" disabled={!on} onChange={v => setF({ ...f, calendar_id: Number(v) || 0 })} options={(calendars || []).map(c => ({ value: String(c.id), label: c.name, hint: c.note }))} /></SettingRow>
      </>
      case 'health': return <SettingRow title="Record my matches" help={off || 'Each match you play, as a workout of its sport.'}><Switch checked={f.health_auto} onChange={v => setF({ ...f, health_auto: v })} label="Record my matches" disabled={!on} /></SettingRow>
      case 'habits': return <SettingRow title="Tick this habit" help={off || 'On the days you play.'}><Select ariaLabel="Habit" value={f.habit_id ? String(f.habit_id) : ''} placeholder="No habit" clearLabel="No habit" disabled={!on} onChange={v => setF({ ...f, habit_id: Number(v) || 0 })} options={(habits || []).map(h => ({ value: String(h.id), label: h.name }))} /></SettingRow>
      case 'split': return <SettingRow title="Costs go to" help={off || 'Offered on a game with a cost: “Split the cost”.'}><Select ariaLabel="Split group" value={f.split_group ? String(f.split_group) : ''} placeholder="Pick a Split group" clearLabel="None" disabled={!on} onChange={v => setF({ ...f, split_group: Number(v) || 0 })} options={(splits || []).map(s => ({ value: String(s.id), label: s.name, hint: s.note }))} /></SettingRow>
      case 'money': return <>
        <SettingRow title="Book my share on" help={off}><Select ariaLabel="Account" value={f.money_account ? String(f.money_account) : ''} placeholder="Pick an account" clearLabel="None" disabled={!on} onChange={v => setF({ ...f, money_account: Number(v) || 0, money_category: 0 })} options={(accounts || []).map(a => ({ value: String(a.id), label: a.name, hint: a.note }))} /></SettingRow>
        <SettingRow title="Category" help={off}><Select ariaLabel="Category" value={f.money_category ? String(f.money_category) : ''} placeholder="No category" clearLabel="No category" disabled={!on || !f.money_account} onChange={v => setF({ ...f, money_category: Number(v) || 0 })} options={(cats || []).map(c => ({ value: String(c.id), label: c.name }))} /></SettingRow>
      </>
      case 'tasks': return <>
        <SettingRow title="Remind me to book the court" help={off || 'Ticked when you plan a game with a cost.'}><Switch checked={f.booking_task} onChange={v => setF({ ...f, booking_task: v })} label="Book the court" disabled={!on} /></SettingRow>
        <SettingRow title="Due" help={off}><Select ariaLabel="Days before" value={String(f.booking_days)} disabled={!on} onChange={v => setF({ ...f, booking_days: Number(v) })} options={[0, 1, 2, 3, 5, 7].map(d => ({ value: String(d), label: d === 0 ? 'On the day' : `${d} ${d === 1 ? 'day' : 'days'} before` }))} /></SettingRow>
      </>
    }
    return null
  }
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours; every group and game works without them. Through <b>Pocket Core</b> League acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
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
            {extra(a.slug)}
          </Section>
        )
      })}
      <Section title="Other Pocket apps" icon="layers" sub="League offers your games to the rest of the box through Pocket Core: Pocket Day shows today's game and writes your results into your journal, Pocket Search finds groups, players and games, Pocket Weather warns you about rain at game time, Pocket AI tells you about your next game and can say you're in, and Pocket Flows can start a flow when a game is planned or short. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>league:read</dt><dd className="muted">Your groups, games, answers, results and tables.</dd></div>
          <div><dt>league:write</dt><dd className="muted">Say for you whether you are in, maybe or out for a game. Nothing else.</dd></div>
          <div><dt>events:read</dt><dd className="muted"><code>league.game_planned</code>, <code>league.game_changed</code>, <code>league.game_cancelled</code>, <code>league.vote_started</code>, <code>league.answered</code>, <code>league.short</code>, <code>league.result</code>, <code>league.reminder</code> — for Pocket Flows.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function EmailTab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const [test, setTest] = useState('')
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const mailOn = f.mail_mode !== 'off' && f.mail_via !== ''
  const via = f.email_via
  const sendTest = async () => {
    setTest('sending…')
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/email/test`); setTest(r.ok ? `✓ ${r.message}` : `✕ ${r.error}`) } catch (e) { setTest(`✕ ${(e as Error).message}`); toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <Section title="How emails go out" icon="mail" sub="Players who are not on this box get their sign-in link by email (or WhatsApp — always there on their page), and, when a group sends emails, plans and reminders. They go out in your name.">
        <Segmented value={via} onChange={v => setF({ ...f, email_via: v, email_via_set: v })} options={[
          { value: 'mail', label: 'From my Pocket Mail', disabled: !coreOn && !mailOn, title: coreOn || mailOn ? undefined : 'Needs Pocket Core or a direct Pocket Mail connection' },
          { value: 'smtp', label: 'My mail server' },
          { value: 'none', label: 'No emails' },
        ]} />
        {!coreOn && !mailOn && <CoreHint />}
        {via === 'mail' && <>
          <p className="muted small">{mailOn ? 'Through Pocket Mail: from your own mailbox, landing in its Sent folder.' : 'Connect Pocket Mail first (Connected apps).'}</p>
          <SettingRow title="Send from" help="One of your Pocket Mail accounts; empty: its default."><input value={f.mail_from} onChange={e => setF({ ...f, mail_from: e.target.value })} placeholder="you@example.com" disabled={!mailOn} /></SettingRow>
        </>}
        {via === 'smtp' && <div className="grid">
          <div className="two-col tight">
            <Field label="Server"><input value={f.smtp_host} onChange={e => setF({ ...f, smtp_host: e.target.value })} placeholder="smtp.gmail.com" /></Field>
            <Field label="Port"><input type="number" value={f.smtp_port} onChange={e => setF({ ...f, smtp_port: Number(e.target.value) || 587 })} /></Field>
          </div>
          <div className="two-col tight">
            <Field label="User"><input value={f.smtp_user} onChange={e => setF({ ...f, smtp_user: e.target.value })} autoComplete="off" /></Field>
            <Field label="Password" help="An app password for Gmail and iCloud."><input type="password" value={f.smtp_password} onChange={e => setF({ ...f, smtp_password: e.target.value })} autoComplete="new-password" /></Field>
          </div>
          <div className="two-col tight">
            <Field label="From address" help="Empty: the user."><input value={f.smtp_from} onChange={e => setF({ ...f, smtp_from: e.target.value })} /></Field>
            <Field label="From name" help="Empty: your name."><input value={f.smtp_from_name} onChange={e => setF({ ...f, smtp_from_name: e.target.value })} /></Field>
          </div>
          <Field label="Connection" group><Segmented size="sm" value={f.smtp_tls} onChange={v => setF({ ...f, smtp_tls: v })} options={[{ value: 'starttls', label: 'STARTTLS (587)' }, { value: 'tls', label: 'TLS (465)' }, { value: 'none', label: 'Plain' }]} /></Field>
        </div>}
        {via === 'none' && <p className="muted small">Nothing is emailed. Send each player their link by WhatsApp, SMS or by copying it (People → Get them in).</p>}
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={sendTest} disabled={via === 'none'}><Icon name="mail" size={15} />Send me a test</button>
        <span className={`small ${test.startsWith('✓') ? 'ok' : test.startsWith('✕') ? 'bad' : 'muted'}`}>{test}</span>
      </div>
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
      <Section title="What the AI does" icon="sparkles" sub="Two things, each only when you press its button and only for you: a short match report to post (you change it, you post it — the AI never writes in a chat), and a catch-up of a group's chat. Questions about all your games go to Pocket AI through the Ask button at the top.">
        <SettingRow title="AI buttons" help="Off: the buttons are gone and nothing is sent to an AI."><Switch checked={f.ai_help} onChange={v => save({ ai_help: v })} label="AI buttons" /></SettingRow>
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
  const line = mode === 'off' ? `Off: League does not reach ${app}.`
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
  const people = users.filter(u => u.role !== 'player'), players = users.filter(u => u.role === 'player')
  const row = (u: User) => (
    <div key={u.id} className="set-person">
      <Avatar name={u.name} email={u.email} size={34} />
      <div className="grow">
        <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
        <div className="muted small ellipsis">{[u.email, u.phone].filter(Boolean).join(' · ') || '—'}{u.role === 'player' ? ` · ${u.last_seen_at ? `last here ${fmtDateTime(u.last_seen_at, false)}` : 'not in yet'}` : ''}</div>
      </div>
      {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
    </div>
  )
  return (
    <div className="stack loose">
      <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
        sub="Members sign in with their own account (or Pocket Core), start groups and connect their own apps. Admins manage accounts; they do not see groups they are not in.">
        <div className="set-people">{people.map(row)}</div>
        {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      </Section>
      {isAdmin && players.length > 0 && <Section title="Players" icon="ball" sub="Accounts organisers made for their groups. Players sign in with a link and see only their groups. Make someone a member to let them start groups; someone who signs in with Pocket Core becomes a member by themselves.">
        <div className="set-people">{players.map(row)}</div>
      </Section>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account goes, with their answers and settings. Their results stay in the tables; groups they made pass to another organiser." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'Removed') }} />}
    </div>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'Saved' : 'Added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={80} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[...(user.role === 'player' ? [{ value: 'player' as const, label: 'Player' }] : []), { value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', phone: user?.phone || '', current_password: '', new_password: '' })
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
          <Field label="Phone" help="Organisers can send you WhatsApp messages."><input type="tel" value={f.phone} onChange={e => setF({ ...f, phone: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub={user?.has_password ? 'Leave empty to keep the current one.' : 'You came in with a link. Set a password to sign in with your email too.'}><div className="stack">
        {user?.has_password && <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>}
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
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: a Siri Shortcut that says you're in, a Homepage widget (<code>GET /api/v1/summary</code>). See <code>docs/api.md</code>.</>}>
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
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /upcoming</code></dt><dd className="muted">Your coming games with your answer — <code>POST /games/&lt;id&gt;/answer</code> with <code>{'{"answer":"in"}'}</code> says you're in.</dd></div>
          <div><dt><code>GET /summary</code></dt><dd className="muted">Your next game, unread chat and this month — for Homepage.</dd></div>
          <div><dt><code>GET /groups/&lt;id&gt;/table</code></dt><dd className="muted">A group's table with ratings and form; <code>?period=year|season|month</code>.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">The games and results of a day — what Pocket Day reads.</dd></div>
          <div><dt><code>GET /feed/&lt;token&gt;.ics</code></dt><dd className="muted">A group's games for any calendar (the group's ⋯ menu → Subscribe).</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const { toast } = useToast()
  const file = useRef<HTMLInputElement>(null)
  const bring = async (f: File) => {
    try { const r = await upload<{ imported: Record<string, number> }>(`${V1}/import`, 'file', f); const i = r.imported; toast(`${i.groups} groups, ${i.people} people, ${i.games} games, ${i.matches} results, ${i.messages} messages brought in`) } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <Section title="Export" icon="archive" sub="Your groups as you see them: the people, the games with who was in, every result and the chat, as one JSON file (it comes back in below, also on another box) — or every result as one table for a spreadsheet.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Results as CSV</a></div>
      </Section>
      {user?.role !== 'player' && <Section title="Bring groups in" icon="upload" sub="A Pocket League export. Bringing the same file twice adds nothing twice. To add people from Spond, TeamSnap, Heja or a spreadsheet, use a group's People → Add someone → A list (CSV).">
        <button type="button" className="btn" onClick={() => file.current?.click()}><Icon name="upload" size={15} />Bring a Pocket League export…</button>
        <input ref={file} type="file" accept=".json,application/json" hidden onChange={e => { const f = e.target.files?.[0]; if (f) bring(f); e.target.value = '' }} />
      </Section>}
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-league.db</code> in the data folder.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
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
  const tiles: [string, number | undefined, string][] = [['Groups', counts.members, 'users'], ['Answers', counts.answers, 'check'], ['Messages', counts.messages, 'message'], ['In other apps', counts.made, 'link']]
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
          <h3>Pocket League</h3>
          <p className="muted small">Sport with friends: groups for tennis, football, padel and more — the next game and who is in, the scores, the table with ratings, and a chat. Players get an account and a sign-in link. One binary, one SQLite file, one volume.</p>
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
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-league" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Calendar, Health, Habits, Split, Money, Weather, Places, Contacts, Mail, Tasks, Day, Search, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}
