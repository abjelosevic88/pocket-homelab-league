package league

import (
	"strconv"
	"strings"
)

// ---- live scoring, point by point ----
//
// A live score is a list of points ("a", "b"; for counting sports "a3" is
// three points for A). Replaying the list gives the score, so undo is
// "drop the last one" and nothing can drift.

// Kinds of live scoring.
const (
	LiveTennis = "tennis" // 15-30-40, games, sets, tie-breaks (tennis, padel)
	LiveRally  = "rally"  // sets to a number of points, win by two (volleyball, badminton, table tennis …)
	LiveCount  = "count"  // a running score (football, basketball, darts legs …)
)

// LiveRules say how a live score counts.
type LiveRules struct {
	Kind       string `json:"kind"`
	Sets       int    `json:"sets"`        // best of (1, 3, 5)
	Games      int    `json:"games"`       // tennis: games to win a set (6)
	TieBreakAt int    `json:"tiebreak_at"` // tennis: a tie-break at games-all (6; 0 = none, play on)
	TieBreakTo int    `json:"tiebreak_to"` // points in a tie-break (7)
	SuperFinal bool   `json:"super_final"` // tennis: the deciding set is a tie-break to 10
	Golden     bool   `json:"golden"`      // tennis: at deuce the next point wins
	Points     int    `json:"points"`      // rally: points to win a set (25, 21, 11)
	LastPoints int    `json:"last_points"` // rally: in the deciding set (15)
	WinBy      int    `json:"win_by"`      // rally: by two
	Cap        int    `json:"cap"`         // rally: at most (30 in badminton; 0 = none)
}

// LiveRulesFor are the usual rules of a sport.
func LiveRulesFor(sport, scoring string) LiveRules {
	switch sport {
	case "tennis":
		return LiveRules{Kind: LiveTennis, Sets: 3, Games: 6, TieBreakAt: 6, TieBreakTo: 7}
	case "padel":
		return LiveRules{Kind: LiveTennis, Sets: 3, Games: 6, TieBreakAt: 6, TieBreakTo: 7, Golden: true}
	case "volleyball":
		return LiveRules{Kind: LiveRally, Sets: 5, Points: 25, LastPoints: 15, WinBy: 2}
	case "beach_volleyball":
		return LiveRules{Kind: LiveRally, Sets: 3, Points: 21, LastPoints: 15, WinBy: 2}
	case "badminton":
		return LiveRules{Kind: LiveRally, Sets: 3, Points: 21, WinBy: 2, Cap: 30}
	case "table_tennis":
		return LiveRules{Kind: LiveRally, Sets: 5, Points: 11, WinBy: 2}
	case "squash":
		return LiveRules{Kind: LiveRally, Sets: 5, Points: 11, WinBy: 2}
	case "pickleball":
		return LiveRules{Kind: LiveRally, Sets: 3, Points: 11, WinBy: 2}
	}
	if scoring == ScoringSets {
		return LiveRules{Kind: LiveRally, Sets: 3, Points: 21, WinBy: 2}
	}
	return LiveRules{Kind: LiveCount}
}

// Clean puts rules in range.
func (r LiveRules) Clean() LiveRules {
	if r.Kind != LiveTennis && r.Kind != LiveRally {
		return LiveRules{Kind: LiveCount}
	}
	if r.Sets != 1 && r.Sets != 3 && r.Sets != 5 {
		r.Sets = 3
	}
	if r.Kind == LiveTennis {
		if r.Games < 1 || r.Games > 9 {
			r.Games = 6
		}
		if r.TieBreakAt < 0 || r.TieBreakAt > r.Games {
			r.TieBreakAt = r.Games
		}
		if r.TieBreakTo < 5 || r.TieBreakTo > 15 {
			r.TieBreakTo = 7
		}
		r.Points, r.LastPoints, r.WinBy, r.Cap = 0, 0, 0, 0
		return r
	}
	if r.Points < 5 || r.Points > 50 {
		r.Points = 21
	}
	if r.LastPoints < 0 || r.LastPoints > 50 {
		r.LastPoints = 0
	}
	if r.WinBy < 1 || r.WinBy > 2 {
		r.WinBy = 2
	}
	if r.Cap != 0 && r.Cap <= r.Points {
		r.Cap = 0
	}
	r.Games, r.TieBreakAt, r.TieBreakTo, r.SuperFinal, r.Golden = 0, 0, 0, false, false
	return r
}

