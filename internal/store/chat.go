package store

import (
	"context"
	"encoding/json"
	"time"
)

// ---- the chat ----

const msgCols = `id, group_id, COALESCE(game_id, 0), COALESCE(user_id, 0), kind, body, data, COALESCE(reply_to, 0), client_id, edited_at, deleted_at, created_at`

func scanMessage(row interface{ Scan(...any) error }) (*Message, error) {
	var m Message
	var data string
	if err := row.Scan(&m.ID, &m.GroupID, &m.GameID, &m.UserID, &m.Kind, &m.Body, &data, &m.ReplyTo, &m.ClientID, &m.EditedAt, &m.DeletedAt, &m.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	if !json.Valid([]byte(data)) {
		data = "{}"
	}
	m.Data = json.RawMessage(data)
	m.Reactions = []Reaction{}
	if m.DeletedAt != nil {
		m.Body, m.Data = "", json.RawMessage("{}")
	}
	return &m, nil
}

// MessageFilter picks a page of a chat: the group's own talk (GameID 0) or
// the comments under one game.
type MessageFilter struct {
	GroupID int64
	GameID  int64
	Before  int64 // older than this id
	After   int64 // newer than this id
	Limit   int
}

// ListMessages lists a page of messages, oldest first.
func (s *Store) ListMessages(ctx context.Context, f MessageFilter) ([]*Message, error) {
	where := `WHERE group_id = ? AND game_id IS NULL`
	args := []any{f.GroupID}
	if f.GameID > 0 {
		where = `WHERE group_id = ? AND game_id = ?`
		args = append(args, f.GameID)
	}
	if f.Before > 0 {
		where += ` AND id < ?`
		args = append(args, f.Before)
	}
	if f.After > 0 {
		where += ` AND id > ?`
		args = append(args, f.After)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 60
	}
	order := ` ORDER BY id DESC`
	if f.After > 0 {
		order = ` ORDER BY id`
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+msgCols+` FROM messages `+where+order+` LIMIT `+itoa(int64(f.Limit)), args...)
	if err != nil {
		return nil, err
	}
	out := []*Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	rows.Close()
	if f.After == 0 {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, s.fillReactions(ctx, out)
}

func (s *Store) fillReactions(ctx context.Context, ms []*Message) error {
	if len(ms) == 0 {
		return nil
	}
	byID := map[int64]*Message{}
	for _, m := range ms {
		byID[m.ID] = m
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT message_id, emoji, user_id FROM reactions WHERE message_id BETWEEN ? AND ? ORDER BY rowid`, ms[0].ID, ms[len(ms)-1].ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var mid, uid int64
		var e string
		if err := rows.Scan(&mid, &e, &uid); err != nil {
			return err
		}
		m := byID[mid]
		if m == nil {
			continue
		}
		found := false
		for i := range m.Reactions {
			if m.Reactions[i].Emoji == e {
				m.Reactions[i].Users = append(m.Reactions[i].Users, uid)
				found = true
			}
		}
		if !found {
			m.Reactions = append(m.Reactions, Reaction{Emoji: e, Users: []int64{uid}})
		}
	}
	return rows.Err()
}

// GetMessage returns one message.
func (s *Store) GetMessage(ctx context.Context, id int64) (*Message, error) {
	m, err := scanMessage(s.DB.QueryRowContext(ctx, `SELECT `+msgCols+` FROM messages WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	return m, s.fillReactions(ctx, []*Message{m})
}

// AddMessage saves a message. A client id seen before returns that message
// (sending twice after a bad connection is the same as once).
func (s *Store) AddMessage(ctx context.Context, m *Message) (bool, error) {
	if m.ClientID != "" && m.UserID > 0 {
		var id int64
		if err := s.DB.QueryRowContext(ctx, `SELECT id FROM messages WHERE user_id = ? AND client_id = ?`, m.UserID, m.ClientID).Scan(&id); err == nil {
			got, err := s.GetMessage(ctx, id)
			if err == nil {
				*m = *got
			}
			return false, err
		}
	}
	if len(m.Data) == 0 {
		m.Data = json.RawMessage("{}")
	}
	if m.Kind == "" {
		m.Kind = "text"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO messages (group_id, game_id, user_id, kind, body, data, reply_to, client_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.GroupID, nullI(&m.GameID), nullI(&m.UserID), m.Kind, m.Body, string(m.Data), nullI(&m.ReplyTo), m.ClientID)
	if err != nil {
		return false, err
	}
	m.ID, _ = res.LastInsertId()
	got, err := s.GetMessage(ctx, m.ID)
	if err == nil {
		*m = *got
	}
	return true, err
}

// EditMessage changes a message's text.
func (s *Store) EditMessage(ctx context.Context, id int64, body string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE messages SET body = ?, edited_at = ? WHERE id = ? AND deleted_at IS NULL`, body, Now(), id)
	return err
}

// DeleteMessage blanks a message (the line stays: "message deleted").
func (s *Store) DeleteMessage(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE messages SET body = '', data = '{}', deleted_at = ? WHERE id = ?`, Now(), id)
	if err == nil {
		_, err = s.DB.ExecContext(ctx, `DELETE FROM reactions WHERE message_id = ?`, id)
	}
	return err
}

// ToggleReaction adds or removes one emoji of a person under a message.
func (s *Store) ToggleReaction(ctx context.Context, mid, uid int64, emoji string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM reactions WHERE message_id = ? AND user_id = ? AND emoji = ?`, mid, uid, emoji)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO reactions (message_id, user_id, emoji) VALUES (?, ?, ?)`, mid, uid, emoji)
	return err
}

// SearchMessages finds text in the chats of a person's groups.
func (s *Store) SearchMessages(ctx context.Context, uid int64, q string, limit int) ([]*Message, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+msgCols+` FROM messages WHERE kind = 'text' AND deleted_at IS NULL AND body LIKE '%' || ? || '%'
		AND group_id IN (SELECT group_id FROM members WHERE user_id = ?) ORDER BY id DESC LIMIT ?`, q, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- sign-in links ----

// CreateSignInLink makes a new one-time link for a player and returns the
// plain token (only its hash is kept). Older unused links stop working.
func (s *Store) CreateSignInLink(ctx context.Context, uid, by, gid int64, ttl time.Duration) (string, *SignInLink, error) {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM sign_in_links WHERE user_id = ? AND used_at IS NULL`, uid); err != nil {
		return "", nil, err
	}
	tok := RandomToken(24)
	l := &SignInLink{UserID: uid, CreatedBy: by, GroupID: gid, ExpiresAt: time.Now().Add(ttl).UTC().Format(time.RFC3339), CreatedAt: Now()}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO sign_in_links (user_id, created_by, group_id, token_hash, expires_at) VALUES (?, ?, ?, ?, ?)`,
		uid, nullI(&by), nullI(&gid), HashToken(tok), l.ExpiresAt)
	if err != nil {
		return "", nil, err
	}
	l.ID, _ = res.LastInsertId()
	return tok, l, nil
}

// PeekSignInLink finds an unused, unexpired link without using it up.
func (s *Store) PeekSignInLink(ctx context.Context, tok string) (*SignInLink, error) {
	var l SignInLink
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, COALESCE(created_by, 0), COALESCE(group_id, 0), expires_at, used_at, created_at
		FROM sign_in_links WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`, HashToken(tok), time.Now().UTC().Format(time.RFC3339)).
		Scan(&l.ID, &l.UserID, &l.CreatedBy, &l.GroupID, &l.ExpiresAt, &l.UsedAt, &l.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

// UseSignInLink uses a link up; only the first use succeeds.
func (s *Store) UseSignInLink(ctx context.Context, tok string) (*SignInLink, error) {
	l, err := s.PeekSignInLink(ctx, tok)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE sign_in_links SET used_at = ? WHERE id = ? AND used_at IS NULL`, Now(), l.ID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return l, nil
}

// LastSignInLink is the newest link made for a player (nil when none).
func (s *Store) LastSignInLink(ctx context.Context, uid int64) *SignInLink {
	var l SignInLink
	if err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, COALESCE(created_by, 0), COALESCE(group_id, 0), expires_at, used_at, created_at
		FROM sign_in_links WHERE user_id = ? ORDER BY id DESC LIMIT 1`, uid).Scan(&l.ID, &l.UserID, &l.CreatedBy, &l.GroupID, &l.ExpiresAt, &l.UsedAt, &l.CreatedAt); err != nil {
		return nil
	}
	return &l
}

// RevokeSignInLinks stops every unused link of a player.
func (s *Store) RevokeSignInLinks(ctx context.Context, uid int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sign_in_links WHERE user_id = ? AND used_at IS NULL`, uid)
	return err
}

// ---- made in other apps ----

// FindMade is what was made in an app for a person and a game or match.
func (s *Store) FindMade(ctx context.Context, uid int64, app string, gameID, matchID int64) (*Made, error) {
	var m Made
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, COALESCE(group_id, 0), COALESCE(game_id, 0), COALESCE(match_id, 0), app, ref, url, title, detail, created_at
		FROM made WHERE user_id = ? AND app = ? AND COALESCE(game_id, 0) = ? AND COALESCE(match_id, 0) = ?`, uid, app, gameID, matchID).
		Scan(&m.ID, &m.UserID, &m.GroupID, &m.GameID, &m.MatchID, &m.App, &m.Ref, &m.URL, &m.Title, &m.Detail, &m.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &m, nil
}

// PutMade records (or replaces) what was made.
func (s *Store) PutMade(ctx context.Context, m *Made) error {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND app = ? AND COALESCE(game_id, 0) = ? AND COALESCE(match_id, 0) = ?`, m.UserID, m.App, m.GameID, m.MatchID)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO made (user_id, group_id, game_id, match_id, app, ref, url, title, detail) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.UserID, nullI(&m.GroupID), nullI(&m.GameID), nullI(&m.MatchID), m.App, m.Ref, m.URL, m.Title, m.Detail)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	m.CreatedAt = Now()
	return nil
}

