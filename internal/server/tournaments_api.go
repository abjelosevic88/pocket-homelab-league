package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
	"github.com/abjelosevic88/pocket-homelab-league/internal/tour"
)

// ---- the tournament API ----

// myTournament loads a tournament of a group the person is in.
func (s *Server) myTournament(w http.ResponseWriter, r *http.Request) (*store.Tournament, *store.Group, bool) {
	ctx := r.Context()
	t, err := s.store.GetTournament(ctx, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "tournament")
		return nil, nil, false
	}
	g, err := s.store.GetGroup(ctx, actorID(ctx), t.GroupID)
	if err != nil {
		s.fail(w, err, "tournament")
		return nil, nil, false
	}
	return t, g, true
}

func (s *Server) needRunner(w http.ResponseWriter, t *store.Tournament, g *store.Group, uid int64) bool {
	if !canRun(t, g, uid) {
		writeErr(w, http.StatusForbidden, "only the group's organisers and whoever made the tournament can do that")
		return false
	}
	return true
}

func (s *Server) handleTournamentFormats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := s.store.GetGroup(ctx, actorID(ctx), int64(queryInt(r, "group")))
	if err != nil {
		s.fail(w, err, "group")
		return
	}
	defaults := map[string]TSettings{}
	for _, f := range tour.Formats {
		defaults[f.Key] = defaultSettings(g, f.Key, 0)
	}
	writeJSON(w, http.StatusOK, map[string]any{"formats": tour.Formats, "suggested": tour.Suggested(g.Sport, g.Scoring, g.Format), "defaults": defaults,
		"tiebreaks": tour.TiebreakWords, "rules": tour.RulesFor(g.Sport, g.Scoring, "round_robin")})
}

func (s *Server) handleListTournaments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var ts []*store.Tournament
	var err error
	if chiParam(r, "id") != "" {
		g, ok := s.myGroup(w, r, "id")
		if !ok {
			return
		}
		ts, err = s.store.Tournaments(ctx, g.ID)
	} else {
		ts, err = s.store.TournamentsOf(ctx, uid)
	}
	if err != nil {
		s.fail(w, err, "tournaments")
		return
	}
	out := make([]tView, 0, len(ts))
	groups := map[int64]*store.Group{}
	for _, t := range ts {
		g := groups[t.GroupID]
		if g == nil {
			if g, err = s.store.GetGroup(ctx, uid, t.GroupID); err != nil {
				continue
			}
			groups[t.GroupID] = g
		}
		out = append(out, s.tView(ctx, t, g, uid))
	}
	writeJSON(w, http.StatusOK, out)
}

// tInput is what may be set on a tournament.
type tInput struct {
	Name        *string         `json:"name"`
	Format      *string         `json:"format"`
	EntryKind   *string         `json:"entry_kind"`
	Rated       *bool           `json:"rated"`
	StartsAt    *string         `json:"starts_at"`
	EndsAt      *string         `json:"ends_at"`
	VenueName   *string         `json:"venue_name"`
	Capacity    *int            `json:"capacity"`
	SignupUntil *string         `json:"signup_until"`
	FeeCents    *int64          `json:"fee_cents"`
	Currency    *string         `json:"currency"`
	Checkin     *bool           `json:"checkin"`
	Settings    json.RawMessage `json:"settings"`
	Courts      []string        `json:"courts"`
	GameID      *int64          `json:"game_id"`
	AddMembers  bool            `json:"add_members"` // everyone in the group signs up at once (players)
	FromGame    int64           `json:"from_game"`   // everyone who is in for this game signs up
}

func (in tInput) apply(t *store.Tournament, g *store.Group) string {
	if in.Name != nil {
		t.Name = clip(strings.TrimSpace(*in.Name), 100)
	}
	if t.Name == "" {
		return "give the tournament a name"
	}
	if in.Format != nil {
		if t.Status != "signup" && *in.Format != t.Format {
			return "the format is fixed once the draw is made"
		}
		f, ok := tour.FormatByKey(*in.Format)
		if !ok {
			return "unknown format"
		}
		t.Format = f.Key
		if !oneOf(t.EntryKind, f.Entries) {
			t.EntryKind = f.Entries[0]
		}
	}
	if in.EntryKind != nil {
		f, _ := tour.FormatByKey(t.Format)
		if !oneOf(*in.EntryKind, f.Entries) {
			return "this format is for " + strings.Join(f.Entries, " or ") + " entries"
		}
		if t.Status != "signup" && *in.EntryKind != t.EntryKind {
			return "who enters is fixed once the draw is made"
		}
		t.EntryKind = *in.EntryKind
	}
	if in.Rated != nil {
		t.Rated = *in.Rated
	}
	for _, x := range []struct {
		v   *string
		dst *string
	}{{in.StartsAt, &t.StartsAt}, {in.EndsAt, &t.EndsAt}, {in.SignupUntil, &t.SignupUntil}} {
		if x.v != nil {
			if *x.v != "" && !validTS(*x.v) {
				return "a time is a date and a time"
			}
			*x.dst = *x.v
		}
	}
	if in.VenueName != nil {
		t.VenueName = clip(strings.TrimSpace(*in.VenueName), 120)
	}
	if in.Capacity != nil {
		if *in.Capacity < 0 || *in.Capacity > 512 {
			return "at most 512 entries"
		}
		t.Capacity = *in.Capacity
	}
	if in.FeeCents != nil {
		if *in.FeeCents < 0 {
			return "the fee is 0 or more"
		}
		t.FeeCents = *in.FeeCents
	}
	if in.Currency != nil {
		if c := cleanCurrency(*in.Currency); c != "" {
			t.Currency = c
		}
	}
	if in.Checkin != nil {
		t.Checkin = *in.Checkin
	}
	if in.GameID != nil {
		t.GameID = *in.GameID
	}
	if len(in.Settings) > 0 && string(in.Settings) != "null" {
		set := settingsOf(t)
		if err := json.Unmarshal(in.Settings, &set); err != nil {
			return "the settings are not right"
		}
		if set.Groups < 0 || set.Groups > 32 || set.Advance < 0 || set.Advance > 16 || set.Rounds < 0 || set.Rounds > 60 || set.Points < 0 || set.Points > 200 {
			return "a setting is out of range"
		}
		for _, k := range set.Tiebreaks {
			if _, ok := tour.TiebreakWords[k]; !ok {
				return "unknown tie-break: " + k
			}
		}
		b, _ := json.Marshal(set)
		t.Settings = b
	}
	return ""
}

