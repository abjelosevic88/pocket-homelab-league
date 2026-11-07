// A tournament: sign-up, the draw (brackets, groups, rounds, leaderboard,
// ladder), what's on now and next, and running it. Also the read-only page
// for anyone with the share link (/t/<token>).
import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useLiveEvents } from '../lib/data'
import { fmtDateTime } from '../lib/format'
import { money, when } from '../lib/league'
import { KIND_WORDS, matchEntries, STATUS_WORDS, type TDetail, type TEntry, type TMatch } from '../lib/tour'
import type { Member } from '../lib/types'
import { Bracket, Leaderboard, TMatchLine, TResultForm, TTable } from '../components/TBits'
import { TournamentForm } from '../components/TournamentForm'
import { Icon } from '../components/icons'
import { useCrumb } from '../components/Layout'
import { Select } from '../components/Select'
import { Confirm, Empty, Loading, type MenuItem, MenuButton, Modal, Segmented, Switch, Tabs, useToast } from '../components/ui'

type Tab = 'now' | 'draw' | 'entries' | 'log'

export default function Tournament() {
  const { id } = useParams()
  const [d, setD] = useState<TDetail | null>(null)
  const [err, setErr] = useState('')
  const load = useCallback(() => api.get<TDetail>(`${V1}/tournaments/${id}`).then(x => { setD(x); setErr('') }).catch(e => setErr((e as Error).message)), [id])
  useEffect(() => { setD(null); load() }, [load])
  useLiveEvents(e => {
    if (e.type === 'tournament' && e.data.id === Number(id)) load()
    if (e.type === 'resync') load()
    if (e.type === 'live' && e.data.tmatch_id) setD(x => {
      if (!x || !x.matches.some(m => m.id === e.data.tmatch_id)) return x
      const live = { ...(x.live || {}) }
      if (e.data.deleted || e.data.finished_at) delete live[String(e.data.tmatch_id)]
      else live[String(e.data.tmatch_id)] = { id: e.data.id, line: e.data.line, view: e.data.view, rules: e.data.rules, a_name: e.data.a_name, b_name: e.data.b_name, updated_at: e.data.updated_at, scorer_name: e.data.scorer_name }
      return { ...x, live }
    })
  })
  useCrumb(d?.tournament.name)
  if (err) return <Empty icon="alert" title="Not found">This tournament does not exist, or you are not in its group. <Link to="/">Home</Link></Empty>
  if (!d) return <Loading />
  return <TournamentView d={d} setD={setD} reload={load} />
}

/** The page for anyone with the link: the same, read only, refreshed now and then. */
export function PublicTournament() {
  const { token } = useParams()
  const [d, setD] = useState<TDetail | null>(null)
  const [err, setErr] = useState('')
  const load = useCallback(() => api.get<TDetail>(`${V1}/public/t/${token}`).then(setD).catch(e => setErr((e as Error).message)), [token])
  const anyLive = Object.keys(d?.live || {}).length > 0
  useEffect(() => { load(); const t = window.setInterval(() => { if (document.visibilityState === 'visible') load() }, anyLive ? 5000 : 20000); return () => window.clearInterval(t) }, [load, anyLive])
  if (err) return <div className="public-page"><Empty icon="alert" title="Not found">This link does not work (any more).</Empty></div>
  if (!d) return <div className="public-page"><Loading /></div>
  return <div className="public-page"><div className="public-brand"><img src="/favicon.svg" alt="" width={22} height={22} /> Pocket League</div><TournamentView d={d} setD={setD} reload={load} readOnly /></div>
}

