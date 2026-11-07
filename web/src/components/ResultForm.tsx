// Enter (or fix) a result: who played on which side and the score, the way
// the group's sport counts it.
import { useEffect, useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { localTS, sideName, statInfo } from '../lib/league'
import type { GameDetail, Group, Match, Member } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Loading, Modal, Segmented, useToast } from './ui'

type Side = '' | 'a' | 'b'

export function ResultForm({ groupId, gameId, match, onClose, onSaved }: { groupId?: number; gameId?: number; match?: Match; onClose: () => void; onSaved: () => void }) {
  const { groups } = useData()
  const { user } = useApp()
  const { toast } = useToast()
  const live = (groups || []).filter(g => !g.archived)
  const [gid, setGid] = useState<number>(match?.group_id || groupId || live[0]?.id || 0)
  const [members, setMembers] = useState<Member[] | null>(null)
  const [group, setGroup] = useState<Group | null>(null)
  const [game, setGame] = useState<GameDetail | null>(null)
  const [sides, setSides] = useState<Record<number, Side>>({})
  const [scores, setScores] = useState<Record<number, string>>({})
  const [sets, setSets] = useState<[string, string][]>([['', ''], ['', '']])
  const [ab, setAB] = useState<[string, string]>(['', ''])
  const [winner, setWinner] = useState<'' | 'a' | 'b' | 'draw'>('')
  const [playedAt, setPlayedAt] = useState(match?.played_at || localTS(new Date()))
  const [note, setNote] = useState(match?.note || '')
  const [scorers, setScorers] = useState(false)
  const [stats, setStats] = useState<Record<number, Record<string, string>>>({})
  const [showStats, setShowStats] = useState(false)
  const [busy, setBusy] = useState(false)
  const gameID = match?.game_id || gameId || 0

  useEffect(() => {
    if (!gid) return
    api.get<{ group: Group; members: Member[] }>(`${V1}/groups/${gid}`).then(d => { setGroup(d.group); setMembers(d.members) }).catch(e => toast((e as Error).message, 'err'))
  }, [gid]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!gameID) return
    api.get<GameDetail>(`${V1}/games/${gameID}`).then(d => { setGame(d); if (!match && d.game.starts_at) setPlayedAt(d.game.starts_at) }).catch(() => {})
  }, [gameID]) // eslint-disable-line react-hooks/exhaustive-deps
  // fill from the match being fixed, or from the game's teams of the day
  useEffect(() => {
    if (!group || !members) return
    if (match) {
      const s: Record<number, Side> = {}, sc: Record<number, string> = {}
      const st: Record<number, Record<string, string>> = {}
      for (const p of match.players) { s[p.user_id] = p.side || 'a'; if (p.score != null) sc[p.user_id] = String(p.score); if (p.stats) st[p.user_id] = Object.fromEntries(Object.entries(p.stats).map(([k, v]) => [k, String(v)])) }
      setSides(s); setScores(sc); setStats(st); if (Object.keys(st).length) setShowStats(true)
      if (match.score.sets) setSets(match.score.sets.map(x => [String(x[0]), String(x[1])]))
      if (match.score.a != null) setAB([String(match.score.a), String(match.score.b ?? '')])
      setWinner(match.winner)
      return
    }
    if (game && group.format === 'teams') {
      const s: Record<number, Side> = {}
      for (const p of game.people) if (p.answer === 'in' && p.side) s[p.user_id] = p.side
      if (Object.keys(s).length) { setSides(s); return }
    }
    if (game && group.scoring === 'ranking') {
      const s: Record<number, Side> = {}
      for (const p of game.people) if (p.answer === 'in') s[p.user_id] = 'a'
      setSides(s)
    }
  }, [group, members, game, match])

  const inGame = useMemo(() => new Set((game?.people || []).filter(p => p.answer === 'in').map(p => p.user_id)), [game])
  if (!group || !members) return <Modal title="Enter a result" onClose={onClose}><Loading /></Modal>
  const ranking = group.scoring === 'ranking'
  const per = group.format === 'singles' ? 1 : group.format === 'doubles' ? 2 : 0
  const order = [...members].sort((a, b) => Number(inGame.has(b.user_id)) - Number(inGame.has(a.user_id)) || a.name.localeCompare(b.name))
  const on = (side: Side) => Object.entries(sides).filter(([, s]) => s === side).map(([id]) => Number(id))
  const names = Object.fromEntries(members.map(m => [String(m.user_id), m.name]))
  const sideA = on('a'), sideB = on('b')

  const pickSlot = (side: 'a' | 'b', i: number, uid: number) => {
    const cur = on(side)
    const next = { ...sides }
    if (cur[i]) delete next[cur[i]]
    if (uid) next[uid] = side
    setSides(next)
  }
  const cycle = (uid: number) => setSides(s => ({ ...s, [uid]: ranking ? (s[uid] ? '' : 'a') : s[uid] === 'a' ? 'b' : s[uid] === 'b' ? '' : 'a' }))

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    const players = Object.entries(sides).filter(([, s]) => s).map(([id, s]) => {
      const v = scores[Number(id)]
      const st = Object.fromEntries(Object.entries(stats[Number(id)] || {}).filter(([, x]) => Number(x) > 0).map(([k, x]) => [k, Number(x)]))
      return { user_id: Number(id), side: ranking ? '' : s, score: v !== undefined && v !== '' ? Number(v.replace(',', '.')) : null, place: 0, stats: st }
    })
    const score: Record<string, unknown> = {}
    if (group.scoring === 'sets') score.sets = sets.filter(x => x[0] !== '' || x[1] !== '').map(x => [Number(x[0]) || 0, Number(x[1]) || 0])
    if (group.scoring === 'goals' || group.scoring === 'points') { score.a = Number(ab[0]) || 0; score.b = Number(ab[1]) || 0 }
    setBusy(true)
    try {
      const body = { game_id: gameID, played_at: playedAt, players, score, winner, note }
      if (match) await api.put(`${V1}/matches/${match.id}`, body)
      else await api.post(`${V1}/groups/${gid}/matches`, body)
      announceChange(); toast(match ? 'Result fixed' : 'Result saved — the others can confirm it'); onSaved()
    } catch (err) { toast((err as Error).message, 'err') } finally { setBusy(false) }
  }

  const slotSelect = (side: 'a' | 'b', i: number) => {
    const cur = on(side)[i] || 0
    const taken = new Set(Object.entries(sides).filter(([, s]) => s).map(([id]) => Number(id)))
    return <Select key={`${side}${i}`} ariaLabel={`Side ${side.toUpperCase()} player ${i + 1}`} value={cur ? String(cur) : ''} placeholder="Who?" clearLabel="Nobody"
      onChange={v => pickSlot(side, i, Number(v) || 0)}
      options={order.filter(m => !taken.has(m.user_id) || m.user_id === cur).map(m => ({ value: String(m.user_id), label: m.name, hint: inGame.has(m.user_id) ? 'was in' : m.user_id === user?.id ? 'you' : undefined }))} />
  }

  return (
    <Modal title={match ? 'Fix the result' : 'Enter a result'} onClose={onClose} size="lg">
      <form onSubmit={submit} className="stack loose">
        {!match && !gameId && live.length > 1 && <Field label="Group"><Select ariaLabel="Group" value={String(gid)} onChange={v => { setGid(Number(v)); setSides({}) }} options={live.map(x => ({ value: String(x.id), label: `${x.sport_info?.emoji || ''} ${x.name}` }))} /></Field>}
        {!gameID && <Field label="Played"><input type="datetime-local" value={playedAt} onChange={e => setPlayedAt(e.target.value)} required /></Field>}

        {per > 0 ? (
          <div className="sides">
            <div className="side-col"><h4 className="form-sec-h">Side A</h4>{Array.from({ length: per }, (_, i) => slotSelect('a', i))}</div>
            <div className="side-vs" aria-hidden="true">vs</div>
            <div className="side-col"><h4 className="form-sec-h">Side B</h4>{Array.from({ length: per }, (_, i) => slotSelect('b', i))}</div>
          </div>
        ) : (
          <div className="form-sec">
            <h4 className="form-sec-h">{ranking ? 'Who played (tap to add)' : 'Teams: tap a name for A, again for B, again to leave out'}</h4>
            <div className="team-picks">
              {order.map(m => {
                const s = sides[m.user_id] || ''
                return <button key={m.user_id} type="button" className={`team-chip side-${s || 'none'}`} onClick={() => cycle(m.user_id)} aria-pressed={!!s}>
                  {!ranking && s && <b>{s.toUpperCase()}</b>}{m.name}{inGame.has(m.user_id) && !s && <span className="muted small"> · was in</span>}
                </button>
              })}
            </div>
            {!ranking && <p className="muted small">A: {sideA.length} · B: {sideB.length}</p>}
          </div>
        )}

        {group.scoring === 'sets' && <div className="form-sec">
          <h4 className="form-sec-h">Score ({sideName(names, sideA, group.format) || 'A'} – {sideName(names, sideB, group.format) || 'B'})</h4>
          <div className="sets">
            {sets.map((x, i) => (
              <div key={i} className="set-in">
                <span className="muted small">Set {i + 1}</span>
                <input inputMode="numeric" value={x[0]} onChange={e => setSets(ss => ss.map((y, j) => j === i ? [e.target.value.replace(/\D/g, ''), y[1]] : y))} aria-label={`Set ${i + 1}, side A`} />
                <span>–</span>
                <input inputMode="numeric" value={x[1]} onChange={e => setSets(ss => ss.map((y, j) => j === i ? [y[0], e.target.value.replace(/\D/g, '')] : y))} aria-label={`Set ${i + 1}, side B`} />
              </div>
            ))}
          </div>
          <div className="row">
            <button type="button" className="btn sm" disabled={sets.length >= 5} onClick={() => setSets(s => [...s, ['', '']])}><Icon name="plus" size={14} />A set</button>
            {sets.length > 1 && <button type="button" className="btn sm ghost" onClick={() => setSets(s => s.slice(0, -1))}>Remove the last</button>}
          </div>
        </div>}
        {(group.scoring === 'goals' || group.scoring === 'points') && <div className="form-sec">
          <h4 className="form-sec-h">{group.scoring === 'goals' ? 'Goals' : 'Points'}</h4>
          <div className="ab-in">
            <label><span className="muted small">{sideName(names, sideA, group.format) || 'A'}</span><input inputMode="numeric" value={ab[0]} onChange={e => setAB([e.target.value.replace(/\D/g, ''), ab[1]])} aria-label="Side A" required /></label>
            <span className="ab-dash">–</span>
            <label><span className="muted small">{sideName(names, sideB, group.format) || 'B'}</span><input inputMode="numeric" value={ab[1]} onChange={e => setAB([ab[0], e.target.value.replace(/\D/g, '')])} aria-label="Side B" required /></label>
          </div>
          {group.format === 'teams' && <button type="button" className="linkish self-start" onClick={() => setScorers(s => !s)}>{scorers ? 'Hide' : 'Add'} who scored</button>}
          {scorers && <div className="scorers">{[...sideA, ...sideB].map(id => (
            <label key={id} className="scorer"><span className="ellipsis">{names[String(id)]}</span>
              <input inputMode="numeric" value={scores[id] || ''} onChange={e => setScores(s => ({ ...s, [id]: e.target.value.replace(/\D/g, '') }))} aria-label={`${names[String(id)]}'s goals`} placeholder="0" /></label>
          ))}</div>}
        </div>}
        {group.scoring === 'result' && <Field label="Who won" group>
          <Segmented value={winner} onChange={setWinner} options={[{ value: 'a', label: sideName(names, sideA, group.format) || 'Side A' }, { value: 'draw', label: 'Draw' }, { value: 'b', label: sideName(names, sideB, group.format) || 'Side B' }]} />
        </Field>}
        {ranking && <div className="form-sec">
          <h4 className="form-sec-h">Scores ({group.lower_better ? 'the lowest wins' : 'the highest wins'})</h4>
          <div className="scorers">{on('a').map(id => (
            <label key={id} className="scorer"><span className="ellipsis">{names[String(id)]}</span>
              <input inputMode="decimal" value={scores[id] || ''} onChange={e => setScores(s => ({ ...s, [id]: e.target.value }))} aria-label={`${names[String(id)]}'s score`} required /></label>
          ))}</div>
        </div>}
        {(group.stats || []).length > 0 && [...sideA, ...sideB].length > 0 && <div className="form-sec">
          <button type="button" className="linkish self-start" onClick={() => setShowStats(x => !x)} aria-expanded={showStats}><Icon name={showStats ? 'chevron-up' : 'chevron-down'} size={14} /> {showStats ? 'Hide' : 'Add'} {group.stats.map(k => statInfo(k).name.toLowerCase()).join(', ')}</button>
          {showStats && <div className="scroll-x"><table className="table stat-in">
            <thead><tr><th>Player</th>{group.stats.map(k => <th key={k} className="num" title={statInfo(k).name}>{statInfo(k).icon}<span className="hide-sm"> {statInfo(k).name}</span></th>)}</tr></thead>
            <tbody>{[...sideA, ...sideB].map(id => (
              <tr key={id}><td className="ellipsis">{names[String(id)]}</td>{group.stats.map(k => (
                <td key={k} className="num"><input inputMode="numeric" value={stats[id]?.[k] || ''} placeholder="0" aria-label={`${names[String(id)]}: ${statInfo(k).name}`}
                  onChange={e => setStats(s => ({ ...s, [id]: { ...(s[id] || {}), [k]: e.target.value.replace(/\D/g, '').slice(0, 3) } }))} /></td>
              ))}</tr>
            ))}</tbody>
          </table></div>}
        </div>}
        <Field label="Note (optional)"><input value={note} onChange={e => setNote(e.target.value)} placeholder="Tie-break in the third, great rally at 5–5" maxLength={500} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{match ? 'Save' : 'Save the result'}</button></div>
      </form>
    </Modal>
  )
}