func (s *Server) handleCreateTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	u := userFrom(ctx)
	var in tInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Format == nil {
		sug := tour.Suggested(g.Sport, g.Scoring, g.Format)
		in.Format = &sug[0]
	}
	t := &store.Tournament{GroupID: g.ID, Status: "signup", Rated: true, Currency: g.Currency, VenueName: g.VenueName, EntryKind: "player"}
	if g.Format == league.FormatDoubles {
		t.EntryKind = "pair"
	}
	if g.Format == league.FormatTeams {
		t.EntryKind = "team"
	}
	def, _ := json.Marshal(defaultSettings(g, *in.Format, 0))
	t.Settings = def
	if msg := in.apply(t, g); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateTournament(ctx, u.ID, t); err != nil {
		s.fail(w, err, "create tournament")
		return
	}
	courts := in.Courts
	if courts == nil {
		set := settingsOf(t)
		if set.Schedule != "none" {
			courts = []string{"Court 1", "Court 2"}
			if g.Format == league.FormatTeams {
				courts = []string{"Pitch 1"}
			}
		}
	}
	_ = s.store.SetCourts(ctx, t.ID, cleanNames(courts))
	if in.AddMembers && t.EntryKind == "player" {
		ms, _ := s.store.Members(ctx, g.ID)
		for _, m := range ms {
			_ = s.store.SaveEntry(ctx, &store.TEntry{TournamentID: t.ID, Name: m.Display(), Members: []store.TEntryMember{{UserID: m.UserID}}, CreatedBy: u.ID})
		}
	}
	if in.FromGame != 0 {
		_, _ = s.entriesFromGame(ctx, t, g, in.FromGame, u.ID)
	}
	s.store.AddTLog(ctx, t.ID, u.ID, "created", t.Name)
	s.systemMessage(ctx, g, 0, "tournament", fmt.Sprintf("%s made a tournament: %s — sign up!", firstNonEmpty(u.Name, "Someone"), t.Name), map[string]any{"tournament_id": t.ID})
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	s.tellGroup(ctx, g, u.ID, note{kind: "plans", title: g.Name + ": " + t.Name, body: "A new tournament — sign up if you're in.", link: s.tLink(t.ID), button: "Sign up"})
	writeJSON(w, http.StatusCreated, s.tDetail(ctx, t, g, u.ID, false))
}

func cleanNames(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, n := range in {
		n = clip(strings.TrimSpace(n), 40)
		if n == "" || seen[strings.ToLower(n)] || len(out) >= 40 {
			continue
		}
		seen[strings.ToLower(n)] = true
		out = append(out, n)
	}
	return out
}

func (s *Server) entriesFromGame(ctx context.Context, t *store.Tournament, g *store.Group, gameID, by int64) (int, error) {
	gm, err := s.store.GetGame(ctx, by, gameID)
	if err != nil || gm.GroupID != g.ID {
		return 0, store.ErrNotFound
	}
	answers, _ := s.store.Answers(ctx, gm.ID)
	es, _ := s.store.Entries(ctx, t.ID)
	have := map[int64]bool{}
	for _, e := range es {
		for _, m := range e.Members {
			have[m.UserID] = true
		}
	}
	n := 0
	for _, a := range answers {
		if a.Answer != store.AnswerIn || have[a.UserID] {
			continue
		}
		e := &store.TEntry{TournamentID: t.ID, Name: s.nameIn(ctx, g.ID, a.UserID), Members: []store.TEntryMember{{UserID: a.UserID}}, CreatedBy: by}
		if err := s.store.SaveEntry(ctx, e); err == nil {
			n++
		}
	}
	if t.GameID == 0 {
		t.GameID = gm.ID
		if t.StartsAt == "" {
			t.StartsAt = gm.StartsAt
		}
		_ = s.store.UpdateTournament(ctx, t)
	}
	return n, nil
}

// tDetail is everything a tournament page shows.
func (s *Server) tDetail(ctx context.Context, t *store.Tournament, g *store.Group, uid int64, public bool) map[string]any {
	es, _ := s.store.Entries(ctx, t.ID)
	stages, _ := s.store.Stages(ctx, t.ID)
	groups, _ := s.store.TGroups(ctx, t.ID)
	ms, _ := s.store.TMatches(ctx, t.ID)
	courts, _ := s.store.Courts(ctx, t.ID)
	names := s.names(ctx, g.ID)
	mine := []int64{}
	for _, e := range es {
		for _, m := range e.Members {
			if m.UserID == uid && e.Status != "withdrawn" {
				mine = append(mine, e.ID)
			}
		}
	}
	type stageView struct {
		*store.TStage
		Tables      map[int64][]*tour.Row `json:"tables"`
		Leaderboard []*tour.Leader        `json:"leaderboard,omitempty"`
		Cards       []store.Scorecard     `json:"cards,omitempty"`
	}
	sv := []stageView{}
	for _, st := range stages {
		v := stageView{TStage: st, Tables: map[int64][]*tour.Row{}}
		switch st.Kind {
		case "knockout", "double_elim":
		case "ranking":
			v.Leaderboard = s.leaderboard(ctx, t, st)
			v.Cards, _ = s.store.Scorecards(ctx, st.ID)
		default:
			v.Tables = s.stageStandings(ctx, t, st, ms, g)
		}
		sv = append(sv, v)
	}
	view := s.tView(ctx, t, g, uid)
	if public {
		view.ShareToken = ""
	}
	// the podium: the places of the last stage once it is done
	podium := []map[string]int64{}
	if len(stages) > 0 && (t.Status == "finished" || stages[len(stages)-1].Status == "done") {
		last := stages[len(stages)-1].ID
		xs, _ := s.store.StageEntries(ctx, t.ID)
		for _, x := range xs {
			if x.StageID == last && x.FinalRank >= 1 && x.FinalRank <= 3 {
				podium = append(podium, map[string]int64{"rank": int64(x.FinalRank), "entry_id": x.EntryID})
			}
		}
		sort.Slice(podium, func(i, j int) bool { return podium[i]["rank"] < podium[j]["rank"] })
	}
	out := map[string]any{"tournament": view, "group": viewGroup(g), "entries": es, "stages": sv, "groups": groups, "matches": ms, "podium": podium,
		"courts": courts, "names": names, "mine": mine, "can_run": !public && canRun(t, g, uid), "settings": settingsOf(t), "now": store.LocalNow()}
	if t.Format == "ladder" {
		out["challenges"], _ = s.store.Challenges(ctx, t.ID)
	}
	if !public {
		out["log"], _ = s.store.TLogs(ctx, t.ID, 40)
		out["share_url"] = s.cfg.BaseURL + "/t/" + t.ShareToken
		if t.ShareToken != "" && s.cfg.BaseURL == "" {
			out["share_url"] = "/t/" + t.ShareToken
		}
	}
	return out
}

func (s *Server) handleGetTournament(w http.ResponseWriter, r *http.Request) {
	t, g, ok := s.myTournament(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.tDetail(r.Context(), t, g, actorID(r.Context()), false))
}

// handlePublicTournament is the read-only page for anyone with the link.
func (s *Server) handlePublicTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, err := s.store.TournamentByToken(ctx, chiParam(r, "token"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	g, err := s.store.GroupByID(ctx, t.GroupID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// the group as the public sees it: no people's details
	g.MyRole = ""
	d := s.tDetail(ctx, t, g, 0, true)
	delete(d, "log")
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUpdateTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	var in tInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(t, g); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateTournament(ctx, t); err != nil {
		s.fail(w, err, "save tournament")
		return
	}
	if in.Courts != nil {
		_ = s.store.SetCourts(ctx, t.ID, cleanNames(in.Courts))
		s.schedule(ctx, t)
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, actorID(ctx), false))
}

func (s *Server) handleDeleteTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	if err := s.store.DeleteTournament(ctx, t.ID); err != nil {
		s.fail(w, err, "delete tournament")
		return
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID, "deleted": true})
	w.WriteHeader(http.StatusNoContent)
}

// ---- entries ----

