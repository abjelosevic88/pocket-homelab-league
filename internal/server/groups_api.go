package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- groups ----
//
// A group is shared by the people in it and by nobody else: every handler
// loads it for the person asking (store.GetGroup goes through members), so
// someone outside gets 404 — admins too. Organisers change the group and
// its people; everyone in it plans games, answers, chats and enters results.

var (
	reTime  = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	reColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	reCur   = regexp.MustCompile(`^[A-Z]{3}$`)
)

func isPlayer(u *store.User) bool { return u != nil && u.Role == store.RolePlayer }

// myGroup loads a group the person is in, or answers 404.
func (s *Server) myGroup(w http.ResponseWriter, r *http.Request, param string) (*store.Group, bool) {
	g, err := s.store.GetGroup(r.Context(), actorID(r.Context()), idParam(r, param))
	if err != nil {
		s.fail(w, err, "group")
		return nil, false
	}
	return g, true
}

func organiser(g *store.Group) bool { return g.MyRole == "organiser" }

func (s *Server) needOrganiser(w http.ResponseWriter, g *store.Group) bool {
	if !organiser(g) {
		writeErr(w, http.StatusForbidden, "only the group's organisers can do that")
		return false
	}
	return true
}

// groupView is a group with its sport spelled out.
type groupView struct {
	*store.Group
	SportInfo  league.Sport `json:"sport_info"`
	SportLabel string       `json:"sport_label"`
	Next       *store.Game  `json:"next_game,omitempty"`
}

func viewGroup(g *store.Group) groupView {
	return groupView{Group: g, SportInfo: league.SportByKey(g.Sport), SportLabel: league.DisplayName(g.Sport, g.SportName)}
}

