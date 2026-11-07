// Every game of every group: what is coming, and what was played.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useLiveEvents, useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { Game } from '../lib/types'
import { cap, GameCard } from '../components/Bits'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Empty, Loading, Segmented, useAsync } from '../components/ui'

export default function Games() {
  const [which, setWhich] = useState<'upcoming' | 'past'>('upcoming')
  const { data, reload } = useAsync(() => api.get<Game[]>(`${V1}/games?${which}=1&limit=${which === 'past' ? 60 : 100}`), [which])
  useOnChange(reload)
  useLiveEvents(e => { if (e.type === 'game' || e.type === 'answer') reload() })
  return (
    <div className="page stack loose">
      <div className="page-head"><h1 className="page-title grow">Games</h1>
        <Segmented size="sm" value={which} onChange={setWhich} options={[{ value: 'upcoming', label: 'Coming up' }, { value: 'past', label: 'Played' }]} />
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('game')}><Icon name="calendar-plus" size={15} />Plan a game</button>
      </div>
      {!data ? <Loading /> : data.length === 0 ? <Empty icon="calendar" title={which === 'upcoming' ? 'Nothing planned' : 'No games played yet'}>
        {which === 'upcoming' && <button type="button" className="btn primary" onClick={() => openCreate('game')}>Plan a game</button>}</Empty>
        : which === 'upcoming' ? <div className="game-list">{data.map(g => <GameCard key={g.id} game={g} />)}</div>
          : <div className="card flush">{data.map(x => (
            <Link key={x.id} to={`/games/${x.id}`} className="list-row">
              <span className="list-ico" aria-hidden="true">{x.emoji}</span>
              <span className="row-main grow"><span className="row-title">{x.group_name} · {cap(fmtDate(x.starts_at, false))}</span>
                <span className="muted small ellipsis">{x.status === 'cancelled' ? 'cancelled' : `${x.in} played · ${x.matches ? `${x.matches} ${x.matches === 1 ? 'result' : 'results'}` : 'no score yet'}`}{x.my_answer === 'in' ? ' · you played' : ''}</span></span>
              <Icon name="chevron-right" size={15} className="muted" />
            </Link>
          ))}</div>}
    </div>
  )
}