// entryName is how an entry is called: the person, "Ana & Marko", or the team's name.
func (s *Server) entryName(ctx context.Context, gid int64, members []store.TEntryMember, given string) string {
	if given = clip(strings.TrimSpace(given), 60); given != "" {
		return given
	}
	var ns []string
	for _, m := range members {
		n := s.nameIn(ctx, gid, m.UserID)
		if len(members) > 1 {
			n = firstName(n)
		}
		ns = append(ns, n)
	}
	return strings.Join(ns, " & ")
}

// promoteWaiting moves people up from the waiting list while there is room.
func (s *Server) promoteWaiting(ctx context.Context, t *store.Tournament, g *store.Group) {
	if t.Capacity <= 0 {
		return
	}
	es, _ := s.store.Entries(ctx, t.ID)
	in := 0
	for _, e := range es {
		if e.Status == "in" {
			in++
		}
	}
	for _, e := range es {
		if in >= t.Capacity {
			return
		}
		if e.Status == "waiting" {
			e.Status = "in"
			_ = s.store.SaveEntry(ctx, e)
			in++
			var uids []int64
			for _, m := range e.Members {
				uids = append(uids, m.UserID)
			}
			s.tell(ctx, g, uids, 0, note{kind: "plans", title: "You're in: " + t.Name, body: "A place came free and you were first on the waiting list.", link: s.tLink(t.ID)})
		}
	}
}

func (s *Server) handleJoinTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	if t.Status != "signup" {
		writeErr(w, http.StatusBadRequest, "sign-up is closed: the draw is made")
		return
	}
	if t.SignupUntil != "" && store.LocalNow() > t.SignupUntil {
		writeErr(w, http.StatusBadRequest, "sign-up closed on "+whenText(t.SignupUntil))
		return
	}
	var in struct {
		Partner  int64    `json:"partner"`
		TeamName string   `json:"team_name"`
		Handicap *float64 `json:"handicap"`
	}
	_ = decode(r, &in)
	es, _ := s.store.Entries(ctx, t.ID)
	for _, e := range es {
		for _, m := range e.Members {
			if (m.UserID == u.ID || (in.Partner != 0 && m.UserID == in.Partner)) && e.Status != "withdrawn" {
				writeErr(w, http.StatusConflict, s.nameIn(ctx, g.ID, m.UserID)+" is already in")
				return
			}
		}
	}
	members := []store.TEntryMember{{UserID: u.ID}}
	switch t.EntryKind {
	case "pair":
		if in.Partner == 0 || in.Partner == u.ID {
			writeErr(w, http.StatusBadRequest, "pick your partner")
			return
		}
		if _, err := s.store.GetMember(ctx, g.ID, in.Partner); err != nil {
			writeErr(w, http.StatusBadRequest, "your partner must be in the group")
			return
		}
		members = append(members, store.TEntryMember{UserID: in.Partner})
	case "team":
		name := clip(strings.TrimSpace(in.TeamName), 60)
		if name != "" {
			for _, e := range es {
				if strings.EqualFold(e.Name, name) && e.Status != "withdrawn" {
					e.Members = append(e.Members, store.TEntryMember{UserID: u.ID})
					if err := s.store.SaveEntry(ctx, e); err != nil {
						s.fail(w, err, "join team")
						return
					}
					s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
					writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, u.ID, false))
					return
				}
			}
		}
		members[0].Role = "captain"
		in.TeamName = name
	}
	status := "in"
	if t.Capacity > 0 {
		n := 0
		for _, e := range es {
			if e.Status == "in" {
				n++
			}
		}
		if n >= t.Capacity {
			status = "waiting"
		}
	}
	e := &store.TEntry{TournamentID: t.ID, Name: s.entryName(ctx, g.ID, members, in.TeamName), Status: status, Members: members, CreatedBy: u.ID, Handicap: in.Handicap}
	if err := s.store.SaveEntry(ctx, e); err != nil {
		s.fail(w, err, "sign up")
		return
	}
	s.store.AddTLog(ctx, t.ID, u.ID, "signed_up", e.Name)
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, u.ID, false))
}

