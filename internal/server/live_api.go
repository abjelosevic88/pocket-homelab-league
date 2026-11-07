package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- live scoring ----
//
// Someone at the court taps who won each point; everyone else (and the
// tournament's public page) sees the score move. When it's over the score
// goes in as the result, like one entered by hand.

type liveView struct {
	*store.LiveScore
	Rules      league.LiveRules `json:"rules"`
	View       league.LiveView  `json:"view"`
	Line       string           `json:"line"`
	ScorerName string           `json:"scorer_name"`
}

func (s *Server) viewLive(ctx context.Context, l *store.LiveScore) liveView {
	var r league.LiveRules
	_ = json.Unmarshal(l.Rules, &r)
	r = r.Clean()
	v := league.Live(r, l.Points)
	out := liveView{LiveScore: l, Rules: r, View: v, Line: league.LiveLine(r, v)}
	if l.Scorer != 0 {
		out.ScorerName = s.nameIn(ctx, l.GroupID, l.Scorer)
	}
	return out
}

func (s *Server) handleListLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	l, err := s.store.LiveIn(ctx, g.ID)
	if err != nil {
		s.fail(w, err, "live")
		return
	}
	out := make([]liveView, 0, len(l))
	for _, x := range l {
		out = append(out, s.viewLive(ctx, x))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStartLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		GameID   int64             `json:"game_id"`
		TMatchID int64             `json:"tmatch_id"`
		AIDs     []int64           `json:"a_ids"`
		BIDs     []int64           `json:"b_ids"`
		AName    string            `json:"a_name"`
		BName    string            `json:"b_name"`
		Rules    *league.LiveRules `json:"rules"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	rules := league.LiveRulesFor(g.Sport, g.Scoring)
	if g.Scoring != league.ScoringSets {
		rules = league.LiveRules{Kind: league.LiveCount}
	}
	if in.Rules != nil {
		rules = in.Rules.Clean()
	}
	if g.Scoring == league.ScoringRanking || g.Scoring == league.ScoringResult {
		writeErr(w, http.StatusBadRequest, "live scoring is for matches between two sides")
		return
	}
	l := &store.LiveScore{GroupID: g.ID, Scorer: actorID(ctx)}
	names := s.names(ctx, g.ID)
	switch {
	case in.TMatchID != 0:
		m, err := s.store.GetTMatch(ctx, in.TMatchID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		t, err := s.store.GetTournament(ctx, m.TournamentID)
		if err != nil || t.GroupID != g.ID || t.Status != "live" {
			writeErr(w, http.StatusBadRequest, "that match isn't being played")
			return
		}
		es, _ := s.store.Entries(ctx, t.ID)
		side := func(entry int64, list []int64) (string, []int64) {
			if len(list) == 0 && entry != 0 {
				list = []int64{entry}
			}
			var ns []string
			var ids []int64
			for _, e := range es {
				for _, x := range list {
					if e.ID == x {
						ns = append(ns, e.Name)
						for _, mem := range e.Members {
							ids = append(ids, mem.UserID)
						}
					}
				}
			}
			return strings.Join(ns, " & "), ids
		}
		l.AName, l.AIDs = side(m.AEntry, m.Sides["a"])
		l.BName, l.BIDs = side(m.BEntry, m.Sides["b"])
		if l.AName == "" || l.BName == "" {
			writeErr(w, http.StatusBadRequest, "who plays is not known yet")
			return
		}
		l.TMatchID = m.ID
		if old, _ := s.store.LiveForTournament(ctx, t.ID); len(old) > 0 {
			for _, o := range old {
				if o.TMatchID == m.ID {
					writeJSON(w, http.StatusOK, s.viewLive(ctx, o))
					return
				}
			}
		}
	default:
		if in.GameID != 0 {
			gm, err := s.store.GetGame(ctx, actorID(ctx), in.GameID)
			if err != nil || gm.GroupID != g.ID {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			l.GameID = gm.ID
		}
		for _, id := range append(append([]int64{}, in.AIDs...), in.BIDs...) {
			if _, ok := names[id]; !ok {
				writeErr(w, http.StatusBadRequest, "every player must be in the group")
				return
			}
		}
		if n := league.SideSize(g.Format); n > 0 && (len(in.AIDs) != n || len(in.BIDs) != n) {
			writeErr(w, http.StatusBadRequest, "pick who plays on each side")
			return
		}
		l.AIDs, l.BIDs = in.AIDs, in.BIDs
		l.AName = firstNonEmpty(clip(strings.TrimSpace(in.AName), 60), sideLabel(names, in.AIDs, g.Format, "A"))
		l.BName = firstNonEmpty(clip(strings.TrimSpace(in.BName), 60), sideLabel(names, in.BIDs, g.Format, "B"))
	}
	if l.AIDs == nil {
		l.AIDs = []int64{}
	}
	if l.BIDs == nil {
		l.BIDs = []int64{}
	}
	l.Rules, _ = json.Marshal(rules)
	if err := s.store.AddLive(ctx, l); err != nil {
		s.fail(w, err, "live")
		return
	}
	l, _ = s.store.GetLive(ctx, l.ID)
	v := s.viewLive(ctx, l)
	s.toGroup(ctx, g.ID, "live", v)
	writeJSON(w, http.StatusCreated, v)
}

func sideLabel(names map[int64]string, ids []int64, format, def string) string {
	if len(ids) == 0 {
		return "Side " + def
	}
	if format == league.FormatTeams {
		return "Team " + firstName(names[ids[0]])
	}
	var ns []string
	for _, id := range ids {
		ns = append(ns, firstName(names[id]))
	}
	return strings.Join(ns, " & ")
}

func (s *Server) myLive(w http.ResponseWriter, r *http.Request) (*store.LiveScore, *store.Group, bool) {
	ctx := r.Context()
	l, err := s.store.GetLive(ctx, idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	g, err := s.store.GetGroup(ctx, actorID(ctx), l.GroupID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return l, g, true
}

func (s *Server) handleGetLive(w http.ResponseWriter, r *http.Request) {
	l, _, ok := s.myLive(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.viewLive(r.Context(), l))
}

func (s *Server) handleLivePoint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l, g, ok := s.myLive(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	var in struct {
		Side string `json:"side"`
		N    int    `json:"n"`
		Undo bool   `json:"undo"`
		Take bool   `json:"take"` // take over from someone else scoring
		At   int    `json:"at"`   // how many points the scorer saw (stops double taps from two phones)
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if l.FinishedAt != nil {
		writeErr(w, http.StatusBadRequest, "this match is over")
		return
	}
	if l.Scorer != 0 && l.Scorer != uid && !in.Take {
		if t, err := time.Parse(time.RFC3339Nano, l.UpdatedAt); err == nil && time.Since(t) < 2*time.Minute {
			writeJSON(w, http.StatusConflict, map[string]any{"error": s.nameIn(ctx, g.ID, l.Scorer) + " is keeping the score", "scorer": l.Scorer})
			return
		}
	}
	if in.At > 0 && in.At != len(l.Points) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the score moved on — have another look", "live": s.viewLive(ctx, l)})
		return
	}
	points := l.Points
	if in.Undo {
		if len(points) > 0 {
			points = points[:len(points)-1]
		}
	} else {
		if in.Side != "a" && in.Side != "b" {
			writeErr(w, http.StatusBadRequest, "a or b")
			return
		}
		p := in.Side
		if in.N > 1 && in.N <= 9 {
			p += itoa(int64(in.N))
		}
		var rules league.LiveRules
		_ = json.Unmarshal(l.Rules, &rules)
		if league.Live(rules, points).Done {
			writeErr(w, http.StatusBadRequest, "the match is decided — finish it")
			return
		}
		if len(points) >= 2000 {
			writeErr(w, http.StatusBadRequest, "that's a long match")
			return
		}
		points = append(points, p)
	}
	if err := s.store.SaveLivePoints(ctx, l.ID, uid, points); err != nil {
		s.fail(w, err, "point")
		return
	}
	l, _ = s.store.GetLive(ctx, l.ID)
	v := s.viewLive(ctx, l)
	s.toGroup(ctx, g.ID, "live", v)
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleFinishLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l, g, ok := s.myLive(w, r)
	if !ok {
		return
	}
	if err := s.store.FinishLive(ctx, l.ID); err != nil {
		s.fail(w, err, "finish")
		return
	}
	l, _ = s.store.GetLive(ctx, l.ID)
	v := s.viewLive(ctx, l)
	s.toGroup(ctx, g.ID, "live", v)
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l, g, ok := s.myLive(w, r)
	if !ok {
		return
	}
	if l.Scorer != actorID(ctx) && !organiser(g) {
		writeErr(w, http.StatusForbidden, "whoever keeps the score (or an organiser) can drop it")
		return
	}
	if err := s.store.DeleteLive(ctx, l.ID); err != nil {
		s.fail(w, err, "delete")
		return
	}
	s.toGroup(ctx, g.ID, "live", map[string]any{"id": l.ID, "group_id": g.ID, "deleted": true})
	w.WriteHeader(http.StatusNoContent)
}

// tournamentLive is what's being scored live in a tournament, by match.
func (s *Server) tournamentLive(ctx context.Context, tid int64, public bool) map[int64]map[string]any {
	l, _ := s.store.LiveForTournament(ctx, tid)
	out := map[int64]map[string]any{}
	for _, x := range l {
		v := s.viewLive(ctx, x)
		m := map[string]any{"id": x.ID, "line": v.Line, "view": v.View, "rules": v.Rules, "a_name": x.AName, "b_name": x.BName, "updated_at": x.UpdatedAt}
		if !public {
			m["scorer_name"] = v.ScorerName
		}
		out[x.TMatchID] = m
	}
	return out
}
