package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// ---- groups ----
//
// Who sees a group is who is in it: every query here goes through members
// for the person asking. Someone outside a group gets ErrNotFound, never
// the group.

const groupCols = `g.id, g.user_id, g.name, g.sport, g.sport_name, g.scoring, g.format, g.lower_better, g.color, g.description,
	g.venue_name, g.venue_address, g.venue_lat, g.venue_lon, g.venue_ref, g.outdoor, g.weekday, g.time, g.duration_min, g.plan_days,
	g.min_players, g.max_players, g.cost_cents, g.currency, g.win_points, g.draw_points, g.loss_points, g.email_players, g.archived,
	g.pay_note, g.charge_games, g.rating_scope, g.stats, g.created_at, g.updated_at`

func scanGroup(row interface{ Scan(...any) error }, extra ...any) (*Group, error) {
	var g Group
	var stats string
	dst := []any{&g.ID, &g.UserID, &g.Name, &g.Sport, &g.SportName, &g.Scoring, &g.Format, &g.LowerBetter, &g.Color, &g.Description,
		&g.VenueName, &g.VenueAddress, &g.VenueLat, &g.VenueLon, &g.VenueRef, &g.Outdoor, &g.Weekday, &g.Time, &g.DurationMin, &g.PlanDays,
		&g.MinPlayers, &g.MaxPlayers, &g.CostCents, &g.Currency, &g.WinPoints, &g.DrawPoints, &g.LossPoints, &g.EmailPlayers, &g.Archived,
		&g.PayNote, &g.ChargeGames, &g.RatingScope, &stats, &g.CreatedAt, &g.UpdatedAt}
	if err := row.Scan(append(dst, extra...)...); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(stats), &g.Stats)
	if g.Stats == nil {
		g.Stats = []string{}
	}
	return &g, nil
}

// myGroupQuery is a group with the asking person's own membership.
const myGroupQuery = `SELECT ` + groupCols + `, m.role, m.notify, m.last_read_id,
	(SELECT COUNT(1) FROM members x WHERE x.group_id = g.id),
	(SELECT COUNT(1) FROM messages c WHERE c.group_id = g.id AND c.game_id IS NULL AND c.id > m.last_read_id AND COALESCE(c.user_id, 0) <> m.user_id AND c.deleted_at IS NULL),
	COALESCE((SELECT MAX(c.id) FROM messages c WHERE c.group_id = g.id AND c.game_id IS NULL), 0),
	COALESCE((SELECT c.created_at FROM messages c WHERE c.group_id = g.id AND c.game_id IS NULL ORDER BY c.id DESC LIMIT 1), '')
	FROM groups g JOIN members m ON m.group_id = g.id AND m.user_id = ?`

func scanMyGroup(row interface{ Scan(...any) error }) (*Group, error) {
	var g *Group
	var role, notify, lastAt string
	var lastRead, lastID int64
	var members, unread int
	g, err := scanGroup(row, &role, &notify, &lastRead, &members, &unread, &lastID, &lastAt)
	if err != nil {
		return nil, err
	}
	g.MyRole, g.MyNotify, g.LastReadID, g.Members, g.Unread, g.LastMsgID, g.LastMsgAt = role, notify, lastRead, members, unread, lastID, lastAt
	return g, nil
}

