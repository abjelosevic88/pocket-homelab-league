import type { TLive } from './live'
// Tournaments as the pages see them.
import type { Group, Score } from './types'

export interface TFormat { key: string; name: string; description: string; entries: ('player' | 'pair' | 'team')[]; rotating: boolean }

export interface TSettings {
  groups: number; advance: number; double: boolean; third_place: boolean; plate: boolean; reset: boolean
  points: number; rounds: number; split: string; seeding: 'rating' | 'random' | 'manual'; schedule: 'queue' | 'timetable' | 'none'
  minutes: number; buffer: number; auto_confirm: number; mode: '' | 'stroke' | 'stableford' | 'pins' | 'plain'; low: boolean
  course?: { par: number[]; si: number[] }; allowance: number; basis: number; pct: number; ties: string[]
  reach: number; respond_days: number; play_days: number; protect_days: number; box_size: number; promote: number; colours: boolean; tiebreaks: string[]
}

export interface Tournament {
  id: number; group_id: number; user_id: number; name: string; format: string; entry_kind: 'player' | 'pair' | 'team'
  status: 'signup' | 'live' | 'finished' | 'cancelled'; rated: boolean; starts_at: string; ends_at: string; venue_name: string
  capacity: number; signup_until: string; fee_cents: number; currency: string; checkin: boolean; share_token?: string
  settings: TSettings; game_id: number; winner: string; created_at: string; updated_at: string
  format_name: string; sport: string; emoji: string; group_name: string; entries: number; mine: boolean
}

export interface TEntry {
  id: number; tournament_id: number; name: string; status: 'in' | 'waiting' | 'withdrawn' | 'disqualified'; seed: number; rating: number
  handicap: number | null; position: number; protected_until: string; paid: boolean; checked_in: boolean; note: string
  created_by: number; created_at: string; members: { user_id: number; role: string; position: number }[]
}

export interface TStage {
  id: number; seq: number; kind: string; name: string; status: 'pending' | 'live' | 'done'
  tables: Record<string, TRow[]>; leaderboard?: TLeader[]; cards?: TCard[]
}
export interface TGroup { id: number; stage_id: number; name: string; kind: string; position: number }

export interface TMatch {
  id: number; tournament_id: number; stage_id: number; group_id: number; parent_id: number; round: number; position: number; label: string
  a_entry: number; b_entry: number; a_source: string; b_source: string; next_match: number; next_slot: string; loser_match: number
  court_id: number; scheduled_at: string; called_at: string | null; status: 'pending' | 'ready' | 'called' | 'reported' | 'done' | 'void'
  outcome: string; winner: '' | 'a' | 'b' | 'draw'; score: Score; a_total: number | null; b_total: number | null; white: string; deadline: string
  reported_by: number; reported_at: string | null; confirmed_by: number; confirmed_at: string | null; dispute: string; sides: { a: number[]; b: number[] }
}

export interface TRow {
  id: number; played: number; won: number; drawn: number; lost: number; points: number; for: number; against: number
  sets_won: number; sets_lost: number; points_won: number; points_lost: number; buchholz: number; sonneborn_berger: number; byes: number; rank: number; decided_by: string
}
export interface TLeader { id: number; rounds: number[]; total: number; gross: number; rank: number; decided_by: string; holes: number }
export interface TCard { id: number; stage_id: number; round: number; entry_id: number; gross: number | null; holes: number[] }
export interface TChallenge { id: number; challenger: number; defender: number; status: string; respond_by: string; play_by: string; match_id: number; created_at: string }
export interface TCourt { id: number; name: string; position: number; active: boolean }

export interface TDetail {
  tournament: Tournament; group: Group; entries: TEntry[]; stages: TStage[]; groups: TGroup[]; matches: TMatch[]; courts: TCourt[]
  names: Record<string, string>; mine: number[]; can_run: boolean; settings: TSettings; challenges?: TChallenge[]; podium: { rank: number; entry_id: number }[]
  log?: { id: number; user_id: number; action: string; detail: string; created_at: string }[]; share_url?: string; now: string; added?: number
  live?: Record<string, TLive>
}

export const STATUS_WORDS: Record<Tournament['status'], string> = { signup: 'Sign-up open', live: 'Being played', finished: 'Finished', cancelled: 'Cancelled' }
export const ENTRY_WORDS: Record<string, string> = { player: 'Players', pair: 'Pairs', team: 'Teams' }

/** The name of an entry, or where the side comes from. */
export function sideName(d: TDetail, m: TMatch, side: 'a' | 'b'): string {
  const list = m.sides?.[side] || []
  const byId = (id: number) => d.entries.find(e => e.id === id)?.name || '?'
  if (list.length) return list.map(id => byId(id).replace(/\s*\(.*\)$/, '').split(' ')[0]).join(' & ')
  const id = side === 'a' ? m.a_entry : m.b_entry
  if (id) return byId(id)
  const src = side === 'a' ? m.a_source : m.b_source
  return src === 'bye' ? 'Bye' : src || 'To be decided'
}

/** The score in a few characters. */
export function tScore(m: TMatch): string {
  if (m.outcome === 'bye') return 'bye'
  if (m.outcome === 'walkover' || m.outcome === 'no_show') return 'w/o'
  if (m.outcome === 'double_walkover') return 'no match'
  if (m.a_total != null && m.b_total != null) return `${m.a_total}–${m.b_total}`
  if (m.score?.sets?.length) return m.score.sets.map(s => `${s[0]}–${s[1]}`).join(' ') + (m.outcome === 'retired' ? ' ret.' : '')
  if (m.score?.a != null && m.score?.b != null) return `${m.score.a}–${m.score.b}`
  if (m.winner === 'draw') return '½–½'
  if (m.winner) return m.winner === 'a' ? '1–0' : '0–1'
  return ''
}

/** The entries of a match, for "is it mine?". */
export function matchEntries(m: TMatch): number[] {
  if (m.sides?.a?.length) return [...m.sides.a, ...m.sides.b]
  return [m.a_entry, m.b_entry].filter(Boolean)
}

export const KIND_WORDS: Record<string, string> = {
  round_robin: 'Round robin', knockout: 'Knockout', double_elim: 'Double elimination', swiss: 'Swiss', americano: 'Americano', mexicano: 'Mexicano',
  kotc: 'King of the Court', ranking: 'Leaderboard', team_ties: 'Team ties', ladder: 'Ladder', arena: 'Arena',
}
