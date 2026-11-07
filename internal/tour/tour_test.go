package tour

import (
	"reflect"
	"testing"
)

func TestSeedOrderAndKnockout(t *testing.T) {
	if got := SeedOrder(8); !reflect.DeepEqual(got, []int{1, 8, 4, 5, 2, 7, 3, 6}) {
		t.Fatalf("seed order: %v", got)
	}
	p := Knockout([]int64{11, 12, 13, 14, 15, 16}, true, true)
	byes, final, third := 0, 0, 0
	for _, m := range p.Matches {
		if m.Round == 1 && m.Group == 0 && (m.A.Source == Bye || m.B.Source == Bye) {
			byes++
			if m.A.Entry != 11 && m.A.Entry != 12 {
				t.Fatalf("a bye not for a top seed: %+v", m)
			}
		}
		if m.Label == "Final" {
			final++
		}
		if m.Label == "Third place" {
			third++
		}
	}
	if byes != 2 || final != 1 || third != 1 {
		t.Fatalf("6 entries: %d byes, %d finals, %d third place", byes, final, third)
	}
}

func TestDoubleEliminationShape(t *testing.T) {
	p := DoubleElimination([]int64{1, 2, 3, 4, 5, 6, 7, 8}, true)
	wb, lb, gf := 0, 0, 0
	for _, m := range p.Matches {
		switch m.Group {
		case 0:
			wb++
		case 1:
			lb++
		case 2:
			gf++
		}
	}
	// 8 entries: 7 in the winners' bracket, 6 in the losers', a grand final and its reset
	if wb != 7 || lb != 6 || gf != 2 {
		t.Fatalf("double elimination of 8: wb %d lb %d gf %d", wb, lb, gf)
	}
	// every winners' match but the final sends its loser somewhere
	for i, m := range p.Matches {
		if m.Group == 0 && m.Label != "Winners' final" && m.Loser < 0 {
			t.Fatalf("match %d (%s) loses nobody to the losers' bracket", i, m.Label)
		}
	}
}

func TestCircleEveryoneOnce(t *testing.T) {
	for _, n := range []int{3, 4, 5, 6, 7} {
		ids := make([]int64, n)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		seen := map[[2]int64]int{}
		for _, r := range Circle(ids) {
			in := map[int64]bool{}
			for _, pr := range r {
				if in[pr[0]] || in[pr[1]] {
					t.Fatalf("n=%d: someone twice in a round", n)
				}
				in[pr[0]], in[pr[1]] = true, true
				seen[key(pr[0], pr[1])]++
			}
		}
		if len(seen) != n*(n-1)/2 {
			t.Fatalf("n=%d: %d pairs", n, len(seen))
		}
		for k, c := range seen {
			if c != 1 {
				t.Fatalf("n=%d: %v met %d times", n, k, c)
			}
		}
	}
}

func TestAmericanoEightPlayersPartnersVary(t *testing.T) {
	ps := []int64{1, 2, 3, 4, 5, 6, 7, 8}
	h := NewHistory()
	for r := 0; r < 7; r++ {
		cs, sit := AmericanoRound(ps, 2, h, int64(r+1))
		if len(cs) != 2 || len(sit) != 0 {
			t.Fatalf("round %d: %d courts, %d sitting", r, len(cs), len(sit))
		}
		h.Add(cs, sit)
	}
	repeats := 0
	for _, n := range h.Partners {
		if n > 1 {
			repeats += n - 1
		}
	}
	if repeats > 3 {
		t.Fatalf("too many repeated partners over 7 rounds: %d", repeats)
	}
	// 10 players, 2 courts: two sit out each round, everyone in turn
	ten := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	h2 := NewHistory()
	for r := 0; r < 5; r++ {
		cs, sit := AmericanoRound(ten, 2, h2, int64(r))
		h2.Add(cs, sit)
	}
	for _, p := range ten {
		if h2.Sat[p] != 1 {
			t.Fatalf("player %d sat %d times in 5 rounds", p, h2.Sat[p])
		}
	}
}

