// /join/<token>: a player's sign-in link. It shows who invited them to
// what; "Come in" uses the link up (a link preview in a chat app cannot).
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { Icon } from '../components/icons'
import { Field, Loading } from '../components/ui'

interface Peek { name: string; invited_by?: string; expires_at: string; group?: { name: string; sport: string; emoji: string; color: string } }

export default function Join() {
  const { token } = useParams()
  const { refresh, user } = useApp()
  const navigate = useNavigate()
  const [peek, setPeek] = useState<Peek | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState('')
  useEffect(() => { api.get<Peek>(`${V1}/auth/link/${token}`).then(setPeek).catch(e => setErr((e as Error).message)) }, [token])
  const come = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ group_id: number }>(`${V1}/auth/link`, { token })
      await refresh()
      navigate(r.group_id ? `/groups/${r.group_id}` : '/', { replace: true })
    } catch (e) { setErr((e as Error).message); setBusy(false) }
  }
  const ask = async (e: React.FormEvent) => {
    e.preventDefault()
    try { setSent((await api.post<{ message: string }>(`${V1}/auth/link/request`, { email })).message) } catch (er) { setSent((er as Error).message) }
  }
  return (
    <div className="login-wrap">
      <div className="login-card">
        <div className="login-brand">
          <img src="/favicon.svg" alt="" width={44} height={44} />
          {peek?.group ? <>
            <h1><span aria-hidden="true">{peek.group.emoji}</span> {peek.group.name}</h1>
            <p className="login-tagline">Hi {peek.name.split(' ')[0]}! {peek.invited_by ? `${peek.invited_by} added you` : 'You were added'} to this {peek.group.sport.toLowerCase()} group: the games, who's coming, the scores and the chat.</p>
          </> : <h1>Pocket League</h1>}
        </div>
        {!peek && !err && <Loading />}
        {peek && <>
          <button type="button" className="btn primary lg block" onClick={come} disabled={busy}><Icon name="check" size={16} />{busy ? 'Coming in…' : 'Come in'}</button>
          <p className="login-note">No password, nothing to install. This link works once; on this phone or computer you stay signed in.{user ? ' (You are signed in already: coming in switches to this player.)' : ''}</p>
        </>}
        {err && <>
          <div className="callout warn"><Icon name="alert" size={15} /><span>{err}</span></div>
          {sent ? <p className="login-note">{sent}</p> : <form onSubmit={ask} className="stack">
            <Field label="Your email"><input type="email" value={email} onChange={e => setEmail(e.target.value)} required autoComplete="email" /></Field>
            <button className="btn block">Email me a new link</button>
          </form>}
        </>}
      </div>
    </div>
  )
}
