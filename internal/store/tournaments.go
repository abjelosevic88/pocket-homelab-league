package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// ---- tournaments ----

// Tournament is one tournament of a group.
type Tournament struct {
	ID          int64           `json:"id"`
	GroupID     int64           `json:"group_id"`
	UserID      int64           `json:"user_id"`
	Name        string          `json:"name"`
	Format      string          `json:"format"`
	EntryKind   string          `json:"entry_kind"`
	Status      string          `json:"status"`
	Rated       bool            `json:"rated"`
	StartsAt    string          `json:"starts_at"`
	EndsAt      string          `json:"ends_at"`
	VenueName   string          `json:"venue_name"`
	Capacity    int             `json:"capacity"`
	SignupUntil string          `json:"signup_until"`
	FeeCents    int64           `json:"fee_cents"`
	Currency    string          `json:"currency"`
	Checkin     bool            `json:"checkin"`
	ShareToken  string          `json:"share_token,omitempty"`
	Settings    json.RawMessage `json:"settings"`
	GameID      int64           `json:"game_id"`
	Winner      string          `json:"winner"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

const tCols = `id, group_id, COALESCE(user_id, 0), name, format, entry_kind, status, rated, starts_at, ends_at, venue_name, capacity, signup_until,
	fee_cents, currency, checkin, share_token, settings, COALESCE(game_id, 0), winner, created_at, updated_at`

func scanT(row interface{ Scan(...any) error }) (*Tournament, error) {
	var t Tournament
	var settings string
	if err := row.Scan(&t.ID, &t.GroupID, &t.UserID, &t.Name, &t.Format, &t.EntryKind, &t.Status, &t.Rated, &t.StartsAt, &t.EndsAt, &t.VenueName,
		&t.Capacity, &t.SignupUntil, &t.FeeCents, &t.Currency, &t.Checkin, &t.ShareToken, &settings, &t.GameID, &t.Winner, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	if !json.Valid([]byte(settings)) {
		settings = "{}"
	}
	t.Settings = json.RawMessage(settings)
	return &t, nil
}

// Tournaments lists a group's tournaments, newest first.
func (s *Store) Tournaments(ctx context.Context, gid int64) ([]*Tournament, error) {
	return s.tlist(ctx, `WHERE group_id = ? ORDER BY CASE status WHEN 'live' THEN 0 WHEN 'signup' THEN 1 ELSE 2 END, COALESCE(NULLIF(starts_at, ''), created_at) DESC`, gid)
}

// TournamentsOf lists the tournaments of the groups a person is in.
func (s *Store) TournamentsOf(ctx context.Context, uid int64) ([]*Tournament, error) {
	return s.tlist(ctx, `WHERE group_id IN (SELECT group_id FROM members WHERE user_id = ?) ORDER BY CASE status WHEN 'live' THEN 0 WHEN 'signup' THEN 1 ELSE 2 END, COALESCE(NULLIF(starts_at, ''), created_at) DESC`, uid)
}

// LiveTournaments are those being played (background work).
func (s *Store) LiveTournaments(ctx context.Context) ([]*Tournament, error) {
	return s.tlist(ctx, `WHERE status IN ('live', 'signup')`)
}

func (s *Store) tlist(ctx context.Context, where string, args ...any) ([]*Tournament, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+tCols+` FROM tournaments `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Tournament{}
	for rows.Next() {
		t, err := scanT(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTournament returns one tournament (the caller checks the group).
func (s *Store) GetTournament(ctx context.Context, id int64) (*Tournament, error) {
	return scanT(s.DB.QueryRowContext(ctx, `SELECT `+tCols+` FROM tournaments WHERE id = ?`, id))
}

// TournamentByToken finds a tournament by its share link.
func (s *Store) TournamentByToken(ctx context.Context, tok string) (*Tournament, error) {
	if tok == "" {
		return nil, ErrNotFound
	}
	return scanT(s.DB.QueryRowContext(ctx, `SELECT `+tCols+` FROM tournaments WHERE share_token = ?`, tok))
}

func tArgs(t *Tournament) []any {
	if len(t.Settings) == 0 {
		t.Settings = json.RawMessage("{}")
	}
	return []any{t.Name, t.Format, t.EntryKind, t.Status, boolInt(t.Rated), t.StartsAt, t.EndsAt, t.VenueName, t.Capacity, t.SignupUntil,
		t.FeeCents, t.Currency, boolInt(t.Checkin), string(t.Settings), nullI(&t.GameID), t.Winner}
}

// CreateTournament saves a new tournament with its share token.
func (s *Store) CreateTournament(ctx context.Context, uid int64, t *Tournament) error {
	t.ShareToken = RandomToken(12)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO tournaments (group_id, user_id, share_token, name, format, entry_kind, status, rated, starts_at, ends_at, venue_name, capacity,
		signup_until, fee_cents, currency, checkin, settings, game_id, winner) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append([]any{t.GroupID, nullI(&uid), t.ShareToken}, tArgs(t)...)...)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	t.UserID = uid
	return nil
}

// UpdateTournament saves a tournament.
func (s *Store) UpdateTournament(ctx context.Context, t *Tournament) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE tournaments SET name = ?, format = ?, entry_kind = ?, status = ?, rated = ?, starts_at = ?, ends_at = ?, venue_name = ?, capacity = ?,
		signup_until = ?, fee_cents = ?, currency = ?, checkin = ?, settings = ?, game_id = ?, winner = ?, updated_at = ? WHERE id = ?`, append(tArgs(t), Now(), t.ID)...)
	return err
}

