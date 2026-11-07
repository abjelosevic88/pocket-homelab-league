// A group: the sport and how it is played, where, the usual day, how many
// players, what a game costs. New or edited (organisers).
import { useEffect, useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useData } from '../lib/data'
import { CURRENCIES, FORMAT_WORDS, SCORING_WORDS, WEEKDAYS } from '../lib/league'
import type { Format, Group, Scoring, Sport } from '../lib/types'
import { offTitle } from './Apps'
import { Icon } from './icons'
import { Select } from './Select'
import { Confirm, Field, Modal, Segmented, Switch, useDebounce, useToast } from './ui'

type Form = {
  name: string; sport: string; sport_name: string; scoring: Scoring; format: Format; lower_better: boolean; description: string
  venue_name: string; venue_address: string; venue_lat: number | null; venue_lon: number | null; venue_ref: string; outdoor: boolean
  weekday: number; time: string; duration_min: number; plan_days: number; min_players: number; max_players: number
  cost: string; currency: string; win_points: number; draw_points: number; loss_points: number; email_players: boolean
}

function fromGroup(g: Group): Form {
  return { name: g.name, sport: g.sport, sport_name: g.sport_name, scoring: g.scoring, format: g.format, lower_better: g.lower_better, description: g.description,
    venue_name: g.venue_name, venue_address: g.venue_address, venue_lat: g.venue_lat, venue_lon: g.venue_lon, venue_ref: g.venue_ref, outdoor: g.outdoor,
    weekday: g.weekday, time: g.time, duration_min: g.duration_min, plan_days: g.plan_days, min_players: g.min_players, max_players: g.max_players,
    cost: g.cost_cents ? String(g.cost_cents / 100) : '', currency: g.currency, win_points: g.win_points, draw_points: g.draw_points, loss_points: g.loss_points, email_players: g.email_players }
}

function withSport(f: Form, s: Sport): Form {
  return { ...f, sport: s.key, scoring: s.scoring, format: s.format, lower_better: s.lower_better, outdoor: s.outdoor, min_players: s.min_players, max_players: s.max_players,
    duration_min: s.minutes, win_points: s.draws ? 3 : 1, draw_points: s.draws ? 1 : 0, loss_points: 0 }
}

