// What a tournament page is made of: a match line, the result form, a
// bracket, a table, a leaderboard.
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtDateTime } from '../lib/format'
import { matchEntries, sideName, tScore, type TDetail, type TLeader, type TMatch, type TRow } from '../lib/tour'
import { Icon } from './icons'
import { Field, Modal, Segmented, useToast } from './ui'

/** One match: the sides, the score, where and its state; the buttons fit the viewer. */
export function TMatchLine({ d, m, onResult, onChanged, compact = false, readOnly = false }: { d: TDetail; m: TMatch; onResult?: (m: TMatch) => void; onChanged?: (d: TDetail) => void; compact?: boolean; readOnly?: boolean }) {
  const { toast } = useToast()
  const mine = matchEntries(m).some(e => d.mine.includes(e))
  const court = d.courts.find(c => c.id === m.court_id)
  const canEnter = !readOnly && d.tournament.status === 'live' && (mine || d.can_run) && ['ready', 'called', 'reported'].includes(m.status) && (m.a_entry || m.sides?.a?.length) && (m.b_entry || m.sides?.b?.length)
  const myEntered = m.reported_by && d.entries.some(e => d.mine.includes(e.id) && e.members.some(x => x.user_id === m.reported_by))
  const canConfirm = !readOnly && m.status === 'reported' && (d.can_run || (mine && !myEntered))
  const confirm = async (wrong: boolean) => {
    const reason = wrong ? window.prompt('What is not right?') : ''
    if (wrong && reason === null) return
    try { onChanged?.(await api.post<TDetail>(`${V1}/tmatches/${m.id}/confirm`, { wrong, reason })) } catch (e) { toast((e as Error).message, 'err') }
  }
  const a = sideName(d, m, 'a'), b = sideName(d, m, 'b')
  const score = tScore(m)
  if (m.outcome === 'bye') return <div className={`tm done bye ${mine ? 'mine' : ''} ${compact ? 'compact' : ''}`}><div className="tm-main"><span className="tm-side won">{m.winner === 'b' ? b : a}</span> <span className="muted small">· bye</span></div></div>
  return (
    <div className={`tm ${m.status} ${mine ? 'mine' : ''} ${compact ? 'compact' : ''}`}>
      <div className="tm-main">
        <div className="tm-sides">
          <span className={`tm-side ${m.winner === 'a' ? 'won' : ''}`}>{m.white === 'a' && <span className="chess-w" title="White">♔</span>}{a}</span>
          <span className="tm-score tnum">{score || 'vs'}</span>
          <span className={`tm-side right ${m.winner === 'b' ? 'won' : ''}`}>{b}{m.white === 'b' && <span className="chess-w" title="White">♔</span>}</span>
        </div>
        <div className="tm-sub muted small">
          {m.label && !compact && <span>{m.label}</span>}
          {court && m.status === 'called' && <span className="pill st-open"><Icon name="pin" size={11} />{court.name} now</span>}
          {court && m.status !== 'called' && m.status !== 'done' && m.scheduled_at && <span>{court.name} · {fmtDateTime(m.scheduled_at, false)}</span>}
          {m.status === 'reported' && <span className="pill st-due"><Icon name="clock" size={11} />to confirm</span>}
          {m.dispute && <span className="pill st-overdue" title={m.dispute}><Icon name="alert" size={11} />{m.dispute}</span>}
          {m.deadline && m.status !== 'done' && <span>play by {fmtDateTime(m.deadline, false)}</span>}
          {m.outcome === 'retired' && <span>retired</span>}
        </div>
      </div>
      {(canEnter || canConfirm) && <div className="tm-actions">
        {canConfirm && <><button type="button" className="btn sm primary" onClick={() => confirm(false)}><Icon name="check" size={13} />Right</button><button type="button" className="btn sm" onClick={() => confirm(true)}>Not right</button></>}
        {canEnter && !canConfirm && <button type="button" className={`btn sm ${mine ? 'primary' : ''}`} onClick={() => onResult?.(m)}>{m.status === 'reported' ? 'Change' : 'Enter the score'}</button>}
      </div>}
      {!readOnly && d.can_run && m.status === 'done' && m.outcome !== 'bye' && <button type="button" className="btn ghost icon-only sm tm-undo" title="Undo this result" aria-label="Undo this result"
        onClick={async () => { try { onChanged?.(await api.post<TDetail>(`${V1}/tmatches/${m.id}/reopen`)) } catch (e) { toast((e as Error).message, 'err') } }}><Icon name="undo" size={13} /></button>}
    </div>
  )
}

