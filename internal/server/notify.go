package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-league/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- telling people ----
//
// Each person chooses what they hear (Settings → Notifications) and, per
// group, everything / only games / nothing. A message goes to their own
// phones (web push) and notification address (Pocket Alerts, ntfy…);
// someone with neither gets an email instead when the group allows it and
// they did not turn emails off, sent the way the group's maker sends email. Chat messages
// are never emailed, and nobody is told about a chat they have open.

// note is one thing to tell people.
type note struct {
	kind   string // plans | reminders | chat | short | results
	title  string
	body   string
	link   string
	button string
	tags   []string
}

func (n note) wanted(st store.Settings) bool {
	switch n.kind {
	case "plans":
		return st.NotifyPlans
	case "reminders":
		return st.NotifyReminders
	case "chat":
		return st.NotifyChat
	case "short":
		return st.NotifyShort
	case "results":
		return st.NotifyResults
	}
	return true
}

// tell sends a note to these people of a group (never to the one who caused it).
func (s *Server) tell(ctx context.Context, g *store.Group, uids []int64, except int64, n note) {
	for _, uid := range uids {
		if uid == except {
			continue
		}
		m, err := s.store.GetMember(ctx, g.ID, uid)
		if err != nil || m.Notify == "off" || (m.Notify == "games" && n.kind == "chat") {
			continue
		}
		if n.kind == "chat" && s.hub.online(uid) {
			continue
		}
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || !n.wanted(st) {
			continue
		}
		phones := s.webPush(ctx, uid, n)
		if st.NotifyURL != "" {
			s.push(ctx, uid, st, n)
			continue
		}
		if phones > 0 {
			continue
		}
		if n.kind == "chat" || !st.EmailMe || !g.EmailPlayers || m.Email == "" {
			continue
		}
		u, err := s.store.GetUser(ctx, uid)
		if err != nil {
			continue
		}
		if err := s.mailer(ctx, g.UserID, s.noteMail(u, g, n)); err != nil && err != errEmailOff {
			s.log.Debug("notification email failed", "user", uid, "err", err)
		}
	}
}

// tellGroup sends a note to everyone in a group.
func (s *Server) tellGroup(ctx context.Context, g *store.Group, except int64, n note) {
	ids, err := s.store.MemberIDs(ctx, g.ID)
	if err != nil {
		return
	}
	s.tell(ctx, g, ids, except, n)
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, n note) bool {
	if st.NotifyURL == "" {
		return false
	}
	tags := n.tags
	if len(tags) == 0 {
		tags = []string{"trophy"}
	}
	c := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := c.Send(ctx, notify.Message{Title: n.title, Body: n.body, Priority: "info", Tags: tags, Link: n.link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket League", Body: notifyTestBody, Priority: "info", Tags: []string{"trophy"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const notifyTestBody = "Notifications work. You will hear about new games, reminders before you play, results to confirm and — when League is closed — the chat."

// gameLink is a game's page.
func (s *Server) gameLink(gameID int64) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/games/" + itoa(gameID)
}

// groupLink is a group's page.
func (s *Server) groupLink(gid int64) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/groups/" + itoa(gid)
}
