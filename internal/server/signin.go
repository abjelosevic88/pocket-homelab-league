package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- signing in with a link ----
//
// A player gets a link (/join/<token>) by email, WhatsApp or anything else.
// Opening it shows who invited them to which group; tapping "Come in" uses
// the link up (POST, so link previews in chat apps cannot) and signs them
// in for a long time. A used or old link says how to get a new one.

// playerSessionTTL is how long a link sign-in lasts: players should not
// have to ask for a new link every month.
const playerSessionTTL = 180 * 24 * time.Hour

func (s *Server) handlePeekLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l, err := s.store.PeekSignInLink(ctx, chiParam(r, "token"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "This link was already used or has run out. Ask whoever invited you for a new one, or get one by email below.")
		return
	}
	u, err := s.store.GetUser(ctx, l.UserID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	out := map[string]any{"name": u.Name, "expires_at": l.ExpiresAt}
	if by, err := s.store.GetUser(ctx, l.CreatedBy); err == nil {
		out["invited_by"] = firstNonEmpty(by.Name, by.Email)
	}
	if g, err := s.store.GroupByID(ctx, l.GroupID); err == nil {
		sp := league.SportByKey(g.Sport)
		out["group"] = map[string]any{"name": g.Name, "sport": league.DisplayName(g.Sport, g.SportName), "emoji": sp.Emoji, "color": g.Color}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUseLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(r, &in); err != nil || in.Token == "" {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	l, err := s.store.UseSignInLink(ctx, strings.TrimSpace(in.Token))
	if err != nil {
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusNotFound, "This link was already used or has run out. Ask whoever invited you for a new one, or get one by email below.")
		return
	}
	u, err := s.store.GetUser(ctx, l.UserID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	ttl := s.cfg.SessionTTL
	if ttl < playerSessionTTL {
		ttl = playerSessionTTL
	}
	id, err := s.store.CreateSession(ctx, u.ID, ttl)
	if err != nil {
		s.fail(w, err, "create session")
		return
	}
	s.setSessionCookie(w, id, ttl)
	first := u.LastSeenAt == nil
	s.store.TouchUser(ctx, u.ID)
	if first && l.GroupID > 0 {
		if g, err := s.store.GroupByID(ctx, l.GroupID); err == nil {
			if m, err := s.store.GetMember(ctx, g.ID, u.ID); err == nil {
				s.toGroup(ctx, g.ID, "members", map[string]any{"group_id": g.ID})
				s.log.Info("player came in", "group", g.ID, "player", m.UserID)
			}
		}
	}
	s.log.Info("signed in with a link", "user", u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "group_id": l.GroupID, "first": first})
}

// handleRequestLink sends a new link to a player who lost theirs. It always
// answers the same, so nobody learns who has an account here.
func (s *Server) handleRequestLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Email string `json:"email"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	answer := map[string]any{"ok": true, "message": "If that address belongs to a player here, a new sign-in link is on its way. Nothing came? Ask whoever invited you."}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	u, err := s.store.GetUserByEmail(ctx, email)
	if err != nil || u.Role != store.RolePlayer {
		time.Sleep(200 * time.Millisecond)
		writeJSON(w, http.StatusOK, answer)
		return
	}
	key := "linkreq:" + itoa(u.ID)
	if _, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < 3*time.Minute {
			writeJSON(w, http.StatusOK, answer)
			return
		}
	}
	_ = s.store.CachePut(ctx, key, "1")
	// the link comes from whoever invited them, through their way of sending email
	gids, _ := s.store.GroupIDsOf(ctx, u.ID)
	var g *store.Group
	for _, id := range gids {
		if x, err := s.store.GroupByID(ctx, id); err == nil {
			g = x
			break
		}
	}
	if g == nil {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	from := u.InvitedBy
	if from == 0 {
		from = g.UserID
	}
	by, err := s.store.GetUser(ctx, from)
	if err != nil {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	tok, _, err := s.store.CreateSignInLink(ctx, u.ID, by.ID, g.ID, signInTTL)
	if err == nil {
		link := s.publicBase(r) + "/join/" + tok
		if err := s.mailer(ctx, by.ID, s.newLinkMail(ctx, by, u, g, link)); err != nil {
			s.log.Warn("new sign-in link: email failed", "player", u.ID, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, answer)
}