func (s *Server) handleSports(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, league.Sports)
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	gs, err := s.store.ListGroups(ctx, uid, queryBool(r, "archived"))
	if err != nil {
		s.fail(w, err, "groups")
		return
	}
	next, _ := s.store.ListGames(ctx, uid, store.GameFilter{Upcoming: true})
	out := make([]groupView, 0, len(gs))
	for _, g := range gs {
		v := viewGroup(g)
		for _, n := range next {
			if n.GroupID == g.ID && n.Status == store.GamePlanned {
				v.Next = n
				break
			}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// groupInput is what a person may set on a group (nil = not sent).
type groupInput struct {
	Name         *string  `json:"name"`
	Sport        *string  `json:"sport"`
	SportName    *string  `json:"sport_name"`
	Scoring      *string  `json:"scoring"`
	Format       *string  `json:"format"`
	LowerBetter  *bool    `json:"lower_better"`
	Color        *string  `json:"color"`
	Description  *string  `json:"description"`
	VenueName    *string  `json:"venue_name"`
	VenueAddress *string  `json:"venue_address"`
	VenueLat     *float64 `json:"venue_lat"`
	VenueLon     *float64 `json:"venue_lon"`
	VenueRef     *string  `json:"venue_ref"`
	Outdoor      *bool    `json:"outdoor"`
	Weekday      *int     `json:"weekday"`
	Time         *string  `json:"time"`
	DurationMin  *int     `json:"duration_min"`
	PlanDays     *int     `json:"plan_days"`
	MinPlayers   *int     `json:"min_players"`
	MaxPlayers   *int     `json:"max_players"`
	CostCents    *int64   `json:"cost_cents"`
	Currency     *string  `json:"currency"`
	WinPoints    *int     `json:"win_points"`
	DrawPoints   *int     `json:"draw_points"`
	LossPoints   *int     `json:"loss_points"`
	EmailPlayers *bool    `json:"email_players"`
	Archived     *bool    `json:"archived"`
}

// apply validates and copies the sent fields; "" = fine.
func (in groupInput) apply(g *store.Group, keys map[string]bool, isNew bool) string {
	trimP(&g.Name, in.Name)
	if in.Name != nil && (g.Name == "" || len(g.Name) > 80) {
		return "give the group a name (up to 80 characters)"
	}
	sportChanged := false
	if in.Sport != nil {
		if !league.ValidSport(*in.Sport) {
			return "pick a sport from the list (or \"Something else\")"
		}
		sportChanged = g.Sport != *in.Sport
		g.Sport = *in.Sport
	}
	if g.Sport == "" {
		return "pick a sport"
	}
	sp := league.SportByKey(g.Sport)
	trimP(&g.SportName, in.SportName)
	g.SportName = clip(g.SportName, 40)
	if isNew || sportChanged {
		// a new sport brings its usual way of playing, unless the request says otherwise
		g.Scoring, g.Format, g.LowerBetter, g.Outdoor = sp.Scoring, sp.Format, sp.LowerBetter, sp.Outdoor
		g.MinPlayers, g.MaxPlayers, g.DurationMin = sp.Min, sp.Max, sp.Minutes
		g.WinPoints, g.DrawPoints, g.LossPoints = 3, 1, 0
		if !sp.Draws {
			g.WinPoints, g.DrawPoints = 1, 0
		}
		if g.Color == "" || sportChanged {
			g.Color = sp.Color
		}
	}
	if in.Scoring != nil {
		if !league.ValidScoring(*in.Scoring) {
			return "unknown way of scoring"
		}
		if g.Sport != "other" && *in.Scoring != sp.Scoring {
			return "this sport is scored one way; pick \"Something else\" to choose"
		}
		g.Scoring = *in.Scoring
	}
	if in.Format != nil {
		if !league.ValidFormat(*in.Format) || !oneOf(*in.Format, sp.Formats) {
			return "this sport is not played that way here"
		}
		g.Format = *in.Format
	}
	if (g.Scoring == league.ScoringRanking) != (g.Format == league.FormatEveryone) {
		if g.Scoring == league.ScoringRanking {
			g.Format = league.FormatEveryone
		} else {
			g.Format = league.FormatSingles
			if len(sp.Formats) > 0 && sp.Formats[0] != league.FormatEveryone {
				g.Format = sp.Formats[0]
			}
		}
	}
	if in.LowerBetter != nil {
		g.LowerBetter = *in.LowerBetter
	}
	if in.Color != nil {
		if *in.Color != "" && !reColor.MatchString(*in.Color) {
			return "a colour is #rrggbb"
		}
		g.Color = *in.Color
	}
	trimP(&g.Description, in.Description)
	g.Description = clip(g.Description, 2000)
	trimP(&g.VenueName, in.VenueName)
	trimP(&g.VenueAddress, in.VenueAddress)
	trimP(&g.VenueRef, in.VenueRef)
	g.VenueName, g.VenueAddress = clip(g.VenueName, 120), clip(g.VenueAddress, 300)
	if keys["venue_lat"] || keys["venue_lon"] {
		g.VenueLat, g.VenueLon = in.VenueLat, in.VenueLon
		if (g.VenueLat == nil) != (g.VenueLon == nil) || (g.VenueLat != nil && (*g.VenueLat < -90 || *g.VenueLat > 90 || *g.VenueLon < -180 || *g.VenueLon > 180)) {
			return "the venue's position is a latitude and a longitude"
		}
	}
	if in.Outdoor != nil {
		g.Outdoor = *in.Outdoor
	}
	if in.Weekday != nil {
		if *in.Weekday < 0 || *in.Weekday > 7 {
			return "the usual day is Monday to Sunday (or none)"
		}
		g.Weekday = *in.Weekday
	}
	if in.Time != nil {
		if *in.Time != "" && !reTime.MatchString(*in.Time) {
			return "the usual time is HH:MM"
		}
		g.Time = *in.Time
	}
	ints := []struct {
		v        *int
		dst      *int
		min, max int
		msg      string
	}{
		{in.DurationMin, &g.DurationMin, 10, 720, "a game lasts 10 minutes to 12 hours"},
		{in.PlanDays, &g.PlanDays, 0, 14, "plan the usual game 0 to 14 days before"},
		{in.MinPlayers, &g.MinPlayers, 1, 200, "a game needs 1 to 200 players"},
		{in.MaxPlayers, &g.MaxPlayers, 0, 200, "at most 200 players (0 = no limit)"},
		{in.WinPoints, &g.WinPoints, 0, 10, "points are 0 to 10"},
		{in.DrawPoints, &g.DrawPoints, 0, 10, "points are 0 to 10"},
		{in.LossPoints, &g.LossPoints, 0, 10, "points are 0 to 10"},
	}
	for _, x := range ints {
		if x.v == nil {
			continue
		}
		if *x.v < x.min || *x.v > x.max {
			return x.msg
		}
		*x.dst = *x.v
	}
	if g.MaxPlayers > 0 && g.MaxPlayers < g.MinPlayers {
		return "the most players can't be fewer than the fewest"
	}
	if in.CostCents != nil {
		if *in.CostCents < 0 || *in.CostCents > 100_000_00 {
			return "the cost is 0 or more"
		}
		g.CostCents = *in.CostCents
	}
	if in.Currency != nil {
		c := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if c == "KM" {
			c = "BAM"
		}
		if !reCur.MatchString(c) {
			return "the currency is a three-letter code like EUR"
		}
		g.Currency = c
	}
	if in.EmailPlayers != nil {
		g.EmailPlayers = *in.EmailPlayers
	}
	if in.Archived != nil {
		g.Archived = *in.Archived
	}
	return ""
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	if isPlayer(u) {
		writeErr(w, http.StatusForbidden, "players can't start groups here; ask whoever runs this box to make you a member")
		return
	}
	var in groupInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	g := &store.Group{Currency: st.Currency, EmailPlayers: true, DurationMin: 90, MinPlayers: 2}
	if in.Name == nil {
		in.Name = ptr("")
	}
	// the sport's defaults first, then whatever else was sent
	if msg := (groupInput{Name: in.Name, Sport: in.Sport, SportName: in.SportName}).apply(g, map[string]bool{}, true); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	in.Sport = nil
	if msg := in.apply(g, keys, false); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateGroup(ctx, u.ID, g); err != nil {
		s.fail(w, err, "create group")
		return
	}
	_ = s.store.EnsureVenue(ctx, g.ID, store.Venue{Name: g.VenueName, Address: g.VenueAddress, Lat: g.VenueLat, Lon: g.VenueLon, Ref: g.VenueRef})
	g, _ = s.store.GetGroup(ctx, u.ID, g.ID)
	writeJSON(w, http.StatusCreated, viewGroup(g))
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	ms, err := s.store.Members(ctx, g.ID)
	if err != nil {
		s.fail(w, err, "members")
		return
	}
	upcoming, _ := s.store.ListGames(ctx, actorID(ctx), store.GameFilter{GroupID: g.ID, Upcoming: true, Limit: 6})
	v := viewGroup(g)
	for _, n := range upcoming {
		if n.Status == store.GamePlanned {
			v.Next = n
			break
		}
	}
	venues, _ := s.store.Venues(ctx, g.ID)
	writeJSON(w, http.StatusOK, map[string]any{"group": v, "members": s.memberViews(ctx, g, ms), "upcoming": upcoming,
		"feed_url": s.feedURL(r, g.ID), "venues": venues})
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	var in groupInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(g, keys, false); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateGroup(ctx, g); err != nil {
		s.fail(w, err, "save group")
		return
	}
	_ = s.store.EnsureVenue(ctx, g.ID, store.Venue{Name: g.VenueName, Address: g.VenueAddress, Lat: g.VenueLat, Lon: g.VenueLon, Ref: g.VenueRef})
	g, _ = s.store.GetGroup(ctx, actorID(ctx), g.ID)
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
	writeJSON(w, http.StatusOK, viewGroup(g))
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	if g.UserID != actorID(ctx) {
		writeErr(w, http.StatusForbidden, "only the person who made the group can delete it (organisers can archive it)")
		return
	}
	ids, _ := s.store.MemberIDs(ctx, g.ID)
	// the coming games leave everyone's calendars before the group goes
	games, _ := s.store.ListGames(ctx, actorID(ctx), store.GameFilter{GroupID: g.ID, Upcoming: true})
	for _, gm := range games {
		s.dropCalendar(ctx, gm.ID)
	}
	if err := s.store.DeleteGroup(ctx, g.ID); err != nil {
		s.fail(w, err, "delete group")
		return
	}
	s.toPeople(ids, "group", map[string]any{"id": g.ID, "deleted": true})
	w.WriteHeader(http.StatusNoContent)
}

// handleMyGroupSettings is the person's own choice for a group: what they hear.
func (s *Server) handleMyGroupSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Notify string `json:"notify"`
	}
	if err := decode(r, &in); err != nil || !oneOf(in.Notify, []string{"all", "games", "off"}) {
		writeErr(w, http.StatusBadRequest, "hear about everything, only games, or nothing")
		return
	}
	if err := s.store.SetMemberNotify(ctx, g.ID, actorID(ctx), in.Notify); err != nil {
		s.fail(w, err, "save")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notify": in.Notify})
}

func (s *Server) handleLeaveGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	uid := actorID(ctx)
	if organiser(g) && s.store.CountOrganisers(ctx, g.ID) <= 1 {
		if ids, _ := s.store.MemberIDs(ctx, g.ID); len(ids) > 1 {
			writeErr(w, http.StatusBadRequest, "make someone else an organiser first")
			return
		}
	}
	s.removeMember(ctx, g, uid, uid)
	w.WriteHeader(http.StatusNoContent)
}

