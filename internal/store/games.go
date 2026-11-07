package store

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// LocalNow is now in the box's time, as games store it (YYYY-MM-DDTHH:MM).
func LocalNow() string { return time.Now().Format("2006-01-02T15:04") }

// ---- games ----

const gameCols = `g.id, g.group_id, COALESCE(g.user_id, 0), g.status, g.starts_at, g.duration_min, g.title, g.notes,
	g.venue_name, g.venue_address, g.venue_lat, g.venue_lon, g.min_players, g.max_players, g.cost_cents, g.currency,
	g.vote_until, g.cancel_reason, g.auto, g.sent, g.created_at, g.updated_at,
	(SELECT COUNT(1) FROM answers a WHERE a.game_id = g.id AND a.answer = 'in') + (SELECT COUNT(1) FROM game_guests x WHERE x.game_id = g.id AND x.answer = 'in'),
	(SELECT COUNT(1) FROM answers a WHERE a.game_id = g.id AND a.answer = 'maybe'),
	(SELECT COUNT(1) FROM answers a WHERE a.game_id = g.id AND a.answer = 'out'),
	(SELECT COUNT(1) FROM answers a WHERE a.game_id = g.id AND a.answer = 'waiting') + (SELECT COUNT(1) FROM game_guests x WHERE x.game_id = g.id AND x.answer = 'waiting'),
	COALESCE((SELECT a.answer FROM answers a WHERE a.game_id = g.id AND a.user_id = ?), ''),
	(SELECT COUNT(1) FROM matches x WHERE x.game_id = g.id),
	(SELECT COUNT(1) FROM messages c WHERE c.game_id = g.id AND c.kind = 'text' AND c.deleted_at IS NULL),
	(SELECT COUNT(1) FROM game_guests x WHERE x.game_id = g.id)`

func scanGame(row interface{ Scan(...any) error }) (*Game, error) {
	var g Game
	if err := row.Scan(&g.ID, &g.GroupID, &g.UserID, &g.Status, &g.StartsAt, &g.DurationMin, &g.Title, &g.Notes,
		&g.VenueName, &g.VenueAddress, &g.VenueLat, &g.VenueLon, &g.MinPlayers, &g.MaxPlayers, &g.CostCents, &g.Currency,
		&g.VoteUntil, &g.CancelReason, &g.Auto, &g.Sent, &g.CreatedAt, &g.UpdatedAt,
		&g.In, &g.Maybe, &g.Out, &g.Waiting, &g.MyAnswer, &g.Matches, &g.Comments, &g.Guests); err != nil {
		return nil, notFound(err)
	}
	return &g, nil
}

func (s *Store) games(ctx context.Context, uid int64, where string, args ...any) ([]*Game, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+gameCols+` FROM games g `+where, append([]any{uid}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Game{}
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GameFilter narrows a list of games.
type GameFilter struct {
	GroupID  int64
	From, To string // local YYYY-MM-DDTHH:MM, "" = open
	Upcoming bool   // planned or voting, not over yet
	Past     bool   // started before now (any status but voting)
	Limit    int
}

// ListGames lists the games of the groups a person is in.
func (s *Store) ListGames(ctx context.Context, uid int64, f GameFilter) ([]*Game, error) {
	where := `WHERE g.group_id IN (SELECT group_id FROM members WHERE user_id = ?)`
	args := []any{uid}
	if f.GroupID > 0 {
		where += ` AND g.group_id = ?`
		args = append(args, f.GroupID)
	}
	now := LocalNow()
	order := ` ORDER BY g.starts_at, g.id`
	switch {
	case f.Upcoming:
		// a game is upcoming until it ended (so one that is on now shows)
		where += ` AND (g.status = 'voting' OR (g.status = 'planned' AND strftime('%Y-%m-%dT%H:%M', g.starts_at, '+' || g.duration_min || ' minutes') >= ?))`
		args = append(args, now)
		order = ` ORDER BY g.status = 'voting', g.starts_at, g.id`
	case f.Past:
		where += ` AND g.status <> 'voting' AND g.starts_at < ?`
		args = append(args, now)
		order = ` ORDER BY g.starts_at DESC, g.id DESC`
	}
	if f.From != "" {
		where += ` AND g.starts_at >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		where += ` AND g.starts_at < ?`
		args = append(args, f.To)
	}
	q := where + order
	if f.Limit > 0 {
		q += ` LIMIT ` + itoa(int64(f.Limit))
	}
	return s.games(ctx, uid, q, args...)
}

// GetGame returns a game of a group the person is in.
func (s *Store) GetGame(ctx context.Context, uid, id int64) (*Game, error) {
	return scanGame(s.DB.QueryRowContext(ctx, `SELECT `+gameCols+` FROM games g WHERE g.id = ? AND g.group_id IN (SELECT group_id FROM members WHERE user_id = ?)`, uid, id, uid))
}

// GameByID returns a game without a person (background work only).
func (s *Store) GameByID(ctx context.Context, id int64) (*Game, error) {
	return scanGame(s.DB.QueryRowContext(ctx, `SELECT `+gameCols+` FROM games g WHERE g.id = ?`, 0, id))
}

// GamesToWatch are the games the reminders look at: planned ones starting
// in the next two days, and votes still open.
func (s *Store) GamesToWatch(ctx context.Context) ([]*Game, error) {
	now := time.Now()
	return s.games(ctx, 0, `WHERE (g.status = 'planned' AND g.starts_at >= ? AND g.starts_at < ?) OR g.status = 'voting' ORDER BY g.starts_at`,
		now.Add(-30*time.Hour).Format("2006-01-02T15:04"), now.Add(49*time.Hour).Format("2006-01-02T15:04"))
}

// HasGameAt reports whether a group already has a game starting on that day.
func (s *Store) HasGameAt(ctx context.Context, gid int64, date string) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM games WHERE group_id = ? AND substr(starts_at, 1, 10) = ?`, gid, date).Scan(&n)
	return n > 0
}

func gameArgs(g *Game) []any {
	return []any{g.Status, g.StartsAt, g.DurationMin, g.Title, g.Notes, g.VenueName, g.VenueAddress, nullF(g.VenueLat), nullF(g.VenueLon),
		g.MinPlayers, g.MaxPlayers, g.CostCents, g.Currency, g.VoteUntil, g.CancelReason, boolInt(g.Auto)}
}

// CreateGame adds a game to a group.
func (s *Store) CreateGame(ctx context.Context, uid int64, g *Game) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO games (group_id, user_id, status, starts_at, duration_min, title, notes, venue_name, venue_address, venue_lat, venue_lon,
		min_players, max_players, cost_cents, currency, vote_until, cancel_reason, auto) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append([]any{g.GroupID, nullI(&uid)}, gameArgs(g)...)...)
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	g.UserID = uid
	return nil
}

