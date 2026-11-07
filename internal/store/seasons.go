package store

import (
	"context"
	"encoding/json"
)

// Season is a stretch of a group's year with its own table; when it closes
// the table and the awards are kept as they were.
type Season struct {
	ID       int64           `json:"id"`
	GroupID  int64           `json:"group_id"`
	Name     string          `json:"name"`
	StartsOn string          `json:"starts_on"`
	EndsOn   string          `json:"ends_on"` // "" = still on
	ClosedAt *string         `json:"closed_at"`
	Snapshot json.RawMessage `json:"snapshot,omitempty"`
	Awards   json.RawMessage `json:"awards"`
}

const seasonCols = `id, group_id, name, starts_on, ends_on, closed_at, snapshot, awards`

func scanSeason(row interface{ Scan(...any) error }) (*Season, error) {
	var x Season
	var snap, awards string
	if err := row.Scan(&x.ID, &x.GroupID, &x.Name, &x.StartsOn, &x.EndsOn, &x.ClosedAt, &snap, &awards); err != nil {
		return nil, notFound(err)
	}
	if snap != "" && snap != "{}" {
		x.Snapshot = json.RawMessage(snap)
	}
	if !json.Valid([]byte(awards)) {
		awards = "[]"
	}
	x.Awards = json.RawMessage(awards)
	return &x, nil
}

// Seasons lists a group's seasons, newest first.
func (s *Store) Seasons(ctx context.Context, gid int64) ([]*Season, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+seasonCols+` FROM seasons WHERE group_id = ? ORDER BY starts_on DESC, id DESC`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Season{}
	for rows.Next() {
		x, err := scanSeason(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// GetSeason loads a season.
func (s *Store) GetSeason(ctx context.Context, id int64) (*Season, error) {
	return scanSeason(s.DB.QueryRowContext(ctx, `SELECT `+seasonCols+` FROM seasons WHERE id = ?`, id))
}

// CurrentSeason is a group's season that is on today (nil when none).
func (s *Store) CurrentSeason(ctx context.Context, gid int64, today string) *Season {
	x, err := scanSeason(s.DB.QueryRowContext(ctx, `SELECT `+seasonCols+` FROM seasons WHERE group_id = ? AND closed_at IS NULL AND starts_on <= ?
		AND (ends_on = '' OR ends_on >= ?) ORDER BY starts_on DESC LIMIT 1`, gid, today, today))
	if err != nil {
		return nil
	}
	return x
}

// SaveSeason adds or changes a season.
func (s *Store) SaveSeason(ctx context.Context, x *Season) error {
	if x.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO seasons (group_id, name, starts_on, ends_on) VALUES (?, ?, ?, ?)`, x.GroupID, x.Name, x.StartsOn, x.EndsOn)
		if err != nil {
			return err
		}
		x.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE seasons SET name = ?, starts_on = ?, ends_on = ? WHERE id = ?`, x.Name, x.StartsOn, x.EndsOn, x.ID)
	return err
}

// CloseSeason keeps a season's table and awards as they are.
func (s *Store) CloseSeason(ctx context.Context, id int64, endsOn string, snapshot, awards any) error {
	sj, _ := json.Marshal(snapshot)
	aj, _ := json.Marshal(awards)
	_, err := s.DB.ExecContext(ctx, `UPDATE seasons SET ends_on = ?, closed_at = ?, snapshot = ?, awards = ? WHERE id = ?`, endsOn, Now(), string(sj), string(aj), id)
	return err
}

// ReopenSeason makes a closed season live again.
func (s *Store) ReopenSeason(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE seasons SET closed_at = NULL, snapshot = '{}', awards = '[]' WHERE id = ?`, id)
	return err
}

// DeleteSeason removes a season (the results stay).
func (s *Store) DeleteSeason(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM seasons WHERE id = ?`, id)
	return err
}