function TournamentView({ d, setD, reload, readOnly = false }: { d: TDetail; setD: (d: TDetail) => void; reload: () => void; readOnly?: boolean }) {
  const t = d.tournament
  const navigate = useNavigate()
  const { toast } = useToast()
  const [tab, setTab] = useState<Tab>(t.status === 'signup' ? 'entries' : 'now')
  const [result, setResult] = useState<TMatch | null>(null)
  const [editing, setEditing] = useState(false)
  const [ask, setAsk] = useState<null | { title: string; message: string; label: string; run: () => Promise<void> }>(null)
  const act = async (path: string, body?: unknown, ok?: string) => {
    try { setD(await api.post<TDetail>(`${V1}/tournaments/${t.id}/${path}`, body)); if (ok) toast(ok); announceChange() } catch (e) { toast((e as Error).message, 'err') }
  }
  const myMatches = d.matches.filter(m => matchEntries(m).some(e => d.mine.includes(e)) && !['done', 'void'].includes(m.status) && d.matches.every(x => x.parent_id !== m.id))
  const menu = (): MenuItem[] => [
    { label: 'Settings', icon: 'settings', onSelect: () => setEditing(true) },
    { label: 'Copy the share link', icon: 'link', onSelect: () => { navigator.clipboard?.writeText(window.location.origin + '/t/' + (d.share_url || '').split('/t/')[1]); toast('Copied: anyone with it sees the draw, no sign-in needed') } },
    { label: 'Print', icon: 'file', onSelect: () => window.print() },
    'sep',
    ...(t.status === 'live' ? [
      { label: 'Draw again (nothing played yet)', icon: 'refresh', onSelect: () => setAsk({ title: 'Draw again?', message: 'Back to sign-up; the draw is made again when you start.', label: 'Draw again', run: () => act('redraw', undefined, 'Back to sign-up') }) },
      ...(['americano', 'mexicano', 'kotc', 'swiss', 'arena'].includes(t.format) ? [{ label: 'Redraw the rounds not played', icon: 'refresh', onSelect: () => act('regenerate', undefined, 'The coming rounds are drawn again') }] : []),
      ...(t.format === 'box_league' ? [{ label: 'Next round: move up and down', icon: 'arrow-up', onSelect: () => setAsk({ title: 'Start the next round?', message: 'This round finishes; the best of each box go up, the last go down, in a new round.', label: 'Start it', run: async () => { try { const nd = await api.post<TDetail>(`${V1}/tournaments/${t.id}/next-round`); navigate(`/tournaments/${nd.tournament.id}`) } catch (e) { toast((e as Error).message, 'err') } } }) }] : []),
      { label: 'Finish now, as it stands', icon: 'trophy', onSelect: () => setAsk({ title: 'Finish now?', message: 'Matches not played are left out; the places are as they stand.', label: 'Finish', run: () => act('finish', {}, 'Finished') }) },
    ] : []),
    ...(t.status !== 'finished' ? [{ label: 'Cancel the tournament', icon: 'x-circle', danger: true, onSelect: () => setAsk({ title: 'Cancel it?', message: 'Everyone in the group is told.', label: 'Cancel it', run: () => act('finish', { cancel: true }, 'Cancelled') }) }] : []),
    { label: 'Delete', icon: 'trash', danger: true, onSelect: () => setAsk({ title: `Delete ${t.name}?`, message: 'The draw and its results go (results already in the group\'s table stay).', label: 'Delete', run: async () => { await api.del(`${V1}/tournaments/${t.id}`); announceChange(); navigate(`/groups/${t.group_id}?tab=tournaments`) } }) },
  ]
  return (
    <div className="page stack loose tournament-page">
      <header className="t-head">
        <span className="group-head-emoji" aria-hidden="true">{t.status === 'finished' ? '🏆' : t.emoji}</span>
        <div className="grow min0">
          <h1 className="page-title">{t.name}</h1>
          <p className="muted small group-facts">
            {readOnly ? <span>{t.group_name}</span> : <Link to={`/groups/${t.group_id}?tab=tournaments`}>{t.group_name}</Link>}
            <span>{t.format_name}</span>
            <span className={`pill ${t.status === 'live' ? 'st-open' : t.status === 'signup' ? 'st-new' : 'st-closed'}`}>{STATUS_WORDS[t.status]}</span>
            {t.starts_at && <span><Icon name="clock" size={12} /> {when(t.starts_at)}</span>}
            {t.venue_name && <span><Icon name="pin" size={12} /> {t.venue_name}</span>}
            <span><Icon name="users" size={12} /> {t.entries}{t.capacity ? ` of ${t.capacity}` : ''}</span>
            {t.fee_cents > 0 && <span><Icon name="wallet" size={12} /> {money(t.fee_cents, t.currency)}</span>}
            {!t.rated && <span>not rated</span>}
          </p>
        </div>
        {!readOnly && <div className="row group-tools no-print">
          {d.can_run && t.status === 'signup' && <button type="button" className="btn primary" onClick={() => act('start', undefined, 'The draw is made — good luck!')}><Icon name="play" size={15} />Make the draw</button>}
          {d.can_run && <MenuButton label="More" items={menu} align="right" width={280}><Icon name="more" size={18} /></MenuButton>}
          {!d.can_run && <button type="button" className="btn ghost" onClick={() => window.print()} aria-label="Print"><Icon name="file" size={16} /></button>}
        </div>}
      </header>

      {t.status === 'finished' && <Podium d={d} />}
      {!readOnly && t.status === 'signup' && <SignUp d={d} act={act} setD={setD} />}
      {!readOnly && myMatches.length > 0 && t.status === 'live' && <section className="stack">
        <div className="sec-head"><Icon name="target" size={16} /><h3>Your matches</h3></div>
        <div className="card flush">{myMatches.slice(0, 4).map(m => <TMatchLine key={m.id} d={d} m={m} onResult={setResult} onChanged={setD} />)}</div>
      </section>}

      <Tabs value={tab} onChange={setTab} tabs={[
        { id: 'now', label: t.status === 'signup' ? 'How it works' : 'Now', icon: 'activity' },
        { id: 'draw', label: t.format === 'ranking' ? 'Leaderboard' : t.format === 'ladder' ? 'Ladder' : 'Draw', icon: 'layers' },
        { id: 'entries', label: `Entries (${t.entries})`, icon: 'users' },
        ...(d.can_run && !readOnly ? [{ id: 'log' as Tab, label: 'Record', icon: 'list' }] : []),
      ]} />
      {tab === 'now' && <Now d={d} setD={setD} onResult={setResult} readOnly={readOnly} />}
      {tab === 'draw' && <Draw d={d} setD={setD} onResult={setResult} readOnly={readOnly} reload={reload} />}
      {tab === 'entries' && <Entries d={d} setD={setD} readOnly={readOnly} />}
      {tab === 'log' && <div className="card flush">{(d.log || []).map(l => <div key={l.id} className="list-row"><span className="grow"><b>{l.action.replace('_', ' ')}</b> {l.detail}<span className="muted small block-line">{d.names[String(l.user_id)] || 'League'} · {fmtDateTime(l.created_at, false)}</span></span></div>)}</div>}

      {result && <TResultForm d={d} m={result} onClose={() => setResult(null)} onSaved={nd => { setD(nd); setResult(null); announceChange() }} />}
      {editing && <TournamentForm tournament={t} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); reload() }} />}
      {ask && <Confirm title={ask.title} message={ask.message} confirmLabel={ask.label} onCancel={() => setAsk(null)} onConfirm={async () => { const a = ask; setAsk(null); await a.run() }} />}
    </div>
  )
}

