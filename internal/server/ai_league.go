package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- Pocket AI, for the person asking only ----
//
// Two helps: a short match report to post (the person reads it, changes it
// if they like and posts it as their own message — the AI never writes in
// the chat), and a catch-up of the chat they missed. The AI sees that
// match, or that group's recent messages, nothing else.

const aiSystem = `You help friends who play sport together for fun. Write plainly and warmly, like one of the group. Short sentences, no hype, at most one emoji. Use the language the people write in (English when unsure).`

func (s *Server) aiReady(w http.ResponseWriter, ctx context.Context, uid int64) (*ai.Client, func(), bool) {
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "AI help is switched off (Settings → AI)")
		return nil, nil, false
	}
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeErr(w, http.StatusServiceUnavailable, firstNonEmpty(why, "no AI is set up (Settings → AI)"))
		return nil, nil, false
	}
	select {
	case s.aiSem <- struct{}{}:
		return cl, func() { <-s.aiSem }, true
	case <-time.After(20 * time.Second):
		writeErr(w, http.StatusServiceUnavailable, "the AI is busy, try again in a moment")
		return nil, nil, false
	}
}

func (s *Server) handleMatchReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, g, ok := s.myMatch(w, r)
	if !ok {
		return
	}
	cl, done, ok := s.aiReady(w, ctx, actorID(ctx))
	if !ok {
		return
	}
	defer done()
	names := s.names(ctx, g.ID)
	t, _ := s.table(ctx, g, "season")
	var b strings.Builder
	fmt.Fprintf(&b, "Group: %s (%s)\nResult: %s\nPlayed: %s\n", g.Name, league.DisplayName(g.Sport, g.SportName), s.resultLine(g, m, names), whenText(m.PlayedAt))
	if m.Note != "" {
		fmt.Fprintf(&b, "Note: %s\n", m.Note)
	}
	b.WriteString("\nThe table this season (position. name: points, played, form):\n")
	for i, row := range t.Rows {
		if i >= 8 || row.Played == 0 {
			break
		}
		fmt.Fprintf(&b, "%d. %s: %d points, %d played, form %s\n", row.Position, names[row.UserID], row.Points, row.Played, strings.Join(row.Form, ""))
	}
	if m.GameID != 0 {
		cs, _ := s.store.ListMessages(ctx, store.MessageFilter{GroupID: g.ID, GameID: m.GameID, Limit: 30})
		if len(cs) > 0 {
			b.WriteString("\nComments on the game:\n")
			for _, c := range cs {
				if c.Kind == "text" {
					fmt.Fprintf(&b, "%s: %s\n", firstNonEmpty(names[c.UserID], "someone"), clip(c.Body, 300))
				}
			}
		}
	}
	txt, err := cl.Complete(ctx, aiSystem, "Write a match report for the group chat: two to four sentences, a little playful, fair to both sides, mention the score. Only use what is given.\n\n"+b.String(), false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt)})
}

func (s *Server) handleCatchUp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	cl, done, ok := s.aiReady(w, ctx, actorID(ctx))
	if !ok {
		return
	}
	defer done()
	names := s.names(ctx, g.ID)
	ms, _ := s.store.ListMessages(ctx, store.MessageFilter{GroupID: g.ID, Limit: 120})
	var b strings.Builder
	n := 0
	for _, m := range ms {
		if m.ID <= g.LastReadID && n == 0 && len(ms) > 40 {
			continue
		}
		who := firstNonEmpty(names[m.UserID], "League")
		fmt.Fprintf(&b, "[%s] %s: %s\n", m.CreatedAt[:16], who, clip(m.Body, 400))
		n++
	}
	if n == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"text": "Nothing new in the chat."})
		return
	}
	txt, err := cl.Complete(ctx, aiSystem, "Catch me up on this group chat in three to six short bullet lines: what was decided, what is still open, anything asked of me ("+firstNonEmpty(names[actorID(ctx)], "me")+"). Lines from League are the app noting games and results.\n\n"+b.String(), false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt)})
}
