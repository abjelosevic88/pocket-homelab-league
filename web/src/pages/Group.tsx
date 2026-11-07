// One group: its games, chat, table, results and people.
import React, { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useLiveEvents, useOnChange } from '../lib/data'
import { fmtDate, fmtDateTime } from '../lib/format'
import { WEEKDAYS, money, statInfo, when } from '../lib/league'
import type { Award, Game, Group as GroupT, Match, Member, Reliability, Row, Season, Venue } from '../lib/types'
import { cap, FormDots, GameCard, MatchRow } from '../components/Bits'
import { Chat } from '../components/Chat'
import { GroupForm } from '../components/GroupForm'
import { Icon } from '../components/icons'
import { openCreate, useCrumb } from '../components/Layout'
import { AddPerson, EditMember, LinkSheet } from '../components/People'
import { MoneyTab } from '../components/MoneyTab'
import { ResultForm } from '../components/ResultForm'
import { TournamentList } from './Tournaments'
import type { Tournament } from '../lib/tour'
import { Avatar, Confirm, Empty, Field, Loading, type MenuItem, MenuButton, Modal, Segmented, Tabs, useToast } from '../components/ui'
import { Select } from '../components/Select'

type Tab = 'games' | 'chat' | 'table' | 'results' | 'tournaments' | 'money' | 'people'
interface Detail { group: GroupT; members: Member[]; upcoming: Game[]; feed_url: string; venues: Venue[] }