// ---- members ----

// memberView is a person in a group as one viewer sees them: organisers see
// how to reach players (email, phone) and whether they have signed in yet;
// everyone else sees names.
type memberView struct {
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	FullName   string  `json:"full_name"`
	Nickname   string  `json:"nickname"`
	Role       string  `json:"role"`
	Account    string  `json:"account"`
	Email      string  `json:"email,omitempty"`
	Phone      string  `json:"phone,omitempty"`
	Joined     bool    `json:"joined"` // has used League (signed in once)
	JoinedAt   string  `json:"joined_at"`
	LastSeenAt *string `json:"last_seen_at,omitempty"`
	CanLink    bool    `json:"can_link"` // this viewer may send them a sign-in link
	LinkSentAt string  `json:"link_sent_at,omitempty"`
	Online     bool    `json:"online"`
}

func (s *Server) memberViews(ctx context.Context, g *store.Group, ms []*store.Member) []memberView {
	me := userFrom(ctx)
	org := organiser(g)
	out := make([]memberView, 0, len(ms))
	for _, m := range ms {
		v := memberView{UserID: m.UserID, Name: m.Display(), FullName: m.Name, Nickname: m.Nickname, Role: m.Role, Account: m.Account,
			Joined: m.LastSeenAt != nil, JoinedAt: m.JoinedAt, Online: s.hub.online(m.UserID)}
		if org || m.UserID == me.ID {
			v.Email, v.Phone, v.LastSeenAt = m.Email, m.Phone, m.LastSeenAt
		}
		if org && m.Account == store.RolePlayer {
			u := &store.User{ID: m.UserID, Role: m.Account, InvitedBy: m.InvitedBy}
			v.CanLink = s.canIssueLink(ctx, me, u)
			if v.CanLink {
				if l := s.store.LastSignInLink(ctx, m.UserID); l != nil {
					v.LinkSentAt = l.CreatedAt
				}
			}
		}
		out = append(out, v)
	}
	return out
}

