// One game: when and where, who is in (and the teams), the weather, the
// results, player of the game, the cost, and the comments.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useLiveEvents } from '../lib/data'
import { fmtDateTime } from '../lib/format'
import { ANSWER_WORDS, APP_INFO, money, relDay, when } from '../lib/league'
import type { AppSlug, GameDetail, Match, Person } from '../lib/types'
import { AppOff, offTitle } from '../components/Apps'
import { AnswerButtons, cap, MatchRow } from '../components/Bits'
import { Chat } from '../components/Chat'
import { GameForm } from '../components/GameForm'
import { Icon } from '../components/icons'
import { openCreate, useCrumb } from '../components/Layout'
import { ResultForm } from '../components/ResultForm'
import { Select } from '../components/Select'
import { Avatar, Confirm, Empty, Loading, type MenuItem, MenuButton, Modal, Segmented, download, useToast } from '../components/ui'

interface Weather { available: boolean; why?: string; not_connected?: boolean; text?: string; warning?: string; place?: string; at_home?: boolean; outdoor?: boolean; open_url?: string }

export default function Game() {
  const { id } = useParams()
  const gameId = Number(id)
  const navigate = useNavigate()
  const { toast } = useToast()
  const { user } = useApp()
  const appState = useAppState()
  const [d, setD] = useState<GameDetail | null>(null)
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState(false)
  const [cancel, setCancel] = useState<string | null>(null)
  const [del, setDel] = useState(false)
  const [fix, setFix] = useState<Match | null>(null)
  const [wx, setWx] = useState<Weather | null>(null)
  const [busy, setBusy] = useState<Record<string, string[]>>({})
  const load = useCallback(() => api.get<GameDetail>(`${V1}/games/${gameId}`).then(x => { setD(x); setErr('') }).catch(e => setErr((e as Error).message)), [gameId])
  useEffect(() => { setD(null); load() }, [load])
  useLiveEvents(e => {
    if ((e.type === 'game' && e.data.id === gameId) || (e.type === 'answer' && e.data.game_id === gameId) || (e.type === 'match' && e.data.game_id === gameId)) {
      if (e.type === 'game' && e.data.deleted) navigate('/')
      else load()
    }
  })
  const startsAt = d?.game.starts_at
  useEffect(() => {
    if (!d) return
    api.get<Weather>(`${V1}/games/${gameId}/weather`).then(setWx).catch(() => setWx(null))
    api.get<{ available: boolean; busy: Record<string, string[]> }>(`${V1}/games/${gameId}/busy`).then(r => setBusy(r.busy || {})).catch(() => {})
  }, [gameId, startsAt, d?.game.status]) // eslint-disable-line react-hooks/exhaustive-deps
  useCrumb(d ? (d.game.status === 'voting' ? 'Finding a time' : cap(relDay(d.game.starts_at))) : undefined)
  if (err) return <Empty icon="alert" title="Not found">This game does not exist, or you are not in its group. <Link to="/">Home</Link></Empty>
  if (!d) return <Loading />
  const { game: gm, group: g } = d
  const org = g.my_role === 'organiser'
  const me = user?.id || 0
  const post = async (path: string, body: unknown, ok?: string) => {
    try { const r = await api.post<GameDetail>(`${V1}/games/${gameId}/${path}`, body); setD(r); announceChange(); if (ok) toast(ok) } catch (e) { toast((e as Error).message, 'err') }
  }
  const make = async (what: string, slug: AppSlug, extra: Record<string, unknown> = {}) => {
    try { const m = await api.post<{ title: string; url: string; detail: string }>(`${V1}/games/${gameId}/make`, { what, ...extra }); toast(`${APP_INFO[slug].name}: ${m.title}${m.detail ? ` — ${m.detail}` : ''}`, 'ok', m.url ? { label: 'Open', run: () => window.open(m.url, '_blank') } : undefined); load() } catch (e) { toast((e as Error).message, 'err') }
  }
  const menu = (): MenuItem[] => [
    ...(d.can_edit && gm.status !== 'cancelled' ? [{ label: 'Edit', icon: 'edit', onSelect: () => setEdit(true) }] : []),
    ...(gm.starts_at ? [{ label: 'Download for any calendar (.ics)', icon: 'download', onSelect: () => download(`${V1}/games/${gameId}/game.ics`) }] : []),
    ...(d.can_edit ? ['sep' as const, gm.status === 'cancelled' ? { label: 'Bring it back', icon: 'undo', onSelect: () => post('cancel', { undo: true }, 'The game is on again') } : { label: 'Cancel the game', icon: 'x-circle', danger: true, onSelect: () => setCancel('') },
      { label: 'Delete', icon: 'trash', danger: true, onSelect: () => setDel(true) }] : []),
  ]
  const by = (a: string) => d.people.filter(p => p.answer === a)
  const silent = d.people.filter(p => !p.answer)
  const where = gm.venue_name || g.venue_name
  const addr = gm.venue_address || g.venue_address
  const mapURL = where ? `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent([where, addr].filter(Boolean).join(', '))}` : ''
  const ended = !!gm.ended
  return (
    <div className="page game-page">
      <div className="game-cols">
        <div className="stack loose min0">
          <header className="game-head" style={{ ['--grp' as string]: g.color || 'var(--accent)' }}>
            <Link to={`/groups/${g.id}`} className="game-group"><span aria-hidden="true">{g.sport_info?.emoji}</span> {g.name}</Link>
            <h1 className="page-title">{gm.status === 'voting' ? 'When can you play?' : gm.title || when(gm.starts_at, true)}</h1>
            <div className="game-facts">
              {gm.status === 'cancelled' && <span className="pill st-overdue"><Icon name="x-circle" size={12} />Cancelled{gm.cancel_reason ? `: ${gm.cancel_reason}` : ''}</span>}
              {gm.status === 'planned' && gm.title && <span><Icon name="clock" size={13} /> {when(gm.starts_at, true)}</span>}
              {gm.status === 'planned' && <span><Icon name="timer" size={13} /> {gm.duration_min} min</span>}
              {where && <a href={mapURL} target="_blank" rel="noreferrer"><Icon name="pin" size={13} /> {where}{addr ? `, ${addr}` : ''}</a>}
              {gm.cost_cents > 0 && <span><Icon name="wallet" size={13} /> {money(gm.cost_cents, gm.currency)}{d.share_cost ? ` · ${money(d.share_cost, gm.currency)} each` : ''}</span>}
              {gm.auto && <span className="muted"><Icon name="repeat" size={13} /> the usual game</span>}
            </div>
            {gm.notes && <p className="notes">{gm.notes}</p>}
            <div className="row wrap game-tools">
              {gm.status === 'planned' && !ended && <AnswerButtons key={gm.my_answer + gm.updated_at} game={gm} onChange={() => load()} />}
              <span className="spacer" />
              {menu().length > 0 && <MenuButton label="More" items={menu} align="right" width={250}><Icon name="more" size={18} /></MenuButton>}
            </div>
            {gm.status === 'planned' && busy[gm.starts_at]?.length > 0 && <p className="small warn-text"><Icon name="calendar" size={12} /> In your calendar then: {busy[gm.starts_at].join('; ')}</p>}
          </header>

          {gm.status === 'voting' && <Voting d={d} busy={busy} onSaved={setD} />}

          {gm.status === 'planned' && <Who d={d} by={by} silent={silent} org={org} onAnswer={(uid, a) => post('answer', { user_id: uid, answer: a })} />}

          {gm.status === 'planned' && g.format === 'teams' && by('in').length >= 2 && <Teams d={d} onPost={(b, ok) => post('teams', b, ok)} />}

          {gm.status === 'planned' && <section className="stack">
            <div className="sec-head"><Icon name="trophy" size={16} /><h3>{d.matches.length > 1 ? 'Results' : 'Result'}</h3><span className="spacer" />
              {(ended || d.matches.length > 0 || parseStart(gm.starts_at) < Date.now()) && <button type="button" className="btn sm" onClick={() => openCreate('result', g.id, gm.id)}><Icon name="plus" size={14} />{d.matches.length ? 'Another result' : 'Enter the score'}</button>}</div>
            {d.matches.length ? <div className="card flush">{d.matches.map(m => <div key={m.id}><MatchRow m={m} format={g.format} onChanged={load} onEdit={setFix} /><Report m={m} gid={g.id} /></div>)}</div>
              : <p className="muted">{ended ? 'How did it go? Enter the score.' : 'After the game, enter the score here.'}</p>}
          </section>}

          {gm.status === 'planned' && d.mvp.open && <MVP d={d} me={me} onVote={id => post('mvp', { for: id }, id ? 'Voted' : 'Vote taken back')} />}
        </div>

        <aside className="stack loose min0 game-side">
          {gm.starts_at && <WeatherCard wx={wx} slugState={appState('weather', 'weather:read')} outdoor={g.outdoor} />}
          {gm.status === 'planned' && <InApps d={d} make={make} appState={appState} />}
          <section className="card chat-card comments">
            <div className="sec-head pad-x"><Icon name="message" size={16} /><h3>Comments</h3></div>
            <Chat groupId={g.id} gameId={gm.id} organiser={org} />
          </section>
        </aside>
      </div>

      {edit && <GameForm game={gm} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load() }} />}
      {fix && <ResultForm match={fix} onClose={() => setFix(null)} onSaved={() => { setFix(null); load() }} />}
      {cancel !== null && <Modal title="Cancel the game?" onClose={() => setCancel(null)} size="sm">
        <form className="stack" onSubmit={e => { e.preventDefault(); const r = cancel; setCancel(null); post('cancel', { reason: r }, 'Cancelled: everyone is told') }}>
          <input autoFocus value={cancel} onChange={e => setCancel(e.target.value)} placeholder="Why? (rain, not enough players…)" maxLength={300} />
          <p className="muted small">Everyone in the group is told, and the game leaves their calendars.</p>
          <div className="form-actions"><button type="button" className="btn" onClick={() => setCancel(null)}>Keep it</button><button className="btn danger">Cancel the game</button></div>
        </form>
      </Modal>}
      {del && <Confirm title="Delete this game?" confirmLabel="Delete" message="It goes for everyone (the results stay in the table). To call it off, cancel it instead." onCancel={() => setDel(false)}
        onConfirm={async () => { setDel(false); try { await api.del(`${V1}/games/${gameId}`); announceChange(); navigate(`/groups/${g.id}`) } catch (e) { toast((e as Error).message, 'err') } }} />}
    </div>
  )
}

