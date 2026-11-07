package league

import (
	"strings"
	"testing"
)

func pts(s string) []string {
	out := []string{}
	for _, c := range strings.ReplaceAll(s, " ", "") {
		out = append(out, string(c))
	}
	return out
}

func rep(s string, n int) string { return strings.Repeat(s, n) }

func TestLiveTennis(t *testing.T) {
	r := LiveRulesFor("tennis", ScoringSets)
	v := Live(r, pts("aab"))
	if v.Point != [2]string{"30", "15"} {
		t.Fatalf("30–15: %v", v.Point)
	}
	v = Live(r, pts("aaabbb"))
	if !v.Deuce {
		t.Fatalf("deuce: %+v", v)
	}
	v = Live(r, pts("aaabbba"))
	if v.Point != [2]string{"AD", ""} {
		t.Fatalf("advantage: %v", v.Point)
	}
	// 6–4 6–4: four points a game
	game := func(side string) string { return rep(side, 4) }
	set64 := rep(game("a")+game("b"), 4) + game("a") + game("a")
	v = Live(r, pts(set64+set64))
	if !v.Done || v.Winner != "a" || len(v.Sets) != 2 || v.Sets[0] != [2]int{6, 4} {
		t.Fatalf("6–4 6–4: %+v", v)
	}
	if line := LiveLine(r, v); line != "6–4 6–4" {
		t.Fatalf("line: %q", line)
	}
	// 6–6: a tie-break to 7
	six := rep(game("a")+game("b"), 6)
	v = Live(r, pts(six))
	if !v.TieBreak || v.Cur != [2]int{6, 6} {
		t.Fatalf("tie-break: %+v", v)
	}
	v = Live(r, pts(six+rep("a", 6)))
	if v.SetPoint != "a" {
		t.Fatalf("set point in the tie-break: %+v", v)
	}
	v = Live(r, pts(six+rep("a", 7)))
	if len(v.Sets) != 1 || v.Sets[0] != [2]int{7, 6} {
		t.Fatalf("7–6: %+v", v)
	}
	// golden point (padel): at 40–40 the next point wins
	p := LiveRulesFor("padel", ScoringSets)
	v = Live(p, pts("aaabbbb"))
	if v.Cur != [2]int{0, 1} {
		t.Fatalf("golden point: %+v", v)
	}
	// a deciding super tie-break to 10
	sr := r
	sr.SuperFinal = true
	set46 := rep(game("b")+game("a"), 4) + game("b") + game("b")
	v = Live(sr, pts(set64+set46+rep("a", 10)))
	if !v.Done || v.Sets[2] != [2]int{10, 0} {
		t.Fatalf("super tie-break: %+v", v)
	}
}

func TestLiveRallyAndCount(t *testing.T) {
	r := LiveRulesFor("badminton", ScoringSets)
	v := Live(r, pts(rep("ab", 20)+"a"))
	if v.SetPoint != "a" || v.Cur != [2]int{21, 20} {
		t.Fatalf("21–20 is not a set: %+v", v)
	}
	v = Live(r, pts(rep("ab", 29)+"a"))
	if len(v.Sets) != 1 || v.Sets[0] != [2]int{30, 29} {
		t.Fatalf("capped at 30: %+v", v)
	}
	vb := LiveRulesFor("volleyball", ScoringSets)
	set := rep("a", 25)
	lose := rep("b", 25)
	v = Live(vb, pts(set+set+lose+lose+rep("a", 15)))
	if !v.Done || v.Sets[4] != [2]int{15, 0} {
		t.Fatalf("fifth set to 15: %+v", v)
	}
	c := LiveRulesFor("basketball", ScoringPoints)
	v = Live(c, []string{"a2", "b3", "a", "a3", "x"})
	if v.Cur != [2]int{6, 3} || *v.Score.A != 6 || v.Winner != "a" {
		t.Fatalf("count: %+v", v)
	}
}