// UpdateGame saves a game.
func (s *Store) UpdateGame(ctx context.Context, g *Game) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE games SET status = ?, starts_at = ?, duration_min = ?, title = ?, notes = ?, venue_name = ?, venue_address = ?,
		venue_lat = ?, venue_lon = ?, min_players = ?, max_players = ?, cost_cents = ?, currency = ?, vote_until = ?, cancel_reason = ?, auto = ?, updated_at = ?
		WHERE id = ?`, append(gameArgs(g), Now(), g.ID)...)
	return err
}

// MarkSent remembers a reminder went out for a game.
func (s *Store) MarkSent(ctx context.Context, id int64, what string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE games SET sent = trim(sent || ' ' || ?) WHERE id = ?`, what, id)
	return err
}

// ClearSent forgets the reminders (the game moved).
func (s *Store) ClearSent(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE games SET sent = '' WHERE id = ?`, id)
	return err
}

// WasSent reports whether a reminder already went out.
func (g *Game) WasSent(what string) bool {
	for _, w := range strings.Fields(g.Sent) {
		if w == what {
			return true
		}
	}
	return false
}

// DeleteGame removes a game (its results stay, without the game).
func (s *Store) DeleteGame(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM games WHERE id = ?`, id)
	return err
}

// ---- answers ----

// Answers lists who answered for a game, in the order they answered.
func (s *Store) Answers(ctx context.Context, gameID int64) ([]Answer, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT game_id, user_id, answer, side, note, answered_at, no_show FROM answers WHERE game_id = ? ORDER BY answered_at, user_id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Answer{}
	for rows.Next() {
		var a Answer
		if err := rows.Scan(&a.GameID, &a.UserID, &a.Answer, &a.Side, &a.Note, &a.AnsweredAt, &a.NoShow); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAnswer is one person's answer ("" when none).
func (s *Store) GetAnswer(ctx context.Context, gameID, uid int64) string {
	var a string
	_ = s.DB.QueryRowContext(ctx, `SELECT answer FROM answers WHERE game_id = ? AND user_id = ?`, gameID, uid).Scan(&a)
	return a
}

// SetAnswer saves a person's answer. The time only moves when the answer
// changes, so the waiting list keeps its order.
func (s *Store) SetAnswer(ctx context.Context, gameID, uid int64, answer, note string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO answers (game_id, user_id, answer, note, answered_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(game_id, user_id) DO UPDATE SET note = excluded.note,
		answered_at = CASE WHEN answers.answer = excluded.answer THEN answers.answered_at ELSE excluded.answered_at END,
		answer = excluded.answer`, gameID, uid, answer, note, Now())
	return err
}

// ClearAnswer removes a person's answer.
func (s *Store) ClearAnswer(ctx context.Context, gameID, uid int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM answers WHERE game_id = ? AND user_id = ?`, gameID, uid)
	return err
}

// SetSides saves the teams for the day (user id → a | b | "").
func (s *Store) SetSides(ctx context.Context, gameID int64, sides map[int64]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE answers SET side = '' WHERE game_id = ?`, gameID); err != nil {
		return err
	}
	for uid, side := range sides {
		if _, err := tx.ExecContext(ctx, `UPDATE answers SET side = ? WHERE game_id = ? AND user_id = ?`, side, gameID, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- finding a time ----

// Options lists the times (and places) offered for a game with everyone's votes.
func (s *Store) Options(ctx context.Context, gameID int64) ([]*Option, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, game_id, starts_at, venue_name, venue_address, venue_lat, venue_lon FROM options WHERE game_id = ? ORDER BY starts_at, venue_name`, gameID)
	if err != nil {
		return nil, err
	}
	out := []*Option{}
	byID := map[int64]*Option{}
	for rows.Next() {
		o := &Option{Votes: map[string]string{}}
		if err := rows.Scan(&o.ID, &o.GameID, &o.StartsAt, &o.VenueName, &o.VenueAddress, &o.VenueLat, &o.VenueLon); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, o)
		byID[o.ID] = o
	}
	rows.Close()
	vr, err := s.DB.QueryContext(ctx, `SELECT v.option_id, v.user_id, v.answer FROM votes v JOIN options o ON o.id = v.option_id WHERE o.game_id = ?`, gameID)
	if err != nil {
		return nil, err
	}
	defer vr.Close()
	for vr.Next() {
		var oid, uid int64
		var a string
		if err := vr.Scan(&oid, &uid, &a); err != nil {
			return nil, err
		}
		if o := byID[oid]; o != nil {
			o.Votes[itoa(uid)] = a
		}
	}
	return out, vr.Err()
}

