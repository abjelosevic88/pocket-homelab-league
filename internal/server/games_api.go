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

// ---- games ----

const tsLayout = "2006-01-02T15:04"

func validTS(v string) bool {
	_, err := time.ParseInLocation(tsLayout, v, time.Local)
	return err == nil
}

func parseTS(v string) time.Time {
	t, _ := time.ParseInLocation(tsLayout, v, time.Local)
	return t
}

// whenText is a game's time in words: "Tue 12 Nov, 20:00".
func whenText(ts string) string {
	t, err := time.ParseInLocation(tsLayout, ts, time.Local)
	if err != nil {
		return ts
	}
	return t.Format("Mon 2 Jan, 15:04")
}

// myGame loads a game of a group the person is in, with its group.
func (s *Server) myGame(w http.ResponseWriter, r *http.Request) (*store.Game, *store.Group, bool) {
	ctx := r.Context()
	uid := actorID(ctx)
	gm, err := s.store.GetGame(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "game")
		return nil, nil, false
	}
	g, err := s.store.GetGroup(ctx, uid, gm.GroupID)
	if err != nil {
		s.fail(w, err, "group")
		return nil, nil, false
	}
	return gm, g, true
}

// canEditGame: whoever planned it and the organisers.
func canEditGame(gm *store.Game, g *store.Group, uid int64) bool {
	return gm.UserID == uid || organiser(g)
}

// gameItem is a game in a list, with its group.
type gameItem struct {
	*store.Game
	GroupName  string `json:"group_name"`
	GroupColor string `json:"group_color"`
	Emoji      string `json:"emoji"`
	Sport      string `json:"sport"`
	Ended      bool   `json:"ended"`
}

func (s *Server) gameItems(ctx context.Context, uid int64, games []*store.Game) []gameItem {
	groups := map[int64]*store.Group{}
	out := make([]gameItem, 0, len(games))
	now := store.LocalNow()
	for _, gm := range games {
		g := groups[gm.GroupID]
		if g == nil {
			g, _ = s.store.GetGroup(ctx, uid, gm.GroupID)
			if g == nil {
				continue
			}
			groups[gm.GroupID] = g
		}
		end := ""
		if gm.StartsAt != "" {
			end = parseTS(gm.StartsAt).Add(time.Duration(gm.DurationMin) * time.Minute).Format(tsLayout)
		}
		out = append(out, gameItem{Game: gm, GroupName: g.Name, GroupColor: g.Color, Emoji: league.SportByKey(g.Sport).Emoji,
			Sport: league.DisplayName(g.Sport, g.SportName), Ended: end != "" && end < now})
	}
	return out
}

func (s *Server) handleListGames(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := r.URL.Query()
	f := store.GameFilter{Upcoming: queryBool(r, "upcoming"), Past: queryBool(r, "past"), From: q.Get("from"), To: q.Get("to"), Limit: queryInt(r, "limit")}
	if gid := queryInt(r, "group"); gid > 0 {
		f.GroupID = int64(gid)
	}
	if f.From != "" && len(f.From) == 10 {
		f.From += "T00:00"
	}
	if f.To != "" && len(f.To) == 10 {
		f.To += "T00:00"
	}
	games, err := s.store.ListGames(ctx, uid, f)
	if err != nil {
		s.fail(w, err, "games")
		return
	}
	writeJSON(w, http.StatusOK, s.gameItems(ctx, uid, games))
}

func (s *Server) handleUpcoming(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	games, err := s.store.ListGames(ctx, uid, store.GameFilter{Upcoming: true, Limit: 50})
	if err != nil {
		s.fail(w, err, "games")
		return
	}
	writeJSON(w, http.StatusOK, s.gameItems(ctx, uid, games))
}

// person is someone shown on a game page.
type person struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Answer string `json:"answer"`
	Side   string `json:"side"`
	Note   string `json:"note"`
	At     string `json:"answered_at"`
	Rating int    `json:"rating"`
}

// handleGetGame is everything about one game.
func (s *Server) handleGetGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

