// A tournament: pick the format (the sport's best first), then who enters,
// when and where, and the settings that format has.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { announceChange, useData } from '../lib/data'
import { CURRENCIES, nextSlot } from '../lib/league'
import { ENTRY_WORDS, type TDetail, type TFormat, type TSettings, type Tournament } from '../lib/tour'
import type { Game } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Loading, Modal, Segmented, Switch, useToast } from './ui'

interface Formats { formats: TFormat[]; suggested: string[]; defaults: Record<string, TSettings>; tiebreaks: Record<string, string> }

export function TournamentForm({ groupId, tournament, onClose, onSaved }: { groupId?: number; tournament?: Tournament; onClose: () => void; onSaved: (id: number) => void }) {
  const { groups } = useData()
  const { toast } = useToast()
  const live = (groups || []).filter(g => !g.archived)
  const [gid, setGid] = useState(tournament?.group_id || groupId || live[0]?.id || 0)
  const g = (groups || []).find(x => x.id === gid)
  const [fm, setFm] = useState<Formats | null>(null)
  const [format, setFormat] = useState(tournament?.format || '')
  const [f, setF] = useState(() => ({
    name: tournament?.name || '', entry_kind: tournament?.entry_kind || '', starts_at: tournament?.starts_at || '', ends_at: tournament?.ends_at || '',
    venue_name: tournament?.venue_name ?? '', capacity: tournament?.capacity || 0, fee: tournament?.fee_cents ? String(tournament.fee_cents / 100) : '',
    currency: tournament?.currency || '', checkin: !!tournament?.checkin, rated: tournament?.rated ?? true, signup_until: tournament?.signup_until || '',
    courts: '', add_members: false, from_game: 0,
  }))
  const [set, setSet] = useState<TSettings | null>(tournament?.settings || null)
  const [games, setGames] = useState<Game[]>([])
  const [busy, setBusy] = useState(false)
  const [more, setMore] = useState(false)
  useEffect(() => {
    if (!gid) return
    api.get<Formats>(`${V1}/tournaments/formats?group=${gid}`).then(x => {
      setFm(x)
      if (!tournament) setFormat(fv => fv || x.suggested[0])
    }).catch(e => toast((e as Error).message, 'err'))
    api.get<Game[]>(`${V1}/games?group=${gid}&upcoming=1`).then(setGames).catch(() => setGames([]))
  }, [gid]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!fm || !format || tournament) return
    setSet(fm.defaults[format])
    const fmt = fm.formats.find(x => x.key === format)
    const kind = g?.format === 'doubles' ? 'pair' : g?.format === 'teams' ? 'team' : 'player'
    setF(x => ({ ...x, entry_kind: fmt && fmt.entries.includes(kind as 'player') ? kind : fmt?.entries[0] || 'player', currency: x.currency || g?.currency || 'EUR',
      venue_name: x.venue_name || g?.venue_name || '', starts_at: x.starts_at || nextSlot(g),
      name: x.name || `${g?.sport_label || ''} ${fmt?.name || ''}`.trim(), courts: x.courts || (fm.defaults[format].schedule === 'none' ? '' : g?.format === 'teams' ? 'Pitch 1' : 'Court 1, Court 2') }))
  }, [fm, format]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!g) return null
  if (!fm || !set) return <Modal title={tournament ? 'Tournament settings' : 'A new tournament'} onClose={onClose}><Loading /></Modal>
  const fmt = fm.formats.find(x => x.key === format)
  const order = [...fm.suggested, ...fm.formats.map(x => x.key).filter(k => !fm.suggested.includes(k))]
  const S = (p: Partial<TSettings>) => setSet({ ...set, ...p })
  const signup = !tournament || tournament.status === 'signup'

  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    const body: Record<string, unknown> = {
      name: f.name, format, entry_kind: f.entry_kind, starts_at: f.starts_at, ends_at: f.ends_at, venue_name: f.venue_name, capacity: f.capacity,
      fee_cents: Math.round((Number(f.fee.replace(',', '.')) || 0) * 100), currency: f.currency, checkin: f.checkin, rated: f.rated, signup_until: f.signup_until, settings: set,
    }
    if (!tournament || f.courts) body.courts = f.courts.split(',').map(x => x.trim()).filter(Boolean)
    if (!tournament) { body.add_members = f.add_members; body.from_game = f.from_game }
    try {
      const d = tournament ? await api.put<TDetail>(`${V1}/tournaments/${tournament.id}`, body) : await api.post<TDetail>(`${V1}/groups/${gid}/tournaments`, body)
      announceChange(); toast(tournament ? 'Saved' : 'Tournament made — sign-up is open'); onSaved(d.tournament.id)
    } catch (err) { toast((err as Error).message, 'err') } finally { setBusy(false) }
  }

  const rotating = fmt?.rotating
  return (
    <Modal title={tournament ? 'Tournament settings' : 'A new tournament'} onClose={onClose} size="lg">
      <form onSubmit={save} className="stack loose">
        {!tournament && live.length > 1 && <Field label="Group"><Select ariaLabel="Group" value={String(gid)} onChange={v => { setGid(Number(v)); setFormat('') }} options={live.map(x => ({ value: String(x.id), label: `${x.sport_info?.emoji || ''} ${x.name}` }))} /></Field>}
        {signup && <div className="form-sec">
          <h4 className="form-sec-h">Format</h4>
          <div className="format-grid" role="radiogroup" aria-label="Format">
            {order.map((k, i) => {
              const x = fm.formats.find(y => y.key === k)!
              return <button key={k} type="button" role="radio" aria-checked={format === k} className={`format-pick ${format === k ? 'on' : ''}`} onClick={() => { setFormat(k); if (tournament) setSet({ ...fm.defaults[k], ...{} }) }}>
                <span className="format-name">{x.name}{i < 3 && fm.suggested.includes(k) && <span className="badge accent">suits {g.sport_label.toLowerCase()}</span>}</span>
                <span className="muted small">{x.description}</span>
              </button>
            })}
          </div>
        </div>}
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required maxLength={100} /></Field>
        {fmt && fmt.entries.length > 1 && signup && <Field label="Who enters" group><Segmented value={f.entry_kind} onChange={v => setF({ ...f, entry_kind: v as 'player' })} options={fmt.entries.map(k => ({ value: k, label: ENTRY_WORDS[k] }))} /></Field>}

        <div className="form-sec">
          <h4 className="form-sec-h">{format === 'ladder' || format === 'box_league' ? 'The season' : 'The day'}</h4>
          <div className="two-col tight">
            <Field label="Starts"><input type="datetime-local" value={f.starts_at} onChange={e => setF({ ...f, starts_at: e.target.value })} /></Field>
            {(format === 'arena' || format === 'ladder' || format === 'box_league') ? <Field label={format === 'arena' ? 'Pairing stops at' : 'Ends'}><input type="datetime-local" value={f.ends_at} onChange={e => setF({ ...f, ends_at: e.target.value })} /></Field>
              : <Field label="Venue"><input value={f.venue_name} onChange={e => setF({ ...f, venue_name: e.target.value })} maxLength={120} /></Field>}
          </div>
          {set.schedule !== 'none' && <Field label="Courts / pitches" help="Separated by commas. The next match goes to the first free one, and its players are told.">
            <input value={f.courts} onChange={e => setF({ ...f, courts: e.target.value })} placeholder="Court 1, Court 2" />
          </Field>}
        </div>

        {/* the format's own settings */}
        <div className="form-sec">
          <h4 className="form-sec-h">How it's played</h4>
          {(format === 'groups_knockout' || format === 'round_robin' || format === 'team_ties') && <div className="three-col tight">
            <Field label="Groups"><input type="number" min={1} max={32} value={set.groups || 1} onChange={e => S({ groups: Math.max(1, Number(e.target.value) || 1) })} /></Field>
            {format === 'groups_knockout' && <Field label="Through from each"><input type="number" min={1} max={16} value={set.advance} onChange={e => S({ advance: Math.max(1, Number(e.target.value) || 1) })} /></Field>}
            <label className="check-row"><Switch checked={set.double} onChange={v => S({ double: v })} label="Twice" /> Everyone twice (home and away)</label>
          </div>}
          {(format === 'knockout' || format === 'groups_knockout') && <div className="stack tight">
            <label className="check-row"><Switch checked={set.third_place} onChange={v => S({ third_place: v })} label="Third place" /> A match for third place</label>
            <label className="check-row"><Switch checked={set.plate} onChange={v => S({ plate: v })} label="Plate" /> A plate: the first-round losers play on</label>
          </div>}
          {format === 'double_elim' && <label className="check-row"><Switch checked={set.reset} onChange={v => S({ reset: v })} label="Reset" /> A second grand final if the losers' side wins the first</label>}
          {rotating && <div className="three-col tight">
            <Field label="Points a match" help="16, 21, 24 or 32 — or play to time"><input type="number" min={4} max={200} value={set.points} onChange={e => S({ points: Number(e.target.value) || 24 })} /></Field>
            <Field label="Rounds"><input type="number" min={1} max={60} value={set.rounds} onChange={e => S({ rounds: Number(e.target.value) || 1 })} /></Field>
            {format === 'mexicano' && <Field label="On each court"><Select ariaLabel="Mexicano pairs" value={set.split} onChange={v => S({ split: v })} options={[{ value: '1-3', label: '1st + 3rd v 2nd + 4th' }, { value: '1-4', label: '1st + 4th v 2nd + 3rd' }]} /></Field>}
          </div>}
          {format === 'swiss' && <div className="two-col tight">
            <Field label="Rounds"><input type="number" min={1} max={30} value={set.rounds} onChange={e => S({ rounds: Number(e.target.value) || 1 })} /></Field>
            <label className="check-row"><Switch checked={set.colours} onChange={v => S({ colours: v })} label="Colours" /> Balance white and black (chess)</label>
          </div>}
          {format === 'ranking' && <div className="stack tight">
            <div className="two-col tight">
              <Field label="Scoring"><Select ariaLabel="Scoring" value={set.mode || 'plain'} onChange={v => S({ mode: v as TSettings['mode'] })} options={[
                { value: 'stableford', label: 'Stableford points (golf)' }, { value: 'stroke', label: 'Stroke play, net (golf)' }, { value: 'pins', label: 'Pins, with handicap (bowling)' }, { value: 'plain', label: 'A score each round' }]} /></Field>
              <Field label="Rounds"><input type="number" min={1} max={20} value={set.rounds} onChange={e => S({ rounds: Number(e.target.value) || 1 })} /></Field>
            </div>
            {set.mode === 'plain' && <label className="check-row"><Switch checked={set.low} onChange={v => S({ low: v })} label="Lowest wins" /> The lowest score wins</label>}
            {(set.mode === 'stableford' || set.mode === 'stroke') && <>
              <Field label="Par per hole" help="18 numbers (or 9), separated by spaces"><input value={(set.course?.par || []).join(' ')} onChange={e => S({ course: { par: e.target.value.split(/[\s,]+/).map(Number).filter(Boolean), si: set.course?.si || [] } })} /></Field>
              <Field label="Stroke index per hole" help="Which holes get handicap strokes first"><input value={(set.course?.si || []).join(' ')} onChange={e => S({ course: { par: set.course?.par || [], si: e.target.value.split(/[\s,]+/).map(Number).filter(Boolean) } })} /></Field>
              <Field label="Handicap allowance"><Select ariaLabel="Allowance" value={String(set.allowance)} onChange={v => S({ allowance: Number(v) })} options={[{ value: '1', label: '100%' }, { value: '0.95', label: '95% (individual stroke play)' }, { value: '0.85', label: '85%' }, { value: '0.75', label: '75%' }]} /></Field>
            </>}
            {set.mode === 'pins' && <div className="two-col tight">
              <Field label="Handicap basis"><input type="number" value={set.basis} onChange={e => S({ basis: Number(e.target.value) || 220 })} /></Field>
              <Field label="Percent"><input type="number" step="0.05" min={0} max={1} value={set.pct} onChange={e => S({ pct: Number(e.target.value) || 0.9 })} /></Field>
            </div>}
          </div>}
          {format === 'team_ties' && <Field label="The matches of a tie" help="singles or doubles, separated by commas"><input value={set.ties.join(', ')} onChange={e => S({ ties: e.target.value.split(',').map(x => x.trim().toLowerCase()).filter(x => x === 'singles' || x === 'doubles') })} /></Field>}
          {format === 'ladder' && <div className="three-col tight">
            <Field label="Challenge up to" help="places above"><input type="number" min={1} max={20} value={set.reach} onChange={e => S({ reach: Number(e.target.value) || 3 })} /></Field>
            <Field label="Answer within" help="days, or it's a loss"><input type="number" min={1} max={30} value={set.respond_days} onChange={e => S({ respond_days: Number(e.target.value) || 5 })} /></Field>
            <Field label="Play within" help="days after"><input type="number" min={1} max={60} value={set.play_days} onChange={e => S({ play_days: Number(e.target.value) || 14 })} /></Field>
          </div>}
          {format === 'box_league' && <div className="two-col tight">
            <Field label="Players a box"><input type="number" min={3} max={10} value={set.box_size} onChange={e => S({ box_size: Number(e.target.value) || 5 })} /></Field>
            <Field label="Up and down each round"><input type="number" min={1} max={4} value={set.promote} onChange={e => S({ promote: Number(e.target.value) || 1 })} /></Field>
          </div>}
          <div className="three-col tight">
            <Field label="Seeding"><Select ariaLabel="Seeding" value={set.seeding} onChange={v => S({ seeding: v as TSettings['seeding'] })} options={[{ value: 'rating', label: 'By rating' }, { value: 'random', label: 'A random draw' }, { value: 'manual', label: 'Seeds I set, then rating' }]} /></Field>
            {format !== 'ranking' && format !== 'ladder' && <Field label="Calling matches"><Select ariaLabel="Schedule" value={set.schedule} onChange={v => S({ schedule: v as TSettings['schedule'] })} options={[{ value: 'queue', label: 'To the next free court' }, { value: 'timetable', label: 'A timetable up front' }, { value: 'none', label: 'Players find a court' }]} /></Field>}
            {format !== 'ranking' && <Field label="A match takes" help="minutes"><input type="number" min={5} max={480} value={set.minutes} onChange={e => S({ minutes: Number(e.target.value) || 20 })} /></Field>}
          </div>
        </div>

        <button type="button" className="linkish self-start" onClick={() => setMore(m => !m)}><Icon name={more ? 'chevron-up' : 'chevron-down'} size={14} /> {more ? 'Less' : 'More: entries, fee, check-in, results'}</button>
        {more && <div className="form-sec">
          <div className="three-col tight">
            <Field label="At most" help="entries; 0 = no limit (then a waiting list)"><input type="number" min={0} max={512} value={f.capacity} onChange={e => setF({ ...f, capacity: Number(e.target.value) || 0 })} /></Field>
            <Field label="Entry fee"><div className="input-group"><input inputMode="decimal" value={f.fee} onChange={e => setF({ ...f, fee: e.target.value })} placeholder="0" />
              <Select ariaLabel="Currency" value={f.currency} onChange={v => setF({ ...f, currency: v })} options={Array.from(new Set([f.currency, ...CURRENCIES])).filter(Boolean).map(c => ({ value: c, label: c === 'BAM' ? 'KM' : c }))} /></div></Field>
            <Field label="Sign-up until"><input type="datetime-local" value={f.signup_until} onChange={e => setF({ ...f, signup_until: e.target.value })} /></Field>
          </div>
          <label className="check-row"><Switch checked={f.checkin} onChange={v => setF({ ...f, checkin: v })} label="Check-in" /> Check-in on the day: only those who checked in are drawn</label>
          <label className="check-row"><Switch checked={f.rated} onChange={v => setF({ ...f, rated: v })} label="Rated" /> Results count in the group's table and ratings</label>
          <Field label="A result counts by itself after" help="minutes without the other side confirming (0 = only when confirmed)"><input type="number" min={0} max={1440} value={set.auto_confirm} onChange={e => S({ auto_confirm: Number(e.target.value) || 0 })} /></Field>
        </div>}

        {!tournament && <div className="form-sec">
          <h4 className="form-sec-h">Who's in</h4>
          {f.entry_kind === 'player' && <label className="check-row"><Switch checked={f.add_members} onChange={v => setF({ ...f, add_members: v, from_game: 0 })} label="Everyone" /> Sign up everyone in the group (take out who can't come)</label>}
          {games.filter(x => x.status === 'planned').length > 0 && <Field label="Or everyone who's in for a game"><Select ariaLabel="Game" value={f.from_game ? String(f.from_game) : ''} placeholder="No, they sign up themselves" clearLabel="No, they sign up themselves"
            onChange={v => setF({ ...f, from_game: Number(v) || 0, add_members: false })} options={games.filter(x => x.status === 'planned').map(x => ({ value: String(x.id), label: `${x.starts_at.replace('T', ' ')} · ${x.in} in` }))} /></Field>}
        </div>}

        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy || !format}>{tournament ? 'Save' : 'Make the tournament'}</button></div>
      </form>
    </Modal>
  )
}