// DeleteTournament removes a tournament with everything in it.
func (s *Store) DeleteTournament(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM tournaments WHERE id = ?`, id)
	return err
}

// ---- courts ----

// TCourt is a court or a pitch of a tournament.
type TCourt struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	Active   bool   `json:"active"`
}

// Courts lists a tournament's courts in order.
func (s *Store) Courts(ctx context.Context, tid int64) ([]TCourt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, position, active FROM t_courts WHERE tournament_id = ? ORDER BY position, id`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TCourt{}
	for rows.Next() {
		var c TCourt
		if err := rows.Scan(&c.ID, &c.Name, &c.Position, &c.Active); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCourts replaces the courts by name (kept ones keep their id).
func (s *Store) SetCourts(ctx context.Context, tid int64, names []string) error {
	have, err := s.Courts(ctx, tid)
	if err != nil {
		return err
	}
	byName := map[string]TCourt{}
	for _, c := range have {
		byName[strings.ToLower(c.Name)] = c
	}
	keep := map[int64]bool{}
	for i, n := range names {
		if c, ok := byName[strings.ToLower(n)]; ok {
			keep[c.ID] = true
			if _, err := s.DB.ExecContext(ctx, `UPDATE t_courts SET name = ?, position = ?, active = 1 WHERE id = ?`, n, i, c.ID); err != nil {
				return err
			}
			continue
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO t_courts (tournament_id, name, position) VALUES (?, ?, ?)`, tid, n, i); err != nil {
			return err
		}
	}
	for _, c := range have {
		if !keep[c.ID] {
			if _, err := s.DB.ExecContext(ctx, `DELETE FROM t_courts WHERE id = ?`, c.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- entries ----

// TEntry is who plays: a player, a pair or a team.
type TEntry struct {
	ID             int64          `json:"id"`
	TournamentID   int64          `json:"tournament_id"`
	Name           string         `json:"name"`
	Status         string         `json:"status"`
	Seed           int            `json:"seed"`
	Rating         float64        `json:"rating"`
	Handicap       *float64       `json:"handicap"`
	Position       int            `json:"position"`
	ProtectedUntil string         `json:"protected_until"`
	Paid           bool           `json:"paid"`
	CheckedIn      bool           `json:"checked_in"`
	Note           string         `json:"note"`
	CreatedBy      int64          `json:"created_by"`
	CreatedAt      string         `json:"created_at"`
	WithdrawnAt    *string        `json:"withdrawn_at"`
	Members        []TEntryMember `json:"members"`
}

// TEntryMember is a person in an entry.
type TEntryMember struct {
	UserID   int64  `json:"user_id"`
	Role     string `json:"role"`
	Position int    `json:"position"`
}

// Entries lists a tournament's entries (in the order they entered) with their people.
func (s *Store) Entries(ctx context.Context, tid int64) ([]*TEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, tournament_id, name, status, seed, rating, handicap, position, protected_until, paid, checked_in, note,
		COALESCE(created_by, 0), created_at, withdrawn_at FROM t_entries WHERE tournament_id = ? ORDER BY created_at, id`, tid)
	if err != nil {
		return nil, err
	}
	out := []*TEntry{}
	byID := map[int64]*TEntry{}
	for rows.Next() {
		var e TEntry
		if err := rows.Scan(&e.ID, &e.TournamentID, &e.Name, &e.Status, &e.Seed, &e.Rating, &e.Handicap, &e.Position, &e.ProtectedUntil, &e.Paid, &e.CheckedIn,
			&e.Note, &e.CreatedBy, &e.CreatedAt, &e.WithdrawnAt); err != nil {
			rows.Close()
			return nil, err
		}
		e.Members = []TEntryMember{}
		out = append(out, &e)
		byID[e.ID] = &e
	}
	rows.Close()
	mr, err := s.DB.QueryContext(ctx, `SELECT m.entry_id, m.user_id, m.role, m.position FROM t_entry_members m JOIN t_entries e ON e.id = m.entry_id
		WHERE e.tournament_id = ? ORDER BY m.position, m.user_id`, tid)
	if err != nil {
		return nil, err
	}
	defer mr.Close()
	for mr.Next() {
		var eid int64
		var m TEntryMember
		if err := mr.Scan(&eid, &m.UserID, &m.Role, &m.Position); err != nil {
			return nil, err
		}
		if e := byID[eid]; e != nil {
			e.Members = append(e.Members, m)
		}
	}
	return out, mr.Err()
}

// SaveEntry adds (ID 0) or changes an entry and its people.
func (s *Store) SaveEntry(ctx context.Context, e *TEntry) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if e.Status == "" {
		e.Status = "in"
	}
	if e.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO t_entries (tournament_id, name, status, seed, rating, handicap, position, paid, checked_in, note, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.TournamentID, e.Name, e.Status, e.Seed, e.Rating, nullF(e.Handicap), e.Position, boolInt(e.Paid), boolInt(e.CheckedIn), e.Note, nullI(&e.CreatedBy))
		if err != nil {
			return err
		}
		e.ID, _ = res.LastInsertId()
		e.CreatedAt = Now()
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE t_entries SET name = ?, status = ?, seed = ?, rating = ?, handicap = ?, position = ?, protected_until = ?, paid = ?, checked_in = ?, note = ?,
			withdrawn_at = CASE WHEN ? IN ('withdrawn', 'disqualified') THEN COALESCE(withdrawn_at, ?) ELSE NULL END WHERE id = ?`,
			e.Name, e.Status, e.Seed, e.Rating, nullF(e.Handicap), e.Position, e.ProtectedUntil, boolInt(e.Paid), boolInt(e.CheckedIn), e.Note, e.Status, Now(), e.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM t_entry_members WHERE entry_id = ?`, e.ID); err != nil {
			return err
		}
	}
	for i, m := range e.Members {
		role := m.Role
		if role == "" {
			role = "player"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_entry_members (entry_id, user_id, role, position) VALUES (?, ?, ?, ?)`, e.ID, m.UserID, role, i); err != nil {
			return conflict(err)
		}
	}
	return tx.Commit()
}

// DeleteEntry removes an entry (only before the draw).
func (s *Store) DeleteEntry(ctx context.Context, tid, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM t_entries WHERE tournament_id = ? AND id = ?`, tid, id)
	return err
}

// ---- stages, groups and matches ----

// TStage is a part of a tournament (the groups, then the knockout).
type TStage struct {
	ID           int64           `json:"id"`
	TournamentID int64           `json:"tournament_id"`
	Seq          int             `json:"seq"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	Status       string          `json:"status"`
	Settings     json.RawMessage `json:"settings"`
}

// TGroup is a pool or a bracket of a stage.
type TGroup struct {
	ID       int64  `json:"id"`
	StageID  int64  `json:"stage_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Position int    `json:"position"`
}

// Stages lists a tournament's stages in order.
func (s *Store) Stages(ctx context.Context, tid int64) ([]*TStage, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, tournament_id, seq, kind, name, status, settings FROM t_stages WHERE tournament_id = ? ORDER BY seq`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TStage{}
	for rows.Next() {
		var st TStage
		var set string
		if err := rows.Scan(&st.ID, &st.TournamentID, &st.Seq, &st.Kind, &st.Name, &st.Status, &set); err != nil {
			return nil, err
		}
		st.Settings = json.RawMessage(set)
		out = append(out, &st)
	}
	return out, rows.Err()
}

// SaveStage adds (ID 0) or changes a stage.
func (s *Store) SaveStage(ctx context.Context, st *TStage) error {
	if len(st.Settings) == 0 {
		st.Settings = json.RawMessage("{}")
	}
	if st.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO t_stages (tournament_id, seq, kind, name, status, settings) VALUES (?, ?, ?, ?, ?, ?)`,
			st.TournamentID, st.Seq, st.Kind, st.Name, st.Status, string(st.Settings))
		if err != nil {
			return err
		}
		st.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE t_stages SET kind = ?, name = ?, status = ?, settings = ? WHERE id = ?`, st.Kind, st.Name, st.Status, string(st.Settings), st.ID)
	return err
}

// DeleteStagesFrom removes a stage and the ones after it (a new draw).
func (s *Store) DeleteStagesFrom(ctx context.Context, tid int64, seq int) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM t_stages WHERE tournament_id = ? AND seq >= ?`, tid, seq)
	return err
}

