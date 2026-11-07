package store

import (
	"encoding/json"
	"strings"
)

// Roles.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
	// RolePlayer is an account an organiser made for a group: it signs in
	// with a link and sees only its groups.
	RolePlayer = "player"
)

// User is an account on this box.
type User struct {
	ID           int64   `json:"id"`
	Email        string  `json:"email"`
	Name         string  `json:"name"`
	PasswordHash string  `json:"-"`
	Role         string  `json:"role"` // admin | member | player
	Phone        string  `json:"phone"`
	InvitedBy    int64   `json:"invited_by,omitempty"`
	LastSeenAt   *string `json:"last_seen_at"`
	CreatedAt    string  `json:"created_at"`
	HasPassword  bool    `json:"has_password"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Game statuses.
const (
	GameVoting    = "voting"
	GamePlanned   = "planned"
	GameCancelled = "cancelled"
)

// Answers to "are you coming?".
const (
	AnswerIn      = "in"
	AnswerMaybe   = "maybe"
	AnswerOut     = "out"
	AnswerWaiting = "waiting"
)

// Group is a few people who play one sport together.
type Group struct {
	ID           int64    `json:"id"`
	UserID       int64    `json:"user_id"`
	Name         string   `json:"name"`
	Sport        string   `json:"sport"`
	SportName    string   `json:"sport_name"`
	Scoring      string   `json:"scoring"`
	Format       string   `json:"format"`
	LowerBetter  bool     `json:"lower_better"`
	Color        string   `json:"color"`
	Description  string   `json:"description"`
	VenueName    string   `json:"venue_name"`
	VenueAddress string   `json:"venue_address"`
	VenueLat     *float64 `json:"venue_lat"`
	VenueLon     *float64 `json:"venue_lon"`
	VenueRef     string   `json:"venue_ref"`
	Outdoor      bool     `json:"outdoor"`
	Weekday      int      `json:"weekday"`
	Time         string   `json:"time"`
	DurationMin  int      `json:"duration_min"`
	PlanDays     int      `json:"plan_days"`
	MinPlayers   int      `json:"min_players"`
	MaxPlayers   int      `json:"max_players"`
	CostCents    int64    `json:"cost_cents"`
	Currency     string   `json:"currency"`
	WinPoints    int      `json:"win_points"`
	DrawPoints   int      `json:"draw_points"`
	LossPoints   int      `json:"loss_points"`
	EmailPlayers bool     `json:"email_players"`
	Archived     bool     `json:"archived"`
	PayNote      string   `json:"pay_note"`     // how to pay: an IBAN, a Revolut link
	ChargeGames  bool     `json:"charge_games"` // charge each player their share after a game
	RatingScope  string   `json:"rating_scope"` // all | season
	Stats        []string `json:"stats"`        // the stats kept per player
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`

	// For the person asking.
	MyRole     string `json:"my_role"` // organiser | player
	MyNotify   string `json:"my_notify"`
	LastReadID int64  `json:"last_read_id"`
	Unread     int    `json:"unread"`
	Members    int    `json:"members"`
	LastMsgID  int64  `json:"last_message_id"`
	LastMsgAt  string `json:"last_message_at"`
}

// Member is one person in a group.
type Member struct {
	ID         int64   `json:"id"`
	GroupID    int64   `json:"group_id"`
	UserID     int64   `json:"user_id"`
	Role       string  `json:"role"` // organiser | player
	Nickname   string  `json:"nickname"`
	Notify     string  `json:"notify"`
	LastReadID int64   `json:"last_read_id"`
	FeedToken  string  `json:"-"`
	AddedBy    int64   `json:"added_by"`
	JoinedAt   string  `json:"joined_at"`
	Name       string  `json:"name"`
	Email      string  `json:"email"`
	Phone      string  `json:"phone"`
	Account    string  `json:"account"` // the account's role: admin | member | player
	InvitedBy  int64   `json:"invited_by"`
	LastSeenAt *string `json:"last_seen_at"`
}

