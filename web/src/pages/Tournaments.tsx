// Every tournament of every group: being played, sign-up open, done.
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useLiveEvents, useOnChange } from '../lib/data'
import { when } from '../lib/league'
import { STATUS_WORDS, type Tournament } from '../lib/tour'
import { Icon } from '../components/icons'
import { Empty, Loading, useAsync } from '../components/ui'

export function TournamentList({ list, showGroup = true }: { list: Tournament[]; showGroup?: boolean }) {
  return <div className="card flush">{list.map(t => (
    <Link key={t.id} to={`/tournaments/${t.id}`} className="list-row">
      <span className="list-ico" aria-hidden="true">{t.status === 'finished' ? '🏆' : t.emoji}</span>
      <span className="row-main grow">
        <span className="row-title">{t.name}{t.mine && t.status !== 'finished' && <span className="badge accent">you're in</span>}</span>
        <span className="muted small ellipsis">{[showGroup ? t.group_name : '', t.format_name, t.status === 'finished' && t.winner ? `won by ${t.winner}` : `${t.entries} ${t.entry_kind === 'team' ? 'teams' : t.entry_kind === 'pair' ? 'pairs' : 'players'}`, t.starts_at ? when(t.starts_at) : ''].filter(Boolean).join(' · ')}</span>
      </span>
      <span className={`pill ${t.status === 'live' ? 'st-open' : t.status === 'signup' ? 'st-new' : 'st-closed'}`}>{STATUS_WORDS[t.status]}</span>
    </Link>
  ))}</div>
}

export default function Tournaments() {
  const { data, reload } = useAsync(() => api.get<Tournament[]>(`${V1}/tournaments`), [])
  useOnChange(reload)
  useLiveEvents(e => { if (e.type === 'tournament') reload() })
  return (
    <div className="page stack loose">
      <div className="page-head"><h1 className="page-title grow">Tournaments</h1>
        <button type="button" className="btn primary hide-sm" onClick={() => window.dispatchEvent(new CustomEvent('plg:create', { detail: { kind: 'tournament' } }))}><Icon name="trophy" size={15} />New tournament</button></div>
      {!data ? <Loading /> : data.length === 0 ? <Empty icon="trophy" title="No tournaments yet">
        <p>A one-day knockout, an Americano night, a club championship, a ladder for the season — for any sport in your groups.</p>
        <button type="button" className="btn primary" onClick={() => window.dispatchEvent(new CustomEvent('plg:create', { detail: { kind: 'tournament' } }))}>New tournament</button>
      </Empty> : <TournamentList list={data} />}
    </div>
  )
}