// TGroups lists the groups of a tournament.
func (s *Store) TGroups(ctx context.Context, tid int64) ([]TGroup, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT g.id, g.stage_id, g.name, g.kind, g.position FROM t_groups g JOIN t_stages s ON s.id = g.stage_id
		WHERE s.tournament_id = ? ORDER BY s.seq, g.position, g.id`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TGroup{}
	for rows.Next() {
		var g TGroup
		if err := rows.Scan(&g.ID, &g.StageID, &g.Name, &g.Kind, &g.Position); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// AddTGroup adds a group to a stage.
func (s *Store) AddTGroup(ctx context.Context, g *TGroup) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO t_groups (stage_id, name, kind, position) VALUES (?, ?, ?, ?)`, g.StageID, g.Name, g.Kind, g.Position)
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	return nil
}

// StageEntry is an entry's place in a stage.
type StageEntry struct {
	StageID   int64 `json:"stage_id"`
	GroupID   int64 `json:"group_id"`
	EntryID   int64 `json:"entry_id"`
	Seed      int   `json:"seed"`
	FinalRank int   `json:"final_rank"`
}

// SetStageEntries replaces who is in a stage (and in which group).
func (s *Store) SetStageEntries(ctx context.Context, stageID int64, list []StageEntry) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM t_stage_entries WHERE stage_id = ?`, stageID); err != nil {
		return err
	}
	for _, e := range list {
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_stage_entries (stage_id, group_id, entry_id, seed, final_rank) VALUES (?, ?, ?, ?, ?)`,
			stageID, nullI(&e.GroupID), e.EntryID, e.Seed, e.FinalRank); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// StageEntries lists who is in the stages of a tournament.