function parseStart(ts: string) { return ts ? new Date(ts).getTime() : 0 }

function Voting({ d, busy, onSaved }: { d: GameDetail; busy: Record<string, string[]>; onSaved: (d: GameDetail) => void }) {
  const { user } = useApp()
  const { toast } = useToast()
  const me = String(user?.id || 0)
  const names = Object.fromEntries(d.people.map(p => [String(p.user_id), p.name]))
  const vote = async (oid: number, a: string) => { try { onSaved(await api.post<GameDetail>(`${V1}/games/${d.game.id}/votes`, { votes: { [oid]: a } })) } catch (e) { toast((e as Error).message, 'err') } }
  const pick = async (oid: number) => { try { onSaved(await api.post<GameDetail>(`${V1}/games/${d.game.id}/pick`, { option_id: oid })); announceChange(); toast('Planned: everyone is told') } catch (e) { toast((e as Error).message, 'err') } }
  const voters = new Set(d.options.flatMap(o => Object.keys(o.votes)))
  return (
    <section className="stack">
      <div className="sec-head"><Icon name="calendar" size={16} /><h3>Which times suit you?</h3><span className="muted small">{voters.size} of {d.people.length} voted{d.game.vote_until ? ` · until ${when(d.game.vote_until)}` : ''}</span></div>
      <div className="card flush">{d.options.map(o => {
        const yes = Object.entries(o.votes).filter(([, v]) => v === 'yes').map(([k]) => names[k] || '?')
        const maybe = Object.entries(o.votes).filter(([, v]) => v === 'maybe').map(([k]) => names[k] || '?')
        return (
          <div key={o.id} className="opt-row">
            <div className="grow min0">
              <div className="row-title">{when(o.starts_at)}</div>
              <div className="muted small ellipsis">{yes.length} yes{yes.length ? ` (${yes.map(n => n.split(' ')[0]).join(', ')})` : ''}{maybe.length ? ` · ${maybe.length} maybe` : ''}</div>
              {busy[o.starts_at]?.length > 0 && <div className="small warn-text"><Icon name="calendar" size={11} /> You have: {busy[o.starts_at].join('; ')}</div>}
            </div>
            <Segmented size="sm" value={o.votes[me] || null} onChange={v => vote(o.id, o.votes[me] === v ? '' : v)} options={[{ value: 'yes', label: 'Yes' }, { value: 'maybe', label: 'Maybe' }, { value: 'no', label: 'No' }]} />
            {d.can_edit && <button type="button" className="btn sm" onClick={() => pick(o.id)} title="Plan the game at this time">Pick</button>}
          </div>
        )
      })}</div>
    </section>
  )
}