func (s *Server) handleLeaveTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	es, _ := s.store.Entries(ctx, t.ID)
	for _, e := range es {
		for _, m := range e.Members {
			if m.UserID == u.ID && e.Status != "withdrawn" {
				if err := s.withdraw(ctx, t, g, e, u.ID); err != nil {
					s.fail(w, err, "withdraw")
					return
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, u.ID, false))
}

// withdraw takes an entry out: before the draw it goes (and the waiting
// list moves up); once playing, its opponents win the rest by walkover.
func (s *Server) withdraw(ctx context.Context, t *store.Tournament, g *store.Group, e *store.TEntry, by int64) error {
	if t.Status == "signup" {
		if err := s.store.DeleteEntry(ctx, t.ID, e.ID); err != nil {
			return err
		}
		s.promoteWaiting(ctx, t, g)
	} else {
		e.Status = "withdrawn"
		if err := s.store.SaveEntry(ctx, e); err != nil {
			return err
		}
		s.walkovers(ctx, t, g, e.ID, by)
	}
	s.store.AddTLog(ctx, t.ID, by, "withdrew", e.Name)
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	return nil
}

// walkovers gives an entry's remaining matches to the other side.
func (s *Server) walkovers(ctx context.Context, t *store.Tournament, g *store.Group, eid, by int64) {
	ms, _ := s.store.TMatches(ctx, t.ID)
	for _, m := range ms {
		if m.Status == "done" || m.Status == "void" {
			continue
		}
		in := false
		for _, x := range s.sideEntries(m) {
			if x == eid {
				in = true
			}
		}
		if !in {
			continue
		}
		now := store.Now()
		if len(m.Sides["a"]) > 0 {
			m.Status, m.Outcome = "void", "withdrawn"
			_ = s.store.SaveTMatch(ctx, m)
			continue
		}
		switch {
		case m.AEntry == eid && m.BEntry != 0:
			m.Winner = "b"
		case m.BEntry == eid && m.AEntry != 0:
			m.Winner = "a"
		default:
			continue // the other side is not known yet: settled when it is
		}
		m.Status, m.Outcome, m.ConfirmedAt, m.ConfirmedBy = "done", "walkover", &now, by
		if err := s.store.SaveTMatch(ctx, m); err == nil {
			s.confirmed(ctx, t, g, m, by)
		}
	}
}

func (s *Server) handleAddEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	var in struct {
		Name     string   `json:"name"`
		Members  []int64  `json:"members"`
		Seed     int      `json:"seed"`
		Handicap *float64 `json:"handicap"`
		Note     string   `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if t.Status == "finished" || t.Status == "cancelled" {
		writeErr(w, http.StatusBadRequest, "the tournament is over")
		return
	}
	if t.Status == "live" && t.Format != "ladder" && t.Format != "arena" && t.Format != "ranking" {
		writeErr(w, http.StatusBadRequest, "the draw is made: redraw to add someone")
		return
	}
	var members []store.TEntryMember
	for i, id := range in.Members {
		if _, err := s.store.GetMember(ctx, g.ID, id); err != nil {
			writeErr(w, http.StatusBadRequest, "everyone in an entry must be in the group")
			return
		}
		role := "player"
		if t.EntryKind == "team" && i == 0 {
			role = "captain"
		}
		members = append(members, store.TEntryMember{UserID: id, Role: role})
	}
	if len(members) == 0 && strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "pick who plays, or give a name (a guest)")
		return
	}
	e := &store.TEntry{TournamentID: t.ID, Name: s.entryName(ctx, g.ID, members, in.Name), Members: members, Seed: in.Seed, Handicap: in.Handicap, Note: clip(in.Note, 200), CreatedBy: actorID(ctx), Rating: league.StartRating}
	if t.Status == "live" {
		es, _ := s.store.Entries(ctx, t.ID)
		e.Position = len(es) + 1
	}
	if err := s.store.SaveEntry(ctx, e); err != nil {
		s.fail(w, err, "add entry")
		return
	}
	if t.Status == "live" {
		// late entries join the running stage (ladder at the bottom, arena and scored rounds at once)
		stages, _ := s.store.Stages(ctx, t.ID)
		groups, _ := s.store.TGroups(ctx, t.ID)
		for _, st := range stages {
			if st.Status != "live" {
				continue
			}
			xs, _ := s.store.StageEntries(ctx, t.ID)
			var list []store.StageEntry
			var gid int64
			for _, x := range xs {
				if x.StageID == st.ID {
					list = append(list, x)
				}
			}
			for _, tg := range groups {
				if tg.StageID == st.ID {
					gid = tg.ID
				}
			}
			list = append(list, store.StageEntry{StageID: st.ID, GroupID: gid, EntryID: e.ID, Seed: len(list) + 1})
			_ = s.store.SetStageEntries(ctx, st.ID, list)
			if st.Kind == "arena" {
				_ = s.arenaPair(ctx, t, st)
			}
		}
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, actorID(ctx), false))
}

func (s *Server) handleUpdateEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	e, err := s.findEntry(ctx, t.ID, idParam(r, "eid"))
	if err != nil {
		s.fail(w, err, "entry")
		return
	}
	own := false
	for _, m := range e.Members {
		if m.UserID == uid {
			own = true
		}
	}
	runner := canRun(t, g, uid)
	if !runner && !own {
		writeErr(w, http.StatusForbidden, "only the organisers and the entry's own people can change it")
		return
	}
	var in struct {
		Name      *string  `json:"name"`
		Seed      *int     `json:"seed"`
		Handicap  *float64 `json:"handicap"`
		ClearHcp  bool     `json:"clear_handicap"`
		Paid      *bool    `json:"paid"`
		CheckedIn *bool    `json:"checked_in"`
		Status    *string  `json:"status"`
		Members   []int64  `json:"members"`
		Note      *string  `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		e.Name = clip(strings.TrimSpace(*in.Name), 60)
	}
	if in.Handicap != nil {
		e.Handicap = in.Handicap
	}
	if in.ClearHcp {
		e.Handicap = nil
	}
	if in.CheckedIn != nil {
		e.CheckedIn = *in.CheckedIn
	}
	if in.Note != nil {
		e.Note = clip(*in.Note, 200)
	}
	if runner {
		if in.Seed != nil && *in.Seed >= 0 {
			e.Seed = *in.Seed
		}
		if in.Paid != nil {
			e.Paid = *in.Paid
		}
		if in.Members != nil && t.Status == "signup" {
			var ms []store.TEntryMember
			for i, id := range in.Members {
				if _, err := s.store.GetMember(ctx, g.ID, id); err != nil {
					writeErr(w, http.StatusBadRequest, "everyone in an entry must be in the group")
					return
				}
				role := "player"
				if t.EntryKind == "team" && i == 0 {
					role = "captain"
				}
				ms = append(ms, store.TEntryMember{UserID: id, Role: role})
			}
			e.Members = ms
		}
		if in.Status != nil {
			switch *in.Status {
			case "in", "waiting":
				if t.Status == "signup" {
					e.Status = *in.Status
				}
			case "withdrawn", "disqualified":
				if err := s.withdraw(ctx, t, g, e, uid); err != nil {
					s.fail(w, err, "withdraw")
					return
				}
				if *in.Status == "disqualified" && t.Status != "signup" {
					e.Status = "disqualified"
					_ = s.store.SaveEntry(ctx, e)
				}
				writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, uid, false))
				return
			}
		}
	}
	if err := s.store.SaveEntry(ctx, e); err != nil {
		s.fail(w, err, "save entry")
		return
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, uid, false))
}

func (s *Server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	e, err := s.findEntry(ctx, t.ID, idParam(r, "eid"))
	if err != nil {
		s.fail(w, err, "entry")
		return
	}
	if err := s.withdraw(ctx, t, g, e, actorID(ctx)); err != nil {
		s.fail(w, err, "remove entry")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEntriesFromGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	var in struct {
		GameID int64 `json:"game_id"`
	}
	_ = decode(r, &in)
	n, err := s.entriesFromGame(ctx, t, g, in.GameID, actorID(ctx))
	if err != nil {
		s.fail(w, err, "from the game")
		return
	}
	d := s.tDetail(ctx, t, g, actorID(ctx), false)
	d["added"] = n
	writeJSON(w, http.StatusOK, d)
}

// handleFairTeams turns the single sign-ups into teams of close ratings.
func (s *Server) handleFairTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	var in struct {
		Teams int `json:"teams"`
	}
	_ = decode(r, &in)
	if t.Status != "signup" {
		writeErr(w, http.StatusBadRequest, "teams are made before the draw")
		return
	}
	es, _ := s.store.Entries(ctx, t.ID)
	var players []*store.TEntry
	for _, e := range es {
		if e.Status == "in" && len(e.Members) == 1 {
			players = append(players, e)
		}
	}
	if in.Teams < 2 || len(players) < in.Teams {
		writeErr(w, http.StatusBadRequest, "at least two teams, and a player for each")
		return
	}
	s.refreshRatings(ctx, g, players)
	sort.SliceStable(players, func(i, j int) bool { return players[i].Rating > players[j].Rating })
	ids := make([]int64, len(players))
	byID := map[int64]*store.TEntry{}
	for i, p := range players {
		ids[i] = p.ID
		byID[p.ID] = p
	}
	teams := tour.Snake(ids, in.Teams)
	for i, team := range teams {
		var ms []store.TEntryMember
		for j, eid := range team {
			role := "player"
			if j == 0 {
				role = "captain"
			}
			ms = append(ms, store.TEntryMember{UserID: byID[eid].Members[0].UserID, Role: role})
			_ = s.store.DeleteEntry(ctx, t.ID, eid)
		}
		name := fmt.Sprintf("Team %s", string(rune('A'+i)))
		if len(ms) > 0 {
			name = "Team " + firstName(s.nameIn(ctx, g.ID, ms[0].UserID))
		}
		_ = s.store.SaveEntry(ctx, &store.TEntry{TournamentID: t.ID, Name: name, Members: ms, CreatedBy: actorID(ctx)})
	}
	t.EntryKind = "team"
	_ = s.store.UpdateTournament(ctx, t)
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, actorID(ctx), false))
}

// ---- running it ----