// ListGroups lists the groups a person is in (archived ones last).
func (s *Store) ListGroups(ctx context.Context, uid int64, archived bool) ([]*Group, error) {
	q := myGroupQuery
	if !archived {
		q += ` WHERE g.archived = 0`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY g.archived, g.name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Group{}
	for rows.Next() {
		g, err := scanMyGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGroup returns a group the person is in.
func (s *Store) GetGroup(ctx context.Context, uid, id int64) (*Group, error) {
	return scanMyGroup(s.DB.QueryRowContext(ctx, myGroupQuery+` WHERE g.id = ?`, uid, id))
}

// GroupByID returns a group without a person (background work only).
func (s *Store) GroupByID(ctx context.Context, id int64) (*Group, error) {
	return scanGroup(s.DB.QueryRowContext(ctx, `SELECT `+groupCols+` FROM groups g WHERE g.id = ?`, id))
}

// AllGroups lists every group (background work only).
func (s *Store) AllGroups(ctx context.Context) ([]*Group, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+groupCols+` FROM groups g WHERE g.archived = 0 ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func groupArgs(g *Group) []any {
	return []any{g.Name, g.Sport, g.SportName, g.Scoring, g.Format, boolInt(g.LowerBetter), g.Color, g.Description,
		g.VenueName, g.VenueAddress, nullF(g.VenueLat), nullF(g.VenueLon), g.VenueRef, boolInt(g.Outdoor), g.Weekday, g.Time, g.DurationMin, g.PlanDays,
		g.MinPlayers, g.MaxPlayers, g.CostCents, g.Currency, g.WinPoints, g.DrawPoints, g.LossPoints, boolInt(g.EmailPlayers), boolInt(g.Archived),
		g.PayNote, boolInt(g.ChargeGames), firstOr(g.RatingScope, "all"), toJSON(nonNil(g.Stats))}
}

func firstOr(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// CreateGroup makes a group with its maker as the first organiser.
func (s *Store) CreateGroup(ctx context.Context, uid int64, g *Group) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO groups (user_id, name, sport, sport_name, scoring, format, lower_better, color, description,
		venue_name, venue_address, venue_lat, venue_lon, venue_ref, outdoor, weekday, time, duration_min, plan_days,
		min_players, max_players, cost_cents, currency, win_points, draw_points, loss_points, email_players, archived, pay_note, charge_games, rating_scope, stats)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, append([]any{uid}, groupArgs(g)...)...)
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	g.UserID = uid
	if _, err := tx.ExecContext(ctx, `INSERT INTO members (group_id, user_id, role, feed_token, added_by) VALUES (?, ?, 'organiser', ?, ?)`, g.ID, uid, RandomToken(16), uid); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateGroup saves a group's settings.
func (s *Store) UpdateGroup(ctx context.Context, g *Group) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE groups SET name = ?, sport = ?, sport_name = ?, scoring = ?, format = ?, lower_better = ?, color = ?, description = ?,
		venue_name = ?, venue_address = ?, venue_lat = ?, venue_lon = ?, venue_ref = ?, outdoor = ?, weekday = ?, time = ?, duration_min = ?, plan_days = ?,
		min_players = ?, max_players = ?, cost_cents = ?, currency = ?, win_points = ?, draw_points = ?, loss_points = ?, email_players = ?, archived = ?,
		pay_note = ?, charge_games = ?, rating_scope = ?, stats = ?, updated_at = ? WHERE id = ?`, append(groupArgs(g), Now(), g.ID)...)
	return err
}

// DeleteGroup removes a group with its games, results and chat.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id)
	return err
}

// ---- members ----

const memberCols = `m.id, m.group_id, m.user_id, m.role, m.nickname, m.notify, m.last_read_id, m.feed_token, COALESCE(m.added_by, 0), m.joined_at,
	u.name, COALESCE(u.email, ''), u.phone, u.role, COALESCE(u.invited_by, 0), u.last_seen_at`

func scanMember(row interface{ Scan(...any) error }) (*Member, error) {
	var m Member
	if err := row.Scan(&m.ID, &m.GroupID, &m.UserID, &m.Role, &m.Nickname, &m.Notify, &m.LastReadID, &m.FeedToken, &m.AddedBy, &m.JoinedAt,
		&m.Name, &m.Email, &m.Phone, &m.Account, &m.InvitedBy, &m.LastSeenAt); err != nil {
		return nil, notFound(err)
	}
	return &m, nil
}

// Members lists a group's people, organisers first.
func (s *Store) Members(ctx context.Context, gid int64) ([]*Member, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+memberCols+` FROM members m JOIN users u ON u.id = m.user_id WHERE m.group_id = ?
		ORDER BY (m.role = 'organiser') DESC, COALESCE(NULLIF(m.nickname, ''), u.name) COLLATE NOCASE`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Member{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMember is one person's membership of a group.
func (s *Store) GetMember(ctx context.Context, gid, uid int64) (*Member, error) {
	return scanMember(s.DB.QueryRowContext(ctx, `SELECT `+memberCols+` FROM members m JOIN users u ON u.id = m.user_id WHERE m.group_id = ? AND m.user_id = ?`, gid, uid))
}

// MemberByFeed finds the membership a calendar feed belongs to.
func (s *Store) MemberByFeed(ctx context.Context, token string) (*Member, error) {
	return scanMember(s.DB.QueryRowContext(ctx, `SELECT `+memberCols+` FROM members m JOIN users u ON u.id = m.user_id WHERE m.feed_token = ?`, token))
}

// MemberIDs are the user ids in a group.
func (s *Store) MemberIDs(ctx context.Context, gid int64) ([]int64, error) {
	return s.ids(ctx, `SELECT user_id FROM members WHERE group_id = ?`, gid)
}

// GroupIDsOf are the groups a person is in.
func (s *Store) GroupIDsOf(ctx context.Context, uid int64) ([]int64, error) {
	return s.ids(ctx, `SELECT group_id FROM members WHERE user_id = ?`, uid)
}

// SharesGroup reports whether two people are in a group together.
func (s *Store) SharesGroup(ctx context.Context, a, b int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM members x JOIN members y ON y.group_id = x.group_id WHERE x.user_id = ? AND y.user_id = ?`, a, b).Scan(&n)
	return n > 0
}

// OrganisesAllGroupsOf reports whether org organises every group that uid is in.
func (s *Store) OrganisesAllGroupsOf(ctx context.Context, org, uid int64) bool {
	var total, mine int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(SUM(EXISTS (SELECT 1 FROM members o WHERE o.group_id = m.group_id AND o.user_id = ? AND o.role = 'organiser')), 0)
		FROM members m WHERE m.user_id = ?`, org, uid).Scan(&total, &mine)
	return total > 0 && total == mine
}

func (s *Store) ids(ctx context.Context, q string, args ...any) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AddMember puts a person in a group (nothing changes when they already are).
func (s *Store) AddMember(ctx context.Context, gid, uid, by int64, role string) (bool, error) {
	if role != "organiser" {
		role = "player"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO members (group_id, user_id, role, feed_token, added_by) VALUES (?, ?, ?, ?, ?) ON CONFLICT(group_id, user_id) DO NOTHING`,
		gid, uid, role, RandomToken(16), nullI(&by))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpdateMember changes someone's role or nickname in a group.
func (s *Store) UpdateMember(ctx context.Context, gid, uid int64, role, nickname *string) error {
	if role != nil {
		if _, err := s.DB.ExecContext(ctx, `UPDATE members SET role = ? WHERE group_id = ? AND user_id = ?`, *role, gid, uid); err != nil {
			return err
		}
	}
	if nickname != nil {
		if _, err := s.DB.ExecContext(ctx, `UPDATE members SET nickname = ? WHERE group_id = ? AND user_id = ?`, *nickname, gid, uid); err != nil {
			return err
		}
	}
	return nil
}

// SetMemberNotify is a person's own choice of what they hear about a group.
func (s *Store) SetMemberNotify(ctx context.Context, gid, uid int64, notify string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE members SET notify = ? WHERE group_id = ? AND user_id = ?`, notify, gid, uid)
	return err
}

// RemoveMember takes someone out of a group (their past results stay).
func (s *Store) RemoveMember(ctx context.Context, gid, uid int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM members WHERE group_id = ? AND user_id = ?`, gid, uid)
	if err != nil {
		return err
	}
	// they are no longer coming to the group's next games
	_, err = s.DB.ExecContext(ctx, `DELETE FROM answers WHERE user_id = ? AND game_id IN (SELECT id FROM games WHERE group_id = ? AND starts_at >= ?)`, uid, gid, LocalNow())
	return err
}

// CountOrganisers is how many organisers a group has.
func (s *Store) CountOrganisers(ctx context.Context, gid int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM members WHERE group_id = ? AND role = 'organiser'`, gid).Scan(&n)
	return n
}

// MarkRead moves a person's read mark in a group's chat forward.
func (s *Store) MarkRead(ctx context.Context, gid, uid, upTo int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE members SET last_read_id = MAX(last_read_id, ?) WHERE group_id = ? AND user_id = ?`, upTo, gid, uid)
	return err
}

// PlayerUsage is how many groups a player account is in.
func (s *Store) PlayerUsage(ctx context.Context, uid int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM members WHERE user_id = ?`, uid).Scan(&n)
	return n
}

// FindPlayer looks for an existing account by email, else by phone (players
// added with a phone only), so the same person is not made twice.
func (s *Store) FindPlayer(ctx context.Context, email, phone string) (*User, error) {
	if email != "" {
		return s.GetUserByEmail(ctx, email)
	}
	p := normPhone(phone)
	if p == "" {
		return nil, ErrNotFound
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+` FROM users WHERE phone <> '' AND email IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		if normPhone(u.Phone) == p {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func normPhone(p string) string {
	var b strings.Builder
	for i, r := range p {
		if r >= '0' && r <= '9' || (r == '+' && i == 0) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if strings.HasPrefix(out, "00") {
		out = "+" + out[2:]
	}
	return out
}

var _ = sql.ErrNoRows