function Podium({ d }: { d: TDetail }) {
  const last = d.stages[d.stages.length - 1]
  const places: { rank: number; name: string }[] = (d.podium || []).map(p => ({ rank: p.rank, name: d.entries.find(e => e.id === p.entry_id)?.name || '?' }))
  if (places.length) { /* from the server */ } else if (last?.leaderboard) last.leaderboard.slice(0, 3).forEach(r => places.push({ rank: r.rank, name: d.entries.find(e => e.id === r.id)?.name || '?' }))
  else if (last && Object.keys(last.tables || {}).length) Object.values(last.tables)[0].slice(0, 3).forEach(r => places.push({ rank: r.rank, name: d.entries.find(e => e.id === r.id)?.name || '?' }))
  if (!places.length && d.tournament.winner) places.push({ rank: 1, name: d.tournament.winner })
  const medal = ['', '🥇', '🥈', '🥉']
  return (
    <div className="podium card pad-card">
      <div className="sec-head"><Icon name="trophy" size={16} /><h3>{d.tournament.winner ? `${d.tournament.winner} won` : 'Finished'}</h3></div>
      {places.length > 1 && <div className="podium-row">{places.map(p => <span key={p.rank + p.name} className={`podium-p r${p.rank}`}><span className="podium-m">{medal[p.rank] || p.rank}</span>{p.name}</span>)}</div>}
    </div>
  )
}

