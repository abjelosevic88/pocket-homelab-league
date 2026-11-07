package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- League in the other Pocket apps ----
//
// Everything here is done as ONE person, with their own Core link or direct
// token: the games they are in go to their calendar, the matches they
// played to their health history and their habit, their share of the court
// to Pocket Money, the court itself to a Split group; the venue comes from
// their places and the players from their contacts. Nothing is needed for
// League itself: when an app is not there or not switched on, the page
// greys the option out and says what to switch on, and League keeps the
// game (with a calendar file to download instead).

// handleApps says per app whether it works for the person, and if not why
// and what to do.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

// handleAppOptions lists what to pick from in an app (for Settings).
func (s *Server) handleAppOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	type opt struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Note string `json:"note,omitempty"`
	}
	out := []opt{}
	fail := func(a pocketApp, err error) {
		if isNotConnected(err) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
	}
	switch chiParam(r, "what") {
	case "calendars":
		sib, err := c.conn(ctx, appCalendar, scopeCalendarRead)
		if err != nil {
			fail(appCalendar, err)
			return
		}
		cals, err := sib.client.Calendars(ctx)
		if err != nil {
			fail(appCalendar, err)
			return
		}
		for _, x := range cals {
			o := opt{ID: x.ID, Name: x.Name}
			if x.Default {
				o.Note = "default"
			}
			out = append(out, o)
		}
	case "habits":
		sib, err := c.conn(ctx, appHabits, scopeHabitsRead)
		if err != nil {
			fail(appHabits, err)
			return
		}
		hs, err := sib.client.Habits(ctx)
		if err != nil {
			fail(appHabits, err)
			return
		}
		for _, x := range hs {
			out = append(out, opt{ID: x.ID, Name: x.Name})
		}
	case "split_groups":
		sib, err := c.conn(ctx, appSplit, scopeGroupsRead)
		if err != nil {
			fail(appSplit, err)
			return
		}
		gs, err := sib.client.SplitGroups(ctx)
		if err != nil {
			fail(appSplit, err)
			return
		}
		for _, x := range gs {
			out = append(out, opt{ID: x.ID, Name: x.Name, Note: fmt.Sprintf("%d people · %s", len(x.Members), x.Currency)})
		}
	case "accounts":
		sib, err := c.conn(ctx, appMoney, scopeAccountsRead)
		if err != nil {
			fail(appMoney, err)
			return
		}
		as, err := sib.client.Accounts(ctx)
		if err != nil {
			fail(appMoney, err)
			return
		}
		for _, x := range as {
			out = append(out, opt{ID: x.ID, Name: x.Name, Note: x.Currency})
		}
	case "categories":
		sib, err := c.conn(ctx, appMoney, scopeAccountsRead)
		if err != nil {
			fail(appMoney, err)
			return
		}
		acc, _ := strconv.ParseInt(r.URL.Query().Get("account"), 10, 64)
		cs, err := sib.client.Categories(ctx, acc)
		if err != nil {
			fail(appMoney, err)
			return
		}
		for _, x := range cs {
			out = append(out, opt{ID: x.ID, Name: firstNonEmpty(x.FullName, x.Name)})
		}
	default:
		writeErr(w, http.StatusNotFound, "unknown list")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleContactSearch finds people in the person's Pocket Contacts (to add to a group).
func (s *Server) handleContactSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appContacts, scopeContactsRead)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cs, err := sib.client.SearchContacts(ctx, q, 12)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Contacts: "+err.Error())
		return
	}
	out := []map[string]any{}
	for _, c := range cs {
		email, phone := "", ""
		if len(c.Emails) > 0 {
			email = c.Emails[0]
		}
		if len(c.Phones) > 0 {
			phone = c.Phones[0]
		}
		out = append(out, map[string]any{"id": c.ID, "name": c.FN, "email": email, "phone": phone, "org": c.Org})
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePlaceSearch finds venues in the person's Pocket Places.
func (s *Server) handlePlaceSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appPlaces, scopePlacesRead)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ps, err := sib.client.SearchPlaces(ctx, q, 12)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Places: "+err.Error())
		return
	}
	out := []map[string]any{}
	for _, p := range ps {
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "address": joinNonEmpty(", ", p.Address, p.City), "lat": p.Lat, "lon": p.Lng, "kind": p.Kind})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSaveVenue saves a group's venue to the person's Pocket Places.
func (s *Server) handleSaveVenue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	if g.VenueName == "" {
		writeErr(w, http.StatusBadRequest, "the group has no venue yet")
		return
	}
	uid := actorID(ctx)
	sib, err := s.clients(ctx, uid).conn(ctx, appPlaces, scopePlacesWrite)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := sib.client.SavePlace(ctx, g.VenueName, g.VenueAddress, g.VenueLat, g.VenueLon, "Where "+g.Name+" plays ("+sportLine(g)+"). From Pocket League: "+s.groupLink(g.ID))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Places: "+err.Error())
		return
	}
	link := s.siblingLink(ctx, uid, appPlaces, fmt.Sprintf("/places/%d", id))
	if organiser(g) && g.VenueRef == "" {
		g.VenueRef = itoa(id)
		_ = s.store.UpdateGroup(ctx, g)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "url": link})
}

