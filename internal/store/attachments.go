package store

import "context"

// Attachment is a photo in a group's chat.
type Attachment struct {
	ID        int64  `json:"id"`
	GroupID   int64  `json:"group_id"`
	UserID    int64  `json:"user_id"`
	MessageID int64  `json:"message_id"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Bytes     int64  `json:"bytes"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Path      string `json:"-"`
	Thumb     string `json:"-"`
	CreatedAt string `json:"created_at"`
}

const attCols = `id, group_id, COALESCE(user_id, 0), COALESCE(message_id, 0), name, mime, bytes, width, height, path, thumb, created_at`

func scanAtt(row interface{ Scan(...any) error }) (*Attachment, error) {
	var a Attachment
	if err := row.Scan(&a.ID, &a.GroupID, &a.UserID, &a.MessageID, &a.Name, &a.Mime, &a.Bytes, &a.Width, &a.Height, &a.Path, &a.Thumb, &a.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &a, nil
}

// AddAttachment keeps an uploaded photo (not yet in a message).
func (s *Store) AddAttachment(ctx context.Context, a *Attachment) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO attachments (group_id, user_id, name, mime, bytes, width, height, path, thumb) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.GroupID, a.UserID, a.Name, a.Mime, a.Bytes, a.Width, a.Height, a.Path, a.Thumb)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

// GetAttachment loads one photo (the caller checks the group).
func (s *Store) GetAttachment(ctx context.Context, id int64) (*Attachment, error) {
	return scanAtt(s.DB.QueryRowContext(ctx, `SELECT `+attCols+` FROM attachments WHERE id = ?`, id))
}

// ClaimAttachments puts a person's own loose photos of a group into a message.
func (s *Store) ClaimAttachments(ctx context.Context, groupID, userID, messageID int64, ids []int64) ([]*Attachment, error) {
	out := []*Attachment{}
	for _, id := range ids {
		res, err := s.DB.ExecContext(ctx, `UPDATE attachments SET message_id = ? WHERE id = ? AND group_id = ? AND user_id = ? AND message_id IS NULL`, messageID, id, groupID, userID)
		if err != nil {
			return out, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		if a, err := s.GetAttachment(ctx, id); err == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// LooseAttachment reports whether a photo is the person's own and not sent yet.
func (s *Store) LooseAttachment(ctx context.Context, groupID, userID, id int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM attachments WHERE id = ? AND group_id = ? AND user_id = ? AND message_id IS NULL`, id, groupID, userID).Scan(&n)
	return n > 0
}

// DropAttachments forgets the photos of a message (or, with messageID 0,
// loose ones older than a day) and returns their files.
func (s *Store) DropAttachments(ctx context.Context, messageID int64) ([]string, error) {
	q := `SELECT path, thumb FROM attachments WHERE message_id = ?`
	d := `DELETE FROM attachments WHERE message_id = ?`
	args := []any{messageID}
	if messageID == 0 {
		q = `SELECT path, thumb FROM attachments WHERE message_id IS NULL AND created_at < ?`
		d = `DELETE FROM attachments WHERE message_id IS NULL AND created_at < ?`
		args = []any{Ago(24)}
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	files := []string{}
	for rows.Next() {
		var p, t string
		if rows.Scan(&p, &t) == nil {
			files = append(files, p, t)
		}
	}
	rows.Close()
	_, err = s.DB.ExecContext(ctx, d, args...)
	return files, err
}

// AttachmentBytes is how much the chat photos take.
func (s *Store) AttachmentBytes(ctx context.Context) int64 {
	var n int64
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(bytes), 0) FROM attachments`).Scan(&n)
	return n
}