function SignUp({ d, act, setD }: { d: TDetail; act: (path: string, body?: unknown, ok?: string) => Promise<void>; setD: (d: TDetail) => void }) {
  const t = d.tournament
  const { user } = useApp()
  const [members, setMembers] = useState<Member[]>([])
  const [partner, setPartner] = useState('')
  const [team, setTeam] = useState('')
  useEffect(() => { api.get<{ members: Member[] }>(`${V1}/groups/${t.group_id}`).then(x => setMembers(x.members)).catch(() => {}) }, [t.group_id])
  const taken = new Set(d.entries.filter(e => e.status !== 'withdrawn').flatMap(e => e.members.map(m => m.user_id)))
  const mine = d.entries.find(e => d.mine.includes(e.id))
  const teams = d.entries.filter(e => e.status !== 'withdrawn' && t.entry_kind === 'team')
  const closed = t.signup_until && d.now > t.signup_until
  return (
    <section className="card pad-card stack signup-card">
      {mine ? <div className="row wrap">
        <Icon name="check-circle" size={18} className="ok" />
        <span className="grow"><b>You're {mine.status === 'waiting' ? 'on the waiting list' : 'in'}</b>{mine.members.length > 1 && <> as {mine.name}</>}{t.checkin && !mine.checked_in && <span className="muted"> · check in on the day</span>}</span>
        {t.checkin && <button type="button" className="btn sm" onClick={async () => { try { setD(await api.put<TDetail>(`${V1}/tournaments/${t.id}/entries/${mine.id}`, { checked_in: !mine.checked_in })) } catch { /* nothing to do */ } }}>{mine.checked_in ? 'Checked in ✓' : 'Check in'}</button>}
        <button type="button" className="btn sm ghost" onClick={() => act('leave', undefined, 'You are out')}>I can't play</button>
      </div> : closed ? <p className="muted">Sign-up closed {fmtDateTime(t.signup_until, false)}.</p> : <div className="stack tight">
        <div className="row wrap">
          {t.entry_kind === 'pair' && <Select className="partner-sel" ariaLabel="Your partner" value={partner} placeholder="Pick your partner" onChange={setPartner}
            options={members.filter(m => m.user_id !== user?.id && !taken.has(m.user_id)).map(m => ({ value: String(m.user_id), label: m.name }))} />}
          {t.entry_kind === 'team' && <Select className="partner-sel" ariaLabel="Team" value={team} placeholder="Your team (or a new one)" onChange={setTeam} onCreate={v => { setTeam(v); return v }} createLabel="A new team"
            options={teams.map(e => ({ value: e.name, label: e.name, hint: `${e.members.length} in` }))} />}
          <button type="button" className="btn primary" disabled={t.entry_kind === 'pair' && !partner}
            onClick={() => act('join', { partner: Number(partner) || 0, team_name: team }, 'You are in')}><Icon name="check" size={15} />Sign me up</button>
        </div>
        <p className="muted small">{t.capacity && t.entries >= t.capacity ? 'It is full: you go on the waiting list and move up when a place comes free.' : t.signup_until ? `Sign up until ${fmtDateTime(t.signup_until, false)}.` : 'Sign up until the organisers make the draw.'}</p>
      </div>}
    </section>
  )
}

function Now({ d, setD, onResult, readOnly }: { d: TDetail; setD: (d: TDetail) => void; onResult: (m: TMatch) => void; readOnly: boolean }) {
  const t = d.tournament
  if (t.status === 'signup') return <div className="card pad-card stack">
    <p>{FORMAT_HELP[t.format] || 'Sign up, and when the organisers make the draw you see who you play.'}</p>
    <p className="muted small">Results: either side enters the score, the other confirms it{d.settings.auto_confirm > 0 ? ` (or it counts by itself after ${d.settings.auto_confirm} minutes)` : ''}. {t.rated ? 'They count in the group\'s table and ratings too.' : ''}</p>
  </div>
  const parents = new Set(d.matches.filter(m => m.parent_id).map(m => m.parent_id))
  const playable = d.matches.filter(m => !parents.has(m.id))
  const called = playable.filter(m => m.status === 'called')
  const reported = playable.filter(m => m.status === 'reported')
  const next = playable.filter(m => m.status === 'ready').slice(0, 8)
  const recent = playable.filter(m => m.status === 'done' && m.outcome !== 'bye').slice(-6).reverse()
  return (
    <div className="stack loose">
      {called.length > 0 && <section className="stack"><div className="sec-head"><Icon name="play" size={16} /><h3>Playing now</h3></div>
        <div className="court-grid">{called.map(m => <div key={m.id} className="court-card"><div className="court-name">{d.courts.find(c => c.id === m.court_id)?.name || 'Court'}</div><TMatchLine d={d} m={m} onResult={onResult} onChanged={setD} readOnly={readOnly} /></div>)}</div></section>}
      {reported.length > 0 && <section className="stack"><div className="sec-head"><Icon name="clock" size={16} /><h3>Waiting to be confirmed</h3></div>
        <div className="card flush">{reported.map(m => <TMatchLine key={m.id} d={d} m={m} onResult={onResult} onChanged={setD} readOnly={readOnly} />)}</div></section>}
      {next.length > 0 && <section className="stack"><div className="sec-head"><Icon name="list" size={16} /><h3>Up next</h3></div>
        <div className="card flush">{next.map(m => <TMatchLine key={m.id} d={d} m={m} onResult={onResult} onChanged={setD} readOnly={readOnly} />)}</div></section>}
      {recent.length > 0 && <section className="stack"><div className="sec-head"><Icon name="trophy" size={16} /><h3>Just played</h3></div>
        <div className="card flush">{recent.map(m => <TMatchLine key={m.id} d={d} m={m} onResult={onResult} onChanged={setD} readOnly={readOnly} />)}</div></section>}
      {t.format === 'ranking' && <p className="muted">Enter your score cards on the Leaderboard tab.</p>}
      {t.format === 'ladder' && <p className="muted">Challenge someone above you on the Ladder tab.</p>}
      {!called.length && !next.length && !reported.length && t.status === 'live' && t.format !== 'ranking' && t.format !== 'ladder' && <p className="muted">Nothing to play right now.</p>}
    </div>
  )
}