// ---- the weather for a game ----

// handleGameWeather is the forecast at the venue for the game's hours, when
// the game is within the next ten days (at the person's home place when the
// venue has no position).
func (s *Server) handleGameWeather(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	if gm.StartsAt == "" {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "why": "no time yet"})
		return
	}
	start := parseTS(gm.StartsAt)
	if time.Until(start) > 10*24*time.Hour || time.Since(start) > 3*time.Hour {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "why": "The forecast shows up ten days before the game."})
		return
	}
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appWeather, scopeWeatherRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "why": err.Error(), "not_connected": isNotConnected(err)})
		return
	}
	lat, lon := firstF(gm.VenueLat, g.VenueLat), firstF(gm.VenueLon, g.VenueLon)
	f, err := sib.client.ForecastAt(ctx, lat, lon)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "why": "Pocket Weather: " + err.Error()})
		return
	}
	wx := summarizeWeather(f, start, gm.DurationMin)
	wx["available"] = true
	wx["outdoor"] = g.Outdoor
	wx["at_home"] = lat == nil
	if f.Location != nil {
		wx["place"] = f.Location.Name
	}
	wx["open_url"] = s.siblingLink(ctx, actorID(ctx), appWeather, "/")
	writeJSON(w, http.StatusOK, wx)
}

func firstF(v ...*float64) *float64 {
	for _, x := range v {
		if x != nil {
			return x
		}
	}
	return nil
}

