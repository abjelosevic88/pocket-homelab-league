package tour

import (
	"math"
	"sort"
)

// Result is a finished match as the table sees it. A and B are the
// entries (or, in rotating formats, the players) on each side.
type Result struct {
	A, B         []int64
	Winner       string  // a | b | draw | "" (not played: double walkover)
	ForA, ForB   float64 // goals, points, legs, games (sets: games), Americano points
	SetsA, SetsB int
	PointsA      float64 // points ratio for volleyball (rally points), else = For
	PointsB      float64
	Walkover     bool // a walkover: counts as a win, not for the ratios
	Bye          bool
}

// Rules are how a stage counts.
type Rules struct {
	Win, Draw, Loss float64
	Bye             float64 // points for a bye (Swiss)
	Volley          bool    // FIVB: 3–0 / 3–1 = 3:0, 3–2 = 2:1
	PlayedLoss      bool    // table tennis: 1 for a loss played, 0 for one not played
	Individual      bool    // credit every player on a side (Americano)
	ByFor           bool    // the points are what was scored (Americano)
	Tiebreaks       []string
}

// Row is one entry's line.
type Row struct {
	ID        int64   `json:"id"`
	Played    int     `json:"played"`
	Won       int     `json:"won"`
	Drawn     int     `json:"drawn"`
	Lost      int     `json:"lost"`
	Points    float64 `json:"points"`
	For       float64 `json:"for"`
	Against   float64 `json:"against"`
	SetsWon   int     `json:"sets_won"`
	SetsLost  int     `json:"sets_lost"`
	PtsWon    float64 `json:"points_won"`
	PtsLost   float64 `json:"points_lost"`
	Buchholz  float64 `json:"buchholz"`
	SB        float64 `json:"sonneborn_berger"`
	Byes      int     `json:"byes"`
	Rank      int     `json:"rank"`
	DecidedBy string  `json:"decided_by"`

	opp    []int64
	scored map[int64]float64 // score against each opponent (for Sonneborn-Berger)
}

// Tie-break keys, in plain words for the table.
var TiebreakWords = map[string]string{
	"points": "points", "wins": "wins", "h2h": "head-to-head", "diff": "difference", "for": "scored",
	"set_ratio": "set ratio", "game_ratio": "game ratio", "point_ratio": "point ratio",
	"buchholz": "Buchholz", "sb": "Sonneborn-Berger", "rating": "rating", "lots": "lots",
}

// Standings works out the table of a stage. ids are everyone in it (also
// those who did not play yet); ratings break the last ties.
func Standings(ids []int64, res []Result, r Rules, ratings map[int64]float64) []*Row {
	rows := map[int64]*Row{}
	get := func(id int64) *Row {
		if x := rows[id]; x != nil {
			return x
		}
		x := &Row{ID: id, scored: map[int64]float64{}}
		rows[id] = x
		return x
	}
	for _, id := range ids {
		get(id)
	}
	for _, m := range res {
		if m.Bye {
			for _, id := range append(m.A, m.B...) {
				x := get(id)
				x.Byes++
				x.Points += r.Bye
			}
			continue
		}
		if m.Winner == "" {
			continue
		}
		pa, pb := r.Loss, r.Loss
		switch m.Winner {
		case "a":
			pa = r.Win
			if r.Volley && m.SetsB > 0 && m.SetsA-m.SetsB == 1 {
				pa, pb = 2, 1
			}
		case "b":
			pb = r.Win
			if r.Volley && m.SetsA > 0 && m.SetsB-m.SetsA == 1 {
				pa, pb = 1, 2
			}
		case "draw":
			pa, pb = r.Draw, r.Draw
		}
		if r.PlayedLoss && m.Walkover {
			if m.Winner == "a" {
				pb = 0
			} else if m.Winner == "b" {
				pa = 0
			}
		}
		if r.ByFor {
			pa, pb = m.ForA, m.ForB
		}
		side := func(ids, opp []int64, won bool, draw bool, pts, f, ag float64, sw, sl int, pw, pl float64, oppScore float64) {
			for _, id := range ids {
				x := get(id)
				x.Played++
				switch {
				case draw:
					x.Drawn++
				case won:
					x.Won++
				default:
					x.Lost++
				}
				x.Points += pts
				if !m.Walkover {
					x.For += f
					x.Against += ag
					x.SetsWon += sw
					x.SetsLost += sl
					x.PtsWon += pw
					x.PtsLost += pl
				}
				x.opp = append(x.opp, opp...)
				for _, o := range opp {
					x.scored[o] += oppScore
				}
			}
		}
		sa, sb := 0.0, 0.0
		switch m.Winner {
		case "a":
			sa = 1
		case "b":
			sb = 1
		case "draw":
			sa, sb = 0.5, 0.5
		}
		pwa, pla := m.PointsA, m.PointsB
		if pwa == 0 && pla == 0 {
			pwa, pla = m.ForA, m.ForB
		}
		side(m.A, m.B, m.Winner == "a", m.Winner == "draw", pa, m.ForA, m.ForB, m.SetsA, m.SetsB, pwa, pla, sa)
		side(m.B, m.A, m.Winner == "b", m.Winner == "draw", pb, m.ForB, m.ForA, m.SetsB, m.SetsA, pla, pwa, sb)
	}
	// Buchholz (sum of opponents' points) and Sonneborn-Berger (opponents' points weighted by your result)
	for _, x := range rows {
		for _, o := range x.opp {
			x.Buchholz += rows[o].Points
		}
		for o, sc := range x.scored {
			x.SB += sc * rows[o].Points
		}
	}
	list := make([]*Row, 0, len(rows))
	for _, x := range rows {
		list = append(list, x)
	}
	chain := r.Tiebreaks
	if len(chain) == 0 {
		chain = []string{"points", "diff", "for"}
	}
	if chain[0] != "points" {
		chain = append([]string{"points"}, chain...)
	}
	sortRows(list, chain, res, r, ratings)
	return list
}