const FORMAT_HELP: Record<string, string> = {
  americano: 'Come alone: partners change every round, so you play with everyone. Each match is a set number of points, and every point you win counts for you. Most points wins.',
  mexicano: 'Round 1 is a draw; after that the four at the top play together, the next four together, and so on — always close matches. Most points wins.',
  kotc: 'Winners move up a court, losers down, partners change. Whoever is on the top court at the end is king.',
  groups_knockout: 'Small groups where everyone plays everyone; the best of each group go into a knockout.',
  knockout: 'Lose once and you are out. The top seeds get byes when the numbers are not even.',
  double_elim: 'You are out after your second loss: lose once and you drop to the losers\' bracket, where you can still win it all.',
  round_robin: 'Everyone plays everyone; the table decides, with the sport\'s own tie-breakers.',
  swiss: 'Each round you meet someone on the same score you have not played. A few rounds, many players.',
  ranking: 'Everyone plays the same rounds and enters a score card; the leaderboard decides (with handicaps).',
  ladder: 'A ranking you climb: challenge someone a few places above you, win, and you take their place.',
  box_league: 'Boxes of a few players who all play each other over the weeks; the best go up a box, the last down.',
  arena: 'Play as many games as you can before the time is up; you always get the next opponent close to your score.',
  team_ties: 'Teams meet in ties of a few matches; win more of them and the tie is yours.',
}

function Draw({ d, setD, onResult, readOnly, reload }: { d: TDetail; setD: (d: TDetail) => void; onResult: (m: TMatch) => void; readOnly: boolean; reload: () => void }) {
  if (!d.stages.length) return <Empty icon="layers" title="No draw yet">The organisers make it when everyone has signed up.</Empty>
  return <div className="stack loose">{d.stages.map(st => <StageView key={st.id} d={d} st={st} setD={setD} onResult={onResult} readOnly={readOnly} reload={reload} />)}</div>
}

function StageView({ d, st, setD, onResult, readOnly, reload }: { d: TDetail; st: TDetail['stages'][number]; setD: (d: TDetail) => void; onResult: (m: TMatch) => void; readOnly: boolean; reload: () => void }) {
  const groups = d.groups.filter(g => g.stage_id === st.id)
  const ms = d.matches.filter(m => m.stage_id === st.id)
  const [round, setRound] = useState<number | null>(null)
  const rounds = useMemo(() => Array.from(new Set(ms.filter(m => !m.parent_id).map(m => m.round))).sort((a, b) => a - b), [ms])
  const head = d.stages.length > 1 && <div className="sec-head"><Icon name="layers" size={16} /><h3>{st.name || KIND_WORDS[st.kind]}</h3>{st.status === 'done' && <span className="pill st-closed">done</span>}</div>
  if (st.kind === 'knockout' || st.kind === 'double_elim') return <section className="stack">{head}
    {groups.map(g => <div key={g.id} className="stack tight">{groups.length > 1 && g.kind !== 'third' && g.kind !== 'final' && <h4 className="form-sec-h">{g.name}</h4>}<Bracket d={d} ms={ms.filter(m => m.group_id === g.id)} onResult={onResult} onChanged={setD} readOnly={readOnly} /></div>)}
  </section>
  if (st.kind === 'ranking') return <section className="stack">{head}<Leaderboard d={d} rows={st.leaderboard || []} mode={d.settings.mode} />{!readOnly && d.tournament.status === 'live' && <Cards d={d} st={st} setD={setD} />}</section>
  if (st.kind === 'ladder') return <section className="stack">{head}<Ladder d={d} setD={setD} readOnly={readOnly} />
    {ms.length > 0 && <div className="card flush">{[...ms].reverse().map(m => <TMatchLine key={m.id} d={d} m={m} onResult={onResult} onChanged={setD} readOnly={readOnly} />)}</div>}</section>
  const shown = round == null ? (rounds.find(r => ms.some(m => m.round === r && !['done', 'void'].includes(m.status))) ?? rounds[rounds.length - 1]) : round
  return (
    <section className="stack">{head}
      <div className={groups.length > 1 ? 'groups-grid' : ''}>{groups.map(g => <div key={g.id} className="stack tight">{groups.length > 1 && <h4 className="form-sec-h">{g.name}</h4>}<TTable d={d} rows={st.tables[String(g.id)] || []} kind={st.kind} /></div>)}</div>
      {rounds.length > 0 && <div className="stack tight">
        <div className="scroll-x"><Segmented size="sm" value={shown ?? 1} onChange={setRound} options={rounds.map(r => ({ value: r, label: st.kind === 'arena' ? `${r}` : `Round ${r}` }))} /></div>
        <div className="card flush">{ms.filter(m => m.round === shown && !m.parent_id).map(m => <React.Fragment key={m.id}>
          <TMatchLine d={d} m={m} onResult={onResult} onChanged={setD} readOnly={readOnly} />
          {ms.filter(r => r.parent_id === m.id).map(r => <div key={r.id} className="rubber"><TMatchLine d={d} m={r} onResult={onResult} onChanged={setD} readOnly={readOnly} compact /></div>)}
        </React.Fragment>)}</div>
      </div>}
      {readOnly ? null : <button type="button" className="btn ghost sm self-start no-print" onClick={reload}><Icon name="refresh" size={13} />Refresh</button>}
    </section>
  )
}