func (s *Server) gameDetail(ctx context.Context, gm *store.Game, g *store.Group) map[string]any {
	uid := actorID(ctx)
	ms, _ := s.store.Members(ctx, g.ID)
	names := map[int64]string{}
	for _, m := range ms {
		names[m.UserID] = m.Display()
	}
	answers, _ := s.store.Answers(ctx, gm.ID)
	ratings := s.ratings(ctx, g)
	people := []person{}
	answered := map[int64]bool{}
	for _, a := range answers {
		answered[a.UserID] = true
		people = append(people, person{UserID: a.UserID, Name: firstNonEmpty(names[a.UserID], "Former player"), Answer: a.Answer, Side: a.Side, Note: a.Note, At: a.AnsweredAt, Rating: int(ratings[a.UserID])})
	}
	for _, m := range ms {
		if !answered[m.UserID] {
			people = append(people, person{UserID: m.UserID, Name: m.Display(), Rating: int(ratings[m.UserID])})
		}
	}
	opts, _ := s.store.Options(ctx, gm.ID)
	matches, _ := s.store.ListMatches(ctx, uid, store.MatchFilter{GameID: gm.ID, Oldest: true})
	rules := league.RulesOf(g)
	mviews := make([]matchView, 0, len(matches))
	for _, m := range matches {
		mviews = append(mviews, s.viewMatch(rules, m, names, uid, g))
	}
	votes, _ := s.store.MVPVotes(ctx, gm.ID)
	tally := map[int64]int{}
	for _, f := range votes {
		tally[f]++
	}
	made, _ := s.store.MadeFor(ctx, uid, gm.ID)
	item := s.gameItems(ctx, uid, []*store.Game{gm})
	out := map[string]any{
		"game": item[0], "group": viewGroup(g), "people": people, "options": opts, "matches": mviews,
		"mvp":      map[string]any{"mine": votes[uid], "tally": tally, "open": len(matches) > 0 || item[0].Ended},
		"can_edit": canEditGame(gm, g, uid), "made": made, "share_cost": shareCost(gm),
	}
	out["venues"], _ = s.store.Venues(ctx, g.ID)
	return out
}

// shareCost is each player's share of what the game costs.
func shareCost(gm *store.Game) int64 {
	if gm.CostCents <= 0 || gm.In <= 0 {
		return 0
	}
	return (gm.CostCents + int64(gm.In) - 1) / int64(gm.In)
}

// gameInput is what a person may set on a game (nil = not sent).
type gameInput struct {
	StartsAt     *string    `json:"starts_at"`
	DurationMin  *int       `json:"duration_min"`
	Title        *string    `json:"title"`
	Notes        *string    `json:"notes"`
	VenueName    *string    `json:"venue_name"`
	VenueAddress *string    `json:"venue_address"`
	VenueLat     *float64   `json:"venue_lat"`
	VenueLon     *float64   `json:"venue_lon"`
	MinPlayers   *int       `json:"min_players"`
	MaxPlayers   *int       `json:"max_players"`
	CostCents    *int64     `json:"cost_cents"`
	Currency     *string    `json:"currency"`
	VoteUntil    *string    `json:"vote_until"`
	Options      []optionIn `json:"options"` // times (and places) to vote on
	ImIn         bool       `json:"im_in"`
	BookingTask  *bool      `json:"booking_task"`
}

func (in gameInput) apply(gm *store.Game, keys map[string]bool) string {
	if in.StartsAt != nil {
		if *in.StartsAt != "" && !validTS(*in.StartsAt) {
			return "the time is a date and a time"
		}
		gm.StartsAt = *in.StartsAt
	}
	if in.DurationMin != nil {
		if *in.DurationMin < 10 || *in.DurationMin > 720 {
			return "a game lasts 10 minutes to 12 hours"
		}
		gm.DurationMin = *in.DurationMin
	}
	trimP(&gm.Title, in.Title)
	trimP(&gm.Notes, in.Notes)
	trimP(&gm.VenueName, in.VenueName)
	trimP(&gm.VenueAddress, in.VenueAddress)
	gm.Title, gm.Notes, gm.VenueName, gm.VenueAddress = clip(gm.Title, 100), clip(gm.Notes, 2000), clip(gm.VenueName, 120), clip(gm.VenueAddress, 300)
	if keys["venue_lat"] || keys["venue_lon"] {
		gm.VenueLat, gm.VenueLon = in.VenueLat, in.VenueLon
		if (gm.VenueLat == nil) != (gm.VenueLon == nil) {
			return "the venue's position is a latitude and a longitude"
		}
	}
	if in.MinPlayers != nil {
		if *in.MinPlayers < 1 || *in.MinPlayers > 200 {
			return "a game needs 1 to 200 players"
		}
		gm.MinPlayers = *in.MinPlayers
	}
	if in.MaxPlayers != nil {
		if *in.MaxPlayers < 0 || *in.MaxPlayers > 200 {
			return "at most 200 players (0 = no limit)"
		}
		gm.MaxPlayers = *in.MaxPlayers
	}
	if gm.MaxPlayers > 0 && gm.MaxPlayers < gm.MinPlayers {
		return "the most players can't be fewer than the fewest"
	}
	if in.CostCents != nil {
		if *in.CostCents < 0 || *in.CostCents > 100_000_00 {
			return "the cost is 0 or more"
		}
		gm.CostCents = *in.CostCents
	}
	if in.Currency != nil {
		c := cleanCurrency(*in.Currency)
		if c == "" {
			return "the currency is a three-letter code like EUR"
		}
		gm.Currency = c
	}
	if in.VoteUntil != nil {
		if *in.VoteUntil != "" && !validTS(*in.VoteUntil) {
			return "answer by: a date and a time"
		}
		gm.VoteUntil = *in.VoteUntil
	}
	return ""
}

