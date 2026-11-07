package store

import (
	"context"
	"encoding/json"
)

// ---- matches (results) ----

const matchCols = `id, group_id, COALESCE(game_id, 0), COALESCE(user_id, 0), played_at, score, winner, note,
	COALESCE(confirmed_by, 0), confirmed_at, COALESCE(disputed_by, 0), dispute, created_at, updated_at`

func scanMatch(row interface{ Scan(...any) error }) (*Match, error) {
	var m Match
	var score string
	if err := row.Scan(&m.ID, &m.GroupID, &m.GameID, &m.UserID, &m.PlayedAt, &score, &m.Winner, &m.Note,
		&m.ConfirmedBy, &m.ConfirmedAt, &m.DisputedBy, &m.Dispute, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(score), &m.Score)
	m.Players = []MatchPlayer{}
	return &m, nil
}

// MatchFilter narrows a list of results.
type MatchFilter struct {
	GroupID int64
	GameID  int64
	UserID  int64 // only matches this person played
	From    string
	To      string
	Limit   int
	Oldest  bool // oldest first (the table is worked out in order)
}

// ListMatches lists results in the groups a person is in (uid 0 = any:
// background work only), each with its players.
func (s *Store) ListMatches(ctx context.Context, uid int64, f MatchFilter) ([]*Match, error) {
	where := `WHERE 1 = 1`
	args := []any{}
	if uid > 0 {
		where += ` AND group_id IN (SELECT group_id FROM members WHERE user_id = ?)`
		args = append(args, uid)
	}
	if f.GroupID > 0 {
		where += ` AND group_id = ?`
		args = append(args, f.GroupID)
	}
	if f.GameID > 0 {
		where += ` AND game_id = ?`
		args = append(args, f.GameID)
	}
	if f.UserID > 0 {
		where += ` AND id IN (SELECT match_id FROM match_players WHERE user_id = ?)`
		args = append(args, f.UserID)
	}
	if f.From != "" {
		where += ` AND played_at >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		where += ` AND played_at < ?`
		args = append(args, f.To)
	}
	order := ` ORDER BY played_at DESC, id DESC`
	if f.Oldest {
		order = ` ORDER BY played_at, id`
	}
	q := `SELECT ` + matchCols + ` FROM matches ` + where + order
	if f.Limit > 0 {
		q += ` LIMIT ` + itoa(int64(f.Limit))
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []*Match{}
	byID := map[int64]*Match{}
	for rows.Next() {
		m, err := scanMatch(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
		byID[m.ID] = m
	}
	rows.Close()
	if len(out) == 0 {
		return out, nil
	}
	return out, s.fillPlayers(ctx, byID, where, args)
}

func (s *Store) fillPlayers(ctx context.Context, byID map[int64]*Match, where string, args []any) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT match_id, user_id, side, score, place, stats FROM match_players
		WHERE match_id IN (SELECT id FROM matches `+where+`) ORDER BY side, place, user_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var mid int64
		var p MatchPlayer
		var stats string
		if err := rows.Scan(&mid, &p.UserID, &p.Side, &p.Score, &p.Place, &stats); err != nil {
			return err
		}
		if stats != "" && stats != "{}" {
			_ = json.Unmarshal([]byte(stats), &p.Stats)
		}
		if m := byID[mid]; m != nil {
			m.Players = append(m.Players, p)
		}
	}
	return rows.Err()
}

// GetMatch returns a result in a group the person is in.
func (s *Store) GetMatch(ctx context.Context, uid, id int64) (*Match, error) {
	m, err := scanMatch(s.DB.QueryRowContext(ctx, `SELECT `+matchCols+` FROM matches WHERE id = ? AND group_id IN (SELECT group_id FROM members WHERE user_id = ?)`, id, uid))
	if err != nil {
		return nil, err
	}
	return m, s.fillPlayers(ctx, map[int64]*Match{m.ID: m}, `WHERE id = ?`, []any{m.ID})
}

// MatchByID returns a result without a person (background work only).
func (s *Store) MatchByID(ctx context.Context, id int64) (*Match, error) {
	m, err := scanMatch(s.DB.QueryRowContext(ctx, `SELECT `+matchCols+` FROM matches WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	return m, s.fillPlayers(ctx, map[int64]*Match{m.ID: m}, `WHERE id = ?`, []any{m.ID})
}

// SaveMatch adds (ID 0) or replaces a result with its players. Changing a
// result clears its confirmation.
func (s *Store) SaveMatch(ctx context.Context, uid int64, m *Match) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	score := toJSON(m.Score)
	if m.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO matches (group_id, game_id, user_id, played_at, score, winner, note) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			m.GroupID, nullI(&m.GameID), nullI(&uid), m.PlayedAt, score, m.Winner, m.Note)
		if err != nil {
			return err
		}
		m.ID, _ = res.LastInsertId()
		m.UserID = uid
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE matches SET played_at = ?, score = ?, winner = ?, note = ?, user_id = ?, confirmed_by = NULL, confirmed_at = NULL,
			disputed_by = NULL, dispute = '', updated_at = ? WHERE id = ?`, m.PlayedAt, score, m.Winner, m.Note, nullI(&uid), Now(), m.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM match_players WHERE match_id = ?`, m.ID); err != nil {
			return err
		}
	}
	for _, p := range m.Players {
		if _, err := tx.ExecContext(ctx, `INSERT INTO match_players (match_id, user_id, side, score, place, stats) VALUES (?, ?, ?, ?, ?, ?)`,
			m.ID, p.UserID, p.Side, nullF(p.Score), p.Place, statsJSON(p.Stats)); err != nil {
			return conflict(err)
		}
	}
	return tx.Commit()
}

// ConfirmMatch marks a result as right.
func (s *Store) ConfirmMatch(ctx context.Context, id, uid int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE matches SET confirmed_by = ?, confirmed_at = ?, disputed_by = NULL, dispute = '' WHERE id = ?`, nullI(&uid), Now(), id)
	return err
}

// DisputeMatch says a result is not right (and why).
func (s *Store) DisputeMatch(ctx context.Context, id, uid int64, why string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE matches SET disputed_by = ?, dispute = ?, confirmed_by = NULL, confirmed_at = NULL WHERE id = ?`, nullI(&uid), why, id)
	return err
}

// AutoConfirm confirms the results nobody questioned for two days.
func (s *Store) AutoConfirm(ctx context.Context, before string) ([]int64, error) {
	ids, err := s.ids(ctx, `SELECT id FROM matches WHERE confirmed_at IS NULL AND disputed_by IS NULL AND created_at < ?`, before)
	if err != nil || len(ids) == 0 {
		return ids, err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE matches SET confirmed_at = ? WHERE confirmed_at IS NULL AND disputed_by IS NULL AND created_at < ?`, Now(), before)
	return ids, err
}

// DeleteMatch removes a result.
func (s *Store) DeleteMatch(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM matches WHERE id = ?`, id)
	return err
}

func statsJSON(m map[string]int) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}
