package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- the chat ----
//
// Every group has one chat, and every game has its own comments. The app
// itself writes short lines into the chat when something happens (a game
// planned, a result, someone joined) so the talk and the plans stay
// together. Pocket AI never writes here: what it drafts goes to the person
// who asked, who may post it as their own message.

// systemMessage writes a line from League into a group's chat (gameID 0)
// or under a game. Lines about a game go to both.
func (s *Server) systemMessage(ctx context.Context, g *store.Group, gameID int64, what, text string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["what"] = what
	b, _ := json.Marshal(data)
	m := &store.Message{GroupID: g.ID, Kind: "system", Body: clip(text, 1000), Data: b}
	if _, err := s.store.AddMessage(ctx, m); err != nil {
		s.log.Warn("system message", "err", err)
		return
	}
	s.toGroup(ctx, g.ID, "message", m)
	if gameID > 0 {
		c := &store.Message{GroupID: g.ID, GameID: gameID, Kind: "system", Body: m.Body, Data: b}
		if _, err := s.store.AddMessage(ctx, c); err == nil {
			s.toGroup(ctx, g.ID, "message", c)
		}
	}
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	f := store.MessageFilter{GroupID: g.ID, Before: int64(queryInt(r, "before")), After: int64(queryInt(r, "after")), Limit: queryInt(r, "limit")}
	if gid := queryInt(r, "game"); gid > 0 {
		gm, err := s.store.GetGame(ctx, actorID(ctx), int64(gid))
		if err != nil || gm.GroupID != g.ID {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		f.GameID = gm.ID
	}
	ms, err := s.store.ListMessages(ctx, f)
	if err != nil {
		s.fail(w, err, "messages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": ms, "names": s.names(ctx, g.ID), "last_read_id": g.LastReadID})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	u := userFrom(ctx)
	var in struct {
		Body     string `json:"body"`
		GameID   int64  `json:"game_id"`
		ReplyTo  int64  `json:"reply_to"`
		ClientID string `json:"client_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		writeErr(w, http.StatusBadRequest, "write something")
		return
	}
	if utf8.RuneCountInString(in.Body) > 4000 {
		writeErr(w, http.StatusBadRequest, "a message is up to 4000 characters")
		return
	}
	var game *store.Game
	if in.GameID != 0 {
		gm, err := s.store.GetGame(ctx, u.ID, in.GameID)
		if err != nil || gm.GroupID != g.ID {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		game = gm
	}
	if in.ReplyTo != 0 {
		if p, err := s.store.GetMessage(ctx, in.ReplyTo); err != nil || p.GroupID != g.ID {
			in.ReplyTo = 0
		}
	}
	m := &store.Message{GroupID: g.ID, GameID: in.GameID, UserID: u.ID, Kind: "text", Body: in.Body, ReplyTo: in.ReplyTo, ClientID: clip(in.ClientID, 64)}
	isNew, err := s.store.AddMessage(ctx, m)
	if err != nil {
		s.fail(w, err, "send")
		return
	}
	if isNew {
		_ = s.store.MarkRead(ctx, g.ID, u.ID, m.ID)
		s.toGroup(ctx, g.ID, "message", m)
		name := s.nameIn(ctx, g.ID, u.ID)
		title := name + " in " + g.Name
		link := s.groupLink(g.ID) + "?tab=chat"
		if game != nil {
			title = name + " on " + g.Name + ", " + whenText(game.StartsAt)
			link = s.gameLink(game.ID)
		}
		go s.tellChat(g, u.ID, note{kind: "chat", title: title, body: clip(in.Body, 300), link: link, tags: []string{"speech_balloon"}})
	}
	writeJSON(w, http.StatusCreated, m)
}

// tellChat tells the people who don't have League open about a message.
func (s *Server) tellChat(g *store.Group, from int64, n note) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.tellGroup(ctx, g, from, n)
}

// myMessage loads a message of a group the person is in.
func (s *Server) myMessage(w http.ResponseWriter, r *http.Request) (*store.Message, *store.Group, bool) {
	ctx := r.Context()
	m, err := s.store.GetMessage(ctx, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "message")
		return nil, nil, false
	}
	g, err := s.store.GetGroup(ctx, actorID(ctx), m.GroupID)
	if err != nil {
		s.fail(w, err, "message")
		return nil, nil, false
	}
	return m, g, true
}

func (s *Server) handleEditMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, g, ok := s.myMessage(w, r)
	if !ok {
		return
	}
	if m.UserID != actorID(ctx) || m.Kind != "text" || m.DeletedAt != nil {
		writeErr(w, http.StatusForbidden, "you can only change your own messages")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Body) == "" || utf8.RuneCountInString(in.Body) > 4000 {
		writeErr(w, http.StatusBadRequest, "write something (up to 4000 characters)")
		return
	}
	if err := s.store.EditMessage(ctx, m.ID, strings.TrimSpace(in.Body)); err != nil {
		s.fail(w, err, "edit")
		return
	}
	m, _ = s.store.GetMessage(ctx, m.ID)
	s.toGroup(ctx, g.ID, "message_update", m)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, g, ok := s.myMessage(w, r)
	if !ok {
		return
	}
	if m.UserID != actorID(ctx) && !organiser(g) {
		writeErr(w, http.StatusForbidden, "you can delete your own messages (organisers: any)")
		return
	}
	if err := s.store.DeleteMessage(ctx, m.ID); err != nil {
		s.fail(w, err, "delete")
		return
	}
	m, _ = s.store.GetMessage(ctx, m.ID)
	s.toGroup(ctx, g.ID, "message_update", m)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, g, ok := s.myMessage(w, r)
	if !ok {
		return
	}
	var in struct {
		Emoji string `json:"emoji"`
	}
	if err := decode(r, &in); err != nil || in.Emoji == "" || utf8.RuneCountInString(in.Emoji) > 8 || m.DeletedAt != nil {
		writeErr(w, http.StatusBadRequest, "pick an emoji")
		return
	}
	if err := s.store.ToggleReaction(ctx, m.ID, actorID(ctx), in.Emoji); err != nil {
		s.fail(w, err, "react")
		return
	}
	m, _ = s.store.GetMessage(ctx, m.ID)
	s.toGroup(ctx, g.ID, "message_update", m)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		UpTo int64 `json:"up_to"`
	}
	_ = decode(r, &in)
	if in.UpTo <= 0 {
		in.UpTo = g.LastMsgID
	}
	if err := s.store.MarkRead(ctx, g.ID, actorID(ctx), in.UpTo); err != nil {
		s.fail(w, err, "read")
		return
	}
	s.toPeople([]int64{actorID(ctx)}, "read", map[string]any{"group_id": g.ID, "last_read_id": in.UpTo})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleTyping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	uid := actorID(ctx)
	var in struct {
		GameID int64 `json:"game_id"`
	}
	_ = decode(r, &in)
	if s.hub.typingAllowed(g.ID, uid) {
		ids, _ := s.store.MemberIDs(ctx, g.ID)
		others := ids[:0]
		for _, id := range ids {
			if id != uid {
				others = append(others, id)
			}
		}
		s.toPeople(others, "typing", map[string]any{"group_id": g.ID, "game_id": in.GameID, "user_id": uid, "name": s.nameIn(ctx, g.ID, uid)})
	}
	w.WriteHeader(http.StatusNoContent)
}
