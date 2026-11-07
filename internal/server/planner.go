package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- the planner ----
//
// Every minute: the usual weekly game is planned a few days ahead (when the
// group wants that); two days before a game the people who did not answer
// are asked; the day before, the organisers hear when the game is short of
// players and everyone who is in gets a reminder (with the weather for
// outdoor games), then another a few hours before (each person chooses how
// many); a vote with an "answer by" closes itself on the best time; after
// the game, the players are asked for the score; results nobody questioned
// for two days are confirmed.

func (s *Server) plan(ctx context.Context) {
	if !s.planMu.TryLock() {
		return
	}
	defer s.planMu.Unlock()
	now := time.Now()
	s.planUsualGames(ctx, now)
	s.remind(ctx, now)
	if ids, err := s.store.AutoConfirm(ctx, now.Add(-48*time.Hour).UTC().Format(time.RFC3339Nano)); err == nil && len(ids) > 0 {
		s.log.Debug("results confirmed by themselves", "count", len(ids))
	}
}

// isoWeekday is 1 = Monday … 7 = Sunday.
func isoWeekday(t time.Time) int {
	if wd := int(t.Weekday()); wd != 0 {
		return wd
	}
	return 7
}

// nextUsual is the group's next usual game within its planning window ("" = none).
func nextUsual(g *store.Group, now time.Time) string {
	if g.Weekday == 0 || g.Time == "" || g.PlanDays <= 0 || g.Archived {
		return ""
	}
	for d := 0; d <= g.PlanDays; d++ {
		day := now.AddDate(0, 0, d)
		if isoWeekday(day) != g.Weekday {
			continue
		}
		ts := day.Format("2006-01-02") + "T" + g.Time
		if parseTS(ts).After(now) {
			return ts
		}
	}
	return ""
}

func (s *Server) planUsualGames(ctx context.Context, now time.Time) {
	groups, err := s.store.AllGroups(ctx)
	if err != nil {
		return
	}
	for _, g := range groups {
		ts := nextUsual(g, now)
		if ts == "" || s.store.HasGameAt(ctx, g.ID, ts[:10]) || !s.once(ctx, fmt.Sprintf("auto:%d:%s", g.ID, ts[:10])) {
			continue
		}
		gm := &store.Game{GroupID: g.ID, Status: store.GamePlanned, StartsAt: ts, DurationMin: g.DurationMin, VenueName: g.VenueName, VenueAddress: g.VenueAddress,
			VenueLat: g.VenueLat, VenueLon: g.VenueLon, MinPlayers: g.MinPlayers, MaxPlayers: g.MaxPlayers, CostCents: g.CostCents, Currency: g.Currency, Auto: true}
		if err := s.store.CreateGame(ctx, g.UserID, gm); err != nil {
			s.log.Warn("planning the usual game", "group", g.ID, "err", err)
			continue
		}
		gm, _ = s.store.GameByID(ctx, gm.ID)
		s.announceGame(ctx, g, gm, nil, "planned")
		s.log.Info("usual game planned", "group", g.ID, "at", ts)
	}
}

