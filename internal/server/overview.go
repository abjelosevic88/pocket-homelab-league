package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- overviews ----
//
// The home page, and what the rest of the box reads through league:read:
// Pocket Day (today's game, yesterday's results in the journal), Search,
// Pocket AI (the briefing) and anything showing "your next game".

// handleToday is what needs the person now: the next games, the ones they
// have not answered, the votes open, the results to confirm, the unread chats.
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	games, _ := s.store.ListGames(ctx, uid, store.GameFilter{Upcoming: true, Limit: 30})
	items := s.gameItems(ctx, uid, games)
	var next, unanswered, voting []gameItem
	for _, it := range items {
		switch {
		case it.Status == store.GameVoting:
			voting = append(voting, it)
		case it.MyAnswer == "":
			unanswered = append(unanswered, it)
			next = append(next, it)
		default:
			next = append(next, it)
		}
	}
	if len(next) > 8 {
		next = next[:8]
	}
	// results waiting for me to confirm
	recent, _ := s.store.ListMatches(ctx, uid, store.MatchFilter{UserID: uid, From: time.Now().AddDate(0, 0, -14).Format(tsLayout)})
	confirm := []map[string]any{}
	for _, m := range recent {
		if m.ConfirmedAt != nil || m.DisputedBy != 0 || m.UserID == uid {
			continue
		}
		g, err := s.store.GetGroup(ctx, uid, m.GroupID)
		if err != nil {
			continue
		}
		confirm = append(confirm, map[string]any{"match": s.viewMatch(league.RulesOf(g), m, s.names(ctx, g.ID), uid, g), "group_name": g.Name, "emoji": league.SportByKey(g.Sport).Emoji})
	}
	groups, _ := s.store.ListGroups(ctx, uid, false)
	unread := 0
	for _, g := range groups {
		unread += g.Unread
	}
	// my last results
	mine, _ := s.store.ListMatches(ctx, uid, store.MatchFilter{UserID: uid, Limit: 5})
	last := []map[string]any{}
	for _, m := range mine {
		g, err := s.store.GetGroup(ctx, uid, m.GroupID)
		if err != nil {
			continue
		}
		v := s.viewMatch(league.RulesOf(g), m, s.names(ctx, g.ID), uid, g)
		last = append(last, map[string]any{"match": v, "group_name": g.Name, "emoji": league.SportByKey(g.Sport).Emoji, "line": s.resultLine(g, m, s.names(ctx, g.ID)),
			"won": myOutcome(m, uid)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"next": nz(next), "unanswered": nz(unanswered), "voting": nz(voting), "confirm": confirm,
		"unread": unread, "groups": len(groups), "recent": last})
}

func nz(v []gameItem) []gameItem {
	if v == nil {
		return []gameItem{}
	}
	return v
}

// myOutcome is W, D or L for a person in a match ("" when not in it).
func myOutcome(m *store.Match, uid int64) string {
	for _, p := range m.Players {
		if p.UserID != uid {
			continue
		}
		if p.Side == "" {
			if p.Place == 1 {
				return "W"
			}
			return "L"
		}
		switch m.Winner {
		case "draw":
			return "D"
		case p.Side:
			return "W"
		}
		return "L"
	}
	return ""
}

