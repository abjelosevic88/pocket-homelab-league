// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { GroupForm } from './GroupForm'
import { GameForm } from './GameForm'
import { ResultForm } from './ResultForm'
import { TournamentForm } from './TournamentForm'
import { Avatar, Loading, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'plg-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'group' | 'game' | 'result' | 'tournament'

/** Start something from anywhere (a page's button), for a group when known. */
export const openCreate = (k: CreateKind, groupId?: number, gameId?: number) => window.dispatchEvent(new CustomEvent('plg:create', { detail: { kind: k, groupId, gameId } }))

/** The "more ways to start" menu: the sidebar's caret. */
export function createItems(onCreate: (k: CreateKind) => void, { player, hasGroups }: { player: boolean; hasGroups: boolean }): MenuItem[] {
  return [
    { label: 'Plan a game', icon: 'calendar-plus', hint: 'N', disabled: !hasGroups, onSelect: () => onCreate('game') },
    { label: 'Enter a result', icon: 'trophy', hint: 'R', disabled: !hasGroups, onSelect: () => onCreate('result') },
    { label: 'New tournament', icon: 'layers', hint: 'T', disabled: !hasGroups, onSelect: () => onCreate('tournament') },
    'sep',
    { label: player ? 'New group (members only)' : 'New group', icon: 'users', hint: 'G', disabled: player, onSelect: () => onCreate('group') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { groups } = useData()
  const { user } = useApp()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [making, setMaking] = useState<{ kind: CreateKind; groupId?: number; gameId?: number } | null>(null)
  const player = user?.role === 'player'
  const live = (groups || []).filter(g => !g.archived)
  const here = Number((loc.pathname.match(/^\/groups\/(\d+)/) || [])[1] || 0) || undefined

  const create = (k: CreateKind, groupId?: number, gameId?: number) => {
    closeDrawer()
    if (k === 'group' && player) return
    if (k !== 'group' && live.length === 0) { if (!player) setMaking({ kind: 'group' }); return }
    setMaking({ kind: k, groupId: groupId || here, gameId })
  }
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind; groupId?: number; gameId?: number }>).detail; createRef.current(d.kind, d.groupId, d.gameId) }
    window.addEventListener('plg:shortcuts', open)
    window.addEventListener('plg:create', make)
    return () => { window.removeEventListener('plg:shortcuts', open); window.removeEventListener('plg:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': mk('game'); break
        case 'r': mk('result'); break
        case 'g': mk('group'); break
        case 'h': go('/'); break
        case 'p': go('/games'); break
        case 't': go('/tournaments'); break
        case 'u': go('/results'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'game', label: 'Plan a game', icon: 'calendar-plus', hint: 'N', keywords: 'new match session find a time vote', run: () => create('game') },
    { id: 'result', label: 'Enter a result', icon: 'trophy', hint: 'R', keywords: 'score won lost', run: () => create('result') },
    ...(player ? [] : [{ id: 'group', label: 'New group', icon: 'users', hint: 'G', keywords: 'team club sport create', run: () => create('group') }]),
    { id: 'home', label: 'Home', icon: 'home', hint: 'H', run: () => navigate('/') },
    { id: 'games', label: 'Games', icon: 'calendar', hint: 'P', keywords: 'upcoming next', run: () => navigate('/games') },
    { id: 'results', label: 'Results', icon: 'trophy', hint: 'U', keywords: 'scores matches', run: () => navigate('/results') },
    { id: 'tournaments', label: 'Tournaments', icon: 'layers', hint: 'T', keywords: 'cup knockout bracket americano ladder', run: () => navigate('/tournaments') },
    { id: 'tournament', label: 'New tournament', icon: 'layers', keywords: 'cup knockout bracket americano mexicano ladder swiss league', run: () => create('tournament') },
    ...live.flatMap(g => [
      { id: `g-${g.id}`, label: g.name, icon: 'users', keywords: `${g.sport_label} group`, run: () => navigate(`/groups/${g.id}`) },
      { id: `t-${g.id}`, label: `${g.name}: the table`, icon: 'chart', keywords: 'standings rating', run: () => navigate(`/groups/${g.id}?tab=table`) },
      { id: `c-${g.id}`, label: `${g.name}: chat`, icon: 'message', keywords: 'talk messages', run: () => navigate(`/groups/${g.id}?tab=chat`) },
    ]),
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'notif', label: 'Settings: notifications', icon: 'bell', keywords: 'reminders email push', run: () => navigate('/settings/notifications') },
    ...(player ? [] : [
      { id: 'apps', label: 'Settings: connected apps', icon: 'link', keywords: 'calendar health habits split money weather places contacts mail tasks core', run: () => navigate('/settings/apps') },
      { id: 'email', label: 'Settings: email to players', icon: 'mail', keywords: 'smtp invitations links', run: () => navigate('/settings/email') },
    ]),
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  const onGroupPage = /^\/groups\/\d+/.test(loc.pathname)
  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={k => create(k)} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className="content"><div className="content-inner">{groups ? <Outlet /> : <Loading />}</div></main>
        </div>
        {mobile && !/^\/(games\/\d|settings)/.test(loc.pathname) && !(onGroupPage && /tab=chat/.test(loc.search)) && (live.length > 0 || !player) &&
          <button type="button" className="fab" aria-label={live.length ? 'Plan a game' : 'New group'} title={live.length ? 'Plan a game' : 'New group'} onClick={() => create(live.length ? 'game' : 'group')}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {making?.kind === 'group' && <GroupForm onClose={() => setMaking(null)} onSaved={g => { setMaking(null); navigate(`/groups/${g.id}`) }} />}
        {making?.kind === 'game' && <GameForm groupId={making.groupId} onClose={() => setMaking(null)} onSaved={id => { setMaking(null); navigate(`/games/${id}`) }} />}
        {making?.kind === 'tournament' && <TournamentForm groupId={making.groupId} onClose={() => setMaking(null)} onSaved={id => { setMaking(null); navigate(`/tournaments/${id}`) }} />}
        {making?.kind === 'result' && <ResultForm groupId={making.groupId} gameId={making.gameId} onClose={() => setMaking(null)} onSaved={() => setMaking(null)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = { '': ['Home', '/'], games: ['Games', '/games'], results: ['Results', '/results'], groups: ['Groups', '/'], tournaments: ['Tournaments', '/tournaments'] }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('plg:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <>{section !== 'Groups' && <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /></>}<span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