// summarizeWeather picks the hours of the game out of a forecast and says
// in a line what to expect.
func summarizeWeather(f *pocket.Forecast, start time.Time, minutes int) map[string]any {
	end := start.Add(time.Duration(minutes) * time.Minute)
	var temps []float64
	rain, wind := 0.0, 0.0
	cond := ""
	hours := []pocket.WeatherHour{}
	for _, h := range f.Hours {
		ht := h.Time
		if len(ht) > 16 {
			ht = ht[:16]
		}
		t, err := time.ParseInLocation(tsLayout, ht, time.Local)
		if err != nil {
			continue
		}
		if t.Before(start.Truncate(time.Hour)) || !t.Before(end) {
			continue
		}
		hours = append(hours, h)
		if h.Temp != nil {
			temps = append(temps, *h.Temp)
		}
		if h.PrecipProb != nil && *h.PrecipProb > rain {
			rain = *h.PrecipProb
		}
		if h.Gust != nil && *h.Gust > wind {
			wind = *h.Gust
		} else if h.Wind != nil && *h.Wind > wind {
			wind = *h.Wind
		}
		if cond == "" {
			cond = h.Condition
		}
	}
	out := map[string]any{"hours": hours}
	if len(hours) == 0 {
		// beyond the hourly forecast: the day
		for _, d := range f.Days {
			if d.Date == start.Format("2006-01-02") {
				out["day"] = d
				line := d.Text
				if line == "" {
					line = d.Condition
				}
				out["text"] = line
				if d.PrecipProb != nil && *d.PrecipProb >= 50 {
					out["warning"] = fmt.Sprintf("%d%% chance of rain that day", int(*d.PrecipProb))
				}
			}
		}
		return out
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, t := range temps {
		lo, hi = math.Min(lo, t), math.Max(hi, t)
	}
	text := cond
	if len(temps) > 0 {
		if math.Round(lo) == math.Round(hi) {
			text = joinNonEmpty(", ", cond, fmt.Sprintf("%d°", int(math.Round(hi))))
		} else {
			text = joinNonEmpty(", ", cond, fmt.Sprintf("%d–%d°", int(math.Round(lo)), int(math.Round(hi))))
		}
		out["temp_min"], out["temp_max"] = math.Round(lo), math.Round(hi)
	}
	out["rain"] = rain
	out["wind"] = wind
	switch {
	case rain >= 60:
		out["warning"] = fmt.Sprintf("Rain likely (%d%%)", int(rain))
	case wind >= 40:
		out["warning"] = fmt.Sprintf("Windy: gusts up to %d km/h", int(wind))
	case len(temps) > 0 && hi >= 32:
		out["warning"] = "Hot: bring water"
	case len(temps) > 0 && lo <= 2:
		out["warning"] = "Cold: near freezing"
	case rain >= 30:
		text += fmt.Sprintf(", %d%% chance of rain", int(rain))
	}
	out["text"] = text
	return out
}

// ---- what is already in my calendar ----

// handleGameBusy lists what the person already has on at the game's time
// (or at each time to vote on), from their Pocket Calendar.
func (s *Server) handleGameBusy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, _, ok := s.myGame(w, r)
	if !ok {
		return
	}
	times := []string{}
	if gm.Status == store.GameVoting {
		opts, _ := s.store.Options(ctx, gm.ID)
		for _, o := range opts {
			times = append(times, o.StartsAt)
		}
	} else if gm.StartsAt != "" {
		times = append(times, gm.StartsAt)
	}
	if len(times) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "busy": map[string]any{}})
		return
	}
	uid := actorID(ctx)
	sib, err := s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "why": err.Error()})
		return
	}
	from := parseTS(times[0]).Format("2006-01-02")
	to := parseTS(times[len(times)-1]).AddDate(0, 0, 1).Format("2006-01-02")
	evs, err := sib.client.Events(ctx, from, to)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "why": "Pocket Calendar: " + err.Error()})
		return
	}
	mine := map[string]bool{}
	if m, err := s.store.FindMade(ctx, uid, "calendar", gm.ID, 0); err == nil {
		mine[m.Title] = true
	}
	busy := map[string][]string{}
	for _, t := range times {
		st := parseTS(t)
		en := st.Add(time.Duration(gm.DurationMin) * time.Minute)
		for _, e := range evs {
			if e.AllDay || (e.Busy != nil && !*e.Busy) || mine[e.Title] || e.Source == "league" {
				continue
			}
			es, err1 := time.Parse(time.RFC3339, e.Start)
			ee, err2 := time.Parse(time.RFC3339, e.End)
			if err1 != nil || err2 != nil {
				continue
			}
			if es.Before(en) && ee.After(st) {
				busy[t] = append(busy[t], fmt.Sprintf("%s (%s–%s)", e.Title, es.Local().Format("15:04"), ee.Local().Format("15:04")))
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "busy": busy})
}

// ---- the games in each person's calendar ----

// syncCalendarLater brings everyone's calendar in line with a game, in the
// background (answering never waits on another app).
func (s *Server) syncCalendarLater(gameID int64, drop bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if drop {
			s.dropCalendar(ctx, gameID)
			return
		}
		s.syncCalendar(ctx, gameID)
	}()
}

// syncCalendar: the people who are in have the game in their calendar
// (when they want that), at its current time; the others don't.
func (s *Server) syncCalendar(ctx context.Context, gameID int64) {
	gm, err := s.store.GameByID(ctx, gameID)
	if err != nil {
		return
	}
	g, err := s.store.GroupByID(ctx, gm.GroupID)
	if err != nil {
		return
	}
	answers, _ := s.store.Answers(ctx, gameID)
	in := map[int64]bool{}
	for _, a := range answers {
		if a.Answer == store.AnswerIn && gm.Status == store.GamePlanned && gm.StartsAt != "" {
			in[a.UserID] = true
		}
	}
	have, _ := s.store.MadeCalendarFor(ctx, gameID)
	had := map[int64]*store.Made{}
	for i := range have {
		had[have[i].UserID] = &have[i]
	}
	for uid := range in {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || !st.CalendarAuto {
			continue
		}
		s.putInCalendar(ctx, uid, st, g, gm, had[uid])
	}
	for uid, m := range had {
		if in[uid] {
			continue
		}
		s.removeFromCalendar(ctx, uid, m)
	}
}

