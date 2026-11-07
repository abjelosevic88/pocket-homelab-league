// Package tour draws tournaments: knockout brackets (with a third place
// match, a plate, a losers' bracket), round robins, groups, Swiss rounds,
// the rotating-partner formats (Americano, Mexicano, King of the Court),
// and the table with each sport's tie-breakers. It knows nothing about the
// database: a Plan is matches with links between them, which the store
// saves and then follows (a winner moves to next, a loser to loser).
package tour

import (
	"fmt"
	"math/bits"
)

// Slot is one side of a planned match: an entry, or where it comes from.
type Slot struct {
	Entry  int64  // 0 = not known yet
	Source string // "Winner of 12", "Group A, 1st", "bye"
}

// Bye is the source of an empty side.
const Bye = "bye"

// PlanGroup is a pool or a bracket of a stage.
type PlanGroup struct {
	Name string
	Kind string // pool | main | third | plate | wb | lb | final
}

// PlanMatch is a match before it is saved; Next and Loser are indexes
// into Plan.Matches (-1 = none).
type PlanMatch struct {
	Group     int
	Round     int
	Position  int
	Label     string
	A, B      Slot
	Players   [2][]int64 // rotating formats: who plays on each side
	Next      int
	NextSlot  string
	Loser     int
	LoserSlot string
}

// Plan is what a draw makes.
type Plan struct {
	Groups  []PlanGroup
	Matches []PlanMatch
}

func (p *Plan) add(m PlanMatch) int {
	if m.Next == 0 {
		m.Next = -1
	}
	if m.Loser == 0 {
		m.Loser = -1
	}
	p.Matches = append(p.Matches, m)
	return len(p.Matches) - 1
}

// SeedOrder is the standard order of seeds in a bracket of size n (a power
// of two): seeds 1 and 2 can only meet in the final, 1–4 in the semis…
func SeedOrder(n int) []int {
	order := []int{1, 2}
	for len(order) < n {
		m := len(order) * 2
		next := make([]int, 0, m)
		for _, s := range order {
			next = append(next, s, m+1-s)
		}
		order = next
	}
	if n < 2 {
		return []int{1}
	}
	return order
}

// pow2 is the smallest power of two ≥ n (at least 2).
func pow2(n int) int {
	if n <= 2 {
		return 2
	}
	return 1 << bits.Len(uint(n-1))
}

func roundName(roundsLeft, i, of int) string {
	n := ""
	if of > 1 {
		n = fmt.Sprintf(" %d", i+1)
	}
	switch roundsLeft {
	case 0:
		return "Final"
	case 1:
		return "Semi-final" + n
	case 2:
		return "Quarter-final" + n
	}
	return ""
}

// Knockout draws a bracket for entries in seed order (best first). Byes go
// to the top seeds. Options add a match for third place and a plate for
// the losers of the first round.
func Knockout(seeded []int64, thirdPlace, plate bool) Plan {
	var p Plan
	p.Groups = append(p.Groups, PlanGroup{Name: "Main draw", Kind: "main"})
	slots := make([]Slot, len(seeded))
	for i, e := range seeded {
		slots[i] = Slot{Entry: e}
	}
	first, final := bracket(&p, 0, slots, "")
	if thirdPlace && len(final) > 0 {
		// the semi-finals feed their losers into the match for third place
		semis := semisOf(&p, final[0])
		if len(semis) == 2 {
			p.Groups = append(p.Groups, PlanGroup{Name: "Third place", Kind: "third"})
			t := p.add(PlanMatch{Group: len(p.Groups) - 1, Round: p.Matches[final[0]].Round, Label: "Third place",
				A: Slot{Source: "Loser of semi-final 1"}, B: Slot{Source: "Loser of semi-final 2"}})
			p.Matches[semis[0]].Loser, p.Matches[semis[0]].LoserSlot = t, "a"
			p.Matches[semis[1]].Loser, p.Matches[semis[1]].LoserSlot = t, "b"
		}
	}
	if plate {
		// the losers of real first-round matches play on in the plate
		var real []int
		for _, i := range first {
			if p.Matches[i].A.Source != Bye && p.Matches[i].B.Source != Bye {
				real = append(real, i)
			}
		}
		if len(real) >= 2 {
			p.Groups = append(p.Groups, PlanGroup{Name: "Plate", Kind: "plate"})
			g := len(p.Groups) - 1
			ps := make([]Slot, len(real))
			for k := range real {
				ps[k] = Slot{Source: fmt.Sprintf("Loser of match %d", k+1)}
			}
			pfirst, _ := bracket(&p, g, ps, "Plate ")
			// wire each first-round loser to its plate slot (in the same seeded order)
			slotOf := plateSlots(&p, pfirst)
			for k, mi := range real {
				if k < len(slotOf) {
					p.Matches[mi].Loser, p.Matches[mi].LoserSlot = slotOf[k].match, slotOf[k].side
				}
			}
		}
	}
	return p
}

type slotRef struct {
	match int
	side  string
}

