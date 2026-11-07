package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
	"github.com/abjelosevic88/pocket-homelab-league/internal/tour"
)

// ---- tournaments ----
//
// A tournament lives in a group: everyone in it sees it, signs up and
// enters results; its organisers (and whoever made it) run it. The draw
// turns the entries into stages and matches (internal/tour); a confirmed
// result moves the winner (and, in some formats, the loser) on, fills
// byes, frees the court for the next match, makes the next round of the
// formats that are drawn round by round, and finishes the tournament.

// TSettings are a tournament's format settings.
type TSettings struct {
	Groups      int          `json:"groups"`       // round robin / groups → knockout: how many groups
	Advance     int          `json:"advance"`      // groups → knockout: how many of each group go through
	Double      bool         `json:"double"`       // round robin twice (home and away)
	ThirdPlace  bool         `json:"third_place"`  // knockout: a match for third place
	Plate       bool         `json:"plate"`        // knockout: the first-round losers play on
	Reset       bool         `json:"reset"`        // double elimination: a second grand final if needed
	Points      int          `json:"points"`       // Americano / Mexicano / KOTC: points per match
	Rounds      int          `json:"rounds"`       // Americano / Mexicano / KOTC / Swiss / scored rounds
	Split       string       `json:"split"`        // Mexicano: "1-3" (1st+3rd v 2nd+4th) or "1-4"
	Seeding     string       `json:"seeding"`      // rating | random | manual
	Schedule    string       `json:"schedule"`     // queue (call matches to free courts) | timetable | none
	Minutes     int          `json:"minutes"`      // a match takes
	Buffer      int          `json:"buffer"`       // minutes between matches
	AutoConfirm int          `json:"auto_confirm"` // minutes until a reported result counts by itself (0 = only when confirmed)
	Mode        string       `json:"mode"`         // scored rounds: stroke | stableford | pins | plain
	Low         bool         `json:"low"`          // plain scores: the lowest wins
	Course      *tour.Course `json:"course,omitempty"`
	Allowance   float64      `json:"allowance"` // golf: share of the handicap (0.95)
	Basis       float64      `json:"basis"`     // bowling: handicap basis (220)
	Pct         float64      `json:"pct"`       // bowling: handicap percent (0.9)
	Ties        []string     `json:"ties"`      // team ties: the matches of a tie ("singles", "doubles")
	Reach       int          `json:"reach"`     // ladder: how many places up you may challenge
	RespondDays int          `json:"respond_days"`
	PlayDays    int          `json:"play_days"`
	ProtectDays int          `json:"protect_days"`
	BoxSize     int          `json:"box_size"` // box league: players per box
	Promote     int          `json:"promote"`  // box league: up and down each round
	Colours     bool         `json:"colours"`  // Swiss / chess: balance white and black
	Tiebreaks   []string     `json:"tiebreaks"`
}

func settingsOf(t *store.Tournament) TSettings {
	var s TSettings
	_ = json.Unmarshal(t.Settings, &s)
	return s
}

// defaultSettings for a new tournament of a format in a group.
func defaultSettings(g *store.Group, format string, entries int) TSettings {
	s := TSettings{Seeding: "rating", Schedule: "queue", Minutes: 20, Buffer: 5, AutoConfirm: 10, ThirdPlace: true, Reset: true,
		Points: 24, Split: "1-3", Advance: 2, Groups: 2, Allowance: 0.95, Basis: 220, Pct: 0.9, Reach: 3, RespondDays: 5, PlayDays: 14,
		ProtectDays: 3, BoxSize: 5, Promote: 1, Colours: g.Sport == "chess", Ties: []string{"singles", "singles", "doubles"}}
	sp := league.SportByKey(g.Sport)
	if sp.Minutes > 0 {
		s.Minutes = sp.Minutes
		if g.Format == league.FormatSingles || g.Format == league.FormatDoubles {
			s.Minutes = sp.Minutes / 2
		}
	}
	switch format {
	case "americano", "mexicano", "kotc":
		s.Minutes, s.Rounds = 15, 7
	case "swiss":
		s.Rounds = 5
	case "ranking":
		s.Rounds, s.Schedule = 1, "none"
		switch g.Sport {
		case "golf":
			s.Mode = "stableford"
			par := []int{4, 4, 3, 5, 4, 4, 3, 4, 5, 4, 3, 5, 4, 4, 3, 4, 5, 4}
			si := []int{7, 3, 15, 1, 11, 5, 17, 9, 13, 8, 16, 2, 6, 10, 18, 4, 12, 14}
			s.Course = &tour.Course{Par: par, SI: si}
		case "bowling":
			s.Mode, s.Rounds = "pins", 3
		default:
			s.Mode = "plain"
		}
	case "ladder", "box_league":
		s.Schedule = "none"
	case "arena":
		s.Minutes = 60
	}
	if g.Sport == "chess" {
		s.Schedule = "none"
	}
	return s
}

// tView is a tournament with what its pages show.
type tView struct {
	*store.Tournament
	FormatName string `json:"format_name"`
	Sport      string `json:"sport"`
	Emoji      string `json:"emoji"`
	GroupName  string `json:"group_name"`
	Entries    int    `json:"entries"`
	Mine       bool   `json:"mine"` // I am in it
}

func (s *Server) tView(ctx context.Context, t *store.Tournament, g *store.Group, uid int64) tView {
	f, _ := tour.FormatByKey(t.Format)
	v := tView{Tournament: t, FormatName: f.Name, Sport: league.DisplayName(g.Sport, g.SportName), Emoji: league.SportByKey(g.Sport).Emoji, GroupName: g.Name}
	es, _ := s.store.Entries(ctx, t.ID)
	for _, e := range es {
		if e.Status == "in" {
			v.Entries++
		}
		for _, m := range e.Members {
			if m.UserID == uid && e.Status != "withdrawn" {
				v.Mine = true
			}
		}
	}
	return v
}

// canRun: the group's organisers and whoever made the tournament.
func canRun(t *store.Tournament, g *store.Group, uid int64) bool {
	return organiser(g) || t.UserID == uid
}

// ---- the draw ----

// seedEntries orders the entries for the draw.
func seedEntries(es []*store.TEntry, how string, seed int64) []*store.TEntry {
	out := append([]*store.TEntry{}, es...)
	switch how {
	case "random":
		rand.New(rand.NewSource(seed)).Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	default: // rating, with manual seeds first
		rand.New(rand.NewSource(seed)).Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		sort.SliceStable(out, func(i, j int) bool { return out[i].Rating > out[j].Rating })
	}
	if how != "random" {
		sort.SliceStable(out, func(i, j int) bool {
			a, b := out[i].Seed, out[j].Seed
			if (a > 0) != (b > 0) {
				return a > 0
			}
			if a > 0 {
				return a < b
			}
			return false
		})
	}
	return out
}

