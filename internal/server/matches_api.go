package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- results ----
//
// Anyone in a group enters a result; it counts at once. Someone else who
// played (or an organiser) confirms it, or says it is not right — a
// disputed result leaves the table until it is fixed. Results nobody
// questioned are confirmed by themselves after two days.

// matchView is a result as the page shows it.
type matchView struct {
	*store.Match
	Text       string            `json:"text"` // "6–4 3–6 7–5"
	Names      map[string]string `json:"names"`
	State      string            `json:"state"` // confirmed | waiting | disputed
	CanConfirm bool              `json:"can_confirm"`
	CanEdit    bool              `json:"can_edit"`
	Changes    map[int64]float64 `json:"changes,omitempty"`
}

func (s *Server) viewMatch(r league.Rules, m *store.Match, names map[int64]string, uid int64, g *store.Group) matchView {
	v := matchView{Match: m, Text: league.ScoreText(r, m), Names: map[string]string{}}
	mine := false
	for _, p := range m.Players {
		v.Names[itoa(p.UserID)] = firstNonEmpty(names[p.UserID], "Former player")
		if p.UserID == uid {
			mine = true
		}
	}
	switch {
	case m.DisputedBy != 0:
		v.State = "disputed"
	case m.ConfirmedAt != nil:
		v.State = "confirmed"
	default:
		v.State = "waiting"
	}
	v.CanEdit = m.UserID == uid || mine || organiser(g)
	v.CanConfirm = v.State != "confirmed" && m.UserID != uid && (mine || organiser(g))
	return v
}

func (s *Server) names(ctx context.Context, gid int64) map[int64]string {
	out := map[int64]string{}
	ms, _ := s.store.Members(ctx, gid)
	for _, m := range ms {
		out[m.UserID] = m.Display()
	}
	return out
}

func (s *Server) handleListMatches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	f := store.MatchFilter{GroupID: int64(queryInt(r, "group")), GameID: int64(queryInt(r, "game")), UserID: int64(queryInt(r, "player")),
		From: r.URL.Query().Get("from"), To: r.URL.Query().Get("to"), Limit: queryInt(r, "limit")}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	ms, err := s.store.ListMatches(ctx, uid, f)
	if err != nil {
		s.fail(w, err, "results")
		return
	}
	groups := map[int64]*store.Group{}
	names := map[int64]map[int64]string{}
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		g := groups[m.GroupID]
		if g == nil {
			g, _ = s.store.GetGroup(ctx, uid, m.GroupID)
			if g == nil {
				continue
			}
			groups[m.GroupID], names[m.GroupID] = g, s.names(ctx, m.GroupID)
		}
		v := s.viewMatch(league.RulesOf(g), m, names[m.GroupID], uid, g)
		out = append(out, map[string]any{"match": v, "group_name": g.Name, "emoji": league.SportByKey(g.Sport).Emoji, "group_color": g.Color})
	}
	writeJSON(w, http.StatusOK, out)
}

// matchInput is a result as sent.
type matchInput struct {
	GameID   int64               `json:"game_id"`
	PlayedAt string              `json:"played_at"`
	Players  []store.MatchPlayer `json:"players"`
	Score    store.Score         `json:"score"`
	Winner   string              `json:"winner"`
	Note     string              `json:"note"`
}

// build checks a result against the group and fills a match.
func (s *Server) buildMatch(ctx context.Context, g *store.Group, in matchInput, m *store.Match) string {
	m.GroupID = g.ID
	if in.GameID != 0 {
		gm, err := s.store.GetGame(ctx, actorID(ctx), in.GameID)
		if err != nil || gm.GroupID != g.ID {
			return "that game is not in this group"
		}
		m.GameID = gm.ID
		if in.PlayedAt == "" {
			in.PlayedAt = gm.StartsAt
		}
	}
	if in.PlayedAt == "" {
		in.PlayedAt = store.LocalNow()
	}
	if len(in.PlayedAt) == 10 {
		in.PlayedAt += "T12:00"
	}
	if !validTS(in.PlayedAt) {
		return "when was it played? a date and a time"
	}
	if in.PlayedAt > time.Now().Add(24*time.Hour).Format(tsLayout) {
		return "a result is for a match that was played"
	}
	members := map[int64]bool{}
	ids, _ := s.store.MemberIDs(ctx, g.ID)
	for _, id := range ids {
		members[id] = true
	}
	for i, p := range in.Players {
		if !members[p.UserID] {
			return "every player must be in the group"
		}
		clean := map[string]int{}
		for k, v := range p.Stats {
			if !league.ValidStat(k) || v < 0 || v > 999 {
				return "a stat is out of range"
			}
			if v > 0 {
				clean[k] = v
			}
		}
		in.Players[i].Stats = clean
	}
	m.PlayedAt, m.Players, m.Score, m.Winner, m.Note = in.PlayedAt, in.Players, in.Score, in.Winner, clip(strings.TrimSpace(in.Note), 500)
	if err := league.Check(league.RulesOf(g), m); err != nil {
		return err.Error()
	}
	return ""
}

