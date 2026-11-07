// Pieces the pages share: a game card with "are you in?", a result line
// with confirm, form dots.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import { ANSWER_WORDS, fullness, money, relDay, sideName, when } from '../lib/league'
import type { AnswerKind, Format, Game, Match } from '../lib/types'
import { Icon } from './icons'
import { Modal, useToast } from './ui'

/** In / Maybe / Out, saved at once. */
export function AnswerButtons({ game, onChange, size = '' }: { game: Game; onChange?: (a: AnswerKind) => void; size?: '' | 'sm' }) {
  const { toast } = useToast()
  const [mine, setMine] = useState<AnswerKind>(game.my_answer)
  const [busy, setBusy] = useState(false)
  if (game.status !== 'planned' || game.ended) return null
  const set = async (a: AnswerKind) => {
    const next = a === mine ? '' : a
    setBusy(true)
    try {
      const r = await api.post<{ result: AnswerKind }>(`${V1}/games/${game.id}/answer`, { answer: next })
      setMine(r.result)
      if (r.result === 'waiting') toast('The game is full: you are on the waiting list, and move up when a place comes free')
      onChange?.(r.result); announceChange()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const opts: { a: Exclude<AnswerKind, '' | 'waiting'>; icon: string }[] = [{ a: 'in', icon: 'check' }, { a: 'maybe', icon: 'help' }, { a: 'out', icon: 'x' }]
  return (
    <div className={`answers ${size}`} role="group" aria-label="Are you coming?">
      {opts.map(o => {
        const on = mine === o.a || (o.a === 'in' && mine === 'waiting')
        return <button key={o.a} type="button" className={`ans ans-${o.a} ${on ? 'on' : ''}`} aria-pressed={on} disabled={busy} onClick={() => set(o.a)}>
          <Icon name={o.icon} size={14} strokeWidth={2.2} />{o.a === 'in' && mine === 'waiting' ? 'Waiting' : ANSWER_WORDS[o.a]}
        </button>
      })}
    </div>
  )
}

/** A coming game: when, where, how full, and the answer buttons. */
export function GameCard({ game, showGroup = true }: { game: Game; showGroup?: boolean }) {
  const full = fullness(game)
  return (
    <div className={`game-card ${game.status}`} style={{ ['--grp' as string]: game.group_color || 'var(--accent)' }}>
      <Link to={`/games/${game.id}`} className="game-card-main">
        <span className="game-emoji" aria-hidden="true">{game.emoji || '🏅'}</span>
        <span className="grow min0">
          <span className="game-when">{game.status === 'voting' ? 'Finding a time' : <><b>{cap(relDay(game.starts_at))}</b> · {when(game.starts_at).split(' · ')[1]}</>}</span>
          <span className="game-title ellipsis">{showGroup ? game.group_name : game.title || (game.status === 'voting' ? 'Vote on the times' : when(game.starts_at))}{game.title && showGroup ? ` · ${game.title}` : ''}</span>
          <span className="game-sub ellipsis">{[game.venue_name, game.status === 'planned' && game.cost_cents && game.in ? `${money(Math.ceil(game.cost_cents / Math.max(game.in, 1)), game.currency)} each` : ''].filter(Boolean).join(' · ')}</span>
        </span>
        <span className={`fill fill-${full.tone}`}>{full.tone === 'warn' && <Icon name="alert" size={12} />}{full.text}</span>
      </Link>
      {game.status === 'planned' && !game.ended && <AnswerButtons game={game} size="sm" />}
      {game.status === 'voting' && <Link to={`/games/${game.id}`} className="btn sm">Vote on the times</Link>}
    </div>
  )
}

export function cap(s: string) { return s ? s[0].toUpperCase() + s.slice(1) : s }

/** W / D / L as small marks (with the letter, never colour alone). */
export function FormDots({ form }: { form: ('W' | 'D' | 'L')[] }) {
  return <span className="form-dots" aria-label={`Form: ${form.join(' ')}`}>{form.map((f, i) => <span key={i} className={`fd fd-${f}`}>{f}</span>)}</span>
}

/** A result: the sides, the score, its state, and confirm / not right. */
export function MatchRow({ m, format, group, emoji, onChanged, onEdit }: { m: Match; format: Format; group?: string; emoji?: string; onChanged?: () => void; onEdit?: (m: Match) => void }) {
  const { user } = useApp()
  const { toast } = useToast()
  const [why, setWhy] = useState<string | null>(null)
  const a = m.players.filter(p => p.side === 'a').map(p => p.user_id)
  const b = m.players.filter(p => p.side === 'b').map(p => p.user_id)
  const ranking = a.length === 0 && b.length === 0
  const mineIn = m.players.some(p => p.user_id === user?.id)
  const confirm = async (wrong: boolean, reason = '') => {
    try { await api.post(`${V1}/matches/${m.id}/confirm`, { wrong, reason }); toast(wrong ? 'Marked as not right: it leaves the table until it is fixed' : 'Confirmed'); setWhy(null); announceChange(); onChanged?.() } catch (e) { toast((e as Error).message, 'err') }
  }
  const scorers = m.players.filter(p => p.side && p.score).map(p => `${(m.names[String(p.user_id)] || '?').split(' ')[0]}${p.score && p.score > 1 ? ` ×${p.score}` : ''}`)
  return (
    <div className={`match ${m.state}`}>
      <div className="match-main">
        {emoji && <span className="game-emoji sm" aria-hidden="true">{emoji}</span>}
        <div className="grow min0">
          {ranking ? (
            <div className="rank-line">{[...m.players].sort((x, y) => x.place - y.place).map(p => <span key={p.user_id} className={p.place === 1 ? 'won' : ''}>{p.place}. {m.names[String(p.user_id)]?.split(' ')[0]} <b className="tnum">{p.score}</b></span>)}</div>
          ) : (
            <div className="vs-line">
              <span className={`vs-side ${m.winner === 'a' ? 'won' : ''}`}>{sideName(m.names, a, format)}</span>
              <span className="vs-score tnum">{m.text || (m.winner === 'draw' ? 'draw' : '–')}</span>
              <span className={`vs-side right ${m.winner === 'b' ? 'won' : ''}`}>{sideName(m.names, b, format)}</span>
            </div>
          )}
          <div className="match-sub muted small">
            {group && <span>{group} · </span>}{fmtDate(m.played_at, false)}
            {scorers.length > 0 && <span> · ⚽ {scorers.join(', ')}</span>}
            {m.note && <span> · {m.note}</span>}
            {m.state === 'waiting' && <span className="pill st-due"><Icon name="clock" size={11} />to confirm</span>}
            {m.state === 'disputed' && <span className="pill st-overdue" title={m.dispute}><Icon name="alert" size={11} />not right{m.dispute ? `: ${m.dispute}` : ''}</span>}
          </div>
        </div>
        {onEdit && m.can_edit && <button type="button" className="btn ghost icon-only sm" aria-label="Fix the result" title="Fix the result" onClick={() => onEdit(m)}><Icon name="edit" size={14} /></button>}
      </div>
      {m.can_confirm && mineIn && <div className="row tight match-actions">
        <button type="button" className="btn sm primary" onClick={() => confirm(false)}><Icon name="check" size={14} />Right</button>
        <button type="button" className="btn sm" onClick={() => setWhy('')}>Not right</button>
      </div>}
      {why !== null && <Modal title="What's not right?" onClose={() => setWhy(null)} size="sm">
        <form className="stack" onSubmit={e => { e.preventDefault(); confirm(true, why) }}>
          <input autoFocus value={why} onChange={e => setWhy(e.target.value)} placeholder="It was 6–4 in the second" maxLength={300} />
          <div className="form-actions"><button type="button" className="btn" onClick={() => setWhy(null)}>Cancel</button><button className="btn primary">Send</button></div>
        </form>
      </Modal>}
    </div>
  )
}
