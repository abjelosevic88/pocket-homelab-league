// Away days (holidays, an injury): games on those days are "out" for you
// and you aren't asked; organisers see it when they plan.
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtDate } from '../lib/format'
import type { Away } from '../lib/types'
import { Icon } from './icons'
import { Empty, Loading, useAsync, useToast } from './ui'

export function AwayDays() {
  const { toast } = useToast()
  const { data, reload } = useAsync(() => api.get<Away[]>(`${V1}/away`), [])
  const today = new Date().toISOString().slice(0, 10)
  const [from, setFrom] = useState(today)
  const [to, setTo] = useState(today)
  const [note, setNote] = useState('')
  const add = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      const r = await api.post<{ games_out: number }>(`${V1}/away`, { from, to: to < from ? from : to, note })
      toast(r.games_out ? `Saved — out for ${r.games_out} ${r.games_out === 1 ? 'game' : 'games'} already planned` : 'Saved'); setNote(''); reload()
    } catch (err) { toast((err as Error).message, 'err') }
  }
  const del = async (a: Away) => { try { await api.del(`${V1}/away/${a.id}`); reload() } catch (e) { toast((e as Error).message, 'err') } }
  if (!data) return <Loading />
  return (
    <div className="stack loose">
      <form className="set-section stack away-form" onSubmit={add}>
        <div className="row wrap">
          <label className="field-inline"><span className="small muted">From</span><input type="date" value={from} min={today} onChange={e => { setFrom(e.target.value); if (to < e.target.value) setTo(e.target.value) }} required /></label>
          <label className="field-inline"><span className="small muted">To</span><input type="date" value={to} min={from} onChange={e => setTo(e.target.value)} required /></label>
          <input className="grow" value={note} onChange={e => setNote(e.target.value)} placeholder="Why? (holidays, injured …) — optional" maxLength={80} />
          <button className="btn primary"><Icon name="plus" size={15} />Add</button>
        </div>
        <p className="muted small">Games on those days are “out” for you and you aren't asked. Organisers see it when they find a time.</p>
      </form>
      {data.length === 0 ? <Empty icon="plane" title="Not away">When you can't play for a while, add the days here.</Empty> : (
        <div className="card flush">{data.map(a => (
          <div key={a.id} className="duty-row">
            <Icon name="plane" size={16} />
            <span className="grow min0"><span className="block-line">{a.from === a.to ? fmtDate(a.from) : `${fmtDate(a.from)} – ${fmtDate(a.to)}`}</span>{a.note && <span className="muted small block-line">{a.note}</span>}</span>
            <button type="button" className="btn ghost icon-only sm" aria-label="Remove" onClick={() => del(a)}><Icon name="trash" size={14} /></button>
          </div>
        ))}</div>
      )}
    </div>
  )
}