func (s *Server) handleCreateMatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	u := userFrom(ctx)
	var in matchInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	m := &store.Match{}
	if msg := s.buildMatch(ctx, g, in, m); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.SaveMatch(ctx, u.ID, m); err != nil {
		s.fail(w, err, "save result")
		return
	}
	m, _ = s.store.GetMatch(ctx, u.ID, m.ID)
	s.afterResult(ctx, g, m, u, true)
	writeJSON(w, http.StatusCreated, s.viewMatch(league.RulesOf(g), m, s.names(ctx, g.ID), u.ID, g))
}

// resultLine is a result in words: "Ana & Marko beat Ivan & Edin 6–4 3–6 7–5".
func (s *Server) resultLine(g *store.Group, m *store.Match, names map[int64]string) string {
	r := league.RulesOf(g)
	nameList := func(ids []int64) string {
		var parts []string
		for _, id := range ids {
			parts = append(parts, firstNonEmpty(names[id], "?"))
		}
		if len(parts) > 2 {
			return parts[0] + " & co."
		}
		return strings.Join(parts, " & ")
	}
	if r.Scoring == league.ScoringRanking {
		var parts []string
		for place := 1; place <= len(m.Players) && len(parts) < 3; place++ {
			for _, p := range m.Players {
				if p.Place == place && p.Score != nil {
					parts = append(parts, fmt.Sprintf("%d. %s %s", place, firstNonEmpty(names[p.UserID], "?"), trimNum(*p.Score)))
				}
			}
		}
		return strings.Join(parts, " · ")
	}
	a, b := league.Side(m, "a"), league.Side(m, "b")
	if g.Format == league.FormatTeams {
		ta, tb := "Team A", "Team B"
		if len(a) > 0 {
			ta = "Team " + firstName(names[a[0]])
		}
		if len(b) > 0 {
			tb = "Team " + firstName(names[b[0]])
		}
		switch m.Winner {
		case "a":
			return ta + " beat " + tb + " " + league.ScoreText(r, m)
		case "b":
			return tb + " beat " + ta + " " + reverseScore(r, m)
		}
		return ta + " and " + tb + " drew " + league.ScoreText(r, m)
	}
	switch m.Winner {
	case "a":
		return strings.TrimSpace(nameList(a) + " beat " + nameList(b) + " " + league.ScoreText(r, m))
	case "b":
		return strings.TrimSpace(nameList(b) + " beat " + nameList(a) + " " + reverseScore(r, m))
	}
	return strings.TrimSpace(nameList(a) + " and " + nameList(b) + " drew " + league.ScoreText(r, m))
}

// reverseScore is the score from side b's view.
func reverseScore(r league.Rules, m *store.Match) string {
	cp := *m
	cp.Score = store.Score{A: m.Score.B, B: m.Score.A}
	for _, st := range m.Score.Sets {
		cp.Score.Sets = append(cp.Score.Sets, [2]int{st[1], st[0]})
	}
	return league.ScoreText(r, &cp)
}

func trimNum(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.1f", f)
}

// afterResult tells the group, asks the other side to confirm, and records
// the match in each player's own apps.
func (s *Server) afterResult(ctx context.Context, g *store.Group, m *store.Match, by *store.User, isNew bool) {
	names := s.names(ctx, g.ID)
	line := s.resultLine(g, m, names)
	what := "result"
	if !isNew {
		what = "result_changed"
	}
	s.systemMessage(ctx, g, m.GameID, what, line, map[string]any{"match_id": m.ID, "game_id": m.GameID})
	s.toGroup(ctx, g.ID, "match", map[string]any{"id": m.ID, "group_id": g.ID, "game_id": m.GameID})
	var others []int64
	for _, p := range m.Players {
		if p.UserID != by.ID {
			others = append(others, p.UserID)
		}
	}
	link := s.groupLink(g.ID) + "?tab=results"
	if m.GameID != 0 {
		link = s.gameLink(m.GameID)
	}
	s.tell(ctx, g, others, by.ID, note{kind: "results", title: g.Name + ": " + line, body: firstNonEmpty(by.Name, "Someone") + " entered the result. Is it right? Confirm it, or say what's wrong.",
		link: link, button: "Check the result"})
	s.publishGroup(ctx, g, "league.result", fmt.Sprintf("match:%d:%s", m.ID, m.UpdatedAt), map[string]any{"match_id": m.ID, "group_id": g.ID, "group": g.Name,
		"sport": g.Sport, "text": line, "played_at": m.PlayedAt, "url": link})
	go s.recordMatch(g, m.ID)
}

