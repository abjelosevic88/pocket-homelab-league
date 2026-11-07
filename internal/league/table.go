package league

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// Rules are how a group counts.
type Rules struct {
	Scoring     string
	Format      string
	LowerBetter bool
	Win         int
	Draw        int
	Loss        int
}

// RulesOf is a group's rules.
func RulesOf(g *store.Group) Rules {
	return Rules{Scoring: g.Scoring, Format: g.Format, LowerBetter: g.LowerBetter, Win: g.WinPoints, Draw: g.DrawPoints, Loss: g.LossPoints}
}

// Check validates a result and works out its winner (and, in a ranking,
// everyone's place). It changes m in place.
func Check(r Rules, m *store.Match) error {
	a, b := 0, 0
	seen := map[int64]bool{}
	for _, p := range m.Players {
		if seen[p.UserID] {
			return errors.New("a player can only be in a match once")
		}
		seen[p.UserID] = true
		switch p.Side {
		case "a":
			a++
		case "b":
			b++
		}
	}
	if r.Scoring == ScoringRanking {
		if len(m.Players) < 2 {
			return errors.New("a ranking needs at least two players")
		}
		for i := range m.Players {
			m.Players[i].Side = ""
			if m.Players[i].Score == nil {
				return errors.New("give every player a score")
			}
		}
		rank(m.Players, r.LowerBetter)
		m.Score, m.Winner = store.Score{}, ""
		return nil
	}
	if a == 0 || b == 0 || a+b != len(m.Players) {
		return errors.New("put the players on two sides")
	}
	if n := SideSize(r.Format); n > 0 && (a != n || b != n) {
		return fmt.Errorf("each side has %d %s", n, map[bool]string{true: "player", false: "players"}[n == 1])
	}
	switch r.Scoring {
	case ScoringSets:
		if len(m.Score.Sets) == 0 || len(m.Score.Sets) > 7 {
			return errors.New("give the score of each set")
		}
		sa, sb := 0, 0
		for _, s := range m.Score.Sets {
			if s[0] < 0 || s[1] < 0 || s[0] > 99 || s[1] > 99 {
				return errors.New("a set score is 0 to 99")
			}
			switch {
			case s[0] > s[1]:
				sa++
			case s[1] > s[0]:
				sb++
			}
		}
		m.Score.A, m.Score.B = nil, nil
		m.Winner = winnerOf(float64(sa), float64(sb))
	case ScoringGoals, ScoringPoints:
		if m.Score.A == nil || m.Score.B == nil || *m.Score.A < 0 || *m.Score.B < 0 || *m.Score.A > 9999 || *m.Score.B > 9999 {
			return errors.New("give both scores")
		}
		m.Score.Sets = nil
		m.Winner = winnerOf(*m.Score.A, *m.Score.B)
	case ScoringResult:
		if m.Winner != "a" && m.Winner != "b" && m.Winner != "draw" {
			return errors.New("say who won, or that it was a draw")
		}
		m.Score = store.Score{}
	default:
		return errors.New("unknown way of scoring")
	}
	return nil
}

func winnerOf(a, b float64) string {
	switch {
	case a > b:
		return "a"
	case b > a:
		return "b"
	}
	return "draw"
}

// rank gives each player a place by score (ties share a place).
func rank(ps []store.MatchPlayer, lowerBetter bool) {
	idx := make([]int, len(ps))
	for i := range idx {
		idx[i] = i
	}
	better := func(x, y float64) bool {
		if lowerBetter {
			return x < y
		}
		return x > y
	}
	sort.SliceStable(idx, func(i, j int) bool { return better(*ps[idx[i]].Score, *ps[idx[j]].Score) })
	for pos, i := range idx {
		if pos > 0 && *ps[i].Score == *ps[idx[pos-1]].Score {
			ps[i].Place = ps[idx[pos-1]].Place
		} else {
			ps[i].Place = pos + 1
		}
	}
}

// ScoreText is a result in a few characters: "6–4 3–6 7–5", "3–2", "draw".
func ScoreText(r Rules, m *store.Match) string {
	switch r.Scoring {
	case ScoringSets:
		var parts []string
		for _, s := range m.Score.Sets {
			parts = append(parts, fmt.Sprintf("%d–%d", s[0], s[1]))
		}
		return strings.Join(parts, " ")
	case ScoringGoals, ScoringPoints:
		if m.Score.A != nil && m.Score.B != nil {
			return num(*m.Score.A) + "–" + num(*m.Score.B)
		}
	case ScoringResult:
		if m.Winner == "draw" {
			return "draw"
		}
		return "won"
	}
	return ""
}