func (s *Store) StageEntries(ctx context.Context, tid int64) ([]StageEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT x.stage_id, COALESCE(x.group_id, 0), x.entry_id, x.seed, x.final_rank FROM t_stage_entries x JOIN t_stages s ON s.id = x.stage_id
		WHERE s.tournament_id = ? ORDER BY x.stage_id, x.group_id, x.seed`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StageEntry{}
	for rows.Next() {
		var e StageEntry
		if err := rows.Scan(&e.StageID, &e.GroupID, &e.EntryID, &e.Seed, &e.FinalRank); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetFinalRank notes where an entry finished a stage.
func (s *Store) SetFinalRank(ctx context.Context, stageID, entryID int64, rank int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE t_stage_entries SET final_rank = ? WHERE stage_id = ? AND entry_id = ?`, rank, stageID, entryID)
	return err
}

// TMatch is a match of a tournament.
type TMatch struct {
	ID           int64              `json:"id"`
	TournamentID int64              `json:"tournament_id"`
	StageID      int64              `json:"stage_id"`
	GroupID      int64              `json:"group_id"`
	ParentID     int64              `json:"parent_id"`
	Round        int                `json:"round"`
	Position     int                `json:"position"`
	Label        string             `json:"label"`
	AEntry       int64              `json:"a_entry"`
	BEntry       int64              `json:"b_entry"`
	ASource      string             `json:"a_source"`
	BSource      string             `json:"b_source"`
	NextMatch    int64              `json:"next_match"`
	NextSlot     string             `json:"next_slot"`
	LoserMatch   int64              `json:"loser_match"`
	LoserSlot    string             `json:"loser_slot"`
	CourtID      int64              `json:"court_id"`
	ScheduledAt  string             `json:"scheduled_at"`
	CalledAt     *string            `json:"called_at"`
	Status       string             `json:"status"`
	Outcome      string             `json:"outcome"`
	Winner       string             `json:"winner"`
	Score        Score              `json:"score"`
	ATotal       *float64           `json:"a_total"`
	BTotal       *float64           `json:"b_total"`
	White        string             `json:"white"`
	Deadline     string             `json:"deadline"`
	ReportedBy   int64              `json:"reported_by"`
	ReportedAt   *string            `json:"reported_at"`
	ConfirmedBy  int64              `json:"confirmed_by"`
	ConfirmedAt  *string            `json:"confirmed_at"`
	Dispute      string             `json:"dispute"`
	LeagueMatch  int64              `json:"league_match"`
	UpdatedAt    string             `json:"updated_at"`
	Sides        map[string][]int64 `json:"sides"` // rotating formats: "a" / "b" → entry ids
}