/** Enter a tournament match's result, the way the stage and the sport count it. */
export function TResultForm({ d, m, onClose, onSaved }: { d: TDetail; m: TMatch; onClose: () => void; onSaved: (d: TDetail) => void }) {
  const { toast } = useToast()
  const stage = d.stages.find(s => s.id === m.stage_id)
  const rotating = ['americano', 'mexicano', 'kotc'].includes(stage?.kind || '')
  const scoring = d.group.scoring
  const draws = ['round_robin', 'swiss', 'arena', 'team_ties'].includes(stage?.kind || '')
  const [outcome, setOutcome] = useState('normal')
  const [winner, setWinner] = useState<'' | 'a' | 'b' | 'draw'>(m.winner || '')
  const [sets, setSets] = useState<[string, string][]>(m.score?.sets?.length ? m.score.sets.map(s => [String(s[0]), String(s[1])]) : [['', ''], ['', '']])
  const [ab, setAB] = useState<[string, string]>([m.score?.a != null ? String(m.score.a) : m.a_total != null ? String(m.a_total) : '', m.score?.b != null ? String(m.score.b) : m.b_total != null ? String(m.b_total) : ''])
  const [busy, setBusy] = useState(false)
  const a = sideName(d, m, 'a'), b = sideName(d, m, 'b')
  const target = d.settings.points
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    const body: Record<string, unknown> = { outcome }
    if (outcome !== 'normal' && outcome !== 'retired') body.winner = winner
    else if (rotating) { body.a_total = Number(ab[0]) || 0; body.b_total = Number(ab[1]) || 0 }
    else if (scoring === 'sets') { body.score = { sets: sets.filter(s => s[0] !== '' || s[1] !== '').map(s => [Number(s[0]) || 0, Number(s[1]) || 0]) }; if (outcome === 'retired') body.winner = winner }
    else if (scoring === 'goals' || scoring === 'points') body.score = { a: Number(ab[0]) || 0, b: Number(ab[1]) || 0 }
    else body.winner = winner
    try { onSaved(await api.post<TDetail>(`${V1}/tmatches/${m.id}/result`, body)); toast(d.can_run ? 'Result saved' : 'Result saved — the other side confirms it') } catch (err) { toast((err as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={m.label || 'The result'} onClose={onClose}>
      <form onSubmit={save} className="stack loose">
        <p className="tm-head"><b>{a}</b> <span className="muted">vs</span> <b>{b}</b></p>
        <Segmented size="sm" value={outcome} onChange={setOutcome} options={[{ value: 'normal', label: 'Played' }, { value: 'retired', label: 'Retired' }, { value: 'walkover', label: 'Walkover' }, { value: 'no_show', label: 'No-show' }, { value: 'double_walkover', label: 'Neither came' }]} />
        {outcome === 'normal' && rotating && <div className="ab-in">
          <label><span className="muted small">{a}</span><input inputMode="numeric" value={ab[0]} onChange={e => { const v = e.target.value.replace(/\D/g, ''); setAB([v, target && v ? String(Math.max(0, target - Number(v))) : ab[1]]) }} required /></label>
          <span className="ab-dash">–</span>
          <label><span className="muted small">{b}</span><input inputMode="numeric" value={ab[1]} onChange={e => setAB([ab[0], e.target.value.replace(/\D/g, '')])} required /></label>
        </div>}
        {outcome === 'normal' && rotating && target > 0 && <p className="muted small">A match is {target} points: the other side's points fill in by themselves.</p>}
        {(outcome === 'normal' || outcome === 'retired') && !rotating && scoring === 'sets' && <div className="form-sec">
          <div className="sets">{sets.map((x, i) => (
            <div key={i} className="set-in"><span className="muted small">Set {i + 1}</span>
              <input inputMode="numeric" value={x[0]} onChange={e => setSets(ss => ss.map((y, j) => j === i ? [e.target.value.replace(/\D/g, ''), y[1]] : y))} aria-label={`Set ${i + 1}, ${a}`} />
              <span>–</span>
              <input inputMode="numeric" value={x[1]} onChange={e => setSets(ss => ss.map((y, j) => j === i ? [y[0], e.target.value.replace(/\D/g, '')] : y))} aria-label={`Set ${i + 1}, ${b}`} />
            </div>))}</div>
          <div className="row"><button type="button" className="btn sm" disabled={sets.length >= 5} onClick={() => setSets(s => [...s, ['', '']])}><Icon name="plus" size={14} />A set</button>
            {sets.length > 1 && <button type="button" className="btn sm ghost" onClick={() => setSets(s => s.slice(0, -1))}>Remove the last</button>}</div>
        </div>}
        {outcome === 'normal' && !rotating && (scoring === 'goals' || scoring === 'points') && <div className="ab-in">
          <label><span className="muted small">{a}</span><input inputMode="numeric" value={ab[0]} onChange={e => setAB([e.target.value.replace(/\D/g, ''), ab[1]])} required /></label>
          <span className="ab-dash">–</span>
          <label><span className="muted small">{b}</span><input inputMode="numeric" value={ab[1]} onChange={e => setAB([ab[0], e.target.value.replace(/\D/g, '')])} required /></label>
        </div>}
        {((outcome === 'normal' && !rotating && scoring !== 'sets' && scoring !== 'goals' && scoring !== 'points') || ['walkover', 'no_show', 'retired'].includes(outcome)) &&
          <Field label={outcome === 'normal' ? 'Who won' : 'Who goes through'} group><Segmented value={winner} onChange={setWinner} options={[{ value: 'a', label: a }, ...(outcome === 'normal' && draws ? [{ value: 'draw' as const, label: 'Draw' }] : []), { value: 'b', label: b }]} /></Field>}
        {!draws && outcome === 'normal' && !rotating && <p className="muted small">A knockout match needs a winner — count the tie-break, extra time or penalties.</p>}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Modal>
  )
}

/** A knockout bracket: one column per round, read left to right. */
export function Bracket({ d, ms, onResult, onChanged, readOnly }: { d: TDetail; ms: TMatch[]; onResult: (m: TMatch) => void; onChanged: (d: TDetail) => void; readOnly?: boolean }) {
  const rounds = Array.from(new Set(ms.map(m => m.round))).sort((x, y) => x - y)
  return (
    <div className="bracket scroll-x">
      {rounds.map(r => {
        const list = ms.filter(m => m.round === r).sort((x, y) => x.position - y.position)
        const title = list.length === 1 ? (list[0].label.replace(/ \d+$/, '') || `Round ${r}`) : list[0]?.label.replace(/, match \d+$/, '').replace(/ \d+$/, '') || `Round ${r}`
        return (
          <div key={r} className="br-col">
            <h5 className="br-h">{title}</h5>
            <div className="br-list">{list.map(m => <div key={m.id} className="br-cell"><TMatchLine d={d} m={m} onResult={onResult} onChanged={onChanged} compact readOnly={readOnly} /></div>)}</div>
          </div>
        )
      })}
    </div>
  )
}

/** A group's table with what decided the ties. */
export function TTable({ d, rows, kind }: { d: TDetail; rows: TRow[]; kind: string }) {
  const name = (id: number) => d.entries.find(e => e.id === id)?.name || '?'
  const rotating = ['americano', 'mexicano', 'kotc'].includes(kind)
  const swiss = kind === 'swiss'
  const sets = d.group.scoring === 'sets' && !rotating
  const advance = d.tournament.format === 'groups_knockout' && kind === 'round_robin' ? d.settings.advance : 0
  return (
    <div className="card flush scroll-x">
      <table className="table league-table">
        <thead><tr><th className="num">#</th><th>{d.tournament.entry_kind === 'team' ? 'Team' : 'Player'}</th><th className="num">P</th><th className="num">W</th>
          {!rotating && <th className="num">D</th>}<th className="num">L</th>
          {sets && <th className="num hide-sm">Sets</th>}
          {!swiss && <th className="num hide-sm">{rotating ? 'Diff' : sets ? 'Games' : '+/−'}</th>}
          {swiss && <th className="num hide-sm" title="Buchholz">Bh</th>}{swiss && <th className="num hide-sm" title="Sonneborn-Berger">SB</th>}
          <th className="num">{rotating ? 'Points' : 'Pts'}</th></tr></thead>
        <tbody>{rows.map(r => (
          <tr key={r.id} className={`${advance && r.rank <= advance && r.played > 0 ? 'top' : ''} ${d.mine.includes(r.id) ? 'me' : ''}`}>
            <td className="num muted">{r.played ? r.rank : ''}</td>
            <td><span className="tbl-name">{name(r.id)}</span>{r.decided_by && <span className="muted tiny" title={`Tied: decided by ${r.decided_by}`}> · {r.decided_by}</span>}</td>
            <td className="num">{r.played}</td><td className="num">{r.won}</td>{!rotating && <td className="num">{r.drawn}</td>}<td className="num">{r.lost}</td>
            {sets && <td className="num hide-sm muted">{r.sets_won}–{r.sets_lost}</td>}
            {!swiss && <td className="num hide-sm muted">{sets ? `${r.for}–${r.against}` : `${r.for - r.against > 0 ? '+' : ''}${r.for - r.against}`}</td>}
            {swiss && <td className="num hide-sm muted">{r.buchholz}</td>}{swiss && <td className="num hide-sm muted">{r.sonneborn_berger}</td>}
            <td className="num"><b>{r.points}</b></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  )
}

/** Scored rounds: the leaderboard, with each round. */
export function Leaderboard({ d, rows, mode }: { d: TDetail; rows: TLeader[]; mode: string }) {
  const name = (id: number) => d.entries.find(e => e.id === id)?.name || '?'
  const n = Math.max(0, ...rows.map(r => r.rounds.length))
  const unit = mode === 'stableford' ? 'pts' : mode === 'pins' ? 'pins' : mode === 'stroke' ? 'net' : ''
  return (
    <div className="card flush scroll-x">
      <table className="table league-table">
        <thead><tr><th className="num">#</th><th>Player</th>{Array.from({ length: n }, (_, i) => <th key={i} className="num hide-sm">R{i + 1}</th>)}<th className="num">Total {unit}</th>{mode !== 'pins' && <th className="num hide-sm">Gross</th>}</tr></thead>
        <tbody>{rows.map(r => (
          <tr key={r.id} className={`${r.rank === 1 && r.gross > 0 ? 'top' : ''} ${d.mine.includes(r.id) ? 'me' : ''}`}>
            <td className="num muted">{r.gross > 0 ? r.rank : ''}</td>
            <td><span className="tbl-name">{name(r.id)}</span>{r.decided_by && <span className="muted tiny"> · countback</span>}{r.holes > 0 && r.holes < 18 && mode !== 'pins' && <span className="muted tiny"> · thru {r.holes}</span>}</td>
            {Array.from({ length: n }, (_, i) => <td key={i} className="num hide-sm muted">{r.rounds[i] || ''}</td>)}
            <td className="num"><b>{r.gross > 0 ? r.total : '–'}</b></td>{mode !== 'pins' && <td className="num hide-sm muted">{r.gross || ''}</td>}
          </tr>
        ))}</tbody>
      </table>
    </div>
  )
}