func (s *Server) handleStartTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	if t.Status != "signup" {
		writeErr(w, http.StatusBadRequest, "the draw is already made")
		return
	}
	if err := s.draw(ctx, t, g); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u := userFrom(ctx)
	s.store.AddTLog(ctx, t.ID, u.ID, "draw", "")
	s.systemMessage(ctx, g, 0, "tournament", t.Name+" has started — good luck everyone!", map[string]any{"tournament_id": t.ID})
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	es, _ := s.store.Entries(ctx, t.ID)
	var uids []int64
	for _, e := range es {
		for _, m := range e.Members {
			uids = append(uids, m.UserID)
		}
	}
	s.tell(ctx, g, uids, u.ID, note{kind: "plans", title: t.Name + " has started", body: "The draw is made: see who you play first.", link: s.tLink(t.ID), button: "See the draw"})
	s.publishGroup(ctx, g, "league.tournament_started", fmt.Sprintf("tournament:%d:started", t.ID), map[string]any{"tournament_id": t.ID, "group": g.Name, "name": t.Name, "url": s.tLink(t.ID)})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, u.ID, false))
}

// handleRedraw goes back to sign-up while nothing has been played.
func (s *Server) handleRedraw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	ms, _ := s.store.TMatches(ctx, t.ID)
	for _, m := range ms {
		if m.Status == "done" && m.Outcome != "bye" {
			writeErr(w, http.StatusBadRequest, "results are in already: undo them first, or finish it")
			return
		}
	}
	_ = s.store.DeleteStagesFrom(ctx, t.ID, 0)
	t.Status = "signup"
	_ = s.store.UpdateTournament(ctx, t)
	s.store.AddTLog(ctx, t.ID, actorID(ctx), "redraw", "")
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, actorID(ctx), false))
}

// handleFinishTournament ends a ladder, a box league, an arena or scored
// rounds (or anything, early) with the places as they stand.
func (s *Server) handleFinishTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	var in struct {
		Cancel bool `json:"cancel"`
	}
	_ = decode(r, &in)
	if in.Cancel {
		t.Status = "cancelled"
		_ = s.store.UpdateTournament(ctx, t)
		s.systemMessage(ctx, g, 0, "tournament", t.Name+" is cancelled", map[string]any{"tournament_id": t.ID})
	} else {
		stages, _ := s.store.Stages(ctx, t.ID)
		var last *store.TStage
		for _, st := range stages {
			last = st
		}
		if last == nil {
			writeErr(w, http.StatusBadRequest, "make the draw first")
			return
		}
		ms, _ := s.store.TMatches(ctx, t.ID)
		for _, m := range ms {
			if m.Status != "done" && m.Status != "void" {
				m.Status, m.Outcome = "void", "not played"
				_ = s.store.SaveTMatch(ctx, m)
			}
		}
		for _, st := range stages {
			if st.Status != "done" {
				st.Status = "done"
				_ = s.store.SaveStage(ctx, st)
			}
		}
		ranks := s.finalRanks(ctx, t, last, ms)
		for id, rk := range ranks {
			_ = s.store.SetFinalRank(ctx, last.ID, id, rk)
		}
		_ = s.finishTournament(ctx, t, g, last, ranks)
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, actorID(ctx), false))
}

// handleRegenerate draws the rounds not played yet again (an Americano
// after someone left or came late).
func (s *Server) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	stages, _ := s.store.Stages(ctx, t.ID)
	for _, st := range stages {
		if st.Status != "live" {
			continue
		}
		ms, _ := s.store.TMatches(ctx, t.ID)
		first := 1 << 30
		for _, m := range ms {
			if m.StageID == st.ID && m.Status != "done" && m.Status != "reported" && m.Round < first {
				first = m.Round
			}
		}
		if first == 1<<30 {
			continue
		}
		// a round partly played stays; the next ones are drawn again
		for _, m := range ms {
			if m.StageID == st.ID && m.Round == first && (m.Status == "done" || m.Status == "reported") {
				first++
				break
			}
		}
		_ = s.store.DeleteTMatches(ctx, st.ID, first, true)
		switch st.Kind {
		case "americano":
			all, _ := s.store.TMatches(ctx, t.ID)
			players := s.stageIDs(ctx, t, st.ID)
			h, _ := s.rotHistory(ctx, st.ID, all, players)
			set := settingsOf(t)
			rounds := set.Rounds
			if rounds <= 0 {
				rounds = len(players) - 1
			}
			groups, _ := s.store.TGroups(ctx, t.ID)
			var gid int64
			for _, tg := range groups {
				if tg.StageID == st.ID {
					gid = tg.ID
				}
			}
			for rd := first; rd <= rounds; rd++ {
				cs, sit := tour.AmericanoRound(players, s.courtsFor(ctx, t), h, time.Now().UnixNano()+int64(rd))
				h.Add(cs, sit)
				_ = s.saveCourts(ctx, t, st, gid, rd, cs)
			}
		case "mexicano", "kotc", "swiss":
			_ = s.nextRound(ctx, t, st)
		case "arena":
			_ = s.arenaPair(ctx, t, st)
		}
	}
	s.schedule(ctx, t)
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, actorID(ctx), false))
}

// ---- matches ----

func (s *Server) myTMatch(w http.ResponseWriter, r *http.Request) (*store.TMatch, *store.Tournament, *store.Group, bool) {
	ctx := r.Context()
	m, err := s.store.GetTMatch(ctx, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "match")
		return nil, nil, nil, false
	}
	t, err := s.store.GetTournament(ctx, m.TournamentID)
	if err != nil {
		s.fail(w, err, "match")
		return nil, nil, nil, false
	}
	g, err := s.store.GetGroup(ctx, actorID(ctx), t.GroupID)
	if err != nil {
		s.fail(w, err, "match")
		return nil, nil, nil, false
	}
	return m, t, g, true
}

// sideOf is the side of the match the person plays on ("" = neither).
func (s *Server) sideOf(ctx context.Context, t *store.Tournament, m *store.TMatch, uid int64) string {
	es, _ := s.store.Entries(ctx, t.ID)
	mine := map[int64]bool{}
	for _, e := range es {
		for _, mem := range e.Members {
			if mem.UserID == uid {
				mine[e.ID] = true
			}
		}
	}
	for _, side := range []string{"a", "b"} {
		list := m.Sides[side]
		if len(list) == 0 {
			if side == "a" {
				list = []int64{m.AEntry}
			} else {
				list = []int64{m.BEntry}
			}
		}
		for _, e := range list {
			if mine[e] {
				return side
			}
		}
	}
	return ""
}