// dropCalendar removes a game from everyone's calendar (it is going away).
func (s *Server) dropCalendar(ctx context.Context, gameID int64) {
	have, _ := s.store.MadeCalendarFor(ctx, gameID)
	for i := range have {
		s.removeFromCalendar(ctx, have[i].UserID, &have[i])
	}
}

func (s *Server) calendarEvent(g *store.Group, gm *store.Game, st store.Settings) pocket.Event {
	desc := g.Name + " · " + sportLine(g)
	if gm.Notes != "" {
		desc += "\n\n" + gm.Notes
	}
	desc += "\n\nFrom Pocket League: " + s.gameLink(gm.ID)
	return pocket.Event{CalendarID: st.CalendarID, Title: firstNonEmpty(gm.Title, sportLine(g)+" · "+g.Name), Description: desc,
		Location: joinNonEmpty(", ", gm.VenueName, gm.VenueAddress), URL: s.gameLink(gm.ID), Start: gm.StartsAt, Minutes: gm.DurationMin}
}

func (s *Server) putInCalendar(ctx context.Context, uid int64, st store.Settings, g *store.Group, gm *store.Game, had *store.Made) (*store.Made, error) {
	sib, err := s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return nil, err
	}
	ev := s.calendarEvent(g, gm, st)
	if had != nil {
		id, _ := strconv.ParseInt(had.Ref, 10, 64)
		if had.Detail == gm.StartsAt+"|"+ev.Location+"|"+ev.Title {
			return had, nil
		}
		if err := sib.client.UpdateEvent(ctx, id, ev); err == nil {
			had.Detail = gm.StartsAt + "|" + ev.Location + "|" + ev.Title
			_ = s.store.PutMade(ctx, had)
			return had, nil
		} else if !errors.Is(err, pocket.ErrNotFound) {
			return nil, err
		}
	}
	id, err := sib.client.CreateEvent(ctx, ev)
	if err != nil {
		s.log.Debug("calendar: add game failed", "user", uid, "game", gm.ID, "err", err)
		return nil, err
	}
	m := &store.Made{UserID: uid, GroupID: g.ID, GameID: gm.ID, App: "calendar", Ref: itoa(id), Title: ev.Title,
		URL: s.siblingLink(ctx, uid, appCalendar, "/?event="+itoa(id)), Detail: gm.StartsAt + "|" + ev.Location + "|" + ev.Title}
	if err := s.store.PutMade(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Server) removeFromCalendar(ctx context.Context, uid int64, m *store.Made) {
	sib, err := s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return
	}
	id, _ := strconv.ParseInt(m.Ref, 10, 64)
	if err := sib.client.DeleteEvent(ctx, id); err != nil {
		s.log.Debug("calendar: remove game failed", "user", uid, "err", err)
		return
	}
	_ = s.store.DeleteMade(ctx, uid, m.ID)
}

// ---- a match in each player's own apps ----

// recordMatch records a match for every player who wants it: a workout in
// Pocket Health, a tick in Pocket Habits.
func (s *Server) recordMatch(g *store.Group, matchID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	m, err := s.store.MatchByID(ctx, matchID)
	if err != nil {
		return
	}
	for _, p := range m.Players {
		st, err := s.store.GetSettings(ctx, p.UserID)
		if err != nil {
			continue
		}
		if st.HealthAuto {
			_, _ = s.matchToHealth(ctx, p.UserID, g, m)
		}
		if st.HabitID > 0 {
			_, _ = s.matchToHabit(ctx, p.UserID, st.HabitID, g, m)
		}
	}
}

// matchMinutes is how long a match took: the game's length, shared by the
// matches played that evening.
func (s *Server) matchMinutes(ctx context.Context, g *store.Group, m *store.Match) int {
	minutes := g.DurationMin
	if m.GameID != 0 {
		if gm, err := s.store.GameByID(ctx, m.GameID); err == nil {
			minutes = gm.DurationMin
			if gm.Matches > 1 {
				minutes = max(15, minutes/gm.Matches)
			}
		}
	}
	return max(10, minutes)
}

