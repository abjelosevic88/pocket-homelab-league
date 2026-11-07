package store

import (
	"context"
	"database/sql"
	"errors"
)

// ---- guests (+1) ----

// Guest is someone's friend for one game, not in the group.
type Guest struct {
	ID        int64  `json:"id"`
	GameID    int64  `json:"game_id"`
	AddedBy   int64  `json:"added_by"`
	Name      string `json:"name"`
	Answer    string `json:"answer"` // in | waiting
	CreatedAt string `json:"created_at"`
}

// Guests lists a game's guests, in the order they were added.
func (s *Store) Guests(ctx context.Context, gameID int64) ([]Guest, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, game_id, COALESCE(added_by, 0), name, answer, created_at FROM game_guests WHERE game_id = ? ORDER BY id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Guest{}
	for rows.Next() {
		var g Guest
		if err := rows.Scan(&g.ID, &g.GameID, &g.AddedBy, &g.Name, &g.Answer, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// AddGuest adds a guest to a game.
func (s *Store) AddGuest(ctx context.Context, g *Guest) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO game_guests (game_id, added_by, name, answer) VALUES (?, ?, ?, ?)`, g.GameID, g.AddedBy, g.Name, g.Answer)
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	return nil
}

// GetGuest loads one guest.
func (s *Store) GetGuest(ctx context.Context, id int64) (*Guest, error) {
	var g Guest
	err := s.DB.QueryRowContext(ctx, `SELECT id, game_id, COALESCE(added_by, 0), name, answer, created_at FROM game_guests WHERE id = ?`, id).
		Scan(&g.ID, &g.GameID, &g.AddedBy, &g.Name, &g.Answer, &g.CreatedAt)
	return &g, notFound(err)
}

// SetGuestAnswer moves a guest in or to the waiting list.
func (s *Store) SetGuestAnswer(ctx context.Context, id int64, answer string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE game_guests SET answer = ? WHERE id = ?`, answer, id)
	return err
}

// DeleteGuest removes a guest.
func (s *Store) DeleteGuest(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM game_guests WHERE id = ?`, id)
	return err
}

// ---- reliability ----

// LogAnswer notes a change of answer; late = dropped out less than a day before.
func (s *Store) LogAnswer(ctx context.Context, gameID, uid int64, from, to string, late bool) {
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO answer_log (game_id, user_id, from_a, to_a, late) VALUES (?, ?, ?, ?, ?)`, gameID, uid, from, to, late)
}

// SetNoShow marks that someone who was in did not come.
func (s *Store) SetNoShow(ctx context.Context, gameID, uid int64, v bool) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE answers SET no_show = ? WHERE game_id = ? AND user_id = ? AND answer = 'in'`, v, gameID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Reliability is how someone shows up for a group's games.
type Reliability struct {
	UserID   int64 `json:"user_id"`
	Games    int   `json:"games"`     // past games they could answer
	Came     int   `json:"came"`      // in, and came
	LateOuts int   `json:"late_outs"` // dropped out less than a day before
	NoShows  int   `json:"no_shows"`  // in, and did not come
	NoAnswer int   `json:"no_answer"` // never answered
	Score    int   `json:"score"`     // 0–100, -1 when too few games
}

// sinceJoined is when someone's games in a group start counting: when they
// joined, or their first answer if that was earlier (people imported later).
const sinceJoined = `min(substr(m.joined_at, 1, 16), COALESCE((SELECT MIN(g2.starts_at) FROM answers a2 JOIN games g2 ON g2.id = a2.game_id
	WHERE a2.user_id = m.user_id AND g2.group_id = m.group_id), '9999'))`

// GroupReliability counts, for everyone in a group, the past planned games
// since they joined.
func (s *Store) GroupReliability(ctx context.Context, gid int64, before string) (map[int64]*Reliability, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT m.user_id,
		(SELECT COUNT(1) FROM games g WHERE g.group_id = m.group_id AND g.status = 'planned' AND g.starts_at < ? AND g.starts_at >= `+sinceJoined+`),
		(SELECT COUNT(1) FROM answers a JOIN games g ON g.id = a.game_id WHERE g.group_id = m.group_id AND g.status = 'planned' AND g.starts_at < ? AND a.user_id = m.user_id AND a.answer = 'in' AND a.no_show = 0),
		(SELECT COUNT(DISTINCT l.game_id) FROM answer_log l JOIN games g ON g.id = l.game_id WHERE g.group_id = m.group_id AND l.user_id = m.user_id AND l.late = 1
			AND COALESCE((SELECT a.answer FROM answers a WHERE a.game_id = l.game_id AND a.user_id = l.user_id), '') <> 'in'),
		(SELECT COUNT(1) FROM answers a JOIN games g ON g.id = a.game_id WHERE g.group_id = m.group_id AND a.user_id = m.user_id AND a.no_show = 1),
		(SELECT COUNT(1) FROM games g WHERE g.group_id = m.group_id AND g.status = 'planned' AND g.starts_at < ? AND g.starts_at >= `+sinceJoined+`
			AND NOT EXISTS (SELECT 1 FROM answers a WHERE a.game_id = g.id AND a.user_id = m.user_id))
		FROM members m WHERE m.group_id = ?`, before, before, before, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*Reliability{}
	for rows.Next() {
		var r Reliability
		if err := rows.Scan(&r.UserID, &r.Games, &r.Came, &r.LateOuts, &r.NoShows, &r.NoAnswer); err != nil {
			return nil, err
		}
		r.Score = -1
		if d := r.Came + r.LateOuts + r.NoShows; d >= 3 {
			r.Score = r.Came * 100 / d
		}
		out[r.UserID] = &r
	}
	return out, rows.Err()
}

