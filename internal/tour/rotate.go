package tour

import (
	"math/rand"
	"sort"
)

// ---- formats where partners change every round ----

// Court is one match of a rotating round: two pairs.
type Court [2][2]int64

// History is who partnered and who played whom so far, and who sat out.
type History struct {
	Partners  map[[2]int64]int
	Opponents map[[2]int64]int
	Sat       map[int64]int
	Played    map[int64]int
}

// NewHistory is an empty history.
func NewHistory() *History {
	return &History{Partners: map[[2]int64]int{}, Opponents: map[[2]int64]int{}, Sat: map[int64]int{}, Played: map[int64]int{}}
}

func key(a, b int64) [2]int64 {
	if a > b {
		a, b = b, a
	}
	return [2]int64{a, b}
}

// Add records a round.
func (h *History) Add(courts []Court, sitting []int64) {
	for _, c := range courts {
		for s := 0; s < 2; s++ {
			h.Partners[key(c[s][0], c[s][1])]++
			h.Played[c[s][0]]++
			h.Played[c[s][1]]++
		}
		for _, x := range c[0] {
			for _, y := range c[1] {
				h.Opponents[key(x, y)]++
			}
		}
	}
	for _, s := range sitting {
		h.Sat[s]++
	}
}

func (h *History) cost(c Court) int {
	n := h.Partners[key(c[0][0], c[0][1])]*10 + h.Partners[key(c[1][0], c[1][1])]*10
	for _, x := range c[0] {
		for _, y := range c[1] {
			n += h.Opponents[key(x, y)] * 2
		}
	}
	return n
}

// sitters picks who sits out this round: those who sat least (then played most).
func sitters(players []int64, n int, h *History, rnd *rand.Rand) (sit, play []int64) {
	ps := append([]int64{}, players...)
	rnd.Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
	sort.SliceStable(ps, func(i, j int) bool {
		if h.Sat[ps[i]] != h.Sat[ps[j]] {
			return h.Sat[ps[i]] < h.Sat[ps[j]]
		}
		return h.Played[ps[i]] > h.Played[ps[j]]
	})
	if n > len(ps) {
		n = len(ps)
	}
	return append([]int64{}, ps[:n]...), append([]int64{}, ps[n:]...)
}

// AmericanoRound makes one round: as many courts as there are four
// players for (at most courts), the fewest repeated partners and
// opponents; the rest sit out in turn.
func AmericanoRound(players []int64, courts int, h *History, seed int64) ([]Court, []int64) {
	rnd := rand.New(rand.NewSource(seed))
	k := len(players) / 4
	if courts > 0 && k > courts {
		k = courts
	}
	if k == 0 {
		return nil, append([]int64{}, players...)
	}
	sit, play := sitters(players, len(players)-4*k, h, rnd)
	var best []Court
	bestCost := 1 << 30
	for try := 0; try < 600; try++ {
		rnd.Shuffle(len(play), func(i, j int) { play[i], play[j] = play[j], play[i] })
		cs := make([]Court, k)
		total := 0
		for c := 0; c < k; c++ {
			q := play[4*c : 4*c+4]
			// the best of the three ways to split four into two pairs
			opts := []Court{{{q[0], q[1]}, {q[2], q[3]}}, {{q[0], q[2]}, {q[1], q[3]}}, {{q[0], q[3]}, {q[1], q[2]}}}
			bc := opts[0]
			for _, o := range opts[1:] {
				if h.cost(o) < h.cost(bc) {
					bc = o
				}
			}
			cs[c] = bc
			total += h.cost(bc)
		}
		if total < bestCost {
			best, bestCost = cs, total
			if total == 0 {
				break
			}
		}
	}
	return best, sit
}

// MexicanoRound makes the next round from the standing (best first):
// groups of four in order, 1st + 3rd against 2nd + 4th (or 1st + 4th
// against 2nd + 3rd when split is "1-4"). The first round is a draw.
func MexicanoRound(ranked []int64, courts int, h *History, split string, seed int64) ([]Court, []int64) {
	rnd := rand.New(rand.NewSource(seed))
	k := len(ranked) / 4
	if courts > 0 && k > courts {
		k = courts
	}
	if k == 0 {
		return nil, append([]int64{}, ranked...)
	}
	sit, _ := sitters(ranked, len(ranked)-4*k, h, rnd)
	out := map[int64]bool{}
	for _, s := range sit {
		out[s] = true
	}
	var play []int64
	for _, p := range ranked {
		if !out[p] {
			play = append(play, p)
		}
	}
	cs := make([]Court, k)
	for c := 0; c < k; c++ {
		q := play[4*c : 4*c+4]
		if split == "1-4" {
			cs[c] = Court{{q[0], q[3]}, {q[1], q[2]}}
		} else {
			cs[c] = Court{{q[0], q[2]}, {q[1], q[3]}}
		}
	}
	return cs, sit
}

// KOTCResult is how a court of the last round went: its pairs and the winner.
type KOTCResult struct {
	Court  Court
	Winner int // 0 = side a, 1 = side b
}