// drawable are the entries that go into the draw: in (and checked in when that is asked).
func (s *Server) drawable(ctx context.Context, t *store.Tournament) ([]*store.TEntry, error) {
	es, err := s.store.Entries(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	var out []*store.TEntry
	for _, e := range es {
		if e.Status == "in" && (!t.Checkin || e.CheckedIn) {
			out = append(out, e)
		}
	}
	return out, nil
}

// refreshRatings puts each entry's current rating in it (the average of its people).
func (s *Server) refreshRatings(ctx context.Context, g *store.Group, es []*store.TEntry) {
	r := s.ratings(ctx, g)
	for _, e := range es {
		if len(e.Members) == 0 {
			continue
		}
		var sum float64
		for _, m := range e.Members {
			v := r[m.UserID]
			if v == 0 {
				v = league.StartRating
			}
			sum += v
		}
		e.Rating = sum / float64(len(e.Members))
		_ = s.store.SaveEntry(ctx, e)
	}
}

// minEntries is the fewest entries a format needs.
func minEntries(format string) int {
	switch format {
	case "americano", "mexicano", "kotc":
		return 4
	case "double_elim", "groups_knockout":
		return 3
	case "ranking":
		return 1
	}
	return 2
}

// draw makes the first stage and its matches, and starts the tournament.
func (s *Server) draw(ctx context.Context, t *store.Tournament, g *store.Group) error {
	es, err := s.drawable(ctx, t)
	if err != nil {
		return err
	}
	if len(es) < minEntries(t.Format) {
		return fmt.Errorf("%s needs at least %d entries (%d now)", t.Format, minEntries(t.Format), len(es))
	}
	if t.Format == "kotc" && len(es)%4 != 0 {
		return fmt.Errorf("King of the Court needs a multiple of four players (%d now)", len(es))
	}
	s.refreshRatings(ctx, g, es)
	set := settingsOf(t)
	seeded := seedEntries(es, set.Seeding, time.Now().UnixNano())
	ids := make([]int64, len(seeded))
	for i, e := range seeded {
		ids[i] = e.ID
	}
	_ = s.store.DeleteStagesFrom(ctx, t.ID, 0)
	switch t.Format {
	case "round_robin", "groups_knockout", "box_league", "team_ties":
		k := set.Groups
		if t.Format == "round_robin" || t.Format == "team_ties" {
			k = max(1, set.Groups)
			if set.Groups == 0 {
				k = 1
			}
		}
		var groups [][]int64
		names := []string{}
		if t.Format == "box_league" {
			size := max(3, set.BoxSize)
			for i := 0; i < len(ids); i += size {
				end := min(len(ids), i+size)
				groups = append(groups, ids[i:end])
				names = append(names, fmt.Sprintf("Box %d", len(groups)))
			}
			// a last box of one or two joins the one above
			if n := len(groups); n > 1 && len(groups[n-1]) < 3 {
				groups[n-2] = append(groups[n-2], groups[n-1]...)
				groups = groups[:n-1]
				names = names[:n-1]
			}
		} else {
			if k > len(ids)/2 {
				k = max(1, len(ids)/2)
			}
			groups = tour.Snake(ids, k)
		}
		kind := "round_robin"
		if t.Format == "team_ties" {
			kind = "team_ties"
		}
		st := &store.TStage{TournamentID: t.ID, Seq: 1, Kind: kind, Name: map[bool]string{true: "Groups", false: "Matches"}[len(groups) > 1], Status: "live"}
		if t.Format == "box_league" {
			st.Name = "Boxes"
		}
		if err := s.store.SaveStage(ctx, st); err != nil {
			return err
		}
		if err := s.savePlan(ctx, t, st, tour.RoundRobin(groups, names, set.Double)); err != nil {
			return err
		}
	case "knockout":
		st := &store.TStage{TournamentID: t.ID, Seq: 1, Kind: "knockout", Name: "Knockout", Status: "live"}
		if err := s.store.SaveStage(ctx, st); err != nil {
			return err
		}
		if err := s.savePlan(ctx, t, st, tour.Knockout(ids, set.ThirdPlace, set.Plate)); err != nil {
			return err
		}
	case "double_elim":
		st := &store.TStage{TournamentID: t.ID, Seq: 1, Kind: "double_elim", Name: "Double elimination", Status: "live"}
		if err := s.store.SaveStage(ctx, st); err != nil {
			return err
		}
		if err := s.savePlan(ctx, t, st, tour.DoubleElimination(ids, set.Reset)); err != nil {
			return err
		}
	case "americano", "mexicano", "kotc", "swiss", "arena":
		st := &store.TStage{TournamentID: t.ID, Seq: 1, Kind: t.Format, Name: map[string]string{"americano": "Americano", "mexicano": "Mexicano", "kotc": "King of the Court", "swiss": "Swiss", "arena": "Arena"}[t.Format], Status: "live"}
		if err := s.store.SaveStage(ctx, st); err != nil {
			return err
		}
		g := &store.TGroup{StageID: st.ID, Name: st.Name, Kind: "pool"}
		if err := s.store.AddTGroup(ctx, g); err != nil {
			return err
		}
		list := make([]store.StageEntry, len(ids))
		for i, id := range ids {
			list[i] = store.StageEntry{StageID: st.ID, GroupID: g.ID, EntryID: id, Seed: i + 1}
		}
		if err := s.store.SetStageEntries(ctx, st.ID, list); err != nil {
			return err
		}
		if t.Format == "americano" {
			if err := s.americanoAll(ctx, t, st, g.ID, ids, set); err != nil {
				return err
			}
		} else if t.Format == "arena" {
			if err := s.arenaPair(ctx, t, st); err != nil {
				return err
			}
		} else if err := s.nextRound(ctx, t, st); err != nil {
			return err
		}
	case "ranking":
		st := &store.TStage{TournamentID: t.ID, Seq: 1, Kind: "ranking", Name: "Leaderboard", Status: "live"}
		if err := s.store.SaveStage(ctx, st); err != nil {
			return err
		}
		g := &store.TGroup{StageID: st.ID, Name: "Leaderboard", Kind: "pool"}
		if err := s.store.AddTGroup(ctx, g); err != nil {
			return err
		}
		list := make([]store.StageEntry, len(ids))
		for i, id := range ids {
			list[i] = store.StageEntry{StageID: st.ID, GroupID: g.ID, EntryID: id, Seed: i + 1}
		}
		if err := s.store.SetStageEntries(ctx, st.ID, list); err != nil {
			return err
		}
	case "ladder":
		st := &store.TStage{TournamentID: t.ID, Seq: 1, Kind: "ladder", Name: "Ladder", Status: "live"}
		if err := s.store.SaveStage(ctx, st); err != nil {
			return err
		}
		g := &store.TGroup{StageID: st.ID, Name: "Ladder", Kind: "pool"}
		if err := s.store.AddTGroup(ctx, g); err != nil {
			return err
		}
		list := make([]store.StageEntry, len(seeded))
		for i, e := range seeded {
			list[i] = store.StageEntry{StageID: st.ID, GroupID: g.ID, EntryID: e.ID, Seed: i + 1}
			e.Position = i + 1
			_ = s.store.SaveEntry(ctx, e)
		}
		if err := s.store.SetStageEntries(ctx, st.ID, list); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown format")
	}
	t.Status = "live"
	if err := s.store.UpdateTournament(ctx, t); err != nil {
		return err
	}
	if err := s.settleByes(ctx, t.ID); err != nil {
		return err
	}
	s.schedule(ctx, t)
	return nil
}

// savePlan saves a planned stage: its groups, who is in which, the matches
// and the links between them.
func (s *Server) savePlan(ctx context.Context, t *store.Tournament, st *store.TStage, p tour.Plan) error {
	gids := make([]int64, len(p.Groups))
	for i, pg := range p.Groups {
		g := &store.TGroup{StageID: st.ID, Name: pg.Name, Kind: pg.Kind, Position: i}
		if err := s.store.AddTGroup(ctx, g); err != nil {
			return err
		}
		gids[i] = g.ID
	}
	ids := make([]int64, len(p.Matches))
	seen := map[int64]bool{}
	var entries []store.StageEntry
	for i, pm := range p.Matches {
		m := &store.TMatch{TournamentID: t.ID, StageID: st.ID, GroupID: gids[pm.Group], Round: pm.Round, Position: pm.Position, Label: pm.Label,
			AEntry: pm.A.Entry, BEntry: pm.B.Entry, ASource: pm.A.Source, BSource: pm.B.Source,
			Sides: map[string][]int64{"a": pm.Players[0], "b": pm.Players[1]}}
		if m.AEntry != 0 && m.BEntry != 0 {
			m.Status = "ready"
		}
		if err := s.store.SaveTMatch(ctx, m); err != nil {
			return err
		}
		ids[i] = m.ID
		for _, e := range []int64{pm.A.Entry, pm.B.Entry} {
			if e != 0 && !seen[e] {
				seen[e] = true
				entries = append(entries, store.StageEntry{StageID: st.ID, GroupID: gids[pm.Group], EntryID: e, Seed: len(entries) + 1})
			}
		}
	}
	// the links
	for i, pm := range p.Matches {
		if pm.Next < 0 && pm.Loser < 0 {
			continue
		}
		m, err := s.store.GetTMatch(ctx, ids[i])
		if err != nil {
			return err
		}
		if pm.Next >= 0 {
			m.NextMatch, m.NextSlot = ids[pm.Next], pm.NextSlot
		}
		if pm.Loser >= 0 {
			m.LoserMatch, m.LoserSlot = ids[pm.Loser], pm.LoserSlot
		}
		if err := s.store.SaveTMatch(ctx, m); err != nil {
			return err
		}
	}
	if st.Kind == "team_ties" {
		if err := s.addRubbers(ctx, t, st); err != nil {
			return err
		}
	}
	return s.store.SetStageEntries(ctx, st.ID, entries)
}

// addRubbers gives every tie its matches (singles 1, singles 2, doubles …).
func (s *Server) addRubbers(ctx context.Context, t *store.Tournament, st *store.TStage) error {
	set := settingsOf(t)
	ties := set.Ties
	if len(ties) == 0 {
		ties = []string{"singles", "singles", "doubles"}
	}
	ms, err := s.store.TMatches(ctx, t.ID)
	if err != nil {
		return err
	}
	for _, tie := range ms {
		if tie.StageID != st.ID || tie.ParentID != 0 {
			continue
		}
		count := map[string]int{}
		for i, kind := range ties {
			count[kind]++
			r := &store.TMatch{TournamentID: t.ID, StageID: st.ID, GroupID: tie.GroupID, ParentID: tie.ID, Round: tie.Round, Position: i + 1,
				Label: fmt.Sprintf("%s %d", strings.ToUpper(kind[:1])+kind[1:], count[kind]), AEntry: tie.AEntry, BEntry: tie.BEntry, Status: tie.Status}
			if err := s.store.SaveTMatch(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- the rotating formats ----

// rotHistory is who partnered and played whom, and who sat out, in a stage so far.
func (s *Server) rotHistory(ctx context.Context, stageID int64, all []*store.TMatch, players []int64) (*tour.History, int) {
	h := tour.NewHistory()
	rounds := map[int][]tour.Court{}
	played := map[int]map[int64]bool{}
	last := 0
	for _, m := range all {
		if m.StageID != stageID || len(m.Sides["a"]) != 2 || len(m.Sides["b"]) != 2 {
			continue
		}
		c := tour.Court{{m.Sides["a"][0], m.Sides["a"][1]}, {m.Sides["b"][0], m.Sides["b"][1]}}
		rounds[m.Round] = append(rounds[m.Round], c)
		if played[m.Round] == nil {
			played[m.Round] = map[int64]bool{}
		}
		for _, p := range append(m.Sides["a"], m.Sides["b"]...) {
			played[m.Round][p] = true
		}
		last = max(last, m.Round)
	}
	for r := 1; r <= last; r++ {
		var sit []int64
		for _, p := range players {
			if !played[r][p] {
				sit = append(sit, p)
			}
		}
		h.Add(rounds[r], sit)
	}
	return h, last
}

// courtsFor is how many courts a rotating round may use.
func (s *Server) courtsFor(ctx context.Context, t *store.Tournament) int {
	cs, _ := s.store.Courts(ctx, t.ID)
	n := 0
	for _, c := range cs {
		if c.Active {
			n++
		}
	}
	return n
}

// americanoAll draws every round of an Americano at the start, so everyone
// sees when they play with whom.
func (s *Server) americanoAll(ctx context.Context, t *store.Tournament, st *store.TStage, gid int64, players []int64, set TSettings) error {
	rounds := set.Rounds
	if rounds <= 0 {
		rounds = len(players) - 1
	}
	h := tour.NewHistory()
	courts := s.courtsFor(ctx, t)
	for r := 1; r <= rounds; r++ {
		cs, sit := tour.AmericanoRound(players, courts, h, int64(t.ID*100+int64(r)))
		h.Add(cs, sit)
		if err := s.saveCourts(ctx, t, st, gid, r, cs); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) saveCourts(ctx context.Context, t *store.Tournament, st *store.TStage, gid int64, round int, cs []tour.Court) error {
	for i, c := range cs {
		m := &store.TMatch{TournamentID: t.ID, StageID: st.ID, GroupID: gid, Round: round, Position: i + 1, Label: fmt.Sprintf("Round %d, court %d", round, i+1),
			Sides: map[string][]int64{"a": {c[0][0], c[0][1]}, "b": {c[1][0], c[1][1]}}, Status: "ready"}
		if err := s.store.SaveTMatch(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// stageIDs are the entries of a stage in seed order.
func (s *Server) stageIDs(ctx context.Context, t *store.Tournament, stageID int64) []int64 {
	xs, _ := s.store.StageEntries(ctx, t.ID)
	es, _ := s.store.Entries(ctx, t.ID)
	out := map[int64]bool{}
	for _, e := range es {
		if e.Status == "withdrawn" || e.Status == "disqualified" {
			out[e.ID] = true
		}
	}
	var ids []int64
	for _, x := range xs {
		if x.StageID == stageID && !out[x.EntryID] {
			ids = append(ids, x.EntryID)
		}
	}
	return ids
}

// nextRound draws the next round of the formats drawn round by round
// (Mexicano, King of the Court, Swiss); it does nothing while a round is
// still being played, and finishes the stage after the last.
func (s *Server) nextRound(ctx context.Context, t *store.Tournament, st *store.TStage) error {
	set := settingsOf(t)
	all, err := s.store.TMatches(ctx, t.ID)
	if err != nil {
		return err
	}
	var mine []*store.TMatch
	last := 0
	for _, m := range all {
		if m.StageID != st.ID {
			continue
		}
		mine = append(mine, m)
		if m.Round > last {
			last = m.Round
		}
		if m.Status != "done" && m.Status != "void" {
			return nil // a round is still on
		}
	}
	rounds := set.Rounds
	if rounds <= 0 {
		rounds = 5
	}
	if last >= rounds {
		return s.finishStage(ctx, t, st)
	}
	players := s.stageIDs(ctx, t, st.ID)
	groups, _ := s.store.TGroups(ctx, t.ID)
	var gid int64
	for _, g := range groups {
		if g.StageID == st.ID {
			gid = g.ID
		}
	}
	round := last + 1
	switch st.Kind {
	case "mexicano":
		h, _ := s.rotHistory(ctx, st.ID, all, players)
		ranked := players
		if last > 0 {
			ranked = nil
			for _, r := range s.stageStandings(ctx, t, st, all, nil)[gid] {
				ranked = append(ranked, r.ID)
			}
		} else {
			rand.New(rand.NewSource(t.ID)).Shuffle(len(ranked), func(i, j int) { ranked[i], ranked[j] = ranked[j], ranked[i] })
		}
		cs, _ := tour.MexicanoRound(ranked, s.courtsFor(ctx, t), h, set.Split, t.ID+int64(round))
		return s.saveCourts(ctx, t, st, gid, round, cs)
	case "kotc":
		var cs []tour.Court
		if last == 0 {
			for i := 0; i+3 < len(players); i += 4 {
				q := players[i : i+4]
				cs = append(cs, tour.Court{{q[0], q[3]}, {q[1], q[2]}})
			}
		} else {
			var res []tour.KOTCResult
			for _, m := range mine {
				if m.Round == last && len(m.Sides["a"]) == 2 {
					w := 0
					if m.Winner == "b" {
						w = 1
					}
					res = append(res, tour.KOTCResult{Court: tour.Court{{m.Sides["a"][0], m.Sides["a"][1]}, {m.Sides["b"][0], m.Sides["b"][1]}}, Winner: w})
				}
			}
			sort.SliceStable(res, func(i, j int) bool { return i < j })
			cs = tour.KOTCRound(res)
		}
		return s.saveCourts(ctx, t, st, gid, round, cs)
	case "swiss":
		played := map[[2]int64]bool{}
		byes := map[int64]bool{}
		colours := map[int64][]string{}
		for _, m := range mine {
			if m.AEntry != 0 && m.BEntry != 0 {
				a, b := m.AEntry, m.BEntry
				if a > b {
					a, b = b, a
				}
				played[[2]int64{a, b}] = true
				if m.White != "" {
					wa, wb := "b", "w"
					if m.White == "a" {
						wa, wb = "w", "b"
					}
					colours[m.AEntry] = append(colours[m.AEntry], wa)
					colours[m.BEntry] = append(colours[m.BEntry], wb)
				}
			}
			if m.Outcome == "bye" {
				byes[m.AEntry] = true
			}
		}
		rows := s.stageStandings(ctx, t, st, all, nil)[gid]
		score := map[int64]float64{}
		for _, r := range rows {
			score[r.ID] = r.Points
		}
		es, _ := s.store.Entries(ctx, t.ID)
		rating := map[int64]float64{}
		for _, e := range es {
			rating[e.ID] = e.Rating
		}
		var se []tour.SwissEntry
		for _, id := range players {
			c := colours[id]
			x := tour.SwissEntry{ID: id, Score: score[id], Rating: rating[id], HadBye: byes[id]}
			for _, v := range c {
				if v == "w" {
					x.Whites++
				} else {
					x.Blacks++
				}
			}
			if n := len(c); n > 0 {
				x.Last = c[n-1]
				if n > 1 {
					x.Last2 = c[n-2]
				}
			}
			se = append(se, x)
		}
		pairs, bye := tour.SwissRound(se, played, set.Colours)
		for i, p := range pairs {
			m := &store.TMatch{TournamentID: t.ID, StageID: st.ID, GroupID: gid, Round: round, Position: i + 1, Label: fmt.Sprintf("Round %d, board %d", round, i+1),
				AEntry: p.A, BEntry: p.B, White: p.White, Status: "ready"}
			if err := s.store.SaveTMatch(ctx, m); err != nil {
				return err
			}
		}
		if bye != 0 {
			now := store.Now()
			m := &store.TMatch{TournamentID: t.ID, StageID: st.ID, GroupID: gid, Round: round, Position: len(pairs) + 1, Label: fmt.Sprintf("Round %d, bye", round),
				AEntry: bye, BSource: tour.Bye, Status: "done", Outcome: "bye", Winner: "a", ConfirmedAt: &now}
			if err := s.store.SaveTMatch(ctx, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// arenaPair pairs everyone in an arena who is not playing now.
func (s *Server) arenaPair(ctx context.Context, t *store.Tournament, st *store.TStage) error {
	if t.EndsAt != "" && store.LocalNow() >= t.EndsAt {
		return nil
	}
	all, err := s.store.TMatches(ctx, t.ID)
	if err != nil {
		return err
	}
	busy := map[int64]bool{}
	last := map[int64]int64{}
	results := map[int64][]string{}
	round := 0
	for _, m := range all {
		if m.StageID != st.ID {
			continue
		}
		round = max(round, m.Round)
		if m.Status != "done" && m.Status != "void" {
			busy[m.AEntry], busy[m.BEntry] = true, true
			continue
		}
		last[m.AEntry], last[m.BEntry] = m.BEntry, m.AEntry
		switch m.Winner {
		case "a":
			results[m.AEntry] = append(results[m.AEntry], "W")
			results[m.BEntry] = append(results[m.BEntry], "L")
		case "b":
			results[m.AEntry] = append(results[m.AEntry], "L")
			results[m.BEntry] = append(results[m.BEntry], "W")
		case "draw":
			results[m.AEntry] = append(results[m.AEntry], "D")
			results[m.BEntry] = append(results[m.BEntry], "D")
		}
	}
	var free []tour.SwissEntry
	for _, id := range s.stageIDs(ctx, t, st.ID) {
		if !busy[id] {
			p, _ := tour.ArenaPoints(results[id])
			free = append(free, tour.SwissEntry{ID: id, Score: p})
		}
	}
	groups, _ := s.store.TGroups(ctx, t.ID)
	var gid int64
	for _, g := range groups {
		if g.StageID == st.ID {
			gid = g.ID
		}
	}
	for i, pr := range tour.ArenaPick(free, last) {
		round++
		m := &store.TMatch{TournamentID: t.ID, StageID: st.ID, GroupID: gid, Round: round, Position: i + 1, Label: fmt.Sprintf("Game %d", round),
			AEntry: pr[0], BEntry: pr[1], Status: "ready"}
		if settingsOf(t).Colours {
			m.White = "a"
			if rand.Intn(2) == 1 {
				m.White = "b"
			}
		}
		if err := s.store.SaveTMatch(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// ---- results and what follows ----

// settleByes plays the matches with a bye on one side (the other side goes
// on) and passes "nobody" on when both are byes.
func (s *Server) settleByes(ctx context.Context, tid int64) error {
	es, _ := s.store.Entries(ctx, tid)
	gone := map[int64]bool{}
	for _, e := range es {
		if e.Status == "withdrawn" || e.Status == "disqualified" {
			gone[e.ID] = true
		}
	}
	for pass := 0; pass < 12; pass++ {
		ms, err := s.store.TMatches(ctx, tid)
		if err != nil {
			return err
		}
		changed := false
		for _, m := range ms {
			if m.Status == "done" || m.Status == "void" || m.ParentID != 0 {
				continue
			}
			// someone who withdrew: whoever they would have met goes on by walkover
			if m.AEntry != 0 && m.BEntry != 0 && (gone[m.AEntry] != gone[m.BEntry]) {
				now := store.Now()
				m.Status, m.Outcome, m.ConfirmedAt = "done", "walkover", &now
				w, l := m.AEntry, m.BEntry
				m.Winner = "a"
				if gone[m.AEntry] {
					m.Winner, w, l = "b", m.BEntry, m.AEntry
				}
				if err := s.store.SaveTMatch(ctx, m); err != nil {
					return err
				}
				s.passOn(ctx, m.NextMatch, m.NextSlot, w)
				s.passOn(ctx, m.LoserMatch, m.LoserSlot, l)
				changed = true
				continue
			}
			aBye, bBye := m.ASource == tour.Bye && m.AEntry == 0, m.BSource == tour.Bye && m.BEntry == 0
			now := store.Now()
			switch {
			case aBye && bBye:
				m.Status, m.Outcome = "void", "bye"
				if err := s.store.SaveTMatch(ctx, m); err != nil {
					return err
				}
				s.passOn(ctx, m.NextMatch, m.NextSlot, 0)
				s.passOn(ctx, m.LoserMatch, m.LoserSlot, 0)
				changed = true
			case aBye && m.BEntry != 0, bBye && m.AEntry != 0:
				m.Status, m.Outcome, m.ConfirmedAt = "done", "bye", &now
				m.Winner = "a"
				if aBye {
					m.Winner = "b"
				}
				if err := s.store.SaveTMatch(ctx, m); err != nil {
					return err
				}
				w := m.AEntry
				if m.Winner == "b" {
					w = m.BEntry
				}
				s.passOn(ctx, m.NextMatch, m.NextSlot, w)
				s.passOn(ctx, m.LoserMatch, m.LoserSlot, 0)
				changed = true
			}
		}
		if !changed {
			return nil
		}
	}
	return nil
}

// passOn puts an entry (0 = a bye) in a slot of a later match.
func (s *Server) passOn(ctx context.Context, mid int64, slot string, entry int64) {
	if mid == 0 || slot == "" {
		return
	}
	m, err := s.store.GetTMatch(ctx, mid)
	if err != nil {
		return
	}
	if slot == "a" {
		m.AEntry = entry
		if entry == 0 {
			m.ASource = tour.Bye
		}
	} else {
		m.BEntry = entry
		if entry == 0 {
			m.BSource = tour.Bye
		}
	}
	if m.AEntry != 0 && m.BEntry != 0 && m.Status == "pending" {
		m.Status = "ready"
	}
	_ = s.store.SaveTMatch(ctx, m)
}

// confirmed is what happens once a result counts: the bracket moves on, the
// court is free, the group's table gets the match (when rated), the next
// round is drawn and the tournament finishes after its last match.
func (s *Server) confirmed(ctx context.Context, t *store.Tournament, g *store.Group, m *store.TMatch, by int64) {
	if m.ParentID != 0 {
		s.settleTie(ctx, t, g, m.ParentID, by)
		return
	}
	stages, _ := s.store.Stages(ctx, t.ID)
	var st *store.TStage
	for _, x := range stages {
		if x.ID == m.StageID {
			st = x
		}
	}
	if st == nil {
		return
	}
	w, l := m.AEntry, m.BEntry
	if m.Winner == "b" {
		w, l = l, w
	}
	if m.Winner == "draw" {
		w, l = 0, 0
	}
	// double elimination: the grand final's reset is played only when the losers' side wins
	if st.Kind == "double_elim" && m.Label == "Grand final" {
		ms, _ := s.store.TMatches(ctx, t.ID)
		for _, r := range ms {
			if r.StageID == st.ID && r.Label == "Grand final, reset" {
				if m.Winner == "b" {
					r.AEntry, r.BEntry, r.Status = m.AEntry, m.BEntry, "ready"
				} else {
					r.Status, r.Outcome = "void", "not needed"
				}
				_ = s.store.SaveTMatch(ctx, r)
			}
		}
	}
	if w != 0 {
		s.passOn(ctx, m.NextMatch, m.NextSlot, w)
	}
	if l != 0 {
		s.passOn(ctx, m.LoserMatch, m.LoserSlot, l)
	}
	_ = s.settleByes(ctx, t.ID)
	if t.Rated {
		s.rateMatch(ctx, t, g, m, by)
	}
	if st.Kind == "ladder" {
		s.ladderResult(ctx, t, m)
	}
	switch st.Kind {
	case "mexicano", "kotc", "swiss":
		_ = s.nextRound(ctx, t, st)
	case "arena":
		_ = s.arenaPair(ctx, t, st)
	}
	s.checkStage(ctx, t, g, st)
	s.schedule(ctx, t)
}

// settleTie decides a team tie once its matches are played.
func (s *Server) settleTie(ctx context.Context, t *store.Tournament, g *store.Group, tieID, by int64) {
	tie, err := s.store.GetTMatch(ctx, tieID)
	if err != nil || tie.Status == "done" {
		return
	}
	ms, _ := s.store.TMatches(ctx, t.ID)
	wa, wb, open := 0, 0, 0
	for _, r := range ms {
		if r.ParentID != tieID || r.Status == "void" {
			continue
		}
		switch {
		case r.Status != "done":
			open++
		case r.Winner == "a":
			wa++
		case r.Winner == "b":
			wb++
		}
	}
	if open > 0 {
		return
	}
	fa, fb := float64(wa), float64(wb)
	tie.Score = store.Score{A: &fa, B: &fb}
	tie.Winner = "draw"
	if wa > wb {
		tie.Winner = "a"
	} else if wb > wa {
		tie.Winner = "b"
	}
	now := store.Now()
	tie.Status, tie.ConfirmedAt, tie.Outcome = "done", &now, "normal"
	_ = s.store.SaveTMatch(ctx, tie)
	s.confirmed(ctx, t, g, tie, by)
}

// checkStage finishes a stage when all its matches are played (except the
// formats that draw their own next rounds, which do that themselves).
func (s *Server) checkStage(ctx context.Context, t *store.Tournament, g *store.Group, st *store.TStage) {
	if st.Status == "done" {
		return
	}
	switch st.Kind {
	case "mexicano", "kotc", "swiss", "ladder", "arena", "ranking":
		return
	}
	ms, _ := s.store.TMatches(ctx, t.ID)
	for _, m := range ms {
		if m.StageID == st.ID && m.Status != "done" && m.Status != "void" {
			return
		}
	}
	_ = s.finishStage(ctx, t, st)
}

// finishStage notes the final places of a stage, then draws the next stage
// (groups → knockout) or finishes the tournament.
func (s *Server) finishStage(ctx context.Context, t *store.Tournament, st *store.TStage) error {
	st.Status = "done"
	if err := s.store.SaveStage(ctx, st); err != nil {
		return err
	}
	all, _ := s.store.TMatches(ctx, t.ID)
	ranks := s.finalRanks(ctx, t, st, all)
	for id, r := range ranks {
		_ = s.store.SetFinalRank(ctx, st.ID, id, r)
	}
	g, err := s.store.GroupByID(ctx, t.GroupID)
	if err != nil {
		return err
	}
	if t.Format == "groups_knockout" && st.Seq == 1 {
		return s.knockoutAfterGroups(ctx, t, g, st, all)
	}
	return s.finishTournament(ctx, t, g, st, ranks)
}

// knockoutAfterGroups draws the knockout from the best of each group:
// group winners are the top seeds, and two from the same group are kept
// apart for as long as can be.
func (s *Server) knockoutAfterGroups(ctx context.Context, t *store.Tournament, g *store.Group, st *store.TStage, all []*store.TMatch) error {
	set := settingsOf(t)
	adv := max(1, set.Advance)
	table := s.stageStandings(ctx, t, st, all, nil)
	groups, _ := s.store.TGroups(ctx, t.ID)
	var seeded []int64
	for place := 0; place < adv; place++ {
		for _, tg := range groups {
			if tg.StageID != st.ID {
				continue
			}
			rows := table[tg.ID]
			if place < len(rows) {
				seeded = append(seeded, rows[place].ID)
			}
		}
	}
	if len(seeded) < 2 {
		return s.finishTournament(ctx, t, g, st, s.finalRanks(ctx, t, st, all))
	}
	ko := &store.TStage{TournamentID: t.ID, Seq: 2, Kind: "knockout", Name: "Knockout", Status: "live"}
	if err := s.store.SaveStage(ctx, ko); err != nil {
		return err
	}
	if err := s.savePlan(ctx, t, ko, tour.Knockout(seeded, set.ThirdPlace, set.Plate)); err != nil {
		return err
	}
	_ = s.settleByes(ctx, t.ID)
	s.systemMessage(ctx, g, 0, "tournament", fmt.Sprintf("%s: the groups are done — %d go through to the knockout", t.Name, len(seeded)), map[string]any{"tournament_id": t.ID})
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	return nil
}

// finalRanks are the places at the end of a stage: the table, or for a
// knockout the final, the match for third place and how far each got.
func (s *Server) finalRanks(ctx context.Context, t *store.Tournament, st *store.TStage, all []*store.TMatch) map[int64]int {
	out := map[int64]int{}
	switch st.Kind {
	case "knockout", "double_elim":
		var mine []*store.TMatch
		for _, m := range all {
			if m.StageID == st.ID && m.ParentID == 0 {
				mine = append(mine, m)
			}
		}
		groups, _ := s.store.TGroups(ctx, t.ID)
		kind := map[int64]string{}
		for _, g := range groups {
			kind[g.ID] = g.Kind
		}
		var final, third *store.TMatch
		for _, m := range mine {
			if m.Status != "done" {
				continue
			}
			switch {
			case m.Label == "Grand final, reset" || (m.Label == "Grand final" && final == nil) || (m.Label == "Final" && kind[m.GroupID] == "main"):
				final = m
			case m.Label == "Third place":
				third = m
			}
		}
		place := func(m *store.TMatch, first int) {
			if m == nil || m.Winner == "" || m.Winner == "draw" {
				return
			}
			w, l := m.AEntry, m.BEntry
			if m.Winner == "b" {
				w, l = l, w
			}
			out[w], out[l] = first, first+1
		}
		place(final, 1)
		place(third, 3)
		// the rest by the round they went out in
		maxRound := 0
		for _, m := range mine {
			if kind[m.GroupID] == "main" || kind[m.GroupID] == "wb" || kind[m.GroupID] == "lb" {
				maxRound = max(maxRound, m.Round)
			}
		}
		for _, m := range mine {
			if m.Status != "done" || m.Winner == "" || m.Winner == "draw" || (kind[m.GroupID] != "main" && kind[m.GroupID] != "lb") {
				continue
			}
			l := m.BEntry
			if m.Winner == "b" {
				l = m.AEntry
			}
			if l != 0 && out[l] == 0 {
				out[l] = 1 << uint(max(0, maxRound-m.Round+1))
				if kind[m.GroupID] == "lb" {
					out[l] = 2 + max(0, 20-m.Round)
				}
			}
		}
	case "ranking":
		for _, x := range s.leaderboard(ctx, t, st) {
			out[x.ID] = x.Rank
		}
	case "ladder":
		es, _ := s.store.Entries(ctx, t.ID)
		for _, e := range es {
			if e.Status == "in" {
				out[e.ID] = e.Position
			}
		}
	default:
		for _, rows := range s.stageStandings(ctx, t, st, all, nil) {
			for _, r := range rows {
				out[r.ID] = r.Rank
			}
		}
	}
	return out
}

// finishTournament names the winner, tells the group with a podium and
// files it on the players' pages.
func (s *Server) finishTournament(ctx context.Context, t *store.Tournament, g *store.Group, st *store.TStage, ranks map[int64]int) error {
	es, _ := s.store.Entries(ctx, t.ID)
	byID := map[int64]*store.TEntry{}
	for _, e := range es {
		byID[e.ID] = e
	}
	type place struct {
		r    int
		name string
	}
	var podium []place
	for id, r := range ranks {
		if r >= 1 && r <= 3 && byID[id] != nil {
			podium = append(podium, place{r, byID[id].Name})
		}
	}
	sort.Slice(podium, func(i, j int) bool { return podium[i].r < podium[j].r })
	t.Status = "finished"
	if len(podium) > 0 && podium[0].r == 1 {
		t.Winner = podium[0].name
	}
	if err := s.store.UpdateTournament(ctx, t); err != nil {
		return err
	}
	var lines []string
	medal := map[int]string{1: "🥇", 2: "🥈", 3: "🥉"}
	for _, p := range podium {
		lines = append(lines, medal[p.r]+" "+p.name)
	}
	text := t.Name + " is over"
	if len(lines) > 0 {
		text += ": " + strings.Join(lines, "  ")
	}
	s.systemMessage(ctx, g, 0, "tournament_finished", text, map[string]any{"tournament_id": t.ID})
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID})
	s.tellGroup(ctx, g, 0, note{kind: "results", title: t.Name + ": " + firstNonEmpty(t.Winner, "it's over"), body: strings.Join(lines, "\n"), link: s.tLink(t.ID), button: "See it all"})
	s.publishGroup(ctx, g, "league.tournament_finished", fmt.Sprintf("tournament:%d:finished", t.ID), map[string]any{"tournament_id": t.ID, "group": g.Name, "name": t.Name, "winner": t.Winner, "url": s.tLink(t.ID)})
	s.store.AddTLog(ctx, t.ID, 0, "finished", text)
	return nil
}

func (s *Server) tLink(id int64) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/tournaments/" + itoa(id)
}

// ---- the table of a stage ----

// tResults turns a stage's played matches into what the table counts.
func tResults(st *store.TStage, all []*store.TMatch, groupID int64) []tour.Result {
	var out []tour.Result
	for _, m := range all {
		if m.StageID != st.ID || m.ParentID != 0 || (groupID != 0 && m.GroupID != groupID) || m.Status != "done" {
			continue
		}
		r := tour.Result{Winner: m.Winner, Walkover: m.Outcome == "walkover" || m.Outcome == "no_show", Bye: m.Outcome == "bye"}
		if len(m.Sides["a"]) > 0 {
			r.A, r.B = m.Sides["a"], m.Sides["b"]
		} else {
			r.A, r.B = []int64{m.AEntry}, []int64{m.BEntry}
			if m.BEntry == 0 {
				r.B = nil
			}
		}
		switch {
		case len(m.Score.Sets) > 0:
			for _, x := range m.Score.Sets {
				r.ForA += float64(x[0])
				r.ForB += float64(x[1])
				if x[0] > x[1] {
					r.SetsA++
				} else if x[1] > x[0] {
					r.SetsB++
				}
			}
		case m.Score.A != nil && m.Score.B != nil:
			r.ForA, r.ForB = *m.Score.A, *m.Score.B
		}
		if m.ATotal != nil && m.BTotal != nil {
			r.ForA, r.ForB = *m.ATotal, *m.BTotal
		}
		out = append(out, r)
	}
	return out
}

// stageStandings are the tables of a stage, per group.
func (s *Server) stageStandings(ctx context.Context, t *store.Tournament, st *store.TStage, all []*store.TMatch, g *store.Group) map[int64][]*tour.Row {
	if g == nil {
		g, _ = s.store.GroupByID(ctx, t.GroupID)
	}
	set := settingsOf(t)
	rules := tour.RulesFor(g.Sport, g.Scoring, st.Kind)
	if st.Kind == "team_ties" {
		rules = tour.Rules{Win: 2, Draw: 1, Tiebreaks: []string{"points", "diff", "h2h"}}
	}
	if len(set.Tiebreaks) > 0 {
		rules.Tiebreaks = set.Tiebreaks
	}
	es, _ := s.store.Entries(ctx, t.ID)
	ratings := map[int64]float64{}
	for _, e := range es {
		ratings[e.ID] = e.Rating
	}
	xs, _ := s.store.StageEntries(ctx, t.ID)
	out := map[int64][]*tour.Row{}
	ids := map[int64][]int64{}
	for _, x := range xs {
		if x.StageID == st.ID {
			ids[x.GroupID] = append(ids[x.GroupID], x.EntryID)
		}
	}
	for gid, list := range ids {
		var res []tour.Result
		if st.Kind == "arena" {
			// 2 a win, 1 a draw, doubled on a streak of two wins
			byE := map[int64][]string{}
			for _, m := range all {
				if m.StageID != st.ID || m.Status != "done" {
					continue
				}
				switch m.Winner {
				case "a":
					byE[m.AEntry], byE[m.BEntry] = append(byE[m.AEntry], "W"), append(byE[m.BEntry], "L")
				case "b":
					byE[m.AEntry], byE[m.BEntry] = append(byE[m.AEntry], "L"), append(byE[m.BEntry], "W")
				case "draw":
					byE[m.AEntry], byE[m.BEntry] = append(byE[m.AEntry], "D"), append(byE[m.BEntry], "D")
				}
			}
			rows := tour.Standings(list, tResults(st, all, gid), rules, ratings)
			for _, r := range rows {
				r.Points, _ = tour.ArenaPoints(byE[r.ID])
			}
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Points != rows[j].Points {
					return rows[i].Points > rows[j].Points
				}
				return rows[i].Won > rows[j].Won
			})
			for i, r := range rows {
				r.Rank = i + 1
			}
			out[gid] = rows
			continue
		}
		res = tResults(st, all, gid)
		out[gid] = tour.Standings(list, res, rules, ratings)
	}
	return out
}

// leaderboard is a scored-round stage's ranking.
func (s *Server) leaderboard(ctx context.Context, t *store.Tournament, st *store.TStage) []*tour.Leader {
	set := settingsOf(t)
	cards, _ := s.store.Scorecards(ctx, st.ID)
	es, _ := s.store.Entries(ctx, t.ID)
	hc := map[int64]float64{}
	var ids []int64
	for _, e := range es {
		if e.Status != "in" {
			continue
		}
		ids = append(ids, e.ID)
		if e.Handicap != nil {
			hc[e.ID] = *e.Handicap
		}
	}
	tc := make([]tour.Card, 0, len(cards))
	for _, c := range cards {
		tc = append(tc, tour.Card{Entry: c.EntryID, Round: c.Round, Gross: c.Gross, Holes: c.Holes})
	}
	course := tour.Course{}
	if set.Course != nil {
		course = *set.Course
	}
	allow := set.Allowance
	if set.Mode == "pins" {
		// bowling: the handicap is given per game in full; it is worked out from the average when the entry has one
		allow = 1
	}
	return tour.Leaderboard(ids, tc, set.Mode, course, hc, allow, set.Low)
}

// ---- rated: the group's table ----

// rateMatch files a tournament match in the group's results (and so its
// ratings), when both sides are people of the group.
func (s *Server) rateMatch(ctx context.Context, t *store.Tournament, g *store.Group, m *store.TMatch, by int64) {
	if m.LeagueMatch != 0 || m.Outcome == "bye" || m.Outcome == "walkover" || m.Outcome == "no_show" || m.Outcome == "double_walkover" || m.Winner == "" {
		return
	}
	es, _ := s.store.Entries(ctx, t.ID)
	byID := map[int64]*store.TEntry{}
	for _, e := range es {
		byID[e.ID] = e
	}
	people := func(side string) []int64 {
		var ids []int64
		list := m.Sides[side]
		if len(list) == 0 {
			if side == "a" {
				list = []int64{m.AEntry}
			} else {
				list = []int64{m.BEntry}
			}
		}
		for _, eid := range list {
			e := byID[eid]
			if e == nil {
				return nil
			}
			for _, mem := range e.Members {
				if mem.Role != "sub" {
					ids = append(ids, mem.UserID)
				}
			}
		}
		return ids
	}
	a, b := people("a"), people("b")
	if len(a) == 0 || len(b) == 0 {
		return
	}
	members, _ := s.store.MemberIDs(ctx, g.ID)
	in := map[int64]bool{}
	for _, id := range members {
		in[id] = true
	}
	lm := &store.Match{GroupID: g.ID, PlayedAt: store.LocalNow(), Winner: m.Winner, Note: t.Name + (map[bool]string{true: " · " + m.Label, false: ""}[m.Label != ""])}
	for _, id := range a {
		if !in[id] {
			return
		}
		lm.Players = append(lm.Players, store.MatchPlayer{UserID: id, Side: "a"})
	}
	for _, id := range b {
		if !in[id] {
			return
		}
		lm.Players = append(lm.Players, store.MatchPlayer{UserID: id, Side: "b"})
	}
	lm.Score = m.Score
	if m.ATotal != nil && m.BTotal != nil && m.Score.A == nil && len(m.Score.Sets) == 0 {
		lm.Score = store.Score{A: m.ATotal, B: m.BTotal}
	}
	rules := league.RulesOf(g)
	// the group's own format may differ (a doubles match in a singles group): keep the score, let the table count it
	rules.Format = league.FormatTeams
	if rules.Scoring == league.ScoringRanking {
		return
	}
	if league.Check(rules, lm) != nil {
		// a score the group's sport does not take: count who won only
		lm.Score = store.Score{}
		w := m.Winner
		if err := league.Check(league.Rules{Scoring: league.ScoringResult, Format: league.FormatTeams}, lm); err != nil {
			return
		}
		lm.Winner = w
	}
	if err := s.store.SaveMatch(ctx, by, lm); err != nil {
		return
	}
	_ = s.store.ConfirmMatch(ctx, lm.ID, by)
	m.LeagueMatch = lm.ID
	_ = s.store.SaveTMatch(ctx, m)
}

// ---- the courts ----

// schedule calls the next matches to the free courts (queue), or gives
// every match a time (timetable).
func (s *Server) schedule(ctx context.Context, t *store.Tournament) {
	if t.Status != "live" {
		return
	}
	set := settingsOf(t)
	courts, _ := s.store.Courts(ctx, t.ID)
	var active []store.TCourt
	for _, c := range courts {
		if c.Active {
			active = append(active, c)
		}
	}
	if len(active) == 0 || set.Schedule == "none" {
		return
	}
	ms, _ := s.store.TMatches(ctx, t.ID)
	if set.Schedule == "timetable" {
		s.timetable(ctx, t, set, active, ms)
		return
	}
	busyCourt := map[int64]bool{}
	busyEntry := map[int64]bool{}
	for _, m := range ms {
		if m.Status == "called" {
			busyCourt[m.CourtID] = true
			for _, e := range s.sideEntries(m) {
				busyEntry[e] = true
			}
		}
	}
	g, err := s.store.GroupByID(ctx, t.GroupID)
	if err != nil {
		return
	}
	parents := map[int64]bool{}
	for _, m := range ms {
		if m.ParentID != 0 {
			parents[m.ParentID] = true
		}
	}
	for _, c := range active {
		if busyCourt[c.ID] {
			continue
		}
		for _, m := range ms {
			// a team tie is played as its matches, each on a court
			if m.Status != "ready" || parents[m.ID] {
				continue
			}
			es := s.sideEntries(m)
			clash := false
			for _, e := range es {
				if busyEntry[e] {
					clash = true
				}
			}
			if clash {
				continue
			}
			now := store.Now()
			m.Status, m.CourtID, m.CalledAt = "called", c.ID, &now
			if err := s.store.SaveTMatch(ctx, m); err != nil {
				continue
			}
			busyCourt[c.ID] = true
			for _, e := range es {
				busyEntry[e] = true
			}
			s.callPlayers(ctx, t, g, m, c.Name)
			break
		}
	}
}

// sideEntries are the entries playing a match.
func (s *Server) sideEntries(m *store.TMatch) []int64 {
	if len(m.Sides["a"]) > 0 {
		return append(append([]int64{}, m.Sides["a"]...), m.Sides["b"]...)
	}
	var out []int64
	for _, e := range []int64{m.AEntry, m.BEntry} {
		if e != 0 {
			out = append(out, e)
		}
	}
	return out
}

// callPlayers tells the people of a match where to go.
func (s *Server) callPlayers(ctx context.Context, t *store.Tournament, g *store.Group, m *store.TMatch, court string) {
	es, _ := s.store.Entries(ctx, t.ID)
	byID := map[int64]*store.TEntry{}
	for _, e := range es {
		byID[e.ID] = e
	}
	var uids []int64
	var names []string
	for _, eid := range s.sideEntries(m) {
		if e := byID[eid]; e != nil {
			names = append(names, e.Name)
			for _, mem := range e.Members {
				uids = append(uids, mem.UserID)
			}
		}
	}
	s.toGroup(ctx, g.ID, "tournament", map[string]any{"id": t.ID, "group_id": g.ID, "called": m.ID})
	s.tell(ctx, g, uids, 0, note{kind: "reminders", title: "You're on " + court + " now", body: t.Name + ": " + strings.Join(names, " · "), link: s.tLink(t.ID), button: "Open the tournament", tags: []string{"loudspeaker"}})
}

// timetable gives each round a start time over the courts.
func (s *Server) timetable(ctx context.Context, t *store.Tournament, set TSettings, courts []store.TCourt, ms []*store.TMatch) {
	start := parseTS(t.StartsAt)
	if t.StartsAt == "" {
		start = time.Now().Truncate(5 * time.Minute).Add(5 * time.Minute)
	}
	slot := time.Duration(max(5, set.Minutes)+max(0, set.Buffer)) * time.Minute
	i := 0
	for _, m := range ms {
		if m.ParentID != 0 || m.Status == "done" || m.Status == "void" || m.ScheduledAt != "" {
			if m.ScheduledAt != "" {
				i++
			}
			continue
		}
		c := courts[i%len(courts)]
		m.CourtID = c.ID
		m.ScheduledAt = start.Add(time.Duration(i/len(courts)) * slot).Format(tsLayout)
		_ = s.store.SaveTMatch(ctx, m)
		i++
	}
}

// ---- ladders ----

// ladderResult moves a winning challenger into the defender's place.
func (s *Server) ladderResult(ctx context.Context, t *store.Tournament, m *store.TMatch) {
	cs, _ := s.store.Challenges(ctx, t.ID)
	var ch *store.Challenge
	for i := range cs {
		if cs[i].MatchID == m.ID {
			ch = &cs[i]
		}
	}
	if ch == nil {
		return
	}
	ch.Status = "played"
	_ = s.store.SaveChallenge(ctx, ch)
	challengerWon := (m.AEntry == ch.Challenger && m.Winner == "a") || (m.BEntry == ch.Challenger && m.Winner == "b")
	es, _ := s.store.Entries(ctx, t.ID)
	var cha, def *store.TEntry
	for _, e := range es {
		if e.ID == ch.Challenger {
			cha = e
		}
		if e.ID == ch.Defender {
			def = e
		}
	}
	if cha == nil || def == nil {
		return
	}
	set := settingsOf(t)
	if challengerWon && cha.Position > def.Position {
		from, to := def.Position, cha.Position
		for _, e := range es {
			if e.Status == "in" && e.Position >= from && e.Position < to {
				e.Position++
				_ = s.store.SaveEntry(ctx, e)
			}
		}
		cha.Position = from
		_ = s.store.SaveEntry(ctx, cha)
		def, _ = s.findEntry(ctx, t.ID, def.ID)
	}
	if def != nil && set.ProtectDays > 0 {
		def.ProtectedUntil = time.Now().AddDate(0, 0, set.ProtectDays).Format(tsLayout)
		_ = s.store.SaveEntry(ctx, def)
	}
}

func (s *Server) findEntry(ctx context.Context, tid, id int64) (*store.TEntry, error) {
	es, err := s.store.Entries(ctx, tid)
	if err != nil {
		return nil, err
	}
	for _, e := range es {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, store.ErrNotFound
}

// ---- background: auto-confirm, ladder deadlines, arenas ending ----

// tournamentChores runs with the planner every minute.
func (s *Server) tournamentChores(ctx context.Context, now time.Time) {
	ts, err := s.store.LiveTournaments(ctx)
	if err != nil {
		return
	}
	for _, t := range ts {
		if t.Status != "live" {
			continue
		}
		g, err := s.store.GroupByID(ctx, t.GroupID)
		if err != nil {
			continue
		}
		set := settingsOf(t)
		ms, _ := s.store.TMatches(ctx, t.ID)
		for _, m := range ms {
			if m.Status == "reported" && set.AutoConfirm > 0 && m.ReportedAt != nil && m.Dispute == "" {
				if at, err := time.Parse(time.RFC3339Nano, *m.ReportedAt); err == nil && now.Sub(at) >= time.Duration(set.AutoConfirm)*time.Minute {
					nowS := store.Now()
					m.Status, m.ConfirmedAt = "done", &nowS
					if err := s.store.SaveTMatch(ctx, m); err == nil {
						s.confirmed(ctx, t, g, m, m.ReportedBy)
					}
				}
			}
		}
		if t.Format == "ladder" {
			s.ladderDeadlines(ctx, t, g, now)
		}
		if t.Format == "arena" && t.EndsAt != "" && store.LocalNow() >= t.EndsAt {
			open := false
			for _, m := range ms {
				if m.Status != "done" && m.Status != "void" {
					open = true
				}
			}
			if !open {
				stages, _ := s.store.Stages(ctx, t.ID)
				for _, st := range stages {
					if st.Status != "done" {
						_ = s.finishStage(ctx, t, st)
					}
				}
			}
		}
	}
}

// ladderDeadlines: a challenge not answered in time is a walkover for the
// challenger; one accepted but not played by its day is cancelled.
func (s *Server) ladderDeadlines(ctx context.Context, t *store.Tournament, g *store.Group, now time.Time) {
	cs, _ := s.store.Challenges(ctx, t.ID)
	ts := now.Format(tsLayout)
	for i := range cs {
		c := &cs[i]
		switch {
		case c.Status == "open" && c.RespondBy != "" && ts > c.RespondBy:
			m, err := s.store.GetTMatch(ctx, c.MatchID)
			if err != nil {
				continue
			}
			nowS := store.Now()
			m.Status, m.Outcome, m.ConfirmedAt = "done", "walkover", &nowS
			m.Winner = "a"
			if m.BEntry == c.Challenger {
				m.Winner = "b"
			}
			if err := s.store.SaveTMatch(ctx, m); err == nil {
				s.store.AddTLog(ctx, t.ID, 0, "walkover", "a challenge was not answered in time")
				s.confirmed(ctx, t, g, m, 0)
			}
		case c.Status == "accepted" && c.PlayBy != "" && ts > c.PlayBy:
			c.Status = "expired"
			_ = s.store.SaveChallenge(ctx, c)
			if m, err := s.store.GetTMatch(ctx, c.MatchID); err == nil && m.Status != "done" {
				m.Status = "void"
				_ = s.store.SaveTMatch(ctx, m)
			}
		}
	}
}
