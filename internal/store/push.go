package store

import "context"

// PushSub is a phone or browser that takes League's notifications.
type PushSub struct {
	ID       int64   `json:"id"`
	UserID   int64   `json:"-"`
	Endpoint string  `json:"endpoint"`
	P256dh   string  `json:"-"`
	Auth     string  `json:"-"`
	Agent    string  `json:"agent"`
	Fails    int     `json:"fails"`
	LastOK   *string `json:"last_ok"`
	Created  string  `json:"created_at"`
}

// PushSubs lists a person's phones.
func (s *Store) PushSubs(ctx context.Context, uid int64) ([]PushSub, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, endpoint, p256dh, auth, agent, fails, last_ok, created_at FROM push_subs WHERE user_id = ? ORDER BY id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushSub{}
	for rows.Next() {
		var p PushSub
		if err := rows.Scan(&p.ID, &p.UserID, &p.Endpoint, &p.P256dh, &p.Auth, &p.Agent, &p.Fails, &p.LastOK, &p.Created); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SavePushSub adds a phone (or moves an endpoint to this person).
func (s *Store) SavePushSub(ctx context.Context, p *PushSub) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO push_subs (user_id, endpoint, p256dh, auth, agent) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET user_id = excluded.user_id, p256dh = excluded.p256dh, auth = excluded.auth, agent = excluded.agent, fails = 0`,
		p.UserID, p.Endpoint, p.P256dh, p.Auth, p.Agent)
	return err
}

// DeletePushSub forgets a phone.
func (s *Store) DeletePushSub(ctx context.Context, uid int64, endpoint string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM push_subs WHERE user_id = ? AND endpoint = ?`, uid, endpoint)
	return err
}

// PushResult notes how sending to a phone went; a gone one is forgotten.
func (s *Store) PushResult(ctx context.Context, id int64, ok, gone bool) {
	switch {
	case gone:
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM push_subs WHERE id = ?`, id)
	case ok:
		_, _ = s.DB.ExecContext(ctx, `UPDATE push_subs SET fails = 0, last_ok = ? WHERE id = ?`, Now(), id)
	default:
		_, _ = s.DB.ExecContext(ctx, `UPDATE push_subs SET fails = fails + 1 WHERE id = ?`, id)
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM push_subs WHERE id = ? AND fails >= 10`, id)
	}
}
