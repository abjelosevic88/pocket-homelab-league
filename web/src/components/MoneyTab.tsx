// Who paid, who owes: a group's accounts — shares of the court after each
// game, tournament entry fees, payments (said by a player, confirmed by an
// organiser) and how to pay.
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useLiveEvents } from '../lib/data'
import { fmtDate } from '../lib/format'
import { money } from '../lib/league'
import type { Group } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Avatar, Empty, Loading, Modal, Segmented, useToast } from './ui'

interface Balance { user_id: number; name: string; charged: number; paid: number; pending: number; owes: number }
interface Entry { id: number; user_id: number; name: string; kind: 'charge' | 'payment'; amount_cents: number; currency: string; game_id: number; tournament_id: number; note: string; by_name: string; confirmed_at: string | null; created_at: string; game_at?: string; tournament?: string }
interface Money { currency: string; pay_note: string; charge_games: boolean; balances: Balance[]; entries: Entry[]; total_charged: number; outstanding: number; can_edit: boolean }

export function MoneyTab({ g, onEditGroup }: { g: Group; onEditGroup: () => void }) {
  const { user } = useApp()
  const { toast } = useToast()
  const [m, setM] = useState<Money | null>(null)
  const [add, setAdd] = useState<{ kind: 'charge' | 'payment'; user_id: number; amount: string; note: string } | null>(null)
  const load = useCallback(() => api.get<Money>(`${V1}/groups/${g.id}/money`).then(setM).catch(() => setM(null)), [g.id])
  useEffect(() => { load() }, [load])
  useLiveEvents(e => { if (e.type === 'group' && (e.data as { id?: number }).id === g.id) load() })
  if (!m) return <Loading />
  const me = user?.id || 0
  const mine = m.balances.find(b => b.user_id === me)
  const cur = m.currency
  const act = async (fn: () => Promise<Money>, ok?: string) => { try { setM(await fn()); if (ok) toast(ok) } catch (e) { toast((e as Error).message, 'err') } }
  const save = (e: React.FormEvent) => {
    e.preventDefault()
    if (!add) return
    const cents = Math.round((Number(add.amount.replace(',', '.')) || 0) * 100)
    act(() => api.post<Money>(`${V1}/groups/${g.id}/money`, { kind: add.kind, user_id: add.user_id, amount_cents: cents, note: add.note }), m.can_edit ? 'Booked' : 'Sent — an organiser confirms it when it arrives')
    setAdd(null)
  }
  const pending = m.entries.filter(e => e.kind === 'payment' && !e.confirmed_at)
  const nothing = m.entries.length === 0 && m.balances.every(b => !b.charged && !b.paid)
  return (
    <div className="stack loose">
      <div className="money-head card pad-card">
        <div className="grow min0">
          <div className="muted small">{mine && mine.owes < 0 ? 'You are ahead' : 'You owe'}</div>
          <div className={`money-big ${mine && mine.owes > 0 ? 'owe' : ''}`}>{money(Math.abs(mine?.owes || 0), cur)}</div>
          {mine && mine.pending > 0 && <div className="muted small">{money(mine.pending, cur)} paid, waiting for an organiser</div>}
        </div>
        <div className="stack tight money-actions">
          {(mine?.owes || 0) > 0 && <button type="button" className="btn primary" onClick={() => setAdd({ kind: 'payment', user_id: me, amount: String((mine!.owes / 100).toFixed(2)), note: '' })}><Icon name="check" size={15} />I paid</button>}
          {m.can_edit && <button type="button" className="btn" onClick={() => setAdd({ kind: 'payment', user_id: 0, amount: '', note: '' })}><Icon name="plus" size={15} />Book a payment or charge</button>}
        </div>
      </div>
      {m.pay_note ? <div className="card pad-card pay-note"><Icon name="wallet" size={16} /><div className="grow min0"><div className="small muted">How to pay</div><div className="pre">{m.pay_note}</div></div>
        <button type="button" className="btn ghost sm" onClick={() => { navigator.clipboard?.writeText(m.pay_note); toast('Copied') }}><Icon name="copy" size={14} />Copy</button></div>
        : m.can_edit && <p className="muted small">Add how to pay (an IBAN, a Revolut link) in <button type="button" className="linkish" onClick={onEditGroup}>the group's settings</button>.</p>}

      {m.can_edit && pending.length > 0 && <section className="stack">
        <div className="sec-head"><Icon name="clock" size={16} /><h3>To confirm</h3><span className="count">{pending.length}</span></div>
        <div className="card flush">{pending.map(e => (
          <div key={e.id} className="ledger-row">
            <Avatar name={e.name} size={28} />
            <span className="grow min0"><span className="block-line"><b>{e.name}</b> paid {money(e.amount_cents, e.currency)}</span><span className="muted small block-line">{fmtDate(e.created_at, false)}{e.note ? ` · ${e.note}` : ''}</span></span>
            <button type="button" className="btn sm primary" onClick={() => act(() => api.post<Money>(`${V1}/money/${e.id}/confirm`), 'Confirmed')}>It arrived</button>
            <button type="button" className="btn sm ghost" onClick={() => act(() => api.del<Money>(`${V1}/money/${e.id}`))}>No</button>
          </div>
        ))}</div>
      </section>}

      {nothing ? <Empty icon="wallet" title="No accounts yet">{m.charge_games ? 'After the next game, everyone who was in owes their share here.' : <>Turn on “Keep accounts” in <button type="button" className="linkish" onClick={onEditGroup}>the group's settings</button>, and after each game everyone owes their share of the court.</>}</Empty> : <>
        <section className="stack">
          <div className="sec-head"><Icon name="users" size={16} /><h3>Everyone</h3><span className="muted small">{money(m.outstanding, cur)} still open</span></div>
          <div className="card flush">{m.balances.filter(b => b.charged || b.paid || b.pending).map(b => (
            <div key={b.user_id} className="ledger-row">
              <Avatar name={b.name} size={28} />
              <span className="grow min0"><Link to={`/groups/${g.id}/players/${b.user_id}`} className="block-line">{b.name}{b.user_id === me && <span className="badge">you</span>}</Link>
                <span className="muted small block-line">{money(b.charged, cur)} owed in all · {money(b.paid, cur)} paid{b.pending ? ` · ${money(b.pending, cur)} to confirm` : ''}</span></span>
              <span className={`money-amt ${b.owes > 0 ? 'owe' : b.owes < 0 ? 'ahead' : 'even'}`}>{b.owes === 0 ? 'even' : b.owes > 0 ? money(b.owes, cur) : `+${money(-b.owes, cur)}`}</span>
            </div>
          ))}</div>
        </section>
        <section className="stack">
          <div className="sec-head"><Icon name="receipt" size={16} /><h3>{m.can_edit ? 'The books' : 'Yours'}</h3></div>
          <div className="card flush">{m.entries.map(e => (
            <div key={e.id} className="ledger-row">
              <span className={`ledger-ico ${e.kind}`}><Icon name={e.kind === 'charge' ? 'receipt' : 'check'} size={14} /></span>
              <span className="grow min0"><span className="block-line">{m.can_edit && <b>{e.name}: </b>}{e.note || (e.kind === 'charge' ? 'Charge' : 'Payment')}</span>
                <span className="muted small block-line">{fmtDate(e.created_at, false)}{e.game_id ? <> · <Link to={`/games/${e.game_id}`}>the game</Link></> : ''}{e.tournament_id ? <> · <Link to={`/tournaments/${e.tournament_id}`}>{e.tournament || 'tournament'}</Link></> : ''}{e.kind === 'payment' && !e.confirmed_at ? ' · waiting to be confirmed' : ''}</span></span>
              <span className={`money-amt ${e.kind === 'charge' ? 'owe' : 'ahead'}`}>{e.kind === 'charge' ? '' : '−'}{money(e.amount_cents, e.currency)}</span>
              {(m.can_edit || (e.user_id === me && e.kind === 'payment' && !e.confirmed_at)) && <button type="button" className="btn ghost icon-only sm" aria-label="Delete" onClick={() => act(() => api.del<Money>(`${V1}/money/${e.id}`))}><Icon name="trash" size={14} /></button>}
            </div>
          ))}</div>
        </section>
      </>}

      {add && <Modal title={m.can_edit && add.user_id !== me ? 'Book money' : 'I paid'} onClose={() => setAdd(null)} size="sm">
        <form className="stack" onSubmit={save}>
          {m.can_edit && <>
            <Segmented value={add.kind} onChange={v => setAdd({ ...add, kind: v })} options={[{ value: 'payment', label: 'A payment' }, { value: 'charge', label: 'A charge' }]} />
            <Select ariaLabel="Who" value={add.user_id ? String(add.user_id) : ''} placeholder="Who" onChange={v => setAdd({ ...add, user_id: Number(v) })}
              options={m.balances.map(b => ({ value: String(b.user_id), label: b.name, hint: b.owes ? (b.owes > 0 ? `owes ${money(b.owes, cur)}` : `+${money(-b.owes, cur)}`) : undefined }))} />
          </>}
          <div className="input-group"><input inputMode="decimal" autoFocus value={add.amount} onChange={e => setAdd({ ...add, amount: e.target.value })} placeholder="0.00" aria-label="Amount" /><span className="input-suffix">{cur === 'BAM' ? 'KM' : cur}</span></div>
          <input value={add.note} onChange={e => setAdd({ ...add, note: e.target.value })} placeholder={add.kind === 'charge' ? 'What for (new balls, the bibs …)' : 'How (cash, Revolut …) — optional'} maxLength={200} />
          {!m.can_edit && <p className="muted small">An organiser confirms it when the money has arrived.</p>}
          <div className="form-actions"><button type="button" className="btn" onClick={() => setAdd(null)}>Cancel</button><button className="btn primary" disabled={!(Number(add.amount.replace(',', '.')) > 0) || !add.user_id}>Save</button></div>
        </form>
      </Modal>}
    </div>
  )
}
