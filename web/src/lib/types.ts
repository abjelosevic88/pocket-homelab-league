export interface User { id: number; email: string; name: string; role: 'admin' | 'member' | 'player'; phone: string; has_password: boolean; last_seen_at: string | null; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'calendar' | 'contacts' | 'places' | 'weather' | 'health' | 'habits' | 'split' | 'money' | 'mail' | 'tasks'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number; currency: string
  calendar_auto: boolean; calendar_id: number; health_auto: boolean; habit_id: number
  money_account: number; money_category: number; split_group: number; booking_task: boolean; booking_days: number
  mail_from: string; email_via: 'mail' | 'smtp' | 'none'; email_via_set: string
  smtp_host: string; smtp_port: number; smtp_user: string; smtp_password: string; smtp_from: string; smtp_from_name: string; smtp_tls: string; smtp_configured: boolean
  notify_plans: boolean; notify_reminders: boolean; remind_hours: number; notify_chat: boolean; notify_short: boolean; notify_results: boolean; email_me: boolean
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_help: boolean; ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type Scoring = 'sets' | 'goals' | 'points' | 'result' | 'ranking'
export type Format = 'singles' | 'doubles' | 'teams' | 'everyone'

export interface Sport {
  key: string; name: string; emoji: string; scoring: Scoring; format: Format; formats: Format[]; min_players: number; max_players: number
  outdoor: boolean; lower_better: boolean; minutes: number; unit: string; health: string; draws: boolean; color: string
}

export interface Group {
  id: number; user_id: number; name: string; sport: string; sport_name: string; scoring: Scoring; format: Format; lower_better: boolean; color: string; description: string
  venue_name: string; venue_address: string; venue_lat: number | null; venue_lon: number | null; venue_ref: string; outdoor: boolean
  weekday: number; time: string; duration_min: number; plan_days: number; min_players: number; max_players: number
  cost_cents: number; currency: string; win_points: number; draw_points: number; loss_points: number; email_players: boolean; archived: boolean
  created_at: string; updated_at: string
  my_role: 'organiser' | 'player'; my_notify: 'all' | 'games' | 'off'; last_read_id: number; unread: number; members: number; last_message_id: number; last_message_at: string
  sport_info: Sport; sport_label: string; next_game?: Game
}

export type GameStatus = 'voting' | 'planned' | 'cancelled'
export type AnswerKind = 'in' | 'maybe' | 'out' | 'waiting' | ''

export interface Game {
  id: number; group_id: number; user_id: number; status: GameStatus; starts_at: string; duration_min: number; title: string; notes: string
  venue_name: string; venue_address: string; venue_lat: number | null; venue_lon: number | null; min_players: number; max_players: number
  cost_cents: number; currency: string; vote_until: string; cancel_reason: string; auto: boolean; created_at: string; updated_at: string
  in: number; maybe: number; out: number; waiting: number; my_answer: AnswerKind; matches: number; comments: number
  group_name?: string; group_color?: string; emoji?: string; sport?: string; ended?: boolean
}

export interface Member {
  user_id: number; name: string; full_name: string; nickname: string; role: 'organiser' | 'player'; account: 'admin' | 'member' | 'player'
  email?: string; phone?: string; joined: boolean; joined_at: string; last_seen_at?: string | null; can_link: boolean; link_sent_at?: string; online: boolean
}

export interface Person { user_id: number; name: string; answer: AnswerKind; side: '' | 'a' | 'b'; note: string; answered_at: string; rating: number }
export interface Option { id: number; game_id: number; starts_at: string; votes: Record<string, 'yes' | 'maybe' | 'no'> }

export interface Score { sets?: [number, number][]; a?: number; b?: number }
export interface MatchPlayer { user_id: number; side: '' | 'a' | 'b'; score: number | null; place: number }
export interface Match {
  id: number; group_id: number; game_id: number; user_id: number; played_at: string; score: Score; winner: '' | 'a' | 'b' | 'draw'; note: string
  confirmed_by: number; confirmed_at: string | null; disputed_by: number; dispute: string; players: MatchPlayer[]; created_at: string; updated_at: string
  text: string; names: Record<string, string>; state: 'confirmed' | 'waiting' | 'disputed'; can_confirm: boolean; can_edit: boolean; changes?: Record<string, number>
}

export interface Made { id: number; group_id: number; game_id: number; match_id: number; app: string; ref: string; url: string; title: string; detail: string; created_at: string }

export interface GameDetail {
  game: Game; group: Group; people: Person[]; options: Option[]; matches: Match[]
  mvp: { mine: number; tally: Record<string, number>; open: boolean }; can_edit: boolean; made: Made[]; share_cost: number; result?: string; rating_gap?: number
}

export interface Row {
  user_id: number; name: string; member: boolean; played: number; won: number; drawn: number; lost: number; points: number; for: number; against: number
  sets_won: number; sets_lost: number; scored: number; wins: number; best: number | null; average: number | null; rating: number; change: number
  form: ('W' | 'D' | 'L')[]; streak: string; mvp: number; position: number
}

export interface Versus { user_id: number; name: string; played: number; won: number; drawn: number; lost: number; together: number; together_won: number; last_played_at: string }

export interface Message {
  id: number; group_id: number; game_id: number; user_id: number; kind: 'text' | 'system'; body: string; data: Record<string, unknown>; reply_to: number
  client_id?: string; edited_at: string | null; deleted_at: string | null; created_at: string; reactions: { emoji: string; users: number[] }[]
  pending?: boolean
}

export interface LinkResult { url: string; expires_at: string; text: string; whatsapp: string; sms: string; mailto: string; emailed: boolean; email_error?: string; email_via: string }

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'
export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

export interface ResultItem { match: Match; group_name: string; emoji: string; group_color?: string; line?: string; won?: string }
export interface Today { next: Game[]; unanswered: Game[]; voting: Game[]; confirm: ResultItem[]; unread: number; groups: number; recent: ResultItem[] }