// Display is the name shown in the group.
func (m Member) Display() string {
	if m.Nickname != "" {
		return m.Nickname
	}
	if m.Name != "" {
		return m.Name
	}
	if m.Email != "" {
		return m.Email
	}
	return "Player"
}

// Game is one meeting of a group.
type Game struct {
	ID           int64    `json:"id"`
	GroupID      int64    `json:"group_id"`
	UserID       int64    `json:"user_id"`
	Status       string   `json:"status"`
	StartsAt     string   `json:"starts_at"`
	DurationMin  int      `json:"duration_min"`
	Title        string   `json:"title"`
	Notes        string   `json:"notes"`
	VenueName    string   `json:"venue_name"`
	VenueAddress string   `json:"venue_address"`
	VenueLat     *float64 `json:"venue_lat"`
	VenueLon     *float64 `json:"venue_lon"`
	MinPlayers   int      `json:"min_players"`
	MaxPlayers   int      `json:"max_players"`
	CostCents    int64    `json:"cost_cents"`
	Currency     string   `json:"currency"`
	VoteUntil    string   `json:"vote_until"`
	CancelReason string   `json:"cancel_reason"`
	Auto         bool     `json:"auto"`
	Sent         string   `json:"-"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`

	// Filled for lists.
	In       int    `json:"in"`
	Maybe    int    `json:"maybe"`
	Out      int    `json:"out"`
	Waiting  int    `json:"waiting"`
	MyAnswer string `json:"my_answer"`
	Matches  int    `json:"matches"`
	Comments int    `json:"comments"`
}

// Answer is one person's "are you coming?".
type Answer struct {
	GameID     int64  `json:"game_id"`
	UserID     int64  `json:"user_id"`
	Answer     string `json:"answer"`
	Side       string `json:"side"`
	Note       string `json:"note"`
	AnsweredAt string `json:"answered_at"`
}

// Option is a time offered while finding one.
type Option struct {
	ID           int64             `json:"id"`
	GameID       int64             `json:"game_id"`
	StartsAt     string            `json:"starts_at"`
	VenueName    string            `json:"venue_name"` // "" = the game's own place
	VenueAddress string            `json:"venue_address"`
	VenueLat     *float64          `json:"venue_lat"`
	VenueLon     *float64          `json:"venue_lon"`
	Votes        map[string]string `json:"votes"` // user id → yes | maybe | no
}

// OptionIn is a time (and maybe a place) offered for a vote.
type OptionIn struct {
	StartsAt     string   `json:"starts_at"`
	VenueName    string   `json:"venue_name"`
	VenueAddress string   `json:"venue_address"`
	VenueLat     *float64 `json:"venue_lat"`
	VenueLon     *float64 `json:"venue_lon"`
}

// Key is what makes two options the same: the time and the place.
func (o OptionIn) Key() string {
	return o.StartsAt + "|" + strings.ToLower(strings.TrimSpace(o.VenueName))
}

// Match is a result.
type Match struct {
	ID          int64         `json:"id"`
	GroupID     int64         `json:"group_id"`
	GameID      int64         `json:"game_id"`
	UserID      int64         `json:"user_id"`
	PlayedAt    string        `json:"played_at"`
	Score       Score         `json:"score"`
	Winner      string        `json:"winner"`
	Note        string        `json:"note"`
	ConfirmedBy int64         `json:"confirmed_by"`
	ConfirmedAt *string       `json:"confirmed_at"`
	DisputedBy  int64         `json:"disputed_by"`
	Dispute     string        `json:"dispute"`
	Players     []MatchPlayer `json:"players"`
	CreatedAt   string        `json:"created_at"`
	UpdatedAt   string        `json:"updated_at"`
}

