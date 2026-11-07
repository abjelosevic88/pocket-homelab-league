// One person in a group: their line in the table, their rating over time,
// how they do against (and with) everyone, and their last matches.
import { Link, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { fmtDate } from '../lib/format'
import type { Group, Match, Row, Versus } from '../lib/types'
import { FormDots, MatchRow } from '../components/Bits'
import { Icon } from '../components/icons'
import { useCrumb } from '../components/Layout'
import { Avatar, Empty, Loading, useAsync } from '../components/ui'

interface PlayerView { user_id: number; name: string; row: Row | null; history: { at: string; rating: number }[]; recent: Match[]; versus: Versus[]; group: Group }

export default function Player() {
  const { id, uid } = useParams()
  const { data: p, error } = useAsync(() => api.get<PlayerView>(`${V1}/groups/${id}/players/${uid}`), [id, uid])
  useCrumb(p ? `${p.group.name} · ${p.name}` : undefined)
  if (error) return <Empty icon="alert" title="Not found"><Link to={`/groups/${id}`}>Back to the group</Link></Empty>
  if (!p) return <Loading />
  const r = p.row
  const g = p.group
  const pct = r && r.played ? Math.round((r.won / r.played) * 100) : 0
  return (
    <div className="page stack loose">
      <header className="player-head">
        <Avatar name={p.name} size={56} />
        <div className="grow min0">
          <h1 className="page-title">{p.name}</h1>
          <p className="muted small"><Link to={`/groups/${g.id}?tab=table`}>{g.sport_info?.emoji} {g.name}</Link>{r?.position ? ` · #${r.position} in the table` : ''}</p>
        </div>
      </header>
      {!r || r.played === 0 ? <Empty icon="trophy" title="No matches yet">Results show up here once they have played.</Empty> : <>
        <div className="stats-row">
          <div className="stat"><span className="stat-l">Rating</span><span className="stat-v tnum">{r.rating}</span><span className="stat-s small">{r.change ? `${r.change > 0 ? '+' : ''}${Math.round(r.change)} last match` : 'start 1000'}</span></div>
          <div className="stat"><span className="stat-l">Played</span><span className="stat-v tnum">{r.played}</span><span className="stat-s small">{r.won} won · {r.lost} lost{r.drawn ? ` · ${r.drawn} drawn` : ''}</span></div>
          <div className="stat"><span className="stat-l">Win rate</span><span className="stat-v tnum">{pct}%</span><span className="stat-s small">{r.points} points</span></div>
          <div className="stat"><span className="stat-l">Form</span><span className="stat-v"><FormDots form={r.form} /></span><span className="stat-s small">{streakWords(r.streak)}</span></div>
          {r.scored > 0 && <div className="stat"><span className="stat-l">Scored</span><span className="stat-v tnum">{r.scored}</span><span className="stat-s small">{(r.scored / r.played).toFixed(1)} a game</span></div>}
          {r.mvp > 0 && <div className="stat"><span className="stat-l">Player of the game</span><span className="stat-v tnum">★ {r.mvp}</span></div>}
        </div>
        {p.history.length > 1 && <section className="card pad-card stack">
          <div className="sec-head"><Icon name="trending-up" size={16} /><h3>Rating over time</h3></div>
          <RatingLine points={p.history} />
        </section>}
        {p.versus.length > 0 && <section className="stack">
          <div className="sec-head"><Icon name="users" size={16} /><h3>Against and with</h3></div>
          <div className="card flush scroll-x"><table className="table compact">
            <thead><tr><th>Who</th><th className="num">Against</th><th className="num">Won</th><th className="num">Lost</th>{g.format !== 'singles' && <th className="num">Together</th>}<th className="hide-sm">Last</th></tr></thead>
            <tbody>{p.versus.map(v => (
              <tr key={v.user_id}><td><Link to={`/groups/${g.id}/players/${v.user_id}`}>{v.name}</Link></td><td className="num">{v.played}</td><td className="num">{v.won}</td><td className="num">{v.lost}{v.drawn ? <span className="muted"> ({v.drawn}=)</span> : ''}</td>
                {g.format !== 'singles' && <td className="num">{v.together ? `${v.together_won}/${v.together}` : ''}</td>}<td className="hide-sm muted">{v.last_played_at ? fmtDate(v.last_played_at, false) : ''}</td></tr>
            ))}</tbody>
          </table></div>
        </section>}
        <section className="stack">
          <div className="sec-head"><Icon name="trophy" size={16} /><h3>Last matches</h3></div>
          <div className="card flush">{p.recent.map(m => (
            <div key={m.id} className="with-change"><MatchRow m={m} format={g.format} />{m.changes?.[String(p.user_id)] !== undefined && <span className={`rt-ch ${m.changes[String(p.user_id)] > 0 ? 'up' : 'down'}`}>{m.changes[String(p.user_id)] > 0 ? '+' : ''}{Math.round(m.changes[String(p.user_id)])}</span>}</div>
          ))}</div>
        </section>
      </>}
    </div>
  )
}

function streakWords(s: string): string {
  if (!s) return ''
  const n = Number(s.slice(1))
  return s[0] === 'W' ? `${n} ${n === 1 ? 'win' : 'wins'} in a row` : s[0] === 'L' ? `${n} ${n === 1 ? 'loss' : 'losses'} in a row` : `${n} ${n === 1 ? 'draw' : 'draws'} in a row`
}

/** A small line of the rating over time (an SVG, no library). */
function RatingLine({ points }: { points: { at: string; rating: number }[] }) {
  const w = 600, h = 120, pad = 18
  const vals = points.map(p => p.rating)
  const lo = Math.min(...vals, 1000) - 10, hi = Math.max(...vals, 1000) + 10
  const x = (i: number) => pad + (i / Math.max(points.length - 1, 1)) * (w - 2 * pad)
  const y = (v: number) => h - pad - ((v - lo) / (hi - lo || 1)) * (h - 2 * pad)
  const d = points.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.rating).toFixed(1)}`).join(' ')
  const last = points[points.length - 1]
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="rating-line" role="img" aria-label={`Rating from ${points[0].rating} to ${last.rating}`}>
      <line x1={pad} x2={w - pad} y1={y(1000)} y2={y(1000)} className="rl-base" />
      <text x={w - pad} y={y(1000) - 4} className="rl-lbl" textAnchor="end">1000</text>
      <path d={d} className="rl-path" />
      <circle cx={x(points.length - 1)} cy={y(last.rating)} r={4} className="rl-dot" />
      <text x={x(points.length - 1) - 8} y={y(last.rating) - 8} className="rl-lbl strong" textAnchor="end">{last.rating}</text>
    </svg>
  )
}