// handleSummary is a short overview: the next game, counts, the latest results.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	games, _ := s.store.ListGames(ctx, uid, store.GameFilter{Upcoming: true, Limit: 10})
	items := s.gameItems(ctx, uid, games)
	var next *gameItem
	for i := range items {
		if items[i].Status == store.GamePlanned {
			next = &items[i]
			break
		}
	}
	groups, _ := s.store.ListGroups(ctx, uid, false)
	month := time.Now().Format("2006-01") + "-01T00:00"
	played, _ := s.store.ListMatches(ctx, uid, store.MatchFilter{UserID: uid, From: month})
	won := 0
	for _, m := range played {
		if myOutcome(m, uid) == "W" {
			won++
		}
	}
	unread := 0
	type gsum struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Sport  string `json:"sport"`
		Emoji  string `json:"emoji"`
		Unread int    `json:"unread"`
		URL    string `json:"url"`
	}
	gl := []gsum{}
	for _, g := range groups {
		unread += g.Unread
		gl = append(gl, gsum{g.ID, g.Name, league.DisplayName(g.Sport, g.SportName), league.SportByKey(g.Sport).Emoji, g.Unread, s.groupLink(g.ID)})
	}
	out := map[string]any{"groups": gl, "upcoming": len(items), "unread": unread, "this_month": map[string]int{"played": len(played), "won": won}}
	if next != nil {
		out["next_game"] = map[string]any{"id": next.ID, "group": next.GroupName, "emoji": next.Emoji, "starts_at": next.StartsAt, "when": whenText(next.StartsAt),
			"venue": next.VenueName, "in": next.In, "min_players": next.MinPlayers, "my_answer": next.MyAnswer, "url": s.gameLink(next.ID)}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDay is one day for Pocket Day's journal: the games and the results.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	day := chiParam(r, "date")
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the day is YYYY-MM-DD")
		return
	}
	from, to := day+"T00:00", t.AddDate(0, 0, 1).Format("2006-01-02")+"T00:00"
	games, _ := s.store.ListGames(ctx, uid, store.GameFilter{From: from, To: to})
	type dayGame struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Group    string `json:"group"`
		Emoji    string `json:"emoji"`
		Time     string `json:"time"`
		Venue    string `json:"venue"`
		Status   string `json:"status"`
		MyAnswer string `json:"my_answer"`
		In       int    `json:"in"`
		URL      string `json:"url"`
	}
	gl := []dayGame{}
	for _, it := range s.gameItems(ctx, uid, games) {
		gl = append(gl, dayGame{it.ID, firstNonEmpty(it.Title, it.Sport+" · "+it.GroupName), it.GroupName, it.Emoji, it.StartsAt[11:], it.VenueName, it.Status, it.MyAnswer, it.In, s.gameLink(it.ID)})
	}
	ms, _ := s.store.ListMatches(ctx, uid, store.MatchFilter{UserID: uid, From: from, To: to, Oldest: true})
	type dayMatch struct {
		ID      int64  `json:"id"`
		Group   string `json:"group"`
		Emoji   string `json:"emoji"`
		Text    string `json:"text"`
		Outcome string `json:"outcome"` // W | D | L
		URL     string `json:"url"`
	}
	ml := []dayMatch{}
	for _, m := range ms {
		g, err := s.store.GetGroup(ctx, uid, m.GroupID)
		if err != nil {
			continue
		}
		link := s.groupLink(g.ID) + "?tab=results"
		if m.GameID != 0 {
			link = s.gameLink(m.GameID)
		}
		ml = append(ml, dayMatch{m.ID, g.Name, league.SportByKey(g.Sport).Emoji, s.resultLine(g, m, s.names(ctx, g.ID)), myOutcome(m, uid), link})
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": day, "games": gl, "matches": ml})
}

