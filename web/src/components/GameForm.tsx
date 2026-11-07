// Plan a game: at one time, or a few times for everyone to vote on. Also
// edits a game (time, place, players, cost, notes).
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useData } from '../lib/data'
import { CURRENCIES, localTS, nextSlot } from '../lib/league'
import type { Game, GameDetail } from '../lib/types'
import { offTitle } from './Apps'
import { VenuePicker } from './GroupForm'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Modal, Segmented, Switch, useToast } from './ui'

export function GameForm({ groupId, game, onClose, onSaved }: { groupId?: number; game?: Game; onClose: () => void; onSaved: (id: number) => void }) {
  const { groups } = useData()
  const { settings } = useApp()
  const { toast } = useToast()
  const tasks = useAppState()('tasks', 'tasks:write')
  const live = (groups || []).filter(g => !g.archived)
  const [gid, setGid] = useState<number>(game?.group_id || groupId || live[0]?.id || 0)
  const g = live.find(x => x.id === gid) || (groups || []).find(x => x.id === gid)
  const [mode, setMode] = useState<'time' | 'vote'>(game?.status === 'voting' ? 'vote' : 'time')
  const [f, setF] = useState(() => ({
    starts_at: game?.starts_at || '', title: game?.title || '', notes: game?.notes || '', duration_min: game?.duration_min || 0,
    venue_name: game?.venue_name, venue_address: game?.venue_address, venue_lat: game?.venue_lat ?? null, venue_lon: game?.venue_lon ?? null,
    min_players: game?.min_players || 0, max_players: game?.max_players ?? -1, cost: game ? (game.cost_cents ? String(game.cost_cents / 100) : '') : '', currency: game?.currency || '',
    vote_until: game?.vote_until || '', im_in: !game, booking_task: !!settings?.booking_task,
  }))
  const [options, setOptions] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  // a new game starts from the group's usual time, place, players and cost
  useEffect(() => {
    if (game || !g) return
    const first = nextSlot(g)
    setF(x => ({ ...x, starts_at: x.starts_at || first, duration_min: g.duration_min, venue_name: g.venue_name, venue_address: g.venue_address, venue_lat: g.venue_lat, venue_lon: g.venue_lon,
      min_players: g.min_players, max_players: g.max_players, cost: g.cost_cents ? String(g.cost_cents / 100) : '', currency: g.currency }))
    const d = new Date(first)
    setOptions([first, localTS(new Date(d.getTime() + 86400000)), localTS(new Date(d.getTime() + 2 * 86400000))])
  }, [gid]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (game?.status !== 'voting') return
    api.get<GameDetail>(`${V1}/games/${game.id}`).then(d => setOptions(d.options.map(o => o.starts_at))).catch(() => {})
  }, [game])
  if (!g) return null
  const set = (p: Partial<typeof f>) => setF({ ...f, ...p })

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    const body: Record<string, unknown> = {
      title: f.title, notes: f.notes, duration_min: f.duration_min, venue_name: f.venue_name, venue_address: f.venue_address, venue_lat: f.venue_lat, venue_lon: f.venue_lon,
      min_players: f.min_players, max_players: Math.max(0, f.max_players), cost_cents: Math.round((Number(String(f.cost).replace(',', '.')) || 0) * 100), currency: f.currency,
    }
    if (mode === 'vote') { body.options = options.filter(Boolean); body.vote_until = f.vote_until }
    else if (game?.status === 'voting') { body.options = [f.starts_at] }
    else body.starts_at = f.starts_at
    if (!game) { body.im_in = f.im_in; body.booking_task = f.booking_task && tasks.ok && mode === 'time' }
    try {
      const d = game ? await api.put<GameDetail>(`${V1}/games/${game.id}`, body) : await api.post<GameDetail>(`${V1}/groups/${gid}/games`, body)
      announceChange(); toast(game ? 'Saved' : mode === 'vote' ? 'Sent round: everyone can vote' : 'Game planned'); onSaved(d.game.id)
    } catch (err) { toast((err as Error).message, 'err') } finally { setBusy(false) }
  }

  return (
    <Modal title={game ? 'Edit the game' : 'Plan a game'} onClose={onClose} size="lg">
      <form onSubmit={save} className="stack loose">
        {!game && live.length > 1 && <Field label="Group"><Select ariaLabel="Group" value={String(gid)} onChange={v => setGid(Number(v))} options={live.map(x => ({ value: String(x.id), label: `${x.sport_info?.emoji || ''} ${x.name}` }))} /></Field>}
        {(!game || game.status === 'voting') && <Segmented value={mode} onChange={setMode} options={[{ value: 'time', label: game ? 'Settle on one time' : 'At a set time' }, { value: 'vote', label: 'Let everyone vote on a time' }]} />}
        {mode === 'time' ? (
          <Field label="When"><input type="datetime-local" value={f.starts_at} onChange={e => set({ starts_at: e.target.value })} required /></Field>
        ) : (
          <div className="form-sec">
            <h4 className="form-sec-h">Times to vote on</h4>
            {options.map((o, i) => (
              <div key={i} className="row tight">
                <input type="datetime-local" value={o} onChange={e => setOptions(os => os.map((x, j) => j === i ? e.target.value : x))} aria-label={`Time ${i + 1}`} />
                <button type="button" className="btn ghost icon-only" aria-label="Remove this time" disabled={options.length <= 2} onClick={() => setOptions(os => os.filter((_, j) => j !== i))}><Icon name="x" size={15} /></button>
              </div>
            ))}
            <button type="button" className="btn sm self-start" disabled={options.length >= 12} onClick={() => setOptions(os => [...os, os[os.length - 1] ? localTS(new Date(new Date(os[os.length - 1]).getTime() + 86400000)) : nextSlot(g)])}><Icon name="plus" size={14} />Another time</button>
            <Field label="Answer by" help="Then League picks the time most people can make, and tells everyone. Empty: you pick it.">
              <input type="datetime-local" value={f.vote_until} onChange={e => set({ vote_until: e.target.value })} />
            </Field>
          </div>
        )}
        <div className="form-sec">
          <h4 className="form-sec-h">Where</h4>
          <VenuePicker value={{ venue_name: f.venue_name || '' }} onPick={v => set(v)} />
          <div className="two-col tight">
            <Field label="Venue"><input value={f.venue_name || ''} onChange={e => set({ venue_name: e.target.value })} maxLength={120} /></Field>
            <Field label="Address"><input value={f.venue_address || ''} onChange={e => set({ venue_address: e.target.value })} maxLength={300} /></Field>
          </div>
        </div>
        <div className="three-col tight">
          <Field label="At least"><input type="number" min={1} max={200} value={f.min_players} onChange={e => set({ min_players: Math.max(1, Number(e.target.value) || 1) })} /></Field>
          <Field label="At most" help="0 = no limit"><input type="number" min={0} max={200} value={Math.max(0, f.max_players)} onChange={e => set({ max_players: Math.max(0, Number(e.target.value) || 0) })} /></Field>
          <Field label="Cost"><div className="input-group"><input inputMode="decimal" value={f.cost} onChange={e => set({ cost: e.target.value })} placeholder="0" />
            <Select ariaLabel="Currency" value={f.currency} onChange={v => set({ currency: v })} options={Array.from(new Set([f.currency, ...CURRENCIES])).filter(Boolean).map(c => ({ value: c, label: c === 'BAM' ? 'KM' : c }))} /></div></Field>
        </div>
        <div className="two-col tight">
          <Field label="Title (optional)"><input value={f.title} onChange={e => set({ title: e.target.value })} placeholder="Final of the month" maxLength={100} /></Field>
          <Field label="Lasts"><Select ariaLabel="Length" value={String(f.duration_min)} onChange={v => set({ duration_min: Number(v) })}
            options={[30, 45, 60, 75, 90, 105, 120, 150, 180, 240].concat(f.duration_min || 90).filter((x, i, a) => a.indexOf(x) === i).sort((a, b) => a - b).map(m => ({ value: String(m), label: m < 60 ? `${m} min` : `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ''}` }))} /></Field>
        </div>
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => set({ notes: e.target.value })} placeholder="Court 3, bring both shirts…" maxLength={2000} /></Field>
        {!game && <div className="stack tight">
          {mode === 'time' && <label className="check-row"><Switch checked={f.im_in} onChange={v => set({ im_in: v })} label="I'm in" /> Count me in</label>}
          {mode === 'time' && <label className={`check-row ${tasks.ok ? '' : 'disabled'}`} title={tasks.ok ? undefined : offTitle(tasks.app, 'tasks')}>
            <Switch checked={f.booking_task && tasks.ok} onChange={v => set({ booking_task: v })} label="Book the court" disabled={!tasks.ok} /> Remind me to book the court (Pocket Tasks)
          </label>}
        </div>}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{game ? 'Save' : mode === 'vote' ? 'Ask everyone' : 'Plan it'}</button></div>
      </form>
    </Modal>
  )
}
