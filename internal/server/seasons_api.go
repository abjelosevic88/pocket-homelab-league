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
)

// ---- seasons ----
//
// A group can split its year into seasons ("Spring 2027"). Each has its own
// table; closing one keeps its table and awards as they were, posts them in
// the chat, and can start the next. With "fresh ratings each season" the
// ratings start at 1000 again.

// seasonEnd is the moment (exclusive) a season's matches stop counting.
func seasonEnd(x *store.Season) string {
	if x.EndsOn == "" {
		return ""
	}
	return x.EndsOn + "T99"
}

// Award is a line in a closed season: the champion, the most games …
type Award struct {
	Key    string `json:"key"`
	Icon   string `json:"icon"`
	Title  string `json:"title"`
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

type tableRow struct {
	*league.Row
	Name   string `json:"name"`
	Member bool   `json:"member"`
}

func (s *Server) tableRows(ctx context.Context, g *store.Group, t *league.Table) []tableRow {
	names := s.names(ctx, g.ID)
	rows := make([]tableRow, 0, len(t.Rows))
	for _, x := range t.Rows {
		n, member := names[x.UserID]
		rows = append(rows, tableRow{Row: x, Name: firstNonEmpty(n, "Former player"), Member: member})
	}
	return rows
}

func (s *Server) handleSeasons(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	l, err := s.store.Seasons(ctx, g.ID)
	if err != nil {
		s.fail(w, err, "seasons")
		return
	}
	for _, x := range l {
		x.Snapshot = nil
	}
	cur := s.store.CurrentSeason(ctx, g.ID, store.LocalNow()[:10])
	var curID int64
	if cur != nil {
		curID = cur.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"seasons": l, "current": curID})
}

type seasonInput struct {
	Name     string `json:"name"`
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
}

func (in seasonInput) check() string {
	if strings.TrimSpace(in.Name) == "" {
		return "name the season"
	}
	if _, err := time.Parse("2006-01-02", in.StartsOn); err != nil {
		return "when does it start?"
	}
	if in.EndsOn != "" {
		if e, err := time.Parse("2006-01-02", in.EndsOn); err != nil || in.EndsOn < in.StartsOn || e.IsZero() {
			return "it ends after it starts"
		}
	}
	return ""
}