func ratio(a, b float64) float64 {
	if b == 0 {
		if a == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return a / b
}

func value(x *Row, k string, ratings map[int64]float64) float64 {
	switch k {
	case "points":
		return x.Points
	case "wins":
		return float64(x.Won)
	case "diff":
		return x.For - x.Against
	case "for":
		return x.For
	case "set_ratio":
		return ratio(float64(x.SetsWon), float64(x.SetsLost))
	case "game_ratio":
		return ratio(x.For, x.Against)
	case "point_ratio":
		return ratio(x.PtsWon, x.PtsLost)
	case "buchholz":
		return x.Buchholz
	case "sb":
		return x.SB
	case "rating":
		return ratings[x.ID]
	}
	return 0
}

// sortRows orders by the chain; head-to-head is a small table of the tied
// entries only (as UEFA, ITTF and BWF do). It notes what decided each tie.
func sortRows(list []*Row, chain []string, res []Result, r Rules, ratings map[int64]float64) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	var order func(rows []*Row, from int) []*Row
	order = func(rows []*Row, from int) []*Row {
		if len(rows) <= 1 || from >= len(chain) {
			if len(rows) > 1 {
				for _, x := range rows {
					if x.DecidedBy == "" && x.Played > 0 {
						x.DecidedBy = "lots"
					}
				}
			}
			return rows
		}
		k := chain[from]
		vals := map[int64]float64{}
		if k == "h2h" {
			in := map[int64]bool{}
			for _, x := range rows {
				in[x.ID] = true
			}
			var mini []Result
			for _, m := range res {
				if len(m.A) > 0 && len(m.B) > 0 && in[m.A[0]] && in[m.B[0]] {
					mini = append(mini, m)
				}
			}
			ids := make([]int64, 0, len(rows))
			for _, x := range rows {
				ids = append(ids, x.ID)
			}
			rr := r
			rr.Tiebreaks = []string{"points"}
			for _, x := range Standings(ids, mini, rr, nil) {
				vals[x.ID] = x.Points*1e6 + (x.For - x.Against)
			}
		} else {
			for _, x := range rows {
				vals[x.ID] = value(x, k, ratings)
			}
		}
		sort.SliceStable(rows, func(i, j int) bool { return vals[rows[i].ID] > vals[rows[j].ID] })
		var out []*Row
		for i := 0; i < len(rows); {
			j := i
			for j < len(rows) && vals[rows[j].ID] == vals[rows[i].ID] {
				j++
			}
			tied := rows[i:j]
			if j-i == 1 && len(rows) > 1 && from > 0 && rows[i].DecidedBy == "" && rows[i].Played > 0 {
				rows[i].DecidedBy = TiebreakWords[k]
			}
			if j-i > 1 {
				tied = order(append([]*Row{}, tied...), from+1)
				if from > 0 {
					for _, x := range tied {
						if x.DecidedBy == "" && x.Played > 0 {
							x.DecidedBy = TiebreakWords[k]
						}
					}
				}
			}
			out = append(out, tied...)
			i = j
		}
		return out
	}
	// the first key (points) splits the whole table; ties go down the chain
	sorted := order(append([]*Row{}, list...), 0)
	copy(list, sorted)
	for i, x := range list {
		x.Rank = i + 1
	}
}

// ---- scored rounds: golf, bowling, darts nights ----

// Card is one entry's round: the gross total, or the strokes per hole.
type Card struct {
	Entry int64
	Round int
	Gross *float64
	Holes []float64
}

// Course is a golf course: par and stroke index per hole.
type Course struct {
	Par []int `json:"par"`
	SI  []int `json:"si"`
}

