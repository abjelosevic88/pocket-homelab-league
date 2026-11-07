// A group's chat, or the comments under one game: live, with reactions,
// your own messages to change or delete, and the lines League adds itself
// (a game planned, a result) in between.
import React, { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData, useLiveEvents } from '../lib/data'
import { fmtDate, fmtTime } from '../lib/format'
import { REACTIONS } from '../lib/league'
import type { Message } from '../lib/types'
import { messagePhotos, uploadPhoto, type Photo } from '../lib/photos'
import { Icon } from './icons'
import { Avatar, Confirm, type MenuItem, MenuButton, Modal, useToast } from './ui'

interface Page { messages: Message[]; names: Record<string, string>; last_read_id: number }

export function Chat({ groupId, gameId = 0, organiser = false, placeholder, aiCatchUp = false }: { groupId: number; gameId?: number; organiser?: boolean; placeholder?: string; aiCatchUp?: boolean }) {
  const { user } = useApp()
  const { setActiveGroup } = useData()
  const { toast } = useToast()
  const me = user?.id || 0
  const [msgs, setMsgs] = useState<Message[] | null>(null)
  const [names, setNames] = useState<Record<string, string>>({})
  const [more, setMore] = useState(false)
  const [text, setText] = useState('')
  const [edit, setEdit] = useState<Message | null>(null)
  const [del, setDel] = useState<Message | null>(null)
  const [typing, setTyping] = useState<{ name: string; until: number } | null>(null)
  const [firstUnread, setFirstUnread] = useState(0)
  const [catchUp, setCatchUp] = useState<string | null>(null)
  const [photos, setPhotos] = useState<(Photo | { id: number; pending: true; preview: string })[]>([])
  const [view, setView] = useState<Photo | null>(null)
  const pick = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const stick = useRef(true)
  const q = gameId ? `&game=${gameId}` : ''

  const load = useCallback(async () => {
    try {
      const p = await api.get<Page>(`${V1}/groups/${groupId}/messages?limit=60${q}`)
      setMsgs(p.messages); setNames(p.names); setMore(p.messages.length >= 60)
      if (!gameId) setFirstUnread(p.messages.find(m => m.id > p.last_read_id && m.user_id !== me && m.kind === 'text')?.id || 0)
    } catch (e) { toast((e as Error).message, 'err'); setMsgs([]) }
  }, [groupId, q, gameId, me, toast])
  useEffect(() => { setMsgs(null); stick.current = true; load() }, [load])
  useEffect(() => { if (!gameId) { setActiveGroup(groupId); return () => setActiveGroup(0) } }, [groupId, gameId, setActiveGroup])

  // read up to the newest while the chat is open and visible
  const lastID = msgs?.length ? msgs[msgs.length - 1].id : 0
  useEffect(() => {
    if (gameId || !lastID || document.visibilityState !== 'visible') return
    api.post(`${V1}/groups/${groupId}/read`, { up_to: lastID }).catch(() => {})
  }, [lastID, groupId, gameId])

  useLiveEvents(e => {
    if (e.type === 'message' && e.data.group_id === groupId && (e.data.game_id || 0) === gameId) {
      const m = e.data
      setMsgs(ms => !ms ? ms : ms.some(x => x.id === m.id || (m.client_id && x.client_id === m.client_id && x.user_id === m.user_id)) ? ms.map(x => (x.client_id && x.client_id === m.client_id && x.user_id === m.user_id) || x.id === m.id ? m : x) : [...ms, m])
      if (m.user_id && !names[String(m.user_id)]) load()
      setTyping(null)
    } else if (e.type === 'message_update' && e.data.group_id === groupId) {
      setMsgs(ms => ms && ms.map(x => x.id === e.data.id ? e.data : x))
    } else if (e.type === 'typing' && e.data.group_id === groupId && (e.data.game_id || 0) === gameId) {
      setTyping({ name: e.data.name, until: Date.now() + 5000 })
    } else if (e.type === 'resync') load()
  })
  useEffect(() => {
    if (!typing) return
    const t = window.setTimeout(() => setTyping(null), Math.max(0, typing.until - Date.now()))
    return () => window.clearTimeout(t)
  }, [typing])

  // stay at the bottom when new lines come in, unless reading older ones
  useLayoutEffect(() => {
    const el = list.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [msgs, typing])
  const onScroll = () => { const el = list.current; if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80 }

  const older = async () => {
    if (!msgs?.length) return
    const el = list.current
    const h = el?.scrollHeight || 0
    const p = await api.get<Page>(`${V1}/groups/${groupId}/messages?limit=60&before=${msgs[0].id}${q}`).catch(() => null)
    if (!p) return
    stick.current = false
    setMsgs(ms => [...p.messages, ...(ms || [])]); setNames(n => ({ ...n, ...p.names })); setMore(p.messages.length >= 60)
    requestAnimationFrame(() => { if (el) el.scrollTop = el.scrollHeight - h })
  }

  const lastTyping = useRef(0)
  const onType = (v: string) => {
    setText(v)
    if (Date.now() - lastTyping.current > 3000 && v.trim()) { lastTyping.current = Date.now(); api.post(`${V1}/groups/${groupId}/typing`, { game_id: gameId }).catch(() => {}) }
  }
  const ready = photos.filter((p): p is Photo => !('pending' in p))
  const uploading = photos.length > ready.length
  const addPhotos = async (files: FileList | null) => {
    const list = Array.from(files || []).slice(0, Math.max(0, 6 - photos.length))
    if (pick.current) pick.current.value = ''
    await Promise.all(list.map(async (f, i) => {
      const tmp = -(Date.now() + i)
      const preview = URL.createObjectURL(f)
      setPhotos(ps => [...ps, { id: tmp, pending: true, preview }])
      try { const p = await uploadPhoto(groupId, f); setPhotos(ps => ps.map(x => x.id === tmp ? p : x)) } catch (e) { setPhotos(ps => ps.filter(x => x.id !== tmp)); toast((e as Error).message, 'err') } finally { URL.revokeObjectURL(preview) }
    }))
  }
  const send = async () => {
    const body = text.trim()
    if ((!body && !ready.length) || uploading) return
    const sent = ready
    const client_id = `c${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`
    const temp: Message = { id: -Date.now(), group_id: groupId, game_id: gameId, user_id: me, kind: 'text', body, data: sent.length ? { photos: sent } : {}, reply_to: 0, client_id, edited_at: null, deleted_at: null, created_at: new Date().toISOString(), reactions: [], pending: true }
    stick.current = true
    setMsgs(ms => [...(ms || []), temp]); setText(''); setPhotos([])
    try {
      const m = await api.post<Message>(`${V1}/groups/${groupId}/messages`, { body, game_id: gameId, client_id, photos: sent.map(p => p.id) })
      setMsgs(ms => ms && (ms.some(x => x.id === m.id) ? ms.filter(x => x.id !== temp.id) : ms.map(x => x.id === temp.id ? m : x)))
    } catch (e) { setMsgs(ms => ms && ms.filter(x => x.id !== temp.id)); setText(body); setPhotos(sent); toast((e as Error).message, 'err') }
  }
  const react = async (m: Message, emoji: string) => { try { const r = await api.post<Message>(`${V1}/messages/${m.id}/react`, { emoji }); setMsgs(ms => ms && ms.map(x => x.id === r.id ? r : x)) } catch (e) { toast((e as Error).message, 'err') } }
  const runCatchUp = async () => { setCatchUp(''); try { setCatchUp((await api.post<{ text: string }>(`${V1}/groups/${groupId}/catchup`)).text) } catch (e) { setCatchUp(null); toast((e as Error).message, 'err') } }

  let lastDay = ''
  return (
    <div className={`chat ${gameId ? 'is-comments' : ''}`}>
      {aiCatchUp && !gameId && <div className="chat-tools"><button type="button" className="btn sm ghost" onClick={runCatchUp} title="A few lines on what you missed, only for you"><Icon name="sparkles" size={14} />Catch me up</button></div>}
      <div className="chat-list" ref={list} onScroll={onScroll} aria-live="polite">
        {more && <button type="button" className="btn sm ghost chat-older" onClick={older}>Older messages</button>}
        {msgs === null && <p className="muted small chat-empty">Loading…</p>}
        {msgs && msgs.length === 0 && <p className="muted small chat-empty">{gameId ? 'No comments yet. Running late? Court changed? Say it here.' : 'Nothing said yet. Say hello!'}</p>}
        {msgs?.map((m, i) => {
          const day = m.created_at.slice(0, 10)
          const dayLine = day !== lastDay ? <div className="chat-day" key={`d${day}`}><span>{fmtDate(day)}</span></div> : null
          lastDay = day
          const prev = msgs[i - 1]
          const same = prev && prev.user_id === m.user_id && prev.kind === 'text' && m.kind === 'text' && !dayLine && new Date(m.created_at).getTime() - new Date(prev.created_at).getTime() < 300000
          const unreadLine = firstUnread === m.id ? <div className="chat-new" key="new"><span>New</span></div> : null
          if (m.kind === 'system') {
            const gid = Number((m.data as { game_id?: number }).game_id || 0)
            return <React.Fragment key={m.id}>{dayLine}{unreadLine}<div className="chat-sys"><Icon name={sysIcon(m)} size={13} />{gid && !gameId ? <Link to={`/games/${gid}`}>{m.body}</Link> : <span>{m.body}</span>}<span className="chat-time">{fmtTime(new Date(m.created_at))}</span></div></React.Fragment>
          }
          const mine = m.user_id === me
          const name = names[String(m.user_id)] || 'Former player'
          const items: MenuItem[] = [
            ...REACTIONS.map(e => ({ label: <span className="react-opt">{e}</span>, onSelect: () => react(m, e) })),
            ...(mine && !m.deleted_at ? ['sep' as const, { label: 'Change', icon: 'edit', onSelect: () => setEdit(m) }] : []),
            ...((mine || organiser) && !m.deleted_at ? [{ label: 'Delete', icon: 'trash', danger: true, onSelect: () => setDel(m) }] : []),
            ...(!m.deleted_at ? [{ label: 'Copy the text', icon: 'copy', onSelect: () => { navigator.clipboard?.writeText(m.body) } }] : []),
          ]
          return (
            <React.Fragment key={m.id}>
              {dayLine}{unreadLine}
              <div className={`msg ${mine ? 'mine' : ''} ${same ? 'same' : ''} ${m.pending ? 'pending' : ''}`}>
                {!mine && <span className="msg-ava">{!same && <Avatar name={name} size={28} />}</span>}
                <div className="msg-col">
                  {!same && !mine && <div className="msg-name">{name}</div>}
                  <div className="msg-bubble-row">
                    <div className={`msg-bubble ${m.deleted_at ? 'deleted' : ''}`}>
                      {!m.deleted_at && messagePhotos(m.data).length > 0 && <div className={`msg-photos n${Math.min(messagePhotos(m.data).length, 4)}`}>{messagePhotos(m.data).map(p => (
                        <button key={p.id} type="button" className="msg-photo" style={{ aspectRatio: messagePhotos(m.data).length === 1 ? `${p.w} / ${p.h}` : '1' }} onClick={() => setView(p)} aria-label="Open the photo"><img src={p.thumb} alt="" loading="lazy" /></button>
                      ))}</div>}
                      {m.deleted_at ? <i>Message deleted</i> : m.body && <span className="msg-text">{m.body}</span>}
                      <span className="msg-meta">{m.edited_at && !m.deleted_at ? 'changed · ' : ''}{fmtTime(new Date(m.created_at))}</span>
                    </div>
                    {!m.pending && <MenuButton label="Message actions" className="btn ghost icon-only sm msg-more" items={items} align={mine ? 'right' : 'left'} width={200}><Icon name="more" size={15} /></MenuButton>}
                  </div>
                  {m.reactions.length > 0 && <div className="msg-reacts">{m.reactions.map(r => (
                    <button key={r.emoji} type="button" className={`react ${r.users.includes(me) ? 'on' : ''}`} onClick={() => react(m, r.emoji)} title={r.users.map(u => names[String(u)] || '?').join(', ')}>{r.emoji} {r.users.length}</button>
                  ))}</div>}
                </div>
              </div>
            </React.Fragment>
          )
        })}
        <div className="chat-typing">{typing ? `${typing.name} is writing…` : ''}</div>
      </div>
      {photos.length > 0 && <div className="composer-photos">{photos.map(p => (
        <span key={p.id} className={`composer-photo ${'pending' in p ? 'pending' : ''}`}>
          <img src={'pending' in p ? p.preview : p.thumb} alt="" />
          {!('pending' in p) && <button type="button" className="btn icon-only sm" aria-label="Remove the photo" onClick={() => setPhotos(ps => ps.filter(x => x.id !== p.id))}><Icon name="x" size={12} /></button>}
        </span>
      ))}</div>}
      <form className="composer" onSubmit={e => { e.preventDefault(); send() }}>
        <input ref={pick} type="file" accept="image/*" multiple hidden onChange={e => addPhotos(e.target.files)} />
        <button type="button" className="btn ghost icon-only composer-add" aria-label="Add photos" title="Add photos" onClick={() => pick.current?.click()} disabled={photos.length >= 6}><Icon name="image" size={18} /></button>
        <textarea rows={1} value={text} onChange={e => onType(e.target.value)} placeholder={placeholder || (gameId ? 'Write a comment' : 'Write to the group')} aria-label="Message"
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); send() } }} maxLength={4000} />
        <button className="btn primary icon-only" aria-label="Send" disabled={(!text.trim() && !ready.length) || uploading}><Icon name="send" size={17} /></button>
      </form>
      {edit && <EditMessage m={edit} onClose={() => setEdit(null)} onSaved={m => { setMsgs(ms => ms && ms.map(x => x.id === m.id ? m : x)); setEdit(null) }} />}
      {del && <Confirm title="Delete this message?" message="It is replaced by “Message deleted” for everyone." confirmLabel="Delete" onCancel={() => setDel(null)}
        onConfirm={async () => { const d = del; setDel(null); try { await api.del(`${V1}/messages/${d.id}`); setMsgs(ms => ms && ms.map(x => x.id === d.id ? { ...x, body: '', deleted_at: new Date().toISOString(), reactions: [] } : x)) } catch (e) { toast((e as Error).message, 'err') } }} />}
      {view && <Modal title="Photo" onClose={() => setView(null)} size="lg">
        <img className="photo-full" src={view.url} alt="" style={{ aspectRatio: `${view.w} / ${view.h}` }} />
        <div className="form-actions"><a className="btn" href={`${view.url}?download=1`}><Icon name="download" size={15} />Save</a><button type="button" className="btn primary" onClick={() => setView(null)}>Close</button></div>
      </Modal>}
      {catchUp !== null && <Modal title="What you missed" onClose={() => setCatchUp(null)} size="sm">
        {catchUp === '' ? <p className="muted">Reading the chat…</p> : <p className="pre">{catchUp}</p>}
        <p className="muted small mt">Written by Pocket AI, only for you.</p>
      </Modal>}
    </div>
  )
}

function sysIcon(m: Message): string {
  const w = String((m.data as { what?: string }).what || '')
  if (w.startsWith('game_cancelled')) return 'x-circle'
  if (w.startsWith('game')) return 'calendar'
  if (w.startsWith('result')) return 'trophy'
  if (w === 'disputed') return 'alert'
  if (w === 'short') return 'users'
  if (w === 'joined' || w === 'moved_up') return 'user'
  if (w === 'left') return 'log-out'
  return 'info'
}

function EditMessage({ m, onClose, onSaved }: { m: Message; onClose: () => void; onSaved: (m: Message) => void }) {
  const { toast } = useToast()
  const [v, setV] = useState(m.body)
  const save = async (e: React.FormEvent) => { e.preventDefault(); try { onSaved(await api.put<Message>(`${V1}/messages/${m.id}`, { body: v })) } catch (err) { toast((err as Error).message, 'err') } }
  return (
    <Modal title="Change the message" onClose={onClose} size="sm">
      <form onSubmit={save} className="stack">
        <textarea rows={4} value={v} onChange={e => setV(e.target.value)} autoFocus maxLength={4000} />
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={!v.trim()}>Save</button></div>
      </form>
    </Modal>
  )
}