func (s *Server) handleTMatchResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, t, g, ok := s.myTMatch(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	runner := canRun(t, g, uid)
	side := s.sideOf(ctx, t, m, uid)
	if !runner && side == "" {
		writeErr(w, http.StatusForbidden, "the players and the organisers enter results")
		return
	}
	if t.Status != "live" {
		writeErr(w, http.StatusBadRequest, "the tournament is not being played")
		return
	}
	if m.Status == "pending" || (m.AEntry == 0 && len(m.Sides["a"]) == 0) || (m.BEntry == 0 && len(m.Sides["b"]) == 0 && m.Outcome != "bye") {
		writeErr(w, http.StatusBadRequest, "who plays is not known yet")
		return
	}
	if m.Status == "done" && !runner {
		writeErr(w, http.StatusBadRequest, "this result counts already: ask an organiser to change it")
		return
	}
	var in struct {
		Winner  string      `json:"winner"`
		Score   store.Score `json:"score"`
		ATotal  *float64    `json:"a_total"`
		BTotal  *float64    `json:"b_total"`
		Outcome string      `json:"outcome"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Outcome == "" {
		in.Outcome = "normal"
	}
	if !oneOf(in.Outcome, []string{"normal", "walkover", "retired", "no_show", "double_walkover"}) {
		writeErr(w, http.StatusBadRequest, "unknown outcome")
		return
	}
	stages, _ := s.store.Stages(ctx, t.ID)
	kind := ""
	for _, st := range stages {
		if st.ID == m.StageID {
			kind = st.Kind
		}
	}
	draws := kind == "round_robin" || kind == "swiss" || kind == "arena" || kind == "team_ties"
	if msg := s.checkTResult(g, kind, m, &in.Winner, &in.Score, in.ATotal, in.BTotal, in.Outcome, draws); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	m.Winner, m.Score, m.ATotal, m.BTotal, m.Outcome, m.Dispute = in.Winner, in.Score, in.ATotal, in.BTotal, in.Outcome, ""
	if in.Outcome == "double_walkover" {
		m.Winner = ""
	}
	now := store.Now()
	m.ReportedBy, m.ReportedAt = uid, &now
	wasDone := m.Status == "done"
	set := settingsOf(t)
	if runner || set.AutoConfirm < 0 {
		m.Status, m.ConfirmedBy, m.ConfirmedAt = "done", uid, &now
	} else {
		m.Status = "reported"
	}
	if err := s.store.SaveTMatch(ctx, m); err != nil {
		s.fail(w, err, "save result")
		return
	}
	s.store.AddTLog(ctx, t.ID, uid, "result", m.Label)
	if m.Status == "done" && !wasDone {
		s.confirmed(ctx, t, g, m, uid)
	} else if m.Status == "reported" {
		// the other side is asked to confirm
		other := map[string]string{"a": "b", "b": "a"}[side]
		var uids []int64
		es, _ := s.store.Entries(ctx, t.ID)
		list := m.Sides[other]
		if len(list) == 0 {
			if other == "a" {
				list = []int64{m.AEntry}
			} else {
				list = []int64{m.BEntry}
			}
		}
		for _, e := range es {
			for _, x := range list {
				if e.ID == x {
					for _, mem := range e.Members {
						uids = append(uids, mem.UserID)
					}
				}
			}
		}
		s.tell(ctx, g, uids, uid, note{kind: "results", title: t.Name + ": is this right?", body: firstNonEmpty(m.Label, "Your match") + " — confirm the score, or say what's wrong.", link: s.tLink(t.ID)})
		s.schedule(ctx, t)
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	t, _ = s.store.GetTournament(ctx, t.ID)
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, uid, false))
}

// checkTResult validates a tournament result the way the group's sport
// counts, and works out the winner; it returns what is wrong, or "".
func (s *Server) checkTResult(g *store.Group, kind string, m *store.TMatch, winner *string, score *store.Score, at, bt *float64, outcome string, draws bool) string {
	switch outcome {
	case "walkover", "no_show", "retired":
		if *winner != "a" && *winner != "b" {
			return "say who goes through"
		}
		if outcome != "retired" {
			*score = store.Score{}
		}
		return ""
	case "double_walkover":
		*score = store.Score{}
		return ""
	}
	switch kind {
	case "americano", "mexicano", "kotc":
		if at == nil || bt == nil || *at < 0 || *bt < 0 {
			return "give both sides' points"
		}
		*winner = map[bool]string{true: "a", false: "b"}[*at > *bt]
		if *at == *bt {
			*winner = "draw"
		}
		return ""
	case "team_ties":
		if m.ParentID == 0 {
			return "a tie is decided by its matches"
		}
	}
	switch g.Scoring {
	case league.ScoringSets:
		if len(score.Sets) == 0 {
			if *winner == "a" || *winner == "b" {
				return "" // only who won (a quick result)
			}
			return "give the score of each set"
		}
		sa, sb := 0, 0
		for _, x := range score.Sets {
			if x[0] < 0 || x[1] < 0 || x[0] > 99 || x[1] > 99 {
				return "a set score is 0 to 99"
			}
			if x[0] > x[1] {
				sa++
			} else if x[1] > x[0] {
				sb++
			}
		}
		*winner = "draw"
		if sa > sb {
			*winner = "a"
		} else if sb > sa {
			*winner = "b"
		}
	case league.ScoringGoals, league.ScoringPoints:
		if score.A == nil || score.B == nil {
			if *winner == "a" || *winner == "b" {
				return ""
			}
			return "give both scores"
		}
		*winner = "draw"
		if *score.A > *score.B {
			*winner = "a"
		} else if *score.B > *score.A {
			*winner = "b"
		}
	default:
		if *winner != "a" && *winner != "b" && *winner != "draw" {
			return "say who won"
		}
		*score = store.Score{}
	}
	if *winner == "draw" && !draws {
		if outcome == "normal" {
			return "a knockout match needs a winner (extra time, penalties, a tie-break)"
		}
	}
	return ""
}

func (s *Server) handleTMatchConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, t, g, ok := s.myTMatch(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	runner := canRun(t, g, uid)
	side := s.sideOf(ctx, t, m, uid)
	var in struct {
		Wrong  bool   `json:"wrong"`
		Reason string `json:"reason"`
	}
	_ = decode(r, &in)
	if m.Status != "reported" {
		writeErr(w, http.StatusBadRequest, "there is no result to confirm")
		return
	}
	if !runner && (side == "" || m.ReportedBy == uid) {
		writeErr(w, http.StatusForbidden, "the other side (or an organiser) confirms a result")
		return
	}
	if in.Wrong {
		m.Dispute = clip(strings.TrimSpace(in.Reason), 300)
		if m.Dispute == "" {
			m.Dispute = "not right"
		}
		_ = s.store.SaveTMatch(ctx, m)
		s.store.AddTLog(ctx, t.ID, uid, "disputed", m.Label+": "+m.Dispute)
		// the organisers sort it out
		ms, _ := s.store.Members(ctx, g.ID)
		var orgs []int64
		for _, x := range ms {
			if x.Role == "organiser" {
				orgs = append(orgs, x.UserID)
			}
		}
		s.tell(ctx, g, orgs, uid, note{kind: "results", title: t.Name + ": a result to sort out", body: firstNonEmpty(m.Label, "A match") + ": " + m.Dispute, link: s.tLink(t.ID)})
	} else {
		now := store.Now()
		m.Status, m.ConfirmedBy, m.ConfirmedAt, m.Dispute = "done", uid, &now, ""
		if err := s.store.SaveTMatch(ctx, m); err != nil {
			s.fail(w, err, "confirm")
			return
		}
		s.store.AddTLog(ctx, t.ID, uid, "confirmed", m.Label)
		s.confirmed(ctx, t, g, m, uid)
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	t, _ = s.store.GetTournament(ctx, t.ID)
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, uid, false))
}

// handleTMatchReopen undoes a result while what followed from it is not played.
func (s *Server) handleTMatchReopen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, t, g, ok := s.myTMatch(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	for _, next := range []int64{m.NextMatch, m.LoserMatch} {
		if next == 0 {
			continue
		}
		n, err := s.store.GetTMatch(ctx, next)
		if err == nil && (n.Status == "done" || n.Status == "reported") && n.Outcome != "bye" {
			writeErr(w, http.StatusBadRequest, "the next match is played already: undo that first")
			return
		}
	}
	// take the entries back out of the next matches
	for _, x := range []struct {
		id   int64
		slot string
	}{{m.NextMatch, m.NextSlot}, {m.LoserMatch, m.LoserSlot}} {
		if x.id == 0 {
			continue
		}
		if n, err := s.store.GetTMatch(ctx, x.id); err == nil {
			if x.slot == "a" {
				n.AEntry = 0
			} else {
				n.BEntry = 0
			}
			n.Status, n.Winner, n.Outcome, n.ConfirmedAt, n.CourtID = "pending", "", "", nil, 0
			_ = s.store.SaveTMatch(ctx, n)
		}
	}
	if m.LeagueMatch != 0 {
		_ = s.store.DeleteMatch(ctx, m.LeagueMatch)
		m.LeagueMatch = 0
	}
	m.Status, m.Winner, m.Outcome, m.Score, m.ATotal, m.BTotal, m.ConfirmedAt, m.ConfirmedBy, m.ReportedAt, m.ReportedBy, m.Dispute =
		"ready", "", "", store.Score{}, nil, nil, nil, 0, nil, 0, ""
	if err := s.store.SaveTMatch(ctx, m); err != nil {
		s.fail(w, err, "reopen")
		return
	}
	stages, _ := s.store.Stages(ctx, t.ID)
	for _, st := range stages {
		if st.ID == m.StageID && st.Status == "done" {
			st.Status = "live"
			_ = s.store.SaveStage(ctx, st)
		}
	}
	if t.Status == "finished" {
		t.Status, t.Winner = "live", ""
		_ = s.store.UpdateTournament(ctx, t)
	}
	s.store.AddTLog(ctx, t.ID, actorID(ctx), "reopened", m.Label)
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, actorID(ctx), false))
}

// handleTMatchCall calls a match to a court (or moves it).
func (s *Server) handleTMatchCall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, t, g, ok := s.myTMatch(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	var in struct {
		CourtID int64  `json:"court_id"`
		At      string `json:"scheduled_at"`
	}
	_ = decode(r, &in)
	courts, _ := s.store.Courts(ctx, t.ID)
	name := ""
	for _, c := range courts {
		if c.ID == in.CourtID {
			name = c.Name
		}
	}
	if in.CourtID != 0 && name == "" {
		writeErr(w, http.StatusBadRequest, "unknown court")
		return
	}
	if in.At != "" && !validTS(in.At) {
		writeErr(w, http.StatusBadRequest, "a time is a date and a time")
		return
	}
	m.CourtID, m.ScheduledAt = in.CourtID, firstNonEmpty(in.At, m.ScheduledAt)
	if in.CourtID != 0 && (m.Status == "ready" || m.Status == "called") {
		now := store.Now()
		m.Status, m.CalledAt = "called", &now
	}
	if in.CourtID == 0 && m.Status == "called" {
		m.Status, m.CalledAt = "ready", nil
	}
	if err := s.store.SaveTMatch(ctx, m); err != nil {
		s.fail(w, err, "call")
		return
	}
	if m.Status == "called" {
		s.callPlayers(ctx, t, g, m, name)
	}
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, actorID(ctx), false))
}

// ---- scored rounds ----

func (s *Server) handleScorecard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	var in struct {
		StageID int64     `json:"stage_id"`
		Round   int       `json:"round"`
		EntryID int64     `json:"entry_id"`
		Gross   *float64  `json:"gross"`
		Holes   []float64 `json:"holes"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	e, err := s.findEntry(ctx, t.ID, in.EntryID)
	if err != nil {
		s.fail(w, err, "entry")
		return
	}
	own := false
	for _, m := range e.Members {
		if m.UserID == uid {
			own = true
		}
	}
	if !own && !canRun(t, g, uid) {
		writeErr(w, http.StatusForbidden, "your own card, or an organiser")
		return
	}
	set := settingsOf(t)
	if in.Round < 1 || (set.Rounds > 0 && in.Round > set.Rounds) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("a round is 1 to %d", max(1, set.Rounds)))
		return
	}
	if len(in.Holes) > 40 {
		writeErr(w, http.StatusBadRequest, "too many holes or games")
		return
	}
	stages, _ := s.store.Stages(ctx, t.ID)
	found := false
	for _, st := range stages {
		if st.ID == in.StageID && st.Kind == "ranking" {
			found = true
		}
	}
	if !found || t.Status != "live" {
		writeErr(w, http.StatusBadRequest, "no scored rounds being played")
		return
	}
	c := &store.Scorecard{StageID: in.StageID, Round: in.Round, EntryID: e.ID, Gross: in.Gross, Holes: in.Holes, EnteredBy: uid}
	if err := s.store.PutScorecard(ctx, c); err != nil {
		s.fail(w, err, "save card")
		return
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, uid, false))
}