function Who({ d, by, silent, org, onAnswer }: { d: GameDetail; by: (a: string) => Person[]; silent: Person[]; org: boolean; onAnswer: (uid: number, a: string) => void }) {
  const gm = d.game
  const short = gm.min_players - gm.in
  const col = (title: string, list: Person[], cls: string) => list.length > 0 && (
    <div className={`who-col ${cls}`}>
      <h4 className="form-sec-h">{title} <span className="count">{list.length}</span></h4>
      {list.map(p => (
        <div key={p.user_id} className="who">
          <Avatar name={p.name} size={26} />
          <span className="grow min0"><span className="ellipsis block-line">{p.name}</span>{p.note && <span className="muted small ellipsis block-line">{p.note}</span>}</span>
          {org && <MenuButton label={`Answer for ${p.name}`} className="btn ghost icon-only sm" align="right" width={200}
            items={[{ heading: `Answer for ${p.name.split(' ')[0]}` }, ...(['in', 'maybe', 'out'] as const).map(a => ({ label: ANSWER_WORDS[a], checked: p.answer === a, onSelect: () => onAnswer(p.user_id, a) }))]}>
            <Icon name="more" size={14} /></MenuButton>}
        </div>
      ))}
    </div>
  )
  return (
    <section className="stack">
      <div className="sec-head"><Icon name="users" size={16} /><h3>Who's coming</h3>
        <span className={`fill fill-${short > 0 ? 'warn' : gm.max_players && gm.in >= gm.max_players ? 'full' : 'ok'}`}>{gm.in} in{gm.max_players ? ` of ${gm.max_players}` : ''}{short > 0 ? ` · ${short} short` : ''}</span></div>
      <div className="who-grid">
        {col('In', by('in'), 'in')}
        {col('Waiting list', by('waiting'), 'waiting')}
        {col('Maybe', by('maybe'), 'maybe')}
        {col('Out', by('out'), 'out')}
        {col('No answer yet', silent, 'silent')}
      </div>
    </section>
  )
}

