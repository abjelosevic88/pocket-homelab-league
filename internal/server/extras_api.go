package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- guests (+1) ----
//
// Anyone in the group may bring a friend to a game. A guest takes a place
// like everyone else (and goes on the waiting list when it's full); a
// member waiting moves up before a guest does.

const maxGuestsEach = 3

func (s *Server) handleAddGuest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "say the guest's name")
		return
	}
	if gm.Status != store.GamePlanned {
		writeErr(w, http.StatusBadRequest, "guests come to planned games")
		return
	}
	uid := actorID(ctx)
	guests, _ := s.store.Guests(ctx, gm.ID)
	mine := 0
	for _, x := range guests {
		if x.AddedBy == uid {
			mine++
		}
	}
	if mine >= maxGuestsEach && !organiser(g) {
		writeErr(w, http.StatusBadRequest, "up to 3 guests each")
		return
	}
	ans := store.AnswerIn
	if gm.MaxPlayers > 0 && gm.In >= gm.MaxPlayers {
		ans = store.AnswerWaiting
	}
	x := &store.Guest{GameID: gm.ID, AddedBy: uid, Name: clip(strings.TrimSpace(in.Name), 60), Answer: ans}
	if err := s.store.AddGuest(ctx, x); err != nil {
		s.fail(w, err, "guest")
		return
	}
	who := s.nameIn(ctx, g.ID, uid)
	line := who + " brings " + x.Name
	if ans == store.AnswerWaiting {
		line += " (on the waiting list)"
	}
	s.systemMessage(ctx, g, gm.ID, "guest", line, map[string]any{"game_id": gm.ID})
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
	gm, _ = s.store.GetGame(ctx, uid, gm.ID)
	writeJSON(w, http.StatusCreated, s.gameDetail(ctx, gm, g))
}

func (s *Server) handleDeleteGuest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	x, err := s.store.GetGuest(ctx, idParam(r, "guest"))
	if err != nil || x.GameID != gm.ID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if x.AddedBy != actorID(ctx) && !canEditGame(gm, g, actorID(ctx)) {
		writeErr(w, http.StatusForbidden, "whoever brought them (or an organiser) can take a guest off")
		return
	}
	if err := s.store.DeleteGuest(ctx, x.ID); err != nil {
		s.fail(w, err, "guest")
		return
	}
	if x.Answer == store.AnswerIn {
		s.moveUp(ctx, g, gm.ID)
	}
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
	gm, _ = s.store.GetGame(ctx, actorID(ctx), gm.ID)
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

// moveUpGuest lets the first guest waiting in (after members).
func (s *Server) moveUpGuest(ctx context.Context, g *store.Group, gm *store.Game) bool {
	guests, _ := s.store.Guests(ctx, gm.ID)
	for _, x := range guests {
		if x.Answer != store.AnswerWaiting {
			continue
		}
		if s.store.SetGuestAnswer(ctx, x.ID, store.AnswerIn) == nil {
			s.systemMessage(ctx, g, gm.ID, "moved_up", x.Name+" (guest) moved up from the waiting list", map[string]any{"game_id": gm.ID})
			if x.AddedBy > 0 {
				s.tell(ctx, g, []int64{x.AddedBy}, 0, note{kind: "plans", title: x.Name + " is in: " + g.Name + ", " + whenText(gm.StartsAt),
					body: "A place came free for your guest.", link: s.gameLink(gm.ID)})
			}
			return true
		}
	}
	return false
}

// ---- reliability ----