// ---- ladders ----

func (s *Server) handleChallenge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	var in struct {
		Defender   int64 `json:"defender"`
		Challenger int64 `json:"challenger"` // organisers may set up a challenge for someone
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if t.Format != "ladder" || t.Status != "live" {
		writeErr(w, http.StatusBadRequest, "challenges are for a ladder that is running")
		return
	}
	es, _ := s.store.Entries(ctx, t.ID)
	var cha, def *store.TEntry
	for _, e := range es {
		if e.Status != "in" {
			continue
		}
		if e.ID == in.Defender {
			def = e
		}
		if in.Challenger != 0 && e.ID == in.Challenger && canRun(t, g, uid) {
			cha = e
		}
		if in.Challenger == 0 {
			for _, m := range e.Members {
				if m.UserID == uid {
					cha = e
				}
			}
		}
	}
	if cha == nil || def == nil || cha.ID == def.ID {
		writeErr(w, http.StatusBadRequest, "pick someone above you on the ladder")
		return
	}
	set := settingsOf(t)
	reach := max(1, set.Reach)
	if def.Position >= cha.Position || cha.Position-def.Position > reach {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("you can challenge up to %d places above you", reach))
		return
	}
	if def.ProtectedUntil != "" && store.LocalNow() < def.ProtectedUntil {
		writeErr(w, http.StatusBadRequest, def.Name+" can't be challenged until "+whenText(def.ProtectedUntil))
		return
	}
	cs, _ := s.store.Challenges(ctx, t.ID)
	for _, c := range cs {
		if (c.Status == "open" || c.Status == "accepted") && (c.Challenger == cha.ID || c.Defender == cha.ID || c.Challenger == def.ID || c.Defender == def.ID) {
			writeErr(w, http.StatusConflict, "one of you has a challenge going already")
			return
		}
	}
	stages, _ := s.store.Stages(ctx, t.ID)
	groups, _ := s.store.TGroups(ctx, t.ID)
	var st *store.TStage
	for _, x := range stages {
		if x.Kind == "ladder" {
			st = x
		}
	}
	if st == nil {
		writeErr(w, http.StatusBadRequest, "make the ladder first")
		return
	}
	var gid int64
	for _, tg := range groups {
		if tg.StageID == st.ID {
			gid = tg.ID
		}
	}
	ms, _ := s.store.TMatches(ctx, t.ID)
	round := 1
	for _, m := range ms {
		if m.StageID == st.ID && m.Round >= round {
			round = m.Round + 1
		}
	}
	now := time.Now()
	c := &store.Challenge{TournamentID: t.ID, Challenger: cha.ID, Defender: def.ID, Status: "open",
		RespondBy: now.AddDate(0, 0, max(1, set.RespondDays)).Format(tsLayout), PlayBy: now.AddDate(0, 0, max(1, set.RespondDays+set.PlayDays)).Format(tsLayout)}
	m := &store.TMatch{TournamentID: t.ID, StageID: st.ID, GroupID: gid, Round: round, Label: fmt.Sprintf("%s challenges %s", cha.Name, def.Name),
		AEntry: cha.ID, BEntry: def.ID, Status: "ready", Deadline: c.PlayBy}
	if err := s.store.SaveTMatch(ctx, m); err != nil {
		s.fail(w, err, "challenge")
		return
	}
	c.MatchID = m.ID
	if err := s.store.SaveChallenge(ctx, c); err != nil {
		s.fail(w, err, "challenge")
		return
	}
	var uids []int64
	for _, mm := range def.Members {
		uids = append(uids, mm.UserID)
	}
	s.tell(ctx, g, uids, uid, note{kind: "plans", title: t.Name + ": " + cha.Name + " challenges you", body: "Accept by " + whenText(c.RespondBy) + ", or it counts as a loss.", link: s.tLink(t.ID), button: "Answer"})
	s.store.AddTLog(ctx, t.ID, uid, "challenge", m.Label)
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, uid, false))
}