// plateSlots lists the first-round slots of a bracket in seed order.
func plateSlots(p *Plan, first []int) []slotRef {
	n := len(first) * 2
	order := SeedOrder(n)
	out := make([]slotRef, n)
	for i, s := range order {
		side := "a"
		if i%2 == 1 {
			side = "b"
		}
		out[s-1] = slotRef{first[i/2], side}
	}
	// only slots that were not byes
	var real []slotRef
	for _, r := range out {
		m := p.Matches[r.match]
		src := m.A.Source
		if r.side == "b" {
			src = m.B.Source
		}
		if src != Bye {
			real = append(real, r)
		}
	}
	return real
}

func semisOf(p *Plan, final int) []int {
	var out []int
	for i, m := range p.Matches {
		if m.Next == final {
			out = append(out, i)
		}
	}
	return out
}

// bracket adds a single-elimination bracket for slots in seed order to
// group g; it returns the first-round matches and the final.
func bracket(p *Plan, g int, seeded []Slot, prefix string) (first []int, final []int) {
	n := len(seeded)
	if n < 2 {
		return nil, nil
	}
	size := pow2(n)
	order := SeedOrder(size)
	rounds := bits.Len(uint(size)) - 1
	prev := []int{}
	for i := 0; i < size/2; i++ {
		a, b := order[2*i], order[2*i+1]
		sa, sb := Slot{Source: Bye}, Slot{Source: Bye}
		if a <= n {
			sa = seeded[a-1]
		}
		if b <= n {
			sb = seeded[b-1]
		}
		lbl := roundName(rounds-1, i, size/2)
		if lbl == "" {
			lbl = fmt.Sprintf("Round 1, match %d", i+1)
		}
		prev = append(prev, p.add(PlanMatch{Group: g, Round: 1, Position: i + 1, Label: prefix + lbl, A: sa, B: sb}))
	}
	first = prev
	for r := 2; r <= rounds; r++ {
		var cur []int
		for i := 0; i < len(prev)/2; i++ {
			lbl := roundName(rounds-r, i, len(prev)/2)
			if lbl == "" {
				lbl = fmt.Sprintf("Round %d, match %d", r, i+1)
			}
			m := p.add(PlanMatch{Group: g, Round: r, Position: i + 1, Label: prefix + lbl,
				A: Slot{Source: "Winner of " + p.Matches[prev[2*i]].Label}, B: Slot{Source: "Winner of " + p.Matches[prev[2*i+1]].Label}})
			p.Matches[prev[2*i]].Next, p.Matches[prev[2*i]].NextSlot = m, "a"
			p.Matches[prev[2*i+1]].Next, p.Matches[prev[2*i+1]].NextSlot = m, "b"
			cur = append(cur, m)
		}
		prev = cur
	}
	return first, prev
}