// afterAnswer notes a change of answer and what follows from it: a late
// drop-out tells the organisers (and offers to find a sub), a chore goes to
// someone else, an offered place counts as taken.
func (s *Server) afterAnswer(ctx context.Context, g *store.Group, gm *store.Game, uid int64, before, after string) {
	if before == after {
		return
	}
	until := time.Until(parseTS(gm.StartsAt))
	late := before == store.AnswerIn && after != store.AnswerIn && after != store.AnswerWaiting && until > 0 && until < 24*time.Hour
	s.store.LogAnswer(ctx, gm.ID, uid, before, after, late)
	if after == store.AnswerIn {
		s.store.TakeSubOffer(ctx, gm.ID, uid)
	}
	if before == store.AnswerIn && after != store.AnswerIn {
		s.reassignDuties(ctx, g, gm.ID, uid)
	}
	if !late {
		return
	}
	fresh, err := s.store.GameByID(ctx, gm.ID)
	if err != nil || (fresh.MinPlayers > 0 && fresh.In >= fresh.MinPlayers && (fresh.MaxPlayers == 0 || fresh.In >= fresh.MaxPlayers)) {
		return
	}
	ms, _ := s.store.Members(ctx, g.ID)
	var orgs []int64
	for _, m := range ms {
		if m.Role == "organiser" {
			orgs = append(orgs, m.UserID)
		}
	}
	name := s.nameIn(ctx, g.ID, uid)
	body := fmt.Sprintf("%s dropped out %s before. %d in now", name, untilWords(until), fresh.In)
	if fresh.MinPlayers > 0 {
		body += fmt.Sprintf(", %d needed", fresh.MinPlayers)
	}
	s.tell(ctx, g, orgs, uid, note{kind: "short", title: g.Name + ": " + name + " can't come", body: body + ". Find a sub on the game page.", link: s.gameLink(gm.ID), button: "Find a sub"})
}

func (s *Server) handleNoShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	if !canEditGame(gm, g, actorID(ctx)) {
		writeErr(w, http.StatusForbidden, "only the organisers mark who didn't come")
		return
	}
	var in struct {
		UserID int64 `json:"user_id"`
		NoShow bool  `json:"no_show"`
	}
	if err := decode(r, &in); err != nil || in.UserID == 0 {
		writeErr(w, http.StatusBadRequest, "who?")
		return
	}
	if time.Now().Before(parseTS(gm.StartsAt)) {
		writeErr(w, http.StatusBadRequest, "after the game has started")
		return
	}
	if err := s.store.SetNoShow(ctx, gm.ID, in.UserID, in.NoShow); err != nil {
		s.fail(w, err, "no-show")
		return
	}
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