func (s *Server) matchToHealth(ctx context.Context, uid int64, g *store.Group, m *store.Match) (*store.Made, error) {
	sp := league.SportByKey(g.Sport)
	if sp.Health == "" {
		return nil, &notConnected{"this sport is not a workout"}
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appHealth, scopeHealthWrite)
	if err != nil {
		return nil, err
	}
	minutes := s.matchMinutes(ctx, g, m)
	start := parseTS(m.PlayedAt)
	if m.GameID != 0 {
		// several matches in an evening follow one another
		games, _ := s.store.ListMatches(ctx, 0, store.MatchFilter{GameID: m.GameID, Oldest: true})
		for i, x := range games {
			if x.ID == m.ID {
				start = start.Add(time.Duration(i*minutes) * time.Minute)
			}
		}
	}
	end := start.Add(time.Duration(minutes) * time.Minute)
	names := s.names(ctx, g.ID)
	w := pocket.NewHealthWorkout{Kind: sp.Health, Name: league.DisplayName(g.Sport, g.SportName) + " · " + g.Name,
		StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339), DurationS: minutes * 60,
		Notes: s.resultLine(g, m, names) + "\nFrom Pocket League"}
	id, err := sib.client.SendWorkout(ctx, w)
	if err != nil {
		s.log.Debug("health: record match failed", "user", uid, "err", err)
		return nil, err
	}
	made := &store.Made{UserID: uid, GroupID: g.ID, GameID: m.GameID, MatchID: m.ID, App: "health", Ref: itoa(id), Title: w.Name,
		URL: s.siblingLink(ctx, uid, appHealth, "/workouts/"+itoa(id))}
	return made, s.store.PutMade(ctx, made)
}

func (s *Server) matchToHabit(ctx context.Context, uid, habit int64, g *store.Group, m *store.Match) (*store.Made, error) {
	sib, err := s.clients(ctx, uid).conn(ctx, appHabits, scopeEntriesWrite)
	if err != nil {
		return nil, err
	}
	day := m.PlayedAt[:10]
	if err := sib.client.MarkHabit(ctx, habit, day); err != nil {
		return nil, err
	}
	made := &store.Made{UserID: uid, GroupID: g.ID, GameID: m.GameID, MatchID: m.ID, App: "habits", Ref: itoa(habit), Title: "Ticked on " + day,
		URL: s.siblingLink(ctx, uid, appHabits, "/")}
	return made, s.store.PutMade(ctx, made)
}

// bookingTask puts "book the court" in the planner's task list.
func (s *Server) bookingTask(uid int64, g *store.Group, gm *store.Game) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := s.makeBookingTask(ctx, uid, g, gm); err != nil {
		s.log.Debug("tasks: booking task failed", "user", uid, "err", err)
	}
}

func (s *Server) makeBookingTask(ctx context.Context, uid int64, g *store.Group, gm *store.Game) (*store.Made, error) {
	st, _ := s.store.GetSettings(ctx, uid)
	sib, err := s.clients(ctx, uid).conn(ctx, appTasks, scopeTasksWrite)
	if err != nil {
		return nil, err
	}
	days := st.BookingDays
	if days <= 0 {
		days = 2
	}
	start := parseTS(gm.StartsAt)
	due := start.AddDate(0, 0, -days)
	if due.Before(time.Now()) {
		due = time.Now()
	}
	where := firstNonEmpty(gm.VenueName, g.VenueName, "the court")
	t, err := sib.client.CreateTask(ctx, "Book "+where+" for "+g.Name+" ("+whenText(gm.StartsAt)+")",
		"The game: "+s.gameLink(gm.ID), due.Format("2006-01-02"), s.gameLink(gm.ID))
	if err != nil {
		return nil, err
	}
	made := &store.Made{UserID: uid, GroupID: g.ID, GameID: gm.ID, App: "tasks", Ref: itoa(t.ID), Title: t.Title, URL: s.siblingLink(ctx, uid, appTasks, "/")}
	return made, s.store.PutMade(ctx, made)
}

// ---- POST /games/{id}/make: one thing in another app, now ----