func (s *Server) handleCreateSeason(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	var in seasonInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.check(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	x := &store.Season{GroupID: g.ID, Name: clip(strings.TrimSpace(in.Name), 60), StartsOn: in.StartsOn, EndsOn: in.EndsOn}
	if err := s.store.SaveSeason(ctx, x); err != nil {
		s.fail(w, err, "season")
		return
	}
	s.systemMessage(ctx, g, 0, "season", x.Name+" has begun — the table starts again", map[string]any{"season_id": x.ID})
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
	writeJSON(w, http.StatusCreated, x)
}

func (s *Server) mySeason(w http.ResponseWriter, r *http.Request) (*store.Season, *store.Group, bool) {
	ctx := r.Context()
	x, err := s.store.GetSeason(ctx, idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	g, err := s.store.GetGroup(ctx, actorID(ctx), x.GroupID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return x, g, true
}

func (s *Server) handleUpdateSeason(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	x, g, ok := s.mySeason(w, r)
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	var in seasonInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.check(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	x.Name, x.StartsOn, x.EndsOn = clip(strings.TrimSpace(in.Name), 60), in.StartsOn, in.EndsOn
	if err := s.store.SaveSeason(ctx, x); err != nil {
		s.fail(w, err, "season")
		return
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) handleDeleteSeason(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	x, g, ok := s.mySeason(w, r)
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	if err := s.store.DeleteSeason(ctx, x.ID); err != nil {
		s.fail(w, err, "season")
		return
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCloseSeason(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	x, g, ok := s.mySeason(w, r)
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	var in struct {
		Reopen bool         `json:"reopen"`
		Next   *seasonInput `json:"next"`
	}
	_ = decode(r, &in)
	if in.Reopen {
		_ = s.store.ReopenSeason(ctx, x.ID)
		s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
		x, _ = s.store.GetSeason(ctx, x.ID)
		writeJSON(w, http.StatusOK, x)
		return
	}
	if x.ClosedAt != nil {
		writeErr(w, http.StatusBadRequest, "it's closed already")
		return
	}
	if in.Next != nil {
		if msg := in.Next.check(); msg != "" {
			writeErr(w, http.StatusBadRequest, "the next season: "+msg)
			return
		}
	}
	today := store.LocalNow()[:10]
	if x.EndsOn == "" || x.EndsOn > today {
		x.EndsOn = today
	}
	t, ms := s.tableBetween(ctx, g, x.StartsOn+"T00:00", seasonEnd(x))
	rows := s.tableRows(ctx, g, t)
	awards := s.awards(ctx, g, x, rows)
	snap := map[string]any{"rows": rows, "matches": len(ms)}
	if err := s.store.CloseSeason(ctx, x.ID, x.EndsOn, snap, awards); err != nil {
		s.fail(w, err, "close")
		return
	}
	text := x.Name + " is over"
	var lines []string
	for _, a := range awards {
		if len(lines) == 4 {
			break
		}
		lines = append(lines, a.Icon+" "+a.Title+": "+a.Name)
	}
	if len(lines) > 0 {
		text += " — " + strings.Join(lines, " · ")
	}
	s.systemMessage(ctx, g, 0, "season_closed", text, map[string]any{"season_id": x.ID})
	ids, _ := s.store.MemberIDs(ctx, g.ID)
	body := "The final table and the awards are in."
	if len(awards) > 0 {
		body = awards[0].Icon + " " + awards[0].Title + ": " + awards[0].Name + ". " + body
	}
	s.tell(ctx, g, ids, actorID(ctx), note{kind: "results", title: g.Name + ": " + x.Name + " is over", body: body, link: s.groupLink(g.ID) + "?tab=table"})
	if in.Next != nil {
		nx := &store.Season{GroupID: g.ID, Name: clip(strings.TrimSpace(in.Next.Name), 60), StartsOn: in.Next.StartsOn, EndsOn: in.Next.EndsOn}
		if err := s.store.SaveSeason(ctx, nx); err == nil {
			s.systemMessage(ctx, g, 0, "season", nx.Name+" has begun — the table starts again", map[string]any{"season_id": nx.ID})
		}
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
	x, _ = s.store.GetSeason(ctx, x.ID)
	writeJSON(w, http.StatusOK, x)
}

// awards are a season's honours, from its table.
func (s *Server) awards(ctx context.Context, g *store.Group, x *store.Season, rows []tableRow) []Award {
	out := []Award{}
	played := []tableRow{}
	for _, r := range rows {
		if r.Played > 0 {
			played = append(played, r)
		}
	}
	if len(played) == 0 {
		return out
	}
	medals := []struct{ icon, title string }{{"🏆", "Champion"}, {"🥈", "Runner-up"}, {"🥉", "Third"}}
	for i := 0; i < len(played) && i < 3; i++ {
		r := played[i]
		v := fmt.Sprintf("%d pts", r.Points)
		if g.Scoring == league.ScoringRanking {
			v = fmt.Sprintf("%d wins", r.Wins)
		}
		out = append(out, Award{Key: fmt.Sprintf("place%d", i+1), Icon: medals[i].icon, Title: medals[i].title, UserID: r.UserID, Name: r.Name, Value: v})
	}
	best := func(key, icon, title string, val func(tableRow) float64, show func(tableRow) string, ok func(tableRow) bool) {
		var top *tableRow
		for i := range played {
			r := played[i]
			if ok != nil && !ok(r) {
				continue
			}
			if val(r) <= 0 {
				continue
			}
			if top == nil || val(r) > val(*top) {
				top = &played[i]
			}
		}
		if top != nil {
			out = append(out, Award{Key: key, Icon: icon, Title: title, UserID: top.UserID, Name: top.Name, Value: show(*top)})
		}
	}
	best("most_played", "🔁", "Most games", func(r tableRow) float64 { return float64(r.Played) }, func(r tableRow) string { return fmt.Sprintf("%d played", r.Played) }, nil)
	if g.Scoring != league.ScoringRanking {
		best("win_rate", "🎯", "Best win rate", func(r tableRow) float64 { return float64(r.Won) / float64(r.Played) },
			func(r tableRow) string { return fmt.Sprintf("%d%% of %d", r.Won*100/r.Played, r.Played) }, func(r tableRow) bool { return r.Played >= 5 })
	}
	best("mvp", "⭐", "Most player-of-the-game votes", func(r tableRow) float64 { return float64(r.MVP) }, func(r tableRow) string { return fmt.Sprintf("%d×", r.MVP) }, nil)
	if g.Scoring == league.ScoringGoals {
		best("scorer", "⚽", "Top scorer", func(r tableRow) float64 { return r.Scored }, func(r tableRow) string { return trimNum(r.Scored) + " goals" }, nil)
	}
	// the stats the group counts: the leader of each (cards are no honour)
	keys := append([]string{}, g.Stats...)
	sort.Strings(keys)
	for _, k := range keys {
		if k == "yellow" || k == "red" || k == "double_faults" {
			continue
		}
		info := statInfo(k)
		best("stat_"+k, info.Icon, "Most "+strings.ToLower(info.Name), func(r tableRow) float64 { return float64(r.Stats[k]) }, func(r tableRow) string { return fmt.Sprintf("%d", r.Stats[k]) }, nil)
	}
	// who came most: reliability over the season
	if rel, err := s.store.GroupReliability(ctx, g.ID, x.EndsOn+"T99"); err == nil {
		var top *store.Reliability
		for _, r := range rel {
			if r.Came > 0 && (top == nil || r.Came > top.Came || (r.Came == top.Came && r.LateOuts+r.NoShows < top.LateOuts+top.NoShows)) {
				top = r
			}
		}
		if top != nil {
			out = append(out, Award{Key: "always_there", Icon: "💪", Title: "Always there", UserID: top.UserID, Name: s.nameIn(ctx, g.ID, top.UserID), Value: fmt.Sprintf("came %d times", top.Came)})
		}
	}
	return out
}

func statInfo(k string) league.Stat {
	for _, s := range league.Stats {
		if s.Key == k {
			return s
		}
	}
	return league.Stat{Key: k, Name: k, Icon: "•"}
}

// seasonTable answers a table request for a season: the kept table when
// closed, worked out live while it's on.
func (s *Server) seasonTable(ctx context.Context, g *store.Group, x *store.Season) map[string]any {
	if x.ClosedAt != nil && len(x.Snapshot) > 0 {
		var snap map[string]json.RawMessage
		_ = json.Unmarshal(x.Snapshot, &snap)
		return map[string]any{"period": "season", "season": x, "rows": snap["rows"], "matches": snap["matches"], "frozen": true, "awards": x.Awards,
			"rules": league.RulesOf(g), "group": viewGroup(g)}
	}
	t, ms := s.tableBetween(ctx, g, x.StartsOn+"T00:00", seasonEnd(x))
	snapless := *x
	snapless.Snapshot = nil
	return map[string]any{"period": "season", "season": snapless, "rows": s.tableRows(ctx, g, t), "matches": len(ms), "frozen": false,
		"rules": league.RulesOf(g), "group": viewGroup(g)}
}