func (s *Server) handleReliability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	rel, err := s.store.GroupReliability(ctx, g.ID, "", store.LocalNow())
	if err != nil {
		s.fail(w, err, "reliability")
		return
	}
	out := []*store.Reliability{}
	for uid, x := range rel {
		// everyone sees their own; the organisers see everyone's
		if organiser(g) || uid == actorID(ctx) {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	writeJSON(w, http.StatusOK, out)
}

// ---- away ----

func (s *Server) handleListAway(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.AwayList(r.Context(), actorID(r.Context()), r.URL.Query().Get("past") == "1")
	if err != nil {
		s.fail(w, err, "away")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleAddAway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
		Note string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.To == "" {
		in.To = in.From
	}
	f, e1 := time.Parse("2006-01-02", in.From)
	t, e2 := time.Parse("2006-01-02", in.To)
	if e1 != nil || e2 != nil || t.Before(f) || t.Sub(f) > 366*24*time.Hour {
		writeErr(w, http.StatusBadRequest, "pick the first and last day (up to a year)")
		return
	}
	a := &store.Away{UserID: actorID(ctx), From: in.From, To: in.To, Note: clip(strings.TrimSpace(in.Note), 80)}
	if err := s.store.AddAway(ctx, a); err != nil {
		s.fail(w, err, "away")
		return
	}
	// the games already planned on those days: out, unless already answered
	n := s.applyAwayToGames(ctx, a)
	writeJSON(w, http.StatusCreated, map[string]any{"away": a, "games_out": n})
}

func (s *Server) handleDeleteAway(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteAway(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "away")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const awayNote = "Away"

func (s *Server) applyAwayToGames(ctx context.Context, a *store.Away) int {
	groups, _ := s.store.ListGroups(ctx, a.UserID, false)
	n := 0
	for _, g := range groups {
		games, _ := s.store.ListGames(ctx, a.UserID, store.GameFilter{GroupID: g.ID, From: a.From + "T00:00", To: a.To + "T23:59", Upcoming: true})
		for _, gm := range games {
			if gm.Status != store.GamePlanned || gm.MyAnswer != "" {
				continue
			}
			if _, err := s.answer(ctx, g, gm, a.UserID, store.AnswerOut, firstNonEmpty(a.Note, awayNote)); err == nil {
				n++
			}
		}
	}
	return n
}

// markAway answers "out" for whoever is away on a planned game's day and
// hasn't answered; it returns them, so they aren't asked.
func (s *Server) markAway(ctx context.Context, g *store.Group, gm *store.Game) map[int64]bool {
	out := map[int64]bool{}
	if gm.Status != store.GamePlanned || len(gm.StartsAt) < 10 {
		return out
	}
	away, _ := s.store.AwayOn(ctx, g.ID, gm.StartsAt[:10])
	for uid, n := range away {
		out[uid] = true
		if s.store.GetAnswer(ctx, gm.ID, uid) == "" {
			_ = s.store.SetAnswer(ctx, gm.ID, uid, store.AnswerOut, firstNonEmpty(n, awayNote))
		}
	}
	return out
}

// awayForGame is who is away on the game's day and on each time voted on.
func (s *Server) awayForGame(ctx context.Context, g *store.Group, gm *store.Game, opts []*store.Option) map[string][]int64 {
	days := map[string]bool{}
	if len(gm.StartsAt) >= 10 {
		days[gm.StartsAt[:10]] = true
	}
	for _, o := range opts {
		if len(o.StartsAt) >= 10 {
			days[o.StartsAt[:10]] = true
		}
	}
	out := map[string][]int64{}
	for d := range days {
		a, _ := s.store.AwayOn(ctx, g.ID, d)
		for uid := range a {
			out[d] = append(out[d], uid)
		}
	}
	return out
}

// ---- finding a sub ----
//
// "Find a sub" asks, in turn: those who said maybe, those who haven't
// answered, and — when the one asking organises other groups of the same
// sport — the people there. Whoever takes it first plays (people from other
// groups join this one as players).

func (s *Server) handleFindSub(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	var in struct {
		Others bool `json:"others"` // also people from my other groups of this sport
		DryRun bool `json:"dry_run"`
	}
	_ = decode(r, &in)
	if gm.Status != store.GamePlanned || !time.Now().Before(parseTS(gm.StartsAt)) {
		writeErr(w, http.StatusBadRequest, "only for games still to come")
		return
	}
	if gm.MaxPlayers > 0 && gm.In >= gm.MaxPlayers {
		writeErr(w, http.StatusBadRequest, "the game is full")
		return
	}
	answers, _ := s.store.Answers(ctx, gm.ID)
	ans := map[int64]string{}
	for _, a := range answers {
		ans[a.UserID] = a.Answer
	}
	away, _ := s.store.AwayOn(ctx, g.ID, gm.StartsAt[:10])
	var maybe, silent, others []int64
	ms, _ := s.store.Members(ctx, g.ID)
	inGroup := map[int64]bool{}
	for _, m := range ms {
		inGroup[m.UserID] = true
		if m.UserID == uid || away[m.UserID] != "" {
			continue
		}
		switch ans[m.UserID] {
		case store.AnswerMaybe:
			maybe = append(maybe, m.UserID)
		case "":
			silent = append(silent, m.UserID)
		}
	}
	if in.Others {
		groups, _ := s.store.ListGroups(ctx, uid, false)
		seen := map[int64]bool{}
		for _, og := range groups {
			if og.ID == g.ID || og.Sport != g.Sport || !organiser(og) {
				continue
			}
			oms, _ := s.store.Members(ctx, og.ID)
			oaway, _ := s.store.AwayOn(ctx, og.ID, gm.StartsAt[:10])
			for _, m := range oms {
				if inGroup[m.UserID] || seen[m.UserID] || oaway[m.UserID] != "" {
					continue
				}
				seen[m.UserID] = true
				others = append(others, m.UserID)
			}
		}
	}
	if in.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{"maybe": len(maybe), "silent": len(silent), "others": len(others)})
		return
	}
	need := 1
	if gm.MinPlayers > gm.In {
		need = gm.MinPlayers - gm.In
	}
	where := ""
	if gm.VenueName != "" {
		where = " at " + gm.VenueName
	}
	asked := 0
	ask := func(ids []int64, outsider bool) {
		for _, id := range ids {
			if !s.store.OfferSub(ctx, gm.ID, id, uid) {
				continue
			}
			asked++
			link := s.gameLink(gm.ID)
			if outsider {
				link = strings.TrimRight(s.cfg.BaseURL, "/") + "/?subs=1"
			}
			s.tellOne(ctx, g, id, outsider, note{kind: "plans", title: fmt.Sprintf("%s: can you play %s?", g.Name, whenText(gm.StartsAt)),
				body: fmt.Sprintf("%s %s%s. %d %s needed — the first to say yes plays.", league.DisplayName(g.Sport, g.SportName), whenText(gm.StartsAt), where, need, plural(need, "player", "players")),
				link: link, button: "I can play"})
		}
	}
	ask(maybe, false)
	ask(silent, false)
	ask(others, true)
	s.systemMessage(ctx, g, gm.ID, "sub", fmt.Sprintf("%s is looking for %d %s for %s — asked %d %s", s.nameIn(ctx, g.ID, uid), need, plural(need, "player", "players"), whenText(gm.StartsAt), asked, plural(asked, "person", "people")), map[string]any{"game_id": gm.ID})
	writeJSON(w, http.StatusOK, map[string]any{"asked": asked, "maybe": len(maybe), "silent": len(silent), "others": len(others)})
}

// tellOne tells one person; someone not (yet) in the group is told without
// the group's per-member choices.
func (s *Server) tellOne(ctx context.Context, g *store.Group, uid int64, outsider bool, n note) {
	if !outsider {
		s.tell(ctx, g, []int64{uid}, 0, n)
		return
	}
	st, err := s.store.GetSettings(ctx, uid)
	if err != nil || !n.wanted(st) {
		return
	}
	phones := s.webPush(ctx, uid, n)
	if st.NotifyURL != "" {
		s.push(ctx, uid, st, n)
		return
	}
	if phones > 0 || !st.EmailMe {
		return
	}
	if u, err := s.store.GetUser(ctx, uid); err == nil && u.Email != "" {
		_ = s.mailer(ctx, actorID(ctx), s.noteMail(u, g, n))
	}
}

type subView struct {
	ID        int64   `json:"id"`
	Game      gameRef `json:"game"`
	OfferedBy string  `json:"offered_by"`
	InGroup   bool    `json:"in_group"`
}

type gameRef struct {
	ID        int64  `json:"id"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Emoji     string `json:"emoji"`
	Sport     string `json:"sport"`
	StartsAt  string `json:"starts_at"`
	Venue     string `json:"venue"`
	In        int    `json:"in"`
	Min       int    `json:"min_players"`
	Max       int    `json:"max_players"`
}

func (s *Server) handleMySubs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	offers, err := s.store.OpenSubOffers(ctx, uid)
	if err != nil {
		s.fail(w, err, "subs")
		return
	}
	out := []subView{}
	for _, o := range offers {
		gm, err := s.store.GameByID(ctx, o.GameID)
		if err != nil || (gm.MaxPlayers > 0 && gm.In >= gm.MaxPlayers) {
			continue
		}
		g, err := s.store.GroupByID(ctx, gm.GroupID)
		if err != nil {
			continue
		}
		_, notIn := s.store.GetMember(ctx, g.ID, uid)
		if notIn == nil && s.store.GetAnswer(ctx, gm.ID, uid) == store.AnswerIn {
			continue
		}
		sp := league.SportByKey(g.Sport)
		out = append(out, subView{ID: o.ID, OfferedBy: s.nameIn(ctx, g.ID, o.OfferedBy), InGroup: notIn == nil,
			Game: gameRef{ID: gm.ID, GroupID: g.ID, GroupName: g.Name, Emoji: sp.Emoji, Sport: league.DisplayName(g.Sport, g.SportName), StartsAt: gm.StartsAt,
				Venue: firstNonEmpty(gm.VenueName, g.VenueName), In: gm.In, Min: gm.MinPlayers, Max: gm.MaxPlayers}})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTakeSub(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, err := s.store.GetSubOffer(ctx, idParam(r, "id"))
	if err != nil || o.UserID != uid {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var in struct {
		No bool `json:"no"`
	}
	_ = decode(r, &in)
	gm, err := s.store.GameByID(ctx, o.GameID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	g, err := s.store.GroupByID(ctx, gm.GroupID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if in.No {
		s.store.TakeSubOffer(ctx, gm.ID, uid) // closed for them
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if gm.Status != store.GamePlanned || !time.Now().Before(parseTS(gm.StartsAt)) {
		writeErr(w, http.StatusConflict, "that game isn't on any more")
		return
	}
	if gm.MaxPlayers > 0 && gm.In >= gm.MaxPlayers {
		writeErr(w, http.StatusConflict, "someone was quicker — the game is full now")
		return
	}
	if _, err := s.store.GetMember(ctx, g.ID, uid); err != nil {
		if _, err := s.store.AddMember(ctx, g.ID, uid, o.OfferedBy, "player"); err != nil {
			s.fail(w, err, "join")
			return
		}
		ids, _ := s.store.MemberIDs(ctx, g.ID)
		s.toPeople(ids, "members", map[string]any{"group_id": g.ID})
		s.systemMessage(ctx, g, 0, "joined", s.nameIn(ctx, g.ID, uid)+" joined to fill in", map[string]any{"user_id": uid})
		g, _ = s.store.GroupByID(ctx, g.ID)
	}
	gg, err := s.store.GetGroup(ctx, uid, g.ID)
	if err != nil {
		s.fail(w, err, "group")
		return
	}
	res, err := s.answer(ctx, gg, gm, uid, store.AnswerIn, "")
	if err != nil {
		s.fail(w, err, "answer")
		return
	}
	if res == store.AnswerIn && o.OfferedBy > 0 && o.OfferedBy != uid {
		s.tell(ctx, gg, []int64{o.OfferedBy}, 0, note{kind: "short", title: s.nameIn(ctx, g.ID, uid) + " fills in: " + g.Name + ", " + whenText(gm.StartsAt), body: "Your sub is found.", link: s.gameLink(gm.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"answer": res, "game_id": gm.ID, "group_id": g.ID})
}

// ---- chores ----
//
// A group lists its chores (bring the balls, book the court, the bibs …).
// The day before a game each one goes to someone who is in, whoever did it
// least (and longest ago); they can hand it to someone else.

func (s *Server) handleSetDuties(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	var in struct {
		Names []string `json:"names"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	names := []string{}
	seen := map[string]bool{}
	for _, n := range in.Names {
		n = clip(strings.TrimSpace(n), 60)
		if n != "" && !seen[strings.ToLower(n)] {
			seen[strings.ToLower(n)] = true
			names = append(names, n)
		}
	}
	if len(names) > 10 {
		writeErr(w, http.StatusBadRequest, "up to 10 chores")
		return
	}
	if err := s.store.SetDuties(ctx, g.ID, names); err != nil {
		s.fail(w, err, "chores")
		return
	}
	l, _ := s.store.Duties(ctx, g.ID)
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleGetDuties(w http.ResponseWriter, r *http.Request) {
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	l, err := s.store.Duties(r.Context(), g.ID)
	if err != nil {
		s.fail(w, err, "chores")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleGameDuty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	dutyID := idParam(r, "duty")
	var in struct {
		UserID *int64 `json:"user_id"`
		Done   *bool  `json:"done"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	list, _ := s.store.GameDuties(ctx, g.ID, gm.ID)
	var d *store.GameDuty
	for i := range list {
		if list[i].DutyID == dutyID {
			d = &list[i]
		}
	}
	if d == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// the one doing it, or the organisers
	if d.UserID != uid && !canEditGame(gm, g, uid) && !(d.UserID == 0 && in.UserID != nil && *in.UserID == uid) {
		writeErr(w, http.StatusForbidden, "only whoever has it (or an organiser) can change it")
		return
	}
	if in.UserID != nil {
		if *in.UserID != 0 {
			if _, err := s.store.GetMember(ctx, g.ID, *in.UserID); err != nil {
				writeErr(w, http.StatusBadRequest, "they aren't in the group")
				return
			}
		}
		if err := s.store.SetGameDuty(ctx, gm.ID, dutyID, *in.UserID); err != nil {
			s.fail(w, err, "chore")
			return
		}
		if *in.UserID != 0 && *in.UserID != uid {
			s.tell(ctx, g, []int64{*in.UserID}, 0, note{kind: "reminders", title: g.Name + ": you have “" + d.Name + "”", body: s.nameIn(ctx, g.ID, uid) + " handed it to you for " + whenText(gm.StartsAt) + ".", link: s.gameLink(gm.ID)})
		}
	}
	if in.Done != nil {
		_ = s.store.SetDutyDone(ctx, gm.ID, dutyID, *in.Done)
	}
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

// assignDuties gives each chore of a game nobody has yet to someone in.
func (s *Server) assignDuties(ctx context.Context, g *store.Group, gm *store.Game, tellThem bool) {
	list, err := s.store.GameDuties(ctx, g.ID, gm.ID)
	if err != nil || len(list) == 0 {
		return
	}
	answers, _ := s.store.Answers(ctx, gm.ID)
	var in []int64
	for _, a := range answers {
		if a.Answer == store.AnswerIn {
			in = append(in, a.UserID)
		}
	}
	if len(in) == 0 {
		return
	}
	busy := map[int64]bool{}
	for _, d := range list {
		if d.UserID != 0 {
			busy[d.UserID] = true
		}
	}
	for _, d := range list {
		if d.UserID != 0 {
			continue
		}
		turns, last, _ := s.store.DutyTurns(ctx, d.DutyID)
		cands := append([]int64{}, in...)
		sort.SliceStable(cands, func(i, j int) bool {
			a, b := cands[i], cands[j]
			if busy[a] != busy[b] {
				return !busy[a]
			}
			if turns[a] != turns[b] {
				return turns[a] < turns[b]
			}
			return last[a] < last[b]
		})
		who := cands[0]
		if err := s.store.SetGameDuty(ctx, gm.ID, d.DutyID, who); err != nil {
			continue
		}
		busy[who] = true
		if tellThem {
			s.tell(ctx, g, []int64{who}, 0, note{kind: "reminders", title: g.Name + ": your turn — " + d.Name, body: "For " + whenText(gm.StartsAt) + ". Can't? Hand it to someone else on the game page.", link: s.gameLink(gm.ID)})
		}
	}
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
}

// reassignDuties gives the chores of someone who dropped out to someone else.
func (s *Server) reassignDuties(ctx context.Context, g *store.Group, gameID, uid int64) {
	list, err := s.store.GameDuties(ctx, g.ID, gameID)
	if err != nil {
		return
	}
	had := false
	for _, d := range list {
		if d.UserID == uid {
			_ = s.store.SetGameDuty(ctx, gameID, d.DutyID, 0)
			had = true
		}
	}
	if !had {
		return
	}
	if gm, err := s.store.GameByID(ctx, gameID); err == nil && gm.WasSent("day") {
		s.assignDuties(ctx, g, gm, true)
	}
}
