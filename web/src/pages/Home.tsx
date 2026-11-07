// Home: what needs you (answers, votes, results to confirm), the next games,
// your last results and your groups.
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData, useLiveEvents, useOnChange } from '../lib/data'
import { when } from '../lib/league'
import type { Today } from '../lib/types'
import { GameCard, MatchRow } from '../components/Bits'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Empty, Loading, useAsync } from '../components/ui'

export default function Home() {
  const { user } = useApp()
  const { groups } = useData()
  const { data, reload } = useAsync(() => api.get<Today>(`${V1}/today`), [])
  useOnChange(reload)
  useLiveEvents(e => { if (e.type === 'game' || e.type === 'match' || e.type === 'members') reload() })
  const player = user?.role === 'player'
  const live = (groups || []).filter(g => !g.archived)
  if (!data) return <Loading />
  const first = (user?.name || '').split(' ')[0]
  if (live.length === 0) return (
    <div className="page stack loose">
      <div className="page-head"><div><h1 className="page-title">{first ? `Hi ${first}` : 'Welcome'}</h1></div></div>
      <Empty icon="ball" title={player ? 'No groups yet' : 'Start your first group'}>
        {player ? <p>When someone adds you to a group, it shows up here with its games and chat.</p> : <>
          <p>A group is a few people who play one sport together: tennis on Tuesdays, five-a-side on Fridays. Plan the games, see who's in, enter the scores and watch the table.</p>
          <button type="button" className="btn primary" onClick={() => openCreate('group')}><Icon name="plus" size={15} />New group</button>
        </>}
      </Empty>
    </div>
  )
  const next = data.next.filter(g => g.my_answer !== '')
  return (
    <div className="page stack loose">
      <div className="page-head"><div className="grow"><h1 className="page-title">{first ? `Hi ${first}` : 'Home'}</h1>
        <p className="muted">{data.next[0] ? <>Next: <b>{data.next[0].group_name}</b>, {when(data.next[0].starts_at)}</> : 'Nothing planned yet.'}</p></div></div>

      {data.unanswered.length > 0 && <section className="stack">
        <div className="sec-head"><Icon name="help" size={16} /><h3>Are you in?</h3><span className="count">{data.unanswered.length}</span></div>
        <div className="game-list">{data.unanswered.map(g => <GameCard key={g.id} game={g} />)}</div>
      </section>}

      {data.voting.length > 0 && <section className="stack">
        <div className="sec-head"><Icon name="calendar" size={16} /><h3>When can you play?</h3></div>
        <div className="game-list">{data.voting.map(g => <GameCard key={g.id} game={g} />)}</div>
      </section>}

      {data.confirm.length > 0 && <section className="stack">
        <div className="sec-head"><Icon name="trophy" size={16} /><h3>Results to check</h3><span className="count">{data.confirm.length}</span></div>
        <div className="card flush">{data.confirm.map(r => <MatchRow key={r.match.id} m={r.match} format="singles" group={r.group_name} emoji={r.emoji} onChanged={reload} />)}</div>
      </section>}

      <section className="stack">
        <div className="sec-head"><Icon name="calendar-check" size={16} /><h3>Coming up</h3><span className="spacer" /><Link to="/games" className="small">All games</Link></div>
        {next.length ? <div className="game-list">{next.map(g => <GameCard key={g.id} game={g} />)}</div>
          : <p className="muted">{data.unanswered.length ? 'Answer the games above to see them here.' : <>No games planned. <button type="button" className="linkish" onClick={() => openCreate('game')}>Plan one</button>.</>}</p>}
      </section>

      <section className="stack">
        <div className="sec-head"><Icon name="users" size={16} /><h3>Your groups</h3></div>
        <div className="group-grid">{live.map(g => (
          <Link key={g.id} to={`/groups/${g.id}`} className="group-tile" style={{ ['--grp' as string]: g.color || 'var(--accent)' }}>
            <span className="group-tile-emoji" aria-hidden="true">{g.sport_info?.emoji}</span>
            <span className="grow min0"><span className="group-tile-name ellipsis">{g.name}</span>
              <span className="muted small ellipsis block-line">{g.next_game ? when(g.next_game.starts_at) : g.sport_label} · {g.members} people</span></span>
            {g.unread > 0 && <span className="sb-count" title="New in the chat">{g.unread}</span>}
          </Link>
        ))}</div>
      </section>

      {data.recent.length > 0 && <section className="stack">
        <div className="sec-head"><Icon name="activity" size={16} /><h3>Your last results</h3><span className="spacer" /><Link to="/results" className="small">All results</Link></div>
        <div className="card flush">{data.recent.map(r => (
          <div key={r.match.id} className="recent-row">
            <span className={`fd fd-${r.won || 'D'}`}>{r.won || '·'}</span>
            <span aria-hidden="true">{r.emoji}</span>
            <span className="grow min0 ellipsis">{r.line}</span>
            <span className="muted small">{r.group_name}</span>
          </div>
        ))}</div>
      </section>}
    </div>
  )
}