// Leader is one line of a scored-round leaderboard.
type Leader struct {
	ID        int64     `json:"id"`
	Rounds    []float64 `json:"rounds"` // per round: net (stroke), points (Stableford), pins (bowling)
	Total     float64   `json:"total"`
	Gross     float64   `json:"gross"`
	Rank      int       `json:"rank"`
	DecidedBy string    `json:"decided_by"`
	Holes     int       `json:"holes"` // holes played (golf, live)
}

// StrokesReceived is how many strokes a player gets on a hole: the
// playing handicap spread over the holes by stroke index.
func StrokesReceived(handicap float64, si int, holes int) int {
	h := int(math.Round(handicap))
	if holes == 0 || si <= 0 {
		return 0
	}
	n := h / holes
	if si <= h%holes {
		n++
	}
	return n
}

// Stableford points on one hole: 2 for a net par, one more for each stroke under.
func Stableford(strokes float64, par, received int) float64 {
	if strokes <= 0 {
		return 0
	}
	p := float64(par+received) - strokes + 2
	if p < 0 {
		return 0
	}
	return p
}

// Leaderboard ranks scored rounds. mode: stroke (net, lowest wins),
// stableford (points, highest wins), pins (bowling, highest wins, with a
// handicap per game), plain (a score per round; lower is better when low).
func Leaderboard(ids []int64, cards []Card, mode string, course Course, handicaps map[int64]float64, allowance float64, low bool) []*Leader {
	if allowance <= 0 {
		allowance = 1
	}
	rows := map[int64]*Leader{}
	maxRound := 0
	for _, id := range ids {
		rows[id] = &Leader{ID: id}
	}
	for _, c := range cards {
		if c.Round > maxRound {
			maxRound = c.Round
		}
	}
	for _, x := range rows {
		x.Rounds = make([]float64, maxRound)
	}
	for _, c := range cards {
		x := rows[c.Entry]
		if x == nil || c.Round < 1 {
			continue
		}
		hc := handicaps[c.Entry] * allowance
		var v, gross float64
		switch mode {
		case "stableford":
			for i, s := range c.Holes {
				if i < len(course.Par) {
					si := 0
					if i < len(course.SI) {
						si = course.SI[i]
					}
					v += Stableford(s, course.Par[i], StrokesReceived(hc, si, len(course.Par)))
					gross += s
					if s > 0 {
						x.Holes++
					}
				}
			}
		case "pins":
			for _, g := range c.Holes {
				gross += g
				if g > 0 {
					v += g + math.Floor(hc)
				}
			}
			if len(c.Holes) == 0 && c.Gross != nil {
				gross, v = *c.Gross, *c.Gross+math.Floor(hc)
			}
		default: // stroke or plain
			if len(c.Holes) > 0 {
				for _, s := range c.Holes {
					gross += s
					if s > 0 {
						x.Holes++
					}
				}
			} else if c.Gross != nil {
				gross = *c.Gross
			}
			v = gross
			if mode == "stroke" {
				v = gross - math.Round(hc)
			}
		}
		x.Rounds[c.Round-1] = v
		x.Total += v
		x.Gross += gross
	}
	list := make([]*Leader, 0, len(rows))
	for _, x := range rows {
		list = append(list, x)
	}
	lower := mode == "stroke" || (mode == "plain" && low)
	better := func(a, b float64) bool {
		if lower {
			return a < b
		}
		return a > b
	}
	played := func(x *Leader) bool { return x.Gross > 0 }
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if played(a) != played(b) {
			return played(a)
		}
		if a.Total != b.Total {
			return better(a.Total, b.Total)
		}
		// countback: the last round, then the one before …
		for r := maxRound - 1; r >= 0; r-- {
			if a.Rounds[r] != b.Rounds[r] {
				return better(a.Rounds[r], b.Rounds[r])
			}
		}
		return a.ID < b.ID
	})
	for i, x := range list {
		x.Rank = i + 1
		if i > 0 && x.Total == list[i-1].Total && played(x) {
			x.DecidedBy, list[i-1].DecidedBy = "countback", "countback"
		}
	}
	return list
}

// BowlingHandicap is (basis − average) × percent, never below 0.
func BowlingHandicap(average, basis, pct float64) float64 {
	h := math.Floor((basis - average) * pct)
	if h < 0 {
		return 0
	}
	return h
}

// ArenaPoints scores an arena: 2 a win, 1 a draw, and a win counts
// double once you have won twice in a row (a "streak").
func ArenaPoints(results []string) (points float64, streak bool) {
	wins := 0
	for _, r := range results {
		switch r {
		case "W":
			p := 2.0
			if wins >= 2 {
				p = 4
			}
			points += p
			wins++
		case "D":
			p := 1.0
			if wins >= 2 {
				p = 2
			}
			points += p
			wins = 0
		default:
			wins = 0
		}
	}
	return points, wins >= 2
}
