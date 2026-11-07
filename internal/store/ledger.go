package store

import "context"

// LedgerEntry is a charge (someone owes) or a payment (someone paid) in a group.
type LedgerEntry struct {
	ID           int64   `json:"id"`
	GroupID      int64   `json:"group_id"`
	UserID       int64   `json:"user_id"`
	Kind         string  `json:"kind"` // charge | payment
	AmountCents  int64   `json:"amount_cents"`
	Currency     string  `json:"currency"`
	GameID       int64   `json:"game_id"`
	TournamentID int64   `json:"tournament_id"`
	Note         string  `json:"note"`
	CreatedBy    int64   `json:"created_by"`
	ConfirmedBy  int64   `json:"confirmed_by"`
	ConfirmedAt  *string `json:"confirmed_at"`
	CreatedAt    string  `json:"created_at"`
}

const ledgerCols = `id, group_id, user_id, kind, amount_cents, currency, COALESCE(game_id, 0), COALESCE(tournament_id, 0), note,
	COALESCE(created_by, 0), COALESCE(confirmed_by, 0), confirmed_at, created_at`

func scanLedger(row interface{ Scan(...any) error }) (*LedgerEntry, error) {
	var e LedgerEntry
	if err := row.Scan(&e.ID, &e.GroupID, &e.UserID, &e.Kind, &e.AmountCents, &e.Currency, &e.GameID, &e.TournamentID, &e.Note,
		&e.CreatedBy, &e.ConfirmedBy, &e.ConfirmedAt, &e.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &e, nil
}

func nullID(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// AddLedger books a charge or payment; a charge for a game or tournament is
// booked once per person (false when it already was).
func (s *Store) AddLedger(ctx context.Context, e *LedgerEntry) (bool, error) {
	var conf any
	if e.ConfirmedBy != 0 {
		now := Now()
		e.ConfirmedAt = &now
		conf = now
	}
	res, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO ledger (group_id, user_id, kind, amount_cents, currency, game_id, tournament_id, note, created_by, confirmed_by, confirmed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.GroupID, e.UserID, e.Kind, e.AmountCents, e.Currency, nullID(e.GameID), nullID(e.TournamentID), e.Note,
		nullID(e.CreatedBy), nullID(e.ConfirmedBy), conf)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		e.ID, _ = res.LastInsertId()
	}
	return n > 0, nil
}

// Ledger lists a group's entries, newest first (uid 0 = everyone's).
func (s *Store) Ledger(ctx context.Context, gid, uid int64, limit int) ([]*LedgerEntry, error) {
	q := `SELECT ` + ledgerCols + ` FROM ledger WHERE group_id = ?`
	args := []any{gid}
	if uid != 0 {
		q += ` AND user_id = ?`
		args = append(args, uid)
	}
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY created_at DESC, id DESC LIMIT `+itoa(int64(limit)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*LedgerEntry{}
	for rows.Next() {
		e, err := scanLedger(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetLedger loads one entry.
func (s *Store) GetLedger(ctx context.Context, id int64) (*LedgerEntry, error) {
	return scanLedger(s.DB.QueryRowContext(ctx, `SELECT `+ledgerCols+` FROM ledger WHERE id = ?`, id))
}

// ConfirmLedger confirms a payment someone said they made.
func (s *Store) ConfirmLedger(ctx context.Context, id, by int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE ledger SET confirmed_by = ?, confirmed_at = ? WHERE id = ? AND confirmed_at IS NULL`, by, Now(), id)
	return err
}

// DeleteLedger removes an entry.
func (s *Store) DeleteLedger(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM ledger WHERE id = ?`, id)
	return err
}

// Balance is what someone owes a group (negative: paid ahead).
type Balance struct {
	UserID  int64 `json:"user_id"`
	Charged int64 `json:"charged"`
	Paid    int64 `json:"paid"`
	Pending int64 `json:"pending"` // payments said to be made, not confirmed yet
	Owes    int64 `json:"owes"`
}

// Balances is each person's balance in a group.
func (s *Store) Balances(ctx context.Context, gid int64) (map[int64]*Balance, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id,
		COALESCE(SUM(CASE WHEN kind = 'charge' THEN amount_cents END), 0),
		COALESCE(SUM(CASE WHEN kind = 'payment' AND confirmed_at IS NOT NULL THEN amount_cents END), 0),
		COALESCE(SUM(CASE WHEN kind = 'payment' AND confirmed_at IS NULL THEN amount_cents END), 0)
		FROM ledger WHERE group_id = ? GROUP BY user_id`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*Balance{}
	for rows.Next() {
		var b Balance
		if err := rows.Scan(&b.UserID, &b.Charged, &b.Paid, &b.Pending); err != nil {
			return nil, err
		}
		b.Owes = b.Charged - b.Paid
		out[b.UserID] = &b
	}
	return out, rows.Err()
}
