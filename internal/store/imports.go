package store

import "context"

// importScopes are the tables an import remembers keys in, with the column
// a key is unique within.
var importScopes = map[string]string{"groups": "user_id", "games": "group_id", "matches": "group_id"}

// ImportedID is the row an earlier import made for a key (0 = none yet).
func (s *Store) ImportedID(ctx context.Context, table string, scope int64, key string) int64 {
	col, ok := importScopes[table]
	if !ok || key == "" {
		return 0
	}
	var id int64
	_ = s.DB.QueryRowContext(ctx, `SELECT id FROM `+table+` WHERE `+col+` = ? AND import_key = ?`, scope, key).Scan(&id)
	return id
}

// SetImportKey remembers where a row came from.
func (s *Store) SetImportKey(ctx context.Context, table string, id int64, key string) error {
	if _, ok := importScopes[table]; !ok {
		return ErrNotFound
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE `+table+` SET import_key = ? WHERE id = ?`, key, id)
	return conflict(err)
}