func (s *Server) myMatch(w http.ResponseWriter, r *http.Request) (*store.Match, *store.Group, bool) {
	ctx := r.Context()
	uid := actorID(ctx)
	m, err := s.store.GetMatch(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "result")
		return nil, nil, false
	}
	g, err := s.store.GetGroup(ctx, uid, m.GroupID)
	if err != nil {
		s.fail(w, err, "group")
		return nil, nil, false
	}
	return m, g, true
}

func (s *Server) handleUpdateMatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, g, ok := s.myMatch(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	if !s.viewMatch(league.RulesOf(g), m, nil, u.ID, g).CanEdit {
		writeErr(w, http.StatusForbidden, "only the players, whoever entered it and the organisers can change a result")
		return
	}
	var in matchInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.GameID == 0 {
		in.GameID = m.GameID
	}
	if msg := s.buildMatch(ctx, g, in, m); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.SaveMatch(ctx, u.ID, m); err != nil {
		s.fail(w, err, "save result")
		return
	}
	m, _ = s.store.GetMatch(ctx, u.ID, m.ID)
	s.afterResult(ctx, g, m, u, false)
	writeJSON(w, http.StatusOK, s.viewMatch(league.RulesOf(g), m, s.names(ctx, g.ID), u.ID, g))
}