// SetOptions replaces the times and places offered; an option kept (same
// time, same place) keeps its votes.
func (s *Store) SetOptions(ctx context.Context, gameID int64, opts []OptionIn) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := map[string]bool{}
	for _, o := range opts {
		keep[o.Key()] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO options (game_id, starts_at, venue_name, venue_address, venue_lat, venue_lon) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(game_id, starts_at, venue_name) DO UPDATE SET venue_address = excluded.venue_address, venue_lat = excluded.venue_lat, venue_lon = excluded.venue_lon`,
			gameID, o.StartsAt, strings.TrimSpace(o.VenueName), o.VenueAddress, nullF(o.VenueLat), nullF(o.VenueLon)); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, starts_at, venue_name FROM options WHERE game_id = ?`, gameID)
	if err != nil {
		return err
	}
	var drop []int64
	for rows.Next() {
		var id int64
		var t, v string
		if err := rows.Scan(&id, &t, &v); err != nil {
			rows.Close()
			return err
		}
		if !keep[OptionIn{StartsAt: t, VenueName: v}.Key()] {
			drop = append(drop, id)
		}
	}
	rows.Close()
	for _, id := range drop {
		if _, err := tx.ExecContext(ctx, `DELETE FROM options WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetVotes saves one person's votes on a game's times (option id → answer).
func (s *Store) SetVotes(ctx context.Context, gameID, uid int64, votes map[int64]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for oid, a := range votes {
		var ok int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM options WHERE id = ? AND game_id = ?`, oid, gameID).Scan(&ok); err != nil || ok == 0 {
			continue
		}
		if a == "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM votes WHERE option_id = ? AND user_id = ?`, oid, uid); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO votes (option_id, user_id, answer) VALUES (?, ?, ?) ON CONFLICT(option_id, user_id) DO UPDATE SET answer = excluded.answer`, oid, uid, a); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- player of the game ----

// MVPVotes lists who voted for whom (voter → chosen).
func (s *Store) MVPVotes(ctx context.Context, gameID int64) (map[int64]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id, for_id FROM mvp_votes WHERE game_id = ?`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[a] = b
	}
	return out, rows.Err()
}

// SetMVPVote saves (or, with 0, removes) one person's vote.
func (s *Store) SetMVPVote(ctx context.Context, gameID, uid, forID int64) error {
	if forID == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM mvp_votes WHERE game_id = ? AND user_id = ?`, gameID, uid)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO mvp_votes (game_id, user_id, for_id) VALUES (?, ?, ?) ON CONFLICT(game_id, user_id) DO UPDATE SET for_id = excluded.for_id`, gameID, uid, forID)
	return err
}

// GroupMVPs counts player-of-the-game wins per person in a group: whoever
// has the most votes in a game (a tie counts for each).
func (s *Store) GroupMVPs(ctx context.Context, gid int64, from string) (map[int64]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT v.game_id, v.for_id, COUNT(1) FROM mvp_votes v JOIN games g ON g.id = v.game_id
		WHERE g.group_id = ? AND g.starts_at >= ? GROUP BY v.game_id, v.for_id`, gid, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type c struct {
		uid int64
		n   int
	}
	best := map[int64][]c{}
	for rows.Next() {
		var g, u int64
		var n int
		if err := rows.Scan(&g, &u, &n); err != nil {
			return nil, err
		}
		cur := best[g]
		switch {
		case len(cur) == 0 || n > cur[0].n:
			best[g] = []c{{u, n}}
		case n == cur[0].n:
			best[g] = append(cur, c{u, n})
		}
	}
	out := map[int64]int{}
	for _, cs := range best {
		for _, x := range cs {
			out[x.uid]++
		}
	}
	return out, rows.Err()
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