func (s *Server) handleMake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gm, g, ok := s.myGame(w, r)
	if !ok {
		return
	}
	u := userFrom(ctx)
	var in struct {
		What       string `json:"what"` // calendar | split | money | health | tasks
		SplitGroup int64  `json:"split_group"`
		Amount     int64  `json:"amount"` // cents; 0 = the game's cost (split) or my share (money)
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	var made *store.Made
	var err error
	var app pocketApp
	switch in.What {
	case "calendar":
		app = appCalendar
		if gm.StartsAt == "" || gm.Status != store.GamePlanned {
			writeErr(w, http.StatusBadRequest, "the game has no time yet")
			return
		}
		had, _ := s.store.FindMade(ctx, u.ID, "calendar", gm.ID, 0)
		made, err = s.putInCalendar(ctx, u.ID, st, g, gm, had)
	case "split":
		app = appSplit
		made, err = s.splitCost(ctx, u, st, g, gm, in.SplitGroup, in.Amount)
	case "money":
		app = appMoney
		made, err = s.bookShare(ctx, u, st, g, gm, in.Amount)
	case "health":
		app = appHealth
		ms, _ := s.store.ListMatches(ctx, u.ID, store.MatchFilter{GameID: gm.ID, UserID: u.ID, Oldest: true})
		if len(ms) == 0 {
			writeErr(w, http.StatusBadRequest, "you have no match in this game yet")
			return
		}
		for _, m := range ms {
			if made, err = s.matchToHealth(ctx, u.ID, g, m); err != nil {
				break
			}
		}
	case "tasks":
		app = appTasks
		if gm.StartsAt == "" {
			writeErr(w, http.StatusBadRequest, "the game has no time yet")
			return
		}
		made, err = s.makeBookingTask(ctx, u.ID, g, gm)
	default:
		writeErr(w, http.StatusBadRequest, "unknown app")
		return
	}
	if err != nil {
		if isNotConnected(err) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErr(w, http.StatusBadGateway, app.name+": "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, made)
}

// splitCost adds what the game cost to a Split group, shared by the people
// who were in (matched to the Split group's people by email or name; the
// whole Split group when nobody matches).
func (s *Server) splitCost(ctx context.Context, u *store.User, st store.Settings, g *store.Group, gm *store.Game, groupID, amount int64) (*store.Made, error) {
	if amount <= 0 {
		amount = gm.CostCents
	}
	if amount <= 0 {
		return nil, &notConnected{"say what the game cost first (Edit the game → Cost)"}
	}
	if groupID == 0 {
		groupID = st.SplitGroup
	}
	sib, err := s.clients(ctx, u.ID).conn(ctx, appSplit, scopeGroupsRead, scopeExpensesWrite)
	if err != nil {
		return nil, err
	}
	gs, err := sib.client.SplitGroups(ctx)
	if err != nil {
		return nil, err
	}
	var sg *pocket.SplitGroup
	for i := range gs {
		if gs[i].ID == groupID {
			sg = &gs[i]
		}
	}
	if sg == nil {
		return nil, &notConnected{"pick the Split group the costs go to"}
	}
	answers, _ := s.store.Answers(ctx, gm.ID)
	ms, _ := s.store.Members(ctx, g.ID)
	byUser := map[int64]*store.Member{}
	for _, m := range ms {
		byUser[m.UserID] = m
	}
	var who []int64
	matched := []string{}
	missing := []string{}
	for _, a := range answers {
		if a.Answer != store.AnswerIn {
			continue
		}
		m := byUser[a.UserID]
		if m == nil {
			continue
		}
		found := int64(0)
		for _, sm := range sg.Members {
			if (m.Email != "" && strings.EqualFold(sm.Email, m.Email)) || strings.EqualFold(strings.TrimSpace(sm.Name), strings.TrimSpace(m.Name)) ||
				(m.Nickname != "" && strings.EqualFold(sm.Name, m.Nickname)) || (a.UserID == u.ID && sm.ID == sg.MyMemberID) {
				found = sm.ID
				break
			}
		}
		if found != 0 {
			who = append(who, found)
			matched = append(matched, m.Display())
		} else {
			missing = append(missing, m.Display())
		}
	}
	if len(who) == 0 {
		for _, sm := range sg.Members {
			who = append(who, sm.ID)
		}
	}
	id, err := sib.client.CreateSplitExpense(ctx, pocket.SplitExpense{GroupID: sg.ID, Title: firstNonEmpty(gm.VenueName, g.VenueName, "Court") + " · " + g.Name,
		Amount: amount, Curr: gm.Currency, Date: gm.StartsAt[:10], Notes: "From Pocket League: " + s.gameLink(gm.ID), PaidBy: sg.MyMemberID, Members: who})
	if err != nil {
		return nil, err
	}
	detail := "Shared by " + strings.Join(matched, ", ")
	if len(matched) == 0 {
		detail = "Shared by everyone in " + sg.Name
	}
	if len(missing) > 0 {
		detail += " · not in the Split group: " + strings.Join(missing, ", ")
	}
	made := &store.Made{UserID: u.ID, GroupID: g.ID, GameID: gm.ID, App: "split", Ref: fmt.Sprintf("%d:%d", sg.ID, id), Title: fmtMoney(amount, gm.Currency) + " in " + sg.Name,
		URL: s.siblingLink(ctx, u.ID, appSplit, fmt.Sprintf("/groups/%d", sg.ID)), Detail: detail}
	return made, s.store.PutMade(ctx, made)
}

// bookShare books the person's share of the game as an expense.
func (s *Server) bookShare(ctx context.Context, u *store.User, st store.Settings, g *store.Group, gm *store.Game, amount int64) (*store.Made, error) {
	if amount <= 0 {
		amount = shareCost(gm)
	}
	if amount <= 0 {
		return nil, &notConnected{"say what the game cost first (Edit the game → Cost)"}
	}
	if st.MoneyAccount == 0 {
		return nil, &notConnected{"pick the Money account in Settings → Connected apps"}
	}
	sib, err := s.clients(ctx, u.ID).conn(ctx, appMoney, scopeTxWrite)
	if err != nil {
		return nil, err
	}
	id, err := sib.client.CreateTx(ctx, pocket.Tx{AccountID: st.MoneyAccount, CategoryID: st.MoneyCat, Date: gm.StartsAt[:10], Time: gm.StartsAt[11:],
		Amount: amount, Currency: gm.Currency, Description: league.DisplayName(g.Sport, g.SportName) + " · " + g.Name, Payee: firstNonEmpty(gm.VenueName, g.VenueName),
		Notes: "My share. From Pocket League: " + s.gameLink(gm.ID), ExternalID: fmt.Sprintf("league:game:%d", gm.ID)})
	if err != nil {
		return nil, err
	}
	made := &store.Made{UserID: u.ID, GroupID: g.ID, GameID: gm.ID, App: "money", Ref: itoa(id), Title: fmtMoney(amount, gm.Currency) + " booked",
		URL: s.siblingLink(ctx, u.ID, appMoney, "/transactions/"+itoa(id))}
	return made, s.store.PutMade(ctx, made)
}

func (s *Server) handleDeleteMade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteMade(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "forget")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fmtMoney is 1250 cents EUR as "12.50 EUR" (no cents when there are none).
func fmtMoney(cents int64, cur string) string {
	if cents%100 == 0 {
		return fmt.Sprintf("%d %s", cents/100, cur)
	}
	return fmt.Sprintf("%d.%02d %s", cents/100, cents%100, cur)
}

// ---- events on the box ----

// publishFor publishes an event for one person.
func (s *Server) publishFor(ctx context.Context, uid int64, topic, subject string, payload map[string]any) {
	s.publish(ctx, uid, topic, subject, payload)
}

// publishGroup publishes an event for each person of a group who is a
// person on the box (players from outside are not on Core's bus).
func (s *Server) publishGroup(ctx context.Context, g *store.Group, topic, subject string, payload map[string]any) {
	if !s.core.Configured() {
		return
	}
	ms, err := s.store.Members(ctx, g.ID)
	if err != nil {
		return
	}
	for _, m := range ms {
		if m.Account == store.RolePlayer || m.Email == "" {
			continue
		}
		p := map[string]any{}
		for k, v := range payload {
			p[k] = v
		}
		s.publish(ctx, m.UserID, topic, subject+":"+itoa(m.UserID), p)
	}
}
