// Adding people to a group and getting them in: an account is made for
// anyone new right away, and they get a one-time sign-in link — by email,
// WhatsApp, SMS, or copied anywhere.
import { useEffect, useRef, useState } from 'react'
import { api, upload, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState } from '../lib/data'
import type { Group, LinkResult, Member } from '../lib/types'
import { offTitle } from './Apps'
import { Icon } from './icons'
import { Field, Modal, Switch, useDebounce, useToast } from './ui'

type Contact = { id: number; name: string; email: string; phone: string; org: string }

export function AddPerson({ group, onClose, onAdded }: { group: Group; onClose: () => void; onAdded: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const contacts = useAppState()('contacts', 'contacts:read')
  const [f, setF] = useState({ name: '', email: '', phone: '', role: 'player' })
  const [send, setSend] = useState(settings?.email_via !== 'none')
  const [busy, setBusy] = useState(false)
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [found, setFound] = useState<Contact[]>([])
  const [link, setLink] = useState<{ who: string; res?: LinkResult; note?: string; member: Member } | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (!contacts.ok || dq.trim().length < 2) { setFound([]); return }
    let live = true
    api.get<Contact[]>(`${V1}/apps/contacts?q=${encodeURIComponent(dq)}`).then(r => { if (live) setFound(r) }).catch(() => setFound([]))
    return () => { live = false }
  }, [dq, contacts.ok])

  const add = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const r = await api.post<{ member: Member; created: boolean; note?: string; link?: LinkResult }>(`${V1}/groups/${group.id}/members`, { ...f, send: send && !!f.email })
      onAdded(); announceChange()
      if (r.member.can_link) {
        const res = r.link || await api.post<LinkResult>(`${V1}/groups/${group.id}/members/${r.member.user_id}/link`, { email: false })
        setLink({ who: r.member.name, res, member: r.member })
      } else {
        toast(r.note || `${r.member.name} is in`)
        onClose()
      }
    } catch (err) { toast((err as Error).message, 'err') } finally { setBusy(false) }
  }
  const importCSV = async (file: File) => {
    try {
      const r = await upload<{ added: number; new_accounts: number; skipped: number }>(`${V1}/groups/${group.id}/import-players`, 'file', file)
      toast(`${r.added} added${r.skipped ? `, ${r.skipped} already in or skipped` : ''}`); onAdded(); announceChange(); onClose()
    } catch (err) { toast((err as Error).message, 'err') }
  }

  if (link) return <LinkSheet group={group} who={link.who} member={link.member} initial={link.res} onClose={onClose} />
  return (
    <Modal title={`Add someone to ${group.name}`} onClose={onClose}>
      <form onSubmit={add} className="stack loose">
        <div className="picker">
          <input value={q} onChange={e => setQ(e.target.value)} placeholder="Find them in your contacts" disabled={!contacts.ok} title={contacts.ok ? undefined : offTitle(contacts.app, 'contacts')} aria-label="Find in your contacts" />
          {!contacts.ok && !contacts.loading && <p className="muted small">Pick from Pocket Contacts once it is connected (Settings → Connected apps). Or type them in:</p>}
          {found.length > 0 && <div className="pick-list">{found.map(c => (
            <button key={c.id} type="button" className="pick-row" onClick={() => { setF({ ...f, name: c.name, email: c.email || '', phone: c.phone || '' }); setQ(''); setFound([]) }}>
              <Icon name="user" size={15} /><span className="grow"><span className="block-line">{c.name}</span><span className="muted small">{[c.email, c.phone].filter(Boolean).join(' · ')}</span></span>
            </button>
          ))}</div>}
        </div>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required maxLength={80} autoFocus placeholder="Marko Petrović" /></Field>
        <div className="two-col tight">
          <Field label="Email" help="For the sign-in link and reminders."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} placeholder="marko@example.com" /></Field>
          <Field label="Phone" help="For WhatsApp or SMS."><input type="tel" value={f.phone} onChange={e => setF({ ...f, phone: e.target.value })} placeholder="+387 61 123 456" /></Field>
        </div>
        <label className="check-row"><Switch checked={f.role === 'organiser'} onChange={v => setF({ ...f, role: v ? 'organiser' : 'player' })} label="Organiser" /> Organiser too — can plan for everyone, change the group and add people</label>
        <label className={`check-row ${settings?.email_via === 'none' || !f.email ? 'disabled' : ''}`}>
          <Switch checked={send && !!f.email && settings?.email_via !== 'none'} onChange={setSend} label="Email the link" disabled={settings?.email_via === 'none' || !f.email} />
          <span>Email them their sign-in link now{settings?.email_via === 'none' ? ' (email is off: Settings → Email)' : ''}</span>
        </label>
        <p className="muted small">Nobody here with that email or phone? An account is made for them on the spot. They come in with a one-time link — no password, nothing to install.</p>
        <div className="form-actions">
          <button type="button" className="btn ghost" onClick={() => fileRef.current?.click()} title="A CSV with name, email and phone — from Spond, TeamSnap, Heja or a spreadsheet"><Icon name="upload" size={15} />A list (CSV)…</button>
          <input ref={fileRef} type="file" accept=".csv,text/csv" hidden onChange={e => { const file = e.target.files?.[0]; if (file) importCSV(file); e.target.value = '' }} />
          <span className="spacer" />
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy || (!f.email && !f.phone)}>Add</button>
        </div>
      </form>
    </Modal>
  )
}

