package store

import (
	"context"
	"strings"
)

// Venue is a place a group plays at.
type Venue struct {
	ID        int64    `json:"id"`
	GroupID   int64    `json:"group_id"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	Lat       *float64 `json:"lat"`
	Lon       *float64 `json:"lon"`
	Ref       string   `json:"ref"`
	CreatedAt string   `json:"created_at"`
}

// MaxVenues is how many places a group keeps.
const MaxVenues = 30

// Venues lists a group's places by name.
func (s *Store) Venues(ctx context.Context, gid int64) ([]Venue, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, group_id, name, address, lat, lon, ref, created_at FROM venues WHERE group_id = ? ORDER BY name COLLATE NOCASE`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Venue{}
	for rows.Next() {
		var v Venue
		if err := rows.Scan(&v.ID, &v.GroupID, &v.Name, &v.Address, &v.Lat, &v.Lon, &v.Ref, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVenue is one place of a group.
func (s *Store) GetVenue(ctx context.Context, gid, id int64) (*Venue, error) {
	var v Venue
	err := s.DB.QueryRowContext(ctx, `SELECT id, group_id, name, address, lat, lon, ref, created_at FROM venues WHERE group_id = ? AND id = ?`, gid, id).
		Scan(&v.ID, &v.GroupID, &v.Name, &v.Address, &v.Lat, &v.Lon, &v.Ref, &v.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &v, nil
}

// EnsureVenue keeps a place in a group's list: a new name is added (while
// there is room), a known one gets the address and position when it had none.
func (s *Store) EnsureVenue(ctx context.Context, gid int64, v Venue) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" {
		return nil
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM venues WHERE group_id = ? AND name = ? COLLATE NOCASE`, gid, v.Name).Scan(&id)
	if err == nil {
		_, err = s.DB.ExecContext(ctx, `UPDATE venues SET address = CASE WHEN address = '' THEN ? ELSE address END,
			lat = COALESCE(lat, ?), lon = COALESCE(lon, ?), ref = CASE WHEN ref = '' THEN ? ELSE ref END WHERE id = ?`,
			v.Address, nullF(v.Lat), nullF(v.Lon), v.Ref, id)
		return err
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM venues WHERE group_id = ?`, gid).Scan(&n)
	if n >= MaxVenues {
		return nil
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO venues (group_id, name, address, lat, lon, ref) VALUES (?, ?, ?, ?, ?, ?)`,
		gid, v.Name, v.Address, nullF(v.Lat), nullF(v.Lon), v.Ref)
	return conflict(err)
}

// SaveVenue adds (ID 0) or changes a place.
func (s *Store) SaveVenue(ctx context.Context, v *Venue) error {
	if v.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO venues (group_id, name, address, lat, lon, ref) VALUES (?, ?, ?, ?, ?, ?)`,
			v.GroupID, v.Name, v.Address, nullF(v.Lat), nullF(v.Lon), v.Ref)
		if err != nil {
			return conflict(err)
		}
		v.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE venues SET name = ?, address = ?, lat = ?, lon = ?, ref = ? WHERE id = ? AND group_id = ?`,
		v.Name, v.Address, nullF(v.Lat), nullF(v.Lon), v.Ref, v.ID, v.GroupID)
	return conflict(err)
}

// DeleteVenue removes a place from the list (games keep where they were).
func (s *Store) DeleteVenue(ctx context.Context, gid, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM venues WHERE group_id = ? AND id = ?`, gid, id)
	return err
}
