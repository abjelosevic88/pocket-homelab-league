// "Can you fill in?" — places offered to you, first to say yes plays.
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useLiveEvents } from '../lib/data'
import { when } from '../lib/league'
import type { SubOffer } from '../lib/types'
import { Icon } from './icons'
import { useAsync, useToast } from './ui'

export function SubOffers() {
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data, reload } = useAsync(() => api.get<SubOffer[]>(`${V1}/subs`), [])
  useLiveEvents(e => { if (e.type === 'game' || e.type === 'answer') reload() })
  if (!data?.length) return null
  const answer = async (o: SubOffer, no: boolean) => {
    try {
      await api.post(`${V1}/subs/${o.id}`, { no })
      announceChange()
      if (!no) { toast(`You're in: ${o.game.group_name}`); navigate(`/games/${o.game.id}`) } else reload()
    } catch (e) { toast((e as Error).message, 'err'); reload() }
  }
  return (
    <section className="stack">
      <div className="sec-head"><Icon name="search" size={16} /><h3>Can you fill in?</h3><span className="count">{data.length}</span></div>
      <div className="card flush">{data.map(o => (
        <div key={o.id} className="sub-row">
          <span className="sub-emoji" aria-hidden="true">{o.game.emoji}</span>
          <span className="grow min0">
            <span className="block-line"><b>{o.game.group_name}</b> · {when(o.game.starts_at)}</span>
            <span className="muted small block-line">{o.game.sport}{o.game.venue ? ` at ${o.game.venue}` : ''} · {o.game.in} in{o.game.max_players ? ` of ${o.game.max_players}` : ''} · asked by {o.offered_by}{o.in_group ? '' : ' (you would join the group)'}</span>
          </span>
          <div className="row">
            <button type="button" className="btn primary sm" onClick={() => answer(o, false)}>I can play</button>
            <button type="button" className="btn ghost sm" onClick={() => answer(o, true)}>Not this time</button>
          </div>
        </div>
      ))}</div>
    </section>
  )
}