export default function Group() {
  const { id } = useParams()
  const gid = Number(id)
  const [sp, setSp] = useSearchParams()
  const tab = (['games', 'chat', 'table', 'results', 'tournaments', 'money', 'people'].includes(sp.get('tab') || '') ? sp.get('tab') : 'games') as Tab
  const navigate = useNavigate()
  const { toast } = useToast()
  const places = useAppState()('places', 'places:write')
  const [d, setD] = useState<Detail | null>(null)
  const [err, setErr] = useState('')
  const [editing, setEditing] = useState(false)
  const [leave, setLeave] = useState(false)
  const load = useCallback(() => api.get<Detail>(`${V1}/groups/${gid}`).then(x => { setD(x); setErr('') }).catch(e => setErr((e as Error).message)), [gid])
  useEffect(() => { setD(null); load() }, [load])
  useOnChange(load)
  useLiveEvents(e => { if ((e.type === 'members' || e.type === 'game' || e.type === 'group' || e.type === 'answer') && (e.data as { group_id?: number; id?: number }).group_id === gid) load() })
  useCrumb(d?.group.name)
  if (err) return <Empty icon="alert" title="Not found">This group does not exist, or you are not in it. <Link to="/">Home</Link></Empty>
  if (!d) return <Loading />
  const g = d.group
  const org = g.my_role === 'organiser'
  const go = (t: Tab) => setSp(t === 'games' ? {} : { tab: t }, { replace: true })
  const setNotify = async (n: string) => { try { await api.put(`${V1}/groups/${gid}/me`, { notify: n }); toast('Saved'); load(); announceChange() } catch (e) { toast((e as Error).message, 'err') } }
  const saveVenue = async () => { try { const r = await api.post<{ url: string }>(`${V1}/groups/${gid}/venue/save`); toast('Saved to Pocket Places', 'ok', r.url ? { label: 'Open', run: () => window.open(r.url, '_blank') } : undefined) } catch (e) { toast((e as Error).message, 'err') } }
  const copyFeed = async () => { try { await navigator.clipboard.writeText(d.feed_url); toast('Copied: add it to Apple, Google or Outlook calendar as a subscription') } catch { toast(d.feed_url) } }
  const menu = (): MenuItem[] => [
    ...(org ? [{ label: 'Edit the group', icon: 'edit', onSelect: () => setEditing(true) }] : []),
    { label: 'Subscribe in any calendar', icon: 'calendar', onSelect: copyFeed },
    ...(g.venue_name ? [{ label: places.ok ? 'Save the venue to Pocket Places' : 'Save the venue to Pocket Places (not connected)', icon: 'pin', disabled: !places.ok, onSelect: saveVenue }] : []),
    'sep',
    { heading: 'Tell me about' },
    { label: 'Everything', checked: g.my_notify === 'all', onSelect: () => setNotify('all') },
    { label: 'Only games', checked: g.my_notify === 'games', onSelect: () => setNotify('games') },
    { label: 'Nothing', checked: g.my_notify === 'off', onSelect: () => setNotify('off') },
    'sep',
    { label: 'Leave the group', icon: 'log-out', danger: true, onSelect: () => setLeave(true) },
  ]
  const usual = g.weekday ? `${WEEKDAYS[g.weekday]}s${g.time ? ` at ${g.time}` : ''}` : ''
  return (
    <div className={`page stack loose group-page tab-${tab}`}>
      <header className="group-head" style={{ ['--grp' as string]: g.color || 'var(--accent)' }}>
        <span className="group-head-emoji" aria-hidden="true">{g.sport_info?.emoji}</span>
        <div className="grow min0">
          <h1 className="page-title ellipsis">{g.name}{g.archived && <span className="badge">archived</span>}</h1>
          <p className="muted small group-facts">
            <span>{g.sport_label}</span>
            {usual && <span><Icon name="repeat" size={12} /> {usual}</span>}
            {g.venue_name && <span title={d.venues.map(v => v.name).join(', ')}><Icon name="pin" size={12} /> {g.venue_name}{d.venues.length > 1 ? ` +${d.venues.length - 1}` : ''}</span>}
            <span><Icon name="users" size={12} /> {g.members}</span>
            {g.cost_cents > 0 && <span><Icon name="wallet" size={12} /> {money(g.cost_cents, g.currency)} a game</span>}
          </p>
        </div>
        <div className="row group-tools">
          <button type="button" className="btn primary hide-sm" onClick={() => openCreate('game', gid)}><Icon name="calendar-plus" size={15} />Plan a game</button>
          <button type="button" className="btn hide-sm" onClick={() => openCreate('result', gid)}><Icon name="trophy" size={15} />Enter a result</button>
          <MenuButton label="More" items={menu} align="right" width={260}><Icon name="more" size={18} /></MenuButton>
        </div>
      </header>
      <Tabs value={tab} onChange={go} tabs={[{ id: 'games', label: 'Games', icon: 'calendar' }, { id: 'chat', label: g.unread ? `Chat (${g.unread})` : 'Chat', icon: 'message' }, { id: 'table', label: 'Table', icon: 'chart' }, { id: 'results', label: 'Results', icon: 'trophy' }, { id: 'tournaments', label: 'Tournaments', icon: 'layers' }, { id: 'money', label: 'Money', icon: 'wallet' }, { id: 'people', label: 'People', icon: 'users' }]} />
      {tab === 'games' && <GamesTab d={d} />}
      {tab === 'chat' && <div className="card chat-card"><Chat groupId={gid} organiser={org} aiCatchUp /></div>}
      {tab === 'table' && <TableTab g={g} />}
      {tab === 'results' && <ResultsTab g={g} />}
      {tab === 'tournaments' && <TournamentsTab g={g} />}
      {tab === 'money' && <MoneyTab g={g} onEditGroup={() => setEditing(true)} />}
      {tab === 'people' && <PeopleTab d={d} reload={load} />}
      {editing && <GroupForm group={g} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); load() }} />}
      {leave && <Confirm title={`Leave ${g.name}?`} confirmLabel="Leave" message="You no longer see its games and chat. Your results stay in its table." onCancel={() => setLeave(false)}
        onConfirm={async () => { setLeave(false); try { await api.post(`${V1}/groups/${gid}/leave`); announceChange(); navigate('/') } catch (e) { toast((e as Error).message, 'err') } }} />}
    </div>
  )
}