// LiveView is a live score worked out.
type LiveView struct {
	Sets     [][2]int  `json:"sets"`  // finished sets (games, or points)
	Cur      [2]int    `json:"cur"`   // the set being played: games (tennis), points (rally), the score (count)
	Point    [2]string `json:"point"` // tennis: "15" "40" "AD"; a tie-break's points
	SetsWon  [2]int    `json:"sets_won"`
	TieBreak bool      `json:"tiebreak"`
	Deuce    bool      `json:"deuce"`
	Golden   bool      `json:"golden"`    // the next point decides the game
	SetPoint string    `json:"set_point"` // a | b: one point from the set …
	Match    string    `json:"match_point"`
	Done     bool      `json:"done"`
	Winner   string    `json:"winner"`
	Total    int       `json:"total"` // points played
	Score    struct {
		Sets [][2]int `json:"sets,omitempty"`
		A    *float64 `json:"a,omitempty"`
		B    *float64 `json:"b,omitempty"`
	} `json:"score"` // the result so far, the way the group enters it
}

// ParsePoint reads "a", "b", "a3" into a side and an amount.
func ParsePoint(p string) (string, int, bool) {
	if p == "" || (p[0] != 'a' && p[0] != 'b') {
		return "", 0, false
	}
	n := 1
	if len(p) > 1 {
		v, err := strconv.Atoi(p[1:])
		if err != nil || v < 1 || v > 9 {
			return "", 0, false
		}
		n = v
	}
	return p[:1], n, true
}

// Live replays the points.
func Live(r LiveRules, points []string) LiveView {
	v := LiveView{Sets: [][2]int{}}
	r = r.Clean()
	v.Total = len(points)
	switch r.Kind {
	case LiveCount:
		for _, p := range points {
			side, n, ok := ParsePoint(p)
			if !ok {
				continue
			}
			v.Cur[idx(side)] += n
		}
		a, b := float64(v.Cur[0]), float64(v.Cur[1])
		v.Score.A, v.Score.B = &a, &b
		v.Winner = winnerOf(a, b)
		return v
	case LiveTennis:
		tennis(&v, r, points)
	default:
		rally(&v, r, points)
	}
	v.Score.Sets = append([][2]int{}, v.Sets...)
	if !v.Done && (v.Cur[0] > 0 || v.Cur[1] > 0) {
		v.Score.Sets = append(v.Score.Sets, v.Cur)
	}
	return v
}

func idx(side string) int {
	if side == "b" {
		return 1
	}
	return 0
}

func other(i int) int { return 1 - i }

func need(sets int) int { return sets/2 + 1 }

