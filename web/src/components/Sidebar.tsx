// Left column: plan a game (and the other ways to start), Home, Games,
// Results, then the person's groups with their unread chat. The Pocket
// Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { Icon } from './icons'
import { createItems, type CreateKind } from './Layout'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings, user } = useApp()
  const { groups } = useData()
  const navigate = useNavigate()
  const loc = useLocation()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket League'
  const player = user?.role === 'player'
  const cur = Number((loc.pathname.match(/^\/groups\/(\d+)/) || [])[1] || 0)
  const nav = [
    { to: '/', label: 'Home', icon: 'home', active: loc.pathname === '/' },
    { to: '/games', label: 'Games', icon: 'calendar', active: loc.pathname.startsWith('/games') },
    { to: '/tournaments', label: 'Tournaments', icon: 'bracket', active: loc.pathname.startsWith('/tournaments') },
    { to: '/results', label: 'Results', icon: 'trophy', active: loc.pathname === '/results' },
  ]
  const live = (groups || []).filter(g => !g.archived)
  const canPlan = live.length > 0
  return (
    <aside className="sidebar" aria-label="Pocket League">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket League' ? title : <>Pocket <b>League</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate(canPlan ? 'game' : 'group')} disabled={!canPlan && player} title={canPlan ? 'Plan a game (N)' : 'New group'}>
          <Icon name="plus" size={15} strokeWidth={2.2} />{canPlan ? 'Plan a game' : 'New group'}
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createItems(onCreate, { player, hasGroups: canPlan })} onClose={() => setMenu(null)} width={280} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} className={() => `sb-nav-item ${n.active ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
          </NavLink>
        ))}
      </nav>

      <section className="sb-sec">
        <h4 className="sb-h">Your groups</h4>
        {live.map(g => (
          <button key={g.id} type="button" className={`sb-row sb-list ${cur === g.id ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/groups/${g.id}`) }} title={`${g.sport_label} · ${g.members} people`}>
            <span className="sb-sport" style={{ background: `color-mix(in srgb, ${g.color || 'var(--accent)'} 18%, transparent)` }} aria-hidden="true">{g.sport_info?.emoji}</span>
            <span className="grow ellipsis">{g.name}</span>
            {g.unread > 0 && g.my_notify !== 'off' && <span className="sb-count" title={`${g.unread} new in the chat`}>{g.unread}</span>}
          </button>
        ))}
        {groups && live.length === 0 && !player && <button type="button" className="sb-row sb-list" onClick={() => onCreate('group')}><span className="sb-list-ico" aria-hidden="true"><Icon name="plus" size={14} /></span><span className="grow">Start your first group</span></button>}
        {groups && live.length === 0 && player && <p className="muted small sb-note">When someone adds you to a group, it shows up here.</p>}
      </section>

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('plg:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