export function GroupForm({ group, onClose, onSaved }: { group?: Group; onClose: () => void; onSaved: (g: Group) => void }) {
  const { sports } = useData()
  const { settings } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<Form | null>(group ? fromGroup(group) : null)
  const [busy, setBusy] = useState(false)
  const [more, setMore] = useState(!!group)
  const [del, setDel] = useState(false)
  useEffect(() => {
    if (!f && sports.length) {
      const blank: Form = { name: '', sport: '', sport_name: '', scoring: 'sets', format: 'singles', lower_better: false, description: '', venue_name: '', venue_address: '',
        venue_lat: null, venue_lon: null, venue_ref: '', outdoor: false, weekday: 0, time: '19:00', duration_min: 90, plan_days: 0, min_players: 2, max_players: 0,
        cost: '', currency: settings?.currency || 'EUR', win_points: 3, draw_points: 1, loss_points: 0, email_players: true }
      setF(blank)
    }
  }, [sports, f, settings?.currency])
  const sport = sports.find(s => s.key === f?.sport)
  if (!f) return null
  const set = (p: Partial<Form>) => setF({ ...f, ...p })
  const pickSport = (s: Sport) => setF(withSport(f, s))

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!f.sport) { toast('Pick a sport', 'err'); return }
    setBusy(true)
    const body = { ...f, cost_cents: Math.round((Number(f.cost.replace(',', '.')) || 0) * 100), cost: undefined, plan_days: f.weekday ? f.plan_days : 0 }
    try {
      const g = group ? await api.put<Group>(`${V1}/groups/${group.id}`, body) : await api.post<Group>(`${V1}/groups`, body)
      announceChange(); toast(group ? 'Saved' : 'Group made'); onSaved(g)
    } catch (err) { toast((err as Error).message, 'err') } finally { setBusy(false) }
  }
  const archive = async () => {
    if (!group) return
    try { const g = await api.put<Group>(`${V1}/groups/${group.id}`, { archived: !group.archived }); announceChange(); toast(g.archived ? 'Archived: it stays in your results' : 'Back in your groups'); onSaved(g) } catch (err) { toast((err as Error).message, 'err') }
  }
  const remove = async () => {
    if (!group) return
    try { await api.del(`${V1}/groups/${group.id}`); announceChange(); toast('Group deleted'); onClose(); window.location.assign('/') } catch (err) { toast((err as Error).message, 'err') }
  }

  return (
    <Modal title={group ? `Edit ${group.name}` : 'A new group'} onClose={onClose} size="lg">
      <form onSubmit={save} className="stack loose">
        <div className="form-sec">
          <h4 className="form-sec-h">The sport</h4>
          <div className="sport-grid" role="radiogroup" aria-label="Sport">
            {sports.map(s => (
              <button key={s.key} type="button" role="radio" aria-checked={f.sport === s.key} className={`sport-pick ${f.sport === s.key ? 'on' : ''}`} onClick={() => pickSport(s)}
                style={{ ['--sp' as string]: s.color }}>
                <span className="sport-emoji" aria-hidden="true">{s.emoji}</span><span className="ellipsis">{s.name}</span>
              </button>
            ))}
          </div>
          {f.sport === 'other' && <div className="two-col tight">
            <Field label="Which sport"><input value={f.sport_name} onChange={e => set({ sport_name: e.target.value })} placeholder="Bocce, frisbee, spikeball…" maxLength={40} /></Field>
            <Field label="How you count"><Select ariaLabel="Scoring" value={f.scoring} onChange={v => set({ scoring: v as Scoring, format: v === 'ranking' ? 'everyone' : f.format === 'everyone' ? 'singles' : f.format })}
              options={(Object.keys(SCORING_WORDS) as Scoring[]).map(k => ({ value: k, label: SCORING_WORDS[k] }))} /></Field>
          </div>}
          {sport && sport.formats.length > 1 && f.scoring !== 'ranking' && <Field label="How you play" group>
            <Segmented value={f.format} onChange={v => set({ format: v, ...(v === 'doubles' ? { min_players: 4, max_players: Math.max(f.max_players, 4) } : {}) })}
              options={sport.formats.filter(x => x !== 'everyone' || f.sport === 'other').map(x => ({ value: x, label: FORMAT_WORDS[x] }))} />
          </Field>}
          {sport && <p className="muted small">{SCORING_WORDS[f.scoring]}{f.scoring === 'ranking' ? (f.lower_better ? ' — the lowest wins.' : ' — the highest wins.') : '.'} Everyone gets a rating that moves with each result.</p>}
          {f.sport === 'other' && f.scoring === 'ranking' && <label className="check-row"><Switch checked={f.lower_better} onChange={v => set({ lower_better: v })} label="Lowest wins" /> The lowest score wins (like golf)</label>}
        </div>

        <Field label="Name"><input value={f.name} onChange={e => set({ name: e.target.value })} required maxLength={80} placeholder={sport ? `${['Tuesday', 'Friday', 'Sunday'][sports.indexOf(sport) % 3]} ${sport.name}` : 'Tuesday Tennis'} autoFocus={!group} /></Field>

        <div className="form-sec">
          <h4 className="form-sec-h">Where</h4>
          <VenuePicker value={f} onPick={v => set(v)} />
          <div className="two-col tight">
            <Field label="Venue"><input value={f.venue_name} onChange={e => set({ venue_name: e.target.value })} placeholder="The club, the pitch" maxLength={120} /></Field>
            <Field label="Address"><input value={f.venue_address} onChange={e => set({ venue_address: e.target.value })} maxLength={300} /></Field>
          </div>
          <label className="check-row"><Switch checked={f.outdoor} onChange={v => set({ outdoor: v })} label="Outdoors" /> Outdoors — show the weather for each game</label>
        </div>

        <div className="form-sec">
          <h4 className="form-sec-h">When you usually play</h4>
          <div className="three-col tight">
            <Field label="Day"><Select ariaLabel="Day" value={String(f.weekday)} onChange={v => set({ weekday: Number(v) })} options={WEEKDAYS.map((d, i) => ({ value: String(i), label: i ? d : 'No usual day' }))} /></Field>
            <Field label="Time"><input type="time" value={f.time} onChange={e => set({ time: e.target.value })} disabled={!f.weekday} /></Field>
            <Field label="A game lasts"><Select ariaLabel="Length" value={String(f.duration_min)} onChange={v => set({ duration_min: Number(v) })}
              options={[30, 45, 60, 75, 90, 105, 120, 150, 180, 240].concat(f.duration_min).filter((x, i, a) => a.indexOf(x) === i).sort((a, b) => a - b).map(m => ({ value: String(m), label: m < 60 ? `${m} min` : `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ''}` }))} /></Field>
          </div>
          {f.weekday > 0 && <Field label="Plan the usual game by itself" help="It shows up for everyone, who then say whether they are in.">
            <Select ariaLabel="Plan ahead" value={String(f.plan_days)} onChange={v => set({ plan_days: Number(v) })}
              options={[{ value: '0', label: 'No, I plan each game' }, ...[1, 2, 3, 4, 5, 6, 7, 10, 14].map(d => ({ value: String(d), label: `${d} ${d === 1 ? 'day' : 'days'} before` }))]} />
          </Field>}
        </div>

        <div className="form-sec">
          <h4 className="form-sec-h">Players and cost</h4>
          <div className="three-col tight">
            <Field label="At least" help="Fewer: the organisers hear it's short."><input type="number" min={1} max={200} value={f.min_players} onChange={e => set({ min_players: Math.max(1, Number(e.target.value) || 1) })} /></Field>
            <Field label="At most" help="More: a waiting list. 0 = no limit."><input type="number" min={0} max={200} value={f.max_players} onChange={e => set({ max_players: Math.max(0, Number(e.target.value) || 0) })} /></Field>
            <Field label="A game costs" help="The court, the pitch — shared by who's in.">
              <div className="input-group"><input inputMode="decimal" value={f.cost} onChange={e => set({ cost: e.target.value })} placeholder="0" />
                <Select ariaLabel="Currency" value={f.currency} onChange={v => set({ currency: v })} options={Array.from(new Set([f.currency, ...CURRENCIES])).map(c => ({ value: c, label: c === 'BAM' ? 'KM' : c }))} /></div>
            </Field>
          </div>
        </div>

        <button type="button" className="linkish self-start" onClick={() => setMore(m => !m)} aria-expanded={more}><Icon name={more ? 'chevron-up' : 'chevron-down'} size={14} /> {more ? 'Less' : 'More: the table, emails, a description'}</button>
        {more && <div className="form-sec">
          {f.scoring !== 'ranking' && <div className="three-col tight">
            <Field label="Points for a win"><input type="number" min={0} max={10} value={f.win_points} onChange={e => set({ win_points: Number(e.target.value) || 0 })} /></Field>
            <Field label="For a draw"><input type="number" min={0} max={10} value={f.draw_points} onChange={e => set({ draw_points: Number(e.target.value) || 0 })} /></Field>
            <Field label="For a loss"><input type="number" min={0} max={10} value={f.loss_points} onChange={e => set({ loss_points: Number(e.target.value) || 0 })} /></Field>
          </div>}
          <label className="check-row"><Switch checked={f.email_players} onChange={v => set({ email_players: v })} label="Email players" /> <span>Email the players who have no notifications set up: new games, reminders, "we're short". Each player can turn it off.</span></label>
          <Field label="About the group"><textarea rows={3} value={f.description} onChange={e => set({ description: e.target.value })} placeholder="House rules, who brings what…" maxLength={2000} /></Field>
        </div>}

        <div className="form-actions">
          {group && group.my_role === 'organiser' && <button type="button" className="btn ghost" onClick={archive}><Icon name="archive" size={15} />{group.archived ? 'Bring back' : 'Archive'}</button>}
          {group && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>}
          <span className="spacer" />
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy || !f.sport}>{group ? 'Save' : 'Make the group'}</button>
        </div>
      </form>
      {del && group && <Confirm title={`Delete ${group.name}?`} confirmLabel="Delete for everyone" message="The games, results, table and chat go for everyone in it. Archive it instead to keep the results." onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}