/** Score cards: your own (organisers: anyone's), hole by hole or a total. */
function Cards({ d, st, setD }: { d: TDetail; st: TDetail['stages'][number]; setD: (d: TDetail) => void }) {
  const { toast } = useToast()
  const set = d.settings
  const holesN = set.mode === 'stableford' || set.mode === 'stroke' ? (set.course?.par.length || 18) : set.mode === 'pins' ? 3 : 0
  const mineIDs = d.can_run ? d.entries.filter(e => e.status === 'in').map(e => e.id) : d.mine
  const [entry, setEntry] = useState<number>(mineIDs[0] || 0)
  const [round, setRound] = useState(1)
  const card = (st.cards || []).find(c => c.entry_id === entry && c.round === round)
  const [holes, setHoles] = useState<string[]>([])
  const [gross, setGross] = useState('')
  useEffect(() => { setHoles(Array.from({ length: holesN }, (_, i) => card?.holes?.[i] ? String(card.holes[i]) : '')); setGross(card?.gross != null ? String(card.gross) : '') }, [entry, round, holesN, card?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!mineIDs.length) return null
  const save = async () => {
    try {
      setD(await api.put<TDetail>(`${V1}/tournaments/${d.tournament.id}/scorecards`, { stage_id: st.id, round, entry_id: entry, holes: holes.map(h => Number(h) || 0), gross: holesN ? null : (gross === '' ? null : Number(gross)) }))
      toast('Card saved')
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="card pad-card stack">
      <div className="sec-head"><Icon name="edit" size={16} /><h3>Score card</h3></div>
      <div className="row wrap">
        {mineIDs.length > 1 && <Select ariaLabel="Whose card" value={String(entry)} onChange={v => setEntry(Number(v))} options={mineIDs.map(id => ({ value: String(id), label: d.entries.find(e => e.id === id)?.name || '?' }))} />}
        {set.rounds > 1 && <Segmented size="sm" value={round} onChange={setRound} options={Array.from({ length: set.rounds }, (_, i) => ({ value: i + 1, label: `Round ${i + 1}` }))} />}
      </div>
      {holesN > 0 ? <div className="holes">{holes.map((h, i) => (
        <label key={i} className="hole"><span className="muted tiny">{set.mode === 'pins' ? `Game ${i + 1}` : `${i + 1}${set.course?.par[i] ? ` · par ${set.course.par[i]}` : ''}`}</span>
          <input inputMode="numeric" value={h} onChange={e => setHoles(hs => hs.map((x, j) => j === i ? e.target.value.replace(/\D/g, '') : x))} aria-label={set.mode === 'pins' ? `Game ${i + 1}` : `Hole ${i + 1}`} /></label>
      ))}</div> : <label className="field"><span className="label">Score</span><input inputMode="decimal" value={gross} onChange={e => setGross(e.target.value)} /></label>}
      <div className="row"><button type="button" className="btn primary" onClick={save}>Save the card</button><span className="muted small">You can save it hole by hole: the leaderboard shows where everyone is.</span></div>
    </div>
  )
}

function Ladder({ d, setD, readOnly }: { d: TDetail; setD: (d: TDetail) => void; readOnly: boolean }) {
  const { toast } = useToast()
  const list = d.entries.filter(e => e.status === 'in').sort((a, b) => a.position - b.position)
  const me = list.find(e => d.mine.includes(e.id))
  const busy = new Set((d.challenges || []).filter(c => c.status === 'open' || c.status === 'accepted').flatMap(c => [c.challenger, c.defender]))
  const challenge = async (e: TEntry) => { try { setD(await api.post<TDetail>(`${V1}/tournaments/${d.tournament.id}/challenges`, { defender: e.id })); toast(`Challenge sent to ${e.name}`) } catch (er) { toast((er as Error).message, 'err') } }
  const answer = async (cid: number, accept: boolean) => { try { setD(await api.post<TDetail>(`${V1}/tournaments/${d.tournament.id}/challenges/${cid}`, { accept })) } catch (er) { toast((er as Error).message, 'err') } }
  const open = (d.challenges || []).filter(c => c.status === 'open' && me && c.defender === me.id)
  return (
    <div className="stack">
      {!readOnly && open.map(c => <div key={c.id} className="callout warn"><Icon name="target" size={15} /><span className="grow">{d.entries.find(e => e.id === c.challenger)?.name} challenges you — answer by {fmtDateTime(c.respond_by, false)}.</span>
        <button type="button" className="btn sm primary" onClick={() => answer(c.id, true)}>Accept</button><button type="button" className="btn sm" onClick={() => answer(c.id, false)}>Decline (a loss)</button></div>)}
      <div className="card flush">{list.map(e => {
        const can = !readOnly && me && e.position < me.position && me.position - e.position <= d.settings.reach && !busy.has(e.id) && !busy.has(me.id) && !(e.protected_until && d.now < e.protected_until)
        return <div key={e.id} className={`list-row ${e.id === me?.id ? 'me-row' : ''}`}>
          <span className="ladder-pos tnum">{e.position}</span>
          <span className="grow min0"><span className="row-title">{e.name}</span>{e.protected_until && d.now < e.protected_until && <span className="muted small">safe until {fmtDateTime(e.protected_until, false)}</span>}{busy.has(e.id) && <span className="muted small"> · in a challenge</span>}</span>
          {can && <button type="button" className="btn sm" onClick={() => challenge(e)}><Icon name="target" size={13} />Challenge</button>}
        </div>
      })}</div>
    </div>
  )
}

function Entries({ d, setD, readOnly }: { d: TDetail; setD: (d: TDetail) => void; readOnly: boolean }) {
  const t = d.tournament
  const { toast } = useToast()
  const [adding, setAdding] = useState(false)
  const [teams, setTeams] = useState(false)
  const put = async (e: TEntry, body: Record<string, unknown>) => { try { setD(await api.put<TDetail>(`${V1}/tournaments/${t.id}/entries/${e.id}`, body)) } catch (er) { toast((er as Error).message, 'err') } }
  const list = [...d.entries].sort((a, b) => (a.status === 'in' ? 0 : a.status === 'waiting' ? 1 : 2) - (b.status === 'in' ? 0 : b.status === 'waiting' ? 1 : 2))
  const run = d.can_run && !readOnly
  return (
    <div className="stack">
      {run && <div className="row wrap no-print">
        {(t.status === 'signup' || ['ladder', 'arena', 'ranking'].includes(t.format)) && <button type="button" className="btn" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add an entry</button>}
        {t.status === 'signup' && t.format !== 'americano' && d.entries.some(e => e.members.length === 1) && (t.entry_kind === 'team' || t.format === 'team_ties') && <button type="button" className="btn" onClick={() => setTeams(true)}><Icon name="scale" size={15} />Make fair teams</button>}
      </div>}
      {list.length === 0 ? <Empty icon="users" title="Nobody yet">Sign up above{run ? ', or add entries' : ''}.</Empty> : <div className="card flush">{list.map(e => (
        <div key={e.id} className={`list-row entry-row ${e.status !== 'in' ? 'dim' : ''}`}>
          <span className="grow min0">
            <span className="row-title">{e.name}{e.seed > 0 && <span className="badge">seed {e.seed}</span>}{e.status !== 'in' && <span className="badge">{e.status === 'waiting' ? 'waiting list' : e.status}</span>}</span>
            <span className="muted small ellipsis block-line">{e.members.length > 1 || t.entry_kind === 'team' ? e.members.map(m => d.names[String(m.user_id)] || '?').join(', ') : ''}{e.members.length === 0 ? 'guest' : ''}{e.handicap != null ? ` · handicap ${e.handicap}` : ''}{` · rating ${Math.round(e.rating)}`}</span>
          </span>
          {t.checkin && <span className={`pill ${e.checked_in ? 'st-open' : 'st-closed'}`}>{e.checked_in ? 'here' : 'not here'}</span>}
          {run && t.fee_cents > 0 && <label className="check-row small"><Switch size="sm" checked={e.paid} onChange={v => put(e, { paid: v })} label="Paid" />paid</label>}
          {run && <MenuButton label={`${e.name}: more`} align="right" width={230} items={[
            ...(t.checkin ? [{ label: e.checked_in ? 'Not here after all' : 'Check in', icon: 'check', onSelect: () => put(e, { checked_in: !e.checked_in }) }] : []),
            ...(t.status === 'signup' ? [{ label: 'Set the seed', icon: 'star', onSelect: () => { const v = window.prompt('Seed (1 = best, 0 = none)', String(e.seed)); if (v !== null) put(e, { seed: Number(v) || 0 }) } }] : []),
            { label: 'Handicap', icon: 'sliders', onSelect: () => { const v = window.prompt('Handicap (empty = none)', e.handicap != null ? String(e.handicap) : ''); if (v !== null) put(e, v.trim() === '' ? { clear_handicap: true } : { handicap: Number(v.replace(',', '.')) }) } },
            { label: 'Rename', icon: 'edit', onSelect: () => { const v = window.prompt('Name', e.name); if (v) put(e, { name: v }) } },
            ...(e.status === 'waiting' ? [{ label: 'Let them in', icon: 'check', onSelect: () => put(e, { status: 'in' }) }] : []),
            'sep',
            { label: t.status === 'signup' ? 'Remove' : 'Withdraw (walkovers)', icon: 'x-circle', danger: true, onSelect: () => put(e, { status: 'withdrawn' }) },
            ...(t.status === 'live' ? [{ label: 'Disqualify', icon: 'alert', danger: true, onSelect: () => put(e, { status: 'disqualified' }) }] : []),
          ]}><Icon name="more" size={16} /></MenuButton>}
        </div>
      ))}</div>}
      {adding && <AddEntry d={d} onClose={() => setAdding(false)} onSaved={nd => { setD(nd); setAdding(false) }} />}
      {teams && <Modal title="Make fair teams" onClose={() => setTeams(false)} size="sm"><FairTeams d={d} onDone={nd => { setD(nd); setTeams(false) }} /></Modal>}
    </div>
  )
}

function FairTeams({ d, onDone }: { d: TDetail; onDone: (d: TDetail) => void }) {
  const { toast } = useToast()
  const singles = d.entries.filter(e => e.status === 'in' && e.members.length === 1).length
  const [n, setN] = useState(Math.max(2, Math.floor(singles / Math.max(1, d.settings.ties?.length || 3))))
  return <div className="stack">
    <p className="muted">The {singles} who signed up alone are split into teams with ratings as close as possible (best and worst mixed).</p>
    <label className="field"><span className="label">Teams</span><input type="number" min={2} max={Math.max(2, singles)} value={n} onChange={e => setN(Number(e.target.value) || 2)} /></label>
    <div className="form-actions"><button type="button" className="btn primary" onClick={async () => { try { onDone(await api.post<TDetail>(`${V1}/tournaments/${d.tournament.id}/teams`, { teams: n })) } catch (e) { toast((e as Error).message, 'err') } }}>Make them</button></div>
  </div>
}

function AddEntry({ d, onClose, onSaved }: { d: TDetail; onClose: () => void; onSaved: (d: TDetail) => void }) {
  const { toast } = useToast()
  const [members, setMembers] = useState<Member[]>([])
  const [picked, setPicked] = useState<number[]>([])
  const [name, setName] = useState('')
  const [handicap, setHandicap] = useState('')
  useEffect(() => { api.get<{ members: Member[] }>(`${V1}/groups/${d.tournament.group_id}`).then(x => setMembers(x.members)).catch(() => {}) }, [d.tournament.group_id])
  const taken = new Set(d.entries.filter(e => e.status !== 'withdrawn').flatMap(e => e.members.map(m => m.user_id)))
  const want = d.tournament.entry_kind === 'pair' ? 2 : d.tournament.entry_kind === 'team' ? 0 : 1
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    try { onSaved(await api.post<TDetail>(`${V1}/tournaments/${d.tournament.id}/entries`, { members: picked, name, handicap: handicap === '' ? null : Number(handicap.replace(',', '.')) })) } catch (er) { toast((er as Error).message, 'err') }
  }
  return (
    <Modal title="Add an entry" onClose={onClose}>
      <form onSubmit={save} className="stack loose">
        <div className="team-picks">{members.filter(m => !taken.has(m.user_id)).map(m => {
          const on = picked.includes(m.user_id)
          return <button key={m.user_id} type="button" className={`team-chip ${on ? 'side-a' : ''}`} aria-pressed={on}
            onClick={() => setPicked(p => on ? p.filter(x => x !== m.user_id) : want && p.length >= want ? [...p.slice(1), m.user_id] : [...p, m.user_id])}>{m.name}</button>
        })}</div>
        <label className="field"><span className="label">{d.tournament.entry_kind === 'team' ? 'Team name' : 'Name (empty: from the people; a name alone = a guest)'}</span><input value={name} onChange={e => setName(e.target.value)} maxLength={60} /></label>
        <label className="field"><span className="label">Handicap (golf, bowling — optional)</span><input inputMode="decimal" value={handicap} onChange={e => setHandicap(e.target.value)} /></label>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={!picked.length && !name.trim()}>Add</button></div>
      </form>
    </Modal>
  )
}