func TestSwissNoRematches(t *testing.T) {
	es := []SwissEntry{{ID: 1, Rating: 1500}, {ID: 2, Rating: 1400}, {ID: 3, Rating: 1300}, {ID: 4, Rating: 1200}, {ID: 5, Rating: 1100}}
	played := map[[2]int64]bool{}
	score := map[int64]float64{}
	byes := map[int64]bool{}
	for r := 0; r < 4; r++ {
		for i := range es {
			es[i].Score, es[i].HadBye = score[es[i].ID], byes[es[i].ID]
		}
		pairs, bye := SwissRound(es, played, true)
		if bye == 0 || byes[bye] {
			t.Fatalf("round %d: bye %d (had one: %v)", r, bye, byes[bye])
		}
		byes[bye] = true
		score[bye]++
		for _, p := range pairs {
			if played[key(p.A, p.B)] {
				t.Fatalf("round %d: rematch %v", r, p)
			}
			played[key(p.A, p.B)] = true
			score[p.A]++ // the first-listed wins, whatever
		}
	}
}

func TestStandingsTiebreaks(t *testing.T) {
	// three teams level on points: goal difference decides; two of them level on that too: head-to-head
	res := []Result{
		{A: []int64{1}, B: []int64{2}, Winner: "a", ForA: 2, ForB: 1},
		{A: []int64{2}, B: []int64{3}, Winner: "a", ForA: 2, ForB: 1},
		{A: []int64{3}, B: []int64{1}, Winner: "a", ForA: 3, ForB: 1},
	}
	rows := Standings([]int64{1, 2, 3}, res, RulesFor("football", "goals", "round_robin"), nil)
	if rows[0].ID != 3 || rows[0].DecidedBy == "" {
		t.Fatalf("first: %+v", rows[0])
	}
	// volleyball: a 3–2 win is 2 points, the loser gets 1
	v := Standings([]int64{1, 2}, []Result{{A: []int64{1}, B: []int64{2}, Winner: "a", SetsA: 3, SetsB: 2}}, RulesFor("volleyball", "sets", "round_robin"), nil)
	if v[0].Points != 2 || v[1].Points != 1 {
		t.Fatalf("volleyball 3–2: %v / %v", v[0].Points, v[1].Points)
	}
	// Americano: every player gets their side's points
	a := Standings([]int64{1, 2, 3, 4}, []Result{{A: []int64{1, 2}, B: []int64{3, 4}, Winner: "a", ForA: 14, ForB: 10}}, RulesFor("padel", "sets", "americano"), nil)
	if a[0].Points != 14 || a[3].Points != 10 {
		t.Fatalf("americano: %v", a)
	}
}

func TestGolfStablefordAndCountback(t *testing.T) {
	c := Course{Par: []int{4, 3, 5}, SI: []int{1, 3, 2}}
	if StrokesReceived(4, 1, 3) != 2 || StrokesReceived(4, 3, 3) != 1 {
		t.Fatal("strokes received")
	}
	cards := []Card{{Entry: 1, Round: 1, Holes: []float64{5, 3, 5}}, {Entry: 2, Round: 1, Holes: []float64{4, 4, 5}}}
	lb := Leaderboard([]int64{1, 2}, cards, "stableford", c, map[int64]float64{1: 3, 2: 0}, 1, false)
	if lb[0].ID != 1 || lb[0].Total != 8 || lb[1].Total != 5 {
		t.Fatalf("stableford: %+v %+v", lb[0], lb[1])
	}
	if BowlingHandicap(180, 220, 0.9) != 36 {
		t.Fatal("bowling handicap")
	}
	if p, _ := ArenaPoints([]string{"W", "W", "W", "L", "D"}); p != 9 {
		t.Fatalf("arena points: %v", p)
	}
}