// DeleteMade forgets one thing made.
func (s *Store) DeleteMade(ctx context.Context, uid, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND id = ?`, uid, id)
	return err
}

// MadeFor lists what a person made for a game (and its matches).
func (s *Store) MadeFor(ctx context.Context, uid, gameID int64) ([]Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, COALESCE(group_id, 0), COALESCE(game_id, 0), COALESCE(match_id, 0), app, ref, url, title, detail, created_at
		FROM made WHERE user_id = ? AND (game_id = ? OR match_id IN (SELECT id FROM matches WHERE game_id = ?)) ORDER BY id`, uid, gameID, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Made{}
	for rows.Next() {
		var m Made
		if err := rows.Scan(&m.ID, &m.UserID, &m.GroupID, &m.GameID, &m.MatchID, &m.App, &m.Ref, &m.URL, &m.Title, &m.Detail, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MadeCalendarFor lists the calendar entries of everyone for a game.
func (s *Store) MadeCalendarFor(ctx context.Context, gameID int64) ([]Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, COALESCE(group_id, 0), COALESCE(game_id, 0), COALESCE(match_id, 0), app, ref, url, title, detail, created_at
		FROM made WHERE game_id = ? AND app = 'calendar'`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Made{}
	for rows.Next() {
		var m Made
		if err := rows.Scan(&m.ID, &m.UserID, &m.GroupID, &m.GameID, &m.MatchID, &m.App, &m.Ref, &m.URL, &m.Title, &m.Detail, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