func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request) {
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	ms, err := s.store.Members(r.Context(), g.ID)
	if err != nil {
		s.fail(w, err, "members")
		return
	}
	writeJSON(w, http.StatusOK, s.memberViews(r.Context(), g, ms))
}

// handleAddMember puts someone in the group. When nobody on the box has
// that email (or phone) a player account is made for them on the spot:
// they get in with a sign-in link, no password needed.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	me := userFrom(ctx)
	var in struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Phone string `json:"phone"`
		Role  string `json:"role"`
		Send  bool   `json:"send"` // email the sign-in link right away
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Name, in.Email, in.Phone = clip(strings.TrimSpace(in.Name), 80), strings.ToLower(strings.TrimSpace(in.Email)), clip(strings.TrimSpace(in.Phone), 40)
	if in.Email != "" && (!strings.Contains(in.Email, "@") || strings.ContainsAny(in.Email, " <>,;")) {
		writeErr(w, http.StatusBadRequest, "that email address does not look right")
		return
	}
	if in.Name == "" && in.Email == "" && in.Phone == "" {
		writeErr(w, http.StatusBadRequest, "give a name and an email or a phone number")
		return
	}
	if in.Email == "" && in.Phone == "" {
		writeErr(w, http.StatusBadRequest, "give an email or a phone number, so they can get their sign-in link")
		return
	}
	u, err := s.store.FindPlayer(ctx, in.Email, in.Phone)
	created := false
	if err != nil {
		name := in.Name
		if name == "" {
			name = firstNonEmpty(strings.SplitN(in.Email, "@", 2)[0], in.Phone)
		}
		u = &store.User{Email: in.Email, Name: name, Phone: in.Phone, Role: store.RolePlayer, InvitedBy: me.ID}
		if err := s.store.CreateUser(ctx, u); err != nil {
			s.fail(w, err, "create player")
			return
		}
		created = true
		s.log.Info("player account created", "group", g.ID, "by", me.ID, "player", u.ID)
	}
	added, err := s.store.AddMember(ctx, g.ID, u.ID, me.ID, in.Role)
	if err != nil {
		s.fail(w, err, "add member")
		return
	}
	if !added {
		writeErr(w, http.StatusConflict, firstNonEmpty(u.Name, "they")+" is already in the group")
		return
	}
	m, _ := s.store.GetMember(ctx, g.ID, u.ID)
	s.systemMessage(ctx, g, 0, "joined", m.Display()+" joined the group", map[string]any{"user_id": u.ID})
	s.toGroup(ctx, g.ID, "members", map[string]any{"group_id": g.ID})
	views := s.memberViews(ctx, g, []*store.Member{m})
	out := map[string]any{"member": views[0], "created": created}
	if !created && u.Role != store.RolePlayer {
		out["note"] = u.Name + " already has an account here: the group shows up for them next time they open League."
	}
	if in.Send && views[0].CanLink {
		res, err := s.issueLink(ctx, r, me, u, g, u.Email != "")
		if err == nil {
			out["link"] = res
		}
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	uid := idParam(r, "uid")
	m, err := s.store.GetMember(ctx, g.ID, uid)
	if err != nil {
		s.fail(w, err, "member")
		return
	}
	var in struct {
		Role     *string `json:"role"`
		Nickname *string `json:"nickname"`
		Name     *string `json:"name"`
		Email    *string `json:"email"`
		Phone    *string `json:"phone"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Role != nil {
		if *in.Role != "organiser" && *in.Role != "player" {
			writeErr(w, http.StatusBadRequest, "someone is an organiser or a player")
			return
		}
		if *in.Role == "player" && m.Role == "organiser" && s.store.CountOrganisers(ctx, g.ID) <= 1 {
			writeErr(w, http.StatusBadRequest, "a group needs at least one organiser")
			return
		}
	}
	if in.Nickname != nil {
		in.Nickname = ptr(clip(strings.TrimSpace(*in.Nickname), 40))
	}
	if err := s.store.UpdateMember(ctx, g.ID, uid, in.Role, in.Nickname); err != nil {
		s.fail(w, err, "save member")
		return
	}
	// the name, email and phone of a player account the organiser made
	if in.Name != nil || in.Email != nil || in.Phone != nil {
		u, err := s.store.GetUser(ctx, uid)
		if err == nil && u.Role == store.RolePlayer && s.canIssueLink(ctx, userFrom(ctx), u) {
			if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
				u.Name = clip(strings.TrimSpace(*in.Name), 80)
			}
			if in.Email != nil {
				e := strings.ToLower(strings.TrimSpace(*in.Email))
				if e != "" && !strings.Contains(e, "@") {
					writeErr(w, http.StatusBadRequest, "that email address does not look right")
					return
				}
				u.Email = e
			}
			if in.Phone != nil {
				u.Phone = clip(strings.TrimSpace(*in.Phone), 40)
			}
			if u.Email == "" && u.Phone == "" {
				writeErr(w, http.StatusBadRequest, "keep an email or a phone number")
				return
			}
			if err := s.store.UpdateUser(ctx, u); err != nil {
				s.fail(w, err, "save player")
				return
			}
		}
	}
	m, _ = s.store.GetMember(ctx, g.ID, uid)
	s.toGroup(ctx, g.ID, "members", map[string]any{"group_id": g.ID})
	writeJSON(w, http.StatusOK, s.memberViews(ctx, g, []*store.Member{m})[0])
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	uid := idParam(r, "uid")
	me := actorID(ctx)
	if uid != me && !s.needOrganiser(w, g) {
		return
	}
	m, err := s.store.GetMember(ctx, g.ID, uid)
	if err != nil {
		s.fail(w, err, "member")
		return
	}
	if m.Role == "organiser" && s.store.CountOrganisers(ctx, g.ID) <= 1 {
		writeErr(w, http.StatusBadRequest, "a group needs at least one organiser")
		return
	}
	s.removeMember(ctx, g, uid, me)
	w.WriteHeader(http.StatusNoContent)
}

// removeMember takes someone out; a player account left in no group at all
// is deleted with it (it only existed for the group), keeping the results.
func (s *Server) removeMember(ctx context.Context, g *store.Group, uid, by int64) {
	m, _ := s.store.GetMember(ctx, g.ID, uid)
	coming, _ := s.store.ListGames(ctx, uid, store.GameFilter{GroupID: g.ID, Upcoming: true})
	if err := s.store.RemoveMember(ctx, g.ID, uid); err != nil {
		s.log.Warn("remove member", "err", err)
		return
	}
	// their answers are gone: so are those games from their calendar
	for _, gm := range coming {
		s.syncCalendarLater(gm.ID, false)
	}
	if m != nil {
		what := m.Display() + " left the group"
		if by != uid {
			what = m.Display() + " is no longer in the group"
		}
		s.systemMessage(ctx, g, 0, "left", what, map[string]any{"user_id": uid})
	}
	s.toGroup(ctx, g.ID, "members", map[string]any{"group_id": g.ID})
	s.toPeople([]int64{uid}, "group", map[string]any{"id": g.ID, "deleted": true})
	if u, err := s.store.GetUser(ctx, uid); err == nil && u.Role == store.RolePlayer && s.store.PlayerUsage(ctx, uid) == 0 {
		_ = s.store.RevokeSignInLinks(ctx, uid)
		_ = s.store.DeleteSessions(ctx, uid)
	}
}

// ---- sign-in links for players ----

// canIssueLink: a sign-in link lets someone in as that person, so only for
// player accounts (never a member's or an admin's), and only by the
// organiser who made the account, an organiser of every group they are in,
// or an admin.
func (s *Server) canIssueLink(ctx context.Context, me, u *store.User) bool {
	if me == nil || u == nil || u.Role != store.RolePlayer || me.ID == u.ID || isPlayer(me) {
		return false
	}
	return u.InvitedBy == me.ID || me.Role == store.RoleAdmin || s.store.OrganisesAllGroupsOf(ctx, me.ID, u.ID)
}

// linkResult is a fresh sign-in link and the ways to send it.
type linkResult struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
	Text      string `json:"text"`     // the message to paste anywhere
	WhatsApp  string `json:"whatsapp"` // wa.me link with the message
	SMS       string `json:"sms"`
	Mailto    string `json:"mailto"`
	Emailed   bool   `json:"emailed"`
	EmailErr  string `json:"email_error,omitempty"`
	EmailVia  string `json:"email_via"`
}

// signInTTL is how long a sign-in link works.
const signInTTL = 14 * 24 * time.Hour

func (s *Server) issueLink(ctx context.Context, r *http.Request, me, u *store.User, g *store.Group, email bool) (*linkResult, error) {
	tok, l, err := s.store.CreateSignInLink(ctx, u.ID, me.ID, g.ID, signInTTL)
	if err != nil {
		return nil, err
	}
	link := s.publicBase(r) + "/join/" + tok
	who := firstNonEmpty(me.Name, me.Email)
	text := fmt.Sprintf("Hi %s! %s added you to “%s” (%s) in Pocket League — games, scores and the chat. Tap to get in (works once, for 14 days): %s",
		firstName(u.Name), who, g.Name, league.DisplayName(g.Sport, g.SportName), link)
	res := &linkResult{URL: link, ExpiresAt: l.ExpiresAt, Text: text,
		WhatsApp: "https://wa.me/" + waNumber(u.Phone) + "?text=" + url.QueryEscape(text),
		Mailto:   "mailto:" + u.Email + "?subject=" + url.PathEscape("Join "+g.Name) + "&body=" + url.PathEscape(text)}
	if u.Phone != "" {
		res.SMS = "sms:" + strings.ReplaceAll(u.Phone, " ", "") + "?&body=" + url.QueryEscape(text)
	}
	st, _ := s.store.GetSettings(ctx, me.ID)
	res.EmailVia = s.emailVia(st)
	if email && u.Email != "" {
		err := s.mailer(ctx, me.ID, s.inviteMail(ctx, me, u, g, link))
		if err == nil {
			res.Emailed = true
		} else {
			res.EmailErr = err.Error()
		}
	}
	return res, nil
}

func (s *Server) handleMemberLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	me := userFrom(ctx)
	u, err := s.store.GetUser(ctx, idParam(r, "uid"))
	if err != nil {
		s.fail(w, err, "player")
		return
	}
	if _, err := s.store.GetMember(ctx, g.ID, u.ID); err != nil {
		s.fail(w, err, "member")
		return
	}
	if !s.canIssueLink(ctx, me, u) {
		if u.Role != store.RolePlayer {
			writeErr(w, http.StatusBadRequest, firstNonEmpty(u.Name, "they")+" has a full account here and signs in as usual; the group is already on their list")
			return
		}
		writeErr(w, http.StatusForbidden, "only the organiser who added "+firstNonEmpty(u.Name, "them")+" can send them a sign-in link")
		return
	}
	var in struct {
		Email bool `json:"email"`
	}
	_ = decode(r, &in)
	res, err := s.issueLink(ctx, r, me, u, g, in.Email)
	if err != nil {
		s.fail(w, err, "sign-in link")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleInviteAll emails a sign-in link to every player who never came in.
func (s *Server) handleInviteAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	me := userFrom(ctx)
	ms, _ := s.store.Members(ctx, g.ID)
	sent, failed, noEmail := 0, 0, []string{}
	lastErr := ""
	for _, m := range ms {
		if m.Account != store.RolePlayer || m.LastSeenAt != nil {
			continue
		}
		u, err := s.store.GetUser(ctx, m.UserID)
		if err != nil || !s.canIssueLink(ctx, me, u) {
			continue
		}
		if u.Email == "" {
			noEmail = append(noEmail, m.Display())
			continue
		}
		res, err := s.issueLink(ctx, r, me, u, g, true)
		switch {
		case err != nil:
			failed++
			lastErr = err.Error()
		case res.Emailed:
			sent++
		default:
			failed++
			lastErr = res.EmailErr
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "failed": failed, "error": lastErr, "no_email": noEmail})
}

// ---- helpers ----

// publicBase is the address people open League at.
func (s *Server) publicBase(r *http.Request) string {
	if s.cfg.BaseURL != "" {
		return strings.TrimRight(s.cfg.BaseURL, "/")
	}
	if r == nil {
		return ""
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) feedURL(r *http.Request, gid int64) string {
	m, err := s.store.GetMember(r.Context(), gid, actorID(r.Context()))
	if err != nil {
		return ""
	}
	return s.publicBase(r) + "/feed/" + m.FeedToken + ".ics"
}

func firstName(n string) string {
	n = strings.TrimSpace(n)
	if i := strings.IndexAny(n, " \t"); i > 0 {
		return n[:i]
	}
	if n == "" {
		return "there"
	}
	return n
}

// waNumber is a phone number as wa.me wants it: digits only, with the country code.
func waNumber(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	return strings.TrimPrefix(out, "00")
}