// DoubleElimination draws a winners' bracket, a losers' bracket where
// losers drop in crossed order (so rematches come late), and a grand final
// (a second one is added when reset is on: if the losers' side wins the
// first, both have lost once).
func DoubleElimination(seeded []int64, reset bool) Plan {
	var p Plan
	p.Groups = []PlanGroup{{Name: "Winners' bracket", Kind: "wb"}, {Name: "Losers' bracket", Kind: "lb"}, {Name: "Grand final", Kind: "final"}}
	slots := make([]Slot, len(seeded))
	for i, e := range seeded {
		slots[i] = Slot{Entry: e}
	}
	if len(seeded) < 3 {
		_, fin := bracket(&p, 0, slots, "")
		_ = fin
		return p
	}
	// the winners' bracket, by round
	size := pow2(len(seeded))
	rounds := bits.Len(uint(size)) - 1
	first, wbFinal := bracket(&p, 0, slots, "Winners ")
	wb := [][]int{first}
	for r := 2; r <= rounds; r++ {
		var cur []int
		for i, m := range p.Matches {
			if m.Group == 0 && m.Round == r {
				cur = append(cur, i)
			}
		}
		wb = append(wb, cur)
	}
	// the losers' bracket
	lbRound := 1
	var lbPrev []int
	// round 1: the losers of the first winners' round play each other
	for i := 0; i < len(wb[0])/2; i++ {
		m := p.add(PlanMatch{Group: 1, Round: lbRound, Position: i + 1, Label: fmt.Sprintf("Losers round 1, match %d", i+1),
			A: Slot{Source: "Loser of " + p.Matches[wb[0][2*i]].Label}, B: Slot{Source: "Loser of " + p.Matches[wb[0][2*i+1]].Label}})
		p.Matches[wb[0][2*i]].Loser, p.Matches[wb[0][2*i]].LoserSlot = m, "a"
		p.Matches[wb[0][2*i+1]].Loser, p.Matches[wb[0][2*i+1]].LoserSlot = m, "b"
		lbPrev = append(lbPrev, m)
	}
	for r := 1; r < len(wb); r++ {
		// a drop round: the losers' bracket winners meet this round's winners' losers, crossed
		lbRound++
		var drop []int
		k := len(lbPrev)
		for i := 0; i < k; i++ {
			from := wb[r][k-1-i]
			if r%2 == 0 {
				from = wb[r][i]
			}
			m := p.add(PlanMatch{Group: 1, Round: lbRound, Position: i + 1, Label: fmt.Sprintf("Losers round %d, match %d", lbRound, i+1),
				A: Slot{Source: "Winner of " + p.Matches[lbPrev[i]].Label}, B: Slot{Source: "Loser of " + p.Matches[from].Label}})
			p.Matches[lbPrev[i]].Next, p.Matches[lbPrev[i]].NextSlot = m, "a"
			p.Matches[from].Loser, p.Matches[from].LoserSlot = m, "b"
			drop = append(drop, m)
		}
		lbPrev = drop
		if len(lbPrev) > 1 {
			// a round among the losers' bracket winners
			lbRound++
			var cur []int
			for i := 0; i < len(lbPrev)/2; i++ {
				m := p.add(PlanMatch{Group: 1, Round: lbRound, Position: i + 1, Label: fmt.Sprintf("Losers round %d, match %d", lbRound, i+1),
					A: Slot{Source: "Winner of " + p.Matches[lbPrev[2*i]].Label}, B: Slot{Source: "Winner of " + p.Matches[lbPrev[2*i+1]].Label}})
				p.Matches[lbPrev[2*i]].Next, p.Matches[lbPrev[2*i]].NextSlot = m, "a"
				p.Matches[lbPrev[2*i+1]].Next, p.Matches[lbPrev[2*i+1]].NextSlot = m, "b"
				cur = append(cur, m)
			}
			lbPrev = cur
		}
	}
	if len(lbPrev) == 1 {
		p.Matches[lbPrev[0]].Label = "Losers' final"
	}
	// the grand final
	gf := p.add(PlanMatch{Group: 2, Round: 1, Position: 1, Label: "Grand final",
		A: Slot{Source: "Winner of the winners' bracket"}, B: Slot{Source: "Winner of the losers' bracket"}})
	p.Matches[wbFinal[0]].Next, p.Matches[wbFinal[0]].NextSlot = gf, "a"
	p.Matches[wbFinal[0]].Label = "Winners' final"
	if len(lbPrev) == 1 {
		p.Matches[lbPrev[0]].Next, p.Matches[lbPrev[0]].NextSlot = gf, "b"
	}
	if reset {
		p.add(PlanMatch{Group: 2, Round: 2, Position: 1, Label: "Grand final, reset",
			A: Slot{Source: "Only if the losers' side wins the grand final"}, B: Slot{Source: "Only if the losers' side wins the grand final"}})
	}
	return p
}

// Snake deals entries in seed order into k groups: 1 2 3 4 4 3 2 1 …
func Snake(seeded []int64, k int) [][]int64 {
	if k < 1 {
		k = 1
	}
	out := make([][]int64, k)
	for i, e := range seeded {
		row, col := i/k, i%k
		if row%2 == 1 {
			col = k - 1 - col
		}
		out[col] = append(out[col], e)
	}
	return out
}

// GroupName is "Group A", "Group B" …
func GroupName(i int) string {
	if i < 26 {
		return "Group " + string(rune('A'+i))
	}
	return fmt.Sprintf("Group %d", i+1)
}

// RoundRobin draws every entry against every other in each group (twice,
// home and away, when double), round by round with the circle method; with
// an odd number someone sits out each round.
func RoundRobin(groups [][]int64, names []string, double bool) Plan {
	var p Plan
	for gi, ids := range groups {
		name := GroupName(gi)
		if gi < len(names) && names[gi] != "" {
			name = names[gi]
		}
		p.Groups = append(p.Groups, PlanGroup{Name: name, Kind: "pool"})
		rounds := Circle(ids)
		pos := 0
		for r, pairs := range rounds {
			for _, pr := range pairs {
				pos++
				p.add(PlanMatch{Group: gi, Round: r + 1, Position: pos, A: Slot{Entry: pr[0]}, B: Slot{Entry: pr[1]}})
			}
		}
		if double {
			n := len(rounds)
			for r, pairs := range rounds {
				for _, pr := range pairs {
					pos++
					p.add(PlanMatch{Group: gi, Round: n + r + 1, Position: pos, A: Slot{Entry: pr[1]}, B: Slot{Entry: pr[0]}})
				}
			}
		}
	}
	return p
}

// Circle is the circle (Berger) method: rounds of pairs, nobody twice.
func Circle(ids []int64) [][][2]int64 {
	list := append([]int64{}, ids...)
	if len(list)%2 == 1 {
		list = append(list, 0)
	}
	n := len(list)
	if n < 2 {
		return nil
	}
	var out [][][2]int64
	for r := 0; r < n-1; r++ {
		var pairs [][2]int64
		for i := 0; i < n/2; i++ {
			a, b := list[i], list[n-1-i]
			if a == 0 || b == 0 {
				continue
			}
			if r%2 == 1 && i == 0 {
				a, b = b, a // the fixed one swaps sides every other round
			}
			pairs = append(pairs, [2]int64{a, b})
		}
		out = append(out, pairs)
		// keep the first, rotate the rest clockwise
		last := list[n-1]
		copy(list[2:], list[1:n-1])
		list[1] = last
	}
	return out
}
