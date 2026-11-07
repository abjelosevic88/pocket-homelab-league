// Live scoring: a scoreboard everyone sees move, and the big buttons for
// whoever keeps the score at the court.
import { useEffect, useState } from 'react'
import { useApp } from '../lib/app-context'
import { useLiveEvents } from '../lib/data'
import { ApiError } from '../lib/api'
import { dropLive, finishLive, livePoint, type LiveRules, type LiveScore, type LiveView } from '../lib/live'
import { Icon } from './icons'
import { Confirm, Modal, useToast } from './ui'

/** The score: sets, the set being played and the points. */
export function LiveBoard({ a, b, view, rules, compact = false }: { a: string; b: string; view: LiveView; rules: LiveRules; compact?: boolean }) {
  const count = rules.kind === 'count'
  const row = (i: 0 | 1, name: string) => {
    const side = i === 0 ? 'a' : 'b'
    return (
      <div className={`lb-row ${view.winner === side ? 'won' : ''}`}>
        <span className="lb-name ellipsis">{name}{!view.done && view.match_point === side ? <span className="lb-flag">match point</span> : !view.done && view.set_point === side ? <span className="lb-flag">set point</span> : null}</span>
        {!count && view.sets.map((s, k) => <span key={k} className={`lb-set ${s[i] > s[1 - i] ? 'won' : ''}`}>{s[i]}</span>)}
        {!view.done && <span className="lb-cur">{view.cur[i]}</span>}
        {!count && !view.done && rules.kind === 'tennis' && <span className="lb-pt">{view.point[i] || (view.point[1 - i] === 'AD' ? '40' : '')}</span>}
      </div>
    )
  }
  return (
    <div className={`live-board ${compact ? 'compact' : ''} ${view.done ? 'done' : ''}`}>
      {row(0, a)}{row(1, b)}
      {!compact && !view.done && (view.tiebreak || view.deuce || view.golden) && <div className="lb-note">{view.tiebreak ? 'Tie-break' : view.golden ? 'Golden point' : 'Deuce'}</div>}
    </div>
  )
}

export function LiveDot() { return <span className="live-dot" aria-label="live" /> }

/** Keep the score: tap who won the point. */
export function LiveScorer({ l: first, onClose, onFinish }: { l: LiveScore; onClose: () => void; onFinish: (l: LiveScore) => Promise<boolean> }) {
  const { user } = useApp()
  const { toast } = useToast()
  const [l, setL] = useState(first)
  const [busy, setBusy] = useState(false)
  const [drop, setDrop] = useState(false)
  const [other, setOther] = useState('')
  useLiveEvents(e => { if (e.type === 'live' && e.data.id === l.id && !e.data.deleted) setL(e.data) })
  useEffect(() => { setOther(l.scorer && l.scorer !== user?.id ? l.scorer_name : '') }, [l.scorer, l.scorer_name, user?.id])
  const send = async (body: Record<string, unknown>) => {
    setBusy(true)
    try { setL(await livePoint(l.id, { ...body, at: l.points.length })) } catch (e) {
      const err = e as ApiError
      if (err.status === 409 && err.data.live) setL(err.data.live as LiveScore)
      toast(err.message, 'err')
    } finally { setBusy(false) }
  }
  const count = l.rules.kind === 'count'
  const done = l.view.done
  const finish = async () => {
    setBusy(true)
    try { if (await onFinish(l)) { await finishLive(l.id).catch(() => {}); onClose() } } finally { setBusy(false) }
  }
  const tap = (side: 'a' | 'b', n = 1) => { navigator.vibrate?.(15); send({ side, n, take: !!other }) }
  return (
    <Modal title="Live score" onClose={onClose} size="lg">
      <div className="stack live-scorer">
        <LiveBoard a={l.a_name} b={l.b_name} view={l.view} rules={l.rules} />
        {other && <p className="small warn-text"><Icon name="users" size={12} /> {other} is keeping the score too — your taps take over.</p>}
        {!done && <div className="live-taps">
          {(['a', 'b'] as const).map(side => (
            <div key={side} className="live-tap-col">
              <button type="button" className={`live-tap side-${side}`} disabled={busy} onClick={() => tap(side)}>
                <span className="ellipsis">{side === 'a' ? l.a_name : l.b_name}</span><b>+1</b>
              </button>
              {count && <div className="row tight">{[2, 3].map(n => <button key={n} type="button" className="btn sm grow" disabled={busy} onClick={() => tap(side, n)}>+{n}</button>)}</div>}
            </div>
          ))}
        </div>}
        <div className="form-actions">
          <button type="button" className="btn ghost danger-text" onClick={() => setDrop(true)}>Drop it</button>
          <button type="button" className="btn" disabled={busy || l.points.length === 0} onClick={() => send({ undo: true, take: !!other })}><Icon name="undo" size={15} />Undo</button>
          <span className="spacer" />
          <button type="button" className={`btn ${done ? 'primary' : ''}`} disabled={busy || l.points.length === 0} onClick={finish}><Icon name="check" size={15} />{done ? 'Save the result' : count ? 'Full time — save' : 'Stop here — save'}</button>
        </div>
        <p className="muted small">Everyone in the group{l.tmatch_id ? ' and on the tournament page' : ''} sees the score as you go. {l.rules.kind === 'tennis' ? `Best of ${l.rules.sets}, sets to ${l.rules.games}${l.rules.tiebreak_at ? `, tie-break at ${l.rules.tiebreak_at}–${l.rules.tiebreak_at}` : ''}${l.rules.super_final ? ', a super tie-break to 10 decides' : ''}${l.rules.golden ? ', golden point at deuce' : ''}.` : l.rules.kind === 'rally' ? `Best of ${l.rules.sets}, sets to ${l.rules.points}${l.rules.last_points ? ` (the last to ${l.rules.last_points})` : ''}, win by ${l.rules.win_by}${l.rules.cap ? `, at most ${l.rules.cap}` : ''}.` : ''}</p>
      </div>
      {drop && <Confirm title="Drop the live score?" message="The score so far goes. Enter the result by hand instead." confirmLabel="Drop it" onCancel={() => setDrop(false)}
        onConfirm={async () => { setDrop(false); try { await dropLive(l.id); onClose() } catch (e) { toast((e as Error).message, 'err') } }} />}
    </Modal>
  )
}