func num(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.1f", f)
}

// Side lists the players of one side.
func Side(m *store.Match, side string) []int64 {
	out := []int64{}
	for _, p := range m.Players {
		if p.Side == side {
			out = append(out, p.UserID)
		}
	}
	return out
}

// ---- the table ----

// StartRating is everyone's rating before their first match.
const StartRating = 1000.0

// Row is one person's line in the table.
type Row struct {
	UserID   int64          `json:"user_id"`
	Played   int            `json:"played"`
	Won      int            `json:"won"`
	Drawn    int            `json:"drawn"`
	Lost     int            `json:"lost"`
	Points   int            `json:"points"`
	For      float64        `json:"for"`     // games / goals / points won (sets: games)
	Against  float64        `json:"against"` // and lost
	SetsWon  int            `json:"sets_won"`
	SetsLost int            `json:"sets_lost"`
	Scored   float64        `json:"scored"` // their own goals or points
	Wins     int            `json:"wins"`   // ranking: first places
	Best     *float64       `json:"best"`   // ranking: their best score
	Average  *float64       `json:"average"`
	Rating   float64        `json:"rating"`
	Change   float64        `json:"change"` // the last match's change in rating
	Form     []string       `json:"form"`   // the last five: W | D | L (oldest first)
	Streak   string         `json:"streak"` // "W3", "L1"
	MVP      int            `json:"mvp"`
	Stats    map[string]int `json:"stats,omitempty"` // totals of what the group counts
	Position int            `json:"position"`

	total float64
}

// Table is the worked-out table plus what each match did to the ratings.
type Table struct {
	Rows    []*Row                      `json:"rows"`
	Changes map[int64]map[int64]float64 `json:"changes"` // match → player → rating change
}

// Compute works out the table from results, oldest first. Disputed results
// are left out until someone fixes them.
func Compute(r Rules, matches []*store.Match, people []int64) *Table {
	rows := map[int64]*Row{}
	get := func(uid int64) *Row {
		if x := rows[uid]; x != nil {
			return x
		}
		x := &Row{UserID: uid, Rating: StartRating, Form: []string{}}
		rows[uid] = x
		return x
	}
	for _, uid := range people {
		get(uid)
	}
	t := &Table{Changes: map[int64]map[int64]float64{}}
	for _, m := range matches {
		if m.DisputedBy != 0 {
			continue
		}
		t.Changes[m.ID] = map[int64]float64{}
		if r.Scoring == ScoringRanking {
			computeRanking(r, m, get, t.Changes[m.ID])
			continue
		}
		a, b := Side(m, "a"), Side(m, "b")
		if len(a) == 0 || len(b) == 0 {
			continue
		}
		ra, rb := avgRating(a, get), avgRating(b, get)
		ea := 1 / (1 + math.Pow(10, (rb-ra)/400))
		sa := map[string]float64{"a": 1, "b": 0, "draw": 0.5}[m.Winner]
		k := 32.0
		da := k * (sa - ea)
		var forA, forB float64
		var setsA, setsB int
		switch r.Scoring {
		case ScoringSets:
			for _, s := range m.Score.Sets {
				forA += float64(s[0])
				forB += float64(s[1])
				if s[0] > s[1] {
					setsA++
				} else if s[1] > s[0] {
					setsB++
				}
			}
		case ScoringGoals, ScoringPoints:
			if m.Score.A != nil && m.Score.B != nil {
				forA, forB = *m.Score.A, *m.Score.B
			}
		}
		apply := func(side []int64, res string, delta, f, ag float64, sw, sl int) {
			for _, uid := range side {
				x := get(uid)
				x.Played++
				x.For += f
				x.Against += ag
				x.SetsWon += sw
				x.SetsLost += sl
				switch res {
				case "W":
					x.Won++
					x.Points += r.Win
				case "D":
					x.Drawn++
					x.Points += r.Draw
				case "L":
					x.Lost++
					x.Points += r.Loss
				}
				x.Form = append(x.Form, res)
				x.Rating += delta
				x.Change = delta
				t.Changes[m.ID][uid] = round1(delta)
			}
		}
		resA, resB := "D", "D"
		switch m.Winner {
		case "a":
			resA, resB = "W", "L"
		case "b":
			resA, resB = "L", "W"
		}
		apply(a, resA, da, forA, forB, setsA, setsB)
		apply(b, resB, -da, forB, forA, setsB, setsA)
		for _, p := range m.Players {
			if p.Score != nil {
				get(p.UserID).Scored += *p.Score
			}
		}
	}
	for _, x := range rows {
		finish(x)
	}
	list := make([]*Row, 0, len(rows))
	for _, x := range rows {
		list = append(list, x)
	}
	sortRows(r, list)
	t.Rows = list
	return t
}