func (s *Server) handleChallengeAnswer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	var in struct {
		Accept bool `json:"accept"`
	}
	_ = decode(r, &in)
	cs, _ := s.store.Challenges(ctx, t.ID)
	var c *store.Challenge
	for i := range cs {
		if cs[i].ID == idParam(r, "cid") {
			c = &cs[i]
		}
	}
	if c == nil || c.Status != "open" {
		writeErr(w, http.StatusNotFound, "no open challenge")
		return
	}
	def, err := s.findEntry(ctx, t.ID, c.Defender)
	if err != nil {
		s.fail(w, err, "entry")
		return
	}
	mine := canRun(t, g, uid)
	for _, m := range def.Members {
		if m.UserID == uid {
			mine = true
		}
	}
	if !mine {
		writeErr(w, http.StatusForbidden, "the one challenged answers")
		return
	}
	m, err := s.store.GetTMatch(ctx, c.MatchID)
	if err != nil {
		s.fail(w, err, "match")
		return
	}
	if in.Accept {
		c.Status = "accepted"
		_ = s.store.SaveChallenge(ctx, c)
		s.store.AddTLog(ctx, t.ID, uid, "accepted", m.Label)
	} else {
		// declining counts as losing it
		now := store.Now()
		m.Status, m.Outcome, m.Winner, m.ConfirmedAt, m.ConfirmedBy = "done", "walkover", "a", &now, uid
		_ = s.store.SaveTMatch(ctx, m)
		s.store.AddTLog(ctx, t.ID, uid, "declined", m.Label)
		s.confirmed(ctx, t, g, m, uid)
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.tDetail(ctx, t, g, uid, false))
}

// ---- box leagues: the next round ----

// handleNextSeason starts the next round of a box league: the best of
// each box go up a box, the last go down.
func (s *Server) handleNextSeason(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, g, ok := s.myTournament(w, r)
	if !ok || !s.needRunner(w, t, g, actorID(ctx)) {
		return
	}
	if t.Format != "box_league" {
		writeErr(w, http.StatusBadRequest, "only a box league has rounds of boxes")
		return
	}
	stages, _ := s.store.Stages(ctx, t.ID)
	if len(stages) == 0 {
		writeErr(w, http.StatusBadRequest, "make the draw first")
		return
	}
	st := stages[0]
	ms, _ := s.store.TMatches(ctx, t.ID)
	tables := s.stageStandings(ctx, t, st, ms, g)
	groups, _ := s.store.TGroups(ctx, t.ID)
	var boxes [][]int64
	for _, tg := range groups {
		if tg.StageID != st.ID {
			continue
		}
		var ids []int64
		for _, r := range tables[tg.ID] {
			ids = append(ids, r.ID)
		}
		boxes = append(boxes, ids)
	}
	set := settingsOf(t)
	up := max(1, set.Promote)
	// swap the bottom of each box with the top of the next one
	for i := 0; i+1 < len(boxes); i++ {
		a, b := boxes[i], boxes[i+1]
		k := min(up, min(len(a), len(b)))
		down := append([]int64{}, a[len(a)-k:]...)
		upper := append([]int64{}, b[:k]...)
		boxes[i] = append(append([]int64{}, a[:len(a)-k]...), upper...)
		boxes[i+1] = append(down, b[k:]...)
	}
	// finish this round, start a new tournament with the boxes as they are now
	if t.Status == "live" {
		ranks := s.finalRanks(ctx, t, st, ms)
		st.Status = "done"
		_ = s.store.SaveStage(ctx, st)
		for id, rk := range ranks {
			_ = s.store.SetFinalRank(ctx, st.ID, id, rk)
		}
		_ = s.finishTournament(ctx, t, g, st, ranks)
	}
	es, _ := s.store.Entries(ctx, t.ID)
	byID := map[int64]*store.TEntry{}
	for _, e := range es {
		byID[e.ID] = e
	}
	nt := &store.Tournament{GroupID: g.ID, Name: nextName(t.Name), Format: t.Format, EntryKind: t.EntryKind, Status: "live", Rated: t.Rated, Currency: t.Currency,
		VenueName: t.VenueName, Settings: t.Settings}
	if err := s.store.CreateTournament(ctx, actorID(ctx), nt); err != nil {
		s.fail(w, err, "next round")
		return
	}
	newID := map[int64]int64{}
	for _, box := range boxes {
		for _, old := range box {
			e := byID[old]
			if e == nil {
				continue
			}
			ne := &store.TEntry{TournamentID: nt.ID, Name: e.Name, Members: e.Members, Rating: e.Rating, Handicap: e.Handicap, CreatedBy: actorID(ctx)}
			if err := s.store.SaveEntry(ctx, ne); err == nil {
				newID[old] = ne.ID
			}
		}
	}
	nst := &store.TStage{TournamentID: nt.ID, Seq: 1, Kind: "round_robin", Name: "Boxes", Status: "live"}
	if err := s.store.SaveStage(ctx, nst); err != nil {
		s.fail(w, err, "next round")
		return
	}
	var gs [][]int64
	var names []string
	for i, box := range boxes {
		var ids []int64
		for _, old := range box {
			if id := newID[old]; id != 0 {
				ids = append(ids, id)
			}
		}
		gs = append(gs, ids)
		names = append(names, fmt.Sprintf("Box %d", i+1))
	}
	if err := s.savePlan(ctx, nt, nst, tour.RoundRobin(gs, names, set.Double)); err != nil {
		s.fail(w, err, "next round")
		return
	}
	s.systemMessage(ctx, g, 0, "tournament", nt.Name+" has started: new boxes, some up, some down", map[string]any{"tournament_id": nt.ID})
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": nt.ID, "group_id": g.ID})
	writeJSON(w, http.StatusCreated, s.tDetail(ctx, nt, g, actorID(ctx), false))
}

// nextName is "Box league, round 2" after "Box league" (or round n+1).
func nextName(n string) string {
	var base string
	var k int
	if i := strings.LastIndex(n, ", round "); i > 0 {
		if _, err := fmt.Sscan(n[i+8:], &k); err == nil {
			base = n[:i]
		}
	}
	if base == "" {
		base, k = n, 1
	}
	return fmt.Sprintf("%s, round %d", base, k+1)
}