// optionIn is a time to vote on — "2026-11-10T20:00" — or a time at a
// place: {"starts_at": …, "venue_name": "Mojmilo", "venue_address": …}.
type optionIn store.OptionIn

func (o *optionIn) UnmarshalJSON(b []byte) error {
	var t string
	if err := json.Unmarshal(b, &t); err == nil {
		*o = optionIn{StartsAt: t}
		return nil
	}
	var x store.OptionIn
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	*o = optionIn(x)
	return nil
}

// cleanOptions keeps the valid, distinct options (a time, or a time at a
// place), in order.
func cleanOptions(in []optionIn) ([]store.OptionIn, string) {
	seen := map[string]bool{}
	out := []store.OptionIn{}
	for _, x := range in {
		o := store.OptionIn(x)
		o.StartsAt = strings.TrimSpace(o.StartsAt)
		o.VenueName, o.VenueAddress = clip(strings.TrimSpace(o.VenueName), 120), clip(strings.TrimSpace(o.VenueAddress), 300)
		if o.StartsAt == "" || seen[o.Key()] {
			continue
		}
		if !validTS(o.StartsAt) {
			return nil, "every time to vote on is a date and a time"
		}
		if (o.VenueLat == nil) != (o.VenueLon == nil) || o.VenueName == "" {
			o.VenueLat, o.VenueLon = nil, nil
		}
		seen[o.Key()] = true
		out = append(out, o)
	}
	if len(out) > 12 {
		return nil, "offer at most 12 times and places"
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartsAt != out[j].StartsAt {
			return out[i].StartsAt < out[j].StartsAt
		}
		return out[i].VenueName < out[j].VenueName
	})
	return out, ""
}

// fillVenues completes places given by name only from the group's list
// (the address and the position for the weather and the map).
func (s *Server) fillVenues(ctx context.Context, gid int64, gm *store.Game, opts []store.OptionIn) {
	vs, _ := s.store.Venues(ctx, gid)
	find := func(name string) *store.Venue {
		for i := range vs {
			if strings.EqualFold(vs[i].Name, strings.TrimSpace(name)) {
				return &vs[i]
			}
		}
		return nil
	}
	if gm != nil && gm.VenueName != "" && gm.VenueAddress == "" && gm.VenueLat == nil {
		if v := find(gm.VenueName); v != nil {
			gm.VenueAddress, gm.VenueLat, gm.VenueLon = v.Address, v.Lat, v.Lon
		}
	}
	for i := range opts {
		if opts[i].VenueName != "" && opts[i].VenueAddress == "" && opts[i].VenueLat == nil {
			if v := find(opts[i].VenueName); v != nil {
				opts[i].VenueAddress, opts[i].VenueLat, opts[i].VenueLon = v.Address, v.Lat, v.Lon
			}
		}
	}
}

// rememberVenues keeps the places a game uses in its group's list.
func (s *Server) rememberVenues(ctx context.Context, gid int64, gm *store.Game, opts []store.OptionIn) {
	if gm != nil && gm.VenueName != "" {
		_ = s.store.EnsureVenue(ctx, gid, store.Venue{Name: gm.VenueName, Address: gm.VenueAddress, Lat: gm.VenueLat, Lon: gm.VenueLon})
	}
	for _, o := range opts {
		if o.VenueName != "" {
			_ = s.store.EnsureVenue(ctx, gid, store.Venue{Name: o.VenueName, Address: o.VenueAddress, Lat: o.VenueLat, Lon: o.VenueLon})
		}
	}
}