func computeRanking(r Rules, m *store.Match, get func(int64) *Row, changes map[int64]float64) {
	ps := m.Players
	if len(ps) < 2 {
		return
	}
	k := 32.0 / float64(len(ps)-1)
	before := map[int64]float64{}
	for _, p := range ps {
		before[p.UserID] = get(p.UserID).Rating
	}
	best := 0
	for _, p := range ps {
		if best == 0 || (p.Place > 0 && p.Place < best) {
			best = p.Place
		}
	}
	last := 0
	for _, p := range ps {
		if p.Place > last {
			last = p.Place
		}
	}
	for _, p := range ps {
		var d float64
		for _, q := range ps {
			if q.UserID == p.UserID {
				continue
			}
			e := 1 / (1 + math.Pow(10, (before[q.UserID]-before[p.UserID])/400))
			s := 0.5
			if p.Place < q.Place {
				s = 1
			} else if p.Place > q.Place {
				s = 0
			}
			d += k * (s - e)
		}
		x := get(p.UserID)
		x.Played++
		res := "D"
		switch {
		case p.Place == best:
			x.Wins++
			x.Won++
			x.Points += r.Win
			res = "W"
		case p.Place == last:
			x.Lost++
			x.Points += r.Loss
			res = "L"
		default:
			x.Drawn++
			x.Points += r.Draw
		}
		x.Form = append(x.Form, res)
		if p.Score != nil {
			sc := *p.Score
			x.total += sc
			if x.Best == nil || (r.LowerBetter && sc < *x.Best) || (!r.LowerBetter && sc > *x.Best) {
				v := sc
				x.Best = &v
			}
		}
		x.Rating += d
		x.Change = d
		changes[p.UserID] = round1(d)
	}
}

func avgRating(ids []int64, get func(int64) *Row) float64 {
	var s float64
	for _, id := range ids {
		s += get(id).Rating
	}
	return s / float64(len(ids))
}

func finish(x *Row) {
	x.Rating = math.Round(x.Rating)
	x.Change = round1(x.Change)
	if x.Played > 0 && x.Best != nil {
		v := round1(x.total / float64(x.Played))
		x.Average = &v
	}
	if n := len(x.Form); n > 0 {
		last := x.Form[n-1]
		c := 0
		for i := n - 1; i >= 0 && x.Form[i] == last; i-- {
			c++
		}
		x.Streak = fmt.Sprintf("%s%d", last, c)
	}
	if len(x.Form) > 5 {
		x.Form = x.Form[len(x.Form)-5:]
	}
}

func sortRows(r Rules, list []*Row) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if (a.Played == 0) != (b.Played == 0) {
			return a.Played > 0
		}
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		if r.Scoring == ScoringRanking {
			if a.Wins != b.Wins {
				return a.Wins > b.Wins
			}
		} else if da, db := a.For-a.Against, b.For-b.Against; da != db {
			return da > db
		}
		if a.Rating != b.Rating {
			return a.Rating > b.Rating
		}
		return a.UserID < b.UserID
	})
	pos := 0
	for i, x := range list {
		if x.Played == 0 {
			x.Position = 0
			continue
		}
		if i == 0 || x.Points != list[i-1].Points || x.Rating != list[i-1].Rating {
			pos = i + 1
		}
		x.Position = pos
	}
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// Ratings is everyone's current rating from a table.
func (t *Table) Ratings() map[int64]float64 {
	out := map[int64]float64{}
	for _, r := range t.Rows {
		out[r.UserID] = r.Rating
	}
	return out
}

// ---- one against another ----

// Versus is how two people did against each other and together.
type Versus struct {
	UserID       int64  `json:"user_id"`
	Played       int    `json:"played"` // against each other
	Won          int    `json:"won"`
	Drawn        int    `json:"drawn"`
	Lost         int    `json:"lost"`
	Together     int    `json:"together"` // on the same side
	TogetherWon  int    `json:"together_won"`
	LastPlayedAt string `json:"last_played_at"`
}