function GamesTab({ d }: { d: Detail }) {
  const g = d.group
  const [past, setPast] = useState<Game[] | null>(null)
  useEffect(() => { api.get<Game[]>(`${V1}/games?group=${g.id}&past=1&limit=30`).then(setPast).catch(() => setPast([])) }, [g.id, d])
  return (
    <div className="stack loose">
      {g.description && <p className="notes card pad-card">{g.description}</p>}
      <section className="stack">
        <div className="sec-head"><h3>Coming up</h3></div>
        {d.upcoming.length ? <div className="game-list">{d.upcoming.map(x => <GameCard key={x.id} game={{ ...x, emoji: g.sport_info?.emoji, group_name: g.name, group_color: g.color }} showGroup={false} />)}</div>
          : <p className="muted">Nothing planned. <button type="button" className="linkish" onClick={() => openCreate('game', g.id)}>Plan a game</button>{g.weekday && !g.plan_days ? ' — or let League plan the usual game (Edit the group).' : '.'}</p>}
      </section>
      <section className="stack">
        <div className="sec-head"><h3>Played</h3></div>
        {past === null ? <Loading /> : past.length === 0 ? <p className="muted">No games yet.</p> : <div className="card flush">{past.map(x => (
          <Link key={x.id} to={`/games/${x.id}`} className="list-row">
            <span className="list-ico"><Icon name={x.status === 'cancelled' ? 'x-circle' : x.matches ? 'trophy' : 'calendar'} size={15} /></span>
            <span className="row-main grow"><span className="row-title">{cap(fmtDate(x.starts_at, false))}{x.title && ` · ${x.title}`}</span>
              <span className="muted small ellipsis">{x.status === 'cancelled' ? `cancelled${x.cancel_reason ? `: ${x.cancel_reason}` : ''}` : `${x.in} played · ${x.matches ? `${x.matches} ${x.matches === 1 ? 'result' : 'results'}` : 'no score yet'}`}{x.comments ? ` · ${x.comments} comments` : ''}</span></span>
            <Icon name="chevron-right" size={15} className="muted" />
          </Link>
        ))}</div>}
      </section>
    </div>
  )
}

const PERIODS = [{ value: 'all', label: 'All time' }, { value: 'year', label: 'This year' }, { value: 'month', label: 'Month' }]

interface TableResp { rows: Row[]; frozen?: boolean; awards?: Award[]; season?: Season }