func (s *Server) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	u := userFrom(ctx)
	var in gameInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	gm := &store.Game{GroupID: g.ID, Status: store.GamePlanned, DurationMin: g.DurationMin, VenueName: g.VenueName, VenueAddress: g.VenueAddress,
		VenueLat: g.VenueLat, VenueLon: g.VenueLon, MinPlayers: g.MinPlayers, MaxPlayers: g.MaxPlayers, CostCents: g.CostCents, Currency: g.Currency}
	if msg := in.apply(gm, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	opts, msg := cleanOptions(in.Options)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if keys["venue_name"] && !keys["venue_address"] && !keys["venue_lat"] && !strings.EqualFold(gm.VenueName, g.VenueName) {
		gm.VenueAddress, gm.VenueLat, gm.VenueLon = "", nil, nil
	}
	s.fillVenues(ctx, g.ID, gm, opts)
	switch {
	case len(opts) >= 2:
		gm.Status, gm.StartsAt = store.GameVoting, ""
	case len(opts) == 1:
		gm.StartsAt = opts[0].StartsAt
		if opts[0].VenueName != "" {
			gm.VenueName, gm.VenueAddress, gm.VenueLat, gm.VenueLon = opts[0].VenueName, opts[0].VenueAddress, opts[0].VenueLat, opts[0].VenueLon
		}
	}
	if gm.Status == store.GamePlanned && gm.StartsAt == "" {
		writeErr(w, http.StatusBadRequest, "say when the game is, or offer a few times to vote on")
		return
	}
	if err := s.store.CreateGame(ctx, u.ID, gm); err != nil {
		s.fail(w, err, "create game")
		return
	}
	if gm.Status == store.GameVoting {
		if err := s.store.SetOptions(ctx, gm.ID, opts); err != nil {
			s.fail(w, err, "times")
			return
		}
	}
	s.rememberVenues(ctx, g.ID, gm, opts)
	if in.ImIn && gm.Status == store.GamePlanned {
		_ = s.store.SetAnswer(ctx, gm.ID, u.ID, store.AnswerIn, "")
	}
	gm, _ = s.store.GetGame(ctx, u.ID, gm.ID)
	s.announceGame(ctx, g, gm, u, "planned")
	st, _ := s.store.GetSettings(ctx, u.ID)
	if gm.Status == store.GamePlanned && ((in.BookingTask != nil && *in.BookingTask) || (in.BookingTask == nil && st.BookingTask && gm.CostCents > 0)) {
		go s.bookingTask(u.ID, g, gm)
	}
	if in.ImIn {
		s.syncCalendarLater(gm.ID, false)
	}
	writeJSON(w, http.StatusCreated, s.gameDetail(ctx, gm, g))
}

// announceGame tells the group about a new, moved or cancelled game.
func (s *Server) announceGame(ctx context.Context, g *store.Group, gm *store.Game, by *store.User, what string) {
	who := "League"
	byID := int64(0)
	if by != nil {
		who, byID = firstNonEmpty(by.Name, by.Email), by.ID
	}
	where := firstNonEmpty(gm.VenueName, g.VenueName)
	at := ""
	if where != "" {
		at = " at " + where
	}
	var text, title, body, topic string
	switch what {
	case "planned":
		if gm.Status == store.GameVoting {
			text = who + " wants to find a time for the next game — vote on the times that suit you"
			title, body, topic = g.Name+": when can you play?", who+" offered a few times for the next game. Say which suit you.", "league.vote_started"
			if places := s.optionPlaces(ctx, gm.ID); len(places) > 1 {
				text = who + " wants to find a time and a place for the next game (" + strings.Join(places, " or ") + ") — vote on what suits you"
				title, body = g.Name+": when and where?", who+" offered a few times at "+strings.Join(places, " or ")+". Say which suit you."
			}
		} else {
			text = who + " planned a game: " + whenText(gm.StartsAt) + at
			if gm.Auto {
				text = "The usual game is on: " + whenText(gm.StartsAt) + at
			}
			title, body, topic = g.Name+": "+whenText(gm.StartsAt), firstNonEmpty(strings.TrimSpace(sportLine(g)+at), g.Name)+". Are you in?", "league.game_planned"
		}
	case "moved":
		text = who + " moved the game to " + whenText(gm.StartsAt) + at
		title, body, topic = g.Name+": moved to "+whenText(gm.StartsAt), "The game is now "+whenText(gm.StartsAt)+at+".", "league.game_changed"
	case "cancelled":
		text = who + " cancelled the game of " + whenText(gm.StartsAt)
		if gm.CancelReason != "" {
			text += ": " + gm.CancelReason
		}
		title, body, topic = g.Name+": "+whenText(gm.StartsAt)+" is off", firstNonEmpty(gm.CancelReason, "The game is cancelled."), "league.game_cancelled"
	}
	s.systemMessage(ctx, g, 0, "game_"+what, text, map[string]any{"game_id": gm.ID})
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
	n := note{kind: "plans", title: title, body: body, link: s.gameLink(gm.ID), button: "Am I in?"}
	if what == "cancelled" {
		n.button = "Open the group"
	}
	s.tellGroup(ctx, g, byID, n)
	s.publishGroup(ctx, g, topic, fmt.Sprintf("game:%d:%s", gm.ID, what), map[string]any{"game_id": gm.ID, "group_id": g.ID, "group": g.Name,
		"sport": g.Sport, "starts_at": gm.StartsAt, "venue": where, "status": gm.Status, "url": s.gameLink(gm.ID)})
}