// Score is what happened: sets for set sports, a and b for goals and
// points, nothing for result-only (who won) and ranking (in the players).
type Score struct {
	Sets [][2]int `json:"sets,omitempty"`
	A    *float64 `json:"a,omitempty"`
	B    *float64 `json:"b,omitempty"`
}

// MatchPlayer is one player in a match.
type MatchPlayer struct {
	UserID int64    `json:"user_id"`
	Side   string   `json:"side"`
	Score  *float64 `json:"score"`
	Place  int      `json:"place"`
}

// Message is a line in a group's chat or under a game.
type Message struct {
	ID        int64           `json:"id"`
	GroupID   int64           `json:"group_id"`
	GameID    int64           `json:"game_id"`
	UserID    int64           `json:"user_id"`
	Kind      string          `json:"kind"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data"`
	ReplyTo   int64           `json:"reply_to"`
	ClientID  string          `json:"client_id,omitempty"`
	EditedAt  *string         `json:"edited_at"`
	DeletedAt *string         `json:"deleted_at"`
	CreatedAt string          `json:"created_at"`
	Reactions []Reaction      `json:"reactions"`
}

// Reaction is one emoji under a message, with who used it.
type Reaction struct {
	Emoji string  `json:"emoji"`
	Users []int64 `json:"users"`
}

// SignInLink is a player's one-time way in.
type SignInLink struct {
	ID        int64   `json:"id"`
	UserID    int64   `json:"user_id"`
	CreatedBy int64   `json:"created_by"`
	GroupID   int64   `json:"group_id"`
	ExpiresAt string  `json:"expires_at"`
	UsedAt    *string `json:"used_at"`
	CreatedAt string  `json:"created_at"`
}

// Made is something made in another Pocket app for one person.
type Made struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	GroupID   int64  `json:"group_id"`
	GameID    int64  `json:"game_id"`
	MatchID   int64  `json:"match_id"`
	App       string `json:"app"`
	Ref       string `json:"ref"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday
	Currency   string `json:"currency"`   // new groups start in this currency

	// The other apps, for this person.
	CalendarAuto bool   `json:"calendar_auto"` // games I am in go to my calendar
	CalendarID   int64  `json:"calendar_id"`   // which one (0 = the default)
	HealthAuto   bool   `json:"health_auto"`   // record the matches I played in Pocket Health
	HabitID      int64  `json:"habit_id"`      // tick this habit on days I play (0 = none)
	MoneyAccount int64  `json:"money_account"` // my share of a game goes to this account
	MoneyCat     int64  `json:"money_category"`
	SplitGroup   int64  `json:"split_group"`  // the Split group a game's cost goes to by default
	BookingTask  bool   `json:"booking_task"` // planning a game with a cost: a "book the court" task
	BookingDays  int    `json:"booking_days"` // due this many days before
	MailFrom     string `json:"mail_from"`    // which Pocket Mail account emails leave from ("" = its default)

	// How emails to players go out: Pocket Mail, the person's own SMTP, or not at all.
	EmailVia     string `json:"email_via"` // mail | smtp | none ("" = Pocket Mail with Core, else none)
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUser     string `json:"smtp_user"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from"`
	SMTPFromName string `json:"smtp_from_name"`
	SMTPTLS      string `json:"smtp_tls"`

	// Notifications.
	NotifyPlans     bool   `json:"notify_plans"`     // a game planned, moved, cancelled; a time to vote on
	NotifyReminders bool   `json:"notify_reminders"` // the day before and a few hours before
	RemindHours     int    `json:"remind_hours"`
	NotifyChat      bool   `json:"notify_chat"`    // new messages while League is closed
	NotifyShort     bool   `json:"notify_short"`   // a game is short of players (organisers)
	NotifyResults   bool   `json:"notify_results"` // a result to confirm
	EmailMe         bool   `json:"email_me"`       // without a notification address: emails instead
	NotifyURL       string `json:"notify_url"`
	NotifyFormat    string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIHelp        bool   `json:"ai_help"`
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