function TableTab({ g }: { g: GroupT }) {
  const { toast } = useToast()
  const org = g.my_role === 'organiser'
  const [seasons, setSeasons] = useState<{ seasons: Season[]; current: number } | null>(null)
  const [sel, setSel] = useState<string>('')
  const [t, setT] = useState<TableResp | null>(null)
  const [newSeason, setNewSeason] = useState<{ name: string; starts_on: string; ends_on: string } | null>(null)
  const [closing, setClosing] = useState<Season | null>(null)
  const loadSeasons = useCallback(() => api.get<{ seasons: Season[]; current: number }>(`${V1}/groups/${g.id}/seasons`).then(r => { setSeasons(r); setSel(x => x || (r.current ? `s${r.current}` : 'all')) }).catch(() => { setSeasons({ seasons: [], current: 0 }); setSel(x => x || 'all') }), [g.id])
  useEffect(() => { loadSeasons() }, [loadSeasons])
  const load = useCallback(() => {
    if (!sel) return
    const q = sel.startsWith('s') ? `season=${sel.slice(1)}` : `period=${sel}`
    return api.get<TableResp>(`${V1}/groups/${g.id}/table?${q}`).then(setT).catch(() => setT({ rows: [] }))
  }, [g.id, sel])
  useEffect(() => { load() }, [load])
  useLiveEvents(e => { if (e.type === 'match' && e.data.group_id === g.id) load(); if (e.type === 'group' && (e.data as { id?: number }).id === g.id) loadSeasons() })
  const rows = t?.rows || null
  const season = sel.startsWith('s') ? seasons?.seasons.find(x => `s${x.id}` === sel) : undefined
  const ranking = g.scoring === 'ranking'
  const sets = g.scoring === 'sets'
  const draws = g.draw_points > 0 || g.scoring === 'goals'
  const goals = g.scoring === 'goals' && g.format === 'teams'
  const statCols = (g.stats || []).filter(k => (rows || []).some(r => r.stats?.[k]))
  const played = (rows || []).filter(r => r.played > 0)
  const resting = (rows || []).filter(r => r.played === 0 && r.member)
  const today = new Date().toISOString().slice(0, 10)
  const seasonOpts = (seasons?.seasons || []).map(x => ({ value: `s${x.id}`, label: x.name, hint: x.closed_at ? 'closed' : x.id === seasons?.current ? 'now' : undefined }))
  const addSeason = async (e: React.FormEvent) => {
    e.preventDefault(); if (!newSeason) return
    try { const x = await api.post<Season>(`${V1}/groups/${g.id}/seasons`, newSeason); setNewSeason(null); await loadSeasons(); setSel(`s${x.id}`); toast('Season started') } catch (err) { toast((err as Error).message, 'err') }
  }
  const seasonMenu = (x: Season): MenuItem[] => [
    ...(!x.closed_at ? [{ label: 'Close the season…', icon: 'flag', onSelect: () => setClosing(x) }] : [{ label: 'Open it again', icon: 'undo', onSelect: async () => { try { await api.post(`${V1}/seasons/${x.id}/close`, { reopen: true }); loadSeasons(); load() } catch (e) { toast((e as Error).message, 'err') } } }]),
    { label: 'Delete the season', icon: 'trash', danger: true, onSelect: async () => { try { await api.del(`${V1}/seasons/${x.id}`); setSel('all'); loadSeasons() } catch (e) { toast((e as Error).message, 'err') } } },
  ]
  return (
    <div className="stack">
      <div className="row wrap">
        <Segmented size="sm" value={sel.startsWith('s') ? null : sel} onChange={setSel} options={PERIODS} />
        {seasonOpts.length > 0 && <Select ariaLabel="Season" className="season-sel" value={sel.startsWith('s') ? sel : ''} placeholder="Seasons" onChange={v => v && setSel(v)} options={seasonOpts} />}
        <span className="spacer" />
        {org && <button type="button" className="btn sm" onClick={() => setNewSeason({ name: suggestSeason(), starts_on: today, ends_on: '' })}><Icon name="plus" size={14} />New season</button>}
        {org && season && <MenuButton label="Season" items={() => seasonMenu(season)} align="right" width={220}><Icon name="more" size={16} /></MenuButton>}
      </div>
      {season && <p className="muted small">{season.name}: {fmtDate(season.starts_on, false)}{season.ends_on ? ` – ${fmtDate(season.ends_on, false)}` : ', still on'}{t?.frozen ? ' · the final table' : ''}{g.rating_scope === 'season' ? ' · ratings start at 1000 each season' : ''}</p>}
      {t?.frozen && (t.awards || []).length > 0 && <div className="awards">{t.awards!.map(a => (
        <div key={a.key} className={`award ${a.key.startsWith('place') ? 'medal' : ''}`}><span className="award-ico" aria-hidden="true">{a.icon}</span><span className="grow min0"><span className="muted small block-line">{a.title}</span><b className="ellipsis block-line">{a.name}</b><span className="muted small">{a.value}</span></span></div>
      ))}</div>}
      {rows === null ? <Loading /> : played.length === 0 ? <Empty icon="chart" title="No results yet">{season ? 'No results in this season yet.' : 'Enter the first result and the table starts.'} <button type="button" className="linkish" onClick={() => openCreate('result', g.id)}>Enter a result</button></Empty> :
        <div className="card flush scroll-x">
          <table className="table league-table">
            <thead><tr>
              <th className="num">#</th><th>Player</th><th className="num" title="Played">P</th>
              {ranking ? <><th className="num" title="First places">Wins</th><th className="num hide-sm">Average</th><th className="num hide-sm">Best</th></> : <>
                <th className="num" title="Won">W</th>{draws && <th className="num" title="Drawn">D</th>}<th className="num" title="Lost">L</th>
                <th className="num hide-sm" title={sets ? 'Games won and lost' : 'Scored and conceded'}>{sets ? 'Games' : '+/−'}</th>
              </>}
              {goals && <th className="num hide-sm" title="Own goals scored">⚽</th>}
              {statCols.map(k => <th key={k} className="num hide-sm" title={statInfo(k).name}>{statInfo(k).icon}</th>)}
              <th className="num" title="Points">Pts</th>
              <th className="hide-sm">Form</th><th className="num" title="Rating">Rating</th>
            </tr></thead>
            <tbody>{played.map(r => (
              <tr key={r.user_id} className={r.position === 1 ? 'top' : ''}>
                <td className="num muted">{r.position}</td>
                <td><Link to={`/groups/${g.id}/players/${r.user_id}`} className="tbl-name">{r.name}</Link>{r.mvp > 0 && <span className="mvp-badge" title={`Player of the game ${r.mvp}×`}>★{r.mvp > 1 ? r.mvp : ''}</span>}</td>
                <td className="num">{r.played}</td>
                {ranking ? <><td className="num">{r.wins}</td><td className="num hide-sm">{r.average ?? ''}</td><td className="num hide-sm">{r.best ?? ''}</td></> : <>
                  <td className="num">{r.won}</td>{draws && <td className="num">{r.drawn}</td>}<td className="num">{r.lost}</td>
                  <td className="num hide-sm muted">{sets ? `${r.for}–${r.against}` : `${r.for - r.against > 0 ? '+' : ''}${r.for - r.against}`}</td>
                </>}
                {goals && <td className="num hide-sm">{r.scored || ''}</td>}
                {statCols.map(k => <td key={k} className="num hide-sm">{r.stats?.[k] || ''}</td>)}
                <td className="num"><b>{r.points}</b></td>
                <td className="hide-sm"><FormDots form={r.form} /></td>
                <td className="num tnum">{Math.round(r.rating)}{r.change !== 0 && !t?.frozen && <span className={`rt-ch ${r.change > 0 ? 'up' : 'down'}`}>{r.change > 0 ? '+' : ''}{Math.round(r.change)}</span>}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>}
      {resting.length > 0 && <p className="muted small">Not played yet{sel !== 'all' ? ' in this period' : ''}: {resting.map(r => r.name).join(', ')}</p>}
      <p className="muted small">Rating: everyone starts at 1000; beating someone rated higher moves you up more.</p>
      {newSeason && <Modal title="A new season" onClose={() => setNewSeason(null)} size="sm">
        <form className="stack" onSubmit={addSeason}>
          <Field label="Name"><input autoFocus value={newSeason.name} onChange={e => setNewSeason({ ...newSeason, name: e.target.value })} maxLength={60} required /></Field>
          <div className="two-col tight">
            <Field label="Starts"><input type="date" value={newSeason.starts_on} onChange={e => setNewSeason({ ...newSeason, starts_on: e.target.value })} required /></Field>
            <Field label="Ends (optional)"><input type="date" value={newSeason.ends_on} min={newSeason.starts_on} onChange={e => setNewSeason({ ...newSeason, ends_on: e.target.value })} /></Field>
          </div>
          <p className="muted small">The season has its own table. Close it when it's over: the table and the awards are kept, and everyone hears who won.</p>
          <div className="form-actions"><button type="button" className="btn" onClick={() => setNewSeason(null)}>Cancel</button><button className="btn primary">Start it</button></div>
        </form>
      </Modal>}
      {closing && <CloseSeason x={closing} onClose={() => setClosing(null)} onDone={() => { setClosing(null); loadSeasons(); load() }} />}
    </div>
  )
}

function suggestSeason(d = new Date()) {
  const m = d.getMonth()
  const name = m < 2 || m === 11 ? 'Winter' : m < 5 ? 'Spring' : m < 8 ? 'Summer' : 'Autumn'
  return `${name} ${d.getFullYear()}`
}

function CloseSeason({ x, onClose, onDone }: { x: Season; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [next, setNext] = useState(true)
  const tomorrow = new Date(Date.now() + 86400000)
  const [name, setName] = useState(suggestSeason(new Date(Date.now() + 86400000 * 7)))
  const [from, setFrom] = useState(tomorrow.toISOString().slice(0, 10))
  const go = async (e: React.FormEvent) => {
    e.preventDefault()
    try { await api.post(`${V1}/seasons/${x.id}/close`, next ? { next: { name, starts_on: from } } : {}); toast(`${x.name} is closed — the awards are in the chat`); onDone() } catch (err) { toast((err as Error).message, 'err') }
  }
  return (
    <Modal title={`Close ${x.name}?`} onClose={onClose} size="sm">
      <form className="stack" onSubmit={go}>
        <p>The table is kept as it is now, the awards are handed out (champion, most games, top scorer …) and everyone hears it.</p>
        <label className="check-row"><input type="checkbox" checked={next} onChange={e => setNext(e.target.checked)} /> Start the next season</label>
        {next && <div className="two-col tight">
          <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} maxLength={60} required /></Field>
          <Field label="Starts"><input type="date" value={from} onChange={e => setFrom(e.target.value)} required /></Field>
        </div>}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Not yet</button><button className="btn primary">Close the season</button></div>
      </form>
    </Modal>
  )
}

function ResultsTab({ g }: { g: GroupT }) {
  const [list, setList] = useState<Match[] | null>(null)
  const [edit, setEdit] = useState<Match | null>(null)
  const load = useCallback(() => api.get<{ match: Match }[]>(`${V1}/matches?group=${g.id}&limit=200`).then(r => setList(r.map(x => x.match))).catch(() => setList([])), [g.id])
  useEffect(() => { load() }, [load])
  useLiveEvents(e => { if (e.type === 'match' && e.data.group_id === g.id) load() })
  if (!list) return <Loading />
  if (!list.length) return <Empty icon="trophy" title="No results yet"><button type="button" className="btn primary" onClick={() => openCreate('result', g.id)}>Enter a result</button></Empty>
  return (
    <>
      <div className="card flush">{list.map(m => <MatchRow key={m.id} m={m} format={g.format} onChanged={load} onEdit={setEdit} />)}</div>
      {edit && <ResultForm match={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load() }} />}
    </>
  )
}

function PeopleTab({ d, reload }: { d: Detail; reload: () => void }) {
  const { user, settings } = useApp()
  const { toast } = useToast()
  const g = d.group
  const org = g.my_role === 'organiser'
  const [adding, setAdding] = useState(false)
  const [link, setLink] = useState<Member | null>(null)
  const [edit, setEdit] = useState<Member | null>(null)
  const [del, setDel] = useState<Member | null>(null)
  const waiting = d.members.filter(m => m.can_link && !m.joined)
  const [rel, setRel] = useState<Record<number, Reliability>>({})
  useEffect(() => { api.get<Reliability[]>(`${V1}/groups/${g.id}/reliability`).then(l => setRel(Object.fromEntries(l.map(x => [x.user_id, x])))).catch(() => {}) }, [g.id, d])
  const inviteAll = async () => {
    try {
      const r = await api.post<{ sent: number; failed: number; error: string; no_email: string[] }>(`${V1}/groups/${g.id}/invite-all`)
      toast(`${r.sent} emailed${r.failed ? `, ${r.failed} not (${r.error})` : ''}${r.no_email.length ? ` · no email: ${r.no_email.join(', ')} — send theirs by WhatsApp` : ''}`, r.failed ? 'err' : 'ok')
      reload()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const emailOff = settings?.email_via === 'none'
  return (
    <div className="stack loose">
      {org && <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add someone</button>
        {waiting.length > 0 && <button type="button" className="btn" onClick={inviteAll} disabled={emailOff} title={emailOff ? 'Email is off: Settings → Email' : undefined}><Icon name="mail" size={15} />Email links to the {waiting.length} who haven't come in</button>}
      </div>}
      <div className="card flush">{d.members.map(m => {
        const items: MenuItem[] = [
          ...(m.can_link ? [{ label: m.joined ? 'Send a new sign-in link' : 'Send their sign-in link', icon: 'link', onSelect: () => setLink(m) }] : []),
          ...(org ? [{ label: 'Edit', icon: 'edit', onSelect: () => setEdit(m) }, { label: 'Take out of the group', icon: 'trash', danger: true, onSelect: () => setDel(m) }] : []),
        ]
        return (
          <div key={m.user_id} className="list-row member-row">
            <span className="member-ava"><Avatar name={m.name} size={34} />{m.online && <span className="online-dot" title="Here now" />}</span>
            <Link to={`/groups/${g.id}/players/${m.user_id}`} className="row-main grow">
              <span className="row-title">{m.name}{m.user_id === user?.id && <span className="badge">you</span>}{m.role === 'organiser' && <span className="badge accent">organiser</span>}</span>
              <span className="muted small ellipsis">
                {m.account === 'player' ? (m.joined ? `came in${m.last_seen_at ? ` · last here ${fmtDateTime(m.last_seen_at, false)}` : ''}` : m.link_sent_at ? `link sent ${fmtDate(m.link_sent_at, false)} · not in yet` : 'not in yet') : 'has an account here'}
                {org && (m.email || m.phone) ? ` · ${[m.email, m.phone].filter(Boolean).join(' · ')}` : ''}
              </span>
              {rel[m.user_id] && rel[m.user_id].games > 0 && <span className="small ellipsis block-line rel-line">{relWords(rel[m.user_id])}</span>}
            </Link>
            {m.can_link && !m.joined && <button type="button" className="btn sm" onClick={() => setLink(m)}><Icon name="link" size={14} />Get them in</button>}
            {items.length > 0 && <MenuButton label={`${m.name}: more`} items={items} align="right" width={240}><Icon name="more" size={16} /></MenuButton>}
          </div>
        )
      })}</div>
      {!org && <p className="muted small">The organisers add people and send their sign-in links.</p>}
      {adding && <AddPerson group={g} onClose={() => setAdding(false)} onAdded={reload} />}
      {link && <LinkSheet group={g} member={link} who={link.name} onClose={() => { setLink(null); reload() }} />}
      {edit && <EditMember group={g} member={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload() }} />}
      {del && <Confirm title={`Take ${del.name} out of ${g.name}?`} confirmLabel="Take out" message="They no longer see this group. Their results stay in the table." onCancel={() => setDel(null)}
        onConfirm={async () => { const x = del; setDel(null); try { await api.del(`${V1}/groups/${g.id}/members/${x.user_id}`); reload(); announceChange() } catch (e) { toast((e as Error).message, 'err') } }} />}
      {g.next_game && <p className="muted small">Next game: {when(g.next_game.starts_at)}.</p>}
    </div>
  )
}

function relWords(r: Reliability) {
  const parts = [`came ${r.came} of ${r.games}`]
  if (r.late_outs) parts.push(`${r.late_outs} late ${r.late_outs === 1 ? 'drop-out' : 'drop-outs'}`)
  if (r.no_shows) parts.push(`${r.no_shows} no-${r.no_shows === 1 ? 'show' : 'shows'}`)
  if (r.no_answer) parts.push(`no answer ${r.no_answer}×`)
  return <><span className={`rel-score ${r.score < 0 ? '' : r.score >= 85 ? 'ok' : r.score >= 65 ? 'mid' : 'low'}`}>{r.score < 0 ? '—' : `${r.score}%`}</span> {parts.join(' · ')}</>
}

function TournamentsTab({ g }: { g: GroupT }) {
  const [list, setList] = useState<Tournament[] | null>(null)
  const load = useCallback(() => api.get<Tournament[]>(`${V1}/groups/${g.id}/tournaments`).then(setList).catch(() => setList([])), [g.id])
  useEffect(() => { load() }, [load])
  useLiveEvents(e => { if (e.type === 'tournament' && e.data.group_id === g.id) load() })
  return (
    <div className="stack">
      <div className="row"><button type="button" className="btn primary" onClick={() => openCreate('tournament', g.id)}><Icon name="trophy" size={15} />New tournament</button></div>
      {list === null ? <Loading /> : list.length === 0 ? <Empty icon="trophy" title="No tournaments yet">A knockout cup, an Americano evening, a ladder for the season…</Empty> : <TournamentList list={list} showGroup={false} />}
    </div>
  )
}
