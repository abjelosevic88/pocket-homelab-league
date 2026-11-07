package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// seedDemo fills the owner's account with three groups and four months of
// games, answers, results and chat (DEMO_DATA=true), once.
func (s *Server) seedDemo(ctx context.Context, owner int64) error {
	// with AUTH_MODE=none this runs inside the first request: it must not
	// stop when that client goes away
	ctx = context.WithoutCancel(ctx)
	if gs, _ := s.store.ListGroups(ctx, owner, true); len(gs) > 0 {
		return nil
	}
	rnd := rand.New(rand.NewSource(42))
	now := time.Now()

	type pp struct {
		name, email, phone string
		skill              float64
	}
	people := []pp{
		{"Marko Petrović", "marko@example.com", "+387 61 111 201", 0.62}, {"Ana Kovačević", "ana@example.com", "+387 62 222 302", 0.71},
		{"Ivan Horvat", "ivan@example.com", "", 0.48}, {"Edin Hodžić", "", "+387 61 333 403", 0.55}, {"Lejla Begić", "lejla@example.com", "", 0.66},
		{"Nikola Jovanović", "nikola@example.com", "+387 65 444 504", 0.58}, {"Amra Delić", "amra@example.com", "", 0.45}, {"Dario Marić", "dario@example.com", "", 0.69},
		{"Selma Mujić", "selma@example.com", "+387 62 555 605", 0.52}, {"Tarik Hadžić", "tarik@example.com", "", 0.74}, {"Vedran Kos", "vedran@example.com", "", 0.41},
		{"Haris Karić", "", "+387 61 666 706", 0.6}, {"Emina Softić", "emina@example.com", "", 0.57},
	}
	ids := []int64{}
	skill := map[int64]float64{owner: 0.6}
	for i, p := range people {
		u := &store.User{Email: p.email, Name: p.name, Phone: p.phone, Role: store.RolePlayer, InvitedBy: owner}
		if i < 2 {
			u.Role = store.RoleMember
		}
		if err := s.store.CreateUser(ctx, u); err != nil {
			if ex, err2 := s.store.FindPlayer(ctx, p.email, p.phone); err2 == nil {
				u = ex
			} else {
				return err
			}
		}
		ids = append(ids, u.ID)
		skill[u.ID] = p.skill
		if i%3 != 2 {
			s.store.TouchUser(ctx, u.ID)
		}
	}
	lat, lon := 43.8737, 18.4093
	lat2, lon2 := 43.8475, 18.3530

	mk := func(g *store.Group, members []int64) (*store.Group, error) {
		if err := s.store.CreateGroup(ctx, owner, g); err != nil {
			return nil, err
		}
		for i, id := range members {
			role := "player"
			if i == 0 {
				role = "organiser"
			}
			if _, err := s.store.AddMember(ctx, g.ID, id, owner, role); err != nil {
				return nil, err
			}
		}
		return g, nil
	}
	tennis, err := mk(&store.Group{Name: "Tuesday Tennis", Sport: "tennis", Scoring: league.ScoringSets, Format: league.FormatSingles, Color: "#9fbf00",
		Description: "Singles at Koševo, every Tuesday evening. Loser of the last match brings the balls.", VenueName: "TK Koševo", VenueAddress: "Patriotske lige 35, Sarajevo",
		VenueLat: &lat, VenueLon: &lon, Outdoor: true, Weekday: 2, Time: "20:00", DurationMin: 120, PlanDays: 6, MinPlayers: 4, MaxPlayers: 6,
		CostCents: 4000, Currency: "BAM", WinPoints: 1, DrawPoints: 0, LossPoints: 0, EmailPlayers: true}, ids[0:5])
	if err != nil {
		return err
	}
	football, err := mk(&store.Group{Name: "Friday 5-a-side", Sport: "futsal", Scoring: league.ScoringGoals, Format: league.FormatTeams, Color: "#37b24d",
		Description: "Futsal at Mojmilo. Bring a light and a dark shirt.", VenueName: "Dvorana Mojmilo", VenueAddress: "Olimpijska, Sarajevo", VenueLat: &lat2, VenueLon: &lon2,
		Weekday: 5, Time: "19:00", DurationMin: 60, PlanDays: 5, MinPlayers: 10, MaxPlayers: 12, CostCents: 12000, Currency: "BAM", WinPoints: 3, DrawPoints: 1, EmailPlayers: true},
		append([]int64{ids[5]}, ids[0], ids[2], ids[3], ids[6], ids[7], ids[8], ids[9], ids[10], ids[11], ids[12]))
	if err != nil {
		return err
	}
	padel, err := mk(&store.Group{Name: "Padel crew", Sport: "padel", Scoring: league.ScoringSets, Format: league.FormatDoubles, Color: "#1c7ed6",
		Description: "Doubles whenever four of us can make it.", VenueName: "Padel Centar Stup", VenueAddress: "Stupska, Sarajevo", DurationMin: 90, MinPlayers: 4, MaxPlayers: 4,
		CostCents: 6000, Currency: "BAM", WinPoints: 1, EmailPlayers: true}, []int64{ids[1], ids[4], ids[7], ids[9]})
	if err != nil {
		return err
	}
	// owner is in every group (CreateGroup made them organiser); the places they play at
	for _, g := range []*store.Group{tennis, football, padel} {
		_ = s.store.EnsureVenue(ctx, g.ID, store.Venue{Name: g.VenueName, Address: g.VenueAddress, Lat: g.VenueLat, Lon: g.VenueLon})
	}
	zLat, zLon := 43.8563, 18.4131
	_ = s.store.EnsureVenue(ctx, tennis.ID, store.Venue{Name: "TK Zetra (indoor)", Address: "Alipašina, Sarajevo", Lat: &zLat, Lon: &zLon})
	_ = s.store.EnsureVenue(ctx, football.ID, store.Venue{Name: "Teren Ciglane", Address: "Ciglane, Sarajevo"})

	weekday := func(wd int, weeksAgo int, hm string) string {
		d := now.AddDate(0, 0, -7*weeksAgo)
		for isoWeekday(d) != wd {
			d = d.AddDate(0, 0, -1)
		}
		return d.Format("2006-01-02") + "T" + hm
	}
	win := func(a, b []int64) bool {
		sa, sb := 0.0, 0.0
		for _, x := range a {
			sa += skill[x]
		}
		for _, x := range b {
			sb += skill[x]
		}
		sa, sb = sa/float64(len(a)), sb/float64(len(b))
		return rnd.Float64() < 0.5+(sa-sb)*1.4
	}
	setScore := func(aWins bool) [][2]int {
		var sets [][2]int
		wa, wb := 0, 0
		for wa < 2 && wb < 2 {
			aSet := rnd.Float64() < map[bool]float64{true: 0.7, false: 0.3}[aWins]
			lo := rnd.Intn(5)
			hi := 6
			if lo == 5 || (lo == 4 && rnd.Intn(3) == 0) {
				hi, lo = 7, 5+rnd.Intn(2)
			}
			if aSet {
				sets = append(sets, [2]int{hi, lo})
				wa++
			} else {
				sets = append(sets, [2]int{lo, hi})
				wb++
			}
		}
		return sets
	}
	say := func(g *store.Group, gameID, uid int64, body string, at time.Time) {
		m := &store.Message{GroupID: g.ID, GameID: gameID, UserID: uid, Kind: "text", Body: body}
		if _, err := s.store.AddMessage(ctx, m); err == nil {
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE messages SET created_at = ? WHERE id = ?`, at.UTC().Format(time.RFC3339Nano), m.ID)
		}
	}
	sys := func(g *store.Group, gameID int64, text string, data map[string]any, at time.Time) {
		b, _ := json.Marshal(data)
		m := &store.Message{GroupID: g.ID, GameID: gameID, Kind: "system", Body: text, Data: b}
		if _, err := s.store.AddMessage(ctx, m); err == nil {
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE messages SET created_at = ? WHERE id = ?`, at.UTC().Format(time.RFC3339Nano), m.ID)
		}
	}
	names := func(g *store.Group) map[int64]string { return s.names(ctx, g.ID) }

	// ---- Tuesday Tennis: sixteen weeks of singles ----
	tPlayers := append([]int64{owner}, ids[0:5]...)
	for w := 16; w >= 1; w-- {
		ts := weekday(2, w, "20:00")
		if parseTS(ts).After(now) {
			continue
		}
		gm := &store.Game{GroupID: tennis.ID, Status: store.GamePlanned, StartsAt: ts, DurationMin: 120, VenueName: tennis.VenueName, VenueAddress: tennis.VenueAddress,
			VenueLat: tennis.VenueLat, VenueLon: tennis.VenueLon, MinPlayers: 4, MaxPlayers: 6, CostCents: 4000, Currency: "BAM", Auto: true}
		if w == 9 {
			gm.Status, gm.CancelReason = store.GameCancelled, "Rain all evening"
		}
		_ = s.store.CreateGame(ctx, owner, gm)
		pl := append([]int64{}, tPlayers...)
		rnd.Shuffle(len(pl), func(i, j int) { pl[i], pl[j] = pl[j], pl[i] })
		n := 4
		if rnd.Intn(3) == 0 {
			n = 5
		}
		for i, uid := range pl {
			a := store.AnswerIn
			if i >= n {
				a = []string{store.AnswerOut, store.AnswerMaybe}[rnd.Intn(2)]
			}
			_ = s.store.SetAnswer(ctx, gm.ID, uid, a, "")
		}
		if gm.Status == store.GameCancelled {
			continue
		}
		in := pl[:n]
		for k := 0; k+1 < len(in) && k < 4; k += 2 {
			a, b := in[k], in[k+1]
			m := &store.Match{GroupID: tennis.ID, GameID: gm.ID, PlayedAt: ts, Score: store.Score{Sets: setScore(win([]int64{a}, []int64{b}))},
				Players: []store.MatchPlayer{{UserID: a, Side: "a"}, {UserID: b, Side: "b"}}}
			_ = league.Check(league.RulesOf(tennis), m)
			if err := s.store.SaveMatch(ctx, a, m); err == nil && w > 1 {
				_ = s.store.ConfirmMatch(ctx, m.ID, b)
			}
			if w == 1 && k == 0 {
				// the latest one is waiting for the owner to confirm
				if b != owner && a != owner {
					m2 := &store.Match{GroupID: tennis.ID, GameID: gm.ID, PlayedAt: ts, Score: store.Score{Sets: [][2]int{{6, 4}, {4, 6}, {7, 5}}},
						Players: []store.MatchPlayer{{UserID: ids[0], Side: "a"}, {UserID: owner, Side: "b"}}}
					_ = league.Check(league.RulesOf(tennis), m2)
					_ = s.store.SaveMatch(ctx, ids[0], m2)
				}
			}
		}
	}
	// ---- Friday 5-a-side: teams picked fairly every week ----
	fMembers, _ := s.store.MemberIDs(ctx, football.ID)
	for w := 14; w >= 1; w-- {
		ts := weekday(5, w, "19:00")
		if parseTS(ts).After(now) {
			continue
		}
		gm := &store.Game{GroupID: football.ID, Status: store.GamePlanned, StartsAt: ts, DurationMin: 60, VenueName: football.VenueName, VenueAddress: football.VenueAddress,
			VenueLat: football.VenueLat, VenueLon: football.VenueLon, MinPlayers: 10, MaxPlayers: 12, CostCents: 12000, Currency: "BAM", Auto: true}
		_ = s.store.CreateGame(ctx, owner, gm)
		pl := append([]int64{}, fMembers...)
		rnd.Shuffle(len(pl), func(i, j int) { pl[i], pl[j] = pl[j], pl[i] })
		n := 10 + rnd.Intn(3)
		if n > len(pl) {
			n = len(pl)
		}
		in := pl[:n]
		for i, uid := range pl {
			a := store.AnswerIn
			if i >= n {
				a = store.AnswerOut
			}
			_ = s.store.SetAnswer(ctx, gm.ID, uid, a, "")
		}
		t, _ := s.table(ctx, football, "all")
		ta, tb, _ := league.FairTeams(in, t.Ratings(), int64(w))
		sides := map[int64]string{}
		for _, x := range ta {
			sides[x] = "a"
		}
		for _, x := range tb {
			sides[x] = "b"
		}
		_ = s.store.SetSides(ctx, gm.ID, sides)
		ga, gb := 0, 0
		goals := map[int64]float64{}
		for k := 0; k < 4+rnd.Intn(8); k++ {
			if win(ta, tb) {
				ga++
				goals[ta[rnd.Intn(len(ta))]]++
			} else {
				gb++
				goals[tb[rnd.Intn(len(tb))]]++
			}
		}
		fa, fb := float64(ga), float64(gb)
		m := &store.Match{GroupID: football.ID, GameID: gm.ID, PlayedAt: ts, Score: store.Score{A: &fa, B: &fb}}
		for _, x := range ta {
			mp := store.MatchPlayer{UserID: x, Side: "a"}
			if g := goals[x]; g > 0 {
				v := g
				mp.Score = &v
			}
			m.Players = append(m.Players, mp)
		}
		for _, x := range tb {
			mp := store.MatchPlayer{UserID: x, Side: "b"}
			if g := goals[x]; g > 0 {
				v := g
				mp.Score = &v
			}
			m.Players = append(m.Players, mp)
		}
		_ = league.Check(league.RulesOf(football), m)
		if err := s.store.SaveMatch(ctx, ids[5], m); err == nil {
			_ = s.store.ConfirmMatch(ctx, m.ID, owner)
		}
		// player of the game
		best := in[0]
		for _, x := range in {
			if goals[x] > goals[best] {
				best = x
			}
		}
		for _, voter := range in[:6] {
			if voter != best {
				_ = s.store.SetMVPVote(ctx, gm.ID, voter, best)
			}
		}
		at := parseTS(ts).Add(75 * time.Minute)
		sys(football, 0, s.resultLine(football, m, names(football)), map[string]any{"what": "result", "match_id": m.ID, "game_id": gm.ID}, at)
	}

	// ---- the coming games ----
	nextTue := ""
	for d := 1; d <= 7; d++ {
		day := now.AddDate(0, 0, d)
		if isoWeekday(day) == 2 {
			nextTue = day.Format("2006-01-02") + "T20:00"
		}
	}
	tg := &store.Game{GroupID: tennis.ID, Status: store.GamePlanned, StartsAt: nextTue, DurationMin: 120, VenueName: tennis.VenueName, VenueAddress: tennis.VenueAddress,
		VenueLat: tennis.VenueLat, VenueLon: tennis.VenueLon, MinPlayers: 4, MaxPlayers: 6, CostCents: 4000, Currency: "BAM", Auto: true}
	_ = s.store.CreateGame(ctx, owner, tg)
	_ = s.store.SetAnswer(ctx, tg.ID, ids[0], store.AnswerIn, "")
	_ = s.store.SetAnswer(ctx, tg.ID, ids[1], store.AnswerIn, "")
	_ = s.store.SetAnswer(ctx, tg.ID, ids[4], store.AnswerIn, "might be 10 min late")
	_ = s.store.SetAnswer(ctx, tg.ID, ids[2], store.AnswerMaybe, "")
	_ = s.store.SetAnswer(ctx, tg.ID, ids[3], store.AnswerOut, "away for work")
	sys(tennis, 0, "The usual game is on: "+whenText(nextTue)+" at TK Koševo", map[string]any{"what": "game_planned", "game_id": tg.ID}, now.Add(-30*time.Hour))
	say(tennis, 0, ids[0], "Who's bringing balls this time? Mine are dead 😅", now.Add(-26*time.Hour))
	say(tennis, 0, ids[1], "I'll bring a new tube", now.Add(-25*time.Hour))
	say(tennis, 0, ids[4], "In! Might be 10 minutes late, start without me", now.Add(-20*time.Hour))
	say(tennis, 0, ids[2], "Not sure yet, will know tomorrow", now.Add(-5*time.Hour))
	say(tennis, tg.ID, ids[0], "Court 3 again? It has the better lights", now.Add(-4*time.Hour))

	nextFri := ""
	for d := 1; d <= 7; d++ {
		day := now.AddDate(0, 0, d)
		if isoWeekday(day) == 5 {
			nextFri = day.Format("2006-01-02") + "T19:00"
		}
	}
	fg := &store.Game{GroupID: football.ID, Status: store.GamePlanned, StartsAt: nextFri, DurationMin: 60, VenueName: football.VenueName, VenueAddress: football.VenueAddress,
		VenueLat: football.VenueLat, VenueLon: football.VenueLon, MinPlayers: 10, MaxPlayers: 12, CostCents: 12000, Currency: "BAM", Auto: true}
	_ = s.store.CreateGame(ctx, owner, fg)
	for i, uid := range fMembers {
		switch {
		case i < 8:
			_ = s.store.SetAnswer(ctx, fg.ID, uid, store.AnswerIn, "")
		case i == 8:
			_ = s.store.SetAnswer(ctx, fg.ID, uid, store.AnswerMaybe, "")
		case i == 9:
			_ = s.store.SetAnswer(ctx, fg.ID, uid, store.AnswerOut, "knee still sore")
		}
	}
	sys(football, 0, "The usual game is on: "+whenText(nextFri)+" at Dvorana Mojmilo", map[string]any{"what": "game_planned", "game_id": fg.ID}, now.Add(-40*time.Hour))
	say(football, 0, ids[5], "8 in so far — we need two more for Friday, ask your colleagues!", now.Add(-28*time.Hour))
	say(football, 0, ids[9], "My brother might come, I'll let you know", now.Add(-27*time.Hour))
	say(football, 0, ids[7], "Last week was a great game. Rematch 💪", now.Add(-10*time.Hour))
	say(football, 0, ids[11], "Who has the bibs?", now.Add(-3*time.Hour))

	pg := &store.Game{GroupID: padel.ID, Status: store.GameVoting, DurationMin: 90, VenueName: padel.VenueName, VenueAddress: padel.VenueAddress, MinPlayers: 4, MaxPlayers: 4,
		CostCents: 6000, Currency: "BAM", VoteUntil: now.AddDate(0, 0, 2).Format("2006-01-02") + "T12:00"}
	_ = s.store.CreateGame(ctx, ids[1], pg)
	// a time and a place: Stup on two evenings, or the new courts at Grbavica
	grb, grbLat, grbLon := "Padel Arena Grbavica", 43.8519, 18.3897
	opts := []store.OptionIn{
		{StartsAt: now.AddDate(0, 0, 3).Format("2006-01-02") + "T18:30", VenueName: padel.VenueName, VenueAddress: padel.VenueAddress},
		{StartsAt: now.AddDate(0, 0, 4).Format("2006-01-02") + "T19:00", VenueName: grb, VenueAddress: "Grbavička, Sarajevo", VenueLat: &grbLat, VenueLon: &grbLon},
		{StartsAt: now.AddDate(0, 0, 6).Format("2006-01-02") + "T18:30", VenueName: padel.VenueName, VenueAddress: padel.VenueAddress},
	}
	s.rememberVenues(ctx, padel.ID, nil, opts)
	_ = s.store.SetOptions(ctx, pg.ID, opts)
	if os, err := s.store.Options(ctx, pg.ID); err == nil && len(os) == 3 {
		_ = s.store.SetVotes(ctx, pg.ID, ids[1], map[int64]string{os[0].ID: "yes", os[1].ID: "maybe", os[2].ID: "yes"})
		_ = s.store.SetVotes(ctx, pg.ID, ids[4], map[int64]string{os[0].ID: "no", os[1].ID: "yes", os[2].ID: "yes"})
		_ = s.store.SetVotes(ctx, pg.ID, ids[9], map[int64]string{os[0].ID: "yes", os[2].ID: "yes"})
	}
	sys(padel, 0, "Ana Kovačević wants to find a time for the next game — vote on the times that suit you", map[string]any{"what": "game_planned", "game_id": pg.ID}, now.Add(-6*time.Hour))
	say(padel, 0, ids[9], "Saturday works best for me", now.Add(-5*time.Hour))
	// a few padel matches in the past
	pPlayers := []int64{owner, ids[1], ids[4], ids[7], ids[9]}
	for w := 10; w >= 2; w -= 2 {
		ts := weekday(6, w, "18:30")
		pl := append([]int64{}, pPlayers...)
		rnd.Shuffle(len(pl), func(i, j int) { pl[i], pl[j] = pl[j], pl[i] })
		a, b := pl[:2], pl[2:4]
		sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
		m := &store.Match{GroupID: padel.ID, PlayedAt: ts, Score: store.Score{Sets: setScore(win(a, b))},
			Players: []store.MatchPlayer{{UserID: a[0], Side: "a"}, {UserID: a[1], Side: "a"}, {UserID: b[0], Side: "b"}, {UserID: b[1], Side: "b"}}}
		_ = league.Check(league.RulesOf(padel), m)
		if err := s.store.SaveMatch(ctx, a[0], m); err == nil {
			_ = s.store.ConfirmMatch(ctx, m.ID, b[0])
		}
	}
	// read marks: the owner has seen most of it
	for _, g := range []*store.Group{tennis, football, padel} {
		ms, _ := s.store.ListMessages(ctx, store.MessageFilter{GroupID: g.ID, Limit: 200})
		if len(ms) > 2 {
			_ = s.store.MarkRead(ctx, g.ID, owner, ms[len(ms)-3].ID)
		}
	}
	s.log.Info("demo data seeded", "groups", 3, "people", len(ids))
	_ = fmt.Sprint
	return nil
}