function Teams({ d, onPost }: { d: GameDetail; onPost: (b: unknown, ok?: string) => void }) {
  const ins = d.people.filter(p => p.answer === 'in')
  const a = ins.filter(p => p.side === 'a'), b = ins.filter(p => p.side === 'b'), none = ins.filter(p => !p.side)
  const avg = (l: Person[]) => l.length ? Math.round(l.reduce((s, p) => s + (p.rating || 1000), 0) / l.length) : 0
  const move = (p: Person) => {
    const next = p.side === 'a' ? 'b' : p.side === 'b' ? '' : 'a'
    const sides = Object.fromEntries(ins.map(x => [String(x.user_id), x.user_id === p.user_id ? next : x.side]))
    onPost({ mode: 'set', sides })
  }
  const has = a.length + b.length > 0
  const team = (title: string, l: Person[], cls: string) => (
    <div className={`team-col ${cls}`}>
      <h4 className="form-sec-h">{title} <span className="count">{l.length}</span>{l.length > 0 && <span className="muted small"> · rating {avg(l)}</span>}</h4>
      {l.map(p => <button key={p.user_id} type="button" className="team-chip" onClick={() => move(p)} title="Tap to move">{p.name}</button>)}
    </div>
  )
  return (
    <section className="stack">
      <div className="sec-head"><Icon name="shield" size={16} /><h3>Teams</h3><span className="spacer" />
        <button type="button" className="btn sm primary" onClick={() => onPost({ mode: 'fair' }, 'Fair teams by rating')}><Icon name="scale" size={14} />{has ? 'Fair teams again' : 'Pick fair teams'}</button>
        {has && <button type="button" className="btn sm" onClick={() => onPost({ mode: 'shuffle', seed: Date.now() }, 'Another fair split')}><Icon name="refresh" size={14} />Shuffle</button>}
        {has && <button type="button" className="btn sm ghost" onClick={() => onPost({ mode: 'clear' })}>Clear</button>}
      </div>
      {has ? <div className="teams">{team('Team A', a, 'side-a')}{team('Team B', b, 'side-b')}{none.length > 0 && team('Not picked', none, 'side-none')}</div>
        : <p className="muted">League splits the {ins.length} who are in into two teams with ratings as close as it can. Tap a name to move them.</p>}
      {has && Math.abs(avg(a) - avg(b)) > 0 && <p className="muted small">Average ratings {avg(a)} against {avg(b)}.</p>}
    </section>
  )
}