func tennis(v *LiveView, r LiveRules, points []string) {
	var games, pts [2]int
	tb := false
	tbTo := r.TieBreakTo
	superSet := func() bool {
		return r.SuperFinal && r.Sets > 1 && v.SetsWon[0] == need(r.Sets)-1 && v.SetsWon[1] == need(r.Sets)-1
	}
	if superSet() {
		tb, tbTo = true, 10
	}
	winSet := func(w int) {
		v.Sets = append(v.Sets, games)
		v.SetsWon[w]++
		games, pts = [2]int{}, [2]int{}
		tb, tbTo = false, r.TieBreakTo
		if v.SetsWon[w] >= need(r.Sets) {
			v.Done, v.Winner = true, string("ab"[w])
			return
		}
		if superSet() {
			tb, tbTo = true, 10
		}
	}
	for _, p := range points {
		if v.Done {
			break
		}
		side, _, ok := ParsePoint(p)
		if !ok {
			continue
		}
		w := idx(side)
		pts[w]++
		if tb {
			if pts[w] >= tbTo && pts[w]-pts[other(w)] >= 2 {
				if tbTo == 10 && games == [2]int{} {
					games = pts // a deciding super tie-break is written as its points: 10–8
				} else {
					games[w]++
				}
				winSet(w)
			}
			continue
		}
		won := false
		if r.Golden && pts[0] >= 3 && pts[1] >= 3 && pts[w] == pts[other(w)]+1 && pts[w]+pts[other(w)] == 7 {
			won = true // the golden point at 40–40
		} else if pts[w] >= 4 && pts[w]-pts[other(w)] >= 2 {
			won = true
		}
		if !won {
			continue
		}
		pts = [2]int{}
		games[w]++
		g, o := games[w], games[other(w)]
		switch {
		case g >= r.Games && g-o >= 2:
			winSet(w)
		case r.TieBreakAt > 0 && games[0] == r.TieBreakAt && games[1] == r.TieBreakAt:
			tb = true
		}
	}
	v.Cur, v.TieBreak = games, tb
	if v.Done {
		return
	}
	if tb {
		v.Point = [2]string{strconv.Itoa(pts[0]), strconv.Itoa(pts[1])}
		for i := 0; i < 2; i++ {
			if pts[i]+1 >= tbTo && pts[i]+1-pts[other(i)] >= 2 {
				v.SetPoint = string("ab"[i])
			}
		}
	} else {
		names := []string{"0", "15", "30", "40"}
		switch {
		case pts[0] >= 3 && pts[1] >= 3:
			if r.Golden {
				v.Point, v.Golden = [2]string{"40", "40"}, true
			} else if pts[0] == pts[1] {
				v.Point, v.Deuce = [2]string{"40", "40"}, true
			} else if pts[0] > pts[1] {
				v.Point = [2]string{"AD", ""}
			} else {
				v.Point = [2]string{"", "AD"}
			}
		default:
			v.Point = [2]string{names[min(pts[0], 3)], names[min(pts[1], 3)]}
		}
		for i := 0; i < 2; i++ {
			gamePoint := (pts[i] >= 3 && pts[i] > pts[other(i)]) || (pts[i] == 3 && pts[other(i)] < 3) || (r.Golden && pts[0] == 3 && pts[1] == 3)
			if gamePoint && games[i]+1 >= r.Games && games[i]+1-games[other(i)] >= 2 {
				v.SetPoint = string("ab"[i])
			}
		}
	}
	if v.SetPoint != "" && v.SetsWon[idx(v.SetPoint)]+1 >= need(r.Sets) {
		v.Match = v.SetPoint
	}
}

func rally(v *LiveView, r LiveRules, points []string) {
	var pts [2]int
	target := func() int {
		if r.LastPoints > 0 && r.Sets > 1 && v.SetsWon[0] == need(r.Sets)-1 && v.SetsWon[1] == need(r.Sets)-1 {
			return r.LastPoints
		}
		return r.Points
	}
	for _, p := range points {
		if v.Done {
			break
		}
		side, _, ok := ParsePoint(p)
		if !ok {
			continue
		}
		w := idx(side)
		pts[w]++
		t := target()
		if (pts[w] >= t && pts[w]-pts[other(w)] >= r.WinBy) || (r.Cap > 0 && pts[w] >= r.Cap) {
			v.Sets = append(v.Sets, pts)
			v.SetsWon[w]++
			pts = [2]int{}
			if v.SetsWon[w] >= need(r.Sets) {
				v.Done, v.Winner = true, string("ab"[w])
			}
		}
	}
	v.Cur = pts
	if v.Done {
		return
	}
	t := target()
	for i := 0; i < 2; i++ {
		n := pts[i] + 1
		if (n >= t && n-pts[other(i)] >= r.WinBy) || (r.Cap > 0 && n >= r.Cap) {
			v.SetPoint = string("ab"[i])
		}
	}
	if v.SetPoint != "" && v.SetsWon[idx(v.SetPoint)]+1 >= need(r.Sets) {
		v.Match = v.SetPoint
	}
}

// LiveLine is a live score in a few words: "6–4 3–2 (30–15)".
func LiveLine(r LiveRules, v LiveView) string {
	if r.Kind == LiveCount {
		return strconv.Itoa(v.Cur[0]) + "–" + strconv.Itoa(v.Cur[1])
	}
	var parts []string
	for _, s := range v.Sets {
		parts = append(parts, strconv.Itoa(s[0])+"–"+strconv.Itoa(s[1]))
	}
	if !v.Done {
		cur := strconv.Itoa(v.Cur[0]) + "–" + strconv.Itoa(v.Cur[1])
		if r.Kind == LiveTennis && (v.Point[0] != "" || v.Point[1] != "") {
			cur += " (" + firstOr(v.Point[0], "40") + "–" + firstOr(v.Point[1], "40") + ")"
		}
		parts = append(parts, cur)
	}
	return strings.Join(parts, " ")
}

func firstOr(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