/** A fresh sign-in link for a player and the ways to send it. */
export function LinkSheet({ group, member, who, initial, onClose }: { group: Group; member: Member; who: string; initial?: LinkResult; onClose: () => void }) {
  const { toast } = useToast()
  const { settings } = useApp()
  const [res, setRes] = useState<LinkResult | null>(initial || null)
  const [busy, setBusy] = useState(false)
  const make = async (email: boolean) => {
    setBusy(true)
    try {
      const r = await api.post<LinkResult>(`${V1}/groups/${group.id}/members/${member.user_id}/link`, { email })
      setRes(r)
      if (email) toast(r.emailed ? `Emailed to ${member.email}` : `Not emailed: ${r.email_error}`, r.emailed ? 'ok' : 'err')
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  useEffect(() => { if (!res) make(false) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const copy = async () => { try { await navigator.clipboard.writeText(res!.text); toast('Copied — paste it anywhere') } catch { toast('Copy it from the box', 'err') } }
  const share = async () => { try { await navigator.share({ title: `Join ${group.name}`, text: res!.text }) } catch { /* closed */ } }
  const canShare = typeof navigator !== 'undefined' && 'share' in navigator
  return (
    <Modal title={`Get ${who.split(' ')[0]} in`} onClose={onClose}>
      <div className="stack loose">
        <p className="muted">{who} has an account for this group. Send them this link: one tap and they are in. It works once, for 14 days; a new link stops the old one.</p>
        {res ? <>
          <div className="link-box"><span className="mono small">{res.url}</span></div>
          {res.emailed && <div className="callout success"><Icon name="check-circle" size={15} /><span>Emailed to {member.email}.</span></div>}
          {res.email_error && <div className="callout warn"><Icon name="alert" size={15} /><span>Not emailed: {res.email_error}</span></div>}
          <div className="share-grid">
            <a className="btn share-wa" href={res.whatsapp} target="_blank" rel="noreferrer"><Icon name="message" size={16} />WhatsApp</a>
            {res.sms && <a className="btn" href={res.sms}><Icon name="smartphone" size={16} />SMS</a>}
            <button type="button" className="btn" onClick={() => make(true)} disabled={busy || !member.email || settings?.email_via === 'none'}
              title={!member.email ? 'No email address' : settings?.email_via === 'none' ? 'Email is off: Settings → Email' : undefined}><Icon name="mail" size={16} />Email it</button>
            {canShare && <button type="button" className="btn" onClick={share}><Icon name="share" size={16} />Share…</button>}
            <button type="button" className="btn" onClick={copy}><Icon name="copy" size={16} />Copy the message</button>
          </div>
          <details className="muted small"><summary>The message</summary><p className="pre">{res.text}</p></details>
        </> : <p className="muted">Making the link…</p>}
        <div className="form-actions"><button type="button" className="btn ghost" onClick={() => make(false)} disabled={busy}><Icon name="refresh" size={15} />A new link</button><span className="spacer" /><button type="button" className="btn primary" onClick={onClose}>Done</button></div>
      </div>
    </Modal>
  )
}

/** Edit how someone is in the group (organisers). */
export function EditMember({ group, member, onClose, onSaved }: { group: Group; member: Member; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ nickname: member.nickname, role: member.role, name: member.full_name, email: member.email || '', phone: member.phone || '' })
  const ownAccount = member.account === 'player' && member.can_link
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    const body: Record<string, unknown> = { nickname: f.nickname, role: f.role }
    if (ownAccount) Object.assign(body, { name: f.name, email: f.email, phone: f.phone })
    try { await api.put(`${V1}/groups/${group.id}/members/${member.user_id}`, body); toast('Saved'); onSaved() } catch (err) { toast((err as Error).message, 'err') }
  }
  return (
    <Modal title={member.name} onClose={onClose} size="sm">
      <form onSubmit={save} className="stack loose">
        {ownAccount && <>
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required maxLength={80} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
          <Field label="Phone"><input type="tel" value={f.phone} onChange={e => setF({ ...f, phone: e.target.value })} /></Field>
        </>}
        <Field label="Called in this group" help="A nickname everyone sees here."><input value={f.nickname} onChange={e => setF({ ...f, nickname: e.target.value })} maxLength={40} placeholder={member.full_name} /></Field>
        <label className="check-row"><Switch checked={f.role === 'organiser'} onChange={v => setF({ ...f, role: v ? 'organiser' : 'player' })} label="Organiser" /> Organiser</label>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Save</button></div>
      </form>
    </Modal>
  )
}