// KOTCRound moves the winners of each court up one court and the losers
// down one (the winners of the top court and the losers of the bottom one
// stay), then splits the pairs that meet so partners change. last is in
// court order, top court first.
func KOTCRound(last []KOTCResult) []Court {
	n := len(last)
	if n == 0 {
		return nil
	}
	arrive := make([][]int64, n)
	for c, r := range last {
		win, lose := r.Court[r.Winner], r.Court[1-r.Winner]
		up, down := c-1, c+1
		if up < 0 {
			up = 0
		}
		if down >= n {
			down = n - 1
		}
		arrive[up] = append(arrive[up], win[0], win[1])
		arrive[down] = append(arrive[down], lose[0], lose[1])
	}
	out := make([]Court, n)
	for c, ps := range arrive {
		if len(ps) < 4 {
			continue
		}
		// the two pairs that meet are split: one of each on a side
		out[c] = Court{{ps[0], ps[2]}, {ps[1], ps[3]}}
	}
	return out
}

// ---- Swiss ----

// SwissEntry is an entry as Swiss pairing sees it.
type SwissEntry struct {
	ID     int64
	Score  float64
	Rating float64
	HadBye bool
	Whites int // chess: games with white so far
	Blacks int
	Last   string // "w" | "b" | ""
	Last2  string // the one before
}

// SwissPair is one pairing; White is "a" or "b" when colours matter.
type SwissPair struct {
	A, B  int64
	White string
}

// SwissRound pairs a round: score groups, the top half of each against
// the bottom half, nobody twice; with an odd number the lowest-ranked
// entry without a bye gets one. colours balances white and black (chess).
func SwissRound(entries []SwissEntry, played map[[2]int64]bool, colours bool) (pairs []SwissPair, bye int64) {
	es := append([]SwissEntry{}, entries...)
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].Score != es[j].Score {
			return es[i].Score > es[j].Score
		}
		return es[i].Rating > es[j].Rating
	})
	if len(es)%2 == 1 {
		for i := len(es) - 1; i >= 0; i-- {
			if !es[i].HadBye {
				bye = es[i].ID
				es = append(es[:i], es[i+1:]...)
				break
			}
		}
		if bye == 0 { // everyone had one: the last gets another
			bye = es[len(es)-1].ID
			es = es[:len(es)-1]
		}
	}
	// the preferred opponent of entry i: the one half a score group below
	half := map[int64]int{}
	for i := 0; i < len(es); {
		j := i
		for j < len(es) && es[j].Score == es[i].Score {
			j++
		}
		for k := i; k < j; k++ {
			half[es[k].ID] = k - i
		}
		i = j
	}
	used := make([]bool, len(es))
	var out [][2]int
	var solve func() bool
	steps := 0
	solve = func() bool {
		steps++
		if steps > 200000 {
			return false
		}
		i := -1
		for k := range es {
			if !used[k] {
				i = k
				break
			}
		}
		if i < 0 {
			return true
		}
		used[i] = true
		// candidates: same score group first (the one about half the group away), then down the list
		var cand []int
		for k := i + 1; k < len(es); k++ {
			if !used[k] && !played[key(es[i].ID, es[k].ID)] {
				cand = append(cand, k)
			}
		}
		groupSize := 0
		for k := range es {
			if es[k].Score == es[i].Score {
				groupSize++
			}
		}
		target := half[es[i].ID] + groupSize/2
		sort.SliceStable(cand, func(x, y int) bool {
			sx, sy := es[cand[x]].Score == es[i].Score, es[cand[y]].Score == es[i].Score
			if sx != sy {
				return sx
			}
			if sx {
				return abs(half[es[cand[x]].ID]-target) < abs(half[es[cand[y]].ID]-target)
			}
			return cand[x] < cand[y]
		})
		for _, k := range cand {
			used[k] = true
			out = append(out, [2]int{i, k})
			if solve() {
				return true
			}
			out = out[:len(out)-1]
			used[k] = false
		}
		used[i] = false
		return false
	}
	if !solve() {
		// nobody new left for someone: allow a rematch, pairing down the list
		out = nil
		for i := 0; i+1 < len(es); i += 2 {
			out = append(out, [2]int{i, i + 1})
		}
	}
	for _, pr := range out {
		a, b := es[pr[0]], es[pr[1]]
		p := SwissPair{A: a.ID, B: b.ID}
		if colours {
			p.White = "a"
			if wantsWhite(b) > wantsWhite(a) {
				p.White = "b"
			}
		}
		pairs = append(pairs, p)
	}
	return pairs, bye
}

// wantsWhite is how much an entry should get white: never three of a colour in a row.
func wantsWhite(e SwissEntry) int {
	if e.Last == "w" && e.Last2 == "w" {
		return -100
	}
	if e.Last == "b" && e.Last2 == "b" {
		return 100
	}
	return (e.Blacks - e.Whites) * 10
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ---- Arena ----

// ArenaPick pairs whoever is free: each with the free one closest in
// score, avoiding the last opponent when there is a choice.
func ArenaPick(free []SwissEntry, last map[int64]int64) [][2]int64 {
	es := append([]SwissEntry{}, free...)
	sort.SliceStable(es, func(i, j int) bool { return es[i].Score > es[j].Score })
	used := make([]bool, len(es))
	var out [][2]int64
	for i := range es {
		if used[i] {
			continue
		}
		pick := -1
		for k := i + 1; k < len(es); k++ {
			if used[k] {
				continue
			}
			if pick < 0 {
				pick = k
			}
			if last[es[i].ID] != es[k].ID {
				pick = k
				break
			}
		}
		if pick < 0 {
			break
		}
		used[i], used[pick] = true, true
		out = append(out, [2]int64{es[i].ID, es[pick].ID})
	}
	return out
}