func (s *Server) handleDeleteMatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, g, ok := s.myMatch(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	if m.UserID != uid && !organiser(g) {
		writeErr(w, http.StatusForbidden, "only whoever entered it and the organisers can delete a result")
		return
	}
	if err := s.store.DeleteMatch(ctx, m.ID); err != nil {
		s.fail(w, err, "delete result")
		return
	}
	s.toGroup(ctx, g.ID, "match", map[string]any{"id": m.ID, "group_id": g.ID, "game_id": m.GameID, "deleted": true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleConfirmMatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, g, ok := s.myMatch(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	var in struct {
		Wrong  bool   `json:"wrong"`
		Reason string `json:"reason"`
	}
	_ = decode(r, &in)
	v := s.viewMatch(league.RulesOf(g), m, nil, u.ID, g)
	if !v.CanEdit || (!in.Wrong && !v.CanConfirm) {
		writeErr(w, http.StatusForbidden, "someone else who played (or an organiser) confirms a result")
		return
	}
	var err error
	if in.Wrong {
		err = s.store.DisputeMatch(ctx, m.ID, u.ID, clip(strings.TrimSpace(in.Reason), 300))
	} else {
		err = s.store.ConfirmMatch(ctx, m.ID, u.ID)
	}
	if err != nil {
		s.fail(w, err, "confirm")
		return
	}
	m, _ = s.store.GetMatch(ctx, u.ID, m.ID)
	names := s.names(ctx, g.ID)
	if in.Wrong {
		text := names[u.ID] + " says this result is not right"
		if in.Reason != "" {
			text += ": " + clip(strings.TrimSpace(in.Reason), 300)
		}
		s.systemMessage(ctx, g, m.GameID, "disputed", text, map[string]any{"match_id": m.ID})
		if m.UserID != 0 && m.UserID != u.ID {
			s.tell(ctx, g, []int64{m.UserID}, u.ID, note{kind: "results", title: g.Name + ": a result to fix", body: text + ".", link: s.groupLink(g.ID) + "?tab=results"})
		}
	}
	s.toGroup(ctx, g.ID, "match", map[string]any{"id": m.ID, "group_id": g.ID, "game_id": m.GameID})
	writeJSON(w, http.StatusOK, s.viewMatch(league.RulesOf(g), m, names, u.ID, g))
}

// ---- the table ----

// periodStart is where a table starts: all | year | season (this half-year) | month | 90d.
func periodStart(p string, now time.Time) string {
	switch p {
	case "year":
		return now.Format("2006") + "-01-01T00:00"
	case "month":
		return now.Format("2006-01") + "-01T00:00"
	case "season":
		if now.Month() >= 7 {
			return now.Format("2006") + "-07-01T00:00"
		}
		return now.Format("2006") + "-01-01T00:00"
	case "90d":
		return now.AddDate(0, 0, -90).Format(tsLayout)
	}
	return ""
}

// table works out a group's table for a period.
func (s *Server) table(ctx context.Context, g *store.Group, period string) (*league.Table, []*store.Match) {
	return s.tableBetween(ctx, g, periodStart(period, time.Now()), "")
}

// tableBetween works out a group's table from the matches between two moments.
func (s *Server) tableBetween(ctx context.Context, g *store.Group, from, to string) (*league.Table, []*store.Match) {
	ms, _ := s.store.ListMatches(ctx, 0, store.MatchFilter{GroupID: g.ID, From: from, To: to, Oldest: true})
	ids, _ := s.store.MemberIDs(ctx, g.ID)
	t := league.Compute(league.RulesOf(g), ms, ids)
	mvps, _ := s.store.GroupMVPs(ctx, g.ID, from)
	stats := map[int64]map[string]int{}
	for _, m := range ms {
		for _, p := range m.Players {
			for k, v := range p.Stats {
				if stats[p.UserID] == nil {
					stats[p.UserID] = map[string]int{}
				}
				stats[p.UserID][k] += v
			}
		}
	}
	for _, row := range t.Rows {
		row.MVP = mvps[row.UserID]
		row.Stats = stats[row.UserID]
	}
	return t, ms
}

// ratings are everyone's current ratings in a group: over all time, or —
// when the group starts ratings fresh each season — in the season that is on.
func (s *Server) ratings(ctx context.Context, g *store.Group) map[int64]float64 {
	if g.RatingScope == "season" {
		if x := s.store.CurrentSeason(ctx, g.ID, store.LocalNow()[:10]); x != nil {
			t, _ := s.tableBetween(ctx, g, x.StartsOn+"T00:00", seasonEnd(x))
			return t.Ratings()
		}
	}
	t, _ := s.table(ctx, g, "all")
	return t.Ratings()
}

func (s *Server) handleTable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	if sid := int64(queryInt(r, "season")); sid > 0 {
		x, err := s.store.GetSeason(ctx, sid)
		if err != nil || x.GroupID != g.ID {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, s.seasonTable(ctx, g, x))
		return
	}
	period := r.URL.Query().Get("period")
	if !oneOf(period, []string{"all", "year", "season", "month", "90d"}) {
		period = "all"
	}
	t, ms := s.table(ctx, g, period)
	writeJSON(w, http.StatusOK, map[string]any{"period": period, "rows": s.tableRows(ctx, g, t), "matches": len(ms), "rules": league.RulesOf(g), "group": viewGroup(g)})
}

// handlePlayer is one person's page in a group: their line in the table,
// how they do against everyone, their last matches and their rating over time.
func (s *Server) handlePlayer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	pid := idParam(r, "uid")
	names := s.names(ctx, g.ID)
	ms, _ := s.store.ListMatches(ctx, 0, store.MatchFilter{GroupID: g.ID, Oldest: true})
	played := false
	for _, m := range ms {
		for _, p := range m.Players {
			if p.UserID == pid {
				played = true
			}
		}
	}
	if _, member := names[pid]; !member && !played {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	rules := league.RulesOf(g)
	t := league.Compute(rules, ms, []int64{pid})
	var mine *league.Row
	for _, x := range t.Rows {
		if x.UserID == pid {
			mine = x
		}
	}
	// the rating after each match, oldest first
	type point struct {
		At     string  `json:"at"`
		Rating float64 `json:"rating"`
	}
	hist := []point{}
	cur := league.StartRating
	recent := []matchView{}
	for _, m := range ms {
		ch, ok := t.Changes[m.ID][pid]
		if !ok {
			continue
		}
		cur += ch
		hist = append(hist, point{At: m.PlayedAt, Rating: float64(int(cur + 0.5))})
	}
	for i := len(ms) - 1; i >= 0 && len(recent) < 15; i-- {
		for _, p := range ms[i].Players {
			if p.UserID == pid {
				v := s.viewMatch(rules, ms[i], names, actorID(ctx), g)
				v.Changes = t.Changes[ms[i].ID]
				recent = append(recent, v)
				break
			}
		}
	}
	vs := league.HeadToHead(pid, ms)
	type versus struct {
		*league.Versus
		Name string `json:"name"`
	}
	out := make([]versus, 0, len(vs))
	for _, v := range vs {
		out = append(out, versus{Versus: v, Name: firstNonEmpty(names[v.UserID], "Former player")})
	}
	mvps, _ := s.store.GroupMVPs(ctx, g.ID, "")
	if mine != nil {
		mine.MVP = mvps[pid]
	}
	titles, _ := s.store.TitlesOf(ctx, g.ID, pid)
	type title struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		At   string `json:"at"`
	}
	tl := []title{}
	for _, t := range titles {
		tl = append(tl, title{t.ID, t.Name, firstNonEmpty(t.StartsAt, t.UpdatedAt)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": pid, "name": firstNonEmpty(names[pid], "Former player"), "row": mine, "history": hist,
		"recent": recent, "versus": out, "group": viewGroup(g), "titles": tl})
}