const tmCols = `id, tournament_id, stage_id, COALESCE(group_id, 0), COALESCE(parent_id, 0), round, position, label, COALESCE(a_entry, 0), COALESCE(b_entry, 0),
	a_source, b_source, a_entries, b_entries, COALESCE(next_match, 0), next_slot, COALESCE(loser_match, 0), loser_slot, COALESCE(court_id, 0), scheduled_at, called_at, status, outcome,
	winner, score, a_total, b_total, white, deadline, COALESCE(reported_by, 0), reported_at, COALESCE(confirmed_by, 0), confirmed_at, dispute, COALESCE(league_match, 0), updated_at`

func scanTM(row interface{ Scan(...any) error }) (*TMatch, error) {
	var m TMatch
	var score, ae, be string
	if err := row.Scan(&m.ID, &m.TournamentID, &m.StageID, &m.GroupID, &m.ParentID, &m.Round, &m.Position, &m.Label, &m.AEntry, &m.BEntry,
		&m.ASource, &m.BSource, &ae, &be, &m.NextMatch, &m.NextSlot, &m.LoserMatch, &m.LoserSlot, &m.CourtID, &m.ScheduledAt, &m.CalledAt, &m.Status, &m.Outcome,
		&m.Winner, &score, &m.ATotal, &m.BTotal, &m.White, &m.Deadline, &m.ReportedBy, &m.ReportedAt, &m.ConfirmedBy, &m.ConfirmedAt, &m.Dispute, &m.LeagueMatch, &m.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(score), &m.Score)
	m.Sides = map[string][]int64{"a": {}, "b": {}}
	var a, b []int64
	_ = json.Unmarshal([]byte(ae), &a)
	_ = json.Unmarshal([]byte(be), &b)
	if a != nil {
		m.Sides["a"] = a
	}
	if b != nil {
		m.Sides["b"] = b
	}
	return &m, nil
}

