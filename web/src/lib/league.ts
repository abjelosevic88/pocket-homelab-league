// What League's pages share: the apps it links to, words for states and
// answers, money, and the sports' own words.
import type { AnswerKind, AppSlug, AppStateName, Format, Game, Group, Scoring } from './types'
import { fmtClock, fmtDate } from './format'

export const APP_INFO: Record<AppSlug, { name: string; icon: string }> = {
  calendar: { name: 'Pocket Calendar', icon: 'calendar' },
  contacts: { name: 'Pocket Contacts', icon: 'users' },
  places: { name: 'Pocket Places', icon: 'pin' },
  weather: { name: 'Pocket Weather', icon: 'cloud' },
  health: { name: 'Pocket Health', icon: 'heart-pulse' },
  habits: { name: 'Pocket Habits', icon: 'habit' },
  split: { name: 'Pocket Split', icon: 'receipt' },
  money: { name: 'Pocket Money', icon: 'wallet' },
  mail: { name: 'Pocket Mail', icon: 'mail' },
  tasks: { name: 'Pocket Tasks', icon: 'check-square' },
}

export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'on', partly: 'partly on', off: 'off', link_off: 'switched off in Pocket Core', missing: 'not on this box', old: 'too old',
  no_core: 'no Pocket Core', not_set_up: 'not set up', error: 'not reachable',
}

export const CURRENCIES = ['EUR', 'BAM', 'RSD', 'HRK', 'USD', 'GBP', 'CHF', 'SEK', 'NOK', 'DKK', 'PLN', 'CZK', 'HUF', 'TRY']

export const WEEKDAYS = ['', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday']
export const WEEKDAYS_SHORT = ['', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

export const SCORING_WORDS: Record<Scoring, string> = {
  sets: 'Sets (6–4 3–6 7–5)', goals: 'Goals (3–2, a draw is possible)', points: 'Points (21–17)', result: 'Only who won (or a draw)', ranking: 'Everyone on their own, a score each',
}
export const FORMAT_WORDS: Record<Format, string> = { singles: 'One against one', doubles: 'Two against two', teams: 'Two teams, picked on the day', everyone: 'Everyone on their own' }

export const ANSWER_WORDS: Record<Exclude<AnswerKind, ''>, string> = { in: 'In', maybe: 'Maybe', out: 'Out', waiting: 'Waiting list' }

/** 1250 cents EUR → "12.50 EUR"; whole amounts without cents. */
export function money(cents: number, cur: string): string {
  if (!cents) return ''
  const v = cents / 100
  return `${Number.isInteger(v) ? v.toLocaleString() : v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ${cur === 'BAM' ? 'KM' : cur}`
}

/** "Tue 12 Nov · 20:00" */
export function when(ts: string, withYear = false): string {
  if (!ts) return ''
  const [y, m, d] = ts.slice(0, 10).split('-').map(Number)
  const wd = new Date(y, m - 1, d).toLocaleDateString(undefined, { weekday: 'short' })
  return `${wd} ${fmtDate(ts, withYear)} · ${fmtClock(ts.slice(11, 16))}`
}

/** "today", "tomorrow", "in 3 days", "Tue 12 Nov" */
export function relDay(ts: string): string {
  if (!ts) return ''
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const [y, m, d] = ts.slice(0, 10).split('-').map(Number)
  const days = Math.round((new Date(y, m - 1, d).getTime() - today.getTime()) / 86400000)
  if (days === 0) return 'today'
  if (days === 1) return 'tomorrow'
  if (days === -1) return 'yesterday'
  if (days > 1 && days < 7) return new Date(y, m - 1, d).toLocaleDateString(undefined, { weekday: 'long' })
  if (days < 0 && days > -7) return `${-days} days ago`
  return fmtDate(ts, false)
}

/** A local "YYYY-MM-DDTHH:MM" for an input. */
export function localTS(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}

/** The next usual game time of a group (or tomorrow 19:00). */
export function nextSlot(g: Group | undefined): string {
  const now = new Date()
  const hm = g?.time || '19:00'
  const [h, mi] = hm.split(':').map(Number)
  for (let i = 0; i < 14; i++) {
    const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() + i, h, mi)
    const wd = d.getDay() || 7
    if ((!g?.weekday || wd === g.weekday) && d > now) return localTS(d)
  }
  return localTS(new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, h, mi))
}

/** How full a game is, in words: "6 in · 2 short", "full", "8 of 10 in". */
export function fullness(g: Game): { text: string; tone: 'ok' | 'warn' | 'full' | '' } {
  if (g.status === 'voting') return { text: 'finding a time', tone: '' }
  if (g.status === 'cancelled') return { text: 'cancelled', tone: '' }
  const short = g.min_players - g.in
  if (g.max_players > 0 && g.in >= g.max_players) return { text: `full · ${g.in} in${g.waiting ? ` · ${g.waiting} waiting` : ''}`, tone: 'full' }
  if (short > 0) return { text: `${g.in} in · ${short} short`, tone: 'warn' }
  return { text: `${g.in} in${g.max_players ? ` of ${g.max_players}` : ''}`, tone: 'ok' }
}

export function sideName(names: Record<string, string>, ids: number[], format: Format): string {
  if (format === 'teams' && ids.length > 2) return `Team ${(names[String(ids[0])] || '').split(' ')[0]}`
  return ids.map(id => (names[String(id)] || '?').split(' ')[0]).join(' & ')
}

export const REACTIONS = ['👍', '🔥', '😂', '👏', '💪', '😮']