/** Pick the venue from Pocket Places (greyed with a word when it is not connected). */
export function VenuePicker({ value, onPick }: { value: { venue_name: string }; onPick: (v: { venue_name: string; venue_address: string; venue_lat: number | null; venue_lon: number | null; venue_ref: string }) => void }) {
  const st = useAppState()('places', 'places:read')
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<{ id: number; name: string; address: string; lat: number | null; lon: number | null }[]>([])
  useEffect(() => {
    if (!open || !st.ok) return
    let live = true
    api.get<typeof list>(`${V1}/apps/places?q=${encodeURIComponent(dq)}`).then(r => { if (live) setList(r) }).catch(() => setList([]))
    return () => { live = false }
  }, [dq, open, st.ok])
  const label = useMemo(() => value.venue_name ? 'Pick another from Pocket Places' : 'Pick from Pocket Places', [value.venue_name])
  if (!open) return <button type="button" className="btn sm self-start" disabled={!st.ok} title={st.ok ? undefined : offTitle(st.app, 'places')} onClick={() => setOpen(true)}><Icon name="pin" size={14} />{label}</button>
  return (
    <div className="picker">
      <div className="row tight"><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Search your places" aria-label="Search your places" /><button type="button" className="btn ghost" onClick={() => setOpen(false)}>Close</button></div>
      <div className="pick-list">
        {list.length === 0 && <p className="muted small" style={{ padding: 8 }}>No places match.</p>}
        {list.map(p => (
          <button key={p.id} type="button" className="pick-row" onClick={() => { onPick({ venue_name: p.name, venue_address: p.address, venue_lat: p.lat, venue_lon: p.lon, venue_ref: String(p.id) }); setOpen(false) }}>
            <Icon name="pin" size={15} /><span className="grow"><span className="block-line">{p.name}</span><span className="muted small">{p.address}</span></span>
          </button>
        ))}
      </div>
    </div>
  )
}