function MVP({ d, me, onVote }: { d: GameDetail; me: number; onVote: (id: number) => void }) {
  const played = new Set([...d.people.filter(p => p.answer === 'in').map(p => p.user_id), ...d.matches.flatMap(m => m.players.map(p => p.user_id))])
  const candidates = d.people.filter(p => played.has(p.user_id) && p.user_id !== me)
  if (!played.has(me) || candidates.length === 0) return null
  const tally = Object.entries(d.mvp.tally).sort((a, b) => b[1] - a[1])
  const top = tally[0]
  const name = (id: string) => d.people.find(p => String(p.user_id) === id)?.name || '?'
  return (
    <section className="stack">
      <div className="sec-head"><Icon name="star" size={16} /><h3>Player of the game</h3>{top && <span className="muted small">so far: <b>{name(top[0])}</b> ({top[1]})</span>}</div>
      <div className="row wrap">
        <Select ariaLabel="Your vote" className="mvp-sel" value={d.mvp.mine ? String(d.mvp.mine) : ''} placeholder="Your vote" clearLabel="No vote" onChange={v => onVote(Number(v) || 0)}
          options={candidates.map(p => ({ value: String(p.user_id), label: p.name, hint: d.mvp.tally[String(p.user_id)] ? `${d.mvp.tally[String(p.user_id)]} ★` : undefined }))} />
      </div>
    </section>
  )
}

function Report({ m, gid }: { m: Match; gid: number }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [text, setText] = useState<string | null>(null)
  if (!settings?.ai_help) return null
  const write = async () => { setText(''); try { setText((await api.post<{ text: string }>(`${V1}/matches/${m.id}/report`)).text) } catch (e) { setText(null); toast((e as Error).message, 'err') } }
  const postIt = async () => { try { await api.post(`${V1}/groups/${gid}/messages`, { body: text, client_id: `rep${m.id}${Date.now()}` }); toast('Posted in the group chat'); setText(null) } catch (e) { toast((e as Error).message, 'err') } }
  return <>
    <div className="report-line"><button type="button" className="linkish small" onClick={write}><Icon name="sparkles" size={12} /> Write a match report</button></div>
    {text !== null && <Modal title="Match report" onClose={() => setText(null)}>
      {text === '' ? <p className="muted">Writing…</p> : <div className="stack">
        <textarea rows={5} value={text} onChange={e => setText(e.target.value)} />
        <p className="muted small">Drafted by Pocket AI, only for you. Change it as you like; it goes to the chat as your message.</p>
        <div className="form-actions"><button type="button" className="btn" onClick={() => setText(null)}>Close</button><button type="button" className="btn primary" onClick={postIt} disabled={!text.trim()}>Post it in the chat</button></div>
      </div>}
    </Modal>}
  </>
}

