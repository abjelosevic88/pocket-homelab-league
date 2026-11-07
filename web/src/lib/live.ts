import { api, V1 } from './api'

export interface LiveRules { kind: 'tennis' | 'rally' | 'count'; sets: number; games: number; tiebreak_at: number; tiebreak_to: number; super_final: boolean; golden: boolean; points: number; last_points: number; win_by: number; cap: number }
export interface LiveView {
  sets: [number, number][]; cur: [number, number]; point: [string, string]; sets_won: [number, number]; tiebreak: boolean; deuce: boolean; golden: boolean
  set_point: '' | 'a' | 'b'; match_point: '' | 'a' | 'b'; done: boolean; winner: '' | 'a' | 'b' | 'draw'; total: number
  score: { sets?: [number, number][]; a?: number; b?: number }
}
export interface LiveScore {
  id: number; group_id: number; game_id: number; tmatch_id: number; a_name: string; b_name: string; a_ids: number[]; b_ids: number[]
  points: string[]; scorer: number; scorer_name: string; started_at: string; updated_at: string; finished_at: string | null
  rules: LiveRules; view: LiveView; line: string
}
/** A live score in a tournament's page (by match). */
export interface TLive { id: number; line: string; view: LiveView; rules: LiveRules; a_name: string; b_name: string; updated_at: string; scorer_name?: string }

export const startLive = (groupId: number, body: Record<string, unknown>) => api.post<LiveScore>(`${V1}/groups/${groupId}/live`, body)
export const livePoint = (id: number, body: Record<string, unknown>) => api.post<LiveScore>(`${V1}/live/${id}/point`, body)
export const finishLive = (id: number) => api.post<LiveScore>(`${V1}/live/${id}/finish`)
export const dropLive = (id: number) => api.del(`${V1}/live/${id}`)

/** The score the way a result is entered. */
export function liveResult(l: LiveScore): Record<string, unknown> {
  if (l.rules.kind === 'count') return { a: l.view.cur[0], b: l.view.cur[1] }
  return { sets: l.view.score.sets || [] }
}

export const RULE_PRESETS: { label: string; rules: Partial<LiveRules> }[] = [
  { label: 'Best of 3, tie-break at 6–6', rules: { kind: 'tennis', sets: 3, games: 6, tiebreak_at: 6, tiebreak_to: 7 } },
  { label: 'Best of 3, deciding super tie-break', rules: { kind: 'tennis', sets: 3, games: 6, tiebreak_at: 6, tiebreak_to: 7, super_final: true } },
  { label: 'One set', rules: { kind: 'tennis', sets: 1, games: 6, tiebreak_at: 6, tiebreak_to: 7 } },
  { label: 'Short sets to 4', rules: { kind: 'tennis', sets: 3, games: 4, tiebreak_at: 4, tiebreak_to: 7 } },
  { label: 'Best of 5', rules: { kind: 'tennis', sets: 5, games: 6, tiebreak_at: 6, tiebreak_to: 7 } },
]