// HeadToHead is one person's record against and with everyone else.
func HeadToHead(uid int64, matches []*store.Match) []*Versus {
	by := map[int64]*Versus{}
	get := func(id int64) *Versus {
		if v := by[id]; v != nil {
			return v
		}
		v := &Versus{UserID: id}
		by[id] = v
		return v
	}
	for _, m := range matches {
		if m.DisputedBy != 0 {
			continue
		}
		var mine *store.MatchPlayer
		for i := range m.Players {
			if m.Players[i].UserID == uid {
				mine = &m.Players[i]
			}
		}
		if mine == nil {
			continue
		}
		for _, p := range m.Players {
			if p.UserID == uid {
				continue
			}
			v := get(p.UserID)
			if m.PlayedAt > v.LastPlayedAt {
				v.LastPlayedAt = m.PlayedAt
			}
			if mine.Side == "" { // a ranking: compare places
				v.Played++
				switch {
				case mine.Place < p.Place:
					v.Won++
				case mine.Place > p.Place:
					v.Lost++
				default:
					v.Drawn++
				}
				continue
			}
			if p.Side == mine.Side {
				v.Together++
				if m.Winner == mine.Side {
					v.TogetherWon++
				}
				continue
			}
			v.Played++
			switch m.Winner {
			case mine.Side:
				v.Won++
			case "draw":
				v.Drawn++
			default:
				v.Lost++
			}
		}
	}
	out := make([]*Versus, 0, len(by))
	for _, v := range by {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Played+out[i].Together != out[j].Played+out[j].Together {
			return out[i].Played+out[i].Together > out[j].Played+out[j].Together
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

// ---- fair teams ----

// FairTeams splits players into two sides whose ratings are as close as
// possible. Among splits about as fair as the best one it picks at random
// (seed), so "Shuffle" gives a different fair split. keepA are put on side a.
func FairTeams(players []int64, ratings map[int64]float64, seed int64) (a, b []int64, diff float64) {
	n := len(players)
	if n < 2 {
		return append([]int64{}, players...), []int64{}, 0
	}
	rt := func(id int64) float64 {
		if r, ok := ratings[id]; ok {
			return r
		}
		return StartRating
	}
	var total float64
	for _, p := range players {
		total += rt(p)
	}
	half := n / 2
	type split struct {
		mask uint64
		diff float64
	}
	var all []split
	best := math.Inf(1)
	if n <= 20 {
		// every way of picking half of them (the first player always on a, so no mirror twins)
		var walk func(i, left int, mask uint64, sum float64)
		walk = func(i, left int, mask uint64, sum float64) {
			if left == 0 {
				d := math.Abs(2*sum - total)
				if n%2 == 1 {
					// the bigger side is a: compare averages
					d = math.Abs(sum/float64(half) - (total-sum)/float64(n-half))
				}
				all = append(all, split{mask, d})
				if d < best {
					best = d
				}
				return
			}
			if n-i < left {
				return
			}
			walk(i+1, left-1, mask|1<<uint(i), sum+rt(players[i]))
			if i > 0 {
				walk(i+1, left, mask, sum)
			}
		}
		walk(0, n-half, 0, 0)
	} else {
		// many players: snake order by rating, then swaps while they help
		idx := make([]int, n)
		for i := range idx {
			idx[i] = i
		}
		sort.Slice(idx, func(i, j int) bool { return rt(players[idx[i]]) > rt(players[idx[j]]) })
		var mask uint64
		for k, i := range idx {
			if k%4 == 0 || k%4 == 3 {
				mask |= 1 << uint(i)
			}
		}
		all = append(all, split{mask, 0})
		best = 0
	}
	tol := math.Max(best*1.15, best+12)
	var good []split
	for _, s := range all {
		if s.diff <= tol {
			good = append(good, s)
		}
	}
	pick := good[rand.New(rand.NewSource(seed)).Intn(len(good))]
	var sa, sb float64
	for i, p := range players {
		if pick.mask&(1<<uint(i)) != 0 {
			a = append(a, p)
			sa += rt(p)
		} else {
			b = append(b, p)
			sb += rt(p)
		}
	}
	if a == nil {
		a = []int64{}
	}
	if b == nil {
		b = []int64{}
	}
	if len(a) > 0 && len(b) > 0 {
		diff = math.Round(math.Abs(sa/float64(len(a)) - sb/float64(len(b))))
	}
	return a, b, diff
}