func (s *Server) handleUpdateGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	if !canEditGame(gm, g, u.ID) {
		writeErr(w, http.StatusForbidden, "only whoever planned the game and the organisers can change it")
		return
	}
	var in gameInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	before := *gm
	if msg := in.apply(gm, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if keys["venue_name"] && !keys["venue_address"] && !keys["venue_lat"] && before.VenueName != gm.VenueName {
		gm.VenueAddress, gm.VenueLat, gm.VenueLon = "", nil, nil
	}
	s.fillVenues(ctx, g.ID, gm, nil)
	if gm.Status == store.GamePlanned && gm.StartsAt == "" {
		writeErr(w, http.StatusBadRequest, "a planned game needs a time")
		return
	}
	if in.Options != nil && gm.Status == store.GameVoting {
		opts, msg := cleanOptions(in.Options)
		if msg != "" || len(opts) < 2 {
			writeErr(w, http.StatusBadRequest, firstNonEmpty(msg, "offer at least two times"))
			return
		}
		s.fillVenues(ctx, g.ID, nil, opts)
		if err := s.store.SetOptions(ctx, gm.ID, opts); err != nil {
			s.fail(w, err, "times")
			return
		}
		s.rememberVenues(ctx, g.ID, nil, opts)
	}
	if err := s.store.UpdateGame(ctx, gm); err != nil {
		s.fail(w, err, "save game")
		return
	}
	s.rememberVenues(ctx, g.ID, gm, nil)
	moved := gm.Status == store.GamePlanned && (before.StartsAt != gm.StartsAt || before.VenueName != gm.VenueName)
	if before.StartsAt != gm.StartsAt {
		_ = s.store.ClearSent(ctx, gm.ID)
	}
	gm, _ = s.store.GetGame(ctx, u.ID, gm.ID)
	if moved {
		s.announceGame(ctx, g, gm, u, "moved")
	} else {
		s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
	}
	s.syncCalendarLater(gm.ID, false)
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

func (s *Server) handleCancelGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	if !canEditGame(gm, g, u.ID) {
		writeErr(w, http.StatusForbidden, "only whoever planned the game and the organisers can cancel it")
		return
	}
	var in struct {
		Reason string `json:"reason"`
		Undo   bool   `json:"undo"`
	}
	_ = decode(r, &in)
	if in.Undo {
		if gm.Status != store.GameCancelled {
			writeErr(w, http.StatusBadRequest, "the game is not cancelled")
			return
		}
		gm.Status, gm.CancelReason = store.GamePlanned, ""
		if gm.StartsAt == "" {
			gm.Status = store.GameVoting
		}
	} else {
		gm.Status, gm.CancelReason = store.GameCancelled, clip(strings.TrimSpace(in.Reason), 300)
	}
	if err := s.store.UpdateGame(ctx, gm); err != nil {
		s.fail(w, err, "cancel game")
		return
	}
	gm, _ = s.store.GetGame(ctx, u.ID, gm.ID)
	if in.Undo {
		s.announceGame(ctx, g, gm, u, "planned")
	} else {
		s.announceGame(ctx, g, gm, u, "cancelled")
	}
	s.syncCalendarLater(gm.ID, false)
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

func (s *Server) handleDeleteGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	if !canEditGame(gm, g, actorID(ctx)) {
		writeErr(w, http.StatusForbidden, "only whoever planned the game and the organisers can delete it")
		return
	}
	s.dropCalendar(ctx, gm.ID)
	if err := s.store.DeleteGame(ctx, gm.ID); err != nil {
		s.fail(w, err, "delete game")
		return
	}
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID, "deleted": true})
	w.WriteHeader(http.StatusNoContent)
}

// ---- are you coming? ----