func (s *Server) remind(ctx context.Context, now time.Time) {
	games, err := s.store.GamesToWatch(ctx)
	if err != nil {
		return
	}
	for _, gm := range games {
		g, err := s.store.GroupByID(ctx, gm.GroupID)
		if err != nil {
			continue
		}
		if gm.Status == store.GameVoting {
			s.closeVote(ctx, g, gm, now)
			continue
		}
		if gm.Status != store.GamePlanned || gm.StartsAt == "" {
			continue
		}
		start := parseTS(gm.StartsAt)
		until := start.Sub(now)
		end := start.Add(time.Duration(gm.DurationMin) * time.Minute)
		answers, _ := s.store.Answers(ctx, gm.ID)
		by := map[string][]int64{}
		answered := map[int64]bool{}
		for _, a := range answers {
			by[a.Answer] = append(by[a.Answer], a.UserID)
			answered[a.UserID] = true
		}
		members, _ := s.store.Members(ctx, g.ID)
		var silent, organisers []int64
		for _, m := range members {
			if !answered[m.UserID] {
				silent = append(silent, m.UserID)
			}
			if m.Role == "organiser" {
				organisers = append(organisers, m.UserID)
			}
		}
		link := s.gameLink(gm.ID)
		where := ""
		if gm.VenueName != "" {
			where = " at " + gm.VenueName
		}
		mark := func(what string) bool {
			if gm.WasSent(what) {
				return false
			}
			_ = s.store.MarkSent(ctx, gm.ID, what)
			gm.Sent += " " + what
			return true
		}
		// two days before: ask those who did not answer
		if until > 0 && until <= 48*time.Hour && len(silent) > 0 && mark("ask") {
			s.tell(ctx, g, silent, 0, note{kind: "plans", title: g.Name + ": are you in for " + whenText(gm.StartsAt) + "?",
				body: fmt.Sprintf("%d %s in so far%s. Tap to answer.", gm.In, plural(gm.In, "is", "are"), where), link: link, button: "Answer"})
		}
		// the day before: short of players?
		if until > 0 && until <= 24*time.Hour && gm.In < gm.MinPlayers && mark("short") {
			short := gm.MinPlayers - gm.In
			text := fmt.Sprintf("%d in, %d needed: %d short for %s%s.", gm.In, gm.MinPlayers, short, whenText(gm.StartsAt), where)
			s.tell(ctx, g, organisers, 0, note{kind: "short", title: fmt.Sprintf("%s: %d short", g.Name, short), body: text + " Ask around, or cancel in time.", link: link})
			ask := append(append([]int64{}, silent...), by[store.AnswerMaybe]...)
			s.tell(ctx, g, ask, 0, note{kind: "plans", title: fmt.Sprintf("%s needs %d more for %s", g.Name, short, whenText(gm.StartsAt)), body: text + " Can you play?", link: link, button: "I'm in"})
			s.systemMessage(ctx, g, gm.ID, "short", fmt.Sprintf("We're %d short for %s — who can play?", short, whenText(gm.StartsAt)), map[string]any{"game_id": gm.ID})
			s.publishGroup(ctx, g, "league.short", fmt.Sprintf("short:%d", gm.ID), map[string]any{"game_id": gm.ID, "group": g.Name, "starts_at": gm.StartsAt, "in": gm.In,
				"needed": gm.MinPlayers, "url": link})
		}
		// the day before: everyone who is in (or maybe)
		if until > 2*time.Hour && until <= 24*time.Hour && mark("day") {
			who := append(append([]int64{}, by[store.AnswerIn]...), by[store.AnswerMaybe]...)
			body := fmt.Sprintf("%s%s. %d in", whenText(gm.StartsAt), where, gm.In)
			if gm.MaxPlayers > 0 {
				body += fmt.Sprintf(" of %d", gm.MaxPlayers)
			}
			body += "."
			if wx := s.weatherLine(ctx, g, gm); wx != "" {
				body += "\n" + wx
			}
			s.tell(ctx, g, who, 0, note{kind: "reminders", title: g.Name + " " + dayWord(start, now) + " at " + start.Format("15:04"), body: body, link: link})
			s.publishGroup(ctx, g, "league.reminder", fmt.Sprintf("reminder:%d", gm.ID), map[string]any{"game_id": gm.ID, "group": g.Name, "starts_at": gm.StartsAt, "url": link})
		}
		// a few hours before, each at their own time
		if until > 0 && until <= 12*time.Hour {
			for _, uid := range by[store.AnswerIn] {
				st, err := s.store.GetSettings(ctx, uid)
				if err != nil || st.RemindHours <= 0 || until > time.Duration(st.RemindHours)*time.Hour || !mark("h"+itoa(uid)) {
					continue
				}
				s.tell(ctx, g, []int64{uid}, 0, note{kind: "reminders", title: g.Name + " at " + start.Format("15:04"),
					body: fmt.Sprintf("In %s%s. %d in.", untilWords(until), where, gm.In), link: link})
			}
		}
		// after the game: the score, please
		if now.After(end.Add(20*time.Minute)) && now.Before(end.Add(24*time.Hour)) && gm.Matches == 0 && gm.In > 0 && mark("result") {
			s.tell(ctx, g, by[store.AnswerIn], 0, note{kind: "results", title: g.Name + ": how did it go?", body: "Add the score, so the table keeps up.", link: link, button: "Add the score"})
		}
	}
}

// closeVote picks the best time when the vote's "answer by" has passed.
func (s *Server) closeVote(ctx context.Context, g *store.Group, gm *store.Game, now time.Time) {
	if gm.VoteUntil == "" || now.Before(parseTS(gm.VoteUntil)) || gm.WasSent("voteclose") {
		return
	}
	_ = s.store.MarkSent(ctx, gm.ID, "voteclose")
	opts, _ := s.store.Options(ctx, gm.ID)
	best := bestOption(opts)
	yes := 0
	if best != nil {
		for _, v := range best.Votes {
			if v == "yes" {
				yes++
			}
		}
	}
	if best == nil || yes == 0 {
		if gm.UserID != 0 {
			s.tell(ctx, g, []int64{gm.UserID}, 0, note{kind: "plans", title: g.Name + ": nobody picked a time", body: "The vote closed without a yes. Offer other times, or cancel it.", link: s.gameLink(gm.ID)})
		}
		return
	}
	if err := s.pickTime(ctx, g, gm, best.ID, nil); err != nil {
		s.log.Warn("closing a vote", "game", gm.ID, "err", err)
	}
}

// weatherLine is the forecast for an outdoor game in a line, read with the
// planner's (else the group maker's) Pocket Weather; "" when there is none.
func (s *Server) weatherLine(ctx context.Context, g *store.Group, gm *store.Game) string {
	if !g.Outdoor {
		return ""
	}
	uid := firstID(gm.UserID, g.UserID)
	sib, err := s.clients(ctx, uid).conn(ctx, appWeather, scopeWeatherRead)
	if err != nil {
		return ""
	}
	lat, lon := firstF(gm.VenueLat, g.VenueLat), firstF(gm.VenueLon, g.VenueLon)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	f, err := sib.client.ForecastAt(ctx, lat, lon)
	if err != nil {
		return ""
	}
	wx := summarizeWeather(f, parseTS(gm.StartsAt), gm.DurationMin)
	text, _ := wx["text"].(string)
	if w, ok := wx["warning"].(string); ok && w != "" {
		return "Weather: " + w + " (" + text + ")"
	}
	if text != "" {
		return "Weather: " + text
	}
	return ""
}

func firstID(v ...int64) int64 {
	for _, x := range v {
		if x != 0 {
			return x
		}
	}
	return 0
}

func dayWord(t, now time.Time) string {
	switch t.Format("2006-01-02") {
	case now.Format("2006-01-02"):
		return "today"
	case now.AddDate(0, 0, 1).Format("2006-01-02"):
		return "tomorrow"
	}
	return t.Format("Mon 2 Jan")
}

func untilWords(d time.Duration) string {
	h := int(d.Round(30*time.Minute) / time.Hour)
	m := int(d.Round(30*time.Minute)%time.Hour) / int(time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%d minutes", max(int(d/time.Minute), 1))
	case m == 0:
		return fmt.Sprintf("%d %s", h, plural(h, "hour", "hours"))
	}
	return strings.TrimSpace(fmt.Sprintf("%d½ hours", h))
}