// handleSearch finds groups, games, players and chat lines.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	type hit struct {
		Kind  string `json:"kind"` // group | game | player | message
		ID    int64  `json:"id"`
		Title string `json:"title"`
		Sub   string `json:"subtitle"`
		Emoji string `json:"emoji"`
		URL   string `json:"url"`
		Path  string `json:"path"`
		At    string `json:"at,omitempty"`
	}
	out := []hit{}
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, map[string]any{"results": out})
		return
	}
	lq := strings.ToLower(q)
	groups, _ := s.store.ListGroups(ctx, uid, true)
	seenPlayer := map[int64]bool{}
	for _, g := range groups {
		sp := league.SportByKey(g.Sport)
		if strings.Contains(strings.ToLower(g.Name+" "+league.DisplayName(g.Sport, g.SportName)+" "+g.VenueName), lq) {
			out = append(out, hit{"group", g.ID, g.Name, league.DisplayName(g.Sport, g.SportName) + " · " + itoa(int64(g.Members)) + " people", sp.Emoji, s.groupLink(g.ID), "/groups/" + itoa(g.ID), ""})
		}
		ms, _ := s.store.Members(ctx, g.ID)
		for _, m := range ms {
			if seenPlayer[m.UserID] || !strings.Contains(strings.ToLower(m.Display()+" "+m.Name), lq) {
				continue
			}
			seenPlayer[m.UserID] = true
			out = append(out, hit{"player", m.UserID, m.Display(), "plays in " + g.Name, sp.Emoji, s.groupLink(g.ID) + "/players/" + itoa(m.UserID), "/groups/" + itoa(g.ID) + "/players/" + itoa(m.UserID), ""})
		}
	}
	games, _ := s.store.ListGames(ctx, uid, store.GameFilter{From: time.Now().AddDate(-1, 0, 0).Format(tsLayout)})
	for _, it := range s.gameItems(ctx, uid, games) {
		if strings.Contains(strings.ToLower(it.Title+" "+it.VenueName+" "+it.Notes), lq) {
			out = append(out, hit{"game", it.ID, firstNonEmpty(it.Title, it.Sport+" · "+it.GroupName), whenText(it.StartsAt) + " · " + it.VenueName, it.Emoji, s.gameLink(it.ID), "/games/" + itoa(it.ID), it.StartsAt})
		}
	}
	msgs, _ := s.store.SearchMessages(ctx, uid, q, limit)
	for _, m := range msgs {
		g, err := s.store.GetGroup(ctx, uid, m.GroupID)
		if err != nil {
			continue
		}
		link, path := s.groupLink(g.ID)+"?tab=chat", "/groups/"+itoa(g.ID)+"?tab=chat"
		if m.GameID != 0 {
			link, path = s.gameLink(m.GameID), "/games/"+itoa(m.GameID)
		}
		out = append(out, hit{"message", m.ID, clip(m.Body, 120), s.nameIn(ctx, g.ID, m.UserID) + " in " + g.Name, league.SportByKey(g.Sport).Emoji, link, path, m.CreatedAt})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

// handleLookup says whether someone (by email) plays with the person, and
// where: for a contact's page in Pocket Contacts.
func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	out := map[string]any{"found": false, "groups": []any{}}
	other, err := s.store.GetUserByEmail(ctx, email)
	if email == "" || err != nil || other.ID == uid || !s.store.SharesGroup(ctx, uid, other.ID) {
		writeJSON(w, http.StatusOK, out)
		return
	}
	mine, _ := s.store.GroupIDsOf(ctx, uid)
	theirs, _ := s.store.GroupIDsOf(ctx, other.ID)
	in := map[int64]bool{}
	for _, id := range theirs {
		in[id] = true
	}
	gl := []map[string]any{}
	for _, gid := range mine {
		if !in[gid] {
			continue
		}
		g, err := s.store.GetGroup(ctx, uid, gid)
		if err != nil {
			continue
		}
		ms, _ := s.store.ListMatches(ctx, uid, store.MatchFilter{GroupID: g.ID, Oldest: true})
		var rec map[string]any
		for _, v := range league.HeadToHead(uid, ms) {
			if v.UserID == other.ID {
				rec = map[string]any{"played": v.Played, "won": v.Won, "drawn": v.Drawn, "lost": v.Lost, "together": v.Together, "last_played_at": v.LastPlayedAt}
			}
		}
		gl = append(gl, map[string]any{"id": g.ID, "name": g.Name, "sport": league.DisplayName(g.Sport, g.SportName), "emoji": league.SportByKey(g.Sport).Emoji,
			"url": s.groupLink(g.ID) + "/players/" + itoa(other.ID), "record": rec})
	}
	out["found"], out["name"], out["groups"] = true, other.Name, gl
	writeJSON(w, http.StatusOK, out)
}
