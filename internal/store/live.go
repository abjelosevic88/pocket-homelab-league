package store

import (
	"context"
	"encoding/json"
)

// LiveScore is a match being scored point by point.
type LiveScore struct {
	ID         int64           `json:"id"`
	GroupID    int64           `json:"group_id"`
	GameID     int64           `json:"game_id"`
	TMatchID   int64           `json:"tmatch_id"`
	AName      string          `json:"a_name"`
	BName      string          `json:"b_name"`
	AIDs       []int64         `json:"a_ids"`
	BIDs       []int64         `json:"b_ids"`
	Rules      json.RawMessage `json:"rules"`
	Points     []string        `json:"points"`
	Scorer     int64           `json:"scorer"`
	StartedAt  string          `json:"started_at"`
	UpdatedAt  string          `json:"updated_at"`
	FinishedAt *string         `json:"finished_at"`
}

const liveCols = `id, group_id, COALESCE(game_id, 0), COALESCE(tmatch_id, 0), a_name, b_name, a_ids, b_ids, rules, state, COALESCE(scorer, 0), started_at, updated_at, finished_at`

func scanLive(row interface{ Scan(...any) error }) (*LiveScore, error) {
	var l LiveScore
	var a, b, rules, state string
	if err := row.Scan(&l.ID, &l.GroupID, &l.GameID, &l.TMatchID, &l.AName, &l.BName, &a, &b, &rules, &state, &l.Scorer, &l.StartedAt, &l.UpdatedAt, &l.FinishedAt); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(a), &l.AIDs)
	_ = json.Unmarshal([]byte(b), &l.BIDs)
	var st struct {
		Points []string `json:"points"`
	}
	_ = json.Unmarshal([]byte(state), &st)
	l.Points = st.Points
	if l.Points == nil {
		l.Points = []string{}
	}
	if l.AIDs == nil {
		l.AIDs = []int64{}
	}
	if l.BIDs == nil {
		l.BIDs = []int64{}
	}
	l.Rules = json.RawMessage(rules)
	return &l, nil
}

// AddLive starts a live score.
func (s *Store) AddLive(ctx context.Context, l *LiveScore) error {
	a, _ := json.Marshal(l.AIDs)
	b, _ := json.Marshal(l.BIDs)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO live_scores (group_id, game_id, tmatch_id, a_name, b_name, a_ids, b_ids, rules, state, scorer) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{"points":[]}', ?)`,
		l.GroupID, nullID(l.GameID), nullID(l.TMatchID), l.AName, l.BName, string(a), string(b), string(l.Rules), nullID(l.Scorer))
	if err != nil {
		return err
	}
	l.ID, _ = res.LastInsertId()
	return nil
}

// GetLive loads a live score.
func (s *Store) GetLive(ctx context.Context, id int64) (*LiveScore, error) {
	return scanLive(s.DB.QueryRowContext(ctx, `SELECT `+liveCols+` FROM live_scores WHERE id = ?`, id))
}

// LiveIn lists a group's live scores that are still going (and those
// finished in the last hour, so a result stays up for a bit).
func (s *Store) LiveIn(ctx context.Context, gid int64) ([]*LiveScore, error) {
	return s.lives(ctx, `WHERE group_id = ? AND (finished_at IS NULL OR finished_at > ?) AND updated_at > ? ORDER BY id DESC`, gid, Ago(1), Ago(12))
}

// LiveForTournament lists the running live scores of a tournament's matches.
func (s *Store) LiveForTournament(ctx context.Context, tid int64) ([]*LiveScore, error) {
	return s.lives(ctx, `WHERE finished_at IS NULL AND updated_at > ? AND tmatch_id IN (SELECT id FROM t_matches WHERE tournament_id = ?) ORDER BY id`, Ago(12), tid)
}

func (s *Store) lives(ctx context.Context, where string, args ...any) ([]*LiveScore, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+liveCols+` FROM live_scores `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*LiveScore{}
	for rows.Next() {
		l, err := scanLive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SaveLivePoints keeps the points (and who scores).
func (s *Store) SaveLivePoints(ctx context.Context, id, scorer int64, points []string) error {
	b, _ := json.Marshal(map[string]any{"points": points})
	_, err := s.DB.ExecContext(ctx, `UPDATE live_scores SET state = ?, scorer = ?, updated_at = ? WHERE id = ?`, string(b), nullID(scorer), Now(), id)
	return err
}

// FinishLive marks a live score over.
func (s *Store) FinishLive(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE live_scores SET finished_at = ?, updated_at = ? WHERE id = ? AND finished_at IS NULL`, Now(), Now(), id)
	return err
}

// DeleteLive drops a live score.
func (s *Store) DeleteLive(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM live_scores WHERE id = ?`, id)
	return err
}