// ---- away ----

// Away is a stretch of days someone can't play.
type Away struct {
	ID      int64  `json:"id"`
	UserID  int64  `json:"-"`
	From    string `json:"from"`
	To      string `json:"to"`
	Note    string `json:"note"`
	Created string `json:"created_at"`
}

// AwayList is a person's away days that haven't ended (all with past = true).
func (s *Store) AwayList(ctx context.Context, uid int64, past bool) ([]Away, error) {
	q := `SELECT id, user_id, from_day, to_day, note, created_at FROM away WHERE user_id = ?`
	if !past {
		q += ` AND to_day >= ?`
	}
	args := []any{uid}
	if !past {
		args = append(args, LocalNow()[:10])
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY from_day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Away{}
	for rows.Next() {
		var a Away
		if err := rows.Scan(&a.ID, &a.UserID, &a.From, &a.To, &a.Note, &a.Created); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AddAway saves away days.
func (s *Store) AddAway(ctx context.Context, a *Away) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO away (user_id, from_day, to_day, note) VALUES (?, ?, ?, ?)`, a.UserID, a.From, a.To, a.Note)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

// DeleteAway removes a person's own away days.
func (s *Store) DeleteAway(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM away WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AwayOn is who of a group is away on a day, with their note.
func (s *Store) AwayOn(ctx context.Context, gid int64, day string) (map[int64]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT w.user_id, w.note FROM away w JOIN members m ON m.user_id = w.user_id AND m.group_id = ?
		WHERE w.from_day <= ? AND w.to_day >= ?`, gid, day, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var uid int64
		var n string
		if rows.Scan(&uid, &n) == nil {
			out[uid] = n
		}
	}
	return out, rows.Err()
}

// ---- finding a sub ----

// SubOffer is a place in a game offered to someone.
type SubOffer struct {
	ID        int64   `json:"id"`
	GameID    int64   `json:"game_id"`
	UserID    int64   `json:"user_id"`
	OfferedBy int64   `json:"offered_by"`
	CreatedAt string  `json:"created_at"`
	TakenAt   *string `json:"taken_at"`
}

// OfferSub offers a place to someone (once per game); it reports whether it is new.
func (s *Store) OfferSub(ctx context.Context, gameID, uid, by int64) bool {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO sub_offers (game_id, user_id, offered_by) VALUES (?, ?, ?) ON CONFLICT(game_id, user_id) DO NOTHING`, gameID, uid, by)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// OpenSubOffers are the places offered to a person in games still to come.
func (s *Store) OpenSubOffers(ctx context.Context, uid int64) ([]SubOffer, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT o.id, o.game_id, o.user_id, COALESCE(o.offered_by, 0), o.created_at, o.taken_at FROM sub_offers o JOIN games g ON g.id = o.game_id
		WHERE o.user_id = ? AND o.taken_at IS NULL AND g.status = 'planned' AND g.starts_at > ? ORDER BY g.starts_at`, uid, LocalNow())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SubOffer{}
	for rows.Next() {
		var o SubOffer
		if err := rows.Scan(&o.ID, &o.GameID, &o.UserID, &o.OfferedBy, &o.CreatedAt, &o.TakenAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// GetSubOffer loads an offer.
func (s *Store) GetSubOffer(ctx context.Context, id int64) (*SubOffer, error) {
	var o SubOffer
	err := s.DB.QueryRowContext(ctx, `SELECT id, game_id, user_id, COALESCE(offered_by, 0), created_at, taken_at FROM sub_offers WHERE id = ?`, id).
		Scan(&o.ID, &o.GameID, &o.UserID, &o.OfferedBy, &o.CreatedAt, &o.TakenAt)
	return &o, notFound(err)
}

// TakeSubOffer marks an offer taken (false when it already was).
func (s *Store) TakeSubOffer(ctx context.Context, gameID, uid int64) bool {
	res, err := s.DB.ExecContext(ctx, `UPDATE sub_offers SET taken_at = ? WHERE game_id = ? AND user_id = ? AND taken_at IS NULL`, Now(), gameID, uid)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// SubOffersFor counts a game's offers: asked, taken.
func (s *Store) SubOffersFor(ctx context.Context, gameID int64) (asked, taken int) {
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1), COUNT(taken_at) FROM sub_offers WHERE game_id = ?`, gameID).Scan(&asked, &taken)
	return
}

// ---- chores ----

// Duty is a chore in a group that people take turns doing.
type Duty struct {
	ID       int64  `json:"id"`
	GroupID  int64  `json:"group_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// GameDuty is who does a chore for one game.
type GameDuty struct {
	DutyID int64  `json:"duty_id"`
	Name   string `json:"name"`
	UserID int64  `json:"user_id"`
	Done   bool   `json:"done"`
}

// Duties lists a group's chores.
func (s *Store) Duties(ctx context.Context, gid int64) ([]Duty, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, group_id, name, position FROM duties WHERE group_id = ? ORDER BY position, id`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Duty{}
	for rows.Next() {
		var d Duty
		if err := rows.Scan(&d.ID, &d.GroupID, &d.Name, &d.Position); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetDuties replaces a group's chores by name, keeping the ones still named.
func (s *Store) SetDuties(ctx context.Context, gid int64, names []string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := map[string]bool{}
	for i, n := range names {
		keep[n] = true
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM duties WHERE group_id = ? AND name = ?`, gid, n).Scan(&id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `INSERT INTO duties (group_id, name, position) VALUES (?, ?, ?)`, gid, n, i); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if _, err := tx.ExecContext(ctx, `UPDATE duties SET position = ? WHERE id = ?`, i, id); err != nil {
				return err
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, name FROM duties WHERE group_id = ?`, gid)
	if err != nil {
		return err
	}
	drop := []int64{}
	for rows.Next() {
		var id int64
		var n string
		if rows.Scan(&id, &n) == nil && !keep[n] {
			drop = append(drop, id)
		}
	}
	rows.Close()
	for _, id := range drop {
		if _, err := tx.ExecContext(ctx, `DELETE FROM duties WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GameDuties is who does what for a game (every chore, someone or not).
func (s *Store) GameDuties(ctx context.Context, gid, gameID int64) ([]GameDuty, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT d.id, d.name, COALESCE(x.user_id, 0), COALESCE(x.done, 0) FROM duties d
		LEFT JOIN game_duties x ON x.duty_id = d.id AND x.game_id = ? WHERE d.group_id = ? ORDER BY d.position, d.id`, gameID, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameDuty{}
	for rows.Next() {
		var d GameDuty
		if err := rows.Scan(&d.DutyID, &d.Name, &d.UserID, &d.Done); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetGameDuty gives a chore of a game to someone (0 = nobody).
func (s *Store) SetGameDuty(ctx context.Context, gameID, dutyID, uid int64) error {
	var who any
	if uid > 0 {
		who = uid
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO game_duties (game_id, duty_id, user_id, done) VALUES (?, ?, ?, 0)
		ON CONFLICT(game_id, duty_id) DO UPDATE SET user_id = excluded.user_id, done = 0`, gameID, dutyID, who)
	return err
}

// SetDutyDone ticks a chore off.
func (s *Store) SetDutyDone(ctx context.Context, gameID, dutyID int64, done bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE game_duties SET done = ? WHERE game_id = ? AND duty_id = ?`, done, gameID, dutyID)
	return err
}

// DutyTurns is, for a chore, how often each person did it and when last.
func (s *Store) DutyTurns(ctx context.Context, dutyID int64) (map[int64]int, map[int64]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT x.user_id, COUNT(1), MAX(g.starts_at) FROM game_duties x JOIN games g ON g.id = x.game_id
		WHERE x.duty_id = ? AND x.user_id IS NOT NULL AND g.status = 'planned' GROUP BY x.user_id`, dutyID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	n, last := map[int64]int{}, map[int64]string{}
	for rows.Next() {
		var uid int64
		var c int
		var l string
		if rows.Scan(&uid, &c, &l) == nil {
			n[uid], last[uid] = c, l
		}
	}
	return n, last, rows.Err()
}