// handleAnswer saves in / maybe / out (organisers may answer for someone).
// A full game puts a yes on the waiting list; when someone drops out, the
// first one waiting moves up and is told.
func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	var in struct {
		Answer string `json:"answer"`
		Note   string `json:"note"`
		UserID int64  `json:"user_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	who := u.ID
	if in.UserID != 0 && in.UserID != u.ID {
		if !organiser(g) || linkApp(ctx) != "" {
			writeErr(w, http.StatusForbidden, "only organisers can answer for someone else")
			return
		}
		if _, err := s.store.GetMember(ctx, g.ID, in.UserID); err != nil {
			s.fail(w, err, "member")
			return
		}
		who = in.UserID
	}
	if gm.Status != store.GamePlanned {
		writeErr(w, http.StatusBadRequest, map[string]string{store.GameVoting: "vote on the times first", store.GameCancelled: "the game is cancelled"}[gm.Status])
		return
	}
	if !oneOf(in.Answer, []string{store.AnswerIn, store.AnswerMaybe, store.AnswerOut, ""}) {
		writeErr(w, http.StatusBadRequest, "answer in, maybe or out")
		return
	}
	res, err := s.answer(ctx, g, gm, who, in.Answer, clip(strings.TrimSpace(in.Note), 120))
	if err != nil {
		s.fail(w, err, "answer")
		return
	}
	gm, _ = s.store.GetGame(ctx, u.ID, gm.ID)
	out := s.gameDetail(ctx, gm, g)
	out["result"] = res
	writeJSON(w, http.StatusOK, out)
}

// answer applies one answer and what follows from it; it returns what the
// person's answer became (in, waiting …).
func (s *Server) answer(ctx context.Context, g *store.Group, gm *store.Game, uid int64, answer, note string) (string, error) {
	before := s.store.GetAnswer(ctx, gm.ID, uid)
	if answer == "" {
		if err := s.store.ClearAnswer(ctx, gm.ID, uid); err != nil {
			return "", err
		}
	} else {
		if answer == store.AnswerIn && before != store.AnswerIn && gm.MaxPlayers > 0 && gm.In >= gm.MaxPlayers {
			answer = store.AnswerWaiting
		}
		if answer == store.AnswerIn && before == store.AnswerWaiting && gm.MaxPlayers > 0 && gm.In >= gm.MaxPlayers {
			answer = store.AnswerWaiting
		}
		if err := s.store.SetAnswer(ctx, gm.ID, uid, answer, note); err != nil {
			return "", err
		}
	}
	// a place came free: the first one waiting moves up
	if before == store.AnswerIn && answer != store.AnswerIn {
		s.moveUp(ctx, g, gm.ID)
	}
	s.toGroup(ctx, g.ID, "answer", map[string]any{"game_id": gm.ID, "group_id": g.ID, "user_id": uid, "answer": answer})
	if before != answer {
		s.publishFor(ctx, uid, "league.answered", fmt.Sprintf("answer:%d:%d:%s", gm.ID, uid, answer), map[string]any{"game_id": gm.ID, "group": g.Name,
			"starts_at": gm.StartsAt, "answer": answer, "url": s.gameLink(gm.ID)})
		s.syncCalendarLater(gm.ID, false)
	}
	return answer, nil
}

// moveUp puts the first person waiting in, and tells them.
func (s *Server) moveUp(ctx context.Context, g *store.Group, gameID int64) {
	gm, err := s.store.GameByID(ctx, gameID)
	if err != nil || (gm.MaxPlayers > 0 && gm.In >= gm.MaxPlayers) {
		return
	}
	answers, _ := s.store.Answers(ctx, gameID)
	for _, a := range answers {
		if a.Answer != store.AnswerWaiting {
			continue
		}
		if err := s.store.SetAnswer(ctx, gameID, a.UserID, store.AnswerIn, a.Note); err != nil {
			return
		}
		name := s.nameIn(ctx, g.ID, a.UserID)
		s.systemMessage(ctx, g, gameID, "moved_up", name+" moved up from the waiting list", map[string]any{"user_id": a.UserID, "game_id": gameID})
		s.tell(ctx, g, []int64{a.UserID}, 0, note{kind: "plans", title: "You're in: " + g.Name + ", " + whenText(gm.StartsAt),
			body: "A place came free and you were first on the waiting list. Can't make it any more? Say so, and the next one moves up.", link: s.gameLink(gameID)})
		s.syncCalendarLater(gameID, false)
		return
	}
}

// nameIn is how someone is called in a group.
func (s *Server) nameIn(ctx context.Context, gid, uid int64) string {
	if m, err := s.store.GetMember(ctx, gid, uid); err == nil {
		return m.Display()
	}
	if u, err := s.store.GetUser(ctx, uid); err == nil {
		return firstNonEmpty(u.Name, "Someone")
	}
	return "Someone"
}

// ---- finding a time ----

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	if gm.Status != store.GameVoting {
		writeErr(w, http.StatusBadRequest, "the time is already chosen")
		return
	}
	var in struct {
		Votes map[string]string `json:"votes"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	votes := map[int64]string{}
	for k, v := range in.Votes {
		var id int64
		if _, err := fmt.Sscan(k, &id); err != nil || !oneOf(v, []string{"yes", "maybe", "no", ""}) {
			writeErr(w, http.StatusBadRequest, "vote yes, maybe or no")
			return
		}
		votes[id] = v
	}
	if err := s.store.SetVotes(ctx, gm.ID, actorID(ctx), votes); err != nil {
		s.fail(w, err, "vote")
		return
	}
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

// handlePick chooses one of the times: the game is planned for it, and
// whoever said yes to that time is in (maybe stays maybe).
func (s *Server) handlePick(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	if !canEditGame(gm, g, u.ID) {
		writeErr(w, http.StatusForbidden, "only whoever planned the game and the organisers pick the time")
		return
	}
	var in struct {
		OptionID int64 `json:"option_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if gm.Status != store.GameVoting {
		writeErr(w, http.StatusBadRequest, "the time is already chosen")
		return
	}
	if err := s.pickTime(ctx, g, gm, in.OptionID, u); err != nil {
		s.fail(w, err, "pick a time")
		return
	}
	gm, _ = s.store.GetGame(ctx, u.ID, gm.ID)
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

func (s *Server) pickTime(ctx context.Context, g *store.Group, gm *store.Game, optionID int64, by *store.User) error {
	opts, err := s.store.Options(ctx, gm.ID)
	if err != nil {
		return err
	}
	var pick *store.Option
	for _, o := range opts {
		if o.ID == optionID {
			pick = o
		}
	}
	if pick == nil {
		return store.ErrNotFound
	}
	gm.Status, gm.StartsAt = store.GamePlanned, pick.StartsAt
	if pick.VenueName != "" {
		gm.VenueName, gm.VenueAddress, gm.VenueLat, gm.VenueLon = pick.VenueName, pick.VenueAddress, pick.VenueLat, pick.VenueLon
	}
	if err := s.store.UpdateGame(ctx, gm); err != nil {
		return err
	}
	for uidS, v := range pick.Votes {
		var uid int64
		_, _ = fmt.Sscan(uidS, &uid)
		switch v {
		case "yes":
			_ = s.store.SetAnswer(ctx, gm.ID, uid, store.AnswerIn, "")
		case "maybe":
			_ = s.store.SetAnswer(ctx, gm.ID, uid, store.AnswerMaybe, "")
		case "no":
			_ = s.store.SetAnswer(ctx, gm.ID, uid, store.AnswerOut, "")
		}
	}
	gm, _ = s.store.GameByID(ctx, gm.ID)
	s.announceGame(ctx, g, gm, by, "planned")
	s.syncCalendarLater(gm.ID, false)
	return nil
}

// optionPlaces are the different places offered in a vote.
func (s *Server) optionPlaces(ctx context.Context, gameID int64) []string {
	opts, _ := s.store.Options(ctx, gameID)
	seen := map[string]bool{}
	out := []string{}
	for _, o := range opts {
		if o.VenueName != "" && !seen[strings.ToLower(o.VenueName)] {
			seen[strings.ToLower(o.VenueName)] = true
			out = append(out, o.VenueName)
		}
	}
	return out
}

// bestOption is the time most people can make (yes counts more than maybe).
func bestOption(opts []*store.Option) *store.Option {
	var best *store.Option
	bestScore := -1.0
	for _, o := range opts {
		sc := 0.0
		for _, v := range o.Votes {
			switch v {
			case "yes":
				sc += 1
			case "maybe":
				sc += 0.4
			}
		}
		if sc > bestScore {
			best, bestScore = o, sc
		}
	}
	return best
}

// ---- teams for the day ----

func (s *Server) handleTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	var in struct {
		Mode  string            `json:"mode"` // fair | shuffle | clear | set
		Seed  int64             `json:"seed"`
		Sides map[string]string `json:"sides"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	answers, _ := s.store.Answers(ctx, gm.ID)
	var in_ []int64
	for _, a := range answers {
		if a.Answer == store.AnswerIn {
			in_ = append(in_, a.UserID)
		}
	}
	sides := map[int64]string{}
	diff := 0.0
	switch in.Mode {
	case "clear":
	case "set":
		for k, v := range in.Sides {
			var uid int64
			if _, err := fmt.Sscan(k, &uid); err != nil || (v != "a" && v != "b" && v != "") {
				writeErr(w, http.StatusBadRequest, "a side is a or b")
				return
			}
			sides[uid] = v
		}
	default:
		if len(in_) < 2 {
			writeErr(w, http.StatusBadRequest, "at least two people need to be in")
			return
		}
		seed := in.Seed
		if in.Mode == "shuffle" && seed == 0 {
			seed = time.Now().UnixNano()
		}
		a, b, d := league.FairTeams(in_, s.ratings(ctx, g), seed)
		for _, id := range a {
			sides[id] = "a"
		}
		for _, id := range b {
			sides[id] = "b"
		}
		diff = d
	}
	if err := s.store.SetSides(ctx, gm.ID, sides); err != nil {
		s.fail(w, err, "teams")
		return
	}
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
	out := s.gameDetail(ctx, gm, g)
	out["rating_gap"] = diff
	writeJSON(w, http.StatusOK, out)
}

// ---- player of the game ----

func (s *Server) handleMVP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	uid := actorID(ctx)
	var in struct {
		For int64 `json:"for"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.For == uid {
		writeErr(w, http.StatusBadRequest, "vote for someone else")
		return
	}
	if in.For != 0 {
		if a := s.store.GetAnswer(ctx, gm.ID, in.For); a != store.AnswerIn && !s.playedIn(ctx, gm.ID, in.For) {
			writeErr(w, http.StatusBadRequest, "vote for someone who played")
			return
		}
	}
	if err := s.store.SetMVPVote(ctx, gm.ID, uid, in.For); err != nil {
		s.fail(w, err, "vote")
		return
	}
	s.toGroup(ctx, g.ID, "game", map[string]any{"id": gm.ID, "group_id": g.ID})
	writeJSON(w, http.StatusOK, s.gameDetail(ctx, gm, g))
}

func (s *Server) playedIn(ctx context.Context, gameID, uid int64) bool {
	ms, _ := s.store.ListMatches(ctx, 0, store.MatchFilter{GameID: gameID, UserID: uid, Limit: 1})
	return len(ms) > 0
}

// ---- calendar files ----

func icsEscape(v string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)
	return r.Replace(v)
}

func (s *Server) icsEvent(b *strings.Builder, g *store.Group, gm *store.Game, desc string) {
	start := parseTS(gm.StartsAt)
	end := start.Add(time.Duration(gm.DurationMin) * time.Minute)
	f := "20060102T150405Z"
	title := firstNonEmpty(gm.Title, sportLine(g)+": "+g.Name)
	if gm.Status == store.GameCancelled {
		title = "Cancelled: " + title
	}
	fmt.Fprintf(b, "BEGIN:VEVENT\r\nUID:league-game-%d@pocket-league\r\nDTSTAMP:%s\r\nDTSTART:%s\r\nDTEND:%s\r\nSUMMARY:%s\r\n",
		gm.ID, time.Now().UTC().Format(f), start.UTC().Format(f), end.UTC().Format(f), icsEscape(title))
	if loc := joinNonEmpty(", ", firstNonEmpty(gm.VenueName, g.VenueName), firstNonEmpty(gm.VenueAddress, g.VenueAddress)); loc != "" {
		fmt.Fprintf(b, "LOCATION:%s\r\n", icsEscape(loc))
	}
	if desc != "" {
		fmt.Fprintf(b, "DESCRIPTION:%s\r\n", icsEscape(desc))
	}
	if s.cfg.BaseURL != "" {
		fmt.Fprintf(b, "URL:%s\r\n", s.gameLink(gm.ID))
	}
	if gm.Status == store.GameCancelled {
		b.WriteString("STATUS:CANCELLED\r\n")
	}
	b.WriteString("END:VEVENT\r\n")
}

func (s *Server) handleGameICS(w http.ResponseWriter, r *http.Request) {
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	if gm.StartsAt == "" {
		writeErr(w, http.StatusBadRequest, "the time is not chosen yet")
		return
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Pocket League//EN\r\nMETHOD:PUBLISH\r\n")
	s.icsEvent(&b, g, gm, firstNonEmpty(gm.Notes, g.Name))
	b.WriteString("END:VCALENDAR\r\n")
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="game-%d.ics"`, gm.ID))
	_, _ = w.Write([]byte(b.String()))
}

// handleFeed is a group's games as a calendar to subscribe to (Apple,
// Google, Outlook): the feed token in the address is the only key, and it
// belongs to one person in one group.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tok := strings.TrimSuffix(chiParam(r, "token"), ".ics")
	m, err := s.store.MemberByFeed(ctx, tok)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	g, err := s.store.GetGroup(ctx, m.UserID, m.GroupID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	from := time.Now().AddDate(0, 0, -60).Format(tsLayout)
	games, _ := s.store.ListGames(ctx, m.UserID, store.GameFilter{GroupID: g.ID, From: from})
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Pocket League//EN\r\nX-WR-CALNAME:" + icsEscape(g.Name) + "\r\n")
	for _, gm := range games {
		if gm.StartsAt == "" {
			continue
		}
		desc := fmt.Sprintf("%d in", gm.In)
		if gm.MyAnswer != "" {
			desc += " · you: " + gm.MyAnswer
		}
		s.icsEvent(&b, g, gm, desc)
	}
	b.WriteString("END:VCALENDAR\r\n")
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