// TMatches lists a tournament's matches in order, with their players.
func (s *Store) TMatches(ctx context.Context, tid int64) ([]*TMatch, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+tmCols+` FROM t_matches WHERE tournament_id = ? ORDER BY stage_id, round, position, id`, tid)
	if err != nil {
		return nil, err
	}
	out := []*TMatch{}
	for rows.Next() {
		m, err := scanTM(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	rows.Close()
	return out, nil
}

// GetTMatch returns one match.
func (s *Store) GetTMatch(ctx context.Context, id int64) (*TMatch, error) {
	return scanTM(s.DB.QueryRowContext(ctx, `SELECT `+tmCols+` FROM t_matches WHERE id = ?`, id))
}

func tmArgs(m *TMatch) []any {
	null := func(v int64) any {
		if v == 0 {
			return nil
		}
		return v
	}
	if m.Sides == nil {
		m.Sides = map[string][]int64{}
	}
	ae, be := m.Sides["a"], m.Sides["b"]
	if ae == nil {
		ae = []int64{}
	}
	if be == nil {
		be = []int64{}
	}
	return []any{null(m.GroupID), null(m.ParentID), m.Round, m.Position, m.Label, null(m.AEntry), null(m.BEntry), m.ASource, m.BSource, toJSON(ae), toJSON(be),
		null(m.NextMatch), m.NextSlot, null(m.LoserMatch), m.LoserSlot, null(m.CourtID), m.ScheduledAt, m.CalledAt, m.Status, m.Outcome, m.Winner,
		toJSON(m.Score), nullF(m.ATotal), nullF(m.BTotal), m.White, m.Deadline, null(m.ReportedBy), m.ReportedAt, null(m.ConfirmedBy), m.ConfirmedAt,
		m.Dispute, null(m.LeagueMatch)}
}

// SaveTMatch adds (ID 0) or saves a match; players are replaced when given.
func (s *Store) SaveTMatch(ctx context.Context, m *TMatch) error {
	return s.saveTMatch(ctx, s.DB, m)
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

func (s *Store) saveTMatch(ctx context.Context, db execer, m *TMatch) error {
	if m.Status == "" {
		m.Status = "pending"
	}
	if m.ID == 0 {
		res, err := db.ExecContext(ctx, `INSERT INTO t_matches (tournament_id, stage_id, group_id, parent_id, round, position, label, a_entry, b_entry, a_source, b_source,
			a_entries, b_entries, next_match, next_slot, loser_match, loser_slot, court_id, scheduled_at, called_at, status, outcome, winner, score, a_total, b_total, white, deadline,
			reported_by, reported_at, confirmed_by, confirmed_at, dispute, league_match) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append([]any{m.TournamentID, m.StageID}, tmArgs(m)...)...)
		if err != nil {
			return err
		}
		m.ID, _ = res.LastInsertId()
	} else {
		if _, err := db.ExecContext(ctx, `UPDATE t_matches SET group_id = ?, parent_id = ?, round = ?, position = ?, label = ?, a_entry = ?, b_entry = ?, a_source = ?, b_source = ?,
			a_entries = ?, b_entries = ?, next_match = ?, next_slot = ?, loser_match = ?, loser_slot = ?, court_id = ?, scheduled_at = ?, called_at = ?, status = ?, outcome = ?, winner = ?, score = ?,
			a_total = ?, b_total = ?, white = ?, deadline = ?, reported_by = ?, reported_at = ?, confirmed_by = ?, confirmed_at = ?, dispute = ?, league_match = ?, updated_at = ?
			WHERE id = ?`, append(tmArgs(m), Now(), m.ID)...); err != nil {
			return err
		}
	}
	return nil
}

// DeleteTMatches removes matches of a stage from a round on (regenerating rounds).
func (s *Store) DeleteTMatches(ctx context.Context, stageID int64, fromRound int, onlyUnplayed bool) error {
	q := `DELETE FROM t_matches WHERE stage_id = ? AND round >= ?`
	if onlyUnplayed {
		q += ` AND status NOT IN ('done', 'reported')`
	}
	_, err := s.DB.ExecContext(ctx, q, stageID, fromRound)
	return err
}

// ---- scorecards ----

// Scorecard is one entry's round in a scored-round stage.
type Scorecard struct {
	ID        int64     `json:"id"`
	StageID   int64     `json:"stage_id"`
	Round     int       `json:"round"`
	EntryID   int64     `json:"entry_id"`
	Gross     *float64  `json:"gross"`
	Holes     []float64 `json:"holes"`
	EnteredBy int64     `json:"entered_by"`
	UpdatedAt string    `json:"updated_at"`
}

// Scorecards lists a stage's cards.
func (s *Store) Scorecards(ctx context.Context, stageID int64) ([]Scorecard, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, stage_id, round, entry_id, gross, holes, COALESCE(entered_by, 0), updated_at FROM t_scorecards WHERE stage_id = ? ORDER BY round, entry_id`, stageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Scorecard{}
	for rows.Next() {
		var c Scorecard
		var holes string
		if err := rows.Scan(&c.ID, &c.StageID, &c.Round, &c.EntryID, &c.Gross, &holes, &c.EnteredBy, &c.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(holes), &c.Holes)
		if c.Holes == nil {
			c.Holes = []float64{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PutScorecard saves a card (one per entry and round).
func (s *Store) PutScorecard(ctx context.Context, c *Scorecard) error {
	if c.Holes == nil {
		c.Holes = []float64{}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO t_scorecards (stage_id, round, entry_id, gross, holes, entered_by) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(stage_id, round, entry_id) DO UPDATE SET gross = excluded.gross, holes = excluded.holes, entered_by = excluded.entered_by, updated_at = ?`,
		c.StageID, c.Round, c.EntryID, nullF(c.Gross), toJSON(c.Holes), nullI(&c.EnteredBy), Now())
	return err
}

// ---- ladder challenges ----

// Challenge is a ladder challenge.
type Challenge struct {
	ID           int64  `json:"id"`
	TournamentID int64  `json:"tournament_id"`
	Challenger   int64  `json:"challenger"`
	Defender     int64  `json:"defender"`
	Status       string `json:"status"`
	RespondBy    string `json:"respond_by"`
	PlayBy       string `json:"play_by"`
	MatchID      int64  `json:"match_id"`
	CreatedAt    string `json:"created_at"`
}

// Challenges lists a ladder's challenges, newest first.
func (s *Store) Challenges(ctx context.Context, tid int64) ([]Challenge, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, tournament_id, challenger, defender, status, respond_by, play_by, COALESCE(match_id, 0), created_at FROM t_challenges
		WHERE tournament_id = ? ORDER BY id DESC`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Challenge{}
	for rows.Next() {
		var c Challenge
		if err := rows.Scan(&c.ID, &c.TournamentID, &c.Challenger, &c.Defender, &c.Status, &c.RespondBy, &c.PlayBy, &c.MatchID, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SaveChallenge adds (ID 0) or saves a challenge.
func (s *Store) SaveChallenge(ctx context.Context, c *Challenge) error {
	if c.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO t_challenges (tournament_id, challenger, defender, status, respond_by, play_by, match_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			c.TournamentID, c.Challenger, c.Defender, c.Status, c.RespondBy, c.PlayBy, nullI(&c.MatchID))
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE t_challenges SET status = ?, respond_by = ?, play_by = ?, match_id = ? WHERE id = ?`, c.Status, c.RespondBy, c.PlayBy, nullI(&c.MatchID), c.ID)
	return err
}

// ---- the record ----

// TLog is a line of what happened.
type TLog struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

// AddTLog notes something that happened in a tournament.
func (s *Store) AddTLog(ctx context.Context, tid, uid int64, action, detail string) {
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO t_log (tournament_id, user_id, action, detail) VALUES (?, ?, ?, ?)`, tid, nullI(&uid), action, detail)
}

// TLogs lists the last lines of a tournament's record.
func (s *Store) TLogs(ctx context.Context, tid int64, limit int) ([]TLog, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, COALESCE(user_id, 0), action, detail, created_at FROM t_log WHERE tournament_id = ? ORDER BY id DESC LIMIT ?`, tid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TLog{}
	for rows.Next() {
		var l TLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.Detail, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// TitlesOf counts the tournaments a person won, per group.
func (s *Store) TitlesOf(ctx context.Context, gid, uid int64) ([]Tournament, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+tCols+` FROM tournaments t WHERE group_id = ? AND status = 'finished' AND EXISTS (
		SELECT 1 FROM t_stage_entries x JOIN t_stages s ON s.id = x.stage_id JOIN t_entry_members m ON m.entry_id = x.entry_id
		WHERE s.tournament_id = t.id AND x.final_rank = 1 AND m.user_id = ? AND s.seq = (SELECT MAX(seq) FROM t_stages WHERE tournament_id = t.id))
		ORDER BY updated_at DESC`, gid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tournament{}
	for rows.Next() {
		t, err := scanT(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}
