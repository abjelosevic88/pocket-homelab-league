// The person's groups, the sports and the other apps, kept current by the
// live stream (Server-Sent Events) and shared by every page.
import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { api, V1 } from './api'
import { useApp } from './app-context'
import type { AppState, Group, Message, Sport } from './types'

const EVENT = 'plg:changed'

export type LiveEvent =
  | { type: 'message' | 'message_update'; data: Message }
  | { type: 'read'; data: { group_id: number; last_read_id: number } }
  | { type: 'typing'; data: { group_id: number; game_id: number; user_id: number; name: string } }
  | { type: 'group' | 'members'; data: { id?: number; group_id?: number; deleted?: boolean } }
  | { type: 'game'; data: { id: number; group_id: number; deleted?: boolean } }
  | { type: 'answer'; data: { game_id: number; group_id: number; user_id: number; answer: string } }
  | { type: 'match'; data: { id: number; group_id: number; game_id: number; deleted?: boolean } }
  | { type: 'resync'; data: null }

interface DataState {
  groups: Group[] | null
  sports: Sport[]
  apps: AppState[] | null
  connected: boolean
  reload: () => Promise<void>
  reloadApps: () => Promise<void>
  subscribe: (fn: (e: LiveEvent) => void) => () => void
  setActiveGroup: (id: number) => void
  unread: number
}
const Ctx = createContext<DataState>(null as unknown as DataState)
export function useData() { return useContext(Ctx) }

/** Something changed (a game planned, a result): lists reload. */
export function announceChange() { window.dispatchEvent(new CustomEvent(EVENT)) }

export function useOnChange(fn: () => void) {
  useEffect(() => {
    window.addEventListener(EVENT, fn)
    return () => window.removeEventListener(EVENT, fn)
  }, [fn])
}

/** Live events for a component (a chat listening to its messages). */
export function useLiveEvents(fn: (e: LiveEvent) => void) {
  const { subscribe } = useData()
  const ref = useRef(fn)
  ref.current = fn
  useEffect(() => subscribe(e => ref.current(e)), [subscribe])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const { user, settings } = useApp()
  const me = user?.id || 0
  const [groups, setGroups] = useState<Group[] | null>(null)
  const [sports, setSports] = useState<Sport[]>([])
  const [apps, setApps] = useState<AppState[] | null>(null)
  const [connected, setConnected] = useState(false)
  const listeners = useRef(new Set<(e: LiveEvent) => void>())
  const active = useRef(0)
  const timer = useRef(0)

  const reload = useCallback(async () => {
    const g = await api.get<Group[]>(`${V1}/groups`).catch(() => null)
    setGroups(x => g || x || [])
  }, [])
  const reloadSoon = useCallback(() => { window.clearTimeout(timer.current); timer.current = window.setTimeout(reload, 300) }, [reload])
  const reloadApps = useCallback(async () => {
    try { setApps((await api.get<{ apps: AppState[] }>(`${V1}/apps`)).apps) } catch { setApps(a => a || []) }
  }, [])
  useEffect(() => {
    reload()
    reloadApps()
    api.get<Sport[]>(`${V1}/sports`).then(setSports).catch(() => {})
  }, [reload, reloadApps])
  useOnChange(reload)
  useEffect(() => {
    const t = window.setInterval(() => { if (document.visibilityState === 'visible') reloadApps() }, 120000)
    return () => window.clearInterval(t)
  }, [reloadApps])

  const emit = useCallback((e: LiveEvent) => { listeners.current.forEach(fn => { try { fn(e) } catch { /* a listener's problem stays its own */ } }) }, [])
  const subscribe = useCallback((fn: (e: LiveEvent) => void) => { listeners.current.add(fn); return () => { listeners.current.delete(fn) } }, [])
  const setActiveGroup = useCallback((id: number) => { active.current = id }, [])

  // the live stream; the browser reconnects by itself, and after a gap the
  // lists reload so nothing is missed
  useEffect(() => {
    if (!me) return
    let es: EventSource | null = null
    let closed = false
    let retry = 0
    let was = false
    const open = () => {
      if (closed) return
      es = new EventSource(`${V1}/stream`)
      es.addEventListener('hello', () => {
        setConnected(true)
        if (was) { reload(); emit({ type: 'resync', data: null }) }
        was = true
        retry = 0
      })
      const on = (type: LiveEvent['type']) => es!.addEventListener(type, ev => {
        let data: unknown
        try { data = JSON.parse((ev as MessageEvent).data) } catch { return }
        handle({ type, data } as LiveEvent)
      })
      ;(['message', 'message_update', 'read', 'typing', 'group', 'members', 'game', 'answer', 'match'] as const).forEach(on)
      es.onerror = () => {
        setConnected(false)
        if (es?.readyState === EventSource.CLOSED) { es.close(); retry = Math.min(retry + 1, 6); window.setTimeout(open, 1000 * 2 ** retry) }
      }
    }
    const handle = (e: LiveEvent) => {
      switch (e.type) {
        case 'message': {
          const m = e.data
          if (m.game_id) break
          setGroups(gs => {
            if (!gs) return gs
            const g = gs.find(x => x.id === m.group_id)
            if (!g) { reloadSoon(); return gs }
            const mine = m.user_id === me
            const seeing = active.current === m.group_id && document.visibilityState === 'visible'
            const counts = !mine && !seeing && m.kind === 'text'
            return gs.map(x => x.id === g.id ? { ...x, last_message_id: Math.max(x.last_message_id, m.id), last_message_at: m.created_at, unread: counts ? x.unread + 1 : mine ? 0 : x.unread } : x)
          })
          break
        }
        case 'read':
          setGroups(gs => gs && gs.map(x => x.id === e.data.group_id ? { ...x, last_read_id: e.data.last_read_id, unread: e.data.last_read_id >= x.last_message_id ? 0 : x.unread } : x))
          break
        case 'group':
        case 'members':
        case 'game':
        case 'answer':
        case 'match':
          reloadSoon()
          break
      }
      emit(e)
    }
    open()
    return () => { closed = true; es?.close(); setConnected(false) }
  }, [me, emit, reload, reloadSoon])

  const unread = useMemo(() => (groups || []).filter(g => !g.archived && g.my_notify !== 'off').reduce((n, g) => n + g.unread, 0), [groups])
  useEffect(() => {
    const base = settings?.title || 'Pocket League'
    document.title = unread > 0 ? `(${unread}) ${base}` : base
  }, [unread, settings?.title])

  const value = useMemo(() => ({ groups, sports, apps, connected, reload, reloadApps, subscribe, setActiveGroup, unread }),
    [groups, sports, apps, connected, reload, reloadApps, subscribe, setActiveGroup, unread])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

/** One app's state for this person; `ok` = the scope works. */
export function useAppState() {
  const { apps } = useData()
  return (slug: string, scope?: string) => {
    const a = apps?.find(x => x.slug === slug)
    const ok = !!a && a.via !== '' && (!scope || a.on.includes(scope))
    return { app: a, ok, loading: !apps }
  }
}