function WeatherCard({ wx, slugState, outdoor }: { wx: Weather | null; slugState: ReturnType<ReturnType<typeof useAppState>>; outdoor: boolean }) {
  if (!wx) return null
  if (!wx.available && wx.not_connected) return outdoor ? <AppOff slug="weather" app={slugState.app} compact /> : null
  if (!wx.available) return outdoor ? <div className="card pad-card wx"><div className="sec-head"><Icon name="cloud" size={16} /><h3>Weather</h3></div><p className="muted small">{wx.why}</p></div> : null
  return (
    <div className={`card pad-card wx ${wx.warning ? 'warn' : ''}`}>
      <div className="sec-head"><Icon name={wx.warning?.startsWith('Rain') ? 'cloud-rain' : wx.warning?.startsWith('Windy') ? 'wind' : 'sun'} size={16} /><h3>Weather</h3>{!outdoor && <span className="muted small">indoors</span>}</div>
      {wx.warning && <p className="warn-text"><b>{wx.warning}</b></p>}
      <p>{wx.text}</p>
      <p className="muted small">{wx.at_home ? `At ${wx.place || 'your home place'} (the venue has no position)` : wx.place || 'At the venue'} · Pocket Weather{wx.open_url && <> · <a href={wx.open_url} target="_blank" rel="noreferrer">Open</a></>}</p>
    </div>
  )
}

function InApps({ d, make, appState }: { d: GameDetail; make: (what: string, slug: AppSlug, extra?: Record<string, unknown>) => void; appState: ReturnType<typeof useAppState> }) {
  const { settings } = useApp()
  const gm = d.game
  const made = (app: string) => d.made.find(m => m.app === app)
  const cal = appState('calendar', 'calendar:write'), split = appState('split', 'expenses:write'), money_ = appState('money', 'transactions:write'), health = appState('health', 'health:write'), tasks = appState('tasks', 'tasks:write')
  const playedMine = d.matches.some(m => m.players.some(p => d.people.some(x => x.user_id === p.user_id)))
  const items: { app: AppSlug; label: string; icon: string; st: typeof cal; what: string; done?: ReturnType<typeof made>; show: boolean; note?: string }[] = [
    { app: 'calendar', label: made('calendar') ? 'In your calendar' : 'Add to my calendar', icon: 'calendar-plus', st: cal, what: 'calendar', done: made('calendar'), show: !!gm.starts_at,
      note: settings?.calendar_auto ? 'Games you are in go there by themselves.' : undefined },
    { app: 'split', label: 'Split the cost', icon: 'receipt', st: split, what: 'split', done: made('split'), show: gm.cost_cents > 0, note: gm.cost_cents ? `${money(gm.cost_cents, gm.currency)} between the ${gm.in} who are in.` : '' },
    { app: 'money', label: 'Book my share', icon: 'wallet', st: money_, what: 'money', done: made('money'), show: gm.cost_cents > 0, note: d.share_cost ? `${money(d.share_cost, gm.currency)} as an expense.` : '' },
    { app: 'health', label: 'Record as a workout', icon: 'heart-pulse', st: health, what: 'health', done: made('health'), show: playedMine, note: settings?.health_auto ? 'Your matches go there by themselves.' : undefined },
    { app: 'tasks', label: 'Remind me to book', icon: 'check-square', st: tasks, what: 'tasks', done: made('tasks'), show: d.can_edit && !gm.ended },
  ]
  const shown = items.filter(i => i.show)
  if (!shown.length) return null
  return (
    <section className="card pad-card stack">
      <div className="sec-head"><Icon name="link" size={16} /><h3>In your other apps</h3></div>
      {shown.map(i => (
        <div key={i.app} className="inapp">
          <span className="app-off-ico"><Icon name={APP_INFO[i.app].icon} size={15} /></span>
          <span className="grow min0">
            {i.done ? <a href={i.done.url || undefined} target="_blank" rel="noreferrer" className="block-line">{i.done.title}</a> : <span className="block-line">{i.label}</span>}
            <span className="muted small block-line">{i.done ? `${APP_INFO[i.app].name} · ${fmtDateTime(i.done.created_at, false)}${i.done.detail ? ` · ${i.done.detail}` : ''}` : i.note}</span>
          </span>
          {!i.done && <button type="button" className="btn sm" disabled={!i.st.ok} title={i.st.ok ? undefined : offTitle(i.st.app, i.app)} onClick={() => make(i.what, i.app)}>{i.st.ok ? 'Do it' : 'Not on'}</button>}
        </div>
      ))}
      {!gm.starts_at ? null : <p className="muted small">No Pocket Calendar? <a href={`${V1}/games/${gm.id}/game.ics`}>Download the game</a> for any calendar.</p>}
    </section>
  )
}
